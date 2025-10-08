// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package typeinfo

import (
	"fmt"
	"maps"
	"slices"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	"github.com/gardener/scaling-advisor/common/objutil"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	nodev1 "k8s.io/api/node/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	resourcev1 "k8s.io/api/resource/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var (
	// SupportedScheme is a scheme with all supported types registered.
	SupportedScheme *runtime.Scheme
)

func init() {
	schemeAdders := []func(scheme *runtime.Scheme) error{
		metav1.AddMetaToScheme,
		corev1.AddToScheme,
		appsv1.AddToScheme,
		coordinationv1.AddToScheme,
		eventsv1.AddToScheme,
		rbacv1.AddToScheme,
		schedulingv1.AddToScheme,
		policyv1.AddToScheme,
		storagev1.AddToScheme,
		nodev1.AddToScheme,
		resourcev1.AddToScheme,
	}

	SupportedScheme = runtime.NewScheme()
	for _, fn := range schemeAdders {
		utilruntime.Must(fn(SupportedScheme))
	}
}

const (
	// KindNamespace represents a Kubernetes Namespace resource.
	KindNamespace string = "Namespace"
	// KindNamespaceList represents a list of Kubernetes Namespace resources.
	KindNamespaceList string = "NamespaceList"
	// KindServiceAccount represents a Kubernetes ServiceAccount resource.
	KindServiceAccount string = "ServiceAccount"
	// KindServiceAccountList represents a list of Kubernetes ServiceAccount resources.
	KindServiceAccountList string = "ServiceAccountList"
	// KindConfigMap represents a Kubernetes ConfigMap resource.
	KindConfigMap string = "ConfigMap"
	// KindConfigMapList represents a list of Kubernetes ConfigMap resources.
	KindConfigMapList string = "ConfigMapList"
	// KindNode represents a Kubernetes Node resource.
	KindNode string = "Node"
	// KindNodeList represents a list of Kubernetes Node resources.
	KindNodeList string = "NodeList"
	// KindPod represents a Kubernetes Pod resource.
	KindPod string = "Pod"
	// KindPodList represents a list of Kubernetes Pod resources.
	KindPodList string = "PodList"
	// KindService represents a Kubernetes Service resource.
	KindService string = "Service"
	// KindServiceList represents a list of Kubernetes Service resources.
	KindServiceList string = "ServiceList"
	// KindPersistentVolume represents a Kubernetes PersistentVolume resource.
	KindPersistentVolume string = "PersistentVolume"
	// KindPersistentVolumeList represents a list of Kubernetes PersistentVolume resources.
	KindPersistentVolumeList string = "PersistentVolumeList"
	// KindPersistentVolumeClaim represents a Kubernetes PersistentVolumeClaim resource.
	KindPersistentVolumeClaim string = "PersistentVolumeClaim"
	// KindPersistentVolumeClaimList represents a list of Kubernetes PersistentVolumeClaim resources.
	KindPersistentVolumeClaimList string = "PersistentVolumeClaimList"
	// KindReplicationController represents a Kubernetes ReplicationController resource.
	KindReplicationController string = "ReplicationController"
	// KindReplicationControllerList represents a list of Kubernetes ReplicationController resources.
	KindReplicationControllerList string = "ReplicationControllerList"
	// KindPriorityClass represents a Kubernetes PriorityClass resource.
	KindPriorityClass string = "PriorityClass"
	// KindPriorityClassList represents a list of Kubernetes PriorityClass resources.
	KindPriorityClassList string = "PriorityClassList"
	// KindLease represents a Kubernetes Lease resource.
	KindLease string = "Lease"
	// KindLeaseList represents a list of Kubernetes Lease resources.
	KindLeaseList string = "LeaseList"
	// KindEvent represents a Kubernetes Event resource.
	KindEvent string = "Event"
	// KindEventList represents a list of Kubernetes Event resources.
	KindEventList string = "EventList"
	// KindRole represents a Kubernetes Role resource.
	KindRole string = "Role"
	// KindRoleList represents a list of Kubernetes Role resources.
	KindRoleList string = "RoleList"
	// KindDeployment represents a Kubernetes Deployment resource.
	KindDeployment string = "Deployment"
	// KindDeploymentList represents a list of Kubernetes Deployment resources.
	KindDeploymentList string = "DeploymentList"
	// KindReplicaSet represents a Kubernetes ReplicaSet resource.
	KindReplicaSet string = "ReplicaSet"
	// KindReplicaSetList represents a list of Kubernetes ReplicaSet resources.
	KindReplicaSetList string = "ReplicaSetList"
	// KindStatefulSet represents a Kubernetes StatefulSet resource.
	KindStatefulSet string = "StatefulSet"
	// KindStatefulSetList represents a list of Kubernetes StatefulSet resources.
	KindStatefulSetList string = "StatefulSetList"
	// KindPodDisruptionBudget represents a Kubernetes PodDisruptionBudget resource.
	KindPodDisruptionBudget string = "PodDisruptionBudget"
	// KindPodDisruptionBudgetList represents a list of Kubernetes PodDisruptionBudget resources.
	KindPodDisruptionBudgetList string = "PodDisruptionBudgetList"
	// KindStorageClass represents a Kubernetes StorageClass resource.
	KindStorageClass string = "StorageClass"
	// KindStorageClassList represents a list of Kubernetes StorageClass resources.
	KindStorageClassList string = "StorageClassList"
	// KindCSIDriver represents a Kubernetes CSIDriver resource.
	KindCSIDriver string = "CSIDriver"
	// KindCSIDriverList represents a list of Kubernetes CSIDriver resources.
	KindCSIDriverList string = "CSIDriverList"
	// KindCSIStorageCapacity represents a Kubernetes CSIStorageCapacity resource.
	KindCSIStorageCapacity string = "CSIStorageCapacity"
	// KindCSIStorageCapacityList represents a list of Kubernetes CSIStorageCapacity resources.
	KindCSIStorageCapacityList string = "CSIStorageCapacityList"
	// KindCSINode represents a Kubernetes CSINode resource.
	KindCSINode string = "CSINode"
	// KindCSINodeList represents a list of Kubernetes CSINode resources.
	KindCSINodeList string = "CSINodeList"
	// KindVolumeAttachment represents a Kubernetes VolumeAttachment resource.
	KindVolumeAttachment string = "VolumeAttachment"
	// KindVolumeAttachmentList represents a list of Kubernetes VolumeAttachment resources.
	KindVolumeAttachmentList string = "VolumeAttachmentList"
	// KindVolumeAttributesClass represents a Kubernetes VolumeAttributesClass resource.
	KindVolumeAttributesClass string = "VolumeAttributesClass"
	// KindVolumeAttributesClassList represents a list of Kubernetes VolumeAttributesClass resources.
	KindVolumeAttributesClassList string = "VolumeAttributesClassList"
	// KindRuntimeClass represents a Kubernetes RuntimeClass resource.
	KindRuntimeClass string = "RuntimeClass"
	// KindRuntimeClassList represents a list of Kubernetes RuntimeClass resources.
	KindRuntimeClassList string = "RuntimeClassList"
	// KindResourceSlice represents a Kubernetes ResourceSlice resource (introduced as part of DRA).
	KindResourceSlice string = "ResourceSlice"
	// KindResourceSliceList represents a list of Kubernetes ResourceSlice resources (introduced as part of DRA).
	KindResourceSliceList string = "ResourceSliceList"
	// KindResourceClaim represents a Kubernetes ResourceClaim resource (introduced as part of DRA).
	KindResourceClaim string = "ResourceClaim"
	// KindResourceClaimList represents a list of Kubernetes ResourceClaim resources (introduced as part of DRA).
	KindResourceClaimList string = "ResourceClaimList"
	// KindDeviceClass represents a Kubernetes DeviceClass resource (introduced as part of DRA).
	KindDeviceClass string = "DeviceClass"
	// KindDeviceClassList represents a list of Kubernetes DeviceClass resources (introduced as part of DRA).
	KindDeviceClassList string = "DeviceClassList"
)

// Descriptor is an aggregate holder of various bits of type information on a given Kind
type Descriptor struct {
	GVK          schema.GroupVersionKind
	ListGVK      schema.GroupVersionKind
	GVR          schema.GroupVersionResource
	ListTypeMeta metav1.TypeMeta
	APIResource  metav1.APIResource
}

// GetKind returns the Kind of the resource represented by this Descriptor.
func (d Descriptor) GetKind() string {
	return d.GVK.Kind
}

// GetListKind returns the Kind of the list resource represented by this Descriptor.
func (d Descriptor) GetListKind() string {
	return d.ListGVK.Kind
}

// CreateObject creates a new instance of the resource represented by this Descriptor.
func (d Descriptor) CreateObject() (obj metav1.Object, err error) {
	runtimeObj, err := SupportedScheme.New(d.GVK)
	if err != nil {
		return
	}
	obj = runtimeObj.(metav1.Object)
	return
}

// NewDescriptor creates a new Descriptor given the meta information about a resource.
func NewDescriptor(gvk schema.GroupVersionKind, listKind string, namespaced bool, pluralName string, shortNames ...string) Descriptor {
	supportedVerbs := []string{"create", "delete", "deletecollection", "get", "list", "patch", "watch", "update"}
	return Descriptor{
		GVK:     gvk,
		GVR:     gvk.GroupVersion().WithResource(pluralName),
		ListGVK: gvk.GroupVersion().WithKind(string(listKind)),
		ListTypeMeta: metav1.TypeMeta{
			Kind:       string(listKind),
			APIVersion: gvk.GroupVersion().String(),
		},
		APIResource: metav1.APIResource{
			Name:         pluralName,
			SingularName: gvk.Kind,
			Namespaced:   namespaced,
			Group:        gvk.Group,
			Version:      gvk.Version,
			Kind:         gvk.Kind,
			Verbs:        supportedVerbs,
			ShortNames:   shortNames,
			// Categories can be optionally defined to group resources with similar characteristics or purpose.
			// These categories if defined can be used for organizational and filtering purposes. By default all
			// resources belong to `all` category. These are set by the API server for natively undertsood resources and by
			// the CRD author for custom resources. Typically, we do not see this being set to anything but `all`. Thus
			// for now we assume that the only category is `all`.
			Categories:         []string{"all"},
			StorageVersionHash: objutil.GenerateName(gvk.Kind),
		},
	}
}

var (
	// NamespacesDescriptor is an aggregate holder of type information for Namespace resources.
	NamespacesDescriptor = NewDescriptor(corev1.SchemeGroupVersion.WithKind(KindNamespace), KindNamespaceList, false, "namespaces", "ns")
	// ServiceAccountsDescriptor is an aggregate holder of type information for ServiceAccount resources.
	ServiceAccountsDescriptor = NewDescriptor(corev1.SchemeGroupVersion.WithKind(KindServiceAccount), KindServiceAccountList, true, "serviceaccounts", "sa")
	// ConfigMapsDescriptor is an aggregate holder of type information for ConfigMap resources.
	ConfigMapsDescriptor = NewDescriptor(corev1.SchemeGroupVersion.WithKind(KindConfigMap), KindConfigMapList, true, "configmaps", "cm")
	// NodesDescriptor is an aggregate holder of type information for Node resources.
	NodesDescriptor = NewDescriptor(corev1.SchemeGroupVersion.WithKind(KindNode), KindNodeList, false, "nodes", "no")
	// PodsDescriptor is an aggregate holder of type information for Pod resources.
	PodsDescriptor = NewDescriptor(corev1.SchemeGroupVersion.WithKind(KindPod), KindPodList, true, "pods", "po")
	// ServicesDescriptor is an aggregate holder of type information for Service resources.
	ServicesDescriptor = NewDescriptor(corev1.SchemeGroupVersion.WithKind(KindService), KindServiceList, true, "services", "svc")
	// PersistentVolumesDescriptor is an aggregate holder of type information for PersistentVolume resources.
	PersistentVolumesDescriptor = NewDescriptor(corev1.SchemeGroupVersion.WithKind(KindPersistentVolume), KindPersistentVolumeList, false, "persistentvolumes", "pv")
	// PersistentVolumeClaimsDescriptor is an aggregate holder of type information for PersistentVolumeClaim resources.
	PersistentVolumeClaimsDescriptor = NewDescriptor(corev1.SchemeGroupVersion.WithKind(KindPersistentVolumeClaim), KindPersistentVolumeClaimList, true, "persistentvolumeclaims", "pvc")
	// ReplicationControllersDescriptor is an aggregate holder of type information for ReplicationController resources.
	ReplicationControllersDescriptor = NewDescriptor(corev1.SchemeGroupVersion.WithKind(KindReplicationController), KindReplicationControllerList, true, "replicationcontrollers", "rc")
	// PriorityClassesDescriptor is an aggregate holder of type information for PriorityClass resources.
	PriorityClassesDescriptor = NewDescriptor(schedulingv1.SchemeGroupVersion.WithKind(KindPriorityClass), KindPriorityClassList, false, "priorityclasses", "pc")
	// LeaseDescriptor is an aggregate holder of type information for Lease resources.
	LeaseDescriptor = NewDescriptor(coordinationv1.SchemeGroupVersion.WithKind(KindLease), KindLeaseList, true, "leases")
	// EventsDescriptor is an aggregate holder of type information for Event resources.
	EventsDescriptor = NewDescriptor(eventsv1.SchemeGroupVersion.WithKind(KindEvent), KindEventList, true, "events", "ev")
	// RolesDescriptor is an aggregate holder of type information for Role resources.
	RolesDescriptor = NewDescriptor(rbacv1.SchemeGroupVersion.WithKind(KindRole), KindRoleList, true, "roles")
	// DeploymentDescriptor is an aggregate holder of type information for Deployment resources.
	DeploymentDescriptor = NewDescriptor(appsv1.SchemeGroupVersion.WithKind(KindDeployment), KindDeploymentList, true, "deployments", "deploy")
	// ReplicaSetDescriptor is an aggregate holder of type information for ReplicaSet resources.
	ReplicaSetDescriptor = NewDescriptor(appsv1.SchemeGroupVersion.WithKind(KindReplicaSet), KindReplicaSetList, true, "replicasets", "rs")
	// StatefulSetDescriptor is an aggregate holder of type information for StatefulSet resources.
	StatefulSetDescriptor = NewDescriptor(appsv1.SchemeGroupVersion.WithKind(KindStatefulSet), KindStatefulSetList, true, "statefulsets", "sts")
	// PodDisruptionBudgetDescriptor is an aggregate holder of type information for PodDisruptionBudget resources.
	PodDisruptionBudgetDescriptor = NewDescriptor(policyv1.SchemeGroupVersion.WithKind(KindPodDisruptionBudget), KindPodDisruptionBudgetList, true, "poddisruptionbudgets", "pdb")
	// StorageClassDescriptor is an aggregate holder of type information for StorageClass resources.
	StorageClassDescriptor = NewDescriptor(storagev1.SchemeGroupVersion.WithKind(KindStorageClass), KindStorageClassList, false, "storageclasses", "sc")
	// CSIDriverDescriptor is an aggregate holder of type information for CSIDriver resources.
	CSIDriverDescriptor = NewDescriptor(storagev1.SchemeGroupVersion.WithKind(KindCSIDriver), KindCSIDriverList, false, "csidrivers")
	// CSIStorageCapacityDescriptor is an aggregate holder of type information for CSIStorageCapacity resources.
	CSIStorageCapacityDescriptor = NewDescriptor(storagev1.SchemeGroupVersion.WithKind(KindCSIStorageCapacity), KindCSIStorageCapacityList, true, "csistoragecapacities")
	// CSINodeDescriptor is an aggregate holder of type information for CSINode resources.
	CSINodeDescriptor = NewDescriptor(storagev1.SchemeGroupVersion.WithKind(KindCSINode), KindCSINodeList, false, "csinodes")
	// VolumeAttachmentDescriptor is an aggregate holder of type information for VolumeAttachment resources.
	VolumeAttachmentDescriptor = NewDescriptor(storagev1.SchemeGroupVersion.WithKind(KindVolumeAttachment), KindVolumeAttachmentList, false, "volumeattachments")
	// VolumeAttributesClassDescriptor is an aggregate holder of type information for VolumeAttributesClass resources.
	VolumeAttributesClassDescriptor = NewDescriptor(storagev1.SchemeGroupVersion.WithKind(KindVolumeAttributesClass), KindVolumeAttributesClassList, false, "volumeattributesclasses")
	// RuntimeClassDescriptor is an aggregate holder of type information for RuntimeClass resources.
	RuntimeClassDescriptor = NewDescriptor(nodev1.SchemeGroupVersion.WithKind(KindRuntimeClass), KindRuntimeClassList, false, "runtimeclasses")
	// ResourceSliceDescriptor is an aggregate holder of type information for ResourceSlice resources (introduced as part of DRA).
	ResourceSliceDescriptor = NewDescriptor(resourcev1.SchemeGroupVersion.WithKind(KindResourceSlice), KindResourceSliceList, false, "resourceslices")
	// ResourceClaimDescriptor is an aggregate holder of type information for ResourceClaim resources (introduced as part of DRA).
	ResourceClaimDescriptor = NewDescriptor(resourcev1.SchemeGroupVersion.WithKind(KindResourceClaim), KindResourceClaimList, true, "resourceclaims")
	// DeviceClassDescriptor is an aggregate holder of type information for DeviceClass resources (introduced as part of DRA).
	DeviceClassDescriptor = NewDescriptor(resourcev1.SchemeGroupVersion.WithKind(KindDeviceClass), KindDeviceClassList, false, "deviceclasses")

	// SupportedDescriptors is a list of all supported resource descriptors.
	SupportedDescriptors = []Descriptor{
		NamespacesDescriptor,
		ServiceAccountsDescriptor,
		ConfigMapsDescriptor,
		NodesDescriptor,
		PodsDescriptor,
		ServicesDescriptor,
		PersistentVolumesDescriptor,
		PersistentVolumeClaimsDescriptor,
		ReplicationControllersDescriptor,
		PriorityClassesDescriptor,
		LeaseDescriptor,
		EventsDescriptor,
		RolesDescriptor,
		DeploymentDescriptor,
		ReplicaSetDescriptor,
		StatefulSetDescriptor,
		PodDisruptionBudgetDescriptor,
		StorageClassDescriptor,
		CSIDriverDescriptor,
		CSIStorageCapacityDescriptor,
		CSINodeDescriptor,
		VolumeAttachmentDescriptor,
		VolumeAttributesClassDescriptor,
		RuntimeClassDescriptor,
		ResourceSliceDescriptor,
		ResourceClaimDescriptor,
		DeviceClassDescriptor,
	}

	// SupportedAPIVersions is a metav1.APIVersions object representing the supported API versions.
	SupportedAPIVersions = metav1.APIVersions{
		TypeMeta: metav1.TypeMeta{
			Kind: "APIVersions",
		},
		Versions: []string{"v1"},
		ServerAddressByClientCIDRs: []metav1.ServerAddressByClientCIDR{
			{
				ClientCIDR:    "0.0.0.0/0",
				ServerAddress: fmt.Sprintf("127.0.0.1:%d/base", commonconstants.DefaultMinKAPIPort),
			},
		},
	}

	// SupportedAPIGroups is a metav1.APIGroupList object representing the supported API groups.
	SupportedAPIGroups = buildAPIGroupList()

	// SupportedCoreAPIResourceList is a metav1.APIResourceList object representing the supported core API resources.
	SupportedCoreAPIResourceList = metav1.APIResourceList{
		TypeMeta: metav1.TypeMeta{
			Kind: "APIResourceList",
		},
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{
			ServiceAccountsDescriptor.APIResource,
			ConfigMapsDescriptor.APIResource,
			NamespacesDescriptor.APIResource,
			NodesDescriptor.APIResource,
			PodsDescriptor.APIResource,
			ServicesDescriptor.APIResource,
			PersistentVolumesDescriptor.APIResource,
			PersistentVolumeClaimsDescriptor.APIResource,
			ReplicationControllersDescriptor.APIResource,
		},
	}

	// SupportedGroupAPIResourceLists is a list of metav1.APIResourceList objects representing the supported non-core API resources, grouped by their API group.
	SupportedGroupAPIResourceLists = buildNonCoreAPIResourceLists()
)

func buildNonCoreAPIResourceLists() []metav1.APIResourceList {
	metaV1APIResourceList := metav1.TypeMeta{
		Kind:       "APIResourceList",
		APIVersion: "v1",
	}

	return []metav1.APIResourceList{
		{
			TypeMeta:     metaV1APIResourceList,
			GroupVersion: appsv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				DeploymentDescriptor.APIResource,
				ReplicaSetDescriptor.APIResource,
				StatefulSetDescriptor.APIResource,
			},
		},
		{
			TypeMeta:     metaV1APIResourceList,
			GroupVersion: coordinationv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				LeaseDescriptor.APIResource,
			},
		},
		{
			TypeMeta:     metaV1APIResourceList,
			GroupVersion: eventsv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				EventsDescriptor.APIResource,
			},
		},
		{
			TypeMeta:     metaV1APIResourceList,
			GroupVersion: rbacv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				RolesDescriptor.APIResource,
			},
		},
		{
			TypeMeta:     metaV1APIResourceList,
			GroupVersion: schedulingv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				PriorityClassesDescriptor.APIResource,
			},
		},
		{
			TypeMeta:     metaV1APIResourceList,
			GroupVersion: policyv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				PodDisruptionBudgetDescriptor.APIResource,
			},
		},
		{
			TypeMeta:     metaV1APIResourceList,
			GroupVersion: storagev1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				StorageClassDescriptor.APIResource,
				CSIDriverDescriptor.APIResource,
				CSIStorageCapacityDescriptor.APIResource,
				CSINodeDescriptor.APIResource,
				VolumeAttachmentDescriptor.APIResource,
				VolumeAttributesClassDescriptor.APIResource,
			},
		},
		{
			TypeMeta:     metaV1APIResourceList,
			GroupVersion: nodev1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				RuntimeClassDescriptor.APIResource,
			},
		},
		{
			TypeMeta:     metaV1APIResourceList,
			GroupVersion: resourcev1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				ResourceSliceDescriptor.APIResource,
				ResourceClaimDescriptor.APIResource,
				DeviceClassDescriptor.APIResource,
			},
		},
	}
}

func buildAPIGroupList() metav1.APIGroupList {
	var groups = make(map[string]metav1.APIGroup)
	for _, d := range SupportedDescriptors {
		if d.GVK.Group == "" {
			//  don't add default group otherwise kubectl will  give errors like the below
			// error: /, Kind=Pod matches multiple kinds [/v1, Kind=Pod /v1, Kind=Pod]
			// OH-MY-GAWD, it took me FOREVER to find this.
			continue
		}
		groups[d.GVR.Group] = metav1.APIGroup{
			Name: d.GVR.Group,
			Versions: []metav1.GroupVersionForDiscovery{
				{
					GroupVersion: d.GVK.GroupVersion().String(),
					Version:      d.GVK.Version,
				},
			},
			PreferredVersion: metav1.GroupVersionForDiscovery{
				GroupVersion: d.GVK.GroupVersion().String(),
				Version:      d.GVK.Version,
			},
		}
	}
	return metav1.APIGroupList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "APIGroupList",
			APIVersion: "v1",
		},
		Groups: slices.Collect(maps.Values(groups)),
	}
}
