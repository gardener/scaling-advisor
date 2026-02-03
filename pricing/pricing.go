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
	pricingapi "github.com/gardener/scaling-advisor/api/pricing"
)

var _ pricingapi.GetProviderInstancePricingAccessFunc = GetInstancePricingAccess

// GetInstancePricingAccess loads instance pricing details from the given pricingDataPath and delegates to GetInstancePricingFromData.
func GetInstancePricingAccess(provider commontypes.CloudProvider, pricingDataPath string) (pricingapi.InstancePricingAccess, error) {
	data, err := os.ReadFile(filepath.Clean(pricingDataPath))
	if err != nil {
		return nil, err
	}
	return GetInstancePricingFromData(provider, data)
}

// GetInstancePricingFromData parses instance pricing data and returns an InstancePricingAccess implementation for the given provider.
func GetInstancePricingFromData(provider commontypes.CloudProvider, data []byte) (pricingapi.InstancePricingAccess, error) {
	var ip infoAccess
	var err error
	ip.CloudProvider = provider
	ip.infosByPriceKey, err = parseInstanceTypeInfos(data)
	return &ip, err
}

func parseInstanceTypeInfos(data []byte) (map[pricingapi.PriceKey]pricingapi.InstancePriceInfo, error) {
	var jsonEntries []pricingapi.InstancePriceInfo
	//consider using streaming decoder instead
	err := json.Unmarshal(data, &jsonEntries)
	if err != nil {
		return nil, err
	}
	infosByPriceKey := make(map[pricingapi.PriceKey]pricingapi.InstancePriceInfo, len(jsonEntries))
	for _, info := range jsonEntries {
		key := pricingapi.PriceKey{
			Name:   info.InstanceType,
			Region: info.Region,
		}
		infosByPriceKey[key] = info
	}
	return infosByPriceKey, nil
}

var _ pricingapi.InstancePricingAccess = (*infoAccess)(nil)

type infoAccess struct {
	infosByPriceKey map[pricingapi.PriceKey]pricingapi.InstancePriceInfo
	CloudProvider   commontypes.CloudProvider
}

func (a *infoAccess) GetInfo(region, instanceType string) (info pricingapi.InstancePriceInfo, err error) {
	info, ok := a.infosByPriceKey[pricingapi.PriceKey{
		Name:   instanceType,
		Region: region,
	}]
	if ok {
		return
	}
	err = fmt.Errorf("no instance type info found for instanceType %q in region %q ", instanceType, region)
	return
}
