package view

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/server/eventsink"
	"github.com/gardener/scaling-advisor/minkapi/server/objutil"
	"github.com/gardener/scaling-advisor/minkapi/server/store"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	"github.com/gardener/scaling-advisor/common/clientutil"
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

var _ api.View = (*baseObjectView)(nil)

type baseObjectView struct {
	log               logr.Logger
	mu                sync.RWMutex
	kubeConfigPath    string
	scheme            *runtime.Scheme
	stores            map[schema.GroupVersionKind]*store.InMemResourceStore
	eventSink         api.EventSink
	watchQueueTimeout time.Duration
}

func New(log logr.Logger, kubeConfigPath string, scheme *runtime.Scheme, watchQueueSize int, watchQueueTimeout time.Duration) (api.View, error) {
	stores := map[schema.GroupVersionKind]*store.InMemResourceStore{}
	for _, d := range typeinfo.SupportedDescriptors {
		stores[d.GVK] = store.NewInMemResourceStore(d.GVK, d.ListGVK, d.GVR.GroupResource().Resource, watchQueueSize, watchQueueTimeout, typeinfo.SupportedScheme, log)
	}
	eventSink := eventsink.New(log)
	return &baseObjectView{
		log:               log,
		kubeConfigPath:    kubeConfigPath,
		scheme:            scheme,
		stores:            stores,
		eventSink:         eventSink,
		watchQueueTimeout: watchQueueTimeout,
	}, nil
}

func (b *baseObjectView) GetClientFacades() (clientFacades *api.ClientFacades, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", api.ErrClientFacadesFailed, err)
		}
	}()
	client, dynClient, err := clientutil.BuildClients(b.kubeConfigPath) //TODO: Make in-mem clients here.
	if err != nil {
		return
	}
	informerFactory, dynInformerFactory := clientutil.BuildInformerFactories(client, dynClient, b.watchQueueTimeout)
	clientFacades = &api.ClientFacades{
		Client:             client,
		DynClient:          dynClient,
		InformerFactory:    informerFactory,
		DynInformerFactory: dynInformerFactory,
	}
	return
}

func (b *baseObjectView) GetEventSink() api.EventSink {
	return b.eventSink
}

func (b *baseObjectView) GetResourceStore(gvk schema.GroupVersionKind) (api.ResourceStore, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	s, exists := b.stores[gvk]
	if !exists {
		return nil, fmt.Errorf("%w: store not found for GVK %q", api.ErrStoreNotFound, gvk)
	}
	return s, nil
}

func (b *baseObjectView) CreateObject(gvk schema.GroupVersionKind, obj metav1.Object) error {
	s, err := b.GetResourceStore(gvk)
	if err != nil {
		return err
	}

	name := obj.GetName()
	namePrefix := obj.GetGenerateName()
	if name == "" {
		if namePrefix == "" {
			return fmt.Errorf("%w: missing both name and generateName in request for creating object of objGvk %q in %q namespace", api.ErrCreateObject, gvk, obj.GetNamespace())
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

	return nil
}

func (b *baseObjectView) DeleteObjects(gvk schema.GroupVersionKind, criteria api.MatchCriteria) error {
	s, err := b.GetResourceStore(gvk)
	if err != nil {
		return err
	}
	return s.DeleteObjects(criteria)
}

func (b *baseObjectView) ListNodes(matchingNodeNames ...string) ([]*corev1.Node, error) {
	nodeNamesSet := sets.New(matchingNodeNames...)
	c := api.MatchCriteria{
		Names: nodeNamesSet,
	}
	gvk := typeinfo.NodesDescriptor.GVK
	s, err := b.GetResourceStore(gvk)
	if err != nil {
		return nil, err
	}
	objs, err := s.ListMetaObjects(c)
	if err != nil {
		return nil, err
	}
	nodes := make([]*corev1.Node, 0, len(objs))
	for _, obj := range objs {
		nodes = append(nodes, obj.(*corev1.Node))
	}
	return nodes, nil
}

func (b *baseObjectView) ListPods(namespace string, matchingPodNames ...string) ([]*corev1.Pod, error) {
	if len(strings.TrimSpace(namespace)) == 0 {
		return nil, apierrors.NewBadRequest("cannot list pods without namespace")
	}
	podNamesSet := sets.New(matchingPodNames...)
	c := api.MatchCriteria{
		Namespace: namespace,
		Names:     podNamesSet,
	}
	gvk := typeinfo.PodsDescriptor.GVK
	s, err := b.GetResourceStore(gvk)
	if err != nil {
		return nil, err
	}
	objs, err := s.ListMetaObjects(c)
	if err != nil {
		return nil, err
	}
	pods := make([]*corev1.Pod, 0, len(objs))
	for _, obj := range objs {
		pods = append(pods, obj.(*corev1.Pod))
	}
	return pods, nil
}

func (b *baseObjectView) ListEvents(namespace string) ([]*eventsv1.Event, error) {
	if len(strings.TrimSpace(namespace)) == 0 {
		return nil, apierrors.NewBadRequest("cannot list events without namespace")
	}
	c := api.MatchCriteria{
		Namespace: namespace,
	}
	gvk := typeinfo.EventsDescriptor.GVK
	s, err := b.GetResourceStore(gvk)
	if err != nil {
		return nil, err
	}
	objs, err := s.ListMetaObjects(c)
	if err != nil {
		return nil, err
	}
	events := make([]*eventsv1.Event, 0, len(objs))
	for _, obj := range objs {
		events = append(events, obj.(*eventsv1.Event))
	}
	return events, nil
}

func (b *baseObjectView) GetKubeConfigPath() string {
	return b.kubeConfigPath
}

func (b *baseObjectView) Close() {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.stores {
		s.Shutdown()
	}
}
