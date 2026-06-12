// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gardener/scaling-advisor/minkapi/view/eventsink"
	"github.com/gardener/scaling-advisor/minkapi/view/inmclient"
	"github.com/gardener/scaling-advisor/minkapi/view/store"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	"github.com/gardener/scaling-advisor/common/clientutil"
	"github.com/gardener/scaling-advisor/common/ioutil"
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

func (v *sandboxView) Reset() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	resettable := asResettable(v.stores)
	resettable = append(resettable, v.eventSink)
	v.changeCount.Store(0)
	return ioutil.ResetAll(resettable...)
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
	return minkapi.ViewTypeSandbox
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
	case commontypes.ClientAccessModeNetwork:
		clientFacades, err = clientutil.CreateNetworkClientFacades(log, v.GetKubeConfigPath(), v.args.WatchConfig.Timeout)
	case commontypes.ClientAccessModeInMemory:
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

func (v *sandboxView) CreateObject(ctx context.Context, gvk schema.GroupVersionKind, obj metav1.Object, opts minkapi.ObjectOptions) (metav1.Object, error) {
	// A successful Create resurrects a tombstoned name; the underlying store's Add replaces
	// any DeletedFinalStateUnknown wrapper at the same key with the new object.
	return storeObject(ctx, v, gvk, obj, opts, &v.changeCount)
}

func (v *sandboxView) GetObject(ctx context.Context, gvk schema.GroupVersionKind, objName cache.ObjectName) (obj runtime.Object, err error) {
	obj, err = v.getSandboxObject(ctx, gvk, objName)
	if err != nil && errors.Is(err, minkapi.ErrObjectDeleted) {
		// Tombstoned: surface as the standard Kubernetes "not found" error and do not fall
		// through to the delegate (the sandbox has explicitly deleted this name).
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, objName.String())
	}
	if obj != nil || !apierrors.IsNotFound(err) {
		// return if I found the object or get an error other than not found error
		return
	}
	obj, err = v.delegateView.GetObject(ctx, gvk, objName)
	return
}

func (v *sandboxView) UpdateObject(ctx context.Context, gvk schema.GroupVersionKind, obj metav1.Object, opts minkapi.ObjectOptions) error {
	objName := objutil.CacheName(obj)
	sandboxObj, err := v.getSandboxObject(ctx, gvk, objName)
	if err != nil && errors.Is(err, minkapi.ErrObjectDeleted) {
		// Tombstoned: caller is updating something that has been deleted from the sandbox.
		return apierrors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, objName.String())
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if sandboxObj != nil { //sandbox object is being updated.
		return updateObject(ctx, v, gvk, obj, opts, &v.changeCount)
	}
	// The object lives only in the delegate. Materialize a deep-copy of the current delegate
	// state into the sandbox without broadcasting (consumers already saw the original via the
	// delegate's initial-list, so this materialization is plumbing). Then run the normal
	// update path against the sandbox copy so the broadcast carries the user-visible change.
	// WatchObjects' filter suppresses the delegate watcher's events for this object name from
	// here on, so the sandbox broadcast is the single observable event.
	delegateObj, err := v.delegateView.GetObject(ctx, gvk, objName)
	if err != nil {
		return err
	}
	materialized := delegateObj.DeepCopyObject()
	asMeta, ok := materialized.(metav1.Object)
	if !ok {
		return fmt.Errorf("%w: %T does not implement metav1.Object", minkapi.ErrUpdateObject, materialized)
	}
	if _, err = v.CreateObject(ctx, gvk, asMeta, minkapi.ObjectOptions{NoBroadcast: true}); err != nil {
		return err
	}
	return updateObject(ctx, v, gvk, obj, opts, &v.changeCount)
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
	_, err = v.CreateObject(ctx, gvk, sandboxPod, minkapi.ObjectOptions{NoBroadcast: true})
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
	// Filter out delegate items whose name has been tombstoned in this sandbox's store.
	s, serr := v.GetResourceStore(gvk)
	if serr == nil && len(delegateItems) > 0 {
		filtered := delegateItems[:0]
		for _, it := range delegateItems {
			if s.IsMarkedForDelete(objutil.CacheName(it).String()) {
				continue
			}
			filtered = append(filtered, it)
		}
		delegateItems = filtered
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
		return v.delegateView.WatchObjects(ctx, gvk, startVersion, namespace, labelSelector, func(event watch.Event) error {
			evObj, err := objutil.AsMeta(event.Object)
			if err != nil {
				return err
			}
			objFullName := objutil.CacheName(evObj)
			sandboxObj, gerr := v.getSandboxObject(ctx, gvk, objFullName)
			if sandboxObj != nil {
				// already covered by sandbox watcher's own event for this object
				return nil
			}
			if gerr != nil && errors.Is(gerr, minkapi.ErrObjectDeleted) {
				// tombstoned in this sandbox: hide delegate's events for this name
				return nil
			}
			return eventCallback(event)
		})
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

// DeleteObject removes an object from the sandbox's perspective. It NEVER mutates the delegate
// view; instead, it parks a [cache.DeletedFinalStateUnknown] tombstone in the sandbox's own
// store via store.MarkForDelete so subsequent Get/List/Watch operations on this sandbox
// behave as if the object is gone.
//
// Three cases:
//   - The object is in the sandbox local store: store.MarkForDelete tombstones it directly and
//     fires a Deleted broadcast.
//   - The object is in the delegate only: materialize a deep-copy into the sandbox store with
//     NoBroadcast=true (consumers already saw the original via the delegate's initial-list),
//     then store.MarkForDelete tombstones the materialized copy and fires the Deleted
//     broadcast.
//   - The object exists in neither (or is already tombstoned): MarkForDelete returns
//     apierrors.NewNotFound, matching the Kubernetes API server's DELETE-on-missing behavior.
func (v *sandboxView) DeleteObject(ctx context.Context, gvk schema.GroupVersionKind, objName cache.ObjectName) error {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return err
	}
	// Already tombstoned or sandbox-local? MarkForDelete handles both cleanly.
	if _, gerr := s.GetByKey(ctx, objName.String()); gerr == nil {
		// Sandbox-local: tombstone directly.
		if err = s.MarkForDelete(ctx, objName.String()); err != nil {
			return err
		}
		v.changeCount.Add(1)
		return nil
	} else if errors.Is(gerr, minkapi.ErrObjectDeleted) {
		// Already tombstoned: NotFound, mirroring K8s API server behavior for repeated delete.
		return apierrors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, objName.String())
	}
	// Sandbox doesn't have it; consult delegate.
	delegateObj, err := v.delegateView.GetObject(ctx, gvk, objName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return err
		}
		return fmt.Errorf("%w: cannot resolve delegate object %q for sandbox delete: %w", minkapi.ErrDeleteObject, objName, err)
	}
	// Materialize a deep-copy of the delegate's object into the sandbox store (no broadcast,
	// since consumers already saw the original via the delegate's initial-list), then tombstone
	// it. MarkForDelete fires the Deleted broadcast on the sandbox watcher.
	materialized := delegateObj.DeepCopyObject()
	asMeta, ok := materialized.(metav1.Object)
	if !ok {
		return fmt.Errorf("%w: %T does not implement metav1.Object", minkapi.ErrDeleteObject, materialized)
	}
	if _, err = v.CreateObject(ctx, gvk, asMeta, minkapi.ObjectOptions{NoBroadcast: true}); err != nil {
		return err
	}
	if err = s.MarkForDelete(ctx, objName.String()); err != nil {
		return err
	}
	v.changeCount.Add(1)
	return nil
}

// DeleteObjects deletes from the sandbox's perspective every object that matches criteria. It
// never mutates the delegate; sandbox-local matches are deleted normally, and delegate-only
// matches are tombstoned (with materialize-then-local-delete to fire the Deleted broadcast).
func (v *sandboxView) DeleteObjects(ctx context.Context, gvk schema.GroupVersionKind, criteria minkapi.MatchCriteria) error {
	// Combined list reflecting tombstones already filters out previously-tombstoned names.
	items, _, err := v.ListMetaObjects(ctx, gvk, criteria)
	if err != nil {
		return fmt.Errorf("%w: cannot list objects for bulk delete: %w", minkapi.ErrDeleteObject, err)
	}
	for _, it := range items {
		objName := objutil.CacheName(it)
		if err := v.DeleteObject(ctx, gvk, objName); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
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

// getSandboxObject retrieves an object from the sandbox's local store. The store itself is
// tombstone-aware: a tombstoned key surfaces as [minkapi.ErrObjectDeleted], an absent key
// surfaces as apierrors.NotFound, and a live key returns the typed runtime.Object. Public
// methods translate ErrObjectDeleted to NotFound for the API surface.
func (v *sandboxView) getSandboxObject(ctx context.Context, gvk schema.GroupVersionKind, objName cache.ObjectName) (obj runtime.Object, err error) {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return
	}
	obj, err = s.GetByKey(ctx, objName.String())
	return
}
