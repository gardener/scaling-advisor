// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"

	bench "github.com/gardener/scaling-advisor/bench/cmd"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	sigyaml "sigs.k8s.io/yaml"
)

var _ ExecScaler = (*caExec)(nil)

type caExec struct{}

const caKwokTemplatePath = "templates/kwok-ca-tmpl.yaml"

func (cae *caExec) DeployScalerData(ctx context.Context, cfg *envconf.Config) (err error) {
	snapshotDir := path.Dir(snapshotFile)
	caKwokCfgFile := path.Join(snapshotDir, bench.FileNameCAKwokProviderConfig)
	err = deployCAKwokConfig(ctx, caKwokCfgFile, cfg)
	if err != nil {
		return
	}

	templateFilePath := path.Join(snapshotDir, bench.FileNameCAKwokProviderTemplate)
	err = deployCAKwokTemplate(ctx, templateFilePath, cfg)
	if err != nil {
		return
	}
	return
}

func (cae *caExec) GetScalerKWOKTemplatePath() string {
	return caKwokTemplatePath
}

func (cae *caExec) CheckRequiredDataPresent(scenarioDir, scalerVersion string) error {
	// Check files and image with tag present in docker
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

func deployCAKwokTemplate(ctx context.Context, templateFilePath string, cfg *envconf.Config) error {
	log.Printf("Deploying CA kwok-provider-templates %q...\n", templateFilePath)
	file, err := os.Open(templateFilePath)
	if err != nil {
		return fmt.Errorf("cannot open the kwok cr file %q: %v", templateFilePath, err)
	}
	defer file.Close()

	kwokTemplateData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("cannot read the kwok cr file %q: %v", file.Name(), err)
	}
	kwokProviderTemplate := corev1.ConfigMap{}
	if err := sigyaml.Unmarshal(kwokTemplateData, &kwokProviderTemplate); err != nil {
		return fmt.Errorf("cannot unmarshal the kwokTemplate data for %q: %v", file.Name(), err)
	}
	if err := cfg.Client().Resources().Create(ctx, &kwokProviderTemplate); err != nil {
		return fmt.Errorf("failed to create kwok provider template: %w", err)
	}
	return nil
}

func deployCAKwokConfig(ctx context.Context, caKwokCfgFile string, cfg *envconf.Config) error {
	log.Printf("Deploying CA kwok-provider-config %q...\n", caKwokCfgFile)
	file, err := os.Open(caKwokCfgFile)
	if err != nil {
		return fmt.Errorf("cannot open the kwok provider config file %q: %v", caKwokCfgFile, err)
	}
	defer file.Close()

	kwokConfigData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("cannot read the kwok provider config file %q: %v", file.Name(), err)
	}
	kwokProviderConfig := corev1.ConfigMap{}
	if err := sigyaml.Unmarshal(kwokConfigData, &kwokProviderConfig); err != nil {
		return fmt.Errorf("cannot unmarshal the kwokConfig data for %q: %v", file.Name(), err)
	}
	// fmt.Printf("Kwok provider cfg data is:\n%#v\n", kwokProviderConfig)

	if err := cfg.Client().Resources().Create(ctx, &kwokProviderConfig); err != nil {
		return fmt.Errorf("failed to create kwok provider config: %w", err)
	}
	return nil
}
