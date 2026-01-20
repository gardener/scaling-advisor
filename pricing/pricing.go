// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package pricing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/planner"
)

var _ planner.GetProviderInstancePricingAccessFunc = GetInstancePricingAccess

// GetInstancePricingAccess loads instance pricing details from the given pricingDataPath and delegates to GetInstancePricingFromData.
func GetInstancePricingAccess(provider commontypes.CloudProvider, pricingDataPath string) (planner.InstancePricingAccess, error) {
	data, err := os.ReadFile(filepath.Clean(pricingDataPath))
	if err != nil {
		return nil, err
	}
	return GetInstancePricingFromData(provider, data)
}

// GetInstancePricingFromData parses instance pricing data and returns an InstancePricingAccess implementation for the given provider.
func GetInstancePricingFromData(provider commontypes.CloudProvider, data []byte) (planner.InstancePricingAccess, error) {
	var ip infoAccess
	var err error
	ip.CloudProvider = provider
	ip.infosByPriceKey, err = parseInstanceTypeInfos(data)
	return &ip, err
}

func parseInstanceTypeInfos(data []byte) (map[planner.PriceKey]planner.InstancePriceInfo, error) {
	var jsonEntries []planner.InstancePriceInfo
	//consider using streaming decoder instead
	err := json.Unmarshal(data, &jsonEntries)
	if err != nil {
		return nil, err
	}
	infosByPriceKey := make(map[planner.PriceKey]planner.InstancePriceInfo, len(jsonEntries))
	for _, info := range jsonEntries {
		key := planner.PriceKey{
			Name:   info.InstanceType,
			Region: info.Region,
		}
		infosByPriceKey[key] = info
	}
	return infosByPriceKey, nil
}

var _ planner.InstancePricingAccess = (*infoAccess)(nil)

type infoAccess struct {
	infosByPriceKey map[planner.PriceKey]planner.InstancePriceInfo
	CloudProvider   commontypes.CloudProvider
}

func (a infoAccess) GetInfo(region, instanceType string) (info planner.InstancePriceInfo, err error) {
	info, ok := a.infosByPriceKey[planner.PriceKey{
		Name:   instanceType,
		Region: region,
	}]
	if ok {
		return
	}
	err = fmt.Errorf("no instance type info found for instanceType %q in region %q ", instanceType, region)
	return
}
