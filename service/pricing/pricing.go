package pricing

import (
	"encoding/json"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/service/api"
	"math"
	"os"
)

var _ api.GetInstancePricing = GetInstancePricing

func GetInstancePricing(provider commontypes.CloudProvider, pricingDataPath string) (api.InstancePricing, error) {
	var ip InstancePricing

	ip.CloudProvider = provider

	//Read pricing data from json file and add it to map
	data, err := os.ReadFile(pricingDataPath)
	if err != nil {
		return nil, err
	}

	var jsonEntries []jsonInstance
	//consider using streaming decoder instead
	err = json.Unmarshal(data, &jsonEntries)
	if err != nil {
		return nil, err
	}

	ip.instancePrice = make(map[InstanceType]InstanceDetails)
	for _, entry := range jsonEntries {
		key := InstanceType{
			instanceName: entry.InstanceName,
			region:       entry.Region,
		}
		value := InstanceDetails{
			VCPU:        entry.VCPU,
			Memory:      entry.Memory,
			HourlyPrice: entry.HourlyPrice,
			OS:          entry.OS,
		}
		ip.instancePrice[key] = value
	}

	return ip, nil
}

var _ api.InstancePricing = (*InstancePricing)(nil)

type InstancePricing struct {
	CloudProvider commontypes.CloudProvider
	//TODO put pricing data in-memory.
	instancePrice map[InstanceType]InstanceDetails
}

type InstanceType struct {
	instanceName string
	region       string
}

type InstanceDetails struct {
	//TODO: Are these extra fields required, or is price sufficient?
	VCPU        int32
	Memory      float64
	HourlyPrice float64
	OS          string
}

type jsonInstance struct {
	InstanceName string  `json:"instanceName"`
	Region       string  `json:"region"`
	VCPU         int32   `json:"VCPU"`
	Memory       float64 `json:"memory"`
	HourlyPrice  float64 `json:"hourlyPrice"`
	OS           string  `json:"os"`
}

func (a InstancePricing) GetPrice(region, instanceType string) (float64, error) {
	price, ok := a.instancePrice[InstanceType{
		instanceName: instanceType,
		region:       region,
	}]
	if ok {
		return price.HourlyPrice, nil
	}
	return math.MaxFloat64, fmt.Errorf("error: No instance pricing data found for instanceType: %s in region: %s", instanceType, region)
}
