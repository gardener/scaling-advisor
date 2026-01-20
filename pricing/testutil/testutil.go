// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"embed"
	"fmt"

	"github.com/gardener/scaling-advisor/pricing"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/planner"
)

//go:embed data/*.json
var dataFS embed.FS

// GetInstancePricingAccessWithFakeData loads and parses fake instance pricing data from sample data for testing purposes.
// Returns an implementation of planner.InstancePricingAccess or an error if loading or parsing the data fails.
// Errors are wrapped with planner.ErrLoadInstanceTypeInfo sentinel error.
func GetInstancePricingAccessWithFakeData() (access planner.InstancePricingAccess, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", planner.ErrLoadInstanceTypeInfo, err)
		}
	}()
	testData, err := dataFS.ReadFile("data/fake-instance_price_infos.json")
	if err != nil {
		return
	}
	return pricing.GetInstancePricingFromData(commontypes.CloudProviderAWS, testData)
}

// GetInstancePricingAccessForTop20AWSInstanceTypes loads pricing data for the top 20 AWS instance types in eu-west-1 region and
// Returns an implementation of planner.InstancePricingAccess or an error if loading or parsing the data fails.
// Errors are wrapped with planner.ErrLoadInstanceTypeInfo sentinel error.
func GetInstancePricingAccessForTop20AWSInstanceTypes() (access planner.InstancePricingAccess, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", planner.ErrLoadInstanceTypeInfo, err)
		}
	}()
	testData, err := dataFS.ReadFile("data/aws_eu-west-1_top20_instance_pricing.json")
	if err != nil {
		return
	}
	return pricing.GetInstancePricingFromData(commontypes.CloudProviderAWS, testData)
}
