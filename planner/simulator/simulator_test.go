package simulator

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestIsPVBindCandidate(t *testing.T) {
	tests := []struct {
		name             string
		pvc              *corev1.PersistentVolumeClaim
		pv               *corev1.PersistentVolume
		storageClassName string
		want             bool
	}{
		{
			name:             "PV not Available → false",
			pvc:              newPVC("10Gi", corev1.ReadWriteOnce),
			pv:               newPV("10Gi", corev1.VolumePending, "standard"),
			storageClassName: "standard",
			want:             false,
		},
		{
			name:             "different storage class → false",
			pvc:              newPVC("10Gi", corev1.ReadWriteOnce),
			pv:               newPV("10Gi", corev1.VolumeAvailable, "premium"),
			storageClassName: "standard",
			want:             false,
		},
		{
			name:             "PV smaller than requested → false",
			pvc:              newPVC("20Gi", corev1.ReadWriteOnce),
			pv:               newPV("10Gi", corev1.VolumeAvailable, "standard"),
			storageClassName: "standard",
			want:             false,
		},
		{
			name:             "incompatible access modes → false",
			pvc:              newPVCWithAccessModes("10Gi", []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}),
			pv:               newPVWithAccessModes("15Gi", corev1.VolumeAvailable, "standard", []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}),
			storageClassName: "standard",
			want:             false,
		},
		{
			name:             "volume mode mismatch Filesystem vs Block → false",
			pvc:              newPVCWithVolumeMode("10Gi", corev1.ReadWriteOnce, corev1.PersistentVolumeBlock),
			pv:               newPVWithVolumeMode("10Gi", corev1.VolumeAvailable, "standard", corev1.PersistentVolumeFilesystem),
			storageClassName: "standard",
			want:             false,
		},
		{
			name:             "PVC has nil VolumeMode → treated as Filesystem",
			pvc:              newPVCWithNilVolumeMode("10Gi", corev1.ReadWriteOnce),
			pv:               newPV("10Gi", corev1.VolumeAvailable, "standard"),
			storageClassName: "standard",
			want:             true,
		},
		{
			name:             "happy path - exact match",
			pvc:              newPVC("10Gi", corev1.ReadWriteOnce),
			pv:               newPV("15Gi", corev1.VolumeAvailable, "standard"),
			storageClassName: "standard",
			want:             true,
		},
		{
			name:             "PV larger than requested is allowed",
			pvc:              newPVC("8Gi", corev1.ReadWriteOnce),
			pv:               newPV("100Gi", corev1.VolumeAvailable, "standard"),
			storageClassName: "standard",
			want:             true,
		},
		{
			name: "PVC selector does not match PV labels → false",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "pvc-sel"},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("10Gi"),
						},
					},
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"disktype": "ssd"},
					},
				},
			},
			pv:               newPVWithLabels("10Gi", corev1.VolumeAvailable, "standard", map[string]string{"disktype": "hdd"}),
			storageClassName: "standard",
			want:             false,
		},
		{
			name: "PVC selector matches PV labels → true",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "pvc-sel-ok"},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("10Gi"),
						},
					},
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"disktype": "ssd", "env": "prod"},
					},
				},
			},
			pv:               newPVWithLabels("12Gi", corev1.VolumeAvailable, "standard", map[string]string{"disktype": "ssd", "env": "prod", "region": "eu"}),
			storageClassName: "standard",
			want:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			got := IsPVBindCandidate(ctx, tt.pvc, tt.pv, tt.storageClassName)
			if tt.want != got {
				t.Errorf("IsPVBindCandidate, got = %v, want %v", got, tt.want)
			}
		})
	}
}

// ────────────────────────────────────────────────
// Helper constructors (updated to use VolumeResourceRequirements)
// ────────────────────────────────────────────────

func newPVC(size string, modes ...corev1.PersistentVolumeAccessMode) *corev1.PersistentVolumeClaim {
	qty := resource.MustParse(size)
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To("standard"),
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: qty,
				},
			},
			AccessModes: modes,
		},
	}
}

func newPVCWithAccessModes(size string, modes []corev1.PersistentVolumeAccessMode) *corev1.PersistentVolumeClaim {
	pvc := newPVC(size)
	pvc.Spec.AccessModes = modes
	return pvc
}

func newPVCWithVolumeMode(size string, accessMode corev1.PersistentVolumeAccessMode, volMode corev1.PersistentVolumeMode) *corev1.PersistentVolumeClaim {
	pvc := newPVC(size)
	pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{accessMode}
	pvc.Spec.VolumeMode = &volMode
	return pvc
}

func newPVCWithNilVolumeMode(size string, accessMode corev1.PersistentVolumeAccessMode) *corev1.PersistentVolumeClaim {
	pvc := newPVC(size)
	pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{accessMode}
	pvc.Spec.VolumeMode = nil
	return pvc
}

func newPV(size string, phase corev1.PersistentVolumePhase, scName string) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv"},
		Spec: corev1.PersistentVolumeSpec{
			StorageClassName: scName,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(size),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: phase,
		},
	}
}

func newPVWithAccessModes(size string, phase corev1.PersistentVolumePhase, scName string, modes []corev1.PersistentVolumeAccessMode) *corev1.PersistentVolume {
	pv := newPV(size, phase, scName)
	pv.Spec.AccessModes = modes
	return pv
}

func newPVWithVolumeMode(size string, phase corev1.PersistentVolumePhase, scName string, volMode corev1.PersistentVolumeMode) *corev1.PersistentVolume {
	pv := newPV(size, phase, scName)
	pv.Spec.VolumeMode = &volMode
	return pv
}

func newPVWithLabels(size string, phase corev1.PersistentVolumePhase, scName string, labels map[string]string) *corev1.PersistentVolume {
	pv := newPV(size, phase, scName)
	pv.Labels = labels
	return pv
}
