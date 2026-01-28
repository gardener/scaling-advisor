// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
	"time"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
)

// NewInMemClientFacades returns ClientFacades populated with in-memory client and informer facades.
func NewInMemClientFacades(view minkapi.View, resyncPeriod time.Duration) commontypes.ClientFacades {
	client := &inMemClient{view: view}
	informerFactory := informers.NewSharedInformerFactory(client, resyncPeriod)
	return commontypes.ClientFacades{
		Mode:               commontypes.ClientAccessModeInMemory,
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

// Start is a no-op implementation
func (i *inMemDummyDynInformerFactory) Start(_ <-chan struct{}) {
}

// ForResource is not implemented and panics.
func (i *inMemDummyDynInformerFactory) ForResource(_ schema.GroupVersionResource) informers.GenericInformer {
	panic(commonerrors.ErrUnimplemented)
}

// WaitForCacheSync is a no-op implementation that returns an empty map.
func (i *inMemDummyDynInformerFactory) WaitForCacheSync(_ <-chan struct{}) map[schema.GroupVersionResource]bool {
	return map[schema.GroupVersionResource]bool{}
}

// Shutdown is a no-op implementation.
func (i *inMemDummyDynInformerFactory) Shutdown() {
}
