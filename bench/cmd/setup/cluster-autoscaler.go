// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// func init() {
// 	setupCmd.AddCommand(caSetupCmd)
// }

var _ SetupScaler = (*caSetup)(nil)

type caSetup struct {
	// version     string
	// dataDir     string
	// constraintsFile string
}

func (cas *caSetup) BuildScaler(ctx context.Context) error {
	imageName := fmt.Sprintf("gcr.io/k8s-staging-autoscaling/cluster-autoscaler-arm64:%s", version)
	if exists := bench.CheckIfImageExists(imageName); exists {
		return nil
	}

	dataDir := os.TempDir()
	unzippedPath, err := bench.GetAssets(ctx, version, bench.ScalerClusterAutoscaler, dataDir)
	if err != nil {
		return err
	}
	fmt.Printf("Unzipped to %q\n", path.Join(dataDir, unzippedPath))

	cmd := exec.Command("make", "make-image", fmt.Sprintf("TAG=%s", version))
	cmd.Dir = path.Join(dataDir, unzippedPath, bench.ScalerClusterAutoscaler)
	var stderr, stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	fmt.Printf("Building %s\n", unzippedPath)
	fmt.Printf("Running %q in %q\n", cmd.String(), cmd.Dir)
	err = cmd.Run()
	if err != nil {
		capturedError := strings.TrimSpace(stderr.String())
		if capturedError != "" {
			return fmt.Errorf("command failed: %s (stderr: %s)", err, capturedError)
		}
		return fmt.Errorf("command failed: %w", err)
	}
	fmt.Printf("DEBUG(stdout): \n%s\n", stdout.String())
	return nil
}

func (cas *caSetup) GenerateKwokData(ctx context.Context, constraintsFile, outputDir string) error {
	err := constructKwokProviderConfig(cakwok.NodegroupsConfig{
		FromNodeLabelKey: "worker.gardener.cloud/pool",
	})
	if err != nil {
		return fmt.Errorf("cannot construct the kwok provider template: %v", err)
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
	err = constructKwokProviderTemplate(clusterScalingConstraint)
	if err != nil {
		return fmt.Errorf("cannot construct the kwok provider template: %v", err)
	}

	return nil
}

func constructKwokProviderConfig(nodegroupsConfig cakwok.NodegroupsConfig) error {
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
		panic(err)
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
	err = bench.SaveYamlToFile(providerConfig, path.Join(outputDir, bench.FileNameCAKwokProviderConfig))
	if err != nil {
		return fmt.Errorf("cannot save the kwok provider template configmap: %v", err)
	}
	fmt.Printf("Saved kwok provider config to %s\n", path.Join(outputDir, bench.FileNameCAKwokProviderConfig))
	return nil
}

func constructKwokProviderTemplate(csc sacorev1alpha1.ScalingConstraint) error {
	var kwokTemplates corev1.NodeList
	kwokTemplates.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("List"))
	for _, nodePool := range csc.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			node := corev1.Node{
				ObjectMeta: v1.ObjectMeta{
					Name:        nodeTemplate.Name,
					Labels:      nodePool.Labels,
					Annotations: nodePool.Annotations,
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
			if node.Annotations == nil {
				// Needed to fix null annotations panic in CA kwok
				node.Annotations = make(map[string]string)
				node.Annotations["kwok.x-k8s.io/node"] = "fake"
			}
			node.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Node"))
			kwokTemplates.Items = append(kwokTemplates.Items, node)
		}
	}
	templateYaml, err := sigyaml.Marshal(kwokTemplates)
	if err != nil {
		return fmt.Errorf("cannot construct the kwok provider template: %v", err)
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

	outFileName := path.Join(outputDir, bench.FileNameCAKwokProviderTemplate)
	err = bench.SaveYamlToFile(providerTemplate, outFileName)
	if err != nil {
		return fmt.Errorf("cannot save the kwok provider template configmap: %v", err)
	}
	fmt.Printf("Saved kwok provider templates to %s\n", outFileName)
	return nil
}
