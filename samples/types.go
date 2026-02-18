// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PoolPreset is the enum type for representing pool presets of a sample scaling constraint.
type PoolPreset string

const (
	// PoolPreset1P is the pool category variant associated with a basic one-pool with one zone scaling constraint.
	PoolPreset1P PoolPreset = "1p"

	// PoolPreset2P is the pool category variant associated with a basic two-pool scaling constraint.
	PoolPreset2P PoolPreset = "2p"
)

// ResourcePreset is the enum type for different presets of resources.
type ResourcePreset string

const (
	// ResourcePresetPea is a preset for a resource list that specifies  1cpu and 1Gi.
	ResourcePresetPea ResourcePreset = "pea"

	// ResourcePresetBerry is a preset for a resource list that nearly fit an AWS m5.large instance type / GCP n2-standard-2 / Azure Standard_D2
	// leaving buffer to account for provider variance and kube and system reserved.
	ResourcePresetBerry ResourcePreset = "berry"

	// ResourcePresetHalfBerry is a preset for a resource list that when doubled nearly fit an AWS m5.large instance type / GCP n2-standard-2 / Azure Standard_D2
	// leaving buffer to account for provider variance and kube and system reserved.
	ResourcePresetHalfBerry ResourcePreset = "half-berry"

	// ResourcePresetGrape is a preset for a resource list that when doubled nearly fits an AWS m5.xlarge / GCP n2-standard-4 / Azure Standard_D3
	// leaving buffer to account for provider variance and kube and system reserved.
	ResourcePresetGrape ResourcePreset = "grape"

	// ResourcePresetHalfGrape is a preset for a resource list that when doubled nearly fits an AWS m5.xlarge / GCP n2-standard-4 / Azure Standard_D3
	// leaving buffer to account for provider variance and kube and system reserved.
	ResourcePresetHalfGrape ResourcePreset = "half-grape"
)

// AsResourceList creates a corev1.ResourceList for the resources associated with this name
func (c ResourcePreset) AsResourceList() corev1.ResourceList {
	return resourcePresetsToResourceListMap[c]
}

var ()

// AppLabels represents standard k8s app labels
type AppLabels struct {
	Name      string
	Instance  string
	Version   string
	Component string
	PartOf    string
	ManagedBy string
}

// SimplePodGenInput holds the input data for generating simple pods.
type SimplePodGenInput struct {
	GenDir        string
	Name          string
	Namespace     string
	AppLabels     AppLabels
	SchedulerName string
	// PVCNames is the names of the PersistentVolumeClaims to be mounted to the pod.
	PVCNames []string
}

type VolCommon struct {
	GenDir           string
	Namespace        string
	Storage          resource.Quantity
	AccessMode       corev1.PersistentVolumeAccessMode
	StorageClassName string
	Unbound          bool
}
type SimplePVGenInput struct {
	VolCommon
	Provider commontypes.CloudProvider
	Zone     string
	PVCNames []string
}

type SimplePVCGenInput struct {
	VolCommon
	Names []string
}

// SimplePodTemplateData holds all the pod template data for the simple pod template.
type SimplePodTemplateData struct {
	//Resources map[corev1.ResourceName]string
	Resources corev1.ResourceList
	SimplePodGenInput
}

// CSIDefaults encapsulate a collection of default CSI config values
type CSIDefaults struct {
	DriverName   string
	TopologyKeys []string
}
