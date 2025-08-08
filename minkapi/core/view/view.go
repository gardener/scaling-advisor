package view

import (
	"fmt"
	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/core/eventsink"
	"github.com/gardener/scaling-advisor/minkapi/core/objutil"
	"github.com/gardener/scaling-advisor/minkapi/core/store"
	"github.com/gardener/scaling-advisor/minkapi/core/typeinfo"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"strings"
	"sync"
	"time"
)

var _ api.View = (*baseObjectView)(nil)

type baseObjectView struct {
	log                logr.Logger
	mu                 sync.RWMutex
	kubeConfigPath     string
	scheme             *runtime.Scheme
	stores             map[schema.GroupVersionKind]*store.InMemResourceStore
	client             kubernetes.Interface
	dynClient          dynamic.Interface
	informerFactory    informers.SharedInformerFactory
	dynInformerFactory dynamicinformer.DynamicSharedInformerFactory
	eventSink          api.EventSink
}

func New(log logr.Logger, kubeConfigPath string, scheme *runtime.Scheme, resyncPeriod time.Duration) (api.View, error) {
	client, dynClient, err := buildClients(kubeConfigPath)
	if err != nil {
		return nil, err
	}
	informerFactory, dynInformerFactory := buildInformerFactories(client, dynClient, resyncPeriod)
	eventSink := eventsink.New(log)
	return &baseObjectView{
		log:                log,
		kubeConfigPath:     kubeConfigPath,
		scheme:             scheme,
		stores:             make(map[schema.GroupVersionKind]*store.InMemResourceStore),
		client:             client,
		dynClient:          dynClient,
		informerFactory:    informerFactory,
		dynInformerFactory: dynInformerFactory,
		eventSink:          eventSink,
	}, nil
}

// buildClients currently constructs a remote client. TODO: change this later to an embedded client.
func buildClients(kubeConfigPath string) (kubernetes.Interface, dynamic.Interface, error) {
	clientConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", api.ErrInitFailed, err)
	}
	clientConfig.ContentType = "application/json"
	client, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", api.ErrInitFailed, err)
	}
	dynClient, err := dynamic.NewForConfig(clientConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", api.ErrInitFailed, err)
	}
	return client, dynClient, nil
}

func (b *baseObjectView) GetClients() (kubernetes.Interface, dynamic.Interface) {
	return b.client, b.dynClient
}

func (b *baseObjectView) GetInformerFactories() (informers.SharedInformerFactory, dynamicinformer.DynamicSharedInformerFactory) {
	return b.informerFactory, b.dynInformerFactory
}

func buildInformerFactories(client kubernetes.Interface, dyncClient dynamic.Interface, resyncPeriod time.Duration) (informerFactory informers.SharedInformerFactory, dynInformerFactory dynamicinformer.DynamicSharedInformerFactory) {
	informerFactory = informers.NewSharedInformerFactory(client, resyncPeriod)
	dynInformerFactory = dynamicinformer.NewDynamicSharedInformerFactory(dyncClient, resyncPeriod)
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
