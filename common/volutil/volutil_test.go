package volutil

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSortPersistentVolumesByIncreasingStorage(t *testing.T) {
	tests := []struct {
		name     string
		input    []corev1.PersistentVolume
		expected []corev1.PersistentVolume
	}{
		{
			name: "already sorted",
			input: []corev1.PersistentVolume{
				{ObjectMeta: meta("pv-10Gi"), Spec: pvWithStorage("10Gi")},
				{ObjectMeta: meta("pv-20Gi"), Spec: pvWithStorage("20Gi")},
				{ObjectMeta: meta("pv-50Gi"), Spec: pvWithStorage("50Gi")},
			},
			expected: []corev1.PersistentVolume{
				{ObjectMeta: meta("pv-10Gi"), Spec: pvWithStorage("10Gi")},
				{ObjectMeta: meta("pv-20Gi"), Spec: pvWithStorage("20Gi")},
				{ObjectMeta: meta("pv-50Gi"), Spec: pvWithStorage("50Gi")},
			},
		},
		{
			name: "reverse sorted → should become ascending",
			input: []corev1.PersistentVolume{
				{ObjectMeta: meta("pv-100Gi"), Spec: pvWithStorage("100Gi")},
				{ObjectMeta: meta("pv-5Gi"), Spec: pvWithStorage("5Gi")},
				{ObjectMeta: meta("pv-25Gi"), Spec: pvWithStorage("25Gi")},
				{ObjectMeta: meta("pv-10Gi"), Spec: pvWithStorage("10Gi")},
			},
			expected: []corev1.PersistentVolume{
				{ObjectMeta: meta("pv-5Gi"), Spec: pvWithStorage("5Gi")},
				{ObjectMeta: meta("pv-10Gi"), Spec: pvWithStorage("10Gi")},
				{ObjectMeta: meta("pv-25Gi"), Spec: pvWithStorage("25Gi")},
				{ObjectMeta: meta("pv-100Gi"), Spec: pvWithStorage("100Gi")},
			},
		},
		{
			name: "equal values should be stable",
			input: []corev1.PersistentVolume{
				{ObjectMeta: meta("pv-a"), Spec: pvWithStorage("30Gi")},
				{ObjectMeta: meta("pv-b"), Spec: pvWithStorage("8Gi")},
				{ObjectMeta: meta("pv-c"), Spec: pvWithStorage("30Gi")},
				{ObjectMeta: meta("pv-d"), Spec: pvWithStorage("30Gi")},
				{ObjectMeta: meta("pv-e"), Spec: pvWithStorage("12Gi")},
			},
			expected: []corev1.PersistentVolume{
				{ObjectMeta: meta("pv-b"), Spec: pvWithStorage("8Gi")},
				{ObjectMeta: meta("pv-e"), Spec: pvWithStorage("12Gi")},
				{ObjectMeta: meta("pv-a"), Spec: pvWithStorage("30Gi")},
				{ObjectMeta: meta("pv-c"), Spec: pvWithStorage("30Gi")},
				{ObjectMeta: meta("pv-d"), Spec: pvWithStorage("30Gi")},
			},
		},
		{
			name: "single element",
			input: []corev1.PersistentVolume{
				{ObjectMeta: meta("pv-single"), Spec: pvWithStorage("42Gi")},
			},
			expected: []corev1.PersistentVolume{
				{ObjectMeta: meta("pv-single"), Spec: pvWithStorage("42Gi")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy because the function sorts in-place
			got := append([]corev1.PersistentVolume(nil), tt.input...)

			SortPersistentVolumesByIncreasingStorage(got)

			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("SortPersistentVolumesByIncreasingStorage() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func meta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: metav1.NamespaceNone}
}

func pvWithStorage(capacity string) corev1.PersistentVolumeSpec {
	q := resource.MustParse(capacity)
	return corev1.PersistentVolumeSpec{
		Capacity: corev1.ResourceList{
			corev1.ResourceStorage: q,
		},
	}
}
