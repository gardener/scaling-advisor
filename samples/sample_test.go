// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGenerateSimplePodsWithResources(t *testing.T) {
	podCount := 4
	for _, resourcePairsLabel := range allResourcePairsLabels {
		t.Run(string(resourcePairsLabel), func(t *testing.T) {
			pods, podYAMLPaths, err := GenerateSimplePodsForResourceCategory(podCount, resourcePairsLabel, SimplePodMetadata{
				Name: string(resourcePairsLabel),
				AppLabels: AppLabels{
					Name:      string(resourcePairsLabel),
					Component: "fruit",
					PartOf:    "food",
					ManagedBy: "logistics",
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("Generated %d pods for %q", len(pods), resourcePairsLabel)
			if len(pods) != podCount {
				t.Errorf("expecting %d pods for %q, got %d", podCount, resourcePairsLabel, len(pods))
			}
			t.Logf("Generated podYAMLPaths %q for %q", podYAMLPaths, resourcePairsLabel)
			if len(podYAMLPaths) != podCount {
				t.Errorf("expecting %d paths for %q, got %d", podCount, resourcePairsLabel, len(podYAMLPaths))
			}
			want := resourcePairsLabel.AsResourceList()
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
					t.Errorf("ResourceList mismatch for %q (-want +got):\n%s", resourcePairsLabel, diff)
				}
			}
		})
	}
}
