// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package objutil

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"reflect"
	"testing"

	"github.com/gardener/scaling-advisor/common/testutil"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResourceListToInt64MapAndBack(t *testing.T) {
	tests := []struct {
		name           string
		src            corev1.ResourceList
		expectedTarget map[corev1.ResourceName]int64
	}{
		{
			name: "simple-cpu_mem_ephemeral-storage",
			src: corev1.ResourceList{
				corev1.ResourceMemory:           *resource.NewQuantity(1024, resource.BinarySI),
				corev1.ResourceCPU:              *resource.NewQuantity(2, resource.DecimalSI),
				corev1.ResourceEphemeralStorage: resource.MustParse("100Mi"),
			},
			expectedTarget: map[corev1.ResourceName]int64{
				corev1.ResourceMemory:           1024,
				corev1.ResourceCPU:              2,
				corev1.ResourceEphemeralStorage: 100 * 1024 * 1024,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualTarget := ResourceListToInt64Map(tt.src)
			if !reflect.DeepEqual(actualTarget, tt.expectedTarget) {
				t.Errorf("ResourceListToInt64Map() = %v, expectedTarget %v", actualTarget, tt.expectedTarget)
			}
			// reverse conversion
			actualReversed := Int64MapToResourceList(actualTarget)
			if !IsResourceListEqual(actualReversed, tt.src) {
				t.Errorf("ResourceListToInt64Map() = %v, expectedTarget %v", actualReversed, tt.src)
			}
		})
	}
}

func TestLoadYamlIntoCoreRuntimeObj(t *testing.T) {
	tests := map[string]struct {
		filePath string
		retErr   error
	}{
		"valid yaml":        {filePath: "../../minkapi/server/testdata/pod-a.yaml", retErr: nil},
		"corrupt yaml":      {filePath: "../../minkapi/server/testdata/corrupt-pod-a.yaml", retErr: fmt.Errorf("failed to unmarshal object")},
		"non-yaml file":     {filePath: "./objutil.go", retErr: fmt.Errorf("failed to unmarshal object")},
		"non-existent path": {filePath: "../../minkapi/server/testdata/bingo.yaml", retErr: fmt.Errorf("failed to read")},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var pod1 corev1.Pod
			gotErr := LoadYamlIntoCoreRuntimeObj(tc.filePath, &pod1)
			testutil.AssertError(t, gotErr, tc.retErr)
		})
	}
}

func TestSetMetaObjectGVK(t *testing.T) {
	testPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bingo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	tests := map[string]struct {
		typeMeta    metav1.TypeMeta
		expectedGVK schema.GroupVersionKind
	}{
		"version and kind present": {
			typeMeta:    metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			expectedGVK: corev1.SchemeGroupVersion.WithKind("Pod"),
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
			expectedGVK: corev1.SchemeGroupVersion.WithKind("Pod"),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			pod1 := testPod.DeepCopy()
			pod1.TypeMeta = tc.typeMeta
			obj1 := metav1.Object(pod1)

			SetMetaObjectGVK(obj1, corev1.SchemeGroupVersion.WithKind("Pod"))

			if rtObj1, ok := obj1.(runtime.Object); ok {
				if gotGVK := rtObj1.GetObjectKind().GroupVersionKind(); gotGVK != tc.expectedGVK {
					t.Errorf("GVK mismatch, got: %#v wanted: %#v", gotGVK, tc.expectedGVK)
				}
			}
		})
	}
}

func TestPatchPodStatus(t *testing.T) {
	testPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bingo",
			Namespace: metav1.NamespaceDefault,
		},
	}
	var testPodPatchStatus = `{
"status" : {
	"conditions" : [ {
	  "lastProbeTime" : null,
	  "lastTransitionTime" : "2025-05-08T08:21:44Z",
	  "message" : "no nodes available to schedule pods",
	  "reason" : "Unschedulable",
	  "status" : "False",
	  "type" : "PodScheduled"
	} ]
  }
}
`
	var testIncorrectPatch = `{
"status" : {
	"conditions" : "not-an-array"
  }
}`

	tests := map[string]struct {
		patch      string
		key        string
		patchErr   error
		passNilObj bool
	}{
		"correct patch": {
			key:      "default/bingo",
			patchErr: nil,
			patch:    testPodPatchStatus,
		},
		"incorrect patch": {
			key:      "default/bingo",
			patchErr: fmt.Errorf("failed to unmarshal patched status"),
			patch:    testIncorrectPatch,
		},
		"nil Object": {
			key:        "default/bingo",
			patchErr:   fmt.Errorf("non-nil pointer"),
			patch:      testPodPatchStatus,
			passNilObj: true,
		},
		"patch with no status": {
			key:      "default/bingo",
			patchErr: fmt.Errorf("does not contain a 'status'"),
			patch:    `{}`,
		},
		"corrupted patch": {
			key:      "default/bingo",
			patchErr: fmt.Errorf("failed to parse patch"),
			patch:    `{{}`,
		},
		"incorrect key": { // TODO Is key only utilized for error messages
			key:      "default/abc",
			patchErr: nil,
			patch:    testPodPatchStatus,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var err error
			pod := testPod.DeepCopy()
			obj := metav1.Object(pod)

			objectName := cache.NewObjectName(metav1.NamespaceDefault, tc.key)
			if tc.passNilObj {
				err = PatchObjectStatus(nil, objectName, []byte(tc.patch))
			} else {
				err = PatchObjectStatus(obj.(runtime.Object), objectName, []byte(tc.patch))
			}
			if err != nil {
				testutil.AssertError(t, err, tc.patchErr)
				return
			}

			t.Logf("Patched pod status: %#v", pod.Status.Conditions)
			if pod.Status.Conditions == nil {
				t.Errorf("Failed to set pod conditions")
			}
		})
	}
}

func TestPatchObjectUsingEvent(t *testing.T) {
	testEvent := eventsv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "a-bingo.aaabbb",
			Namespace: metav1.NamespaceDefault,
		},
	}

	var patchEventSeries = `
{
  "series": {
	"count": 2,
	"lastObservedTime": "2025-05-08T09:05:57.028064Z"
  }
}
`
	var corruptedPatch = `{}}`
	var invalidPatch = `{ "metadata": "abcdefgh"}`
	contentTypeTests := map[string]struct {
		contentType string
		patchData   string
		patchErr    error
		passNilObj  bool
	}{
		"Strategic Merge Patch": {
			contentType: "application/strategic-merge-patch+json",
			patchData:   patchEventSeries,
			patchErr:    nil,
		},
		"Merge Patch": {
			contentType: "application/merge-patch+json",
			patchData:   patchEventSeries,
			patchErr:    nil,
		},
		"Unsupported ContentType": {
			contentType: "application/json-patch+json",
			patchData:   patchEventSeries,
			patchErr:    fmt.Errorf("unsupported patch type"),
		},
		"Corrupted Strategic Merge Patch": {
			contentType: "application/strategic-merge-patch+json",
			patchData:   corruptedPatch,
			patchErr:    fmt.Errorf("invalid JSON"),
		},
		"Corrupted Merge Patch": {
			contentType: "application/merge-patch+json",
			patchData:   corruptedPatch,
			patchErr:    fmt.Errorf("Invalid JSON"),
		},
		"invalid Patch": {
			contentType: "application/merge-patch+json",
			patchData:   invalidPatch,
			patchErr:    fmt.Errorf("failed to unmarshal patched JSON"),
		},
		"Nil object Patch": {
			contentType: "application/merge-patch+json",
			patchData:   patchEventSeries,
			patchErr:    fmt.Errorf("non-nil pointer"),
			passNilObj:  true,
		},
	}

	for name, tc := range contentTypeTests {
		t.Run(name, func(t *testing.T) {
			var err error
			event := testEvent.DeepCopy()
			obj := metav1.Object(event)

			name := cache.NewObjectName("default", "a-bingo.aaabbb")
			if tc.passNilObj {
				err = PatchObject(nil, name, types.PatchType(tc.contentType), []byte(tc.patchData))
			} else {
				err = PatchObject(obj.(runtime.Object), name, types.PatchType(tc.contentType), []byte(tc.patchData))
			}
			if err != nil {
				testutil.AssertError(t, err, tc.patchErr)
				return
			}

			t.Logf("Patched event series: %v", event.Series)
			if event.Series == nil {
				t.Errorf("Failed to patch event series")
			}
		})
	}
}
