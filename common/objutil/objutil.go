// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package objutil

import (
	"bytes"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

// ToYAML serializes the given k8s runtime.Object to YAML.
func ToYAML(obj runtime.Object) (string, error) {
	scheme := runtime.NewScheme()
	serializer := json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme, scheme, json.SerializerOptions{Yaml: true, Pretty: true})
	var buf bytes.Buffer
	err := serializer.Encode(obj, &buf)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// LoadYAMLIntoRuntimeObject deserializes the YAML reading it from the specified path into the given k8s runtime.Object.
func LoadYAMLIntoRuntimeObject(yamlPath string, s *runtime.Scheme, obj runtime.Object) error {
	configDecoder := serializer.NewCodecFactory(s).UniversalDecoder()
	configBytes, err := os.ReadFile(yamlPath)
	if err != nil {
		return err
	}
	if err := runtime.DecodeInto(configDecoder, configBytes, obj); err != nil {
		return err
	}
	return nil
}
