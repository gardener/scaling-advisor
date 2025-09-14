package inmclient

import (
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

var (
	_ corev1.CoreV1Interface = (*coreV1Impl)(nil)
)

type coreV1Impl struct {
	view mkapi.View
}

func (c coreV1Impl) RESTClient() rest.Interface {
	panic("implement RESTClient")
}

func (c coreV1Impl) ComponentStatuses() corev1.ComponentStatusInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) ConfigMaps(namespace string) corev1.ConfigMapInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) Endpoints(namespace string) corev1.EndpointsInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) Events(namespace string) corev1.EventInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) LimitRanges(namespace string) corev1.LimitRangeInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) Namespaces() corev1.NamespaceInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) Nodes() corev1.NodeInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) PersistentVolumes() corev1.PersistentVolumeInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) PersistentVolumeClaims(namespace string) corev1.PersistentVolumeClaimInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) Pods(namespace string) corev1.PodInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) PodTemplates(namespace string) corev1.PodTemplateInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) ReplicationControllers(namespace string) corev1.ReplicationControllerInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) ResourceQuotas(namespace string) corev1.ResourceQuotaInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) Secrets(namespace string) corev1.SecretInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) Services(namespace string) corev1.ServiceInterface {
	//TODO implement me
	panic("implement me")
}

func (c coreV1Impl) ServiceAccounts(namespace string) corev1.ServiceAccountInterface {
	//TODO implement me
	panic("implement me")
}
