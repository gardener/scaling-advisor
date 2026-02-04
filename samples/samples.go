// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	"embed"
	"fmt"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
)

const (
	// CategoryBasic is the name associated with a basic scenario.
	CategoryBasic = "basic"
)

//go:embed data/*.*
var dataFS embed.FS

// LoadClusterConstraints loads cluster constraints from the sample data filesystem.
func LoadClusterConstraints(categoryName string) (*sacorev1alpha1.ScalingConstraint, error) {
	var clusterConstraints sacorev1alpha1.ScalingConstraint
	clusterConstraintsPath := fmt.Sprintf("data/%s-cluster-constraints.json", categoryName)
	switch categoryName {
	case CategoryBasic:
		if err := objutil.LoadIntoRuntimeObj(dataFS, clusterConstraintsPath, &clusterConstraints); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: unknown %q", commonerrors.ErrUnimplemented, categoryName)
	}
	return &clusterConstraints, nil
}

// LoadClusterSnapshot loads a cluster snapshot from the sample data filesystem.
func LoadClusterSnapshot(categoryName string) (*planner.ClusterSnapshot, error) {
	var clusterSnapshot planner.ClusterSnapshot
	clusterSnapshotPath := fmt.Sprintf("data/%s-cluster-snapshot.json", categoryName)
	switch categoryName {
	case CategoryBasic:
		if err := objutil.LoadJSONIntoObject(dataFS, clusterSnapshotPath, &clusterSnapshot); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: unknown %q", commonerrors.ErrUnimplemented, categoryName)
	}
	return &clusterSnapshot, nil
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

// GenerateSimplePVCs generates a slice of corev1.PersistentVolumeClaim objects with the given pvcNames in the given namespace.
func GenerateSimplePVCs(namespace string, pvcNames []string) (pvcs []corev1.PersistentVolumeClaim, pvcYAMLPaths []string, err error) {
	tmpl, err := ioutil.LoadEmbeddedTextTemplate(dataFS, "data/simple-pvc-template.yaml")
	if err != nil {
		return
	}
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}
	for _, pvcName := range pvcNames {
		var pvc corev1.PersistentVolumeClaim
		outYAMLPath := path.Join(ioutil.GetTempDir(), "pvc-"+pvcName+".yaml")
		pvcTmplData := struct {
			Name      string
			Namespace string
		}{
			Name:      pvcName,
			Namespace: namespace,
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

func GeneratePersistentVolumes() {

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
func GenerateAndLoad[T any, U runtime.Object](tmpl *template.Template, params T, outPath string, obj U) error {
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
