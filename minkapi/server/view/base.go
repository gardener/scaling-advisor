// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"context"
	"errors"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/common/clientutil"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/watch"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"github.com/gardener/scaling-advisor/minkapi/server/eventsink"
	"github.com/gardener/scaling-advisor/minkapi/server/store"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
)

var (
	_ minkapi.View = (*baseView)(nil)
	_              = NewSandbox
)

type baseView struct {
	log         logr.Logger
	args        *minkapi.ViewArgs
	mu          *sync.RWMutex
	stores      map[schema.GroupVersionKind]*store.InMemResourceStore
	eventSink   minkapi.EventSink
	changeCount atomic.Int64
}

func New(log logr.Logger, args *minkapi.ViewArgs) (minkapi.View, error) {
	stores := map[schema.GroupVersionKind]*store.InMemResourceStore{}
	for _, d := range typeinfo.SupportedDescriptors {
		versionCounter := &atomic.Int64{}
		stores[d.GVK] = createInMemStore(log, d, versionCounter, args)
	}
	eventSink := eventsink.New(log)
	return &baseView{
		log:       log,
		args:      args,
		stores:    stores,
		eventSink: eventSink,
		mu:        &sync.RWMutex{},
	}, nil
}

func (v *baseView) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	resetStores(v.stores)
	v.eventSink.Reset()
}

func (v *baseView) Close() error {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return closeStores(v.stores)
}

func (v *baseView) GetName() string {
	return v.args.Name
}

func (v *baseView) GetType() minkapi.ViewType {
	return minkapi.BaseViewType
}

func (v *baseView) GetObjectChangeCount() int64 {
	return v.changeCount.Load()
}

func (v *baseView) GetClientFacades() (clientFacades commontypes.ClientFacades, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", minkapi.ErrClientFacadesFailed, err)
		}
	}()
	// TODO: return instance of in-memory client.
	return clientutil.CreateNetworkClientFacades(v.log, v.GetKubeConfigPath(), v.args.WatchConfig.Timeout)
}

func (v *baseView) GetEventSink() minkapi.EventSink {
	return v.eventSink
}

func (v *baseView) GetResourceStore(gvk schema.GroupVersionKind) (minkapi.ResourceStore, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	s, exists := v.stores[gvk]
	if !exists {
		return nil, fmt.Errorf("%w: store not found for GVK %q", minkapi.ErrStoreNotFound, gvk)
	}
	return s, nil
}

func (v *baseView) CreateObject(gvk schema.GroupVersionKind, obj metav1.Object) error {
	return storeObject(v, gvk, obj, &v.changeCount)
}

func (v *baseView) GetObject(gvk schema.GroupVersionKind, objName cache.ObjectName) (obj runtime.Object, err error) {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return
	}
	key := objName.String()
	obj, err = s.GetByKey(key)
	return
}

func (v *baseView) UpdateObject(gvk schema.GroupVersionKind, obj metav1.Object) error {
	return updateObject(v, gvk, obj, &v.changeCount)
}

func (v *baseView) UpdatePodNodeBinding(podName cache.ObjectName, binding corev1.Binding) (*corev1.Pod, error) {
	obj, err := v.GetObject(typeinfo.PodsDescriptor.GVK, podName)
	if err != nil {
		return nil, err
	}
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, fmt.Errorf("%w: cannot update pod node binding since obj %T for name %q not a corev1.Pod", minkapi.ErrUpdateObject, obj, podName)
	}
	return updatePodNodeBinding(v, pod, binding)
}

func (v *baseView) PatchObject(gvk schema.GroupVersionKind, objName cache.ObjectName, patchType types.PatchType, patchData []byte) (patchedObj runtime.Object, err error) {
	return patchObject(v, gvk, objName, patchType, patchData)
}

func (v *baseView) PatchObjectStatus(gvk schema.GroupVersionKind, objName cache.ObjectName, patchData []byte) (patchedObj runtime.Object, err error) {
	return patchObjectStatus(v, gvk, objName, patchData)
}

func (v *baseView) ListMetaObjects(gvk schema.GroupVersionKind, criteria minkapi.MatchCriteria) ([]metav1.Object, int64, error) {
	return listMetaObjects(v, gvk, criteria)
}

func (v *baseView) ListObjects(gvk schema.GroupVersionKind, criteria minkapi.MatchCriteria) (runtime.Object, error) {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return nil, err
	}
	listObj, err := s.List(criteria)
	if err != nil {
		return nil, err
	}
	return listObj, nil
}

func (v *baseView) WatchObjects(ctx context.Context, gvk schema.GroupVersionKind, startVersion int64, namespace string, labelSelector labels.Selector, eventCallback minkapi.WatchEventCallback) error {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return err
	}
	return s.Watch(ctx, startVersion, namespace, labelSelector, eventCallback)
}

func (v *baseView) GetWatcher(ctx context.Context, gvk schema.GroupVersionKind, startVersion int64, namespace string, labelSelector labels.Selector) (watch.Interface, error) {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return nil, err
	}
	return s.GetWatcher(ctx, startVersion, namespace, labelSelector), nil
}

func (v *baseView) DeleteObject(gvk schema.GroupVersionKind, objName cache.ObjectName) error {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return err
	}
	err = s.Delete(objName)
	if err != nil {
		return err
	}
	v.changeCount.Add(1)
	return nil
}

func (v *baseView) DeleteObjects(gvk schema.GroupVersionKind, criteria minkapi.MatchCriteria) error {
	return deleteObjects(v, gvk, criteria, &v.changeCount)
}

func (v *baseView) ListNodes(matchingNodeNames ...string) (nodes []corev1.Node, err error) {
	nodes, _, err = listNodes(v, matchingNodeNames)
	return
}

func (v *baseView) ListPods(c minkapi.MatchCriteria) ([]corev1.Pod, error) {
	gvk := typeinfo.PodsDescriptor.GVK
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return nil, err
	}
	objs, _, err := s.ListMetaObjects(c)
	if err != nil {
		return nil, err
	}
	pods := make([]corev1.Pod, 0, len(objs))
	for _, obj := range objs {
		pods = append(pods, *obj.(*corev1.Pod))
	}
	return pods, nil
}

func (v *baseView) ListEvents(namespace string) ([]eventsv1.Event, error) {
	if len(strings.TrimSpace(namespace)) == 0 {
		return nil, apierrors.NewBadRequest("cannot list events without namespace")
	}
	c := minkapi.MatchCriteria{
		Namespace: namespace,
	}
	gvk := typeinfo.EventsDescriptor.GVK
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return nil, err
	}
	objs, _, err := s.ListMetaObjects(c)
	if err != nil {
		return nil, err
	}
	events := make([]eventsv1.Event, 0, len(objs))
	for _, obj := range objs {
		events = append(events, *obj.(*eventsv1.Event))
	}
	return events, nil
}

func (v *baseView) GetKubeConfigPath() string {
	return v.args.KubeConfigPath
}

func storeObject(v minkapi.View, gvk schema.GroupVersionKind, obj metav1.Object, counter *atomic.Int64) error {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return err
	}

	name := obj.GetName()
	namePrefix := obj.GetGenerateName()
	if name == "" {
		if namePrefix == "" {
			return apierrors.NewBadRequest(fmt.Errorf("%w: cannot create %q object in %q namespace since missing both name and generateName in request", minkapi.ErrCreateObject, gvk.Kind, obj.GetNamespace()).Error())
		}
		name = typeinfo.GenerateName(namePrefix)
	}
	obj.SetName(name)

	createTimestamp := obj.GetCreationTimestamp()
	if (&createTimestamp).IsZero() { // only set creationTimestamp if not already set.
		obj.SetCreationTimestamp(metav1.Time{Time: time.Now()})
	}

	if obj.GetUID() == "" {
		obj.SetUID(uuid.NewUUID())
	}

	objutil.SetMetaObjectGVK(obj, gvk)

	err = s.Add(obj)
	if err != nil {
		return err
	}
	counter.Add(1)
	return nil
}

func updateObject(v minkapi.View, gvk schema.GroupVersionKind, obj metav1.Object, changeCount *atomic.Int64) error {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return err
	}
	err = s.Update(obj)
	if err != nil {
		return err
	}
	changeCount.Add(1)
	return nil
}
func updatePodNodeBinding(v minkapi.View, pod *corev1.Pod, binding corev1.Binding) (*corev1.Pod, error) {
	pod.Spec.NodeName = binding.Target.Name
	podutil.UpdatePodCondition(&pod.Status, &corev1.PodCondition{
		Type:   corev1.PodScheduled,
		Status: corev1.ConditionTrue,
	})
	err := v.UpdateObject(typeinfo.PodsDescriptor.GVK, pod)
	if err != nil {
		return nil, err
	}
	return pod, nil
}

func patchObject(v minkapi.View, gvk schema.GroupVersionKind, objName cache.ObjectName, patchType types.PatchType, patchData []byte) (patchedObj runtime.Object, err error) {
	obj, err := v.GetObject(gvk, objName)
	if err != nil {
		return
	}
	err = objutil.PatchObject(obj, objName, patchType, patchData)
	if err != nil {
		err = fmt.Errorf("failed to patch object %q: %w", objName, err)
		return
	}
	mo, err := meta.Accessor(obj)
	if err != nil {
		err = fmt.Errorf("stored object with key %q is not metav1.Object: %w", objName, err)
		return
	}
	if mo.GetName() != objName.Name {
		fieldErr := field.Error{
			Type:     field.ErrorTypeInvalid,
			BadValue: mo.GetName(),
			Field:    "metadata.name",
			Detail:   fmt.Sprintf("Invalid value: %q: field is immutable", mo.GetName()),
		}
		err = apierrors.NewInvalid(gvk.GroupKind(), objName.Name, field.ErrorList{&fieldErr})
		return
	}
	if mo.GetNamespace() != objName.Namespace {
		fieldErr := field.Error{
			Type:     field.ErrorTypeInvalid,
			Field:    "metadata.namespace",
			BadValue: mo.GetNamespace(),
			Detail:   fmt.Sprintf("Invalid value: %q: field is immutable", mo.GetNamespace()),
		}
		err = apierrors.NewInvalid(gvk.GroupKind(), objName.Name, field.ErrorList{&fieldErr})
		return
	}

	err = v.UpdateObject(gvk, mo)
	if err != nil {
		return
	}
	patchedObj = obj
	return
}
func patchObjectStatus(v minkapi.View, gvk schema.GroupVersionKind, objName cache.ObjectName, patchData []byte) (patchedObj runtime.Object, err error) {
	obj, err := v.GetObject(gvk, objName)
	if err != nil {
		return
	}
	err = objutil.PatchObjectStatus(obj, objName, patchData)
	if err != nil {
		err = fmt.Errorf("failed to patch object status of %q: %w", objName, err)
		return
	}
	mo, err := meta.Accessor(obj)
	if err != nil {
		err = fmt.Errorf("stored object with key %q is not metav1.Object: %w", objName, err)
		return
	}
	err = v.UpdateObject(gvk, mo)
	if err != nil {
		return
	}
	patchedObj = obj
	return
}

func listMetaObjects(v minkapi.View, gvk schema.GroupVersionKind, criteria minkapi.MatchCriteria) (metaObjects []metav1.Object, maxVersion int64, err error) {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return
	}
	return s.ListMetaObjects(criteria)
}

func deleteObjects(v minkapi.View, gvk schema.GroupVersionKind, criteria minkapi.MatchCriteria, changeCount *atomic.Int64) error {
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return err
	}
	delCount, err := s.DeleteObjects(criteria)
	if err != nil {
		return err
	}
	changeCount.Add(int64(delCount))
	return nil
}

func listNodes(v minkapi.View, matchingNodeNames []string) (nodes []corev1.Node, maxVersion int64, err error) {
	nodeNamesSet := sets.New(matchingNodeNames...)
	c := minkapi.MatchCriteria{
		Names: nodeNamesSet,
	}
	gvk := typeinfo.NodesDescriptor.GVK
	s, err := v.GetResourceStore(gvk)
	if err != nil {
		return
	}
	objs, _, err := s.ListMetaObjects(c)
	if err != nil {
		return
	}
	return asNodes(objs)
}
func asNodes(metaObjects []metav1.Object) (nodes []corev1.Node, maxVersion int64, err error) {
	nodes = make([]corev1.Node, 0, len(metaObjects))
	var version int64
	for _, obj := range metaObjects {
		n, ok := obj.(*corev1.Node)
		if !ok {
			err = fmt.Errorf("object %q is not a corev1.Node", objutil.CacheName(obj))
			return
		}
		version, err = store.ParseObjectResourceVersion(obj)
		if err != nil {
			return
		}
		nodes = append(nodes, *n)
		if version > maxVersion {
			maxVersion = version
		}
	}
	return
}

func asPods(metaObjects []metav1.Object) (pods []corev1.Pod, maxVersion int64, err error) {
	pods = make([]corev1.Pod, 0, len(metaObjects))
	var version int64
	for _, obj := range metaObjects {
		p, ok := obj.(*corev1.Pod)
		if !ok {
			err = fmt.Errorf("object %q is not a corev1.Pod", objutil.CacheName(obj))
			return
		}
		version, err = store.ParseObjectResourceVersion(obj)
		if err != nil {
			return
		}
		pods = append(pods, *p)
		if version > maxVersion {
			maxVersion = version
		}
	}
	return
}

func asEvents(metaObjects []metav1.Object) (events []eventsv1.Event, maxVersion int64, err error) {
	events = make([]eventsv1.Event, 0, len(metaObjects))
	var version int64
	for _, obj := range metaObjects {
		e, ok := obj.(*eventsv1.Event)
		if !ok {
			err = fmt.Errorf("object %q is not a corev1.Pod", objutil.CacheName(obj))
			return
		}
		version, err = store.ParseObjectResourceVersion(obj)
		if err != nil {
			return
		}
		events = append(events, *e)
		if version > maxVersion {
			maxVersion = version
		}
	}
	return
}

// combinePrimarySecondary gets a combined slice of metav1.Objects preferring objects in primary over same obj in secondary
func combinePrimarySecondary(primary []metav1.Object, secondary []metav1.Object) (combined []metav1.Object) {
	found := make(map[cache.ObjectName]bool, len(primary))
	for _, o := range primary {
		found[objutil.CacheName(o)] = true
	}
	for _, o := range secondary {
		if found[objutil.CacheName(o)] {
			continue
		}
		combined = append(combined, o)
	}
	for _, o := range primary {
		combined = append(combined, o)
	}
	return
}

func createInMemStore(log logr.Logger, d typeinfo.Descriptor, versionCounter *atomic.Int64, args *minkapi.ViewArgs) *store.InMemResourceStore {
	return store.NewInMemResourceStore(log, &minkapi.ResourceStoreArgs{
		Name:           d.GVR.Resource,
		ObjectGVK:      d.GVK,
		ObjectListGVK:  d.ListGVK,
		Scheme:         typeinfo.SupportedScheme,
		VersionCounter: versionCounter,
		WatchConfig:    args.WatchConfig,
	})
}
func closeStores(stores map[schema.GroupVersionKind]*store.InMemResourceStore) error {
	var errs []error
	for _, s := range stores {
		errs = append(errs, s.Close())
	}
	return errors.Join(errs...)
}

func resetStores(stores map[schema.GroupVersionKind]*store.InMemResourceStore) {
	for _, s := range stores {
		s.Reset()
	}
}
