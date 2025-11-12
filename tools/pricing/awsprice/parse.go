// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package awsprice

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/gardener/scaling-advisor/tools/types/awsprice"

	svcapi "github.com/gardener/scaling-advisor/api/service"
)

// ParseRegionPrices parses the raw pricing JSON for a given AWS region and OS,
// and returns a slice of InstancePriceInfo values.
//
// Parameters:
//   - region:  AWS region name (e.g., "us-east-1").
//   - osName:  Desired operating system (e.g., "Linux").
//   - data:    Raw JSON bytes from the AWS price list endpoint.
//
// Behavior:
//   - Filters products by the given operating system.
//   - Includes only Shared tenancy SKUs.
//   - Extracts per-hour OnDemand prices using extractOnDemandHourlyPriceForSKU.
//   - Deduplicates entries by keeping the lowest valid hourly price
//     per (InstanceType, Region, OS).
//
// Returns:
//   - A slice of svcapi.InstancePriceInfo with normalized pricing data.
//   - An error if the input JSON cannot be parsed.
func ParseRegionPrices(region, osName string, data []byte) ([]svcapi.InstancePriceInfo, error) {
	var raw awsprice.PriceList
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	type priceKey struct {
		InstanceType string
		OS           string
	}

	best := make(map[priceKey]svcapi.InstancePriceInfo, 1000)

	for sku, prod := range raw.Products {
		attrs := prod.Attributes
		if attrs.InstanceType == "" || attrs.VCPU == "" || attrs.Memory == "" {
			continue
		}
		if attrs.OperatingSys != osName {
			continue
		}
		// TODO : Support Dedicated tenancy if needed
		if attrs.Tenancy != "" && attrs.Tenancy != "Shared" {
			continue // skip Dedicated/Host
		}

		vcpu, err := parseInt32(attrs.VCPU)
		if err != nil {
			continue
		}
		mem, err := parseMemory(attrs.Memory)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: skipping SKU %q due to invalid memory attribute %q: %v\n", sku, attrs.Memory, err)
			continue
		}
		var (
			gpu       int32
			gpuMemory int64
		)
		if attrs.GPU != "" {
			gpu, err = parseInt32(attrs.GPU)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "warning: skipping SKU %q due to invalid GPU attribute %q: %v\n", sku, attrs.GPU, err)
				continue
			}
			if attrs.GPUMemory != "" && attrs.GPUMemory != "NA" {
				gpuMemory, err = parseMemory(attrs.GPUMemory)
				if err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "warning: skipping SKU %q due to invalid GPU memory attribute %q: %v\n", sku, attrs.GPUMemory, err)
					continue
				}
			}
		}

		price := extractOnDemandHourlyPriceForSKU(raw.Terms, sku)
		if price <= 0 {
			continue
		}

		key := priceKey{InstanceType: attrs.InstanceType, OS: attrs.OperatingSys}
		if existing, ok := best[key]; !ok || price < existing.HourlyPrice {
			best[key] = svcapi.InstancePriceInfo{
				InstanceType: attrs.InstanceType,
				Region:       region,
				VCPU:         vcpu,
				Memory:       mem,
				GPU:          gpu,
				GPUMemory:    gpuMemory,
				HourlyPrice:  price,
				OS:           attrs.OperatingSys,
			}
		}
	}

	infos := make([]svcapi.InstancePriceInfo, 0, len(best))
	for _, v := range best {
		infos = append(infos, v)
	}
	return infos, nil
}

// extractOnDemandHourlyPriceForSKU returns the lowest non-zero OnDemand hourly price
// for a given SKU. It filters out any price dimensions that are not per-hour
// (e.g., per-second billing).
//
// Parameters:
//   - terms: The full AWS terms data structure (parsed from JSON).
//   - sku:   The product SKU key from the Products map.
//
// Returns:
//   - The hourly OnDemand price in USD, or 0.0 if no per-hour price is found.
//
// Note: This function assumes that the caller has already filtered products
// for shared tenancy and the desired operating system.
//
//	"terms": {
//	 "OnDemand": {
//	   "ABC123": {
//	     "ABC123.SomeOffer": {
//	       "priceDimensions": {
//	         "ABC123.SomeOffer.Dim": {
//	           "unit": "Hrs",
//	           "pricePerUnit": { "USD": "0.0928" }
//	         }
//	       }
//	     }
//	   }
//	 }
//	}
func extractOnDemandHourlyPriceForSKU(terms awsprice.Terms, sku string) float64 {
	offers, ok := terms.OnDemand[sku]
	if !ok {
		return 0.0
	}

	best := 0.0
	for _, offer := range offers {
		for _, dim := range offer.PriceDimensions {
			if dim.Unit != "Hrs" {
				continue // only keep per-hour pricing
			}
			if usd, ok := dim.PricePerUnit["USD"]; ok {
				val, err := strconv.ParseFloat(usd, 64)
				if err != nil {
					continue
				}
				if best == 0.0 || val < best {
					best = val
				}
			}
		}
	}
	return best
}

// parseInt3 converts a string attribute to an int32 count. Ex : "4" -> 4.
func parseInt32(s string) (int32, error) {
	val, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, err
	}
	if val < 0 || val > math.MaxInt32 {
		return 0, fmt.Errorf("value %d out of int32 range", val)
	}
	return int32(val), nil // #nosec G109 -- value has been range-checked. If the value is greater than MaxInt32, an error is returned.
}

// parseMemory converts a memory attribute string like "16 GiB" into
// an int64 representing GiB. Ex: "16 GiB" -> 16
func parseMemory(s string) (int64, error) {
	// Example: "16 GiB"
	parts := strings.Fields(s)
	if len(parts) < 1 {
		return 0, fmt.Errorf("invalid memory string: %q", s)
	}

	val, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory string %q: %v", parts[0], err)
	}
	unit := parts[1]
	switch unit {
	case "GB":
		return int64(val * 1_000_000_000), nil
	case "GiB":
		return int64(val * 1_073_741_824), nil // 1024^3
	default:
		return 0, fmt.Errorf("unknown memory unit: %q", unit)
	}
}
