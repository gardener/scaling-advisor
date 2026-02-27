// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package volutil

import (
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
	corev1 "k8s.io/api/core/v1"
)

// AsPVInfo converts the given corev1.PersistentVolume to a lean plannerapi PVInfo.
func AsPVInfo(pv corev1.PersistentVolume) plannerapi.PVInfo {
	return plannerapi.PVInfo{
		AccessModes:      pv.Spec.AccessModes,
		Capacity:         pv.Spec.Capacity,
		ClaimRef:         objutil.NamespacedName(&pv),
		ObjectMeta:       pv.ObjectMeta,
		NodeAffinity:     pv.Spec.NodeAffinity.Required,
		StorageClassName: pv.Spec.StorageClassName,
		Phase:            pv.Status.Phase,
	}
}

// AsPV converts the given plannerapi PVInfo object to a corev1.PersistentVolume
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

// AsPVCInfo converts the given k8s corev1.PersistentVolumeClaim object to a lean planner PVCInfo.
func AsPVCInfo(pvc corev1.PersistentVolumeClaim) plannerapi.PVCInfo {
	return plannerapi.PVCInfo{
		ObjectMeta:                pvc.ObjectMeta,
		PersistentVolumeClaimSpec: pvc.Spec,
		Phase:                     pvc.Status.Phase,
	}
}

// AsPVC converts the given plannerapi PVCInfo object to a corev1.PersistentVolumeClaim
func AsPVC(p plannerapi.PVCInfo) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: p.ObjectMeta,
		Spec:       p.PersistentVolumeClaimSpec,
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: p.Phase,
		},
	}
}
