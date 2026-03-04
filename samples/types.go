// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	"fmt"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
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

// AppLabels represents standard k8s app labels
type AppLabels struct {
	Name      string
	Instance  string
	Version   string
	Component string
	PartOf    string
	ManagedBy string
}

// ConstraintGenInput holds the input data for generating ScalingConstraint's. Currently, it only supports customization of
// NodePool details, but will be extended in the future.
type ConstraintGenInput struct {
	GenDir string
	// PoolPreset is the PoolPreset variant
	PoolPreset PoolPreset
	// PoolZones specifies the availability zones for each pool given in order. If nil, defaults to the preset zone.
	PoolZones [][]string
}

// ConstraintGenOutput holds the generated output data after generating NodePools.
type ConstraintGenOutput struct {
	// GenYAMLPath provides the generated YAML path of ScalingConstraint
	GenYAMLPath string
	Constraint  sacorev1alpha1.ScalingConstraint
}

// PodGenInput holds the input data for generating simple pods.
type PodGenInput struct {
	GenDir        string
	Name          string
	Namespace     string
	AppLabels     AppLabels
	SchedulerName string
	// PVCNames is the names of the PersistentVolumeClaims to be mounted to the pod.
	PVCNames []string
}

// PodGenOutput is the output container for generated pod data.
type PodGenOutput struct {
	YAMLPaths map[commontypes.NamespacedName]string
	Pods      []corev1.Pod
}

// VolGenInput represents bag of input parameters to generate PVC's and PV's.
type VolGenInput struct {
	Provider commontypes.CloudProvider
	// Namespace represents the PVC nameespace.
	Namespace string
	// Storage is used to set the corev1.ResourceStorage quantity in generated PVC.Spec.Resoures.Requests
	Storage resource.Quantity
	// AccessMode is used to set the generated PVC.Spec.AccessModes and PV.Spec.AccessModes
	AccessMode corev1.PersistentVolumeAccessMode
	// StorageClassName is used to set the generated PVC.spec.storageClassName and PV.spec.storageClassName
	StorageClassName string
	// PVCCLaimPhase represents whether PVC is bound or unbound to a PV. If PVC Phase is "Pending" (default),
	// then the generated PVC is not bound to the generated PV.
	ClaimPhase corev1.PersistentVolumeClaimPhase
	// VolumeBindingMode represents whether the PVC should be bound Immediately or WaitForFirstConsumer (WFFC)
	// Always defaults to Immediate, unless explicitly set.
	// If not explicitly set and if the ClaimPhase is Pending, then VolumeBindingMode is defaulted to Immediate.
	VolumeBindingMode storagev1.VolumeBindingMode
	// PVCNames if specified determine the number of PVCs and names of the generated PVCs.
	PVCNames []string
	// PVZones if specified determine the total number of PV's - there is a PV generated per PVCName and PVZone combo.
	// The zone is used as the match expression in the PersistentVolume.Spec.NodeAffinity for the generated PV.
	PVZones []string
	// MaxAllocatableVolumes specifies the max number of PV's that can be allocated to the Node. It is a CSI specific value.
	MaxAllocatableVolumes int32
	// GeneratePV should be set to true if PV objects should be generated or skipped.
	GeneratePV bool
}

// ValidateAndFillDefaults validates required fields in VolGenInput and fills other fields with defaults.
func (v *VolGenInput) ValidateAndFillDefaults() error {
	if len(v.PVCNames) == 0 {
		return fmt.Errorf("empty PVCNames")
	}
	if v.Provider == "" {
		return fmt.Errorf("provider must be set")
	}
	if v.Namespace == "" {
		v.Namespace = metav1.NamespaceDefault
	}
	if v.ClaimPhase == "" {
		return fmt.Errorf("ClaimPhase must be set")
	}
	if v.VolumeBindingMode == "" {
		v.VolumeBindingMode = storagev1.VolumeBindingImmediate
	}
	if v.StorageClassName == "" {
		v.StorageClassName = "default"
	}
	if v.AccessMode == "" {
		v.AccessMode = corev1.ReadWriteOnce
	}
	if v.StorageClassName == "" {
		v.StorageClassName = "default"
	}
	if v.Storage.IsZero() {
		v.Storage = resource.MustParse("1Gi")
	}
	return nil
}

// VolGenOutput is the output container for generated volume data.
type VolGenOutput struct {
	YAMLPaths map[commontypes.NamespacedName]string
	PVs       []corev1.PersistentVolume
	PVCs      []corev1.PersistentVolumeClaim
}

// PodTemplateData holds all pod template data values for executing the simple pod template.
type PodTemplateData struct {
	//Resources map[corev1.ResourceName]string
	Resources corev1.ResourceList
	PodGenInput
}

// CSIDefaults encapsulate a collection of default CSI config values
type CSIDefaults struct {
	DriverName   string
	TopologyKeys []string
}
