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

	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
)

//go:embed templates/*.yaml
var content embed.FS

var (
	kubeConfigTemplate          *template.Template
	kubeSchedulerConfigTemplate *template.Template
)

func loadKubeConfigTemplate() error {
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

func loadKubeSchedulerConfigTemplate() error {
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
		return nil, fmt.Errorf("%w: cannot read %s from content FS: %w", mkapi.ErrLoadConfigTemplate, templateConfigPath, err)
	}
	templateConfig, err := template.New(templateConfigPath).Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("%w: cannot parse %s template: %w", mkapi.ErrLoadConfigTemplate, templateConfigPath, err)
	}
	return templateConfig, nil
}

// KubeSchedulerTmplParams encapsulates Go template parameters for generating a very simple kube-scheduler configuration that utilizes a minkapi server.
type KubeSchedulerTmplParams struct {
	KubeConfigPath          string
	KubeSchedulerConfigPath string
	QPS                     float32
	Burst                   int
}

// KubeConfigParams encapsulates Go template parametes for generating a plain kubeconfig file that can be used by a k8s client to connect to a minkapi server.
type KubeConfigParams struct {
	Name           string
	KubeConfigPath string
	URL            string
}

// GenKubeConfig generates a kubeconfig file using the provided parameters and writes it to the file path specified in params.KubeConfigPath.
func GenKubeConfig(params KubeConfigParams) error {
	err := loadKubeConfigTemplate()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = kubeConfigTemplate.Execute(&buf, params)
	if err != nil {
		return fmt.Errorf("%w: cannot render %q template with params %q: %w", mkapi.ErrExecuteConfigTemplate, kubeConfigTemplate.Name(), params, err)
	}
	err = os.WriteFile(params.KubeConfigPath, buf.Bytes(), 0600)
	if err != nil {
		return fmt.Errorf("%w: cannot write kubeconfig to %q: %w", mkapi.ErrExecuteConfigTemplate, params.KubeConfigPath, err)
	}
	return nil
}

// GenKubeSchedulerConfig generates a kube-scheduler configuration file using the provided parameters and writes it to the path specified by params.KubeSchedulerConfigPath
func GenKubeSchedulerConfig(params KubeSchedulerTmplParams) error {
	err := loadKubeSchedulerConfigTemplate()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = kubeSchedulerConfigTemplate.Execute(&buf, params)
	if err != nil {
		return fmt.Errorf("%w: execution of %q template failed with params %v: %w", mkapi.ErrExecuteConfigTemplate, kubeSchedulerConfigTemplate.Name(), params, err)
	}
	err = os.WriteFile(params.KubeSchedulerConfigPath, buf.Bytes(), 0600)
	if err != nil {
		return fmt.Errorf("%w: cannot write scheduler config to %q: %w", mkapi.ErrExecuteConfigTemplate, params.KubeSchedulerConfigPath, err)
	}
	return nil
}
