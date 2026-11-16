// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"testing"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	simulationapi "github.com/gardener/scaling-advisor/simulation/api"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodInfo_GetResourceInfo(t *testing.T) {
	podInfo := PodInfo{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "pod-uid-123",
			Namespace: "default",
			Name:      "test-pod",
		},
		AggregatedRequests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2048Mi"),
		},
	}

	expected := simulationapi.PodResourceInfo{
		NamespacedName: apicommon.NamespacedName{
			Namespace: "default",
			Name:      "test-pod",
		},
		AggregatedRequests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2048Mi"),
		},
	}

	result := podInfo.GetResourceInfo()

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("GetResourceInfo() mismatch (-want +got):\n%s", diff)
	}
}

func TestNodeInfo_GetResourceInfo(t *testing.T) {
	nodeInfo := NodeInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		InstanceType: "m5.large",
		Capacity: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse("4000"),
			corev1.ResourceMemory: resource.MustParse("8192"),
		},
		Allocatable: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse("3800"),
			corev1.ResourceMemory: resource.MustParse("7680"),
		},
	}

	expected := simulationapi.NodeResourceInfo{
		Name:         "node-1",
		InstanceType: "m5.large",
		Capacity: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse("4000"),
			corev1.ResourceMemory: resource.MustParse("8192"),
		},
		Allocatable: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse("3800"),
			corev1.ResourceMemory: resource.MustParse("7680"),
		},
	}

	result := nodeInfo.GetResourceInfo()

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("GetResourceInfo() mismatch (-want +got):\n%s", diff)
	}
}

func TestSimGroupKey_String(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		key      simulationapi.GroupKey
	}{
		{
			name: "both priorities positive",
			key: simulationapi.GroupKey{
				NodePoolPriority:     1,
				NodeTemplatePriority: 2,
			},
			expected: "1-2",
		},
		{
			name: "zero priorities",
			key: simulationapi.GroupKey{
				NodePoolPriority:     0,
				NodeTemplatePriority: 0,
			},
			expected: "0-0",
		},
		{
			name: "large priorities",
			key: simulationapi.GroupKey{
				NodePoolPriority:     100,
				NodeTemplatePriority: 200,
			},
			expected: "100-200",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.key.String()
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}
