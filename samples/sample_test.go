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
	vg := StorageVolGenInput{
		GenDir:            testGenDir,
		Namespace:         wantNS,
		Storage:           wantStorage,
		AccessMode:        wantAccessMode,
		PVCNames:          wantNames,
		Provider:          commontypes.CloudProviderAWS,
		VolumeBindingMode: storagev1.VolumeBindingImmediate,
	}
	pvcs, pvcYAMLPaths, err := GeneratePersistentVolumeClaims(vg)
	if err != nil {
		t.Fatalf("GeneratePersistentVolumes() failed: %v", err)
	}
	if len(pvcs) != len(pvcYAMLPaths) {
		t.Fatalf("GeneratePersistentVolumes() returned %d PVs, expected %d", len(pvcs), len(pvcYAMLPaths))
	}
	for i := range pvcs {
		pvc := pvcs[i]
		gotName := pvc.Name
		wantName := wantNames[i]
		if gotName != wantName {
			t.Errorf("GeneratePersistentVolumes gotName %q, wantName %q", gotName, wantName)
		}
		gotStorage := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if gotStorage.Cmp(wantStorage) != 0 {
			t.Errorf("GeneratePersistentVolumes gotStorage %q, wantStorage %q", gotStorage.String(), wantStorage.String())
		}
		gotAccessMode := pvc.Spec.AccessModes[0]
		if gotAccessMode != wantAccessMode {
			t.Errorf("GeneratePersistentVolumes gotAccessMode %q, wantAccessMode %q", gotAccessMode, wantAccessMode)
		}
		gotPhase := pvc.Status.Phase
		if gotPhase != wantPhase {
			t.Errorf("GeneratePersistentVolumes gotPhase %q, wantPhase %q", gotPhase, wantPhase)
		}
	}
}

func TestGeneratePVs(t *testing.T) {
	wantNS := "test"
	wantStorage := resource.MustParse("1Gi")
	wantAccessMode := corev1.ReadWriteOnce
	wantZone := "eu-west-1a"
	wantNames := []string{"stem", "branch"}
	testGenDir, ok := commontestutil.CreateTestGenDir(t)
	if !ok {
		return
	}
	pvs, pvYAMLPaths, err := GeneratePersistentVolumes(StorageVolGenInput{
		GenDir:            testGenDir,
		Namespace:         wantNS,
		Storage:           wantStorage,
		AccessMode:        wantAccessMode,
		PVCNames:          wantNames,
		Provider:          commontypes.CloudProviderAWS,
		PVZones:           []string{wantZone},
		VolumeBindingMode: storagev1.VolumeBindingImmediate,
	})
	if err != nil {
		t.Fatalf("GeneratePersistentVolumes() failed: %v", err)
	}
	if len(pvs) != len(pvYAMLPaths) {
		t.Fatalf("GeneratePersistentVolumes() returned %d PVs, expected %d", len(pvs), len(pvYAMLPaths))
	}
	for i := range pvs {
		pv := pvs[i]
		gotStorage := pv.Spec.Capacity[corev1.ResourceStorage]
		if gotStorage.Cmp(wantStorage) != 0 {
			t.Errorf("GeneratePersistentVolumes gotStorage %q, wantStorage %q", gotStorage.String(), wantStorage.String())
		}
		gotZone := pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Values[0]
		if gotZone != wantZone {
			t.Errorf("GeneratePersistentVolumes gotZone %q, wantZone %q", gotZone, wantZone)
		}
		gotAccessMode := pv.Spec.AccessModes[0]
		if gotAccessMode != wantAccessMode {
			t.Errorf("GeneratePersistentVolumes gotAccessMode %q, wantAccessMode %q", gotAccessMode, wantAccessMode)
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
			pods, podYAMLPaths, err := GenerateSimplePodsForResourcePreset(resourceCategory, podCount, PodGenInput{
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
			if len(pods) != podCount {
				t.Errorf("expecting %d pods for %q, got %d", podCount, resourceCategory, len(pods))
			}
			if len(podYAMLPaths) != podCount {
				t.Errorf("expecting %d paths for %q, got %d", podCount, resourceCategory, len(podYAMLPaths))
			}

			want := resourceCategory.AsResourceList()
			for _, p := range pods {
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
