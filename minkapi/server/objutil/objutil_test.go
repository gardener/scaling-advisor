// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package objutil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestLoadYamlIntoObj(t *testing.T) {
	tests := map[string]struct {
		filePath string
		retErr   error
	}{
		"valid yaml":        {filePath: "../testdata/pod-a.yaml", retErr: nil},
		"corrupt yaml":      {filePath: "../testdata/corrupt-pod-a.yaml", retErr: fmt.Errorf("failed to unmarshal object")},
		"non-yaml file":     {filePath: "./objutil.go", retErr: fmt.Errorf("failed to unmarshal object")},
		"non-existent path": {filePath: "../testdata/bingo.yaml", retErr: fmt.Errorf("failed to read")},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var pod1 corev1.Pod
			gotErr := LoadYamlIntoObj(tc.filePath, &pod1)

			if gotErr != tc.retErr || gotErr != nil {
				if !strings.Contains(gotErr.Error(), tc.retErr.Error()) {
					t.Errorf("Unexpected error when loading yaml into a pod, got: %v, wanted: %v", gotErr, tc.retErr)
				} else {
					t.Logf("Expected error: %v", gotErr)
				}
			}
		})
	}
}

func TestSetMetaObjectGVK(t *testing.T) {
	testPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bingo",
			Namespace: "default",
		},
	}

	tests := map[string]struct {
		typeMeta    metav1.TypeMeta
		expectedGVK schema.GroupVersionKind
	}{
		"version and kind present": {
			typeMeta:    metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			expectedGVK: typeinfo.PodsDescriptor.GVK,
		},
		"kind absent": {
			typeMeta:    metav1.TypeMeta{APIVersion: "v1"},
			expectedGVK: schema.GroupVersionKind{Version: "v1"},
		},
		"version absent": {
			typeMeta:    metav1.TypeMeta{Kind: "Pod"},
			expectedGVK: schema.GroupVersionKind{Kind: "Pod"},
		},
		"version and kind absent": {
			typeMeta:    metav1.TypeMeta{},
			expectedGVK: typeinfo.PodsDescriptor.GVK,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			pod1 := testPod.DeepCopy()
			pod1.TypeMeta = tc.typeMeta
			obj1 := metav1.Object(pod1)

			SetMetaObjectGVK(obj1, typeinfo.PodsDescriptor.GVK)

			if rtObj1, ok := obj1.(runtime.Object); ok {
				if gotGVK := rtObj1.GetObjectKind().GroupVersionKind(); gotGVK != tc.expectedGVK {
					t.Errorf("GVK mismatch, got: %#v wanted: %#v", gotGVK, tc.expectedGVK)
				}
			}
		})
	}
}
