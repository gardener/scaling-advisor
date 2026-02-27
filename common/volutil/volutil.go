// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package volutil

import (
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	storageutil "k8s.io/kubernetes/pkg/apis/storage/util"
)

// AsPVInfo converts the given corev1.PersistentVolume to a lean plannerapi PVInfo.
func AsPVInfo(pv corev1.PersistentVolume) plannerapi.PVInfo {
	pvi := plannerapi.PVInfo{
		AccessModes:      pv.Spec.AccessModes,
		Capacity:         pv.Spec.Capacity,
		ObjectMeta:       pv.ObjectMeta,
		NodeAffinity:     pv.Spec.NodeAffinity.Required,
		StorageClassName: pv.Spec.StorageClassName,
		Phase:            pv.Status.Phase,
	}
	if pv.Spec.ClaimRef != nil {
		pvi.ClaimRef.Namespace = pv.Spec.ClaimRef.Namespace
		pvi.ClaimRef.Name = pv.Spec.ClaimRef.Name
	}
	if pv.Spec.VolumeMode != nil {
		pvi.VolumeMode = *pv.Spec.VolumeMode
	} else {
		pvi.VolumeMode = corev1.PersistentVolumeFilesystem // default according to k8s
	}
	return pvi
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

// GetDefaultStorageClass gets the `StorageClass` annotated with the "storageclass.kubernetes.io/is-default-class"
// annotation in the given classses slice or nil if no such StoreClasss is found
func GetDefaultStorageClass(classes []storagev1.StorageClass) *storagev1.StorageClass {
	for _, sc := range classes {
		if storageutil.IsDefaultAnnotation(sc.ObjectMeta) {
			return &sc
		}
	}
	return nil
}

// FindStorageClassWithName finds a storage class with the given name in the given slice of `StorageClass`s or nil if not found
func FindStorageClassWithName(name string, classes []storagev1.StorageClass) *storagev1.StorageClass {
	for _, sc := range classes {
		if sc.Name == name {
			return &sc
		}
	}
	return nil
}
