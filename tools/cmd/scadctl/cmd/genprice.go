// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/gardener/scaling-advisor/tools/pricing/awsprice"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/spf13/cobra"
)

var (
	providerStr string
	provider    commontypes.CloudProvider
	regions     []string
)

// Default AWS regions
var defaultAWSRegions = []string{
	"us-east-1", "us-east-2", "us-west-1", "us-west-2",
	"eu-north-1", "eu-west-1", "eu-west-2", "eu-west-3",
	"eu-central-1", "eu-south-1",
	"ap-northeast-1", "ap-northeast-2", "ap-northeast-3",
	"ap-south-1", "ap-southeast-1", "ap-southeast-2",
	"ca-central-1", "me-central-1", "sa-east-1",
}

var genpriceCmd = &cobra.Command{
	Use:   "genprice <pricing-dir>",
	Short: "obtain pricing data and write to <pricing-dir> for the given cloud provider",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(_ *cobra.Command, _ []string) (err error) {
		// Normalize providerStr into CloudProvider type
		provider, err = commontypes.AsCloudProvider(providerStr)
		return
	},
	RunE: func(_ *cobra.Command, args []string) error {
		pricingDir := args[0]
		switch provider {
		case commontypes.AWSCloudProvider:
			// If no regions specified, use defaults
			useRegions := regions
			if len(useRegions) == 0 {
				useRegions = defaultAWSRegions
			}
			return generateAWSPrices(pricingDir, useRegions)
		default:
			return fmt.Errorf("pricing not yet implemented for provider: %q", provider)
		}
	},
}

func init() {
	RootCmd.AddCommand(genpriceCmd)
	genpriceCmd.Flags().StringVarP(
		&providerStr,
		"provider", "p",
		string(commontypes.AWSCloudProvider),
		"cloud provider (aws|gcp|azure|ali|openstack)",
	)

	// --regions can be provided multiple times: -r us-east-1 -r us-west-2
	// or as comma-separated: --regions us-east-1,us-west-2
	genpriceCmd.Flags().StringSliceVarP(&regions, "regions", "r", nil, "Comma-separated list of regions")
}

// generateAWSPrices fetches EC2 instance pricing for the given regions and writes to file `aws_instance-type-infos.json` inside pricingDir.
func generateAWSPrices(pricingDir string, regions []string) error {
	if err := os.MkdirAll(pricingDir, 0o750); err != nil {
		return fmt.Errorf("failed to create pricing dir: %w", err)
	}

	var allInfos []svcapi.InstancePriceInfo
	tmpDir := os.TempDir()
	for _, region := range regions {
		regionJSONPath := path.Join(tmpDir, "aws_"+region+".json")
		data, err := os.ReadFile(filepath.Clean(regionJSONPath))
		if err != nil {
			fmt.Printf("Fetching AWS pricing for region: %s\n", region)
			data, err = awsprice.FetchRegionJSON(region)
			if err != nil {
				return fmt.Errorf("failed to fetch region %s: %w", region, err)
			}
			if err = os.WriteFile(regionJSONPath, data, 0o600); err != nil {
				return fmt.Errorf("failed to write temp region file %s: %w", regionJSONPath, err)
			}
			fmt.Printf("Written pricing info for region %q to file %s\n", region, regionJSONPath)
		}
		infos, err := awsprice.ParseRegionPrices(region, "Linux", data)
		if err != nil {
			return fmt.Errorf("failed to parse region %s: %w", region, err)
		}
		fmt.Printf("Fetched %d instance type prices for region %s\n", len(infos), region)
		allInfos = append(allInfos, infos...)
	}
	slices.SortFunc(allInfos, func(a, b svcapi.InstancePriceInfo) int {
		return cmp.Compare(a.InstanceType, b.InstanceType)
	})
	fmt.Printf("Fetched %d instance type prices across %d region(s)\n", len(allInfos), len(regions))
	outputFile := filepath.Join(pricingDir, "aws_instance-type-infos.json")
	return writeInstanceTypeInfos(outputFile, allInfos)
}

func writeInstanceTypeInfos(path string, infos []svcapi.InstancePriceInfo) error {
	f, err := os.Create(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(infos); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	fmt.Printf("AWS pricing data written to %s\n", path)
	return nil
}
