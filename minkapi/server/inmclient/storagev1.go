// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/minkapi/server/inmclient/access"
	clientstoragev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
	"k8s.io/client-go/rest"
)

var (
	_ clientstoragev1.StorageV1Interface = (*storageV1Impl)(nil)
)

type storageV1Impl struct {
	view mkapi.View
}

func (a *storageV1Impl) RESTClient() rest.Interface {
	panic(commonerrors.ErrUnimplemented) //TODO: provide a common implementation of rest.Interface for any resource
}

func (a *storageV1Impl) CSIDrivers() clientstoragev1.CSIDriverInterface {
	return access.NewCSIDriverAccess(a.view)
}

func (a *storageV1Impl) CSINodes() clientstoragev1.CSINodeInterface {
	return access.NewCSINodeAccess(a.view)
}

func (a *storageV1Impl) CSIStorageCapacities(namespace string) clientstoragev1.CSIStorageCapacityInterface {
	return access.NewCSIStorageCapacityAccess(a.view, namespace)
}

func (a *storageV1Impl) StorageClasses() clientstoragev1.StorageClassInterface {
	return access.NewStorageClassAccess(a.view)
}

func (a *storageV1Impl) VolumeAttachments() clientstoragev1.VolumeAttachmentInterface {
	return access.NewVolumeAttachmentAccess(a.view)
}

func (a *storageV1Impl) VolumeAttributesClasses() clientstoragev1.VolumeAttributesClassInterface {
	return access.NewVolumeAttributesClassAccess(a.view)
}
