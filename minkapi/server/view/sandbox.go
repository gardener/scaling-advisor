// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gardener/scaling-advisor/minkapi/server/eventsink"
	"github.com/gardener/scaling-advisor/minkapi/server/store"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
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
	log          logr.Logger
	delegateView minkapi.View
	args         *minkapi.ViewArgs
	mu           *sync.RWMutex
	stores       map[schema.GroupVersionKind]*store.InMemResourceStore
	eventSink    minkapi.EventSink
	changeCount  atomic.Int64
}

// NewSandbox returns a "sandbox" (private) view which holds changes made via its facade into its private store independent of the base view,
// otherwise delegating to the delegate View.
func NewSandbox(log logr.Logger, delegateView minkapi.View, args *minkapi.ViewArgs) (minkapi.View, error) {
	stores := map[schema.GroupVersionKind]*store.InMemResourceStore{}
	for _, d := range typeinfo.SupportedDescriptors {
		baseStore, err := delegateView.GetResourceStore(d.GVK)
		if err != nil {
			return nil, err
		}
		stores[d.GVK] = createInMemStore(log, d, baseStore.GetVersionCounter(), args)
		//stores[d.GVK] = store.NewInMemResourceStore(d.GVK, d.ListGVK, d.GVR.GroupResource().Resource, args.WatchConfig.QueueSize, args.WatchConfig.Timeout, typeinfo.SupportedScheme, log)
	}
	eventSink := eventsink.New(log)
	return &sandboxView{
		log:          log,
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

func (v *sandboxView) GetClientFacades() (clientFacades commontypes.ClientFacades, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", minkapi.ErrClientFacadesFailed, err)
		}
	}()
	panic("inmem client type to be implemented")
	return
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

func (v *sandboxView) CreateObject(gvk schema.GroupVersionKind, obj metav1.Object) error {
	return storeObject(v, gvk, obj, &v.changeCount)
}

func (v *sandboxView) GetObject(gvk schema.GroupVersionKind, objName cache.ObjectName) (obj runtime.Object, err error) {
	obj, err = v.getSandboxObject(gvk, objName)
	if obj != nil || !apierrors.IsNotFound(err) {
		// return if I found the object or get an error other than not found error
		return
	}
	obj, err = v.delegateView.GetObject(gvk, objName)
	return
}

func (v *sandboxView) getSandboxObject(gvk schema.GroupVersionKind, objName cache.ObjectName) (obj runtime.Object, err error) {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return
	}
	obj, err = s.GetByKey(objName.String())
	return
}

func (v *sandboxView) UpdateObject(gvk schema.GroupVersionKind, obj metav1.Object) error {
	objName := objutil.CacheName(obj)
	sandboxObj, err := v.getSandboxObject(gvk, objName)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if sandboxObj != nil { //sandbox object is being updated.
		return updateObject(v, gvk, obj, &v.changeCount)
	}
	// The object is in base view and should not be modified - store in sandbox view now.
	return v.CreateObject(gvk, obj)
}

func (v *sandboxView) UpdatePodNodeBinding(podName cache.ObjectName, binding corev1.Binding) (pod *corev1.Pod, err error) {
	gvk := typeinfo.PodsDescriptor.GVK
	obj, err := v.getSandboxObject(gvk, podName) // get pod from sandbox first.
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
		return updatePodNodeBinding(v, pod, binding)
	}
	// pod is not found in sandbox. now get from base
	obj, err = v.delegateView.GetObject(gvk, podName)
	if err != nil {
		return
	}
	pod, ok = obj.(*corev1.Pod)
	if !ok {
		err = fmt.Errorf("%w: cannot update pod node binding in %q view since obj %T for name %q not a corev1.Pod", minkapi.ErrUpdateObject, v.GetName(), obj, podName)
	}
	// found in base so lets make a copy and store in sandbox
	sandboxPod := pod.DeepCopy()
	err = v.CreateObject(gvk, sandboxPod)
	if err != nil {
		return
	}
	pod = sandboxPod
	return updatePodNodeBinding(v, pod, binding)
}

func (v *sandboxView) PatchObject(gvk schema.GroupVersionKind, objName cache.ObjectName, patchType types.PatchType, patchData []byte) (patchedObj runtime.Object, err error) {
	return patchObject(v, gvk, objName, patchType, patchData)
}

func (v *sandboxView) PatchObjectStatus(gvk schema.GroupVersionKind, objName cache.ObjectName, patchData []byte) (patchedObj runtime.Object, err error) {
	return patchObjectStatus(v, gvk, objName, patchData)
}

func (v *sandboxView) ListMetaObjects(gvk schema.GroupVersionKind, criteria minkapi.MatchCriteria) (items []metav1.Object, maxVersion int64, err error) {
	sandboxItems, myMax, err := listMetaObjects(v, gvk, criteria)
	if err != nil {
		return
	}
	delegateItems, delegateMax, err := v.delegateView.ListMetaObjects(gvk, criteria)
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

func (v *sandboxView) ListObjects(gvk schema.GroupVersionKind, criteria minkapi.MatchCriteria) (listObj runtime.Object, err error) {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return
	}
	items, maxVersion, err := v.ListMetaObjects(gvk, criteria) // v.ListMetaObjcts already invokes delegate
	if err != nil {
		return
	}
	objGVK, objListKind := s.GetObjAndListGVK()
	return store.WrapMetaObjectsIntoRuntimeListObject(maxVersion, objGVK, objListKind, items)
}

func (v *sandboxView) WatchObjects(ctx context.Context, gvk schema.GroupVersionKind, startVersion int64, namespace string, labelSelector labels.Selector, eventCallback minkapi.WatchEventCallback) error {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return err
	}
	var eg errgroup.Group
	eg.Go(func() error {
		v.log.Info("watching sandboxView objects", "gvk", gvk, "startVersion", startVersion, "namespace", namespace, "labelSelector", labelSelector)
		return s.Watch(ctx, startVersion, namespace, labelSelector, eventCallback)
	})
	eg.Go(func() error {
		v.log.Info("watching delegateView objects", "gvk", gvk, "startVersion", startVersion, "namespace", namespace, "labelSelector", labelSelector)
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

func (v *sandboxView) DeleteObject(gvk schema.GroupVersionKind, objName cache.ObjectName) error {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return err
	}
	obj, err := s.GetByKey(objName.String())
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if obj == nil {
		// delegate to delegateView if obj not found in sandbox
		return v.delegateView.DeleteObject(gvk, objName)
	}
	// if found in this views store, delete and return
	err = s.Delete(objName)
	if err != nil {
		return err
	}
	v.changeCount.Add(1)
	return nil

}

func (v *sandboxView) DeleteObjects(gvk schema.GroupVersionKind, criteria minkapi.MatchCriteria) error {
	err := deleteObjects(v, gvk, criteria, &v.changeCount)
	if err != nil {
		return err
	}
	return v.delegateView.DeleteObjects(gvk, criteria)
}

func (v *sandboxView) ListNodes(matchingNodeNames ...string) (nodes []corev1.Node, err error) {
	gvk := typeinfo.NodesDescriptor.GVK
	metaObjs, _, err := v.ListMetaObjects(gvk, minkapi.MatchCriteria{
		Names: sets.New(matchingNodeNames...),
	})
	if err != nil {
		return
	}
	nodes, _, err = asNodes(metaObjs)
	return
}

func (v *sandboxView) ListPods(c minkapi.MatchCriteria) (pods []corev1.Pod, err error) {
	gvk := typeinfo.PodsDescriptor.GVK
	metaObjs, _, err := v.ListMetaObjects(gvk, c)
	if err != nil {
		return
	}
	pods, _, err = asPods(metaObjs)
	return
}

func (v *sandboxView) ListEvents(namespace string) (events []eventsv1.Event, err error) {
	metaObjs, _, err := v.ListMetaObjects(typeinfo.EventsDescriptor.GVK, minkapi.MatchCriteria{
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
