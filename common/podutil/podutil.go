// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package podutil

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpdatePodCondition updates existing pod condition or creates a new one. Sets LastTransitionTime to now if the
// status has changed.
// Returns true if pod condition has changed or has been added.
func UpdatePodCondition(status *corev1.PodStatus, condition *corev1.PodCondition) bool {
	condition.LastTransitionTime = metav1.Now()
	// Try to find this pod condition.
	conditionIndex, oldCondition := GetPodCondition(status, condition.Type)

	if oldCondition == nil {
		// We are adding new pod condition.
		status.Conditions = append(status.Conditions, *condition)
		return true
	}
	// We are updating an existing condition, so we need to check if it has changed.
	if condition.Status == oldCondition.Status {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	}

	isEqual := condition.Status == oldCondition.Status &&
		condition.Reason == oldCondition.Reason &&
		condition.Message == oldCondition.Message &&
		condition.LastProbeTime.Equal(&oldCondition.LastProbeTime) &&
		condition.LastTransitionTime.Equal(&oldCondition.LastTransitionTime)

	status.Conditions[conditionIndex] = *condition
	// Return true if one of the fields have changed.
	return !isEqual
}

// GetPodCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func GetPodCondition(status *corev1.PodStatus, conditionType corev1.PodConditionType) (int, *corev1.PodCondition) {
	if status == nil {
		return -1, nil
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return i, &status.Conditions[i]
		}
	}
	return -1, nil
}

// AggregatePodRequests computes the sum of resource requirements
// for all the init containers and containers present in a pod.
func AggregatePodRequests(p *corev1.Pod) map[corev1.ResourceName]resource.Quantity {
	aggregate := map[corev1.ResourceName]resource.Quantity{}
	containers := slices.AppendSeq(p.Spec.InitContainers, slices.Values(p.Spec.Containers))
	for _, c := range containers {
		for k, v := range c.Resources.Requests {
			current := aggregate[k]
			current.Add(v)
			aggregate[k] = current
		}
	}
	return aggregate
}

// GetPVCNames returns the slice of PVC Names for the given pod
func GetPVCNames(pod corev1.Pod) []string {
	pvcNames := make([]string, 0, len(pod.Spec.Volumes))
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			pvcNames = append(pvcNames, vol.PersistentVolumeClaim.ClaimName)
		}
	}
	return pvcNames
}

// CountUnscheduledPods returns the count of unscheduled pods in the given slice of pods
func CountUnscheduledPods(pods []corev1.Pod) (count int) {
	for _, p := range pods {
		if IsUnscheduledPod(&p) {
			count++
		}
	}
	return
}

// IsUnscheduledPod determines if a given pod is unscheduled by checking if the NodeName in its spec is empty.
func IsUnscheduledPod(pod *corev1.Pod) bool {
	return pod.Spec.NodeName == ""
}
