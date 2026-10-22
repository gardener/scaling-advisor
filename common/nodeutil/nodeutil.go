// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package nodeutil

import (
	"time"

	"github.com/gardener/scaling-advisor/common/objutil"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	minkapi "github.com/gardener/scaling-advisor/minkapi/api"
	plannerapi "github.com/gardener/scaling-advisor/planner/api"
	plannerapivalidation "github.com/gardener/scaling-advisor/planner/api/validation"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// GetInstanceType returns the instance-type of the given node from the label present on it.
func GetInstanceType(node *corev1.Node) string {
	return node.Labels[corev1.LabelInstanceTypeStable]
}

// AsNodeInfo converts a corev1.Node into a plannerapi.NodeInfo object.
func AsNodeInfo(node corev1.Node) plannerapi.NodeInfo {
	return plannerapi.NodeInfo{
		ObjectMeta:    node.ObjectMeta,
		InstanceType:  node.Labels[corev1.LabelInstanceTypeStable],
		Unschedulable: node.Spec.Unschedulable,
		Taints:        node.Spec.Taints,
		Capacity:      node.Status.Capacity,
		Allocatable:   node.Status.Allocatable,
		Conditions:    node.Status.Conditions,
	}
}

// AsNode converts a plannerapi.NodeInfo to a corev1.Node object.
func AsNode(info plannerapi.NodeInfo) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: info.ObjectMeta,
		Spec: corev1.NodeSpec{
			Taints:        info.Taints,
			Unschedulable: info.Unschedulable,
		},
		Status: corev1.NodeStatus{
			Capacity:    info.Capacity,
			Allocatable: info.Allocatable,
			Conditions:  info.Conditions,
		},
	}
}

// BuildAllocatable builds the allocatable resources of a node given its capacity and reserved resources.
func BuildAllocatable(capacity, reservedResources corev1.ResourceList) corev1.ResourceList {
	allocatable := capacity.DeepCopy()
	objutil.SubtractResources(allocatable, reservedResources)
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

// AddNodeLabels adds the node labels for the given NodePlacement, architecture, and hostname to nodeLabels.
func AddNodeLabels(nodeLabels map[string]string, arch string, hostName string, placement sacorev1alpha1.NodePlacement) {
	nodeLabels[corev1.LabelInstanceTypeStable] = placement.InstanceType
	nodeLabels[corev1.LabelArchStable] = arch
	nodeLabels[corev1.LabelTopologyZone] = placement.AvailabilityZone
	nodeLabels[corev1.LabelTopologyRegion] = placement.Region
	nodeLabels[corev1.LabelOSStable] = string(corev1.Linux)
	nodeLabels[corev1.LabelHostname] = hostName
	nodeLabels[apicommon.LabelNodePoolName] = placement.PoolName
	nodeLabels[apicommon.LabelNodeTemplateName] = placement.TemplateName
}

// GetNodePlacement extracts the [sacorev1alpha1.NodePlacement] from a [plannerapi.NodeInfo].
// Returns an error if any required node label is missing.
func GetNodePlacement(n plannerapi.NodeInfo) (sacorev1alpha1.NodePlacement, error) {
	if err := plannerapivalidation.ValidateNodeLabels(n.Labels); err != nil {
		return sacorev1alpha1.NodePlacement{}, err
	}
	return sacorev1alpha1.NodePlacement{
		PoolName:         n.Labels[apicommon.LabelNodePoolName],
		TemplateName:     n.Labels[apicommon.LabelNodeTemplateName],
		InstanceType:     n.InstanceType,
		Region:           n.Labels[corev1.LabelTopologyRegion],
		AvailabilityZone: n.Labels[corev1.LabelTopologyZone],
	}, nil
}

// GetNodeCountByPlacement returns a map from [sacorev1alpha1.NodePlacement] to node count for the given nodes.
// Returns an error if any node is missing required labels.
func GetNodeCountByPlacement(nodes []plannerapi.NodeInfo) (map[sacorev1alpha1.NodePlacement]int32, error) {
	nodeCountByPlacement := make(map[sacorev1alpha1.NodePlacement]int32)
	for _, nodeInfo := range nodes {
		p, err := GetNodePlacement(nodeInfo)
		if err != nil {
			return nil, err
		}
		nodeCountByPlacement[p]++
	}
	return nodeCountByPlacement, nil
}

// NewCSINode returns a fresh CSINode object referring to the node with given name and uid and populated with the given CSISpec
func NewCSINode(nodeName string, nodeUID types.UID, csiNodeSpec storagev1.CSINodeSpec) *storagev1.CSINode {
	return &storagev1.CSINode{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: storagev1.SchemeGroupVersion.Version,
					Kind:       minkapi.CSINodeDescriptor.GetKind(),
					Name:       nodeName,
					UID:        nodeUID,
				},
			},
		},
		Spec: csiNodeSpec,
	}
}
