// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package objutil

import (
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

func LoadYamlIntoObj(path string, obj any) (err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("failed to read %q: %w", path, err)
		return
	}
	err = yaml.Unmarshal(data, obj)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal object in %q: %w", path, err)
		return
	}
	return
}

func SetMetaObjectGVK(obj metav1.Object, gvk schema.GroupVersionKind) {
	if runtimeObj, ok := obj.(runtime.Object); ok {
		objGVK := runtimeObj.GetObjectKind().GroupVersionKind()
		if objGVK.Kind == "" && objGVK.Version == "" {
			runtimeObj.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
				Group:   gvk.Group,
				Version: gvk.Version,
				Kind:    gvk.Kind,
			})
		}
	}
}
