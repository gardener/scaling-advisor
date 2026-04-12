// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package setup

import (
	"archive/tar"
	"compress/gzip"
	"context"
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

var _ SetupScaler = (*karpenterSetup)(nil)

// karpenterSetup holds the karpenter-specific configuration that is not shared
// with other scalers. The pricingFile field is set by getScaler in setup.go so
// that GenerateKwokData never has to reach for a package-level variable.
type karpenterSetup struct {
	pricingFile string
}

func (ks *karpenterSetup) BuildScaler(ctx context.Context, version string) error {
	imageName := fmt.Sprintf("karpenter.local/kwok:%s", version)
	if exists := bench.CheckIfImageExists(imageName); exists {
		return nil
	}

	unzippedPath, err := bench.GetAssets(ctx, version, bench.ScalerKarpenter, os.TempDir())
	if err != nil {
		return err
	}
	sourceDir := path.Join(os.TempDir(), unzippedPath)
	fmt.Printf("Unzipped to %q\n", sourceDir)

	// This is done rather than relying on the make target which uses an
	// external build program "ko".
	binPath, err := buildKarpenterBinary(ctx, sourceDir)
	if err != nil {
		return err
	}

	return importKarpenterImage(binPath, imageName)
}

// GenerateKwokData reads the scaling constraints and pricing information to
// produce the three artefacts that the Karpenter KWOK provider needs:
//   - NodePool list          (node_pools.yaml)
//   - KWOKNodeClass list     (node_classes.yaml)
//   - InstanceType options   (instance_types.json)
func (ks *karpenterSetup) GenerateKwokData(_ context.Context, constraintsFile, outputDir string) error {
	pricingData, err := pricing.GetInstancePricingAccess("dummy-provider", ks.pricingFile)
	if err != nil {
		return fmt.Errorf("error parsing pricing data for karpenter: %v", err)
	}

	constraint, err := bench.LoadJSONFromFile[sacorev1alpha1.ScalingConstraint](constraintsFile)
	if err != nil {
		return fmt.Errorf("cannot load scaling constraint: %v", err)
	}

	if err := constructKwokNodePools(constraint.Spec.NodePools, outputDir); err != nil {
		return fmt.Errorf("cannot construct karpenter node pools: %v", err)
	}
	if err := constructKwokInstanceTypes(constraint, pricingData, outputDir); err != nil {
		return fmt.Errorf("cannot construct the kwok provider instance types: %v", err)
	}

	return nil
}

// buildKarpenterBinary cross-compiles the Karpenter KWOK binary inside
// sourceDir and returns the absolute path to the produced executable.
func buildKarpenterBinary(ctx context.Context, sourceDir string) (string, error) {
	build := exec.CommandContext(ctx, "go", "build", "-o", "./kwok-bin", "./kwok/")
	build.Dir = sourceDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr

	if err := build.Run(); err != nil {
		return "", fmt.Errorf("go build failed: %w", err)
	}
	fmt.Println("Build successful")

	return filepath.Join(sourceDir, "kwok-bin"), nil
}

// importKarpenterImage creates a minimal Docker image from the compiled binary
// by piping a tar.gz archive into `docker import`.
func importKarpenterImage(binPath, imageName string) error {
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

	// Write tar.gz stream into the pipe.
	tarErr := writeTarGz(pw, binPath)
	// Always close the pipe so docker import sees EOF, even on error.
	_ = pw.Close()

	if tarErr != nil {
		// Wait for docker import to finish to avoid zombies, but return the
		// tar error since it is the root cause.
		_ = dockerImport.Wait()
		return tarErr
	}
	if err := dockerImport.Wait(); err != nil {
		return fmt.Errorf("docker import failed: %w", err)
	}
	return nil
}

// constructKwokNodePools builds the Karpenter NodePool and KWOKNodeClass lists
// from the scaling-advisor node pool definitions and writes them to outputDir.
func constructKwokNodePools(nodePools []sacorev1alpha1.NodePool, outputDir string) error {
	var karpenterNodePools karpenterv1.NodePoolList
	karpenterNodePools.SetGroupVersionKind(schema.GroupVersion{
		Group: karpenterapis.Group, Version: "v1",
	}.WithKind("NodePoolList"))

	var kwokNodeClasses karpenterkwokv1alpha1.KWOKNodeClassList
	kwokNodeClasses.SetGroupVersionKind(schema.GroupVersion{
		Group: karpenterkwokapis.Group, Version: "v1alpha1",
	}.WithKind("KWOKNodeClassList"))

	for _, nodePool := range nodePools {
		pool, class := buildNodePoolAndClass(nodePool)
		karpenterNodePools.Items = append(karpenterNodePools.Items, pool)
		kwokNodeClasses.Items = append(kwokNodeClasses.Items, class)
	}

	poolsPath := path.Join(outputDir, bench.FileNameKarpenterNodePools)
	if err := bench.SaveYamlToFile(&karpenterNodePools, poolsPath); err != nil {
		return fmt.Errorf("cannot save karpenter node pools: %v", err)
	}
	fmt.Printf("Saved karpenter node pools to %q\n", poolsPath)

	classesPath := path.Join(outputDir, bench.FileNameKarpenterNodeClasses)
	if err := bench.SaveYamlToFile(&kwokNodeClasses, classesPath); err != nil {
		return fmt.Errorf("cannot save karpenter node classes: %v", err)
	}
	fmt.Printf("Saved karpenter node classes to %q\n", classesPath)

	return nil
}

// buildNodePoolAndClass translates a single scaling-advisor NodePool into the
// corresponding Karpenter NodePool and KWOKNodeClass pair.
func buildNodePoolAndClass(nodePool sacorev1alpha1.NodePool) (karpenterv1.NodePool, karpenterkwokv1alpha1.KWOKNodeClass) {
	instanceTypes := make([]string, len(nodePool.NodeTemplates))
	for i, nodeTemplate := range nodePool.NodeTemplates {
		instanceTypes[i] = nodeTemplate.InstanceType
	}

	pool := karpenterv1.NodePool{
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
				// TODO: the below would be set up once scaling advisor
				// `ScaleInPolicy` has been defined.
				ConsolidateAfter: karpenterv1.NillableDuration{},
				Budgets:          []karpenterv1.Budget{},
			},
			Limits: karpenterv1.Limits(nodePool.Quota),
			// Karpenter forbids a zero weight; offset by 1 so pools with no
			// explicit priority still get a valid value.
			Weight: ptr.To(nodePool.Priority + 1),
		},
	}
	pool.SetGroupVersionKind(schema.GroupVersion{
		Group: karpenterapis.Group, Version: "v1",
	}.WithKind("NodePool"))

	class := karpenterkwokv1alpha1.KWOKNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodePool.Name,
		},
	}
	class.SetGroupVersionKind(schema.GroupVersion{
		Group: karpenterkwokapis.Group, Version: "v1alpha1",
	}.WithKind("KWOKNodeClass"))

	return pool, class
}

// constructNodePoolTemplateSpec builds the NodeClaimTemplateSpec that sits
// inside a Karpenter NodePool, selecting the allowed instance types and
// availability zones.
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

// constructKwokInstanceTypes iterates over every node-pool/template combination
// in the scaling constraint, deduplicates by (name, architecture), enriches
// each entry with pricing data, and writes the result to outputDir.
func constructKwokInstanceTypes(
	constraint sacorev1alpha1.ScalingConstraint,
	pricingData pricingapi.InstancePricingAccess,
	outputDir string,
) error {
	instanceTypes := collectInstanceTypes(constraint, pricingData)

	outPath := path.Join(outputDir, bench.FileNameKarpenterInstanceTypes)
	if err := bench.SaveJsonToFile(instanceTypes, outPath); err != nil {
		return fmt.Errorf("cannot save the kwok provider instance types: %v", err)
	}
	fmt.Printf("Saved instances json to %q\n", outPath)
	return nil
}

// collectInstanceTypes walks all node-pools and their templates, building a
// deduplicated slice of InstanceTypeOptions. When the same (name, arch) pair
// appears in multiple pools, the entries are merged: resources are shrunk to
// the minimum and availability zones are unioned.
func collectInstanceTypes(
	constraint sacorev1alpha1.ScalingConstraint,
	pricingData pricingapi.InstancePricingAccess,
) []karpenterkwok.InstanceTypeOptions {
	var instanceTypes []karpenterkwok.InstanceTypeOptions

	for _, nodePool := range constraint.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			candidate := karpenterkwok.InstanceTypeOptions{
				Name:         nodeTemplate.InstanceType,
				Architecture: nodePool.Labels[corev1.LabelArchStable],
				Resources:    nodeutil.BuildAllocatable(nodeTemplate.Capacity, nodeTemplate.SystemReserved, nodeTemplate.KubeReserved),
			}

			if idx := findInstanceIndex(instanceTypes, candidate); idx != -1 {
				mergeInstanceType(&instanceTypes[idx], candidate, nodePool.AvailabilityZones)
				continue
			}

			instancePricing, err := pricingData.GetInfo(nodePool.Region, candidate.Name)
			if err != nil {
				fmt.Printf("cannot find pricing data for instance %s in region %s: %v\n",
					candidate.Name, nodePool.Region, err)
				continue
			}

			candidate.Offerings = []karpenterkwok.KWOKOffering{
				constructNewOffering(nodePool.AvailabilityZones, instancePricing.HourlyPrice),
			}
			instanceTypes = append(instanceTypes, candidate)
		}
	}

	return instanceTypes
}

// mergeInstanceType folds a duplicate (name, arch) candidate into an existing
// entry: resources are reduced to the per-field minimum (to account for
// differing reserved capacity) and any new availability zones are appended to
// the first offering.
func mergeInstanceType(
	existing *karpenterkwok.InstanceTypeOptions,
	candidate karpenterkwok.InstanceTypeOptions,
	availabilityZones []string,
) {
	// If resources differ between pools it can be because one specifies
	// reserved capacity; pick the smaller value per resource to stay safe.
	if !objutil.IsResourceListEqual(existing.Resources, candidate.Resources) {
		existing.Resources = objutil.MinResourceListQuantity(
			existing.Resources, candidate.Resources,
		)
	}

	// Add any availability zones that are not yet present in
	// the first offering of the given instance type.
	for i, req := range existing.Offerings[0].Requirements {
		if req.Key != corev1.LabelTopologyZone {
			continue
		}
		for _, zone := range availabilityZones {
			if !slices.Contains(req.Values, zone) {
				existing.Offerings[0].Requirements[i].Values = append(
					existing.Offerings[0].Requirements[i].Values, zone,
				)
			}
		}
		break
	}
}

// findInstanceIndex returns the position of an instance with the same name and
// architecture as candidate, or -1 if none exists.
//
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

// constructNewOffering builds a KWOKOffering for an on-demand instance in the
// given availability zones at the specified hourly price.
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
				Values:   []string{karpenterv1.CapacityTypeOnDemand}, // Hardcoded to on-demand for now.
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
