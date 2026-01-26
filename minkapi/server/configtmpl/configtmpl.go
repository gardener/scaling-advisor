// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package configtmpl

import (
	"bytes"
	"embed"
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"os"
	"text/template"
)

//go:embed templates/*.yaml
var content embed.FS

var (
	kubeConfigTemplate          *template.Template
	kubeSchedulerConfigTemplate *template.Template
)

func loadKubeConfigTemplate() (err error) {
	if kubeConfigTemplate != nil {
		return
	}
	kubeConfigTemplate, err = ioutil.LoadEmbeddedTextTemplate(content, "templates/kubeconfig.yaml")
	return
}

func loadKubeSchedulerConfigTemplate() (err error) {
	if kubeSchedulerConfigTemplate != nil {
		return
	}
	kubeSchedulerConfigTemplate, err = ioutil.LoadEmbeddedTextTemplate(content, "templates/kube-scheduler-config.yaml")
	return
}

// KubeSchedulerTmplParams encapsulates Go template parameters for generating a very simple kube-scheduler configuration that utilizes a minkapi server.
type KubeSchedulerTmplParams struct {
	KubeConfigPath          string
	KubeSchedulerConfigPath string
	QPS                     float32
	Burst                   int
}

// KubeConfigParams encapsulates Go template parameters for generating a plain kubeconfig file that can be used by a k8s client to connect to a minkapi server.
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
		return fmt.Errorf("%w: cannot render %q template with params %q: %w", commonerrors.ErrExecuteTemplate, kubeConfigTemplate.Name(), params, err)
	}
	err = os.WriteFile(params.KubeConfigPath, buf.Bytes(), 0600)
	if err != nil {
		return fmt.Errorf("%w: cannot write kubeconfig to %q: %w", commonerrors.ErrExecuteTemplate, params.KubeConfigPath, err)
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
		return fmt.Errorf("%w: execution of %q template failed with params %v: %w", commonerrors.ErrExecuteTemplate, kubeSchedulerConfigTemplate.Name(), params, err)
	}
	err = os.WriteFile(params.KubeSchedulerConfigPath, buf.Bytes(), 0600)
	if err != nil {
		return fmt.Errorf("%w: cannot write scheduler config to %q: %w", commonerrors.ErrExecuteTemplate, params.KubeSchedulerConfigPath, err)
	}
	return nil
}
