// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"text/template"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	plannerapi "github.com/gardener/scaling-advisor/planner/api"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// GetTop30AWSInstanceTypeDefs returns a sample slice of InstanceTypeDef loaded from the embedded
// aws_instance-type-def-set.yaml data file. Intended for use in unit tests.
func GetTop30AWSInstanceTypeDefs() ([]sacorev1alpha1.InstanceTypeDef, error) {
	var its sacorev1alpha1.InstanceTypeDefSet
	if err := objutil.LoadIntoRuntimeObj(dataFS, "data/aws_instance-type-def-set.yaml", &its); err != nil {
		return nil, err
	}
	return its.Spec.InstanceTypeDefs, nil
}

// GetTop30AWSInstancePrices returns a sample slice of InstancePrice loaded from the embedded
// aws_instance-price-set.yaml data file. Intended for use in unit tests.
func GetTop30AWSInstancePrices() ([]sacorev1alpha1.InstancePrice, error) {
	var ips sacorev1alpha1.InstancePriceSet
	if err := objutil.LoadIntoRuntimeObj(dataFS, "data/aws_instance-price-set.yaml", &ips); err != nil {
		return nil, err
	}
	return ips.Spec.Prices, nil
}

// GenScalingConstraints generates a ScalingConstraint and its associated NodeTemplates encapsulated in
// ConstraintGenOutput given the ConstraintGenInput.
func GenScalingConstraints(in ConstraintGenInput) (out ConstraintGenOutput, err error) {
	presetPath := fmt.Sprintf("data/scaling-constraints/%s.yaml", in.PoolPreset)
	ntsPresetPath := fmt.Sprintf("data/scaling-constraints/%s-nts.yaml", in.PoolPreset)
	switch in.PoolPreset {
	case PoolPreset1P, PoolPreset2P:
		if err = objutil.LoadIntoRuntimeObj(dataFS, presetPath, &out.Constraint); err != nil {
			err = fmt.Errorf("failed to load scaling constraints for PoolPreset %q: %v", in.PoolPreset, err)
			return
		}
		var nts sacorev1alpha1.NodeTemplateSet
		if err = objutil.LoadIntoRuntimeObj(dataFS, ntsPresetPath, &nts); err != nil {
			err = fmt.Errorf("failed to load node template set for PoolPreset %q: %v", in.PoolPreset, err)
			return
		}
		out.NodeTemplates = make(map[string]sacorev1alpha1.NodeTemplate, len(nts.Spec.Templates))
		for _, t := range nts.Spec.Templates {
			out.NodeTemplates[t.Name] = t
		}
	default:
		err = fmt.Errorf("%w: unsupported %q", errors.ErrUnsupported, in.PoolPreset)
		return
	}
	if len(in.PoolZones) > len(out.Constraint.Spec.NodePools) {
		err = fmt.Errorf("in.PoolZones %q exceeds number of pools %d in preset %q", in.PoolZones, len(out.Constraint.Spec.NodePools), in.PoolPreset)
		return
	}
	for idx, zones := range in.PoolZones {
		out.Constraint.Spec.NodePools[idx].AvailabilityZones = zones
	}
	if in.GenDir != "" {
		out.GenYAMLPath, err = objutil.SaveRuntimeObjAsYAMLToPath(&out.Constraint, in.GenDir, "constraint.yaml")
		if err != nil {
			return
		}
		nodeTemplateSet := sacorev1alpha1.NodeTemplateSet{
			TypeMeta:   out.Constraint.TypeMeta,
			ObjectMeta: out.Constraint.ObjectMeta,
		}
		nodeTemplateSet.Kind = "NodeTemplateSet"
		for _, t := range out.NodeTemplates {
			nodeTemplateSet.Spec.Templates = append(nodeTemplateSet.Spec.Templates, t)
		}
		out.NodeTemplateSetYAMLPath, err = objutil.SaveRuntimeObjAsYAMLToPath(&nodeTemplateSet, in.GenDir, "node-template-set.yaml")
	}
	return
}

// IncreaseUnscheduledWorkLoad replicates each unscheduled pod by delta inside the given cluster snapshot
func IncreaseUnscheduledWorkLoad(snapshot *plannerapi.ClusterSnapshot, amount int) error {
	var extra []plannerapi.PodInfo
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
func GenerateSimplePodsWithTemplateData(num int, podTmplData PodTemplateData) (output PodGenOutput, err error) {
	tmpl, err := ioutil.LoadEmbeddedTextTemplate(dataFS, simplePodTemplatePath)
	if err != nil {
		return
	}
	if podTmplData.GenDir == "" {
		err = fmt.Errorf("PodTemplateData.GenDir is empty")
		return
	}
	output.Pods = make([]corev1.Pod, 0, num)
	output.YAMLPaths = make(map[apicommon.NamespacedName]string, num)
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
		output.Pods = append(output.Pods, pod)
		output.YAMLPaths[objutil.NamespacedName(&pod)] = outYAMLPath
	}
	return
}

// GenerateSimplePodsForResourcePreset generates simple pods with a container specifying requests for the given
// resourceCategory and using the given metadata. Also generates the pod YAML's for these pods within the temp
// directory. Pod objects and their YAML paths are encapsulated in the returned PodGenOutput.
func GenerateSimplePodsForResourcePreset(category ResourcePreset, num int, metadata PodGenInput) (output PodGenOutput, err error) {
	podTmplData := PodTemplateData{
		PodGenInput: metadata,
		Resources:   category.AsResourceList(),
	}
	return GenerateSimplePodsWithTemplateData(num, podTmplData)
}

// GeneratePersistentVolumeClaims generates a slice of corev1.PersistentVolumeClaim objects with the given
// VolGenInput.PVCNames, VolGenInput.Storage and VolGenInput.AccessMode in the given VolGenInput.Namespace and writes
// generated object YAML files to genDir. Generated PVC's and their YAMLPaths are returned in VolGenOutput.
func GeneratePersistentVolumeClaims(genDir string, vi VolGenInput) (output VolGenOutput, err error) {
	tmpl, err := ioutil.LoadEmbeddedTextTemplate(dataFS, "data/simple-pvc-template.yaml")
	if err != nil {
		return
	}
	if err = vi.ValidateAndFillDefaults(); err != nil {
		return
	}
	output.YAMLPaths = make(map[apicommon.NamespacedName]string, len(vi.PVCNames))
	output.PVCs = make([]corev1.PersistentVolumeClaim, 0, len(vi.PVCNames))
	for _, pvcName := range vi.PVCNames {
		var (
			pvc        corev1.PersistentVolumeClaim
			volumeName string
			yamlPath   = path.Join(genDir, "pvc-"+pvcName+".yaml")
		)
		if vi.ClaimPhase == corev1.ClaimBound {
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
			Phase:            string(vi.ClaimPhase),
			StorageClassName: vi.StorageClassName,
			VolumeName:       volumeName,
			UID:              pvcName,
		}
		err = GenerateAndLoad(tmpl, pvcTmplData, yamlPath, &pvc)
		if err != nil {
			return
		}
		pvc.CreationTimestamp = metav1.Now()
		output.PVCs = append(output.PVCs, pvc)
		output.YAMLPaths[objutil.NamespacedName(&pvc)] = yamlPath
	}
	return
}

// GeneratePersistentVolumes generates a slice of PersistentVolume objects bound to the given VolGenInput.PVCNames
// suitable for the given provider in the given VolGenInput.Namespace for the given VolGenInput.Storage and
// VolGenInput.AccessMode and returns the PersistentVolume objects and their generated YAML paths encapsulated within
// VolGenOutput.
//
// If VolGenInput.ClaimPhase is "Pending", then the generated PVs are unbound with no `claimRef` set
// and have their phase set to `corev1.VolumeAvailable`. Otherwise, their phase is set to `corev1.VolumeBound` with
// their `claimRef` referring to the PVC.
func GeneratePersistentVolumes(genDir string, vi VolGenInput) (output VolGenOutput, err error) {
	// provider apicommon.CloudProvider, namespace string, storage resource.Quantity,
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
	numPVs := len(vi.PVCNames) * len(vi.PVZones)
	output.PVCs = make([]corev1.PersistentVolumeClaim, 0, numPVs)
	output.YAMLPaths = make(map[apicommon.NamespacedName]string, numPVs)
	for _, pvcName := range vi.PVCNames {
		var (
			pv          corev1.PersistentVolume
			volumePhase = corev1.VolumeBound
			pvName      string
			yamlPath    string
		)
		if vi.ClaimPhase == corev1.ClaimPending {
			volumePhase = corev1.VolumeAvailable
		}
		for i, zone := range vi.PVZones {
			pvName = fmt.Sprintf("pv-%s-%d", pvcName, i)
			yamlPath = path.Join(genDir, pvName+".yaml")
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
			output.PVs = append(output.PVs, pv)
			output.YAMLPaths[objutil.NamespacedName(&pv)] = yamlPath
		}
	}
	return
}

// GetCSINodeDrivers gets a slice of sample CSINodeDrivers for the given provider using hard-coded defaults.
func GetCSINodeDrivers(provider apicommon.CloudProvider, maxAllocatableVolumes int32) ([]storagev1.CSINodeDriver, error) {
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

// GenerateDefaultStorageClass generates a StorageClass suitable for the given provider for the specified VolumeBindingMode.
// The generated storage class has the annotation `storageclass.kubernetes.io/is-default-class` set to `true`.
func GenerateDefaultStorageClass(genDir string, provider apicommon.CloudProvider, name string, volumeBindingMode storagev1.VolumeBindingMode) (storageClass storagev1.StorageClass, outYAMLPath string, err error) {
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
		return fmt.Errorf("%w: execution of %q template failed with params %v: %w", apicommon.ErrExecuteTemplate, tmpl.Name(), params, err)
	}
	err = os.WriteFile(outPath, buf.Bytes(), 0600)
	if err != nil {
		return fmt.Errorf("%w: failed to write output of %q template with params %v to path %q: %w", apicommon.ErrExecuteTemplate, tmpl.Name(), params, outPath, err)
	}
	root, err := os.OpenRoot("/")
	if err != nil {
		return fmt.Errorf("%w: failed to open root FS: %w", apicommon.ErrExecuteTemplate, err)
	}
	return objutil.LoadIntoRuntimeObj(root.FS(), strings.TrimPrefix(outPath, "/"), obj)
}

// GetMaxAllocatableVolumes returns a reasonable default value for the max number of allocatable volumes for a node of the given instanceType.
func GetMaxAllocatableVolumes(provider apicommon.CloudProvider, instanceType string) int32 {
	maxAllocatable, ok := maxAllocatableVolumesByInstanceType[instanceType]
	if ok {
		return maxAllocatable
	}
	switch provider {
	case apicommon.CloudProviderAWS:
		maxAllocatable = maxAllocatableVolumesByInstanceType["m5.large"]
	default:
		maxAllocatable = 16 // safe, conservative fallback
	}
	return maxAllocatable
}

// GetCSIDefaults returns an instance of CSIDefaults populated with reasonable values for the given provider.
func GetCSIDefaults(provider apicommon.CloudProvider) (defaults CSIDefaults, err error) {
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

	providerToCSIDefaults = map[apicommon.CloudProvider]CSIDefaults{
		apicommon.CloudProviderAWS: {
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
