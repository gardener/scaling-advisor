// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package podutil

import (
	"slices"

	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

// AsPod converts a planner.PodInfo to a corev1.Pod object.
func AsPod(info planner.PodInfo) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            info.Name,
			Namespace:       info.Namespace,
			Labels:          info.Labels,
			Annotations:     info.Annotations,
			UID:             info.UID,
			OwnerReferences: info.OwnerReferences,
		},
		Spec: corev1.PodSpec{
			Volumes:                   info.Volumes,
			NodeSelector:              info.NodeSelector,
			NodeName:                  info.NodeName,
			Affinity:                  info.Affinity,
			SchedulerName:             info.SchedulerName,
			Tolerations:               info.Tolerations,
			PriorityClassName:         info.PriorityClassName,
			Priority:                  info.Priority,
			RuntimeClassName:          info.RuntimeClassName,
			PreemptionPolicy:          info.PreemptionPolicy,
			Overhead:                  objutil.Int64MapToResourceList(info.Overhead),
			TopologySpreadConstraints: info.TopologySpreadConstraints,
			ResourceClaims:            info.ResourceClaims,
			Containers: []corev1.Container{
				{
					Name: info.Name + "-aggregated-container",
					Resources: corev1.ResourceRequirements{
						Requests: objutil.Int64MapToResourceList(info.AggregatedRequests),
					},
				},
			},
		},
	}
}

// PodResourceInfosFromPodInfo extracts the AggregatedRequests for each pod
// from podInfos alongwith its identification into a PodResourceInfo slice.
func PodResourceInfosFromPodInfo(podInfos []planner.PodInfo) []planner.PodResourceInfo {
	podResourceInfos := make([]planner.PodResourceInfo, 0, len(podInfos))
	for _, podInfo := range podInfos {
		podResourceInfos = append(podResourceInfos, planner.PodResourceInfo{
			UID:                podInfo.UID,
			NamespacedName:     podInfo.NamespacedName,
			AggregatedRequests: podInfo.AggregatedRequests,
		})
	}
	return podResourceInfos
}

// PodResourceInfosFromCoreV1Pods extracts the AggregatedRequests for each pod
// from a corev1 Pod slice alongwith its identification into a PodResourceInfo slice.
func PodResourceInfosFromCoreV1Pods(pods []corev1.Pod) []planner.PodResourceInfo {
	podResourceInfos := make([]planner.PodResourceInfo, 0, len(pods))
	for _, p := range pods {
		podResourceInfos = append(podResourceInfos, PodResourceInfoFromCoreV1Pod(&p))
	}
	return podResourceInfos
}

// PodResourceInfoFromCoreV1Pod extracts the AggregatedRequests for a single
// corev1 pod resource alongwith its identification into a PodResourceInfo object.
func PodResourceInfoFromCoreV1Pod(p *corev1.Pod) planner.PodResourceInfo {
	return planner.PodResourceInfo{
		UID:                p.UID,
		NamespacedName:     types.NamespacedName{Namespace: p.Namespace, Name: p.Name},
		AggregatedRequests: AggregatePodRequests(p),
	}
}

// AggregatePodRequests computes the sum of resource requirements
// for all the init containers and containers present in a pod.
func AggregatePodRequests(p *corev1.Pod) map[corev1.ResourceName]int64 {
	aggregate := map[corev1.ResourceName]int64{}
	containers := slices.AppendSeq(p.Spec.InitContainers, slices.Values(p.Spec.Containers))
	for _, c := range containers {
		cmap := objutil.ResourceListToInt64Map(c.Resources.Requests)
		for k, v := range cmap {
			aggregate[k] += v
		}
	}
	return aggregate
}

// GetObjectNamesFromPodResourceInfos maps a slice of PodResourceInfo to pod names of the form "namespace/name"
func GetObjectNamesFromPodResourceInfos(pods []planner.PodResourceInfo) []string {
	objectNames := make([]string, 0, len(pods))
	for _, pod := range pods {
		objectNames = append(objectNames, pod.String())
	}
	return objectNames
}

// AsPodInfo converts a corev1.Pod to a planner.PodInfo object.
func AsPodInfo(pod corev1.Pod) planner.PodInfo {
	return planner.PodInfo{
		ResourceMeta: planner.ResourceMeta{
			UID:               pod.UID,
			NamespacedName:    types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace},
			Labels:            pod.Labels,
			Annotations:       pod.Annotations,
			DeletionTimestamp: pod.DeletionTimestamp,
			OwnerReferences:   pod.OwnerReferences,
		},
		AggregatedRequests:        AggregatePodRequests(&pod),
		Volumes:                   pod.Spec.Volumes,
		NodeSelector:              pod.Spec.NodeSelector,
		NodeName:                  pod.Spec.NodeName,
		Affinity:                  pod.Spec.Affinity,
		SchedulerName:             pod.Spec.SchedulerName,
		Tolerations:               pod.Spec.Tolerations,
		PriorityClassName:         pod.Spec.PriorityClassName,
		Priority:                  pod.Spec.Priority,
		PreemptionPolicy:          pod.Spec.PreemptionPolicy,
		RuntimeClassName:          pod.Spec.RuntimeClassName,
		Overhead:                  objutil.ResourceListToInt64Map(pod.Spec.Overhead),
		TopologySpreadConstraints: pod.Spec.TopologySpreadConstraints,
		ResourceClaims:            pod.Spec.ResourceClaims,
	}
}
