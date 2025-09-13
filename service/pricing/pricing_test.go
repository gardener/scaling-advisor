// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package pricing_test

import (
	"github.com/gardener/scaling-advisor/service/pricing/testutil"
	"testing"
)

func TestGetInstancePricing(t *testing.T) {
	access, err := testutil.LoadTestInstancePricingAccess()
	if err != nil {
		t.Error(err)
		return
	}
	if access == nil {
		t.Error("access is nil")
		return
	}
	_, err = access.GetInfo("region_a", "instance_type_1")
	if err != nil {
		t.Error("failed to fetch instance price for instance_type_1")
	}
	_, err = access.GetInfo("region_b", "instance_type_2")
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

	access, err := testutil.LoadTestInstancePricingAccess()
	if err != nil {
		t.Error(err)
		return
	}

	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			_, err := access.GetInfo(entry.region, entry.instanceType)
			if entry.error && err == nil {
				t.Error("expected error, found no error instead")
			} else if !entry.error && err != nil {
				t.Errorf("expected no error, got error instead: %v", err)
			}
		})
	}
}
