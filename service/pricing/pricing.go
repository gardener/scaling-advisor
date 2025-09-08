// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package pricing

import (
	"encoding/json"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/service"
	"os"
)

var _ service.GetProviderInstanceTypeInfoAccessFunc = GetInstancePricing

func GetInstancePricing(provider commontypes.CloudProvider, pricingDataPath string) (service.InstanceTypeInfoAccess, error) {
	data, err := os.ReadFile(pricingDataPath)
	if err != nil {
		return nil, err
	}
	return GetInstancePricingFromData(provider, data)
}

func GetInstancePricingFromData(provider commontypes.CloudProvider, data []byte) (service.InstanceTypeInfoAccess, error) {
	var ip infoAccess
	var err error
	ip.CloudProvider = provider
	ip.infosByPriceKey, err = parseInstanceTypeInfos(data)
	return &ip, err
}

func parseInstanceTypeInfos(data []byte) (map[service.PriceKey]service.InstanceTypeInfo, error) {
	var jsonEntries []service.InstanceTypeInfo
	//consider using streaming decoder instead
	err := json.Unmarshal(data, &jsonEntries)
	if err != nil {
		return nil, err
	}
	infosByPriceKey := make(map[service.PriceKey]service.InstanceTypeInfo, len(jsonEntries))
	for _, info := range jsonEntries {
		key := service.PriceKey{
			Name:   info.Name,
			Region: info.Region,
		}
		infosByPriceKey[key] = info
	}
	return infosByPriceKey, nil
}

var _ service.InstanceTypeInfoAccess = (*infoAccess)(nil)

type infoAccess struct {
	CloudProvider   commontypes.CloudProvider
	infosByPriceKey map[service.PriceKey]service.InstanceTypeInfo
}

func (a infoAccess) GetInfo(region, instanceType string) (info service.InstanceTypeInfo, err error) {
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
