// SPDX-FileCopyrightText: Contributors to the Gardener project
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

	awstypes "github.com/gardener/scaling-advisor/tools/types/awsprice"
)

// ParseRegionPrices parses the raw pricing JSON for a given AWS region and OS
// and returns a slice of InstanceTypeInfo values.
//
//   - Filters products by the given operating system.
//   - Includes only Shared tenancy SKUs.
//   - Deduplicates entries by keeping the lowest valid hourly price
//     per (InstanceType, Region, OS).
func ParseRegionPrices(region, osName string, data []byte) ([]awstypes.InstanceTypeInfo, error) {
	var raw awstypes.PriceList
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	type priceKey struct {
		InstanceType string
		OS           string
	}

	best := make(map[priceKey]awstypes.InstanceTypeInfo, 1000)

	for sku, prod := range raw.Products {
		attrs := prod.Attributes
		if attrs.InstanceType == "" || attrs.VCPU == "" || attrs.Memory == "" {
			continue
		}
		if attrs.OperatingSys != osName {
			continue
		}
		// TODO: Support Dedicated tenancy if needed
		if attrs.Tenancy != "" && attrs.Tenancy != "Shared" {
			continue
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
			best[key] = awstypes.InstanceTypeInfo{
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

	infos := make([]awstypes.InstanceTypeInfo, 0, len(best))
	for _, v := range best {
		infos = append(infos, v)
	}
	return infos, nil
}

// extractOnDemandHourlyPriceForSKU returns the lowest non-zero OnDemand hourly
// price for a given SKU, filtering out non-per-hour dimensions.
func extractOnDemandHourlyPriceForSKU(terms awstypes.Terms, sku string) float64 {
	offers, ok := terms.OnDemand[sku]
	if !ok {
		return 0.0
	}
	best := 0.0
	for _, offer := range offers {
		for _, dim := range offer.PriceDimensions {
			if dim.Unit != "Hrs" {
				continue
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

// parseInt32 converts a string attribute to an int32 count. E.g. "4" -> 4.
func parseInt32(s string) (int32, error) {
	val, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, err
	}
	if val < 0 || val > math.MaxInt32 {
		return 0, fmt.Errorf("value %d out of int32 range", val)
	}
	return int32(val), nil // #nosec G109 -- value has been range-checked
}

// parseMemory converts a memory attribute string like "16 GiB" into bytes.
func parseMemory(s string) (int64, error) {
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid memory string: %q", s)
	}
	val, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory string %q: %v", parts[0], err)
	}
	switch parts[1] {
	case "GB":
		return int64(val * 1_000_000_000), nil
	case "GiB":
		return int64(val * 1_073_741_824), nil
	default:
		return 0, fmt.Errorf("unknown memory unit: %q", parts[1])
	}
}
