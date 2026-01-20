// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"reflect"
	"testing"

	"github.com/gardener/scaling-advisor/api/common/constants"
	configv1alpha1 "github.com/gardener/scaling-advisor/api/config/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseLaunchOptions(t *testing.T) {
	tests := []struct {
		want    *LaunchOptions
		name    string
		args    []string
		wantErr bool
	}{
		{
			name: "ShouldSetVersion",
			args: []string{"--version"},
			want: &LaunchOptions{Version: true},
		},
		{
			name: "ShouldSetConfigFile",
			args: []string{"--config=/tmp/scaling-advisor.yaml"},
			want: &LaunchOptions{ConfigFile: "/tmp/scaling-advisor.yaml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLaunchOptions(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLaunchOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseLaunchOptions() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLaunchOptions_ValidateAndLoadOperatorConfig(t *testing.T) {
	tests := []struct {
		want       *configv1alpha1.OperatorConfig
		name       string
		configFile string
		wantErr    bool
	}{
		{
			name:       "ShouldLoadMinimalScalingAdvisorConfig",
			configFile: "testdata/basic-operator-config.yaml",
			want:       updateOperatorConfigWithDefaults(&configv1alpha1.OperatorConfig{}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &LaunchOptions{
				ConfigFile: tt.configFile,
			}
			got, err := o.LoadAndValidateOperatorConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadAndValidateOperatorConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(configv1alpha1.OperatorConfig{})); diff != "" {
				t.Errorf("operator config mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func updateOperatorConfigWithDefaults(operatorConfig *configv1alpha1.OperatorConfig) *configv1alpha1.OperatorConfig {
	configv1alpha1.SetDefaults_ScalingAdvisorServerConfiguration(&operatorConfig.Server)
	operatorConfig.TypeMeta = metav1.TypeMeta{
		Kind:       constants.KindOperatorConfig,
		APIVersion: configv1alpha1.SchemeGroupVersion.String(),
	}
	return operatorConfig
}
