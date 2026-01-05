// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	apiv1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
)

const karpenterReleaseAssetsPrefix = "https://github.com/kubernetes-sigs/karpenter/"

var karpenterSetupCmd = &cobra.Command{
	Use: "karpenter <setup-opts>",
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		fmt.Println("karpenter setup called")
		var ks karpenterSetup

		if !skipScalarBuild {
			if err = ks.BuildScaler(context.TODO(), version, "karpenter"); err != nil {
				return fmt.Errorf("error building karpenter source: %v", err)
			}
		}

		if err = ks.GenerateKwokData(context.TODO(), scenarioDir, outputDir); err != nil {
			return fmt.Errorf("error generating kwok data for karpenter: %v", err)
		}

		return nil
	},
}

func init() {
	setupCmd.AddCommand(karpenterSetupCmd)
}

var _ SetupScaler = (*karpenterSetup)(nil)

type karpenterSetup struct {
	// version     string
	// dataDir     string
	// scenarioDir string
}

func (ks *karpenterSetup) BuildScaler(ctx context.Context, version, scaler string) error {
	fmt.Printf("%s fetchscaler called\n", scaler)
	dataDir := os.TempDir()
	unzippedPath, err := getAssets(ctx, version, scaler, dataDir)
	if err != nil {
		return err
	}
	fmt.Printf("Unzipped to %q\n", path.Join(dataDir, unzippedPath))
	// cmd := exec.Command("sh", "-c", "make", "build")
	// cmd.Dir = (path.Join(dataDir, unzippedPath, "cluster-autoscaler"))
	// fmt.Printf("Building %s\n", unzippedPath)
	// err = cmd.Run()
	// if err != nil {
	// 	return err
	// }
	// TODO build karpenter
	return nil
}

func (ks *karpenterSetup) GenerateKwokData(ctx context.Context, scenarioDir, outputDir string) error {
	fmt.Println("karpenter genKwokData called")

	files, err := os.ReadDir(scenarioDir)
	if err != nil {
		return fmt.Errorf("Could not open the scenario directory %q: %v", scenarioDir, err)
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasPrefix(file.Name(), "cluster-scaling-constraints-") {
			continue
		}
		file, err := os.Open(path.Join(scenarioDir, file.Name()))
		if err != nil {
			return fmt.Errorf("Could not open the scalingConstraint file %q: %v", file.Name(), err)
		}
		defer file.Close()

		scalingConstraintData, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("Could not read the scalingConstraint file %q: %v", file.Name(), err)
		}
		clusterScalingConstraint := apiv1alpha1.ClusterScalingConstraint{}
		if err := json.Unmarshal(scalingConstraintData, &clusterScalingConstraint); err != nil {
			return fmt.Errorf("Could not unmarshal the scalingConstraint data for %q: %v", file.Name(), err)
		}
		err = constructKwokInstanceTypeOptions(clusterScalingConstraint)
		if err != nil {
			return fmt.Errorf("Could not construct the kwok provider template: %v", err)
		}

	}

	return nil
}

const (
	CapacityTypeLabelKey = "karpenter.sh/capacity-type"
	CapacityTypeOnDemand = "on-demand"
)

func constructKwokInstanceTypeOptions(csc apiv1alpha1.ClusterScalingConstraint) error {
	instanceTypesSet := make(map[string]*InstanceTypeOptions, 0)
	for _, nodePool := range csc.Spec.NodePools {
		// TODO: create nodePools
		// constructKwokNodePool(csc.Spec.NodePools)

		for _, nT := range nodePool.NodeTemplates {
			var instanceType InstanceTypeOptions
			instanceType.Name = nT.InstanceType
			instanceType.Architecture = nodePool.Labels[corev1.LabelArchStable]
			// instanceType.OperatingSystems = append(instanceType.OperatingSystems, corev1.OSName(nodePool.Labels["operatingSystemName"])) // FIXME there aren't any OS labels afaik in current scenario
			instanceType.Resources = getAllocatable(nT) // FIXME: if offerings are combined, then are the resources accumulated
			currentOffering := KWOKOffering{
				Offering: Offering{
					Price:     priceFromResourcesTemp(instanceType.Resources),
					Available: true,
				},
				Requirements: []corev1.NodeSelectorRequirement{
					{
						Key:      corev1.LabelTopologyZone,
						Operator: corev1.NodeSelectorOpIn,
						Values:   nodePool.AvailabilityZones,
					},
					{
						Key:      CapacityTypeLabelKey,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{CapacityTypeOnDemand},
					},
				},
			}
			instanceType.Offerings = append(instanceType.Offerings, currentOffering)
			// check if same instance in different zone can be added to the offerings
			// TODO: how to skip the other ones when iterating later
			// for _, nTOther := range cs.AutoscalerConfig.NodeTemplates {
			// 	if instanceType.Name == nTOther.InstanceType && reflect.DeepEqual(instanceType.Resources, nTOther.Allocatable) { // FIXME this is a shit check

			// 	}
			// }
			if iT, ok := instanceTypesSet[instanceType.Name]; ok {
				iT.Offerings = append(iT.Offerings, instanceType.Offerings...)
			} else {
				instanceTypesSet[instanceType.Name] = &instanceType
			}

			// instanceTypesOption = append(instanceTypesOption, instanceType)
		}
	}
	// FIXME TODO HACK there are multiple pools with same instance type, hence the key being instance
	// type is not playing well with the nodeinstances, will need to check if configuring the nodepools
	// in karpenter kwok would allow the separation
	var instanceTypesOption []InstanceTypeOptions
	for _, item := range instanceTypesSet {
		// if skip {
		// 	continue
		// }
		// for itemTwo, skipTwo := range instanceTypesSet {
		// 	if item.Name == itemTwo.Name && reflect.DeepEqual(item.Resources, itemTwo.Resources) && !skipTwo {
		// 		item.Offerings = append(item.Offerings, itemTwo.Offerings[0]) // FIXME
		// 		instanceTypesSet[itemTwo] = true
		// 	}
		// }
		instanceTypesOption = append(instanceTypesOption, *item)
	}

	// 	// TODO save the data
	// 	// Write providerTemplate
	err := saveJsonToFile(instanceTypesOption, path.Join(outputDir, "test-kar.json"))
	if err != nil {
		return fmt.Errorf("Could not save the kwok provider template configmap: %v", err)
	}
	return nil
}

// FIXME get pricing data
func priceFromResourcesTemp(resources corev1.ResourceList) float64 {
	price := 0.0
	for k, v := range resources {
		switch k {
		case corev1.ResourceCPU:
			price += 0.025 * v.AsApproximateFloat64()
		case corev1.ResourceMemory:
			price += 0.001 * v.AsApproximateFloat64() / (1e9)
			// case ResourceGPUVendorA, ResourceGPUVendorB:
			// 	price += 1.0
		}
	}
	return price
}

func saveJsonToFile(data any, path string) error {
	file, err := os.Create(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func getAllocatable(nodeTemplate apiv1alpha1.NodeTemplate) corev1.ResourceList {
	allocatable := make(corev1.ResourceList)
	for resource, value := range nodeTemplate.Capacity {
		result := value.DeepCopy()
		if reserved, ok := nodeTemplate.KubeReserved[resource]; ok {
			result.Sub(reserved)
		}
		if reserved, ok := nodeTemplate.SystemReserved[resource]; ok {
			result.Sub(reserved)
		}
		allocatable[resource] = result
	}
	return allocatable
}

func getKarpenterAssetsURL(version string) (string, error) {
	switch {
	case versionPattern.MatchString(version):
		return karpenterReleaseAssetsPrefix + "archive/refs/tags/" + version + ".zip", nil
	case commitPattern.MatchString(version):
		return karpenterReleaseAssetsPrefix + "archive/" + version + ".zip", nil
	case version == "master" || version == "main":
		return karpenterReleaseAssetsPrefix + "archive/refs/heads/main.zip", nil
	default:
		return "", fmt.Errorf("Could not get the assets URL for the provided version: %q", version)
	}
}
