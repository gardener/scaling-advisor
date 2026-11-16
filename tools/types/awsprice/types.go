// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package awsprice provides types for parsing the AWS EC2 public price list JSON.
package awsprice

// InstanceTypeInfo holds the parsed attributes and on-demand hourly price for
// a single EC2 instance type in a given region. It is the internal
// representation produced by ParseRegionPrices and consumed by the
// geninstancetypedef command to build InstanceTypeDefSet and InstancePriceSet CRs.
type InstanceTypeInfo struct {
	InstanceType string
	Region       string
	OS           string
	// Memory is in bytes.
	Memory int64
	// GPUMemory is in bytes; 0 if no GPU.
	GPUMemory   int64
	HourlyPrice float64
	VCPU        int32
	GPU         int32
}

// PriceList represents the root of the AWS pricing JSON document.
type PriceList struct {
	Products map[string]Product `json:"products"`
	Terms    Terms              `json:"terms"`
}

// Product holds metadata for a single product SKU.
type Product struct {
	Attributes Attributes `json:"attributes"`
}

// Attributes describes the key attributes of an EC2 instance SKU.
type Attributes struct {
	InstanceType string `json:"instanceType"`
	VCPU         string `json:"vcpu"`
	Memory       string `json:"memory"`
	GPU          string `json:"gpu"`
	GPUMemory    string `json:"gpuMemory"`
	OperatingSys string `json:"operatingSystem"`
	Tenancy      string `json:"tenancy"`
}

// Terms contains the pricing terms for products.
type Terms struct {
	OnDemand map[string]map[string]OfferTerm `json:"OnDemand"`
}

// OfferTerm groups one or more price dimensions for a given product offer.
type OfferTerm struct {
	PriceDimensions map[string]PriceDimension `json:"priceDimensions"`
}

// PriceDimension describes the actual unit price for a product offer.
// Example: unit = "Hrs", pricePerUnit["USD"] = "0.0928".
type PriceDimension struct {
	PricePerUnit map[string]string `json:"pricePerUnit"`
	Unit         string            `json:"unit"`
}
