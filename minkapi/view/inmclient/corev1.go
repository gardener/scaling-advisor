// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access/coreaccess"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

var (
	_ clientcorev1.CoreV1Interface = (*coreV1Impl)(nil)
)

type coreV1Impl struct {
	view mkapi.View
}

func (c *coreV1Impl) RESTClient() rest.Interface {
	panic(commonerrors.ErrUnimplemented)
}

func (c *coreV1Impl) ComponentStatuses() clientcorev1.ComponentStatusInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (c *coreV1Impl) ConfigMaps(namespace string) clientcorev1.ConfigMapInterface {
	return coreaccess.NewConfigMapAccess(c.view, namespace)
}

func (c *coreV1Impl) Endpoints(_ string) clientcorev1.EndpointsInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (c *coreV1Impl) Events(namespace string) clientcorev1.EventInterface {
	return coreaccess.NewEventAccess(c.view, namespace)
}

func (c *coreV1Impl) LimitRanges(_ string) clientcorev1.LimitRangeInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (c *coreV1Impl) Namespaces() clientcorev1.NamespaceInterface {
	return coreaccess.NewNamespaceAccess(c.view)
}

func (c *coreV1Impl) Nodes() clientcorev1.NodeInterface {
	return coreaccess.NewNodeAccess(c.view)
}

func (c *coreV1Impl) PersistentVolumes() clientcorev1.PersistentVolumeInterface {
	return coreaccess.NewPersistentVolumeAccess(c.view)
}

func (c *coreV1Impl) PersistentVolumeClaims(namespace string) clientcorev1.PersistentVolumeClaimInterface {
	return coreaccess.NewPersistentVolumeClaimAccess(c.view, namespace)
}

func (c *coreV1Impl) Pods(namespace string) clientcorev1.PodInterface {
	return coreaccess.NewPodAccess(c.view, namespace)
}

func (c *coreV1Impl) PodTemplates(_ string) clientcorev1.PodTemplateInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (c *coreV1Impl) ReplicationControllers(namespace string) clientcorev1.ReplicationControllerInterface {
	return coreaccess.NewReplicationControllerAccess(c.view, namespace)
}

func (c *coreV1Impl) ResourceQuotas(_ string) clientcorev1.ResourceQuotaInterface {
	//TODO implement me
	panic("implement me")
}

func (c *coreV1Impl) Secrets(_ string) clientcorev1.SecretInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (c *coreV1Impl) Services(namespace string) clientcorev1.ServiceInterface {
	return coreaccess.NewServiceAccess(c.view, namespace)
}

func (c *coreV1Impl) ServiceAccounts(_ string) clientcorev1.ServiceAccountInterface {
	panic(commonerrors.ErrUnimplemented)
}
