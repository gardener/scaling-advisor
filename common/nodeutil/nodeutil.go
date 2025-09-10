// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package nodeutil

import (
	"fmt"
	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/objutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"maps"
	"time"
)

func GetInstanceType(node *corev1.Node) string {
	return node.Labels[corev1.LabelInstanceTypeStable]
}

// AsNode converts a svcapi.NodeInfo to a corev1.Node object.
func AsNode(info svcapi.NodeInfo) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              info.Name,
			Labels:            info.Labels,
			Annotations:       info.Annotations,
			DeletionTimestamp: &metav1.Time{Time: info.DeletionTimestamp},
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

// ComputeAllocatable computes the allocatable resources of a node given its capacity, system reserved and kube reserved resources.
func ComputeAllocatable(capacity, systemReserved, kubeReserved corev1.ResourceList) corev1.ResourceList {
	allocatable := capacity.DeepCopy()
	objutil.SubtractResources(allocatable, systemReserved)
	objutil.SubtractResources(allocatable, kubeReserved)
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
	nodeLabels[commonconstants.LabelSimulationGroupPassNum] = fmt.Sprintf("%d", groupRunPassNum)
	nodeLabels[corev1.LabelInstanceTypeStable] = nodeTemplate.InstanceType
	nodeLabels[corev1.LabelArchStable] = nodeTemplate.Architecture
	nodeLabels[corev1.LabelTopologyZone] = az
	nodeLabels[corev1.LabelTopologyRegion] = nodePool.Region
	nodeLabels[corev1.LabelOSStable] = string(corev1.Linux)
	nodeLabels[corev1.LabelHostname] = nodeName

	return nodeLabels
}
