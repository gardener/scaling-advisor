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

var simplePodTemplatePath = "data/simple-pod-template.yaml"

// GenerateSimplePodsWithTemplateData generates a slice of corev1.Pod objects with count length using the given pod template data in podTmplData.
// Also generates the pod YAMLs for these pods within the temp directory.
func GenerateSimplePodsWithTemplateData(num int, podTmplData SimplePodTemplateData) (pods []corev1.Pod, podYAMLPaths []string, err error) {
	tmpl, err := ioutil.LoadEmbeddedTextTemplate(dataFS, simplePodTemplatePath)
	if err != nil {
		return
	}
	for i := 1; i <= num; i++ {
		var pod corev1.Pod
		tmplData := fillPodTemplateDataDefaults(podTmplData)
		tmplData.Name = tmplData.Name + "-" + strconv.Itoa(i)
		outYAMLPath := path.Join(ioutil.GetTempDir(), "pod-"+tmplData.Name+".yaml")
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
func GenerateSimplePodsForResourceCategory(category ResourceCategory, num int, metadata SimplePodGenInput) (pods []corev1.Pod, podYAMLPaths []string, err error) {
	podTmplData := SimplePodTemplateData{
		SimplePodGenInput: metadata,
		Resources:         category.AsResourceList(),
	}
	return GenerateSimplePodsWithTemplateData(num, podTmplData)
}

// GeneratePersistentVolumeClaims generates a slice of corev1.PersistentVolumeClaim objects with the given pvcNames,  storage and accessMode in the given namespace.
func GeneratePersistentVolumeClaims(namespace string, storage resource.Quantity, accessMode corev1.PersistentVolumeAccessMode, names []string) (pvcs []corev1.PersistentVolumeClaim, pvcYAMLPaths []string, err error) {
	tmpl, err := ioutil.LoadEmbeddedTextTemplate(dataFS, "data/simple-pvc-template.yaml")
	if err != nil {
		return
	}
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}
	for _, pvcName := range names {
		var pvc corev1.PersistentVolumeClaim
		outYAMLPath := path.Join(ioutil.GetTempDir(), "pvc-"+pvcName+".yaml")
		pvcTmplData := struct {
			Name       string
			Namespace  string
			AccessMode string
			Storage    string
		}{
			Name:       pvcName,
			Namespace:  namespace,
			Storage:    storage.String(),
			AccessMode: string(accessMode),
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

var providerToCSIDrivers = map[commontypes.CloudProvider]string{
	commontypes.CloudProviderAWS: "ebs.csi.aws.com",
}

// GeneratePersistentVolumes generates a slice of PersistentVolume objects bound to the given pvcNames suitable for the given provider in the given
// namespace for the given storage and access mode and returns the PV objects and their generated YAML paths.
func GeneratePersistentVolumes(genInput SimplePVGenInput) (pvs []corev1.PersistentVolume, pvYAMLPaths []string, err error) {
	// provider commontypes.CloudProvider, namespace string, storage resource.Quantity,
	//	accessMode corev1.PersistentVolumeAccessMode, zone string, pvcNames []string)
	tmpl, err := ioutil.LoadEmbeddedTextTemplate(dataFS, "data/simple-pv-template.yaml")
	if err != nil {
		return
	}
	if genInput.Namespace == "" {
		genInput.Namespace = corev1.NamespaceDefault
	}
	if genInput.Provider == "" {
		genInput.Provider = commontypes.CloudProviderAWS
	}
	if genInput.Storage.IsZero() {
		genInput.Storage = resource.MustParse("1Gi")
	}
	if genInput.Zone == "" {
		err = errors.New("zone must be specified in genInput")
		return
	}
	if len(genInput.PVCNames) == 0 {
		err = errors.New("must specify at least one name")
		return
	}
	if genInput.AccessMode == "" {
		genInput.AccessMode = corev1.ReadWriteMany
	}
	csiDriver := providerToCSIDrivers[genInput.Provider]
	if csiDriver == "" {
		err = fmt.Errorf("no CSIDriver found for provider %s", genInput.Provider)
		return
	}
	for _, pvcName := range genInput.PVCNames {
		var pv corev1.PersistentVolume
		pvName := objutil.GenerateName("pv-")
		outYAMLPath := path.Join(ioutil.GetTempDir(), pvName+".yaml")
		pvTmplData := struct {
			CSIDriver    string
			Name         string
			Namespace    string
			Storage      string
			AccessMode   string
			VolumeHandle string
			PVCName      string
			Zone         string
		}{
			CSIDriver:    csiDriver,
			Name:         pvName,
			Namespace:    genInput.Namespace,
			Storage:      genInput.Storage.String(),
			AccessMode:   string(genInput.AccessMode),
			VolumeHandle: pvName,
			PVCName:      pvcName,
			Zone:         genInput.Zone,
		}
		err = GenerateAndLoad(tmpl, pvTmplData, outYAMLPath, &pv)
		if err != nil {
			return
		}
		pv.CreationTimestamp = metav1.Now()
		pvs = append(pvs, pv)
		pvYAMLPaths = append(pvYAMLPaths, outYAMLPath)
	}
	return
}

func GenerateStorageClass(provider commontypes.CloudProvider, name string, volumeBindingMode storagev1.VolumeBindingMode) (storageClass storagev1.StorageClass, outYAMLPath string, err error) {
	tmpl, err := ioutil.LoadEmbeddedTextTemplate(dataFS, "data/sc-template.yaml")
	if err != nil {
		return
	}
	outYAMLPath = path.Join(ioutil.GetTempDir(), "sc-"+name+".yaml")
	csiDriver := providerToCSIDrivers[provider]
	if csiDriver == "" {
		err = fmt.Errorf("no CSIDriver found for provider %s", provider)
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
		CSIDriver:         csiDriver,
		Name:              name,
		VolumeBindingMode: string(volumeBindingMode),
	}
	err = GenerateAndLoad(tmpl, scTmplData, outYAMLPath, &storageClass)
	if err != nil {
		return
	}
	storageClass.CreationTimestamp = metav1.Now()
	return
}

func fillPodTemplateDataDefaults(podTmplData SimplePodTemplateData) SimplePodTemplateData {
	podTmplData.AppLabels = fillAppLabelDefaults(podTmplData.AppLabels)
	if podTmplData.Namespace == "" {
		podTmplData.Namespace = corev1.NamespaceDefault
	}
	if podTmplData.Name == "" {
		podTmplData.Name = podTmplData.AppLabels.Name
	}
	if len(podTmplData.Resources) == 0 {
		podTmplData.Resources = ResourceCategoryPea.AsResourceList()
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

var (
	//go:embed data
	dataFS embed.FS
)
