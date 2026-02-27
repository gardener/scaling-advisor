// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	"testing"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	commontestutil "github.com/gardener/scaling-advisor/common/testutil"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGeneratePVCs(t *testing.T) {
	wantNS := "test"
	wantStorage := resource.MustParse("1Gi")
	wantAccessMode := corev1.ReadWriteOnce
	wantNames := []string{"stem", "branch"}
	wantPhase := corev1.ClaimBound
	testGenDir, ok := commontestutil.CreateTestGenDir(t)
	if !ok {
		return
	}
	vg := VolGenInput{
		Namespace:  wantNS,
		Storage:    wantStorage,
		AccessMode: wantAccessMode,
		PVCNames:   wantNames,
		Provider:   commontypes.CloudProviderAWS,
		ClaimPhase: corev1.ClaimBound,
	}
	out, err := GeneratePersistentVolumeClaims(testGenDir, vg)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(out.PVCs) != len(out.YAMLPaths) {
		t.Fatalf("mismatch: returned %d PVCs and %d YAMLPaths", len(out.PVCs), len(out.YAMLPaths))
	}
	for i, pvc := range out.PVCs {
		gotName := pvc.Name
		wantName := wantNames[i]
		if gotName != wantName {
			t.Errorf("gotName %q, wantName %q", gotName, wantName)
		}
		gotStorage := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if gotStorage.Cmp(wantStorage) != 0 {
			t.Errorf("pvcName %q, gotStorage %q, wantStorage %q", pvc.Name, gotStorage.String(), wantStorage.String())
		}
		gotAccessMode := pvc.Spec.AccessModes[0]
		if gotAccessMode != wantAccessMode {
			t.Errorf("pvcName %q, gotAccessMode %q, wantAccessMode %q", pvc.Name, gotAccessMode, wantAccessMode)
		}
		gotPhase := pvc.Status.Phase
		if gotPhase != wantPhase {
			t.Errorf("pvcName %q, gotPhase %q, wantPhase %q", pvc.Name, gotPhase, wantPhase)
		}
	}
}

func TestGeneratePVs(t *testing.T) {
	wantNS := "test"
	wantStorage := resource.MustParse("1Gi")
	wantAccessMode := corev1.ReadWriteOnce
	wantZone := "eu-west-1a"
	pvcNames := []string{"stem", "branch"}
	testGenDir, ok := commontestutil.CreateTestGenDir(t)
	if !ok {
		return
	}
	out, err := GeneratePersistentVolumes(testGenDir, VolGenInput{
		Namespace:  wantNS,
		Storage:    wantStorage,
		AccessMode: wantAccessMode,
		PVCNames:   pvcNames,
		Provider:   commontypes.CloudProviderAWS,
		PVZones:    []string{wantZone},
		ClaimPhase: corev1.ClaimBound,
	})
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(out.PVs) != len(out.YAMLPaths) {
		t.Fatalf("mismatch: returned %d PVs and %d YAMLPaths", len(out.PVs), len(out.YAMLPaths))
	}
	for _, pv := range out.PVs {
		gotStorage := pv.Spec.Capacity[corev1.ResourceStorage]
		if gotStorage.Cmp(wantStorage) != 0 {
			t.Errorf("pv %q, gotStorage %q, wantStorage %q", pv.Name, gotStorage.String(), wantStorage.String())
		}
		gotZone := pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Values[0]
		if gotZone != wantZone {
			t.Errorf("pv %q, gotZone %q, wantZone %q", pv.Name, gotZone, wantZone)
		}
		gotAccessMode := pv.Spec.AccessModes[0]
		if gotAccessMode != wantAccessMode {
			t.Errorf("pv %q, gotAccessMode %q, wantAccessMode %q", pv.Name, gotAccessMode, wantAccessMode)
		}
	}
}

func TestGenerateStorageClass(t *testing.T) {
	wantName := "default"
	wantVolumeBindingMode := storagev1.VolumeBindingImmediate
	testGenDir, ok := commontestutil.CreateTestGenDir(t)
	if !ok {
		return
	}
	sc, _, err := GenerateDefaultStorageClass(testGenDir, commontypes.CloudProviderAWS, wantName, wantVolumeBindingMode)
	if err != nil {
		t.Fatalf("GenerateDefaultStorageClass() failed: %v", err)
	}
	gotVolumeBindingMode := sc.VolumeBindingMode
	if gotVolumeBindingMode == nil {
		t.Fatalf("GenerateDefaultStorageClass gotVolumeBindingMode is nil")
	}
	if *gotVolumeBindingMode != wantVolumeBindingMode {
		t.Errorf("GenerateDefaultStorageClass gotVolumeBindingMode %q, wantVolumeBindingMode %q", *gotVolumeBindingMode, wantVolumeBindingMode)
	}
	if sc.Name != wantName {
		t.Errorf("GenerateDefaultStorageClass gotName %q, wantName %q", sc.Name, wantName)
	}
}

func TestGenerateSimplePodsWithResources(t *testing.T) {
	podCount := 4
	pvcNames := []string{"stem", "branch"}
	testGenDir, ok := commontestutil.CreateTestGenDir(t)
	if !ok {
		return
	}
	for _, resourceCategory := range allResourcePresets {
		t.Run(string(resourceCategory), func(t *testing.T) {
			out, err := GenerateSimplePodsForResourcePreset(resourceCategory, podCount, PodGenInput{
				Name: string(resourceCategory),
				AppLabels: AppLabels{
					Name:      string(resourceCategory),
					Component: "fruit",
					PartOf:    "food",
					ManagedBy: "logistics",
				},
				GenDir:   testGenDir,
				PVCNames: pvcNames,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Pods) != podCount {
				t.Errorf("expecting %d pods for %q, got %d", podCount, resourceCategory, len(out.Pods))
			}
			if len(out.YAMLPaths) != podCount {
				t.Errorf("expecting %d paths for %q, got %d", podCount, resourceCategory, len(out.YAMLPaths))
			}

			want := resourceCategory.AsResourceList()
			for _, p := range out.Pods {
				if len(p.Spec.Containers) == 0 {
					t.Fatalf("pod %q has no containers", p.Name)
				}
				container := p.Spec.Containers[0]
				if len(container.Resources.Requests) == 0 {
					t.Fatalf("pod %q has no resources", p.Name)
				}
				got := container.Resources.Requests
				diff := cmp.Diff(want, got,
					cmp.Comparer(func(a, b resource.Quantity) bool {
						return a.Equal(b)
					}),
				)
				if diff != "" {
					t.Errorf("ResourceList mismatch for %q (-want +got):\n%s", resourceCategory, diff)
				}
			}
		})
	}
}
