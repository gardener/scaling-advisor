// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"fmt"
	"reflect"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/core/typeinfo"

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

var _ api.ResourceStore = (*InMemResourceStore)(nil)

type InMemResourceStore struct {
	objGVK       schema.GroupVersionKind
	objListGVK   schema.GroupVersionKind
	resourceName string
	delegate     cache.Store
	broadcaster  *watch.Broadcaster
	watchTimeout time.Duration
	rvCounter    atomic.Int64
	scheme       *runtime.Scheme
	log          logr.Logger
}

// NewInMemResourceStore returns an in-memory store for a given object GVK. TODO: think on simplifying parameters.
func NewInMemResourceStore(
	objGVK schema.GroupVersionKind,
	objListGVK schema.GroupVersionKind,
	resourceName string,
	watchQueueSize int,
	watchTimeout time.Duration,
	scheme *runtime.Scheme,
	log logr.Logger) *InMemResourceStore {
	s := InMemResourceStore{
		objGVK:       objGVK,
		objListGVK:   objListGVK,
		resourceName: resourceName,
		delegate:     cache.NewStore(cache.MetaNamespaceKeyFunc),
		//broadcaster: watch.NewBroadcaster(watchQueueSize, watch.DropIfChannelFull),
		broadcaster:  watch.NewBroadcaster(watchQueueSize, watch.WaitIfChannelFull),
		watchTimeout: watchTimeout,
		scheme:       scheme,
		log:          log,
	}
	s.log.Info("created in memory resource store", "GVK", objGVK, "resourceName", resourceName, "watchTimeout", watchTimeout)
	s.rvCounter.Store(1)
	return &s
}

func (s *InMemResourceStore) Add(mo metav1.Object) error {
	o, err := s.validateRuntimeObj(mo)
	if err != nil {
		return err
	}
	key := cache.NewObjectName(mo.GetNamespace(), mo.GetName())
	mo.SetResourceVersion(s.NextResourceVersionAsString())
	err = s.delegate.Add(o)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("cannot add object %q to store: %w", key, err))
	}
	s.log.V(4).Info("added object to store", "kind", s.objGVK.Kind, "key", key, "resourceVersion", mo.GetResourceVersion())

	go func() {
		err = s.broadcaster.Action(watch.Added, o)
		if err != nil {
			s.log.Error(err, "failed to broadcast object add", "key", key)
		}
	}()
	return nil
}

func (s *InMemResourceStore) Update(mo metav1.Object) error {
	o, err := s.validateRuntimeObj(mo)
	if err != nil {
		return err
	}
	key := cache.NewObjectName(mo.GetNamespace(), mo.GetName())
	mo.SetResourceVersion(s.NextResourceVersionAsString())
	err = s.delegate.Update(o)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("cannot update object %q in store: %w", key, err))
	}
	s.log.V(4).Info("updated object in store", "kind", s.objGVK.Kind, "key", key, "resourceVersion", mo.GetResourceVersion())
	go func() {
		err = s.broadcaster.Action(watch.Modified, o)
		if err != nil {
			s.log.Error(err, "failed to broadcast object update", "key", key)
		}
	}()
	return nil
}

func (s *InMemResourceStore) Delete(key string) error {
	o, err := s.GetByKey(key)
	if err != nil {
		return err
	}
	mo, err := AsMeta(s.log, o)
	if err != nil {
		return err
	}
	err = s.delegate.Delete(mo)
	if err != nil {
		err = fmt.Errorf("cannot delete object with key %q from store: %w", key, err)
		return apierrors.NewInternalError(err)
	}
	mo.SetDeletionTimestamp(&metav1.Time{Time: time.Time{}})
	s.log.V(4).Info("deleted object", "kind", s.objGVK.Kind, "key", key)
	go func() {
		err = s.broadcaster.Action(watch.Deleted, o)
		if err != nil {
			s.log.Error(err, "failed to broadcast object delete", "key", key, "resourceVersion", mo.GetResourceVersion())
		}
	}()
	return nil
}

func (s *InMemResourceStore) GetByKey(key string) (o runtime.Object, err error) {
	obj, exists, err := s.delegate.GetByKey(key)
	if err != nil {
		s.log.Error(err, "failed to find object with key", "key", key)
		err = apierrors.NewInternalError(fmt.Errorf("cannot find object with key %q: %w", key, err))
		return
	}
	if !exists {
		s.log.V(4).Info("did not find object by key", "key", key)
		err = apierrors.NewNotFound(schema.GroupResource{Group: s.objGVK.Group, Resource: s.resourceName}, key)
		return
	}
	o, ok := obj.(runtime.Object)
	if !ok {
		err = fmt.Errorf("cannot convert object with key %q to runtime.Object", key)
		s.log.Error(err, "conversion error", "key", key)
		err = apierrors.NewInternalError(err)
		return
	}
	return
}

func (s *InMemResourceStore) List(namespace string, labelSelector labels.Selector) (listObj runtime.Object, err error) {
	items := s.delegate.List()
	currVersionStr := fmt.Sprintf("%d", s.CurrentResourceVersion())
	typesMap := typeinfo.SupportedScheme.KnownTypes(s.objGVK.GroupVersion())
	listType, ok := typesMap[s.objListGVK.Kind]
	if !ok {
		return nil, runtime.NewNotRegisteredErrForKind(typeinfo.SupportedScheme.Name(), s.objListGVK)
	}
	ptr := reflect.New(listType) // *PodList
	listObjVal := ptr.Elem()     // PodList
	typeMetaVal := listObjVal.FieldByName("TypeMeta")
	if !typeMetaVal.IsValid() {
		return nil, fmt.Errorf("failed to get TypeMeta field on %v", listObjVal)
	}
	listMetaVal := listObjVal.FieldByName("ListMeta")
	if !listMetaVal.IsValid() {
		return nil, fmt.Errorf("failed to get ListMeta field on %v", listObjVal)
	}
	typeMetaVal.Set(reflect.ValueOf(metav1.TypeMeta{
		Kind:       s.objListGVK.Kind,
		APIVersion: s.objGVK.GroupVersion().String(),
	}))
	listMetaVal.Set(reflect.ValueOf(metav1.ListMeta{
		ResourceVersion: currVersionStr,
	}))
	itemsField := listObjVal.FieldByName("Items")
	if !itemsField.IsValid() || !itemsField.CanSet() || itemsField.Kind() != reflect.Slice {
		return nil, fmt.Errorf("list object type %T for kind %q does not have a settable slice field named Items", listObj, s.objGVK.Kind)
	}
	itemType := itemsField.Type().Elem() // e.g., v1.Pod
	resultSlice := reflect.MakeSlice(itemsField.Type(), 0, len(items))

	objs, err := anySliceToRuntimeObjSlice(s.log, items)
	if err != nil {
		return
	}
	for _, obj := range objs {
		metaV1Obj, err := AsMeta(s.log, obj)
		if err != nil {
			s.log.Error(err, "cannot access meta object", "obj", obj)
			continue
		}
		if namespace != "" && metaV1Obj.GetNamespace() != namespace {
			continue
		}
		if !labelSelector.Matches(labels.Set(metaV1Obj.GetLabels())) {
			continue
		}
		val := reflect.ValueOf(obj)
		if val.Kind() != reflect.Ptr || val.IsNil() {
			return nil, fmt.Errorf("element for kind %q is not a non-nil pointer: %T", s.objGVK, obj)
		}
		if val.Elem().Type() != itemType {
			return nil, fmt.Errorf("type mismatch, list kind %q expects items of type %v, but got %v", s.objListGVK, itemType, val.Elem().Type())
		}
		resultSlice = reflect.Append(resultSlice, val.Elem()) // append the dereferenced struct
	}
	itemsField.Set(resultSlice)
	listObj = ptr.Interface().(runtime.Object)
	return listObj, nil
}

func (s *InMemResourceStore) ListMetaObjects(c api.MatchCriteria) ([]metav1.Object, error) {
	items := s.delegate.List()
	objects := make([]metav1.Object, 0, 100)
	for _, item := range items {
		mo, err := AsMeta(s.log, item)
		if err != nil {
			err := fmt.Errorf("%w: %w", api.ErrListObjects, err)
			return nil, err
		}
		if c.Matches(mo) {
			objects = append(objects, mo)
		}
	}
	return objects, nil
}

func (s *InMemResourceStore) DeleteObjects(c api.MatchCriteria) error {
	items := s.delegate.List()
	for _, item := range items {
		mo, err := AsMeta(s.log, item)
		if err != nil {
			return fmt.Errorf("%w: %w", api.ErrDeleteObject, err)
		}
		if !c.Matches(mo) {
			continue
		}
		objKey := cache.NewObjectName(mo.GetNamespace(), mo.GetName()).String()
		if err := s.Delete(objKey); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("%w: %w", api.ErrDeleteObject, err)
			} else {
				s.log.Info("object to delete not found in store", "key", objKey)
			}
		}
	}
	return nil
}

func (s *InMemResourceStore) validateRuntimeObj(mo metav1.Object) (o runtime.Object, err error) {
	key := cache.NewObjectName(mo.GetNamespace(), mo.GetName())
	o, ok := mo.(runtime.Object)
	if !ok {
		err = fmt.Errorf("cannot convert meta object %q of type %T to runtime.Object", key, mo)
		return
	}
	oGVK := o.GetObjectKind().GroupVersionKind()
	if oGVK != s.objGVK {
		err = fmt.Errorf("object objGVK %q does not match expected objGVK %q", oGVK, s.objGVK)
	}
	return
}

func (s *InMemResourceStore) buildPendingWatchEvents(startVersion int64, namespace string, labelSelector labels.Selector) (watchEvents []watch.Event, err error) {
	var skip bool
	allItems := s.delegate.List()
	objs, err := anySliceToRuntimeObjSlice(s.log, allItems)
	if err != nil {
		return
	}
	for _, o := range objs {
		skip, err = shouldSkipObject(s.log, o, startVersion, namespace, labelSelector)
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

type EventCallbackFn func(watch.Event) (err error)

func (s *InMemResourceStore) Watch(ctx context.Context, startVersion int64, namespace string, labelSelector labels.Selector, eventCallback api.WatchEventCallback) error {
	events, err := s.buildPendingWatchEvents(startVersion, namespace, labelSelector)
	if err != nil {
		return err
	}
	watcher, err := s.broadcaster.WatchWithPrefix(events)
	if err != nil {
		return fmt.Errorf("cannot start watch for gvk %q: %w", s.objGVK, err)
	}
	defer watcher.Stop()
	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				s.log.V(4).Info("no more events on watch result channel for gvk.", "gvk", s.objGVK)
				return nil
			}
			skip, err := shouldSkipObject(s.log, event.Object, startVersion, namespace, labelSelector)
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
		case <-time.After(s.watchTimeout):
			s.log.V(4).Info("watcher timed out", "gvk", s.objGVK, "watchTimeout", s.watchTimeout, "startVersion", startVersion, "namespace", namespace, "labelSelector", labelSelector.String())
			return nil
		case <-ctx.Done():
			s.log.V(4).Info("watch context cancelled", "gvk", s.objGVK, "startVersion", startVersion, "namespace", namespace, "labelSelector", labelSelector.String())
			return nil
		}
	}
}

func (s *InMemResourceStore) CurrentResourceVersion() int64 {
	return s.rvCounter.Load()
}

func (s *InMemResourceStore) Shutdown() {
	if s.broadcaster != nil {
		s.log.V(4).Info("shutting down broadcaster for store", "gvk", s.objGVK)
		s.broadcaster.Shutdown()
	}
}

func (s *InMemResourceStore) NextResourceVersionAsString() string {
	return strconv.FormatInt(s.nextResourceVersion(), 10)
}

// nextResourceVersion increments and returns the next version for this store's GVK
func (s *InMemResourceStore) nextResourceVersion() int64 {
	return s.rvCounter.Add(1)
}

func shouldSkipObject(log logr.Logger, obj runtime.Object, startVersion int64, namespace string, labelSelector labels.Selector) (skip bool, err error) {
	o, err := meta.Accessor(obj)
	if err != nil {
		log.Error(err, "cannot access object metadata for obj", "obj", obj)
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
	rv, err := parseObjectResourceVersion(o)
	if err != nil {
		return
	}
	if rv <= startVersion {
		skip = true
		return
	}
	return
}

func anySliceToRuntimeObjSlice(log logr.Logger, objs []any) ([]runtime.Object, error) {
	result := make([]runtime.Object, 0, len(objs))
	for _, item := range objs {
		obj, ok := item.(runtime.Object)
		if !ok {
			err := fmt.Errorf("element %T does not implement runtime.Object", item)
			log.Error(err, "cannot convert 'any' slice to 'runtime.Object' slice because of object mismatch", "obj", obj)
			return nil, apierrors.NewInternalError(err)
		}
		result = append(result, obj)
	}
	return result, nil
}

func parseObjectResourceVersion(obj metav1.Object) (resourceVersion int64, err error) {
	resourceVersion, err = parseResourceVersion(obj.GetResourceVersion())
	if err != nil {
		err = fmt.Errorf("failed to parse resource version %q for object %q in ns %q: %w", obj.GetResourceVersion(), obj.GetName(), obj.GetNamespace(), err)
	}
	return
}

func parseResourceVersion(rvStr string) (resourceVersion int64, err error) {
	if rvStr != "" {
		resourceVersion, err = strconv.ParseInt(rvStr, 10, 64)
	}
	return
}

func AsMeta(log logr.Logger, o any) (mo metav1.Object, err error) {
	mo, err = meta.Accessor(o)
	if err != nil {
		log.Error(err, "cannot access meta object", "object", o)
		err = apierrors.NewInternalError(fmt.Errorf("cannot access meta object for o of type %T", o))
	}
	return
}
