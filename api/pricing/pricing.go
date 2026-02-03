// Package pricing provides domain types and facades for cloud provider instance details and their pricing.
package pricing

import commontypes "github.com/gardener/scaling-advisor/api/common/types"

// InstancePriceInfo contains pricing and specification information for a cloud instance type.
type InstancePriceInfo struct {
	// InstanceType is the name of the instance type.
	InstanceType string
	// Region is the cloud region where the instance is available.
	Region string
	// OS is the operating system for the instance type.
	OS string
	// Memory is the amount of memory in GB for the instance type.
	Memory int64
	// GPUMemory is the amount of GPU memory in GB for the instance type.
	GPUMemory int64
	// HourlyPrice is the hourly cost for the instance type.
	HourlyPrice float64
	// GPU is the number of GPUs for the instance type.
	GPU int32
	// VCPU is the number of virtual CPUs for the instance type.
	VCPU int32
}

// PriceKey represents the key for a instance type price within a cloud provider.
type PriceKey struct {
	// Name is the instance type name.
	Name string
	// Region is the cloud region.
	Region string
}

// InstancePricingAccess defines an interface for accessing instance pricing information.
type InstancePricingAccess interface {
	// GetInfo gets the InstancePriceInfo (whicn includes price) for the given region and instance type.
	// TODO: should we also pass OS name here ? if so, we need to need to change ScalingConstraint.
	GetInfo(region, instanceTypeName string) (InstancePriceInfo, error)
}

// GetProviderInstancePricingAccessFunc is a factory function for creating InstancePricingAccess implementations.
type GetProviderInstancePricingAccessFunc func(provider commontypes.CloudProvider, instanceTypeInfoPath string) (InstancePricingAccess, error)
