// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path"

	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"

	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	sigyaml "sigs.k8s.io/yaml"
)

var _ ExecScaler = (*caExec)(nil)

type caExec struct{}

const (
	caKwokTemplatePath       = "templates/kwok-ca-tmpl.yaml"
	caKwokProviderConfigPath = "templates/ca-kwok-provider-config.yaml"
	caPrometheusPort         = 8085
)

func (cae *caExec) DeployNodes(ctx context.Context, cfg *envconf.Config, snapshot *planner.ClusterSnapshot) error {
	log.Printf("Deploying nodes, count %d...\n", len(snapshot.Nodes))
	for _, nodeInfo := range snapshot.Nodes {
		node := nodeutil.AsNode(nodeInfo)
		node.ResourceVersion = ""
		node.Spec.ProviderID = "kwok://" + node.Name
		if node.Annotations == nil {
			node.Annotations = make(map[string]string)
		}
		node.Annotations["kwok.x-k8s.io/node"] = "fake"
		if err := cfg.Client().Resources().Create(ctx, node); err != nil {
			return fmt.Errorf("failed to create node: %w", err)
		}
	}
	return nil
}

// TODO: check if priority-expander file corresponding to the snapshot is present,
// then deploy that in kube-system namspace.
func (cae *caExec) DeployScalerData(ctx context.Context, cfg *envconf.Config, scenarioDir string) (err error) {
	caKwokConfigData, err := content.ReadFile(caKwokProviderConfigPath)
	if err != nil {
		return
	}
	var providerConfigMap corev1.ConfigMap
	if err = sigyaml.Unmarshal(caKwokConfigData, &providerConfigMap); err != nil {
		return
	}
	if err := cfg.Client().Resources().Create(ctx, &providerConfigMap); err != nil {
		return fmt.Errorf("failed to create %s: %w", providerConfigMap.Name, err)
	}

	templateFilePath := path.Join(scenarioDir, "gen", benchutil.FileNameCAKwokProviderTemplate)
	templatesConfigMap, err := benchutil.LoadYAMLFromFile[corev1.ConfigMap](templateFilePath)
	if err != nil {
		return fmt.Errorf("cannot load %q: %w", templateFilePath, err)
	}
	if err := cfg.Client().Resources().Create(ctx, &templatesConfigMap); err != nil {
		return fmt.Errorf("failed to create %s: %w", templatesConfigMap.Name, err)
	}

	return
}

func (cae *caExec) GetKWOKTemplatePath() string {
	return caKwokTemplatePath
}

func (cae *caExec) GetPrometheusPort() int {
	return caPrometheusPort
}

func (cae *caExec) EventConfig() ScalerEventConfig {
	return ScalerEventConfig{
		Source: benchutil.ScalerClusterAutoscaler,
		WatchedEvents: []string{
			"TriggeredScaleUp", "ScaledUpGroup", "NotTriggerScaleUp", "ScaleDown",
		},
		MarksPodUnschedulable: []string{"NotTriggerScaleUp"},
	}
}

func (cae *caExec) CheckRequiredDataPresent(generatedDir, scalerVersion string) error {
	imageName := fmt.Sprintf("gcr.io/k8s-staging-autoscaling/cluster-autoscaler-arm64:%s", scalerVersion)
	if exists := benchutil.CheckIfImageExists(imageName); !exists {
		return fmt.Errorf("required image %q not found", imageName)
	}

	templateFilePath := path.Join(generatedDir, benchutil.FileNameCAKwokProviderTemplate)
	if _, err := os.Stat(templateFilePath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("required file %q not found", templateFilePath)
	}
	return nil
}
