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

	bench "github.com/gardener/scaling-advisor/bench/cmd"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var _ execScaler = (*caExec)(nil)

type caExec struct{}

const caKwokTemplatePath = "templates/kwok-ca-tmpl.yaml"

func (cae *caExec) DeployScalerData(ctx context.Context, cfg *envconf.Config, scenarioDir string) (err error) {
	caKwokCfgFile := path.Join(scenarioDir, bench.FileNameCAKwokProviderConfig)
	if err = deployConfigMap(ctx, cfg, caKwokCfgFile); err != nil {
		return
	}

	templateFilePath := path.Join(scenarioDir, bench.FileNameCAKwokProviderTemplate)
	if err = deployConfigMap(ctx, cfg, templateFilePath); err != nil {
		return
	}

	return
}

func (cae *caExec) GetScalerContainerName() string {
	return bench.ScalerClusterAutoscaler
}

func (cae *caExec) GetScalerKWOKTemplatePath() string {
	return caKwokTemplatePath
}

func (cae *caExec) CheckRequiredDataPresent(scenarioDir, scalerVersion string) error {
	imageName := fmt.Sprintf("gcr.io/k8s-staging-autoscaling/cluster-autoscaler-arm64:%s", scalerVersion)
	if exists := bench.CheckIfImageExists(imageName); !exists {
		return fmt.Errorf("required image %q not found", imageName)
	}

	caKwokCfgFile := path.Join(scenarioDir, bench.FileNameCAKwokProviderConfig)
	templateFilePath := path.Join(scenarioDir, bench.FileNameCAKwokProviderTemplate)

	for _, filePath := range []string{caKwokCfgFile, templateFilePath} {
		if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("required file %q not found", filePath)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func deployConfigMap(ctx context.Context, cfg *envconf.Config, filePath string) error {
	log.Printf("Deploying %q...\n", filePath)

	configMap, err := bench.LoadYAMLFromFile[corev1.ConfigMap](filePath)
	if err != nil {
		return fmt.Errorf("cannot load %q: %w", filePath, err)
	}

	if err := cfg.Client().Resources().Create(ctx, &configMap); err != nil {
		return fmt.Errorf("failed to create %s: %w", configMap.Name, err)
	}
	return nil
}
