// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package helper

import (
	"slices"

	plannerapi "github.com/gardener/scaling-advisor/planner/api"
	plannerapivalidation "github.com/gardener/scaling-advisor/planner/api/validation"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	simulationapi "github.com/gardener/scaling-advisor/simulation/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

// ── Node helpers ─────────────────────────────────────────────────────────────

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

// ── Pod helpers ──────────────────────────────────────────────────────────────

// AsPodInfo converts a corev1.Pod to a plannerapi.PodInfo object.
func AsPodInfo(pod corev1.Pod) plannerapi.PodInfo {
	return plannerapi.PodInfo{
		ObjectMeta:                pod.ObjectMeta,
		AggregatedRequests:        AggregatePodRequests(&pod),
		Volumes:                   pod.Spec.Volumes,
		NodeSelector:              pod.Spec.NodeSelector,
		NodeName:                  pod.Spec.NodeName,
		Affinity:                  pod.Spec.Affinity,
		SchedulerName:             pod.Spec.SchedulerName,
		Tolerations:               pod.Spec.Tolerations,
		PriorityClassName:         pod.Spec.PriorityClassName,
		Priority:                  ptr.Deref(pod.Spec.Priority, 0),
		PreemptionPolicy:          ptr.Deref(pod.Spec.PreemptionPolicy, ""),
		RuntimeClassName:          ptr.Deref(pod.Spec.RuntimeClassName, ""),
		Overhead:                  pod.Spec.Overhead,
		TopologySpreadConstraints: pod.Spec.TopologySpreadConstraints,
		ResourceClaims:            pod.Spec.ResourceClaims,
	}
}

// AsPod converts a plannerapi.PodInfo to a corev1.Pod object.
func AsPod(info plannerapi.PodInfo) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: info.ObjectMeta,
		Spec: corev1.PodSpec{
			Volumes:                   info.Volumes,
			NodeSelector:              info.NodeSelector,
			NodeName:                  info.NodeName,
			Affinity:                  info.Affinity,
			SchedulerName:             info.SchedulerName,
			Tolerations:               info.Tolerations,
			PriorityClassName:         info.PriorityClassName,
			Priority:                  ptr.To(info.Priority),
			RuntimeClassName:          ptr.To(info.RuntimeClassName),
			PreemptionPolicy:          ptr.To(info.PreemptionPolicy),
			Overhead:                  info.Overhead,
			TopologySpreadConstraints: info.TopologySpreadConstraints,
			ResourceClaims:            info.ResourceClaims,
			Containers: []corev1.Container{
				{
					Name: info.Name + "-aggregated-container",
					Resources: corev1.ResourceRequirements{
						Requests: info.AggregatedRequests,
					},
				},
			},
		},
	}
}

// PodInfosFromCoreV1Pods converts the given slice of corev1.Pod objects into a slice of plannerapi.PodInfo objects.
func PodInfosFromCoreV1Pods(pods []corev1.Pod) []plannerapi.PodInfo {
	podInfos := make([]plannerapi.PodInfo, 0, len(pods))
	for _, p := range pods {
		podInfos = append(podInfos, AsPodInfo(p))
	}
	return podInfos
}

// GetUnscheduledPods returns all pods in the given slice that are not scheduled to any node.
func GetUnscheduledPods(pods []plannerapi.PodInfo) []plannerapi.PodInfo {
	var unscheduled []plannerapi.PodInfo
	for _, pod := range pods {
		if pod.NodeName == "" {
			unscheduled = append(unscheduled, pod)
		}
	}
	return unscheduled
}

// AggregatePodRequests computes the sum of resource requirements
// for all the init containers and containers present in a pod.
func AggregatePodRequests(p *corev1.Pod) map[corev1.ResourceName]resource.Quantity {
	aggregate := map[corev1.ResourceName]resource.Quantity{}
	for _, c := range slices.AppendSeq(p.Spec.InitContainers, slices.Values(p.Spec.Containers)) {
		for k, v := range c.Resources.Requests {
			current := aggregate[k]
			current.Add(v)
			aggregate[k] = current
		}
	}
	return aggregate
}

// PodResourceInfoFromCoreV1Pod extracts the AggregatedRequests for a single
// corev1.Pod along with its identification into a simulationapi.PodResourceInfo object.
func PodResourceInfoFromCoreV1Pod(p *corev1.Pod) simulationapi.PodResourceInfo {
	return simulationapi.PodResourceInfo{
		NamespacedName:     apicommon.NamespacedName{Namespace: p.Namespace, Name: p.Name},
		AggregatedRequests: AggregatePodRequests(p),
	}
}

// PodResourceInfosFromCoreV1Pods converts a corev1.Pod slice to a simulationapi.PodResourceInfo slice.
func PodResourceInfosFromCoreV1Pods(pods []corev1.Pod) []simulationapi.PodResourceInfo {
	podResourceInfos := make([]simulationapi.PodResourceInfo, 0, len(pods))
	for _, p := range pods {
		podResourceInfos = append(podResourceInfos, PodResourceInfoFromCoreV1Pod(&p))
	}
	return podResourceInfos
}

// PodResourceInfosFromPodInfo converts a plannerapi.PodInfo slice to a leaner
// simulationapi.PodResourceInfo slice retaining only name, namespace and aggregated requests.
func PodResourceInfosFromPodInfo(podInfos []plannerapi.PodInfo) []simulationapi.PodResourceInfo {
	podResourceInfos := make([]simulationapi.PodResourceInfo, 0, len(podInfos))
	for _, podInfo := range podInfos {
		podResourceInfos = append(podResourceInfos, simulationapi.PodResourceInfo{
			NamespacedName:     apicommon.NamespacedName{Namespace: podInfo.Namespace, Name: podInfo.Name},
			AggregatedRequests: podInfo.AggregatedRequests,
		})
	}
	return podResourceInfos
}

// GetObjectNamesFromPodResourceInfos maps a slice of PodResourceInfo to pod names of the form "namespace/name".
func GetObjectNamesFromPodResourceInfos(pods []simulationapi.PodResourceInfo) []string {
	objectNames := make([]string, 0, len(pods))
	for _, pod := range pods {
		objectNames = append(objectNames, pod.String())
	}
	return objectNames
}

// ── Volume helpers ───────────────────────────────────────────────────────────

// AsPVInfo converts the given corev1.PersistentVolume to a lean plannerapi.PVInfo.
func AsPVInfo(pv corev1.PersistentVolume) plannerapi.PVInfo {
	pvi := plannerapi.PVInfo{
		AccessModes:      pv.Spec.AccessModes,
		Capacity:         pv.Spec.Capacity,
		ObjectMeta:       pv.ObjectMeta,
		StorageClassName: pv.Spec.StorageClassName,
		Phase:            pv.Status.Phase,
	}
	if pv.Spec.NodeAffinity != nil {
		pvi.NodeAffinity = pv.Spec.NodeAffinity.Required
	}
	if pv.Spec.ClaimRef != nil {
		pvi.ClaimRef.Namespace = pv.Spec.ClaimRef.Namespace
		pvi.ClaimRef.Name = pv.Spec.ClaimRef.Name
	}
	if pv.Spec.VolumeMode != nil {
		pvi.VolumeMode = *pv.Spec.VolumeMode
	} else {
		pvi.VolumeMode = corev1.PersistentVolumeFilesystem
	}
	return pvi
}

// AsPV converts the given plannerapi.PVInfo to a corev1.PersistentVolume.
func AsPV(p plannerapi.PVInfo) *corev1.PersistentVolume {
	var volNodeAffinity *corev1.VolumeNodeAffinity
	if p.NodeAffinity != nil {
		volNodeAffinity = &corev1.VolumeNodeAffinity{Required: p.NodeAffinity}
	}
	return &corev1.PersistentVolume{
		ObjectMeta: p.ObjectMeta,
		Spec: corev1.PersistentVolumeSpec{
			AccessModes:                   p.AccessModes,
			Capacity:                      p.Capacity,
			ClaimRef:                      p.ClaimRef.AsObjectReference(),
			NodeAffinity:                  volNodeAffinity,
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			StorageClassName:              p.StorageClassName,
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: p.Phase,
		},
	}
}

// AsPVCInfo converts the given corev1.PersistentVolumeClaim to a lean plannerapi.PVCInfo.
func AsPVCInfo(pvc corev1.PersistentVolumeClaim) plannerapi.PVCInfo {
	return plannerapi.PVCInfo{
		ObjectMeta:                pvc.ObjectMeta,
		PersistentVolumeClaimSpec: pvc.Spec,
		Phase:                     pvc.Status.Phase,
	}
}

// AsPVC converts the given plannerapi.PVCInfo to a corev1.PersistentVolumeClaim.
func AsPVC(p plannerapi.PVCInfo) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: p.ObjectMeta,
		Spec:       p.PersistentVolumeClaimSpec,
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: p.Phase,
		},
	}
}
