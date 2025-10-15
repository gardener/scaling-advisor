// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gardener/scaling-advisor/minkapi/view/eventsink"
	"github.com/gardener/scaling-advisor/minkapi/view/inmclient"
	"github.com/gardener/scaling-advisor/minkapi/view/store"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/common/clientutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/watchutil"
	"github.com/go-logr/logr"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

var _ minkapi.View = (*sandboxView)(nil)

type sandboxView struct {
	delegateView minkapi.View
	eventSink    minkapi.EventSink
	args         *minkapi.ViewArgs
	mu           *sync.RWMutex
	stores       map[schema.GroupVersionKind]*store.InMemResourceStore
	changeCount  atomic.Int64
}

// NewSandbox returns a "sandbox" (private) view which holds changes made via its facade into its private store independent of the base view,
// otherwise delegating to the delegate View.
func NewSandbox(delegateView minkapi.View, args *minkapi.ViewArgs) (minkapi.View, error) {
	stores := map[schema.GroupVersionKind]*store.InMemResourceStore{}
	for _, d := range typeinfo.SupportedDescriptors {
		baseStore, err := delegateView.GetResourceStore(d.GVK)
		if err != nil {
			return nil, err
		}
		stores[d.GVK] = createInMemStore(d, baseStore.GetVersionCounter(), args)
	}
	eventSink := eventsink.New()
	return &sandboxView{
		args:         args,
		stores:       stores,
		mu:           &sync.RWMutex{},
		eventSink:    eventSink,
		delegateView: delegateView,
	}, nil
}

func (v *sandboxView) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	resetStores(v.stores)
	v.changeCount.Store(0)
	v.eventSink.Reset()
}

func (v *sandboxView) Close() error {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return closeStores(v.stores)
}

func (v *sandboxView) GetName() string {
	return v.args.Name
}

func (v *sandboxView) GetType() minkapi.ViewType {
	return minkapi.SandboxViewType
}

func (v *sandboxView) GetObjectChangeCount() int64 {
	return v.changeCount.Load()
}

func (v *sandboxView) SetKubeConfigPath(path string) {
	v.args.KubeConfigPath = path
}

func (v *sandboxView) GetClientFacades(ctx context.Context, accessMode commontypes.ClientAccessMode) (clientFacades commontypes.ClientFacades, err error) {
	log := logr.FromContextOrDiscard(ctx)
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", minkapi.ErrClientFacadesFailed, err)
		}
	}()
	switch accessMode {
	case commontypes.ClientAccessNetwork:
		clientFacades, err = clientutil.CreateNetworkClientFacades(log, v.GetKubeConfigPath(), v.args.WatchConfig.Timeout)
	case commontypes.ClientAccessInMemory:
		clientFacades = inmclient.NewInMemClientFacades(v, v.args.WatchConfig.Timeout)
	default:
		err = fmt.Errorf("invalid access mode %q", accessMode)
	}
	return
}

func (v *sandboxView) GetResourceStore(gvk schema.GroupVersionKind) (minkapi.ResourceStore, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	s, exists := v.stores[gvk]
	if !exists {
		return nil, fmt.Errorf("%w: store not found for GVK %q in view %q", minkapi.ErrStoreNotFound, gvk, v.args.Name)
	}
	return s, nil
}

func (v *sandboxView) GetEventSink() minkapi.EventSink {
	return v.eventSink
}

func (v *sandboxView) CreateObject(ctx context.Context, gvk schema.GroupVersionKind, obj metav1.Object) (metav1.Object, error) {
	return storeObject(ctx, v, gvk, obj, &v.changeCount)
}

func (v *sandboxView) GetObject(ctx context.Context, gvk schema.GroupVersionKind, objName cache.ObjectName) (obj runtime.Object, err error) {
	obj, err = v.getSandboxObject(ctx, gvk, objName)
	if obj != nil || !apierrors.IsNotFound(err) {
		// return if I found the object or get an error other than not found error
		return
	}
	obj, err = v.delegateView.GetObject(ctx, gvk, objName)
	return
}

func (v *sandboxView) getSandboxObject(ctx context.Context, gvk schema.GroupVersionKind, objName cache.ObjectName) (obj runtime.Object, err error) {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return
	}
	obj, err = s.GetByKey(ctx, objName.String())
	return
}

func (v *sandboxView) UpdateObject(ctx context.Context, gvk schema.GroupVersionKind, obj metav1.Object) error {
	objName := objutil.CacheName(obj)
	sandboxObj, err := v.getSandboxObject(ctx, gvk, objName)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if sandboxObj != nil { //sandbox object is being updated.
		return updateObject(ctx, v, gvk, obj, &v.changeCount)
	}
	// The object is in base view and should not be modified - store in sandbox view now.
	_, err = v.CreateObject(ctx, gvk, obj)
	return err
}

func (v *sandboxView) UpdatePodNodeBinding(ctx context.Context, podName cache.ObjectName, binding corev1.Binding) (pod *corev1.Pod, err error) {
	gvk := typeinfo.PodsDescriptor.GVK
	obj, err := v.getSandboxObject(ctx, gvk, podName) // get pod from sandbox first.
	if err != nil && !apierrors.IsNotFound(err) {
		return
	}
	// TODO: make the below a bit generic later using functional programming
	var ok bool
	if obj != nil { // pod is found in sandbox view update pod node binding directly
		pod, ok = obj.(*corev1.Pod)
		if !ok {
			err = fmt.Errorf("%w: cannot update pod node binding in %q view since obj %T for name %q not a corev1.Pod", minkapi.ErrUpdateObject, v.GetName(), obj, podName)
			return
		}
		return updatePodNodeBinding(ctx, v, pod, binding)
	}
	// pod is not found in sandbox. now get from base
	obj, err = v.delegateView.GetObject(ctx, gvk, podName)
	if err != nil {
		return
	}
	pod, ok = obj.(*corev1.Pod)
	if !ok {
		err = fmt.Errorf("%w: cannot update pod node binding in %q view since obj %T for name %q not a corev1.Pod", minkapi.ErrUpdateObject, v.GetName(), obj, podName)
		if err != nil {
			return
		}
	}
	// found in base so lets make a copy and store in sandbox
	sandboxPod := pod.DeepCopy()
	_, err = v.CreateObject(ctx, gvk, sandboxPod)
	if err != nil {
		return
	}
	pod = sandboxPod
	return updatePodNodeBinding(ctx, v, pod, binding)
}

func (v *sandboxView) PatchObject(ctx context.Context, gvk schema.GroupVersionKind, objName cache.ObjectName, patchType types.PatchType, patchData []byte) (patchedObj runtime.Object, err error) {
	return patchObject(ctx, v, gvk, objName, patchType, patchData)
}

func (v *sandboxView) PatchObjectStatus(ctx context.Context, gvk schema.GroupVersionKind, objName cache.ObjectName, patchData []byte) (patchedObj runtime.Object, err error) {
	return patchObjectStatus(ctx, v, gvk, objName, patchData)
}

func (v *sandboxView) ListMetaObjects(ctx context.Context, gvk schema.GroupVersionKind, criteria minkapi.MatchCriteria) (items []metav1.Object, maxVersion int64, err error) {
	sandboxItems, myMax, err := listMetaObjects(ctx, v, gvk, criteria)
	if err != nil {
		return
	}
	delegateItems, delegateMax, err := v.delegateView.ListMetaObjects(ctx, gvk, criteria)
	if err != nil {
		return
	}
	if myMax >= delegateMax {
		maxVersion = myMax
	} else {
		maxVersion = delegateMax
	}
	items = combinePrimarySecondary(sandboxItems, delegateItems)
	return
}

func (v *sandboxView) ListObjects(ctx context.Context, gvk schema.GroupVersionKind, criteria minkapi.MatchCriteria) (listObj runtime.Object, err error) {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return
	}
	items, maxVersion, err := v.ListMetaObjects(ctx, gvk, criteria) // v.ListMetaObjcts already invokes delegate
	if err != nil {
		return
	}
	objGVK, objListKind := s.GetObjAndListGVK()
	return store.WrapMetaObjectsIntoRuntimeListObject(maxVersion, objGVK, objListKind, items)
}

func (v *sandboxView) WatchObjects(ctx context.Context, gvk schema.GroupVersionKind, startVersion int64, namespace string, labelSelector labels.Selector, eventCallback minkapi.WatchEventCallback) error {
	log := logr.FromContextOrDiscard(ctx)
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return err
	}
	var eg errgroup.Group
	eg.Go(func() error {
		log.Info("watching sandboxView objects", "gvk", gvk, "startVersion", startVersion, "namespace", namespace, "labelSelector", labelSelector)
		return s.Watch(ctx, startVersion, namespace, labelSelector, eventCallback)
	})
	eg.Go(func() error {
		log.Info("watching delegateView objects", "gvk", gvk, "startVersion", startVersion, "namespace", namespace, "labelSelector", labelSelector)
		return v.delegateView.WatchObjects(ctx, gvk, startVersion, namespace, labelSelector, eventCallback)
	})
	return eg.Wait()
}

func (v *sandboxView) GetWatcher(ctx context.Context, gvk schema.GroupVersionKind, namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	log := logr.FromContextOrDiscard(ctx)
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return nil, err
	}
	w1, err := s.GetWatcher(ctx, namespace, opts)
	if err != nil {
		return nil, err
	}
	log.V(4).Info("got watcher for sandboxView objects", "gvk", gvk, "namespace", namespace, "opts", opts)

	w2, err := v.delegateView.GetWatcher(ctx, gvk, namespace, opts)
	if err != nil {
		return nil, err
	}
	log.V(4).Info("got watcher for delegateView objects", "gvk", gvk, "namespace", namespace, "opts", opts, "delegateViewName", v.delegateView.GetName())
	eventWatcher := watchutil.CombineTwoWatchers(ctx, w1, w2)
	log.Info("returning combined watcher for sandboxView+delegateView objects", "gvk", gvk, "namespace", namespace, "opts", opts, "delegateViewName", v.delegateView.GetName())
	return eventWatcher, nil
}

func (v *sandboxView) DeleteObject(ctx context.Context, gvk schema.GroupVersionKind, objName cache.ObjectName) error {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return err
	}
	obj, err := s.GetByKey(ctx, objName.String())
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if obj == nil {
		// delegate to delegateView if obj not found in sandbox
		return v.delegateView.DeleteObject(ctx, gvk, objName)
	}
	// if found in this views store, delete and return
	err = s.Delete(ctx, objName)
	if err != nil {
		return err
	}
	v.changeCount.Add(1)
	return nil
}

func (v *sandboxView) DeleteObjects(ctx context.Context, gvk schema.GroupVersionKind, criteria minkapi.MatchCriteria) error {
	err := deleteObjects(ctx, v, gvk, criteria, &v.changeCount)
	if err != nil {
		return err
	}
	return v.delegateView.DeleteObjects(ctx, gvk, criteria)
}

func (v *sandboxView) ListNodes(ctx context.Context, matchingNodeNames ...string) (nodes []corev1.Node, err error) {
	gvk := typeinfo.NodesDescriptor.GVK
	metaObjs, _, err := v.ListMetaObjects(ctx, gvk, minkapi.MatchCriteria{
		Names: sets.New(matchingNodeNames...),
	})
	if err != nil {
		return
	}
	nodes, _, err = asNodes(metaObjs)
	return
}

func (v *sandboxView) ListPods(ctx context.Context, c minkapi.MatchCriteria) (pods []corev1.Pod, err error) {
	gvk := typeinfo.PodsDescriptor.GVK
	metaObjs, _, err := v.ListMetaObjects(ctx, gvk, c)
	if err != nil {
		return
	}
	pods, _, err = asPods(metaObjs)
	return
}

func (v *sandboxView) ListEvents(ctx context.Context, namespace string) (events []eventsv1.Event, err error) {
	metaObjs, _, err := v.ListMetaObjects(ctx, typeinfo.EventsDescriptor.GVK, minkapi.MatchCriteria{
		Namespace: namespace,
	})
	if err != nil {
		return
	}
	events, _, err = asEvents(metaObjs)
	return
}

func (v *sandboxView) GetKubeConfigPath() string {
	return v.args.KubeConfigPath
}
