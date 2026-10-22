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

// GetNodeUtilization returns the utilization ratio per resource for the given node based on the total
// requests of the pods running on it. Each value is computed as sum(pod requests) / node.Allocatable
// and is clamped to [0, 1]. Resources not present in the node's Allocatable are omitted.
func GetNodeUtilization(node corev1.Node, pods []corev1.Pod) map[corev1.ResourceName]float64 {
	totalRequests := corev1.ResourceList{}
	for i := range pods {
		for _, c := range pods[i].Spec.InitContainers {
			for res, qty := range c.Resources.Requests {
				sum := totalRequests[res]
				sum.Add(qty)
				totalRequests[res] = sum
			}
		}
		for _, c := range pods[i].Spec.Containers {
			for res, qty := range c.Resources.Requests {
				sum := totalRequests[res]
				sum.Add(qty)
				totalRequests[res] = sum
			}
		}
	}
	utilization := make(map[corev1.ResourceName]float64, len(node.Status.Allocatable))
	for res, allocatable := range node.Status.Allocatable {
		allocMillis := allocatable.MilliValue()
		if allocMillis <= 0 {
			continue
		}
		used := totalRequests[res]
		ratio := float64(used.MilliValue()) / float64(allocMillis)
		if ratio > 1 {
			ratio = 1
		}
		utilization[res] = ratio
	}
	return utilization
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
