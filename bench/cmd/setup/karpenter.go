// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package setup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"

	bench "github.com/gardener/scaling-advisor/bench/cmd"

	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	pricingapi "github.com/gardener/scaling-advisor/api/pricing"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/pricing"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	karpenterkwokapis "sigs.k8s.io/karpenter/kwok/apis"
	karpenterkwokv1alpha1 "sigs.k8s.io/karpenter/kwok/apis/v1alpha1"
	karpenterkwok "sigs.k8s.io/karpenter/kwok/cloudprovider"
	karpenterapis "sigs.k8s.io/karpenter/pkg/apis"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	karpenter "sigs.k8s.io/karpenter/pkg/cloudprovider"
)

var pricingData pricingapi.InstancePricingAccess

// func init() {
// setupCmd.AddCommand(karpenterSetupCmd)
// }

var _ SetupScaler = (*karpenterSetup)(nil)

type karpenterSetup struct {
	// version     string
	// dataDir     string
	// constraintsFile string
}

func (ks *karpenterSetup) BuildScaler(ctx context.Context) error {
	imageName := fmt.Sprintf("karpenter.local/kwok:%s", version)
	if exists := bench.CheckIfImageExists(imageName); exists {
		return nil
	}

	dataDir := os.TempDir()
	unzippedPath, err := bench.GetAssets(ctx, version, bench.ScalerKarpenter, dataDir)
	if err != nil {
		return err
	}
	fmt.Printf("Unzipped to %q\n", path.Join(dataDir, unzippedPath))

	// Step 1: go build
	build := exec.Command("go", "build", "-o", "./kwok-bin", "./kwok/")
	build.Dir = path.Join(dataDir, unzippedPath)
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}
	fmt.Println("Build successful")

	// Step 2: create a tar.gz archive in-memory and pipe it into docker import
	binPath := filepath.Join(dataDir, unzippedPath, "kwok-bin")

	dockerImport := exec.Command("docker", "import", "--change", `ENTRYPOINT ["/kwok-bin"]`, "-", imageName)
	dockerImport.Stdout = os.Stdout
	dockerImport.Stderr = os.Stderr

	pw, err := dockerImport.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe for docker import: %w", err)
	}

	if err := dockerImport.Start(); err != nil {
		return fmt.Errorf("failed to start docker import: %w", err)
	}

	// Write tar.gz stream into the pipe
	tarErr := writeTarGz(pw, binPath)
	// Always close the pipe so docker import sees EOF, even on error
	_ = pw.Close()

	if tarErr != nil {
		// Wait for docker import to finish to avoid zombies, but return the tar error
		_ = dockerImport.Wait()
		return tarErr
	}
	if err := dockerImport.Wait(); err != nil {
		return fmt.Errorf("docker import failed: %w", err)
	}
	return nil
}

func (ks *karpenterSetup) GenerateKwokData(ctx context.Context, constraintsFile, outputDir string) (err error) {
	pricingData, err = pricing.GetInstancePricingAccess("dummy-provider", pricingFile)
	if err != nil {
		return fmt.Errorf("error parsing pricing data for karpenter: %v", err)
	}

	file, err := os.Open(constraintsFile)
	if err != nil {
		return fmt.Errorf("cannot open the scalingConstraint file %q: %v", file.Name(), err)
	}
	defer file.Close()

	scalingConstraintData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("cannot read the scalingConstraint file %q: %v", file.Name(), err)
	}
	clusterScalingConstraint := sacorev1alpha1.ScalingConstraint{}
	if err := json.Unmarshal(scalingConstraintData, &clusterScalingConstraint); err != nil {
		return fmt.Errorf("cannot unmarshal the scalingConstraint data for %q: %v", file.Name(), err)
	}
	if err = constructKwokNodePools(clusterScalingConstraint.Spec.NodePools); err != nil {
		return fmt.Errorf("cannot construct karpenter node pools: %v", err)
	}
	if err = constructKwokInstanceTypes(clusterScalingConstraint); err != nil {
		return fmt.Errorf("cannot construct the kwok provider instance types: %v", err)
	}
	return
}

func constructKwokNodePools(nodePools []sacorev1alpha1.NodePool) error {
	var karpenterNodePools karpenterv1.NodePoolList
	karpenterNodePools.SetGroupVersionKind(schema.GroupVersion{
		Group: karpenterapis.Group, Version: "v1",
	}.WithKind("NodePoolList"))
	var kwokNodeClasses karpenterkwokv1alpha1.KWOKNodeClassList
	kwokNodeClasses.SetGroupVersionKind(schema.GroupVersion{
		Group: karpenterkwokapis.Group, Version: "v1alpha1",
	}.WithKind("KWOKNodeClassList"))
	for _, nodePool := range nodePools {
		nodeClass := karpenterkwokv1alpha1.KWOKNodeClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodePool.Name,
			},
		}
		instanceTypes := make([]string, len(nodePool.NodeTemplates))
		for i, nT := range nodePool.NodeTemplates {
			instanceTypes[i] = nT.InstanceType
		}
		karpPool := karpenterv1.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodePool.Name,
			},
			Spec: karpenterv1.NodePoolSpec{
				Template: karpenterv1.NodeClaimTemplate{
					ObjectMeta: karpenterv1.ObjectMeta{
						Labels:      nodePool.Labels,
						Annotations: nodePool.Annotations,
					},
					Spec: constructNodePoolTemplateSpec(&nodePool, instanceTypes),
				},
				Disruption: karpenterv1.Disruption{
					ConsolidationPolicy: karpenterv1.ConsolidationPolicyWhenEmptyOrUnderutilized,
					// TODO the below would be set up once scaling advisor `ScaleInPolicy`
					// has been defined
					ConsolidateAfter: karpenterv1.NillableDuration{},
					Budgets:          []karpenterv1.Budget{},
				},
				Limits: karpenterv1.Limits(nodePool.Quota),
				// In case no priority is specified, weight shouldn't be zero since karpenter doesn't allow that
				Weight: ptr.To(nodePool.Priority + 1),
			},
		}
		nodeClass.SetGroupVersionKind(schema.GroupVersion{
			Group: karpenterkwokapis.Group, Version: "v1alpha1",
		}.WithKind("KWOKNodeClass"))
		kwokNodeClasses.Items = append(kwokNodeClasses.Items, nodeClass)
		karpPool.SetGroupVersionKind(schema.GroupVersion{
			Group: karpenterapis.Group, Version: "v1",
		}.WithKind("NodePool"))
		karpenterNodePools.Items = append(karpenterNodePools.Items, karpPool)
	}

	err := bench.SaveYamlToFile(&karpenterNodePools, path.Join(outputDir, bench.FileNameKarpenterNodePools))
	if err != nil {
		return fmt.Errorf("cannot save karpenter node pools: %v", err)
	}
	fmt.Printf("Saved karpenter node pools to %q\n", path.Join(outputDir, bench.FileNameKarpenterNodePools))

	err = bench.SaveYamlToFile(&kwokNodeClasses, path.Join(outputDir, bench.FileNameKarpenterNodeClasses))
	if err != nil {
		return fmt.Errorf("cannot save karpenter node classes: %v", err)
	}
	fmt.Printf("Saved karpenter node classes to %q\n", path.Join(outputDir, bench.FileNameKarpenterNodeClasses))
	return nil
}

func constructNodePoolTemplateSpec(nodePool *sacorev1alpha1.NodePool, instanceTypes []string) karpenterv1.NodeClaimTemplateSpec {
	return karpenterv1.NodeClaimTemplateSpec{
		Taints: nodePool.Taints,
		Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{
			{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      corev1.LabelInstanceTypeStable,
					Operator: corev1.NodeSelectorOpIn,
					Values:   instanceTypes,
				},
				MinValues: ptr.To(len(nodePool.NodeTemplates)),
			},
			{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      corev1.LabelTopologyZone,
					Operator: corev1.NodeSelectorOpIn,
					Values:   nodePool.AvailabilityZones,
				},
			},
		},
		NodeClassRef: &karpenterv1.NodeClassReference{
			Kind:  "KWOKNodeClass",
			Name:  nodePool.Name,
			Group: karpenterkwokapis.Group,
		},
	}
}

func constructKwokInstanceTypes(csc sacorev1alpha1.ScalingConstraint) error {
	instanceTypes := []karpenterkwok.InstanceTypeOptions{}
	for _, nodePool := range csc.Spec.NodePools {
		for _, nT := range nodePool.NodeTemplates {
			var newInstance karpenterkwok.InstanceTypeOptions
			newInstance.Name = nT.InstanceType
			newInstance.Architecture = nodePool.Labels[corev1.LabelArchStable]
			newInstance.Resources = nodeutil.BuildAllocatable(nT.Capacity, nT.SystemReserved, nT.KubeReserved)
			if idx := findInstanceIndex(instanceTypes, newInstance); idx != -1 {
				// If resources are different, it can be the case that one has reserved capacity
				// specified so we pick the capacity with lesser resources to ensure accuracy.
				if !objutil.IsResourceListEqual(instanceTypes[idx].Resources, newInstance.Resources) {
					instanceTypes[idx].Resources = objutil.MinResourceListQuantity(
						instanceTypes[idx].Resources, newInstance.Resources,
					)
				}
				// instanceType.OperatingSystems = append(instanceType.OperatingSystems, corev1.OSName(nodePool.Labels["operatingSystemName"])) // FIXME there aren't any OS labels afaik in current scenario
				for i, req := range instanceTypes[idx].Offerings[0].Requirements {
					if req.Key == corev1.LabelTopologyZone {
						for _, zone := range nodePool.AvailabilityZones {
							// there are zones for this instance that haven't been added
							if !slices.Contains(req.Values, zone) {
								instanceTypes[idx].Offerings[0].Requirements[i].Values = append(
									instanceTypes[idx].Offerings[0].Requirements[i].Values, zone,
								)
							}
						}
					}
				}
				continue
			}
			instancePricingData, err := pricingData.GetInfo(nodePool.Region, newInstance.Name)
			if err != nil {
				fmt.Printf("cannot find pricing data for instance %s in region %s: %v\n", newInstance.Name, nodePool.Region, err)
				continue
			}
			newOffering := constructNewOffering(nodePool.AvailabilityZones, instancePricingData.HourlyPrice)
			newInstance.Offerings = append(newInstance.Offerings, newOffering)
			instanceTypes = append(instanceTypes, newInstance)
		}
	}
	err := bench.SaveJsonToFile(instanceTypes, path.Join(outputDir, bench.FileNameKarpenterInstanceTypes))
	if err != nil {
		return fmt.Errorf("cannot save the kwok provider instace types: %v", err)
	}
	fmt.Printf("Saved instances json to %q\n", path.Join(outputDir, bench.FileNameKarpenterInstanceTypes))
	return nil
}

// To check if an instance offering has already been created, we just need to ensure that
// these parameters are the same for the currently processed nodePool and nodeTemplate:
//  1. InstanceType
//  2. Architecture
//
// If only OS differs, it can be added to the list of existing instance OSses.
// If resources differ, we pick the minimum resources.
// If only the AZ differs, that can be also added as to existing kwokOffering.
// When there's support for non on-demand instances, then that also would append the
// new requirement to the existing instance offering.
func findInstanceIndex(instances []karpenterkwok.InstanceTypeOptions, candidate karpenterkwok.InstanceTypeOptions) int {
	return slices.IndexFunc(instances, func(existing karpenterkwok.InstanceTypeOptions) bool {
		return existing.Name == candidate.Name && existing.Architecture == candidate.Architecture
	})
}

func constructNewOffering(availabilityZones []string, instanceHourlyPrice float64) karpenterkwok.KWOKOffering {
	return karpenterkwok.KWOKOffering{
		Offering: karpenter.Offering{
			Price:     instanceHourlyPrice,
			Available: true,
		},
		Requirements: []corev1.NodeSelectorRequirement{
			{
				Key:      corev1.LabelTopologyZone,
				Operator: corev1.NodeSelectorOpIn,
				Values:   availabilityZones,
			},
			{
				Key:      karpenterv1.CapacityTypeLabelKey,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{karpenterv1.CapacityTypeOnDemand}, // Hardcoded to on-demand for now
			},
		},
	}
}

// writeTarGz writes a gzip-compressed tar archive containing the file at
// filePath (stored under its base name) to the provided writer.
func writeTarGz(w io.Writer, filePath string) error {
	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)

	fi, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", filepath.Base(filePath), err)
	}
	hdr := &tar.Header{
		Name: filepath.Base(filePath),
		Mode: 0755,
		Size: fi.Size(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}
	f, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", filepath.Base(filePath), err)
	}
	defer f.Close()
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("failed to write %s to tar: %w", filepath.Base(filePath), err)
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}
	return nil
}
