// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package awsprice provides utilities to fetch and parse AWS EC2 instance
// pricing information from the AWS public price list JSON.
//
// It extracts OnDemand hourly prices for EC2 instance types, filtered by
// operating system, region, and tenancy. Results are returned as
// plannerapi.InstanceTypeInfo values suitable for consumption in higher-level
// tools.

package awsprice

// PriceList represents the root of the AWS pricing JSON document.
// It contains both the product metadata (instance attributes) and
// the pricing terms (OnDemand, Reserved).
type PriceList struct {
	Products map[string]Product `json:"products"`
	Terms    Terms              `json:"terms"`
}

// Product holds metadata for a single product SKU (e.g., an EC2 instance type
// under a specific OS, tenancy, and license model).
type Product struct {
	Attributes Attributes `json:"attributes"`
}

// Attributes describes the key attributes of an EC2 instance SKU.
// Not all attributes are relevant for pricing; we keep only the fields
// required for building [plannerapi.InstanceTypeInfo] records.
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
// We are primarily interested in OnDemand pricing.
type Terms struct {
	OnDemand map[string]map[string]OfferTerm `json:"OnDemand"`
}

// OfferTerm groups one or more price dimensions for a given product offer.
// Each dimension may differ in billing granularity (hourly, per-second).
type OfferTerm struct {
	PriceDimensions map[string]PriceDimension `json:"priceDimensions"`
}

// PriceDimension describes the actual unit price for a product offer.
// Example: unit = "Hrs", pricePerUnit["USD"] = "0.0928".
type PriceDimension struct {
	PricePerUnit map[string]string `json:"pricePerUnit"`
	Unit         string            `json:"unit"`
}
