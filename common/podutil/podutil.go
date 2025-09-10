// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package podutil

import (
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/objutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"slices"
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

// AsPod converts a svcapi.PodInfo to a corev1.Pod object.
func AsPod(info svcapi.PodInfo) *corev1.Pod {
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
func PodResourceInfosFromPodInfo(podInfos []svcapi.PodInfo) []svcapi.PodResourceInfo {
	podResourceInfos := make([]svcapi.PodResourceInfo, 0, len(podInfos))
	for _, podInfo := range podInfos {
		podResourceInfos = append(podResourceInfos, svcapi.PodResourceInfo{
			UID:                podInfo.UID,
			NamespacedName:     podInfo.NamespacedName,
			AggregatedRequests: podInfo.AggregatedRequests,
		})
	}
	return podResourceInfos
}

func PodResourceInfosFromCoreV1Pods(pods []corev1.Pod) []svcapi.PodResourceInfo {
	podResourceInfos := make([]svcapi.PodResourceInfo, 0, len(pods))
	for _, p := range pods {
		podResourceInfos = append(podResourceInfos, PodResourceInfoFromCoreV1Pod(&p))
	}
	return podResourceInfos
}

func PodResourceInfoFromCoreV1Pod(p *corev1.Pod) svcapi.PodResourceInfo {
	return svcapi.PodResourceInfo{
		UID:                p.UID,
		NamespacedName:     types.NamespacedName{Namespace: p.Namespace, Name: p.Name},
		AggregatedRequests: AggregatePodRequests(p),
	}
}

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
