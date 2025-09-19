// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package awsprice

import (
	"fmt"
	"io"
	"net/http"
)

// FetchRegionJSON downloads the raw JSON pricing data for a given region.
func FetchRegionJSON(region string) ([]byte, error) {
	url := fmt.Sprintf("https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/%s/index.json", region)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http get failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d from %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}
