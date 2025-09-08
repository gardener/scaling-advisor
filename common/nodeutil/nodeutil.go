// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package nodeutil

import (
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/objutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

	"github.com/gardener/scaling-advisor/common/objutil"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
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
