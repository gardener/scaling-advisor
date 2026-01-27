// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	"bytes"
	"embed"
	"fmt"
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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// LoadBasicScalingConstraints loads the basic scaling constraints for the given poolCardinality from the sample data filesystem.
func LoadBasicScalingConstraints(poolCardinality PoolCardinality) (*sacorev1alpha1.ScalingConstraint, error) {
	var clusterConstraints sacorev1alpha1.ScalingConstraint
	clusterConstraintsPath := fmt.Sprintf("data/%s-scaling-constraints.json", poolCardinality)
	switch poolCardinality {
	case PoolCardinalityOne, PoolCardinalityTwo:
		if err := objutil.LoadIntoRuntimeObj(dataFS, clusterConstraintsPath, &clusterConstraints); err != nil {
			return nil, fmt.Errorf("failed to load scaling constraints for poolCardinality %q: %v", poolCardinality, err)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported %q", commonerrors.ErrUnimplemented, poolCardinality)
	}
	return &clusterConstraints, nil
}

// LoadBasicClusterSnapshot loads the basic cluster snapshot variant from the sample data filesystem for the given poolCardinality
func LoadBasicClusterSnapshot(poolCardinality PoolCardinality) (*planner.ClusterSnapshot, error) {
	var clusterSnapshot planner.ClusterSnapshot
	clusterSnapshotPath := fmt.Sprintf("data/%s-cluster-snapshot.json", poolCardinality)
	switch poolCardinality {
	case PoolCardinalityOne, PoolCardinalityTwo:
		if err := objutil.LoadJSONIntoObject(dataFS, clusterSnapshotPath, &clusterSnapshot); err != nil {
			return nil, fmt.Errorf("failed to load basic cluster snapshot for poolCardinality %q: %w", poolCardinality, err)
		}
	default:
		return nil, fmt.Errorf("%w: unknown %q", commonerrors.ErrUnimplemented, poolCardinality)
	}
	return &clusterSnapshot, nil
}

// IncreaseUnscheduledWorkLoad increases the unscheduled pods by delta for the given cluster snapshot
func IncreaseUnscheduledWorkLoad(snapshot *planner.ClusterSnapshot, amount int) error {
	var extra []planner.PodInfo
	for _, upod := range snapshot.GetUnscheduledPods() {
		for i := 1; i <= amount; i++ {
			p := upod
			p.Name = p.Name + "-" + strconv.Itoa(i)
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
// Also generates the pod YAML's for these pods within the temp directory.
func GenerateSimplePodsWithTemplateData(count int, podTmplData SimplePodTemplateData) (pods []corev1.Pod, podYAMLPaths []string, err error) {
	tmpl, err := ioutil.LoadEmbeddedTextTemplate(dataFS, simplePodTemplatePath)
	if err != nil {
		return
	}
	for i := 1; i <= count; i++ {
		var pod corev1.Pod
		tmplData := fillPodTemplateDataDefaults(podTmplData)
		tmplData.Name = tmplData.Name + "-" + strconv.Itoa(i)
		outYAMLPath := path.Join(ioutil.GetTempDir(), "pod-"+tmplData.Name+".yaml")
		err = GenerateAndLoad(tmpl, tmplData, outYAMLPath, &pod)
		if err != nil {
			return
		}
		pods = append(pods, pod)
		podYAMLPaths = append(podYAMLPaths, outYAMLPath)
	}
	return
}

// GenerateSimplePodsForResourceCategory generates simple pods with a container specifying requests for the given resourceCategory and using the given metadata.
// Also generates the pod YAML's for these pods within the temp directory.
func GenerateSimplePodsForResourceCategory(count int, resourceCategory ResourcePairsName, metadata SimplePodMetadata) (pods []corev1.Pod, podYAMLPaths []string, err error) {
	podTmplData := SimplePodTemplateData{
		SimplePodMetadata: metadata,
		Resources:         resourceCategory.AsResourcePairs(),
	}
	return GenerateSimplePodsWithTemplateData(count, podTmplData)
}

func fillPodTemplateDataDefaults(podTmplData SimplePodTemplateData) SimplePodTemplateData {
	podTmplData.AppLabels = fillAppLabelDefaults(podTmplData.AppLabels)
	if podTmplData.Namespace == "" {
		podTmplData.Namespace = "default"
	}
	if podTmplData.Name == "" {
		podTmplData.Name = podTmplData.AppLabels.Name
	}
	if len(podTmplData.Resources) == 0 {
		podTmplData.Resources = Pea.AsResourcePairs()
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
	//go:embed data/*.*
	dataFS embed.FS
)
