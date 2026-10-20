// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"encoding/json"
	"testing"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	"github.com/gardener/scaling-advisor/api/core/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInstancePriceSetRoundTrip(t *testing.T) {
	original := v1alpha1.InstancePriceSet{
		TypeMeta: metav1.TypeMeta{APIVersion: "core.sa.gardener.cloud/v1alpha1", Kind: "InstancePriceSet"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aws-prices",
			Namespace: "default",
		},
		Spec: v1alpha1.InstancePriceSetSpec{
			Provider: apicommon.CloudProviderAWS,
			Prices: []v1alpha1.InstancePrice{
				{
					InstancePriceKey: v1alpha1.InstancePriceKey{
						InstanceType: "m5.xlarge",
						Region:       "us-east-1",
						CapacityType: apicommon.CapacityTypeOnDemand,
					},
					Price: "0.192",
					ResourceUnitPrice: []v1alpha1.ResourceUnitPrice{
						{Name: corev1.ResourceCPU, Value: "0.048"},
						{Name: corev1.ResourceMemory, Value: "0.006"},
					},
				},
				{
					InstancePriceKey: v1alpha1.InstancePriceKey{
						InstanceType: "m5.xlarge",
						Region:       "us-east-1",
						CapacityType: apicommon.CapacityTypeSpot,
					},
					Price: "0.057",
				},
			},
		},
	}

	got := roundTrip(t, original)

	if got.Spec.Provider != original.Spec.Provider {
		t.Errorf("Provider: got %q, want %q", got.Spec.Provider, original.Spec.Provider)
	}
	if len(got.Spec.Prices) != 2 {
		t.Fatalf("Prices length: got %d, want 2", len(got.Spec.Prices))
	}
	p0 := got.Spec.Prices[0]
	if p0.InstanceType != "m5.xlarge" || p0.Region != "us-east-1" || p0.CapacityType != apicommon.CapacityTypeOnDemand {
		t.Errorf("Prices[0] key: got %+v", p0.InstancePriceKey)
	}
	if p0.Price != "0.192" {
		t.Errorf("Prices[0].Price: got %q, want \"0.192\"", p0.Price)
	}
	if len(p0.ResourceUnitPrice) != 2 {
		t.Errorf("Prices[0].ResourceUnitPrice length: got %d, want 2", len(p0.ResourceUnitPrice))
	}
	// second entry has no ResourceUnitPrice - omitempty must not emit the field
	if got.Spec.Prices[1].ResourceUnitPrice != nil {
		t.Errorf("Prices[1].ResourceUnitPrice: expected nil after omitempty round-trip, got %v", got.Spec.Prices[1].ResourceUnitPrice)
	}
}

// TestInstancePriceKeyInlineEmbed verifies that InstancePriceKey fields are serialized
// at the same JSON level as InstancePrice (inline embed), not nested under a "instancePriceKey" key.
func TestInstancePriceKeyInlineEmbed(t *testing.T) {
	p := v1alpha1.InstancePrice{
		InstancePriceKey: v1alpha1.InstancePriceKey{
			InstanceType: "t3.micro",
			Region:       "eu-west-1",
			CapacityType: apicommon.CapacityTypeOnDemand,
		},
		Price: "0.0116",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	for _, key := range []string{"instanceType", "region", "capacityType"} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected top-level JSON key %q (inline embed), not found in %s", key, data)
		}
	}
	if _, nested := m["instancePriceKey"]; nested {
		t.Errorf("found unexpected nested key \"instancePriceKey\" - embed is broken: %s", data)
	}
}

func TestNodeTemplateSetRoundTrip(t *testing.T) {
	original := v1alpha1.NodeTemplateSet{
		TypeMeta: metav1.TypeMeta{APIVersion: "core.sa.gardener.cloud/v1alpha1", Kind: "NodeTemplateSet"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "templates",
			Namespace: "tenant-a",
		},
		Spec: v1alpha1.NodeTemplateSetSpec{
			Templates: []v1alpha1.NodeTemplate{
				{
					Name:         "m5-standard",
					InstanceType: "m5.xlarge",
					ReservedResources: corev1.ResourceList{
						corev1.ResourceCPU:    apiresource.MustParse("100m"),
						corev1.ResourceMemory: apiresource.MustParse("512Mi"),
					},
				},
				{
					Name:         "m5-gpu",
					InstanceType: "p3.2xlarge",
					ReservedResources: corev1.ResourceList{
						corev1.ResourceCPU: apiresource.MustParse("200m"),
					},
					ExtendedResources: corev1.ResourceList{
						"security.com/dongle": apiresource.MustParse("2"),
					},
				},
			},
		},
	}

	got := roundTrip(t, original)

	if len(got.Spec.Templates) != 2 {
		t.Fatalf("Templates length: got %d, want 2", len(got.Spec.Templates))
	}
	if got.Spec.Templates[0].Name != "m5-standard" {
		t.Errorf("Templates[0].Name: got %q", got.Spec.Templates[0].Name)
	}
	// ExtendedResources is omitempty - first template must not have it
	if got.Spec.Templates[0].ExtendedResources != nil {
		t.Errorf("Templates[0].ExtendedResources: expected nil, got %v", got.Spec.Templates[0].ExtendedResources)
	}
	// Second template must preserve GPU extended resource
	gpuQty, ok := got.Spec.Templates[1].ExtendedResources["security.com/dongle"]
	if !ok {
		t.Fatal("Templates[1].ExtendedResources missing security.com/dongle")
	}
	if gpuQty.Cmp(apiresource.MustParse("2")) != 0 {
		t.Errorf("Templates[1] GPU quantity: got %v", gpuQty)
	}
}

func TestScalingConstraintRoundTrip(t *testing.T) {
	qty := func(s string) apiresource.Quantity { return apiresource.MustParse(s) }

	original := v1alpha1.ScalingConstraint{
		TypeMeta: metav1.TypeMeta{APIVersion: "core.sa.gardener.cloud/v1alpha1", Kind: "ScalingConstraint"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-a",
			Namespace: "tenant-a",
		},
		Spec: v1alpha1.ScalingConstraintSpec{
			PlannerStrategy:   apicommon.PlannerStrategyScaleOutThenScaleIn,
			SimulatorStrategy: apicommon.SimulatorStrategySingleNodeMultiSim,
			NodePools: []v1alpha1.NodePool{
				{
					Name:              "workers",
					Region:            "eu-west-1",
					AvailabilityZones: []string{"eu-west-1a", "eu-west-1b"},
					Priority:          10,
					NodeTemplateRefs: []v1alpha1.NodeTemplateRef{
						{
							Name:     "m5-standard",
							Priority: 5,
							Quota: &v1alpha1.ResourceLimits{
								Min: corev1.ResourceList{corev1.ResourceCPU: qty("2")},
								Max: corev1.ResourceList{corev1.ResourceCPU: qty("32")},
							},
						},
					},
				},
			},
		},
	}

	got := roundTrip(t, original)

	if got.Spec.PlannerStrategy != apicommon.PlannerStrategyScaleOutThenScaleIn {
		t.Errorf("PlannerStrategy: got %q", got.Spec.PlannerStrategy)
	}
	if got.Spec.SimulatorStrategy != apicommon.SimulatorStrategySingleNodeMultiSim {
		t.Errorf("SimulatorStrategy: got %q", got.Spec.SimulatorStrategy)
	}
	if len(got.Spec.NodePools) != 1 {
		t.Fatalf("NodePools length: got %d", len(got.Spec.NodePools))
	}
	np := got.Spec.NodePools[0]
	if np.Priority != 10 {
		t.Errorf("NodePool.Priority: got %d", np.Priority)
	}
	if len(np.AvailabilityZones) != 2 {
		t.Errorf("AvailabilityZones length: got %d", len(np.AvailabilityZones))
	}
	ntr := np.NodeTemplateRefs[0]
	if ntr.Quota == nil {
		t.Fatal("NodeTemplateRef.Quota: expected non-nil after round-trip")
	}
	minCPU := ntr.Quota.Min[corev1.ResourceCPU]
	if minCPU.Cmp(qty("2")) != 0 {
		t.Errorf("Quota.Min[cpu]: got %v", minCPU)
	}

	// ScoringStrategy was not set - omitempty must produce empty string, not an enum error
	if got.Spec.ScoringStrategy != "" {
		t.Errorf("ScoringStrategy: expected empty string, got %q", got.Spec.ScoringStrategy)
	}
}

// TestOptionalEnumOmitempty verifies that optional enum string fields round-trip as "" (not emitting
// the field) when unset, which is critical because CRD enum validators reject the empty string.
func TestOptionalEnumOmitempty(t *testing.T) {
	sc := v1alpha1.ScalingConstraint{
		Spec: v1alpha1.ScalingConstraintSpec{
			PlannerStrategy: apicommon.PlannerStrategyScaleOutThenScaleIn,
			// SimulatorStrategy and ScoringStrategy intentionally omitted
			NodePools: []v1alpha1.NodePool{
				{Name: "p", Region: "r", AvailabilityZones: []string{"z"},
					NodeTemplateRefs: []v1alpha1.NodeTemplateRef{{Name: "t"}}},
			},
		},
	}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	spec, _ := m["spec"].(map[string]any)
	if _, present := spec["simulatorStrategy"]; present {
		t.Errorf("simulatorStrategy must not appear in JSON when unset (omitempty), but found in: %s", data)
	}
	if _, present := spec["scoringStrategy"]; present {
		t.Errorf("scoringStrategy must not appear in JSON when unset (omitempty), but found in: %s", data)
	}
}

// TestNodePlacementTextMarshal verifies that NodePlacement implements TextMarshaler/TextUnmarshaler
// and can be used as a JSON map key.
func TestNodePlacementTextMarshal(t *testing.T) {
	np := v1alpha1.NodePlacement{
		PoolName:         "workers",
		TemplateName:     "m5-standard",
		InstanceType:     "m5.xlarge",
		Region:           "eu-west-1",
		AvailabilityZone: "eu-west-1a",
	}

	// MarshalText / UnmarshalText round-trip
	text, err := np.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	want := "workers|m5-standard|m5.xlarge|eu-west-1|eu-west-1a"
	if string(text) != want {
		t.Errorf("MarshalText: got %q, want %q", string(text), want)
	}
	var got v1alpha1.NodePlacement
	if err := got.UnmarshalText(text); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if got != np {
		t.Errorf("UnmarshalText: got %+v, want %+v", got, np)
	}

	// JSON map key round-trip
	m := map[v1alpha1.NodePlacement]int{np: 3}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal map: %v", err)
	}
	var m2 map[v1alpha1.NodePlacement]int
	if err := json.Unmarshal(data, &m2); err != nil {
		t.Fatalf("unmarshal map: %v", err)
	}
	if m2[np] != 3 {
		t.Errorf("map value after round-trip: got %d, want 3", m2[np])
	}
}

// TestNodePlacementUnmarshalInvalid verifies UnmarshalText rejects malformed input.
func TestNodePlacementUnmarshalInvalid(t *testing.T) {
	var np v1alpha1.NodePlacement
	if err := np.UnmarshalText([]byte("only|three|parts")); err == nil {
		t.Error("expected error for too-few fields, got nil")
	}
}

func roundTrip[T any](t *testing.T, v T) T {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal failed: %v\nJSON: %s", err, data)
	}
	return out
}
