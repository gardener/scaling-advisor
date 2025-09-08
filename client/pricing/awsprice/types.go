// Package awsprice provides utilities to fetch and parse AWS EC2 instance
// pricing information from the AWS public price list JSON.
//
// It extracts OnDemand hourly prices for EC2 instance types, filtered by
// operating system, region, and tenancy. Results are returned as
// svcapi.InstanceTypeInfo values suitable for consumption in higher-level
// tools.

package awsprice

// AWSPriceList represents the root of the AWS pricing JSON document.
// It contains both the product metadata (instance attributes) and
// the pricing terms (OnDemand, Reserved).
type AWSPriceList struct {
	Products map[string]AWSProduct `json:"products"`
	Terms    AWSTerms              `json:"terms"`
}

// AWSProduct holds metadata for a single product SKU (e.g., an EC2 instance type
// under a specific OS, tenancy, and license model).
type AWSProduct struct {
	Attributes AWSAttributes `json:"attributes"`
}

// AWSAttributes describes the key attributes of an EC2 instance SKU.
// Not all attributes are relevant for pricing; we keep only the fields
// required for building [svcapi.InstanceTypeInfo] records.
type AWSAttributes struct {
	InstanceType string `json:"instanceType"`
	VCPU         string `json:"vcpu"`
	Memory       string `json:"memory"`
	OperatingSys string `json:"operatingSystem"`
	Tenancy      string `json:"tenancy"`
}

// AWSTerms contains the pricing terms for products.
// We are primarily interested in OnDemand pricing.
type AWSTerms struct {
	OnDemand map[string]map[string]AWSOfferTerm `json:"OnDemand"`
}

// AWSOfferTerm groups one or more price dimensions for a given product offer.
// Each dimension may differ in billing granularity (hourly, per-second).
type AWSOfferTerm struct {
	PriceDimensions map[string]AWSPriceDimension `json:"priceDimensions"`
}

// AWSPriceDimension describes the actual unit price for a product offer.
// Example: unit = "Hrs", pricePerUnit["USD"] = "0.0928".
type AWSPriceDimension struct {
	Unit         string            `json:"unit"`
	PricePerUnit map[string]string `json:"pricePerUnit"`
}
