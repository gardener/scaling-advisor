// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	apiv1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const caReleaseAssetsPrefix = "https://github.com/kubernetes/autoscaler/"

var caSetupCmd = &cobra.Command{
	Use: "cluster-autoscaler <setup-opts>",
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		fmt.Println("ca setup called")
		var cas caSetup

		if !skipScalarBuild {
			if err = cas.BuildScaler(context.TODO(), version, "cluster-autoscaler"); err != nil {
				return fmt.Errorf("error building ca source: %v", err)
			}
		}

		if err = cas.GenerateKwokData(context.TODO(), scenarioDir, outputDir); err != nil {
			return fmt.Errorf("error generating kwok data for ca: %v", err)
		}

		return nil
	},
}

func init() {
	setupCmd.AddCommand(caSetupCmd)
}

var _ SetupScaler = (*caSetup)(nil)

type caSetup struct {
	// version     string
	// dataDir     string
	// scenarioDir string
}

// TODO: maybe consider using helm charts!!!!!
func (cas *caSetup) BuildScaler(ctx context.Context, version, scaler string) error {
	fmt.Printf("%s fetchscaler called\n", scaler)
	dataDir := os.TempDir()
	unzippedPath, err := getAssets(ctx, version, scaler, dataDir)
	if err != nil {
		return err
	}
	fmt.Printf("Unzipped to %q\n", path.Join(dataDir, unzippedPath))

	// TODO: ensure docker is running (ADD TAG with version)
	cmd := exec.Command("make", fmt.Sprintf("make-image-arch-%s", runtime.GOARCH))
	cmd.Dir = path.Join(dataDir, unzippedPath, "cluster-autoscaler")
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
	fmt.Printf("DEBUG(stdout): \n%s\n", stdout.String()) // FIXME add logging levels?
	return nil
}

func (cas *caSetup) GenerateKwokData(ctx context.Context, scenarioDir, outputDir string) error {
	fmt.Println("ca genKwokData called")
	err := constructKwokProviderConfig(NodegroupsConfig{
		FromNodeLabelKey: "worker.gardener.cloud/pool",
	})
	if err != nil {
		return fmt.Errorf("Could not construct the kwok provider template: %v", err)
	}

	files, err := os.ReadDir(scenarioDir)
	if err != nil {
		return fmt.Errorf("Could not open the scenario directory %q: %v", scenarioDir, err)
	}
	for _, file := range files {
		// FIXME provide cluster-scaling-constraints filename
		if !file.IsDir() && strings.Contains(file.Name(), "constraints") {
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
			err = constructKwokProviderTemplate(clusterScalingConstraint)
			if err != nil {
				return fmt.Errorf("Could not construct the kwok provider template: %v", err)
			}

		}
	}

	return nil
}

func constructKwokProviderConfig(nodegroupsConfig NodegroupsConfig) error {
	var kwokProviderConfig KwokProviderConfig
	kwokProviderConfig.APIVersion = "v1alpha"
	kwokProviderConfig.ReadNodesFrom = "configmap"
	kwokProviderConfig.Nodegroups = &nodegroupsConfig
	kwokProviderConfig.ConfigMap = &ConfigMapConfig{Name: "kwok-provider-templates"}
	kwokProviderConfig.Nodes = &NodeConfig{
		SkipTaint: true,
	}
	// kwokProviderConfig.Nodes.SkipTaint = true // TODO check if fixes

	providerConfigYaml, err := yaml.Marshal(kwokProviderConfig)
	if err != nil {
		panic(err)
	}

	providerConfig := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "kwok-provider-config",
			Namespace: "default",
		},
		Data: map[string]string{
			"config": string(providerConfigYaml),
		},
	}
	err = saveYamlToFile(providerConfig, path.Join(outputDir, "ca-kwok-provider-config.yaml"))
	if err != nil {
		return fmt.Errorf("Could not save the kwok provider template configmap: %v", err)
	}
	fmt.Printf("Saved to %s\n", path.Join(outputDir, "ca-kwok-provider-config.yaml"))
	return nil

}

func constructKwokProviderTemplate(csc apiv1alpha1.ClusterScalingConstraint) error {
	var kwokTemplates KwokProviderTemplates
	kwokTemplates.APIVersion = "v1"
	kwokTemplates.Kind = "List"
	for _, nodePool := range csc.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			node := corev1.Node{
				TypeMeta: v1.TypeMeta{
					Kind:       "Node",
					APIVersion: "v1",
				},
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
					Allocatable: nodeutil.ComputeAllocatable(nodeTemplate.Capacity, nodeTemplate.SystemReserved, nodeTemplate.SystemReserved),
					Phase:       corev1.NodeRunning,
				},
			}
			if node.Annotations == nil {
				// Needed to fix null annotations panic in CA kwok
				node.Annotations = make(map[string]string)
				node.Annotations["kwok.x-k8s.io/node"] = "fake"
			}
			// Fixes `NodeResourcesFit` "Too Many Pods" scheduling failure
			if node.Status.Allocatable.Pods().Cmp(*resource.NewQuantity(0, resource.DecimalSI)) == 0 {
				node.Status.Allocatable["pods"] = *resource.NewQuantity(110, resource.DecimalSI)
			}
			if node.Status.Capacity.Pods().Cmp(*resource.NewQuantity(0, resource.DecimalSI)) == 0 {
				node.Status.Capacity["pods"] = *resource.NewQuantity(110, resource.DecimalSI)
			}
			kwokTemplates.Items = append(kwokTemplates.Items, node)
		}
	}
	templateYaml, err := yaml.Marshal(kwokTemplates)
	if err != nil {
		return fmt.Errorf("Could not construct the kwok provider template: %v", err)
	}
	providerTemplate := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "kwok-provider-templates",
			Namespace: "default",
		},
		Data: map[string]string{
			"templates": string(templateYaml),
		},
	}

	outFileName := path.Join(outputDir, "ca-kwok-provider-template.yaml")
	err = saveYamlToFile(providerTemplate, outFileName)
	if err != nil {
		return fmt.Errorf("Could not save the kwok provider template configmap: %v", err)
	}
	fmt.Printf("Saved to %s\n", outFileName)
	return nil
}

// Helper functions --------------------------------------------------

func saveYamlToFile(data any, path string) error {
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal to yaml: %w", err)
	}

	return os.WriteFile(filepath.Clean(path), yamlData, 0644)
}

func getCAAssetsURL(version string) (string, error) {
	switch {
	case versionPattern.MatchString(version):
		return caReleaseAssetsPrefix + "archive/refs/tags/cluster-autoscaler-" + version + ".zip", nil
	case commitPattern.MatchString(version):
		return caReleaseAssetsPrefix + "archive/" + version + ".zip", nil
	case version == "master" || version == "main":
		return caReleaseAssetsPrefix + "archive/refs/heads/master.zip", nil
	default:
		return "", fmt.Errorf("Could not get the assets URL for the provided version: %q", version)
	}
}

func extractTimestamp(filename string) (string, error) {
	re := regexp.MustCompile(`(\d{8}T\d{6}Z)`)
	matches := re.FindStringSubmatch(filename)
	if len(matches) < 2 {
		return "", fmt.Errorf("timestamp not found")
	}
	return matches[1], nil
}
