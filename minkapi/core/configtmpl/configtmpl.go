// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package configtmpl

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"text/template"

	"github.com/gardener/scaling-advisor/minkapi/api"
)

//go:embed templates/*.yaml
var content embed.FS

var (
	kubeConfigTemplate          *template.Template
	kubeSchedulerConfigTemplate *template.Template
)

func LoadKubeConfigTemplate() error {
	if kubeConfigTemplate != nil {
		return nil
	}
	var err error
	kubeConfigTemplate, err = loadTemplateConfig("templates/kubeconfig.yaml")
	if err != nil {
		return err
	}
	return nil
}

func LoadKubeSchedulerConfigTemplate() error {
	if kubeSchedulerConfigTemplate != nil {
		return nil
	}
	var err error
	kubeSchedulerConfigTemplate, err = loadTemplateConfig("templates/kube-scheduler-config.yaml")
	if err != nil {
		return err
	}
	return nil
}

func loadTemplateConfig(templateConfigPath string) (*template.Template, error) {
	var err error
	var data []byte

	data, err = content.ReadFile(templateConfigPath)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read %s from content FS: %w", api.ErrLoadConfigTemplate, templateConfigPath, err)
	}
	templateConfig, err := template.New(templateConfigPath).Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("%w: cannot parse %s template: %w", api.ErrLoadConfigTemplate, templateConfigPath, err)
	}
	return templateConfig, nil
}

type KubeSchedulerTmplParams struct {
	KubeConfigPath          string
	KubeSchedulerConfigPath string
	QPS                     float32
	Burst                   int
}

type KubeConfigParams struct {
	KubeConfigPath string
	URL            string
}

func GenKubeConfig(params KubeConfigParams) error {
	err := LoadKubeConfigTemplate()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = kubeConfigTemplate.Execute(&buf, params)
	if err != nil {
		return fmt.Errorf("%w: cannot render %q template with params %q: %w", api.ErrExecuteConfigTemplate, kubeConfigTemplate.Name(), params, err)
	}
	err = os.WriteFile(params.KubeConfigPath, buf.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("%w: cannot write kubeconfig to %q: %w", api.ErrExecuteConfigTemplate, params.KubeConfigPath, err)
	}
	return nil
}

func GenKubeSchedulerConfig(params KubeSchedulerTmplParams) error {
	err := LoadKubeSchedulerConfigTemplate()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = kubeSchedulerConfigTemplate.Execute(&buf, params)
	if err != nil {
		return fmt.Errorf("%w: execution of %q template failed with params %v: %w", api.ErrExecuteConfigTemplate, kubeSchedulerConfigTemplate.Name(), params, err)
	}
	err = os.WriteFile(params.KubeSchedulerConfigPath, buf.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("%w: cannot write scheduler config to %q: %w", api.ErrExecuteConfigTemplate, params.KubeSchedulerConfigPath, err)
	}
	return nil
}
