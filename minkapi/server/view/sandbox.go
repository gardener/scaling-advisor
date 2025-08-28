package view

import (
	"context"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/common/clientutil"
	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/server/eventsink"
	"github.com/gardener/scaling-advisor/minkapi/server/store"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sync"
)

var _ api.View = (*sandboxView)(nil)

type sandboxView struct {
	log          logr.Logger
	args         *api.ViewArgs
	mu           sync.RWMutex
	stores       map[schema.GroupVersionKind]*store.InMemResourceStore
	eventSink    api.EventSink
	delegateView api.View
}

// NewSandbox returns a "sandbox" (private) view which holds changes made via its facade into its private store independent of the base view,
// otherwise delegating to the delegate View.
func NewSandbox(log logr.Logger, delegateView api.View, args *api.ViewArgs) (api.View, error) {
	stores := map[schema.GroupVersionKind]*store.InMemResourceStore{}
	for _, d := range typeinfo.SupportedDescriptors {
		stores[d.GVK] = store.NewInMemResourceStore(log, &api.ResourceStoreArgs{
			Name:          d.GVR.Resource,
			ObjectGVK:     d.GVK,
			ObjectListGVK: d.ListGVK,
			Scheme:        typeinfo.SupportedScheme,
			WatchConfig:   args.WatchConfig,
		})
		//stores[d.GVK] = store.NewInMemResourceStore(d.GVK, d.ListGVK, d.GVR.GroupResource().Resource, args.WatchConfig.QueueSize, args.WatchConfig.Timeout, typeinfo.SupportedScheme, log)
	}
	eventSink := eventsink.New(log)
	return &sandboxView{
		log:          log,
		args:         args,
		stores:       stores,
		eventSink:    eventSink,
		delegateView: delegateView,
	}, nil
}

func (s *sandboxView) GetName() string {
	return s.args.Name
}

func (s *sandboxView) GetType() api.ViewType {
	return api.SandboxViewType
}

func (s *sandboxView) GetClientFacades(clientType api.ClientType) (clientFacades commontypes.ClientFacades, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", api.ErrClientFacadesFailed, err)
		}
	}()
	if clientType == api.NetworkClient {
		clientFacades, err = clientutil.CreateNetworkClientFacades(s.log, s.args.KubeConfigPath, s.args.WatchConfig.Timeout)
		return
	} else {
		panic("inmem client type to be implemented")
	}
	return
}

func (s *sandboxView) GetResourceStore(gvk schema.GroupVersionKind) (api.ResourceStore, error) {
	//TODO implement me
	panic("implement me")
}

func (s *sandboxView) GetEventSink() api.EventSink {
	//TODO implement me
	panic("implement me")
}

func (s *sandboxView) StoreObject(gvk schema.GroupVersionKind, obj metav1.Object) error {
	//TODO implement me
	panic("implement me")
}

func (s *sandboxView) GetObject(gvk schema.GroupVersionKind, objName cache.ObjectName) (runtime.Object, error) {
	//TODO implement me
	panic("implement me")
}

func (s *sandboxView) UpdateObject(gvk schema.GroupVersionKind, obj metav1.Object) error {
	//TODO implement me
	panic("implement me")
}

func (s *sandboxView) UpdatePodNodeBinding(podName cache.ObjectName, binding corev1.Binding) (*corev1.Pod, error) {
	//TODO implement me
	panic("implement me")
}

func (s *sandboxView) PatchObject(gvk schema.GroupVersionKind, objName cache.ObjectName, patchType types.PatchType, patchData []byte) (patchedObj runtime.Object, err error) {
	//TODO implement me
	panic("implement me")
}

func (s *sandboxView) PatchObjectStatus(gvk schema.GroupVersionKind, objName cache.ObjectName, patchData []byte) (patchedObj runtime.Object, err error) {
	//TODO implement me
	panic("implement me")
}

func (s *sandboxView) ListObjects(gvk schema.GroupVersionKind, criteria api.MatchCriteria) (runtime.Object, error) {
	//TODO implement me
	panic("implement me")
}

func (s sandboxView) WatchObjects(ctx context.Context, gvk schema.GroupVersionKind, startVersion int64, namespace string, labelSelector labels.Selector, eventCallback api.WatchEventCallback) error {
	//TODO implement me
	panic("implement me")
}

func (s sandboxView) DeleteObject(gvk schema.GroupVersionKind, objName cache.ObjectName) error {
	//TODO implement me
	panic("implement me")
}

func (s sandboxView) DeleteObjects(gvk schema.GroupVersionKind, criteria api.MatchCriteria) error {
	//TODO implement me
	panic("implement me")
}

func (s sandboxView) ListNodes(matchingNodeNames ...string) ([]*corev1.Node, error) {
	//TODO implement me
	panic("implement me")
}

func (s sandboxView) ListPods(namespace string, matchingPodNames ...string) ([]*corev1.Pod, error) {
	//TODO implement me
	panic("implement me")
}

func (s sandboxView) ListEvents(namespace string) ([]*eventsv1.Event, error) {
	//TODO implement me
	panic("implement me")
}

func (s sandboxView) GetKubeConfigPath() string {
	//TODO implement me
	panic("implement me")
}

func (s sandboxView) Close() {
	//TODO implement me
	panic("implement me")
}
