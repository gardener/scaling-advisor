// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package volutil

import (
	"slices"

	"github.com/gardener/scaling-advisor/common/objutil"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	storageutil "k8s.io/kubernetes/pkg/apis/storage/util"
)

// GetDefaultStorageClass gets the `StorageClass` annotated with the "storageclass.kubernetes.io/is-default-class"
// annotation in the given classses slice or nil if no such StoreClasss is found
func GetDefaultStorageClass(classes []*storagev1.StorageClass) *storagev1.StorageClass {
	objutil.SortByDecreasingCreationTime(classes)
	for _, sc := range classes {
		if storageutil.IsDefaultAnnotation(sc.ObjectMeta) {
			return sc
		}
	}
	return nil
}

// FindStorageClassWithName finds a storage class with the given name in the given slice of `StorageClass`s or nil if not found
func FindStorageClassWithName(name string, classes []*storagev1.StorageClass) *storagev1.StorageClass {
	for _, sc := range classes {
		if sc.Name == name {
			return sc
		}
	}
	return nil
}

// SortPVCByIncreasingStorage sorts the given slice of `PersistentVolumeClaim`s by increasing storage capacity.
func SortPVCByIncreasingStorage(pvcs []*corev1.PersistentVolumeClaim) {
	slices.SortFunc(pvcs, func(a, b *corev1.PersistentVolumeClaim) int {
		return a.Spec.Resources.Requests.Storage().Cmp(*b.Spec.Resources.Requests.Storage())
	})
}
