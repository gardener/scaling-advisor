// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gardener/scaling-advisor/tools/pricing/awsprice"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigyaml "sigs.k8s.io/yaml"
)

// GenInstanceTypeDefParams holds the CLI flag values for the geninstancetypedef command.
type GenInstanceTypeDefParams struct {
	ProviderStr string
	Name        string
	Namespace   string
	Regions     []string
	OnlyTypes   []string
	WithPricing bool
}

// ResourceNvidiaGPU is the Kubernetes extended resource name for NVIDIA GPU count.
const ResourceNvidiaGPU corev1.ResourceName = "nvidia.com/gpu"

var (
	genInstanceTypeDefParams GenInstanceTypeDefParams
	// gravitonRE matches AWS Graviton instance type prefixes: family letters,
	// generation digits, then 'g' (e.g. m7g, c6gd, t4g, hpc7g, im4gn).
	gravitonRE = regexp.MustCompile(`^[a-z]+[0-9]+g`)
	// defaultAWSRegions is the set of regions queried when --regions is not specified.
	defaultAWSRegions = []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"eu-north-1", "eu-west-1", "eu-west-2", "eu-west-3",
		"eu-central-1", "eu-south-1",
		"ap-northeast-1", "ap-northeast-2", "ap-northeast-3",
		"ap-south-1", "ap-southeast-1", "ap-southeast-2",
		"ca-central-1", "me-central-1", "sa-east-1",
	}
)

var geninstancetypedefCmd = &cobra.Command{
	Use:   "geninstancetypedef <output-dir>",
	Short: "fetch instance type definitions (and optionally prices) and write CRD YAMLs to <output-dir>",
	Long: `geninstancetypedef fetches EC2 instance metadata and on-demand pricing from the
AWS public price list and writes two CRD YAML files into <output-dir>:

  aws_instance-type-def-set.yaml   — InstanceTypeDefSet CR (always written)
  aws_instance-price-set.yaml      — InstancePriceSet CR  (only with --pricing)

Use --only-types to restrict output to a comma-separated list of instance type
names. Use --regions to restrict the regions fetched; defaults to a standard
set of AWS regions when omitted.`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		outputDir := args[0]
		provider, err := apicommon.AsCloudProvider(genInstanceTypeDefParams.ProviderStr)
		if err != nil {
			return err
		}
		switch provider {
		case apicommon.CloudProviderAWS:
			return generateAWSInstanceTypeData(outputDir)
		default:
			return fmt.Errorf("geninstancetypedef not yet implemented for provider: %q", provider)
		}
	},
}

func init() {
	RootCmd.AddCommand(geninstancetypedefCmd)
	geninstancetypedefCmd.Flags().StringVarP(
		&genInstanceTypeDefParams.ProviderStr, "provider", "p",
		string(apicommon.CloudProviderAWS),
		"cloud provider (aws|gcp|azure|ali|openstack)",
	)
	geninstancetypedefCmd.Flags().StringSliceVarP(
		&genInstanceTypeDefParams.Regions, "regions", "r", nil,
		"comma-separated list of regions (default: all standard AWS regions)",
	)
	geninstancetypedefCmd.Flags().StringSliceVarP(
		&genInstanceTypeDefParams.OnlyTypes, "only-types", "t", nil,
		"comma-separated list of instance type prefixes to include (e.g. m7g,c6i,t4g); matches any type whose name starts with a prefix (default: all)",
	)
	geninstancetypedefCmd.Flags().BoolVarP(
		&genInstanceTypeDefParams.WithPricing, "pricing", "P", false,
		"also write aws_instance-price-set.yaml alongside the type-def YAML",
	)
	geninstancetypedefCmd.Flags().StringVarP(
		&genInstanceTypeDefParams.Name, "name", "n", "aws",
		"ObjectMeta.name for the generated CRs",
	)
	geninstancetypedefCmd.Flags().StringVar(
		&genInstanceTypeDefParams.Namespace, "namespace", metav1.NamespaceDefault,
		"ObjectMeta.namespace for the generated CRs",
	)
}

func generateAWSInstanceTypeData(outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	regions := genInstanceTypeDefParams.Regions
	if len(regions) == 0 {
		regions = defaultAWSRegions
	}

	// Collect best (lowest price) InstanceTypeInfo per (instanceType, region) pair
	// across all fetched regions.
	type typeRegionKey struct{ instanceType, region string }
	type instanceTypeInfo struct {
		instanceType string
		region       string
		os           string
		memory       int64
		gpuMemory    int64
		hourlyPrice  float64
		vcpu         int32
		gpu          int32
	}
	best := make(map[typeRegionKey]instanceTypeInfo)

	tmpDir := os.TempDir()
	for _, region := range regions {
		regionJSONPath := filepath.Join(tmpDir, "aws_"+region+".json")
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
		}

		infos, err := awsprice.ParseRegionPrices(region, "Linux", data)
		if err != nil {
			return fmt.Errorf("failed to parse region %s: %w", region, err)
		}
		fmt.Printf("Parsed %d instance types for region %s\n", len(infos), region)

		for _, info := range infos {
			if !matchesOnlyTypes(info.InstanceType, genInstanceTypeDefParams.OnlyTypes) {
				continue
			}
			key := typeRegionKey{info.InstanceType, info.Region}
			if existing, ok := best[key]; !ok || info.HourlyPrice < existing.hourlyPrice {
				best[key] = instanceTypeInfo{
					instanceType: info.InstanceType,
					region:       info.Region,
					os:           info.OS,
					memory:       info.Memory,
					gpuMemory:    info.GPUMemory,
					hourlyPrice:  info.HourlyPrice,
					vcpu:         info.VCPU,
					gpu:          info.GPU,
				}
			}
		}
	}

	// Deduplicate instance type definitions (same type across regions has the
	// same hardware, so take just one entry per instance type name).
	type defKey struct{ instanceType string }
	typeDefs := make(map[defKey]instanceTypeInfo)
	for _, info := range best {
		k := defKey{info.instanceType}
		if _, seen := typeDefs[k]; !seen {
			typeDefs[k] = info
		}
	}

	allInfos := make([]instanceTypeInfo, 0, len(best))
	for _, v := range best {
		allInfos = append(allInfos, v)
	}
	slices.SortFunc(allInfos, func(a, b instanceTypeInfo) int {
		if n := cmp.Compare(a.instanceType, b.instanceType); n != 0 {
			return n
		}
		return cmp.Compare(a.region, b.region)
	})

	allDefs := make([]instanceTypeInfo, 0, len(typeDefs))
	for _, v := range typeDefs {
		allDefs = append(allDefs, v)
	}
	slices.SortFunc(allDefs, func(a, b instanceTypeInfo) int {
		return cmp.Compare(a.instanceType, b.instanceType)
	})

	fmt.Printf("Collected %d instance type definitions across %d (type, region) pairs\n",
		len(allDefs), len(allInfos))

	instanceTypeDefs := make([]sacorev1alpha1.InstanceTypeDef, 0, len(allDefs))
	for _, info := range allDefs {
		capacity := corev1.ResourceList{
			corev1.ResourceCPU:    *resource.NewMilliQuantity(int64(info.vcpu)*1000, resource.DecimalSI),
			corev1.ResourceMemory: *resource.NewQuantity(info.memory, resource.BinarySI),
		}
		if info.gpu > 0 {
			capacity[ResourceNvidiaGPU] = *resource.NewQuantity(int64(info.gpu), resource.DecimalSI)
		}
		instanceTypeDefs = append(instanceTypeDefs, sacorev1alpha1.InstanceTypeDef{
			Name:          info.instanceType,
			Architecture:  inferArchitecture(info.instanceType),
			BaseResources: capacity,
		})
	}

	its := sacorev1alpha1.InstanceTypeDefSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: sacorev1alpha1.SchemeGroupVersion.String(),
			Kind:       "InstanceTypeDefSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      genInstanceTypeDefParams.Name,
			Namespace: genInstanceTypeDefParams.Namespace,
		},
		Spec: sacorev1alpha1.InstanceTypeDefSetSpec{
			Provider:         apicommon.CloudProviderAWS,
			InstanceTypeDefs: instanceTypeDefs,
		},
	}
	if err := writeYAML(filepath.Join(outputDir, "aws_instance-type-def-set.yaml"), its); err != nil {
		return err
	}

	if !genInstanceTypeDefParams.WithPricing {
		return nil
	}

	prices := make([]sacorev1alpha1.InstancePrice, 0, len(allInfos))
	for _, info := range allInfos {
		prices = append(prices, sacorev1alpha1.InstancePrice{
			InstancePriceKey: sacorev1alpha1.InstancePriceKey{
				InstanceType: info.instanceType,
				Region:       info.region,
				CapacityType: apicommon.CapacityTypeOnDemand,
			},
			Price: fmt.Sprintf("%.10g", info.hourlyPrice),
		})
	}

	ips := sacorev1alpha1.InstancePriceSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: sacorev1alpha1.SchemeGroupVersion.String(),
			Kind:       "InstancePriceSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      genInstanceTypeDefParams.Name,
			Namespace: genInstanceTypeDefParams.Namespace,
		},
		Spec: sacorev1alpha1.InstancePriceSetSpec{
			Provider: apicommon.CloudProviderAWS,
			Prices:   prices,
		},
	}
	return writeYAML(filepath.Join(outputDir, "aws_instance-price-set.yaml"), ips)
}

// matchesOnlyTypes returns true when onlyTypes is empty (no filter) or when
// instanceType starts with at least one of the given prefixes, allowing callers
// to specify families (e.g. "m7g", "c6i") rather than exact names.
func matchesOnlyTypes(instanceType string, onlyTypes []string) bool {
	if len(onlyTypes) == 0 {
		return true
	}
	for _, prefix := range onlyTypes {
		if strings.HasPrefix(instanceType, prefix) {
			return true
		}
	}
	return false
}

// inferArchitecture returns "arm64" for Graviton types, "amd64" otherwise.
func inferArchitecture(instanceType string) string {
	if gravitonRE.MatchString(instanceType) {
		return "arm64"
	}
	return "amd64"
}

func writeYAML(path string, v any) error {
	data, err := sigyaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", path, err)
	}
	if err = os.WriteFile(filepath.Clean(path), data, 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	fmt.Printf("Written %s\n", path)
	return nil
}
