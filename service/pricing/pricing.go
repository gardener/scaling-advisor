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
	"github.com/gardener/scaling-advisor/api/service"
)

var _ service.GetProviderInstancePricingAccessFunc = GetInstancePricingAccess

// GetInstancePricingAccess loads instance pricing details from the given pricingDataPath and delegates to GetInstancePricingFromData.
func GetInstancePricingAccess(provider commontypes.CloudProvider, pricingDataPath string) (service.InstancePricingAccess, error) {
	data, err := os.ReadFile(filepath.Clean(pricingDataPath))
	if err != nil {
		return nil, err
	}
	return GetInstancePricingFromData(provider, data)
}

// GetInstancePricingFromData parses instance pricing data and returns an InstancePricingAccess implementation for the given provider.
func GetInstancePricingFromData(provider commontypes.CloudProvider, data []byte) (service.InstancePricingAccess, error) {
	var ip infoAccess
	var err error
	ip.CloudProvider = provider
	ip.infosByPriceKey, err = parseInstanceTypeInfos(data)
	return &ip, err
}

func parseInstanceTypeInfos(data []byte) (map[service.PriceKey]service.InstancePriceInfo, error) {
	var jsonEntries []service.InstancePriceInfo
	//consider using streaming decoder instead
	err := json.Unmarshal(data, &jsonEntries)
	if err != nil {
		return nil, err
	}
	infosByPriceKey := make(map[service.PriceKey]service.InstancePriceInfo, len(jsonEntries))
	for _, info := range jsonEntries {
		key := service.PriceKey{
			Name:   info.InstanceType,
			Region: info.Region,
		}
		infosByPriceKey[key] = info
	}
	return infosByPriceKey, nil
}

var _ service.InstancePricingAccess = (*infoAccess)(nil)

type infoAccess struct {
	CloudProvider   commontypes.CloudProvider
	infosByPriceKey map[service.PriceKey]service.InstancePriceInfo
}

func (a infoAccess) GetInfo(region, instanceType string) (info service.InstancePriceInfo, err error) {
	info, ok := a.infosByPriceKey[service.PriceKey{
		Name:   instanceType,
		Region: region,
	}]
	if ok {
		return
	}
	err = fmt.Errorf("no instance type info found for instanceType %q in region %q ", instanceType, region)
	return
}
