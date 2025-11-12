// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/go-logr/logr"
	"golang.org/x/net/context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

var _ mkapi.ResourceStore = (*InMemResourceStore)(nil)

// InMemResourceStore represents an in-memory implementation of the ResourceStore interface for managing resources.
// It leverages and wraps a backing cache.Store.
type InMemResourceStore struct {
	args        *mkapi.ResourceStoreArgs
	cache       cache.Store
	broadcaster *watch.Broadcaster
	// versionCounter is the atomic counter for generating monotonically increasing resource versions
	versionCounter *atomic.Int64
}

// GetVersionCounter returns the atomic resource version counter for resources in this store.
func (s *InMemResourceStore) GetVersionCounter() *atomic.Int64 {
	return s.versionCounter
}

// GetObjAndListGVK returns the object and object list group, version and kind.
func (s *InMemResourceStore) GetObjAndListGVK() (objKind schema.GroupVersionKind, objListKind schema.GroupVersionKind) {
	return s.args.ObjectGVK, s.args.ObjectListGVK
}

// NewInMemResourceStore returns an in-memory store for a given object GVK. TODO: think on simplifying parameters.
func NewInMemResourceStore(args *mkapi.ResourceStoreArgs) *InMemResourceStore {
	s := InMemResourceStore{
		args:           args,
		cache:          cache.NewStore(cache.MetaNamespaceKeyFunc),
		broadcaster:    watch.NewBroadcaster(args.WatchConfig.QueueSize, watch.WaitIfChannelFull),
		versionCounter: args.VersionCounter,
	}
	if s.versionCounter == nil {
		s.versionCounter = &atomic.Int64{}
	}
	return &s
}

// Reset resets the backing cache for this story and re-initializes the watch broadcasters.
func (s *InMemResourceStore) Reset() {
	s.cache = cache.NewStore(cache.MetaNamespaceKeyFunc)
	s.broadcaster = watch.NewBroadcaster(s.args.WatchConfig.QueueSize, watch.WaitIfChannelFull)
}

// Add adds the given metav1 Object to this store, setting the right resource version, updating the resource version counter and broadcasting the Add event to any watchers.
// TODO think on how to handle context cancellation
func (s *InMemResourceStore) Add(ctx context.Context, mo metav1.Object) error {
	log := logr.FromContextOrDiscard(ctx)
	o, err := s.validateRuntimeObj(mo)
	if err != nil {
		return err
	}
	key := objutil.CacheName(mo)
	mo.SetResourceVersion(s.nextResourceVersionAsString())
	err = s.cache.Add(o)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("cannot add object %q to store: %w", key, err))
	}
	log.V(4).Info("added object to store", "kind", s.args.ObjectGVK.Kind, "key", key, "resourceVersion", mo.GetResourceVersion())

	go func() {
		err = s.broadcaster.Action(watch.Added, o)
		if err != nil {
			log.Error(err, "failed to broadcast object add", "key", key)
		}
	}()
	return nil
}

// Update updates the given metav1.Object in the store, setting the next resource version and broadcasting a Modified event.
// TODO think on how to handle context cancellation
func (s *InMemResourceStore) Update(ctx context.Context, mo metav1.Object) error {
	log := logr.FromContextOrDiscard(ctx)
	o, err := s.validateRuntimeObj(mo)
	if err != nil {
		return err
	}
	key := objutil.CacheName(mo)
	mo.SetResourceVersion(s.nextResourceVersionAsString())
	err = s.cache.Update(o)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("cannot update object %q in store: %w", key, err))
	}
	log.V(4).Info("updated object in store", "kind", s.args.ObjectGVK.Kind, "key", key, "resourceVersion", mo.GetResourceVersion())
	go func() {
		err = s.broadcaster.Action(watch.Modified, o)
		if err != nil {
			log.Error(err, "failed to broadcast object update", "key", key)
		}
	}()
	return nil
}

// DeleteByKey deletes the object identified by key in the store, sets the deletion timestamp on the object and broadcasts the watch Deleted event.
// TODO think on how to handle context cancellation
func (s *InMemResourceStore) DeleteByKey(ctx context.Context, key string) error {
	log := logr.FromContextOrDiscard(ctx)
	o, err := s.GetByKey(ctx, key)
	if err != nil {
		return err
	}
	mo, err := objutil.AsMeta(o)
	if err != nil {
		return err
	}
	err = s.cache.Delete(mo)
	if err != nil {
		err = fmt.Errorf("cannot delete object with key %q from store: %w", key, err)
		return apierrors.NewInternalError(err)
	}
	mo.SetDeletionTimestamp(&metav1.Time{Time: time.Time{}})
	log.V(4).Info("deleted object", "kind", s.args.ObjectGVK.Kind, "key", key)
	go func() {
		err = s.broadcaster.Action(watch.Deleted, o)
		if err != nil {
			log.Error(err, "failed to broadcast object delete", "key", key, "resourceVersion", mo.GetResourceVersion())
		}
	}()
	return nil
}

// Delete deletes the object identified by its fully qualified cache name from the store. It delegates to DeleteByKey
func (s *InMemResourceStore) Delete(ctx context.Context, objName cache.ObjectName) error {
	return s.DeleteByKey(ctx, objName.String())
}

// GetByKey gets the object identified by the given key from the store and returns the same as a runtime.Object.
func (s *InMemResourceStore) GetByKey(ctx context.Context, key string) (o runtime.Object, err error) {
	log := logr.FromContextOrDiscard(ctx)
	obj, exists, err := s.cache.GetByKey(key)
	if err != nil {
		log.Error(err, "failed to find object with key", "key", key)
		err = apierrors.NewInternalError(fmt.Errorf("cannot find object with key %q: %w", key, err))
		return
	}
	if !exists {
		log.V(4).Info("did not find object by key", "key", key)
		err = apierrors.NewNotFound(schema.GroupResource{Group: s.args.ObjectGVK.Group, Resource: s.args.Name}, key)
		return
	}
	o, ok := obj.(runtime.Object)
	if !ok {
		err = fmt.Errorf("cannot convert object with key %q to runtime.Object", key)
		log.Error(err, "conversion error", "key", key)
		err = apierrors.NewInternalError(err)
		return
	}
	return
}

// Get gets the object identified by the given fully qualified cache name from the store. It delegates to GetByKey.
func (s *InMemResourceStore) Get(ctx context.Context, objName cache.ObjectName) (o runtime.Object, err error) {
	return s.GetByKey(ctx, objName.String())
}

// List queries the store according to the given MatchCriteria, gets objects and creates and returns the List object wrapping individual objects.
func (s *InMemResourceStore) List(ctx context.Context, c mkapi.MatchCriteria) (listObj runtime.Object, err error) {
	log := logr.FromContextOrDiscard(ctx)
	items := s.cache.List()
	currVersionStr := fmt.Sprintf("%d", s.CurrentResourceVersion())
	typesMap := typeinfo.SupportedScheme.KnownTypes(s.args.ObjectGVK.GroupVersion())
	listType, ok := typesMap[s.args.ObjectListGVK.Kind] // Ex: Get Go reflect.type for the PodList
	if !ok {
		return nil, runtime.NewNotRegisteredErrForKind(typeinfo.SupportedScheme.Name(), s.args.ObjectListGVK)
	}
	listObjPtr := reflect.New(listType) // Ex: reflect.Value wrapper of *PodList
	listObjVal := listObjPtr.Elem()     // Ex: reflect.Elem wrapper of PodList
	typeMetaVal := listObjVal.FieldByName("TypeMeta")
	if !typeMetaVal.IsValid() {
		return nil, fmt.Errorf("failed to get TypeMeta field on %v", listObjVal)
	}
	listMetaVal := listObjVal.FieldByName("ListMeta")
	if !listMetaVal.IsValid() {
		return nil, fmt.Errorf("failed to get ListMeta field on %v", listObjVal)
	}
	typeMetaVal.Set(reflect.ValueOf(metav1.TypeMeta{
		Kind:       s.args.ObjectListGVK.Kind,
		APIVersion: s.args.ObjectGVK.GroupVersion().String(),
	}))
	listMetaVal.Set(reflect.ValueOf(metav1.ListMeta{
		ResourceVersion: currVersionStr,
	}))
	itemsField := listObjVal.FieldByName("Items") // // Ex: corev1.Pod
	if !itemsField.IsValid() || !itemsField.CanSet() || itemsField.Kind() != reflect.Slice {
		return nil, fmt.Errorf("list object type %T for kind %q does not have a settable slice field named Items", listObj, s.args.ObjectGVK.Kind)
	}
	itemType := itemsField.Type().Elem() // e.g., corev1.Pod
	resultSlice := reflect.MakeSlice(itemsField.Type(), 0, len(items))

	objs, err := objutil.SliceOfAnyToRuntimeObj(items)
	if err != nil {
		return
	}
	for _, obj := range objs {
		metaV1Obj, err := objutil.AsMeta(obj)
		if err != nil {
			log.Error(err, "cannot access meta object", "obj", obj)
			continue
		}
		if !c.Matches(metaV1Obj) {
			continue
		}
		val := reflect.ValueOf(obj)
		if val.Kind() != reflect.Ptr || val.IsNil() {
			// ensure each cached obj is a non-nil pointer (Ex *corev1.Pod).
			return nil, fmt.Errorf("element for kind %q is not a non-nil pointer: %T", s.args.ObjectGVK, obj)
		}
		if val.Elem().Type() != itemType {
			// ensure each cached obj dereferences to the expected type (Ex corev1.Pod).
			return nil, fmt.Errorf("type mismatch, list kind %q expects items of type %v, but got %v", s.args.ObjectListGVK, itemType, val.Elem().Type())
		}
		resultSlice = reflect.Append(resultSlice, val.Elem()) // Append the struct (not the pointer) into the .Items slice of the list.
	}
	itemsField.Set(resultSlice)
	listObj = listObjPtr.Interface().(runtime.Object) // Ex: listObjPtr.Interface() gets the actual *core1.PodList which is then type-asserted to runtime.Object
	return listObj, nil
}

// ListMetaObjects queries the store according to the given MatchCriteria, gets objects and returns them as a slice, including the maximum resource version found in the returned objects.
func (s *InMemResourceStore) ListMetaObjects(ctx context.Context, c mkapi.MatchCriteria) (metaObjs []metav1.Object, maxVersion int64, err error) {
	items := s.cache.List()
	sliceSize := int(math.Min(float64(len(items)), float64(100)))
	metaObjs = make([]metav1.Object, 0, sliceSize)
	var mo metav1.Object
	var version int64
	for _, item := range items {
		if err = ctx.Err(); err != nil {
			return
		}
		mo, err = objutil.AsMeta(item)
		if err != nil {
			err = fmt.Errorf("%w: %w", mkapi.ErrListObjects, err)
			return
		}
		if !c.Matches(mo) {
			continue
		}
		version, err = objutil.ParseObjectResourceVersion(mo)
		if err != nil {
			return
		}
		metaObjs = append(metaObjs, mo)
		if version > maxVersion {
			maxVersion = version
		}
	}
	return
}

// DeleteObjects deletes objects from the store that match the provided MatchCriteria.
// It returns the number of deleted objects and an error if one occurs during deletion.
func (s *InMemResourceStore) DeleteObjects(ctx context.Context, c mkapi.MatchCriteria) (delCount int, err error) {
	items := s.cache.List()
	var mo metav1.Object
	for _, item := range items {
		if err = ctx.Err(); err != nil {
			return
		}
		mo, err = objutil.AsMeta(item)
		if err != nil {
			err = fmt.Errorf("%w: %w", mkapi.ErrDeleteObject, err)
			return
		}
		if !c.Matches(mo) {
			continue
		}
		objName := objutil.CacheName(mo)
		_, err = s.Get(ctx, objName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			err = fmt.Errorf("%w: %w", mkapi.ErrDeleteObject, err)
			return
		}
		err = s.Delete(ctx, objName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			err = fmt.Errorf("%w: %w", mkapi.ErrDeleteObject, err)
			return
		}
		delCount++
	}
	return
}

func (s *InMemResourceStore) validateRuntimeObj(mo metav1.Object) (o runtime.Object, err error) {
	key := objutil.CacheName(mo)
	o, ok := mo.(runtime.Object)
	if !ok {
		err = fmt.Errorf("cannot convert meta object %q of type %T to runtime.Object", key, mo)
		return
	}
	oGVK := o.GetObjectKind().GroupVersionKind()
	if oGVK != s.args.ObjectGVK {
		err = fmt.Errorf("object objGVK %q does not match expected objGVK %q", oGVK, s.args.ObjectGVK)
	}
	return
}

func (s *InMemResourceStore) buildPendingWatchEvents(startVersion int64, namespace string, labelSelector labels.Selector) (watchEvents []watch.Event, err error) {
	var skip bool
	allItems := s.cache.List()
	objs, err := objutil.SliceOfAnyToRuntimeObj(allItems)
	if err != nil {
		return
	}
	for _, o := range objs {
		skip, err = shouldSkipObject(o, startVersion, namespace, labelSelector)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		watchEvent := watch.Event{Type: watch.Added, Object: o}
		watchEvents = append(watchEvents, watchEvent)
	}
	return
}

// EventCallbackFn  is a typedef for a function that accepts and processes a watch.Event, returning an error if processing failed.
type EventCallbackFn func(watch.Event) (err error)

// Watch is a blocking function that watches the store for object changes beginning from startVersion, belonging tot he given namespace, matching the given labelSelector and invoking the given eventCallback.
// Watch will return after the configured watch timeout or if the given context is cancelled.
func (s *InMemResourceStore) Watch(ctx context.Context, startVersion int64, namespace string, labelSelector labels.Selector, eventCallback mkapi.WatchEventCallback) error {
	log := logr.FromContextOrDiscard(ctx)
	events, err := s.buildPendingWatchEvents(startVersion, namespace, labelSelector)
	if err != nil {
		return err
	}
	watcher, err := s.broadcaster.WatchWithPrefix(events)
	if err != nil {
		return fmt.Errorf("cannot start watch for gvk %q: %w", s.args.ObjectGVK, err)
	}
	defer watcher.Stop()
	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				log.V(4).Info("no more events on watch result channel for gvk.", "gvk", s.args.ObjectGVK)
				return nil
			}
			skip, err := shouldSkipObject(event.Object, startVersion, namespace, labelSelector)
			if err != nil {
				return err
			}
			if skip {
				continue
			}
			err = eventCallback(event)
			if err != nil {
				return err
			}
		case <-time.After(s.args.WatchConfig.Timeout):
			log.V(4).Info("watcher timed out", "gvk", s.args.ObjectGVK, "watchTimeout", s.args.WatchConfig.Timeout, "startVersion", startVersion, "namespace", namespace, "labelSelector", labelSelector.String())
			return nil
		case <-ctx.Done():
			log.V(4).Info("watch context cancelled", "gvk", s.args.ObjectGVK, "startVersion", startVersion, "namespace", namespace, "labelSelector", labelSelector.String())
			return nil
		}
	}
}

// GetWatcher creates a watcher for resource events in the specified namespace matching the given list options.
// It returns a watch.Interface to observe events and an error if the watcher could not be created.
func (s *InMemResourceStore) GetWatcher(ctx context.Context, namespace string, options metav1.ListOptions) (eventWatcher watch.Interface, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w :%w", mkapi.ErrCreateWatcher, err)
		}
	}()
	log := logr.FromContextOrDiscard(ctx)
	out := make(chan watch.Event)
	startVersion, err := objutil.ParseResourceVersion(options.ResourceVersion)
	if err != nil {
		return
	}
	labelSelector := labels.Everything()
	if options.LabelSelector != "" {
		labelSelector, err = labels.Parse(options.LabelSelector)
		if err != nil {
			return
		}
	}
	proxyWatcher := watch.NewProxyWatcher(out)
	// Start the callback-based watcher in its own goroutine
	go func() {
		defer close(out)
		err := s.Watch(ctx, startVersion, namespace, labelSelector, func(e watch.Event) error {
			select {
			case out <- e:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-proxyWatcher.StopChan():
				return context.Canceled
			}
		})
		if err != nil {
			// can do nothing but log this as watch.Interface has no error channel
			log.Error(err, "error in InMemResourceStore.Watch", "gvk", s.args.ObjectGVK, "startVersion", startVersion, "namespace", namespace, "labelSelector", labelSelector.String())
		}
	}()
	eventWatcher = proxyWatcher
	return
}

// CurrentResourceVersion returns the current version of the resource as an int64 from the store's version counter.
func (s *InMemResourceStore) CurrentResourceVersion() int64 {
	return s.versionCounter.Load()
}

// Close clears the resources associated with the store, including gracefully shutting down the event broadcaster if it is initialized.
func (s *InMemResourceStore) Close() error {
	if s.broadcaster != nil {
		s.broadcaster.Shutdown()
	}
	return nil
}

func (s *InMemResourceStore) nextResourceVersionAsString() string {
	return strconv.FormatInt(s.nextResourceVersion(), 10)
}

// nextResourceVersion increments and returns the next version for this store's GVK
func (s *InMemResourceStore) nextResourceVersion() int64 {
	return s.versionCounter.Add(1)
}

// WrapMetaObjectsIntoRuntimeListObject wraps a list of metav1.Object into a runtime.Object of the corresponding list type.
// It sets the TypeMeta and ListMeta fields, ensuring compatibility with the provided GroupVersionKind and its list counterpart.
// Returns the constructed runtime.Object or an error if the operation fails.
func WrapMetaObjectsIntoRuntimeListObject(resourceVersion int64, objectGVK schema.GroupVersionKind, objectListGVK schema.GroupVersionKind, items []metav1.Object) (listObj runtime.Object, err error) {
	resourceVersionStr := strconv.FormatInt(resourceVersion, 10)
	typesMap := typeinfo.SupportedScheme.KnownTypes(objectGVK.GroupVersion())
	listType, ok := typesMap[objectListGVK.Kind] // Ex: Get Go reflect.type for the PodList
	if !ok {
		return nil, runtime.NewNotRegisteredErrForKind(typeinfo.SupportedScheme.Name(), objectListGVK)
	}
	listObjPtr := reflect.New(listType) // Ex: reflect.Value wrapper of *PodList
	listObjVal := listObjPtr.Elem()     // Ex: reflect.Elem wrapper of PodList
	typeMetaVal := listObjVal.FieldByName("TypeMeta")
	if !typeMetaVal.IsValid() {
		return nil, fmt.Errorf("failed to get TypeMeta field on %v", listObjVal)
	}
	listMetaVal := listObjVal.FieldByName("ListMeta")
	if !listMetaVal.IsValid() {
		return nil, fmt.Errorf("failed to get ListMeta field on %v", listObjVal)
	}
	typeMetaVal.Set(reflect.ValueOf(metav1.TypeMeta{
		Kind:       objectListGVK.Kind,
		APIVersion: objectGVK.GroupVersion().String(),
	}))
	listMetaVal.Set(reflect.ValueOf(metav1.ListMeta{
		ResourceVersion: resourceVersionStr,
	}))
	itemsField := listObjVal.FieldByName("Items") // // Ex: corev1.Pod
	if !itemsField.IsValid() || !itemsField.CanSet() || itemsField.Kind() != reflect.Slice {
		return nil, fmt.Errorf("list object type %T for kind %q does not have a settable slice field named Items", listObj, objectGVK.Kind)
	}

	itemType := itemsField.Type().Elem() // e.g., corev1.Pod
	resultSlice := reflect.MakeSlice(itemsField.Type(), 0, len(items))

	objs, err := objutil.SliceOfMetaObjToRuntimeObj(items)
	if err != nil {
		return
	}
	for _, obj := range objs {
		val := reflect.ValueOf(obj)
		if val.Kind() != reflect.Ptr || val.IsNil() {
			// ensure each cached obj is a non-nil pointer (Ex *corev1.Pod).
			return nil, fmt.Errorf("element for kind %q is not a non-nil pointer: %T", objectGVK, obj)
		}
		if val.Elem().Type() != itemType {
			// ensure each cached obj dereferences to the expected type (Ex corev1.Pod).
			return nil, fmt.Errorf("type mismatch, list kind %q expects items of type %v, but got %v", objectListGVK, itemType, val.Elem().Type())
		}
		resultSlice = reflect.Append(resultSlice, val.Elem()) // Append the struct (not the pointer) into the .Items slice of the list.
	}
	itemsField.Set(resultSlice)
	listObj = listObjPtr.Interface().(runtime.Object) // Ex: listObjPtr.Interface() gets the actual *core1.PodList which is then type-asserted to runtime.Object
	return listObj, nil
}

func shouldSkipObject(obj runtime.Object, startVersion int64, namespace string, labelSelector labels.Selector) (skip bool, err error) {
	o, err := meta.Accessor(obj)
	if err != nil {
		err = fmt.Errorf("cannot access object metadata for obj type %T: %w", obj, err)
		return
	}
	if namespace != "" && o.GetNamespace() != namespace {
		skip = true
		return
	}
	if !labelSelector.Matches(labels.Set(o.GetLabels())) {
		skip = true
		return
	}
	rv, err := objutil.ParseObjectResourceVersion(o)
	if err != nil {
		return
	}
	if rv <= startVersion {
		skip = true
		return
	}
	return
}
