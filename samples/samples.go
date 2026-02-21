// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"os"
	"path"
	"strconv"
	"strings"
	"text/template"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// LoadBasicScalingConstraints loads the basic scaling constraints for the given poolCategory from the sample data filesystem.
func LoadBasicScalingConstraints(poolCategory PoolCategory) (*sacorev1alpha1.ScalingConstraint, error) {
	var clusterConstraints sacorev1alpha1.ScalingConstraint
	clusterConstraintsPath := fmt.Sprintf("data/scaling-constraints/%s.json", poolCategory)
	switch poolCategory {
	case PoolCategoryBasicOne, PoolCategoryBasicTwo:
		if err := objutil.LoadIntoRuntimeObj(dataFS, clusterConstraintsPath, &clusterConstraints); err != nil {
			return nil, fmt.Errorf("failed to load scaling constraints for poolCategory %q: %v", poolCategory, err)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported %q", commonerrors.ErrUnimplemented, poolCategory)
	}
	return &clusterConstraints, nil
}

// IncreaseUnscheduledWorkLoad replicates each unscheduled pod by delta inside the given cluster snapshot
func IncreaseUnscheduledWorkLoad(snapshot *planner.ClusterSnapshot, amount int) error {
	var extra []planner.PodInfo
	for _, upod := range snapshot.GetUnscheduledPods() {
		lastCharOfName := upod.Name[len(upod.Name)-1:]
		endsWithDigit := strings.ContainsAny(lastCharOfName, "0123456789")
		existingBase := 0
		prefix := upod.Name
		if endsWithDigit {
			existingBase, _ = strconv.Atoi(lastCharOfName)
			prefix = upod.Name[0 : len(upod.Name)-1]
		}
		for i := 1; i <= amount; i++ {
			p := upod
			if endsWithDigit {
				p.Name = prefix + strconv.Itoa(existingBase+i)
			} else {
				p.Name = prefix + "-" + strconv.Itoa(existingBase+i)
			}
			p.UID = types.UID(fmt.Sprintf("%s-%d", p.UID, i))
			extra = append(extra, p)
		}
	}
	snapshot.Pods = append(snapshot.Pods, extra...)
	return nil
}

// LoadBinPackingSchedulerConfig loads the kube-scheduler configuration from the sample data filesystem.
func LoadBinPackingSchedulerConfig() ([]byte, error) {
	return dataFS.ReadFile("data/bin-packing-scheduler-config.yaml")
}

// GenerateSimplePodsWithTemplateData generates a slice of corev1.Pod objects with count length using the given pod template data in podTmplData.
// Also generates the pod YAMLs for these pods within the temp directory.
func GenerateSimplePodsWithTemplateData(num int, podTmplData PodTemplateData) (pods []corev1.Pod, podYAMLPaths []string, err error) {
	tmpl, err := ioutil.LoadEmbeddedTextTemplate(dataFS, simplePodTemplatePath)
	if err != nil {
		return
	}
	if podTmplData.GenDir == "" {
		err = fmt.Errorf("PodTemplateData.GenDir is empty")
		return
	}
	for i := 1; i <= num; i++ {
		var pod corev1.Pod
		tmplData := fillPodTemplateDataDefaults(podTmplData)
		tmplData.Name = tmplData.Name + "-" + strconv.Itoa(i)
		outYAMLPath := path.Join(podTmplData.GenDir, "pod-"+tmplData.Name+".yaml")
		err = GenerateAndLoad(tmpl, tmplData, outYAMLPath, &pod)
		if err != nil {
			return
		}
		pod.CreationTimestamp = metav1.Now()
		pods = append(pods, pod)
		podYAMLPaths = append(podYAMLPaths, outYAMLPath)
	}
	return
}

// GenerateSimplePodsForResourceCategory generates simple pods with a container specifying requests for the given resourceCategory and using the given metadata.
// Also generates the pod YAML's for these pods within the temp directory.
func GenerateSimplePodsForResourceCategory(category ResourcePreset, num int, metadata PodGenInput) (pods []corev1.Pod, podYAMLPaths []string, err error) {
	podTmplData := PodTemplateData{
		PodGenInput: metadata,
		Resources:   category.AsResourceList(),
	}
	return GenerateSimplePodsWithTemplateData(num, podTmplData)
}

// GeneratePersistentVolumeClaims generates a slice of corev1.PersistentVolumeClaim objects with the given pvcNames,  storage and accessMode in the given namespace.
func GeneratePersistentVolumeClaims(vi StorageVolGenInput) (pvcs []corev1.PersistentVolumeClaim, pvcYAMLPaths []string, err error) {
	tmpl, err := ioutil.LoadEmbeddedTextTemplate(dataFS, "data/simple-pvc-template.yaml")
	if err != nil {
		return
	}
	if err = vi.ValidateAndFillDefaults(); err != nil {
		return
	}
	for _, pvcName := range vi.PVCNames {
		var (
			pvc         corev1.PersistentVolumeClaim
			claimPhase  = corev1.ClaimBound
			volumeName  = "pv-" + pvcName
			outYAMLPath = path.Join(vi.GenDir, "pvc-"+pvcName+".yaml")
		)
		if vi.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
			volumeName = ""
			claimPhase = corev1.ClaimPending
		} else if len(vi.PVZones) >= 0 {
			volumeName = "pv-" + pvcName + "-0" // always bind to first PV if there is more than one PV
		}
		pvcTmplData := struct {
			Name             string
			Namespace        string
			AccessMode       string
			Storage          string
			Phase            string
			StorageClassName string
			VolumeName       string
			UID              string
		}{
			Name:             pvcName,
			Namespace:        vi.Namespace,
			Storage:          vi.Storage.String(),
			AccessMode:       string(vi.AccessMode),
			Phase:            string(claimPhase),
			StorageClassName: vi.StorageClassName,
			VolumeName:       volumeName,
			UID:              pvcName,
		}
		err = GenerateAndLoad(tmpl, pvcTmplData, outYAMLPath, &pvc)
		if err != nil {
			return
		}
		pvc.CreationTimestamp = metav1.Now()
		pvcs = append(pvcs, pvc)
		pvcYAMLPaths = append(pvcYAMLPaths, outYAMLPath)
	}
	return
}

// GeneratePersistentVolumes generates a slice of PersistentVolume objects bound to the given pvcNames suitable for the given provider in the given
// namespace for the given storage and access mode and returns the PV objects and their generated YAML paths.
func GeneratePersistentVolumes(vi StorageVolGenInput) (pvs []corev1.PersistentVolume, pvYAMLPaths []string, err error) {
	// provider commontypes.CloudProvider, namespace string, storage resource.Quantity,
	//	accessMode corev1.PersistentVolumeAccessMode, zone string, pvcNames []string)
	tmpl, err := ioutil.LoadEmbeddedTextTemplate(dataFS, "data/simple-pv-template.yaml")
	if err != nil {
		return
	}
	if err = vi.ValidateAndFillDefaults(); err != nil {
		return
	}
	if len(vi.PVZones) == 0 {
		err = errors.New("empty PVZones")
		return
	}
	csiDefaults, err := GetCSIDefaults(vi.Provider)
	if err != nil {
		return
	}
	for _, pvcName := range vi.PVCNames {
		var (
			pv          corev1.PersistentVolume
			volumePhase = corev1.VolumeBound
			pvName      string
			yamlPath    string
		)
		if vi.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
			volumePhase = corev1.VolumeAvailable
		}
		for i, zone := range vi.PVZones {
			pvName = fmt.Sprintf("pv-%s-%d", pvcName, i)
			yamlPath = path.Join(vi.GenDir, pvName+".yaml")
			pvTmplData := struct {
				CSIDriver        string
				Name             string
				Namespace        string
				Storage          string
				AccessMode       string
				VolumeHandle     string
				PVCName          string
				Zone             string
				Phase            string
				StorageClassName string
				PVCUID           string
			}{
				CSIDriver:        csiDefaults.DriverName,
				Name:             pvName,
				Namespace:        vi.Namespace,
				Storage:          vi.Storage.String(),
				AccessMode:       string(vi.AccessMode),
				VolumeHandle:     pvName,
				PVCName:          pvcName,
				Zone:             zone,
				Phase:            string(volumePhase),
				StorageClassName: vi.StorageClassName,
				PVCUID:           pvcName,
			}
			err = GenerateAndLoad(tmpl, pvTmplData, yamlPath, &pv)
			if err != nil {
				return
			}
			pv.CreationTimestamp = metav1.Now()
			pvs = append(pvs, pv)
			pvYAMLPaths = append(pvYAMLPaths, yamlPath)
		}
	}
	return
}

// GetCSINodeDrivers gets a slice of sample CSINodeDrivers for the given provider using hard-coded defaults.
func GetCSINodeDrivers(provider commontypes.CloudProvider, maxAllocatableVolumes int32) ([]storagev1.CSINodeDriver, error) {
	csiDefaults, err := GetCSIDefaults(provider)
	if err != nil {
		return nil, err
	}
	return []storagev1.CSINodeDriver{
		{
			Name:         csiDefaults.DriverName,
			TopologyKeys: csiDefaults.TopologyKeys,
			Allocatable: &storagev1.VolumeNodeResources{
				Count: &maxAllocatableVolumes,
			},
		},
	}, nil
}

func GenerateStorageClass(genDir string, provider commontypes.CloudProvider, name string, volumeBindingMode storagev1.VolumeBindingMode) (storageClass storagev1.StorageClass, outYAMLPath string, err error) {
	tmpl, err := ioutil.LoadEmbeddedTextTemplate(dataFS, "data/sc-template.yaml")
	if err != nil {
		return
	}
	outYAMLPath = path.Join(genDir, "sc-"+name+".yaml")
	csiDefaults, err := GetCSIDefaults(provider)
	if err != nil {
		return
	}
	if name == "" {
		err = fmt.Errorf("must specify non-empty name for StorageClass %s", provider)
		return
	}
	scTmplData := struct {
		CSIDriver         string
		Name              string
		VolumeBindingMode string
	}{
		CSIDriver:         csiDefaults.DriverName,
		Name:              name,
		VolumeBindingMode: string(volumeBindingMode),
	}
	if err = GenerateAndLoad(tmpl, scTmplData, outYAMLPath, &storageClass); err != nil {
		return
	}
	storageClass.CreationTimestamp = metav1.Now()
	return
}

// GenerateAndLoad executes the given template with the given params, writes the generated output to outPath and loads the same as a runtime object
func GenerateAndLoad[P any, O runtime.Object](tmpl *template.Template, params P, outPath string, obj O) error {
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, params)
	if err != nil {
		return fmt.Errorf("%w: execution of %q template failed with params %v: %w", commonerrors.ErrExecuteTemplate, tmpl.Name(), params, err)
	}
	err = os.WriteFile(outPath, buf.Bytes(), 0600)
	if err != nil {
		return fmt.Errorf("%w: failed to write output of %q template with params %v to path %q: %w", commonerrors.ErrExecuteTemplate, tmpl.Name(), params, outPath, err)
	}
	root, err := os.OpenRoot("/")
	if err != nil {
		return fmt.Errorf("%w: failed to open root FS: %w", commonerrors.ErrExecuteTemplate, err)
	}
	return objutil.LoadIntoRuntimeObj(root.FS(), strings.TrimPrefix(outPath, "/"), obj)
}

// GetMaxAllocatableVolumes returns a reasonable default value for the max number of allocatable volumes for a node of the given instanceType.
func GetMaxAllocatableVolumes(provider commontypes.CloudProvider, instanceType string) int32 {
	maxAllocatable, ok := maxAllocatableVolumesByInstanceType[instanceType]
	if ok {
		return maxAllocatable
	}
	switch provider {
	case commontypes.CloudProviderAWS:
		maxAllocatable = maxAllocatableVolumesByInstanceType["m5.large"]
	default:
		maxAllocatable = 16 // safe, conservative fallback
	}
	return maxAllocatable
}

// GetCSIDefaults returns an instance of CSIDefaults populated with reasonable values for the given provider.
func GetCSIDefaults(provider commontypes.CloudProvider) (defaults CSIDefaults, err error) {
	defaults, ok := providerToCSIDefaults[provider]
	if !ok {
		err = fmt.Errorf("no CSIDefaults found for provider %s", provider)
	}
	return
}

func fillPodTemplateDataDefaults(podTmplData PodTemplateData) PodTemplateData {
	podTmplData.AppLabels = fillAppLabelDefaults(podTmplData.AppLabels)
	if podTmplData.Namespace == "" {
		podTmplData.Namespace = metav1.NamespaceDefault
	}
	if podTmplData.Name == "" {
		podTmplData.Name = podTmplData.AppLabels.Name
	}
	if len(podTmplData.Resources) == 0 {
		podTmplData.Resources = ResourcePresetPea.AsResourceList()
	}
	return podTmplData
}

func fillAppLabelDefaults(appLabels AppLabels) AppLabels {
	if appLabels.Name == "" {
		appLabels.Name = "test"
	}
	if appLabels.Instance == "" {
		appLabels.Instance = appLabels.Name + "-instance"
	}
	if appLabels.Component == "" {
		appLabels.Component = appLabels.Name + "-component"
	}
	if appLabels.Version == "" {
		appLabels.Version = "1.0.0"
	}
	if appLabels.PartOf == "" {
		appLabels.PartOf = appLabels.Name + "-system"
	}
	if appLabels.ManagedBy == "" {
		appLabels.ManagedBy = "scaling-advisor"
	}
	return appLabels
}

var (
	//go:embed data
	dataFS embed.FS

	simplePodTemplatePath = "data/simple-pod-template.yaml"

	providerToCSIDefaults = map[commontypes.CloudProvider]CSIDefaults{
		commontypes.CloudProviderAWS: {
			DriverName: "ebs.csi.aws.com",
			TopologyKeys: []string{
				"topology.ebs.csi.aws.com/zone",
				corev1.LabelTopologyZone,
				corev1.LabelOSStable,
			},
		},
	}

	maxAllocatableVolumesByInstanceType = map[string]int32{
		"m5.large":   26,
		"c3.8xlarge": 38,
	}
	allResourcePresets = []ResourcePreset{
		ResourcePresetPea, ResourcePresetBerry, ResourcePresetHalfBerry, ResourcePresetGrape, ResourcePresetHalfGrape,
	}
	resourcePresetsToResourceListMap = map[ResourcePreset]corev1.ResourceList{
		ResourcePresetPea: {
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		ResourcePresetBerry: {
			corev1.ResourceCPU:    resource.MustParse("1000m"),
			corev1.ResourceMemory: resource.MustParse("5100Mi"),
		},
		ResourcePresetHalfBerry: {
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("2500Mi"),
		},
		ResourcePresetGrape: {
			corev1.ResourceCPU:    resource.MustParse("3"),
			corev1.ResourceMemory: resource.MustParse("13Gi"),
		},
		ResourcePresetHalfGrape: {
			corev1.ResourceCPU:    resource.MustParse("1500m"),
			corev1.ResourceMemory: resource.MustParse("6400Mi"),
		},
	}
)
