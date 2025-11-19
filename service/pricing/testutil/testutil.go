// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"embed"
	"fmt"

	"github.com/gardener/scaling-advisor/service/pricing"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/service"
)

//go:embed testdata/*
var testDataFS embed.FS

// GetInstancePricingAccessWithFakeData loads and parses fake instance pricing data from testdata for testing purposes.
// Returns an implementation of service.InstancePricingAccess or an error if loading or parsing the data fails.
// Errors are wrapped with service.ErrLoadInstanceTypeInfo sentinel error.
func GetInstancePricingAccessWithFakeData() (access service.InstancePricingAccess, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", service.ErrLoadInstanceTypeInfo, err)
		}
	}()
	testData, err := testDataFS.ReadFile("testdata/instance_price_infos.json")
	if err != nil {
		return
	}
	return pricing.GetInstancePricingFromData(commontypes.CloudProviderAWS, testData)
}

// GetInstancePricingAccessForTop20AWSInstanceTypes loads pricing data for the top 20 AWS instance types in eu-west-1 region and
// Returns an implementation of service.InstancePricingAccess or an error if loading or parsing the data fails.
// Errors are wrapped with service.ErrLoadInstanceTypeInfo sentinel error.
func GetInstancePricingAccessForTop20AWSInstanceTypes() (access service.InstancePricingAccess, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", service.ErrLoadInstanceTypeInfo, err)
		}
	}()
	testData, err := testDataFS.ReadFile("testdata/aws_eu-west-1_top20_instance_pricing.json")
	if err != nil {
		return
	}
	return pricing.GetInstancePricingFromData(commontypes.CloudProviderAWS, testData)
}
