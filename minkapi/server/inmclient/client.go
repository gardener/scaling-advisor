// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	discovery "k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	clientadmissionregistrationv1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"
	clientadmissionregistrationv1alpha1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1alpha1"
	clientadmissionregistrationv1beta1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1beta1"
	internalv1alpha1 "k8s.io/client-go/kubernetes/typed/apiserverinternal/v1alpha1"
	clientappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	clientappsv1beta1 "k8s.io/client-go/kubernetes/typed/apps/v1beta1"
	clientappsv1beta2 "k8s.io/client-go/kubernetes/typed/apps/v1beta2"
	clientauthenticationv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	clientauthenticationv1alpha1 "k8s.io/client-go/kubernetes/typed/authentication/v1alpha1"
	clientauthenticationv1beta1 "k8s.io/client-go/kubernetes/typed/authentication/v1beta1"
	clientauthorizationv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	clientauthorizationv1beta1 "k8s.io/client-go/kubernetes/typed/authorization/v1beta1"
	clientautoscalingv1 "k8s.io/client-go/kubernetes/typed/autoscaling/v1"
	clientautoscalingv2 "k8s.io/client-go/kubernetes/typed/autoscaling/v2"
	clientautoscalingv2beta1 "k8s.io/client-go/kubernetes/typed/autoscaling/v2beta1"
	clientautoscalingv2beta2 "k8s.io/client-go/kubernetes/typed/autoscaling/v2beta2"
	clientbatchv1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	clientbatchv1beta1 "k8s.io/client-go/kubernetes/typed/batch/v1beta1"
	clientcertificatesv1 "k8s.io/client-go/kubernetes/typed/certificates/v1"
	clientcertificatesv1alpha1 "k8s.io/client-go/kubernetes/typed/certificates/v1alpha1"
	clientcertificatesv1beta1 "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	clientcoordinationv1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
	clientcoordinationv1alpha2 "k8s.io/client-go/kubernetes/typed/coordination/v1alpha2"
	clientcoordinationv1beta1 "k8s.io/client-go/kubernetes/typed/coordination/v1beta1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	clientdiscoveryv1 "k8s.io/client-go/kubernetes/typed/discovery/v1"
	clientdiscoveryv1beta1 "k8s.io/client-go/kubernetes/typed/discovery/v1beta1"
	clienteventsv1 "k8s.io/client-go/kubernetes/typed/events/v1"
	clienteventsv1beta1 "k8s.io/client-go/kubernetes/typed/events/v1beta1"
	clientextensionsv1beta1 "k8s.io/client-go/kubernetes/typed/extensions/v1beta1"
	clientflowcontrolv1 "k8s.io/client-go/kubernetes/typed/flowcontrol/v1"
	clientflowcontrolv1beta1 "k8s.io/client-go/kubernetes/typed/flowcontrol/v1beta1"
	clientflowcontrolv1beta2 "k8s.io/client-go/kubernetes/typed/flowcontrol/v1beta2"
	clientflowcontrolv1beta3 "k8s.io/client-go/kubernetes/typed/flowcontrol/v1beta3"
	clientnetworkingv1 "k8s.io/client-go/kubernetes/typed/networking/v1"
	clientnetworkingv1alpha1 "k8s.io/client-go/kubernetes/typed/networking/v1alpha1"
	clientnetworkingv1beta1 "k8s.io/client-go/kubernetes/typed/networking/v1beta1"
	clientnodev1 "k8s.io/client-go/kubernetes/typed/node/v1"
	clientnodev1alpha1 "k8s.io/client-go/kubernetes/typed/node/v1alpha1"
	clientnodev1beta1 "k8s.io/client-go/kubernetes/typed/node/v1beta1"
	clientpolicyv1 "k8s.io/client-go/kubernetes/typed/policy/v1"
	clientpolicyv1beta1 "k8s.io/client-go/kubernetes/typed/policy/v1beta1"
	clientrbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
	clientrbacv1alpha1 "k8s.io/client-go/kubernetes/typed/rbac/v1alpha1"
	clientrbacv1beta1 "k8s.io/client-go/kubernetes/typed/rbac/v1beta1"
	clientresourcev1alpha3 "k8s.io/client-go/kubernetes/typed/resource/v1alpha3"
	clientresourcev1beta1 "k8s.io/client-go/kubernetes/typed/resource/v1beta1"
	clientresourcev1beta2 "k8s.io/client-go/kubernetes/typed/resource/v1beta2"
	clientschedulingv1 "k8s.io/client-go/kubernetes/typed/scheduling/v1"
	clientschedulingv1alpha1 "k8s.io/client-go/kubernetes/typed/scheduling/v1alpha1"
	clientschedulingv1beta1 "k8s.io/client-go/kubernetes/typed/scheduling/v1beta1"
	clientstoragev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
	clientstoragev1alpha1 "k8s.io/client-go/kubernetes/typed/storage/v1alpha1"
	clientstoragev1beta1 "k8s.io/client-go/kubernetes/typed/storage/v1beta1"
	clientstoragemigrationv1alpha1 "k8s.io/client-go/kubernetes/typed/storagemigration/v1alpha1"
)

var (
	_ kubernetes.Interface = (*inMemClient)(nil)
)

type inMemClient struct {
	view mkapi.View
}

// AppsV1 retrieves the AppsV1Client
func (c *inMemClient) AppsV1() clientappsv1.AppsV1Interface {
	return &appsV1Impl{c.view}
}

// CoreV1 retrieves the CoreV1Client
func (c *inMemClient) CoreV1() clientcorev1.CoreV1Interface {
	return &coreV1Impl{c.view}
}

// Discovery retrieves the DiscoveryClient
func (c *inMemClient) Discovery() discovery.DiscoveryInterface {
	panic("todo")
}

// DiscoveryV1 retrieves the DiscoveryV1Client
func (c *inMemClient) DiscoveryV1() clientdiscoveryv1.DiscoveryV1Interface {
	panic("todo?")
}

// EventsV1 retrieves the EventsV1Client
func (c *inMemClient) EventsV1() clienteventsv1.EventsV1Interface {
	panic("todo")
}

// RbacV1 retrieves the RbacV1Client
func (c *inMemClient) RbacV1() clientrbacv1.RbacV1Interface {
	panic("todo")
}

// SchedulingV1 retrieves the SchedulingV1Client
func (c *inMemClient) SchedulingV1() clientschedulingv1.SchedulingV1Interface {
	panic("todo")
}

// StorageV1 retrieves the StorageV1Client
func (c *inMemClient) StorageV1() clientstoragev1.StorageV1Interface {
	panic("todo")
}

// AdmissionregistrationV1 retrieves the AdmissionregistrationV1Client
func (c *inMemClient) AdmissionregistrationV1() clientadmissionregistrationv1.AdmissionregistrationV1Interface {
	panic("not implemented")
}

// AdmissionregistrationV1alpha1 retrieves the AdmissionregistrationV1alpha1Client
func (c *inMemClient) AdmissionregistrationV1alpha1() clientadmissionregistrationv1alpha1.AdmissionregistrationV1alpha1Interface {
	panic("not implemented")
}

// AdmissionregistrationV1beta1 retrieves the AdmissionregistrationV1beta1Client
func (c *inMemClient) AdmissionregistrationV1beta1() clientadmissionregistrationv1beta1.AdmissionregistrationV1beta1Interface {
	panic("not implemented")
}

// InternalV1alpha1 retrieves the InternalV1alpha1Client
func (c *inMemClient) InternalV1alpha1() internalv1alpha1.InternalV1alpha1Interface {
	panic("not implemented")
}

// AppsV1beta1 retrieves the AppsV1beta1Client
func (c *inMemClient) AppsV1beta1() clientappsv1beta1.AppsV1beta1Interface {
	panic("not implemented")
}

// AppsV1beta2 retrieves the AppsV1beta2Client
func (c *inMemClient) AppsV1beta2() clientappsv1beta2.AppsV1beta2Interface {
	panic("not implemented")
}

// AuthenticationV1 retrieves the AuthenticationV1Client
func (c *inMemClient) AuthenticationV1() clientauthenticationv1.AuthenticationV1Interface {
	panic("not implemented")
}

// AuthenticationV1alpha1 retrieves the AuthenticationV1alpha1Client
func (c *inMemClient) AuthenticationV1alpha1() clientauthenticationv1alpha1.AuthenticationV1alpha1Interface {
	panic("not implemented")
}

// AuthenticationV1beta1 retrieves the AuthenticationV1beta1Client
func (c *inMemClient) AuthenticationV1beta1() clientauthenticationv1beta1.AuthenticationV1beta1Interface {
	panic("not implemented")
}

// AuthorizationV1 retrieves the AuthorizationV1Client
func (c *inMemClient) AuthorizationV1() clientauthorizationv1.AuthorizationV1Interface {
	panic("not implemented")
}

// AuthorizationV1beta1 retrieves the AuthorizationV1beta1Client
func (c *inMemClient) AuthorizationV1beta1() clientauthorizationv1beta1.AuthorizationV1beta1Interface {
	panic("not implemented")
}

// AutoscalingV1 retrieves the AutoscalingV1Client
func (c *inMemClient) AutoscalingV1() clientautoscalingv1.AutoscalingV1Interface {
	panic("not implemented")
}

// AutoscalingV2 retrieves the AutoscalingV2Client
func (c *inMemClient) AutoscalingV2() clientautoscalingv2.AutoscalingV2Interface {
	panic("not implemented")
}

// AutoscalingV2beta1 retrieves the AutoscalingV2beta1Client
func (c *inMemClient) AutoscalingV2beta1() clientautoscalingv2beta1.AutoscalingV2beta1Interface {
	panic("not implemented")
}

// AutoscalingV2beta2 retrieves the AutoscalingV2beta2Client
func (c *inMemClient) AutoscalingV2beta2() clientautoscalingv2beta2.AutoscalingV2beta2Interface {
	panic("not implemented")
}

// BatchV1 retrieves the BatchV1Client
func (c *inMemClient) BatchV1() clientbatchv1.BatchV1Interface {
	panic("not implemented")
}

// BatchV1beta1 retrieves the BatchV1beta1Client
func (c *inMemClient) BatchV1beta1() clientbatchv1beta1.BatchV1beta1Interface {
	panic("not implemented")
}

// CertificatesV1 retrieves the CertificatesV1Client
func (c *inMemClient) CertificatesV1() clientcertificatesv1.CertificatesV1Interface {
	panic("not implemented")
}

// CertificatesV1beta1 retrieves the CertificatesV1beta1Client
func (c *inMemClient) CertificatesV1beta1() clientcertificatesv1beta1.CertificatesV1beta1Interface {
	panic("not implemented")
}

// CertificatesV1alpha1 retrieves the CertificatesV1alpha1Client
func (c *inMemClient) CertificatesV1alpha1() clientcertificatesv1alpha1.CertificatesV1alpha1Interface {
	panic("not implemented")
}

// CoordinationV1alpha2 retrieves the CoordinationV1alpha2Client
func (c *inMemClient) CoordinationV1alpha2() clientcoordinationv1alpha2.CoordinationV1alpha2Interface {
	panic("not implemented")
}

// CoordinationV1beta1 retrieves the CoordinationV1beta1Client
func (c *inMemClient) CoordinationV1beta1() clientcoordinationv1beta1.CoordinationV1beta1Interface {
	panic("not implemented")
}

// CoordinationV1 retrieves the CoordinationV1Client
func (c *inMemClient) CoordinationV1() clientcoordinationv1.CoordinationV1Interface {
	panic("not implemented")
}

// DiscoveryV1beta1 retrieves the DiscoveryV1beta1Client
func (c *inMemClient) DiscoveryV1beta1() clientdiscoveryv1beta1.DiscoveryV1beta1Interface {
	panic("not implemented")
}

// EventsV1beta1 retrieves the EventsV1beta1Client
func (c *inMemClient) EventsV1beta1() clienteventsv1beta1.EventsV1beta1Interface {
	panic("not implemented")
}

// ExtensionsV1beta1 retrieves the ExtensionsV1beta1Client
func (c *inMemClient) ExtensionsV1beta1() clientextensionsv1beta1.ExtensionsV1beta1Interface {
	panic("not implemented")
}

// FlowcontrolV1 retrieves the FlowcontrolV1Client
func (c *inMemClient) FlowcontrolV1() clientflowcontrolv1.FlowcontrolV1Interface {
	panic("not implemented")
}

// FlowcontrolV1beta1 retrieves the FlowcontrolV1beta1Client
func (c *inMemClient) FlowcontrolV1beta1() clientflowcontrolv1beta1.FlowcontrolV1beta1Interface {
	panic("not implemented")
}

// FlowcontrolV1beta2 retrieves the FlowcontrolV1beta2Client
func (c *inMemClient) FlowcontrolV1beta2() clientflowcontrolv1beta2.FlowcontrolV1beta2Interface {
	panic("not implemented")
}

// FlowcontrolV1beta3 retrieves the FlowcontrolV1beta3Client
func (c *inMemClient) FlowcontrolV1beta3() clientflowcontrolv1beta3.FlowcontrolV1beta3Interface {
	panic("not implemented")
}

// NetworkingV1 retrieves the NetworkingV1Client
func (c *inMemClient) NetworkingV1() clientnetworkingv1.NetworkingV1Interface {
	panic("not implemented")
}

// NetworkingV1alpha1 retrieves the NetworkingV1alpha1Client
func (c *inMemClient) NetworkingV1alpha1() clientnetworkingv1alpha1.NetworkingV1alpha1Interface {
	panic("not implemented")
}

// NetworkingV1beta1 retrieves the NetworkingV1beta1Client
func (c *inMemClient) NetworkingV1beta1() clientnetworkingv1beta1.NetworkingV1beta1Interface {
	panic("not implemented")
}

// NodeV1 retrieves the NodeV1Client
func (c *inMemClient) NodeV1() clientnodev1.NodeV1Interface {
	panic("not implemented")
}

// NodeV1alpha1 retrieves the NodeV1alpha1Client
func (c *inMemClient) NodeV1alpha1() clientnodev1alpha1.NodeV1alpha1Interface {
	panic("not implemented")
}

// NodeV1beta1 retrieves the NodeV1beta1Client
func (c *inMemClient) NodeV1beta1() clientnodev1beta1.NodeV1beta1Interface {
	panic("not implemented")
}

// PolicyV1 retrieves the PolicyV1Client
func (c *inMemClient) PolicyV1() clientpolicyv1.PolicyV1Interface {
	panic("not implemented")
}

// PolicyV1beta1 retrieves the PolicyV1beta1Client
func (c *inMemClient) PolicyV1beta1() clientpolicyv1beta1.PolicyV1beta1Interface {
	panic("not implemented")
}

// RbacV1beta1 retrieves the RbacV1beta1Client
func (c *inMemClient) RbacV1beta1() clientrbacv1beta1.RbacV1beta1Interface {
	panic("not implemented")
}

// RbacV1alpha1 retrieves the RbacV1alpha1Client
func (c *inMemClient) RbacV1alpha1() clientrbacv1alpha1.RbacV1alpha1Interface {
	panic("not implemented")
}

// ResourceV1beta2 retrieves the ResourceV1beta2Client
func (c *inMemClient) ResourceV1beta2() clientresourcev1beta2.ResourceV1beta2Interface {
	panic("not implemented")
}

// ResourceV1beta1 retrieves the ResourceV1beta1Client
func (c *inMemClient) ResourceV1beta1() clientresourcev1beta1.ResourceV1beta1Interface {
	panic("not implemented")
}

// ResourceV1alpha3 retrieves the ResourceV1alpha3Client
func (c *inMemClient) ResourceV1alpha3() clientresourcev1alpha3.ResourceV1alpha3Interface {
	panic("not implemented")
}

// SchedulingV1alpha1 retrieves the SchedulingV1alpha1Client
func (c *inMemClient) SchedulingV1alpha1() clientschedulingv1alpha1.SchedulingV1alpha1Interface {
	panic("not implemented")
}

// SchedulingV1beta1 retrieves the SchedulingV1beta1Client
func (c *inMemClient) SchedulingV1beta1() clientschedulingv1beta1.SchedulingV1beta1Interface {
	panic("not implemented")
}

// StorageV1beta1 retrieves the StorageV1beta1Client
func (c *inMemClient) StorageV1beta1() clientstoragev1beta1.StorageV1beta1Interface {
	panic("not implemented")
}

// StorageV1alpha1 retrieves the StorageV1alpha1Client
func (c *inMemClient) StorageV1alpha1() clientstoragev1alpha1.StorageV1alpha1Interface {
	panic("not implemented")
}

// StoragemigrationV1alpha1 retrieves the StoragemigrationV1alpha1Client
func (c *inMemClient) StoragemigrationV1alpha1() clientstoragemigrationv1alpha1.StoragemigrationV1alpha1Interface {
	panic("not implemented")
}
