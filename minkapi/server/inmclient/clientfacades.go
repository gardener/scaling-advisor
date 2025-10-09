package inmclient

import (
	"time"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
)

func NewInMemClientFacades(view mkapi.View, resyncPeriod time.Duration) commontypes.ClientFacades {
	client := &inMemClient{view: view}
	informerFactory := informers.NewSharedInformerFactory(client, resyncPeriod)
	return commontypes.ClientFacades{
		Mode:               commontypes.ClientAccessInMemory,
		Client:             client,
		DynClient:          nil, // TODO: develop this
		InformerFactory:    informerFactory,
		DynInformerFactory: &inMemDummyDynInformerFactory{},
	}
}

var (
	_ dynamicinformer.DynamicSharedInformerFactory = (*inMemDummyDynInformerFactory)(nil)
)

type inMemDummyDynInformerFactory struct {
}

func (i *inMemDummyDynInformerFactory) Start(stopCh <-chan struct{}) {
	return
}

func (i *inMemDummyDynInformerFactory) ForResource(gvr schema.GroupVersionResource) informers.GenericInformer {
	panic(commonerrors.ErrUnimplemented)
}

func (i *inMemDummyDynInformerFactory) WaitForCacheSync(stopCh <-chan struct{}) map[schema.GroupVersionResource]bool {
	return map[schema.GroupVersionResource]bool{}
}

func (i *inMemDummyDynInformerFactory) Shutdown() {
	return
}
