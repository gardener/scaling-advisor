// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package setup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	bench "github.com/gardener/scaling-advisor/bench/cmd"

	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cakwok "k8s.io/autoscaler/cluster-autoscaler/cloudprovider/kwok"
	sigyaml "sigs.k8s.io/yaml"
)

var _ SetupScaler = (*caSetup)(nil)

type caSetup struct{}

func (cas *caSetup) BuildScaler(ctx context.Context, version string) error {
	imageName := fmt.Sprintf("gcr.io/k8s-staging-autoscaling/cluster-autoscaler-arm64:%s", version)
	if exists := bench.CheckIfImageExists(imageName); exists {
		return nil
	}

	unzippedPath, err := bench.GetAssets(ctx, version, bench.ScalerClusterAutoscaler, os.TempDir())
	if err != nil {
		return err
	}
	sourceDir := path.Join(os.TempDir(), unzippedPath)
	fmt.Printf("Unzipped to %q\n", sourceDir)

	return buildCAImage(ctx, sourceDir, version)
}

func (cas *caSetup) GenerateKwokData(_ context.Context, constraintsFile, outputDir string) error {
	if err := constructKwokProviderConfig(outputDir); err != nil {
		return fmt.Errorf("cannot construct the kwok provider config: %v", err)
	}

	constraint, err := bench.LoadJSONFromFile[sacorev1alpha1.ScalingConstraint](constraintsFile)
	if err != nil {
		return fmt.Errorf("cannot load scaling constraint: %v", err)
	}

	if err := constructKwokProviderTemplate(constraint, outputDir); err != nil {
		return fmt.Errorf("cannot construct the kwok provider template: %v", err)
	}

	return nil
}

// buildCAImage runs `make make-image` inside the CA source tree to produce the
// Docker image used by the KWOK cluster.
func buildCAImage(ctx context.Context, sourceDir, version string) error {
	cmd := exec.CommandContext(ctx, "make", "make-image", fmt.Sprintf("TAG=%s", version))
	cmd.Dir = path.Join(sourceDir, bench.ScalerClusterAutoscaler)

	var stderr, stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	fmt.Printf("Building %s\n", sourceDir)
	fmt.Printf("Running %q in %q\n", cmd.String(), cmd.Dir)

	if err := cmd.Run(); err != nil {
		capturedError := strings.TrimSpace(stderr.String())
		if capturedError != "" {
			return fmt.Errorf("command failed: %s (stderr: %s)", err, capturedError)
		}
		return fmt.Errorf("command failed: %w", err)
	}
	fmt.Printf("DEBUG(stdout): \n%s\n", stdout.String())
	return nil
}

// constructKwokProviderConfig builds the kwok-provider-config ConfigMap that
// tells the CA KWOK cloud-provider where to find node templates and how
// nodegroups are identified.
func constructKwokProviderConfig(outputDir string) error {
	nodegroupsConfig := cakwok.NodegroupsConfig{
		FromNodeLabelKey: "worker.gardener.cloud/pool",
	}

	var kwokProviderConfig cakwok.KwokProviderConfig
	kwokProviderConfig.APIVersion = "v1alpha"
	kwokProviderConfig.ReadNodesFrom = "configmap"
	kwokProviderConfig.Nodegroups = &nodegroupsConfig
	kwokProviderConfig.ConfigMap = &cakwok.ConfigMapConfig{Name: "kwok-provider-templates"}
	kwokProviderConfig.Nodes = &cakwok.NodeConfig{
		SkipTaint: true,
	}

	providerConfigYaml, err := sigyaml.Marshal(kwokProviderConfig)
	if err != nil {
		return fmt.Errorf("cannot marshal kwok provider config to YAML: %w", err)
	}

	providerConfig := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "kwok-provider-config",
			Namespace: "default",
		},
		Data: map[string]string{
			"config": string(providerConfigYaml),
		},
	}
	providerConfig.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))

	outFilePath := path.Join(outputDir, bench.FileNameCAKwokProviderConfig)
	if err := bench.SaveYamlToFile(providerConfig, outFilePath); err != nil {
		return fmt.Errorf("cannot save the kwok provider config configmap: %v", err)
	}
	fmt.Printf("Saved kwok provider config to %s\n", outFilePath)
	return nil
}

// constructKwokProviderTemplate builds the kwok-provider-templates ConfigMap
// that contains one Node object per machine type so the CA KWOK cloud-provider
// knows what capacity each nodegroup offers.
func constructKwokProviderTemplate(constraint sacorev1alpha1.ScalingConstraint, outputDir string) error {
	var kwokTemplates corev1.NodeList
	kwokTemplates.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("List"))

	for _, nodePool := range constraint.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			node := buildTemplateNode(nodePool, nodeTemplate)
			kwokTemplates.Items = append(kwokTemplates.Items, node)
		}
	}

	templateYaml, err := sigyaml.Marshal(kwokTemplates)
	if err != nil {
		return fmt.Errorf("cannot marshal kwok provider templates: %v", err)
	}

	providerTemplate := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "kwok-provider-templates",
			Namespace: "default",
		},
		Data: map[string]string{
			"templates": string(templateYaml),
		},
	}
	providerTemplate.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))

	outPath := path.Join(outputDir, bench.FileNameCAKwokProviderTemplate)
	if err := bench.SaveYamlToFile(providerTemplate, outPath); err != nil {
		return fmt.Errorf("cannot save the kwok provider template configmap: %v", err)
	}
	fmt.Printf("Saved kwok provider templates to %s\n", outPath)
	return nil
}

// buildTemplateNode creates a single corev1.Node from a nodepool/template pair,
// suitable for inclusion in the kwok-provider-templates ConfigMap.
func buildTemplateNode(nodePool sacorev1alpha1.NodePool, nodeTemplate sacorev1alpha1.NodeTemplate) corev1.Node {
	annotations := nodePool.Annotations
	if annotations == nil {
		// Needed to fix null annotations panic in CA kwok
		annotations = make(map[string]string)
	}
	annotations["kwok.x-k8s.io/node"] = "fake"

	node := corev1.Node{
		ObjectMeta: v1.ObjectMeta{
			Name:        nodeTemplate.Name,
			Labels:      nodePool.Labels,
			Annotations: annotations,
		},
		Spec: corev1.NodeSpec{
			Taints: nodePool.Taints,
		},
		Status: corev1.NodeStatus{
			Capacity:    nodeTemplate.Capacity,
			Allocatable: nodeutil.BuildAllocatable(nodeTemplate.Capacity, nodeTemplate.SystemReserved, nodeTemplate.SystemReserved),
			Phase:       corev1.NodeRunning,
		},
	}
	node.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Node"))
	return node
}
