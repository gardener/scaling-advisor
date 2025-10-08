// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/minkapi/server/inmclient/access"
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
	return access.NewConfigMapAccess(c.view, namespace)
}

func (c *coreV1Impl) Endpoints(namespace string) clientcorev1.EndpointsInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (c *coreV1Impl) Events(namespace string) clientcorev1.EventInterface {
	return access.NewEventAccess(c.view, namespace)
}

func (c *coreV1Impl) LimitRanges(namespace string) clientcorev1.LimitRangeInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (c *coreV1Impl) Namespaces() clientcorev1.NamespaceInterface {
	return access.NewNamespaceAccess(c.view)
}

func (c *coreV1Impl) Nodes() clientcorev1.NodeInterface {
	return access.NewNodeAccess(c.view)
}

func (c *coreV1Impl) PersistentVolumes() clientcorev1.PersistentVolumeInterface {
	return access.NewPersistentVolumeAccess(c.view)
}

func (c *coreV1Impl) PersistentVolumeClaims(namespace string) clientcorev1.PersistentVolumeClaimInterface {
	return access.NewPersistentVolumeClaimAccess(c.view, namespace)
}

func (c *coreV1Impl) Pods(namespace string) clientcorev1.PodInterface {
	return access.NewPodAccess(c.view, namespace)
}

func (c *coreV1Impl) PodTemplates(namespace string) clientcorev1.PodTemplateInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (c *coreV1Impl) ReplicationControllers(namespace string) clientcorev1.ReplicationControllerInterface {
	return access.NewReplicationControllerAccess(c.view, namespace)
}

func (c *coreV1Impl) ResourceQuotas(namespace string) clientcorev1.ResourceQuotaInterface {
	//TODO implement me
	panic("implement me")
}

func (c *coreV1Impl) Secrets(namespace string) clientcorev1.SecretInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (c *coreV1Impl) Services(namespace string) clientcorev1.ServiceInterface {
	return access.NewServiceAccess(c.view, namespace)
}

func (c *coreV1Impl) ServiceAccounts(namespace string) clientcorev1.ServiceAccountInterface {
	panic(commonerrors.ErrUnimplemented)
}
