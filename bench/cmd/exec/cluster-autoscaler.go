// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	sigyaml "sigs.k8s.io/yaml"
)

var _ ExecScaler = (*caExec)(nil)

type caExec struct{}

const (
	caKwokTemplatePath       = "templates/kwok-ca-tmpl.yaml"
	caKwokProviderConfigPath = "templates/ca-kwok-provider-config.yaml"
)

func (cae *caExec) DeployScalerData(ctx context.Context, cfg *envconf.Config, scenarioDir string) (err error) {
	caKwokCfgData, err := content.ReadFile(caKwokProviderConfigPath)
	if err != nil {
		return
	}
	var cfgMap corev1.ConfigMap
	if err = sigyaml.Unmarshal(caKwokCfgData, &cfgMap); err != nil {
		return
	}
	if err := cfg.Client().Resources().Create(ctx, &cfgMap); err != nil {
		return fmt.Errorf("failed to create %s: %w", cfgMap.Name, err)
	}

	templateFilePath := path.Join(scenarioDir, benchutil.FileNameCAKwokProviderTemplate)
	configMap, err := benchutil.LoadYAMLFromFile[corev1.ConfigMap](templateFilePath)
	if err != nil {
		return fmt.Errorf("cannot load %q: %w", templateFilePath, err)
	}
	if err := cfg.Client().Resources().Create(ctx, &configMap); err != nil {
		return fmt.Errorf("failed to create %s: %w", configMap.Name, err)
	}

	return
}

func (cae *caExec) GetScalerKWOKTemplatePath() string {
	return caKwokTemplatePath
}

func (cae *caExec) EventConfig() ScalerEventConfig {
	return ScalerEventConfig{
		Source:                 "cluster-autoscaler",
		EventNames:             []string{"TriggeredScaleUp", "ScaledUpGroup", "NotTriggerScaleUp"},
		MarksPodUnschedulable: []string{"NotTriggerScaleUp"},
	}
}

func (cae *caExec) CheckRequiredDataPresent(scenarioDir, scalerVersion string) error {
	imageName := fmt.Sprintf("gcr.io/k8s-staging-autoscaling/cluster-autoscaler-arm64:%s", scalerVersion)
	if exists := benchutil.CheckIfImageExists(imageName); !exists {
		return fmt.Errorf("required image %q not found", imageName)
	}

	templateFilePath := path.Join(scenarioDir, benchutil.FileNameCAKwokProviderTemplate)
	if _, err := os.Stat(templateFilePath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("required file %q not found", templateFilePath)
	}
	return nil
}
