// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"embed"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/service/pricing"
)

//go:embed testdata/*
var testDataFS embed.FS

func LoadTestInstanceTypeInfoAccess() (access service.InstanceTypeInfoAccess, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", service.ErrLoadInstanceTypeInfo, err)
		}
	}()
	testData, err := testDataFS.ReadFile("testdata/instance_type_infos.json")
	if err != nil {
		return
	}
	return pricing.GetInstancePricingFromData(commontypes.AWSCloudProvider, testData)
}
