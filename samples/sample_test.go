// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	commontestutil "github.com/gardener/scaling-advisor/common/testutil"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/resource"
)

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
	pvs, pvYAMLPaths, err := GeneratePersistentVolumes(SimplePVGenInput{
		VolCommon: VolCommon{
			GenDir:     testGenDir,
			Namespace:  wantNS,
			Storage:    wantStorage,
			AccessMode: wantAccessMode,
		},
		Zone:     wantZone,
		PVCNames: wantNames,
	})
	if err != nil {
		t.Fatalf("GeneratePersistentVolumes() failed: %v", err)
	}
	if len(pvs) != len(pvYAMLPaths) {
		t.Fatalf("GeneratePersistentVolumes() returned %d PVs, expected %d", len(pvs), len(pvYAMLPaths))
	}
	for i := range pvs {
		pvYAMLPath := pvYAMLPaths[i]
		pv := pvs[i]
		t.Logf("GeneratePersistentVolumes generated PV yaml %q containing %+v", pvYAMLPath, pv)
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
	sc, scYAMLPath, err := GenerateStorageClass(testGenDir, commontypes.CloudProviderAWS, wantName, wantVolumeBindingMode)
	if err != nil {
		t.Fatalf("GenerateStorageClass() failed: %v", err)
	}
	t.Logf("GenerateStorageClass generated StorageClass yaml %q, StorageClass: %+v", scYAMLPath, sc)
	gotVolumeBindingMode := sc.VolumeBindingMode
	if gotVolumeBindingMode == nil {
		t.Fatalf("GenerateStorageClass gotVolumeBindingMode is nil")
	}
	if *gotVolumeBindingMode != wantVolumeBindingMode {
		t.Errorf("GenerateStorageClass gotVolumeBindingMode %q, wantVolumeBindingMode %q", *gotVolumeBindingMode, wantVolumeBindingMode)
	}
	if sc.Name != wantName {
		t.Errorf("GenerateStorageClass gotName %q, wantName %q", sc.Name, wantName)
	}
}

func TestGeneratePVCs(t *testing.T) {
	wantNS := "test"
	wantStorage := resource.MustParse("1Gi")
	wantAccessMode := corev1.ReadWriteOnce
	wantNames := []string{"stem", "branch"}
	testGenDir, ok := commontestutil.CreateTestGenDir(t)
	if !ok {
		return
	}
	volCommon := VolCommon{
		GenDir:     testGenDir,
		Namespace:  wantNS,
		Storage:    wantStorage,
		AccessMode: wantAccessMode,
	}
	pvcs, pvcYAMLPaths, err := GeneratePersistentVolumeClaims(SimplePVCGenInput{
		VolCommon: volCommon,
		Names:     wantNames,
	})
	if err != nil {
		t.Fatalf("GeneratePersistentVolumes() failed: %v", err)
	}
	if len(pvcs) != len(pvcYAMLPaths) {
		t.Fatalf("GeneratePersistentVolumes() returned %d PVs, expected %d", len(pvcs), len(pvcYAMLPaths))
	}
	for i := range pvcs {
		pvcYAMLPath := pvcYAMLPaths[i]
		pvc := pvcs[i]
		t.Logf("Generated PVC yaml %q containing %+v", pvcYAMLPath, pvc)
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
	}

}

func TestGenerateSimplePodsWithResources(t *testing.T) {
	podCount := 4
	pvcNames := []string{"stem", "branch"}
	testGenDir, ok := commontestutil.CreateTestGenDir(t)
	if !ok {
		return
	}
	for _, resourceCategory := range allResourceCategories {
		t.Run(string(resourceCategory), func(t *testing.T) {
			pods, podYAMLPaths, err := GenerateSimplePodsForResourceCategory(resourceCategory, podCount, SimplePodGenInput{
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
			t.Logf("Generated %d pods for %q", len(pods), resourceCategory)
			if len(pods) != podCount {
				t.Errorf("expecting %d pods for %q, got %d", podCount, resourceCategory, len(pods))
			}
			t.Logf("Generated podYAMLPaths %q for %q", podYAMLPaths, resourceCategory)
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
