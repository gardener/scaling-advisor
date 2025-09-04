package pricing

import (
	"testing"
)

func TestGetInstancePricing(t *testing.T) {
	instancePrices, err := GetInstancePricing("aws", "test_instances.json")
	if err != nil {
		t.Errorf("error creating instance pricing object: %v", err)
	}

	_, err = instancePrices.GetPrice("region_a", "instance_type_1")
	if err != nil {
		t.Error("failed to fetch instance price for instance_type_1")
	}
	_, err = instancePrices.GetPrice("region_b", "instance_type_2")
	if err != nil {
		t.Error("failed to fetch instance price for instance_type_2")
	}
}

func TestGetPrice(t *testing.T) {
	var entries = []struct {
		name         string
		region       string
		instanceType string
		error        bool
	}{
		{"valid region and instance", "region_a", "instance_type_1", false},
		{"valid region and instance, second entry", "region_b", "instance_type_2", false},
		{"valid region but invalid instance", "region_a", "invalid_instance", true},
		{"invalid region but valid instance", "invalid_region", "instance_type_2", true},
	}

	instancePrices, err := GetInstancePricing("aws", "test_instances.json")
	if err != nil {
		t.Error("error is not nil")
	}

	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			_, err := instancePrices.GetPrice(entry.region, entry.instanceType)
			if entry.error && err == nil {
				t.Error("expected error, found no error instead")
			} else if !entry.error && err != nil {
				t.Errorf("expected no error, got error instead: %v", err)
			}
		})
	}
}
