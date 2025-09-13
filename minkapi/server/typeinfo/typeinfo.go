// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package typeinfo

import (
	"fmt"
	"github.com/gardener/scaling-advisor/common/objutil"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/validation"
	"maps"
	"slices"
	"strings"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type KindName string

const (
	NamespaceKind     KindName = "Namespace"
	NamespaceListKind KindName = "NamespaceList"

	ServiceAccountKind     KindName = "ServiceAccount"
	ServiceAccountListKind KindName = "ServiceAccountList"

	ConfigMapKind     KindName = "ConfigMap"
	ConfigMapListKind KindName = "ConfigMapList"

	NodeKind     KindName = "Node"
	NodeListKind KindName = "NodeList"

	PodKind     KindName = "Pod"
	PodListKind KindName = "PodList"

	ServiceKind     KindName = "Service"
	ServiceListKind KindName = "ServiceList"

	PersistentVolumeKind          KindName = "PersistentVolume"
	PersistentVolumeListKind      KindName = "PersistentVolumeList"
	PersistentVolumeClaimKind     KindName = "PersistentVolumeClaim"
	PersistentVolumeClaimListKind KindName = "PersistentVolumeClaimList"

	ReplicationControllerKind     KindName = "ReplicationController"
	ReplicationControllerListKind KindName = "ReplicationControllerList"

	PriorityClassKind     KindName = "PriorityClass"
	PriorityClassListKind KindName = "PriorityClassList"

	LeaseKind     KindName = "Lease"
	LeaseListKind KindName = "LeaseList"

	EventKind     KindName = "Event"
	EventListKind KindName = "EventList"

	RoleKind     KindName = "Role"
	RoleListKind KindName = "RoleList"

	RoleBindingKind     KindName = "RoleBinding"
	RoleBindingListKind KindName = "RoleBindingList"

	DeploymentKind     KindName = "Deployment"
	DeploymentListKind KindName = "DeploymentList"

	ReplicaSetKind     KindName = "ReplicaSet"
	ReplicaSetListKind KindName = "ReplicaSetList"

	StatefulSetKind     KindName = "StatefulSet"
	StatefulSetListKind KindName = "StatefulSetList"

	PodDisruptionBudgetKind     KindName = "PodDisruptionBudget"
	PodDisruptionBudgetListKind KindName = "PodDisruptionBudgetList"

	StorageClassKind     KindName = "StorageClass"
	StorageClassListKind KindName = "StorageClassList"

	CSIDriverKind     KindName = "CSIDriver"
	CSIDriverListKind KindName = "CSIDriverList"

	CSIStorageCapacityKind     KindName = "CSIStorageCapacity"
	CSIStorageCapacityListKind KindName = "CSIStorageCapacityList"

	CSINodeKind     KindName = "CSINode"
	CSINodeListKind KindName = "CSINodeList"

	VolumeAttachmentKind     KindName = "VolumeAttachment"
	VolumeAttachmentListKind KindName = "VolumeAttachmentList"
)

// Descriptor is an aggregate holder of various bits of type information on a given Kind
type Descriptor struct {
	Kind         KindName
	GVK          schema.GroupVersionKind
	ListKind     KindName
	ListGVK      schema.GroupVersionKind
	GVR          schema.GroupVersionResource
	ListTypeMeta metav1.TypeMeta
	APIResource  metav1.APIResource
	//ObjTemplate     runtime.Object
	//ObjListTemplate runtime.Object
	//ObjType         reflect.Type
	//ObjListType     reflect.Type
	//ItemsSliceType  reflect.Type
}

var (
	SupportedScheme      = RegisterSchemes()
	NamespacesDescriptor = NewDescriptor(NamespaceKind, NamespaceListKind, false, corev1.SchemeGroupVersion.WithResource("namespaces"), "ns")

	ServiceAccountsDescriptor = NewDescriptor(ServiceAccountKind, ServiceAccountListKind, true, corev1.SchemeGroupVersion.WithResource("serviceaccounts"), "sa")

	ConfigMapsDescriptor = NewDescriptor(ConfigMapKind, ConfigMapListKind, true, corev1.SchemeGroupVersion.WithResource("configmaps"), "cm")
	NodesDescriptor      = NewDescriptor(NodeKind, NodeListKind, false, corev1.SchemeGroupVersion.WithResource("nodes"), "no")
	PodsDescriptor       = NewDescriptor(PodKind, PodListKind, true, corev1.SchemeGroupVersion.WithResource("pods"), "po")

	ServicesDescriptor          = NewDescriptor(ServiceKind, ServiceListKind, true, corev1.SchemeGroupVersion.WithResource("services"), "svc")
	PersistentVolumesDescriptor = NewDescriptor(PersistentVolumeKind, PersistentVolumeListKind, false, corev1.SchemeGroupVersion.WithResource("persistentvolumes"), "pv")

	PersistentVolumeClaimsDescriptor = NewDescriptor(PersistentVolumeClaimKind, PersistentVolumeClaimListKind, true, corev1.SchemeGroupVersion.WithResource("persistentvolumeclaims"), "pvc")

	ReplicationControllersDescriptor = NewDescriptor(ReplicationControllerKind, ReplicationControllerListKind, true, corev1.SchemeGroupVersion.WithResource("replicationcontrollers"), "rc")
	PriorityClassesDescriptor        = NewDescriptor(PriorityClassKind, PriorityClassListKind, false, schedulingv1.SchemeGroupVersion.WithResource("priorityclasses"), "pc")

	LeaseDescriptor = NewDescriptor(LeaseKind, LeaseListKind, true, coordinationv1.SchemeGroupVersion.WithResource("leases"))

	EventsDescriptor              = NewDescriptor(EventKind, EventListKind, true, eventsv1.SchemeGroupVersion.WithResource("events"), "ev")
	RolesDescriptor               = NewDescriptor(RoleKind, RoleListKind, true, rbacv1.SchemeGroupVersion.WithResource("roles"))
	DeploymentDescriptor          = NewDescriptor(DeploymentKind, DeploymentListKind, true, appsv1.SchemeGroupVersion.WithResource("deployments"), "deploy")
	ReplicaSetDescriptor          = NewDescriptor(ReplicaSetKind, ReplicaSetListKind, true, appsv1.SchemeGroupVersion.WithResource("replicasets"), "rs")
	StatefulSetDescriptor         = NewDescriptor(StatefulSetKind, StatefulSetListKind, true, appsv1.SchemeGroupVersion.WithResource("statefulsets"), "sts")
	PodDisruptionBudgetDescriptor = NewDescriptor(PodDisruptionBudgetKind, PodDisruptionBudgetListKind, true, policyv1.SchemeGroupVersion.WithResource("poddisruptionbudgets"), "pdb")

	StorageClassDescriptor = NewDescriptor(StorageClassKind, StorageClassListKind, false, storagev1.SchemeGroupVersion.WithResource("storageclasses"), "sc")

	CSIDriverDescriptor          = NewDescriptor(CSIDriverKind, CSIDriverListKind, false, storagev1.SchemeGroupVersion.WithResource("csidrivers"))
	CSIStorageCapacityDescriptor = NewDescriptor(CSIStorageCapacityKind, CSIStorageCapacityListKind, true, storagev1.SchemeGroupVersion.WithResource("csistoragecapacities"))

	CSINodeDescriptor = NewDescriptor(CSINodeKind, CSINodeListKind, false, storagev1.SchemeGroupVersion.WithResource("csinodes"))

	VolumeAttachmentDescriptor = NewDescriptor(VolumeAttachmentKind, VolumeAttachmentListKind, false, storagev1.SchemeGroupVersion.WithResource("volumeattachments"))

	SupportedDescriptors = []Descriptor{
		ServiceAccountsDescriptor, ConfigMapsDescriptor, NamespacesDescriptor, NodesDescriptor, PodsDescriptor, ServicesDescriptor, PersistentVolumesDescriptor, PersistentVolumeClaimsDescriptor, ReplicationControllersDescriptor,
		PriorityClassesDescriptor,
		LeaseDescriptor,
		EventsDescriptor,
		RolesDescriptor,
		DeploymentDescriptor, ReplicaSetDescriptor, StatefulSetDescriptor,
		PodDisruptionBudgetDescriptor,
		StorageClassDescriptor, CSIDriverDescriptor, CSIStorageCapacityDescriptor, CSINodeDescriptor, VolumeAttachmentDescriptor,
	}

	SupportedVerbs = []string{"create", "delete", "get", "list", "patch", "watch"}

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

	SupportedAPIGroups = buildAPIGroupList()

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

	SupportedGroupAPIResourceLists = []metav1.APIResourceList{
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
			},
		},
	}
)

var (
	schemeAdders = []func(scheme *runtime.Scheme) error{
		metav1.AddMetaToScheme,
		corev1.AddToScheme,
		appsv1.AddToScheme,
		coordinationv1.AddToScheme,
		eventsv1.AddToScheme,
		rbacv1.AddToScheme,
		schedulingv1.AddToScheme,
		policyv1.AddToScheme,
		storagev1.AddToScheme,
	}

	metaV1APIResourceList = metav1.TypeMeta{
		Kind:       "APIResourceList",
		APIVersion: "v1",
	}
)

func RegisterSchemes() (scheme *runtime.Scheme) {
	scheme = runtime.NewScheme()
	for _, fn := range schemeAdders {
		utilruntime.Must(fn(scheme))
	}
	return
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

func NewDescriptor(kind KindName, listKind KindName, namespaced bool, gvr schema.GroupVersionResource, shortNames ...string) Descriptor {
	var singularName string

	// please pardon the hack below
	if strings.HasSuffix(gvr.Resource, "sses") { // Ex: priorityclasses
		singularName = strings.TrimSuffix(gvr.Resource, "es")
	} else if strings.HasSuffix(gvr.Resource, "ties") { // Ex: csistoragecapacities
		singularName = strings.TrimSuffix(gvr.Resource, "ties") + "ty"
	} else {
		singularName = strings.TrimSuffix(gvr.Resource, "s")
	}
	return Descriptor{
		Kind: kind,
		GVK: schema.GroupVersionKind{
			Group:   gvr.Group,
			Version: gvr.Version,
			Kind:    string(kind),
		},
		ListKind: listKind,
		ListGVK: schema.GroupVersionKind{
			Group:   gvr.Group,
			Version: gvr.Version,
			Kind:    string(listKind),
		},
		GVR: gvr,
		ListTypeMeta: metav1.TypeMeta{
			Kind:       string(listKind),
			APIVersion: gvr.GroupVersion().String(),
		},
		APIResource: metav1.APIResource{
			Name:               gvr.Resource,
			SingularName:       singularName,
			Namespaced:         namespaced,
			Group:              gvr.Group,
			Version:            gvr.Version,
			Kind:               string(kind),
			Verbs:              SupportedVerbs,
			ShortNames:         shortNames,
			Categories:         []string{"all"}, // TODO: Uhhh, WTH is this exactly ? Who uses this ?
			StorageVersionHash: objutil.GenerateName(singularName),
		},
	}
}

func (d Descriptor) CreateObject() (obj metav1.Object, err error) {
	runtimeObj, err := SupportedScheme.New(d.GVK)
	if err != nil {
		return
	}
	obj = runtimeObj.(metav1.Object)
	return
}

func (d Descriptor) Resource() string {
	return d.GVR.Resource
}

func GenerateName(base string) string {
	const suffixLen = 5
	suffix := utilrand.String(suffixLen)
	m := validation.DNS1123SubdomainMaxLength // 253 for subdomains; use DNS1123LabelMaxLength (63) if you need stricter
	if len(base)+len(suffix) > m {
		base = base[:m-len(suffix)]
	}
	return base + suffix
}
