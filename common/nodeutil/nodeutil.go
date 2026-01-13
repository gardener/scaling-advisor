// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package nodeutil

import (
	"fmt"
	"maps"
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// GetInstanceType returns the Instance Type of a node from the label present on it.
func GetInstanceType(node *corev1.Node) string {
	return node.Labels[corev1.LabelInstanceTypeStable]
}

// AsNode converts a planner.NodeInfo to a corev1.NodeResources object.
func AsNode(info planner.NodeInfo) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              info.Name,
			Labels:            info.Labels,
			Annotations:       info.Annotations,
			DeletionTimestamp: info.DeletionTimestamp,
		},
		Spec: corev1.NodeSpec{
			Taints:        info.Taints,
			Unschedulable: info.Unschedulable,
		},
		Status: corev1.NodeStatus{
			Capacity:    objutil.Int64MapToResourceList(info.Capacity),
			Allocatable: objutil.Int64MapToResourceList(info.Allocatable),
			Conditions:  info.Conditions,
		},
	}
}

// BuildAllocatable builds the allocatable resources of a node given its capacity, system reserved and kube reserved resources.
func BuildAllocatable(capacity, systemReserved, kubeReserved corev1.ResourceList) corev1.ResourceList {
	allocatable := capacity.DeepCopy()
	objutil.SubtractResources(allocatable, systemReserved)
	objutil.SubtractResources(allocatable, kubeReserved)
	if _, ok := allocatable[corev1.ResourcePods]; !ok {
		allocatable[corev1.ResourcePods] = resource.MustParse("110")
	}
	return allocatable
}

// BuildReadyConditions builds a slice of NodeCondition for a ready node with the given transition time.
func BuildReadyConditions(transitionTime time.Time) []corev1.NodeCondition {
	return []corev1.NodeCondition{
		{
			Type:               corev1.NodeReady,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Time{Time: transitionTime},
		},
	}
}

// CreateNodeLabels creates the labels for a simulated node.
func CreateNodeLabels(simulationName string, nodePool *sacorev1alpha1.NodePool, nodeTemplate *sacorev1alpha1.NodeTemplate, az string, groupRunPassNum uint32, nodeName string) map[string]string {
	nodeLabels := maps.Clone(nodePool.Labels)

	nodeLabels[commonconstants.LabelSimulationName] = simulationName
	nodeLabels[commonconstants.LabelSimulationGroupNumPasses] = fmt.Sprintf("%d", groupRunPassNum)
	nodeLabels[corev1.LabelInstanceTypeStable] = nodeTemplate.InstanceType
	nodeLabels[corev1.LabelArchStable] = nodeTemplate.Architecture
	nodeLabels[corev1.LabelTopologyZone] = az
	nodeLabels[corev1.LabelTopologyRegion] = nodePool.Region
	nodeLabels[corev1.LabelOSStable] = string(corev1.Linux)
	nodeLabels[corev1.LabelHostname] = nodeName

	return nodeLabels
}

// AsNodeInfo converts a corev1.NodeResources into a planner.NodeInfo object.
// It additionally takes in csiDriverVolumeMaximums which is a map
// of CSI driver names to the maximum number of volumes managed by
// the driver on the node.
func AsNodeInfo(node corev1.Node, csiDriverVolumeMaximums map[string]int32) planner.NodeInfo {
	return planner.NodeInfo{
		ResourceMeta: planner.ResourceMeta{
			UID:               node.UID,
			NamespacedName:    types.NamespacedName{Name: node.Name, Namespace: node.Namespace},
			Labels:            node.Labels,
			Annotations:       node.Annotations,
			DeletionTimestamp: node.DeletionTimestamp,
			OwnerReferences:   node.OwnerReferences,
		},
		InstanceType:            node.Labels[corev1.LabelInstanceTypeStable],
		Unschedulable:           node.Spec.Unschedulable,
		Taints:                  node.Spec.Taints,
		Capacity:                objutil.ResourceListToInt64Map(node.Status.Capacity),
		Allocatable:             objutil.ResourceListToInt64Map(node.Status.Allocatable),
		Conditions:              node.Status.Conditions,
		CSIDriverVolumeMaximums: csiDriverVolumeMaximums,
	}
}
