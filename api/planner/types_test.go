// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"testing"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestClusterSnapshot_GetUnscheduledPods(t *testing.T) {
	tests := []struct {
		snapshot *ClusterSnapshot
		name     string
		expected int
	}{
		{
			name: "all pods scheduled",
			snapshot: &ClusterSnapshot{
				Pods: []PodInfo{
					{NodeName: "node-1"},
					{NodeName: "node-2"},
				},
			},
			expected: 0,
		},
		{
			name: "all pods unscheduled",
			snapshot: &ClusterSnapshot{
				Pods: []PodInfo{
					{NodeName: ""},
					{NodeName: ""},
				},
			},
			expected: 2,
		},
		{
			name: "mixed scheduled and unscheduled",
			snapshot: &ClusterSnapshot{
				Pods: []PodInfo{
					{NodeName: "node-1"},
					{NodeName: ""},
					{NodeName: "node-2"},
					{NodeName: ""},
				},
			},
			expected: 2,
		},
		{
			name: "empty snapshot",
			snapshot: &ClusterSnapshot{
				Pods: []PodInfo{},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.snapshot.GetUnscheduledPods()
			if len(result) != tt.expected {
				t.Errorf("expected %d unscheduled pods, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestClusterSnapshot_GetNodeCountByPlacement(t *testing.T) {
	tests := []struct {
		snapshot    *ClusterSnapshot
		expected    map[sacorev1alpha1.NodePlacement]int32
		name        string
		expectError bool
	}{
		{
			name: "nodes with complete placement info",
			snapshot: &ClusterSnapshot{
				Nodes: []NodeInfo{
					{
						BasicMeta: BasicMeta{
							Labels: map[string]string{
								commonconstants.LabelNodeTemplateName: "template-a",
								commonconstants.LabelNodePoolName:     "pool-a",
								corev1.LabelTopologyRegion:            "us-east-1",
								corev1.LabelTopologyZone:              "us-east-1a",
								corev1.LabelInstanceTypeStable:        "m5.large",
								corev1.LabelHostname:                  "host1",
								corev1.LabelArchStable:                "amd64",
							},
						},
						InstanceType: "m5.large",
					},
					{
						BasicMeta: BasicMeta{
							Labels: map[string]string{
								commonconstants.LabelNodeTemplateName: "template-a",
								commonconstants.LabelNodePoolName:     "pool-a",
								corev1.LabelTopologyRegion:            "us-east-1",
								corev1.LabelTopologyZone:              "us-east-1a",
								corev1.LabelHostname:                  "host1",
								corev1.LabelInstanceTypeStable:        "m5.large",
								corev1.LabelArchStable:                "amd64",
							},
						},
						InstanceType: "m5.large",
					},
					{
						BasicMeta: BasicMeta{
							Labels: map[string]string{
								commonconstants.LabelNodeTemplateName: "template-b",
								commonconstants.LabelNodePoolName:     "pool-b",
								corev1.LabelTopologyRegion:            "us-west-2",
								corev1.LabelTopologyZone:              "us-west-2a",
								corev1.LabelInstanceTypeStable:        "m5.large",
								corev1.LabelHostname:                  "host1",
								corev1.LabelArchStable:                "amd64",
							},
						},
						InstanceType: "m5.xlarge",
					},
				},
			},
			expectError: false,
			expected: map[sacorev1alpha1.NodePlacement]int32{
				{
					NodePoolName:     "pool-a",
					NodeTemplateName: "template-a",
					InstanceType:     "m5.large",
					Region:           "us-east-1",
					AvailabilityZone: "us-east-1a",
				}: 2,
				{
					NodePoolName:     "pool-b",
					NodeTemplateName: "template-b",
					InstanceType:     "m5.xlarge",
					Region:           "us-west-2",
					AvailabilityZone: "us-west-2a",
				}: 1,
			},
		},
		{
			name: "missing node template name label",
			snapshot: &ClusterSnapshot{
				Nodes: []NodeInfo{
					{
						BasicMeta: BasicMeta{
							Labels: map[string]string{
								commonconstants.LabelNodePoolName: "pool-a",
								corev1.LabelTopologyRegion:        "us-east-1",
								corev1.LabelTopologyZone:          "us-east-1a",
							},
						},
						InstanceType: "m5.large",
					},
				},
			},
			expectError: true,
		},
		{
			name: "missing node pool name label",
			snapshot: &ClusterSnapshot{
				Nodes: []NodeInfo{
					{
						BasicMeta: BasicMeta{
							Labels: map[string]string{
								commonconstants.LabelNodeTemplateName: "template-a",
								corev1.LabelTopologyRegion:            "us-east-1",
								corev1.LabelTopologyZone:              "us-east-1a",
							},
						},
						InstanceType: "m5.large",
					},
				},
			},
			expectError: true,
		},
		{
			name: "missing region label",
			snapshot: &ClusterSnapshot{
				Nodes: []NodeInfo{
					{
						BasicMeta: BasicMeta{
							Labels: map[string]string{
								commonconstants.LabelNodeTemplateName: "template-a",
								commonconstants.LabelNodePoolName:     "pool-a",
								corev1.LabelTopologyZone:              "us-east-1a",
							},
						},
						InstanceType: "m5.large",
					},
				},
			},
			expectError: true,
		},
		{
			name: "missing availability zone label",
			snapshot: &ClusterSnapshot{
				Nodes: []NodeInfo{
					{
						BasicMeta: BasicMeta{
							Labels: map[string]string{
								commonconstants.LabelNodeTemplateName: "template-a",
								commonconstants.LabelNodePoolName:     "pool-a",
								corev1.LabelTopologyRegion:            "us-east-1",
							},
						},
						InstanceType: "m5.large",
					},
				},
			},
			expectError: true,
		},
		{
			name: "empty nodes",
			snapshot: &ClusterSnapshot{
				Nodes: []NodeInfo{},
			},
			expectError: false,
			expected:    map[sacorev1alpha1.NodePlacement]int32{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.snapshot.GetNodeCountByPlacement()

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if diff := cmp.Diff(tt.expected, result); diff != "" {
				t.Errorf("GetNodeCountByPlacement() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPodInfo_GetResourceInfo(t *testing.T) {
	podInfo := PodInfo{
		BasicMeta: BasicMeta{
			UID:       "pod-uid-123",
			Namespace: "default",
			Name:      "test-pod",
		},
		AggregatedRequests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2048Mi"),
		},
	}

	expected := PodResourceInfo{
		UID:       "pod-uid-123",
		Namespace: "default",
		Name:      "test-pod",
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
		BasicMeta: BasicMeta{
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

	expected := NodeResourceInfo{
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
		key      SimGroupKey
	}{
		{
			name: "both priorities positive",
			key: SimGroupKey{
				NodePoolPriority:     1,
				NodeTemplatePriority: 2,
			},
			expected: "1-2",
		},
		{
			name: "zero priorities",
			key: SimGroupKey{
				NodePoolPriority:     0,
				NodeTemplatePriority: 0,
			},
			expected: "0-0",
		},
		{
			name: "large priorities",
			key: SimGroupKey{
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
