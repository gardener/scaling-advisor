// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package benchutil

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	sigyaml "sigs.k8s.io/yaml"
)

const (
	// PrometheusPort is the port used to serve scalebench container metrics
	PrometheusPort = 2112

	// ScalerClusterAutoscaler specifies that the scaler being invoked is CA
	ScalerClusterAutoscaler = "cluster-autoscaler"
	// ScalerKarpenter specifies that the scaler being invoked is karpenter
	ScalerKarpenter = "karpenter"

	// FileNameCAKwokProviderTemplate is the filename used for storing CA kwok provider node templates
	FileNameCAKwokProviderTemplate = "ca-kwok-provider-template.yaml"

	// FileNameKarpenterInstanceTypes is the filename used for storing all instance types
	FileNameKarpenterInstanceTypes = "instance_types.json"
	// FileNameKarpenterNodePools is used for storing the NodePools deployed during harness execution
	FileNameKarpenterNodePools = "node_pools.yaml"
	// FileNameKarpenterNodeClasses is used for storing the KWOKNodeClasses
	FileNameKarpenterNodeClasses = "node_classes.yaml"

	// FileNamePricingData is the name to which the provided pricing file is linked to
	FileNamePricingData = "pricing-data.json"

	// OwnerDaemonSet is a constant denoting that the pod owner kind is a daemonset
	OwnerDaemonSet = "DaemonSet"
	// OwnerReplicaSet is a constant denoting that the pod owner kind is a replicaset
	OwnerReplicaSet = "ReplicaSet"
	// OwnerStatefulSet is a constant denoting that the pod owner kind is a statefulset
	OwnerStatefulSet = "StatefulSet"
	// OwnerJob is a constant denoting that the pod owner kind is a job
	OwnerJob = "Job"

	caReleaseAssetsPrefix        = "https://github.com/kubernetes/autoscaler/"
	karpenterReleaseAssetsPrefix = "https://github.com/kubernetes-sigs/karpenter/"
)

var (
	// Matches semantic version tags like v0.32.0, v1.0.0-beta.1, etc.
	versionPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[a-zA-Z0-9.-]+)?$`)
	// Matches Git commit SHA (40 character hex string)
	commitPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
)

// LoadJSONFromFile reads the file at filePath and unmarshals its contents as JSON
// into a value of type T. This eliminates the repeated open→ReadAll→Unmarshal
// boilerplate that otherwise appears at every call site.
func LoadJSONFromFile[T any](filePath string) (T, error) {
	var zero T
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return zero, fmt.Errorf("cannot read file %q: %w", filePath, err)
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, fmt.Errorf("cannot unmarshal JSON from %q: %w", filePath, err)
	}
	return result, nil
}

// LoadYAMLFromFile reads the file at filePath and unmarshals its contents as YAML
// into a value of type T. This eliminates the repeated open→ReadAll→Unmarshal
// boilerplate that otherwise appears at every call site.
func LoadYAMLFromFile[T any](filePath string) (T, error) {
	var zero T
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return zero, fmt.Errorf("cannot read file %q: %w", filePath, err)
	}
	var result T
	if err := sigyaml.Unmarshal(data, &result); err != nil {
		return zero, fmt.Errorf("cannot unmarshal YAML from %q: %w", filePath, err)
	}
	return result, nil
}

// SaveYamlToFile saves the given yaml data to the file specified by the path.
func SaveYamlToFile(data any, path string) error {
	yamlData, err := sigyaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal to yaml: %w", err)
	}

	return os.WriteFile(filepath.Clean(path), yamlData, 0600)
}

// SaveJsonToFile to saves the given json data to the file specified by the path
func SaveJsonToFile(data any, path string) error {
	file, err := os.Create(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// CheckIfDockerRunning checks if docker daemon is running by executing 'docker info'
func CheckIfDockerRunning() error {
	cmd := exec.Command("docker", "info")
	return cmd.Run()
}

// CheckIfImageExists runs "docker image inspect" to find if specified image is already present
func CheckIfImageExists(imageName string) (skipBuild bool) {
	check := exec.Command("docker", "image", "inspect", imageName)
	if err := check.Run(); err == nil {
		fmt.Printf("Docker image %q exists\n", imageName)
		return true
	}
	return false
}

func PullDockerImage(image string) error {
	if exists := CheckIfImageExists(image); exists {
		return nil
	}
	fmt.Printf("Pulling %s...\n", image)
	pull := exec.Command("docker", "pull", image)
	pull.Stdout = os.Stdout
	pull.Stderr = os.Stderr
	if err := pull.Run(); err != nil {
		return fmt.Errorf("docker pull %s: %w", image, err)
	}
	return nil
}

// GetAssets fetches the specified scaler version archive into the dataDir and then unzips it
func GetAssets(ctx context.Context, version, scaler, dataDir string) (unzippedPath string, err error) {
	var url string
	switch scaler {
	case ScalerClusterAutoscaler:
		url, err = getCAAssetsURL(version)
	case ScalerKarpenter:
		url, err = getKarpenterAssetsURL(version)
	default:
		return "", fmt.Errorf("scaling solution assets fetch support not added")
	}
	if err != nil {
		return
	}
	assetsZipFileName := path.Join(dataDir, scaler+"-"+version+".zip")
	err = downloadAssets(ctx, assetsZipFileName, url, version)
	if err != nil {
		return
	}
	unzippedPath, err = unzipSource(assetsZipFileName, dataDir)
	if err != nil {
		return
	}
	return
}

// SetupSignalHandler returns a context that can be cancelled on demand
func SetupSignalHandler() context.Context {
	quit := make(chan os.Signal, 2)
	ctx, cancel := context.WithCancel(context.Background())
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		cancel()
		<-quit
		os.Exit(1)
	}()
	return ctx
}

func downloadAssets(ctx context.Context, filepath, url, version string) error {
	// Unless the version is "master/main", try to check if the required
	// version assets are already present. If so, no need of fetching them again.
	if version != "master" && version != "main" {
		if _, err := os.Stat(filepath); err == nil {
			fmt.Printf("File %q already exists\n", filepath)
			return nil
		}
	}
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	fmt.Printf("Got the required assets: %s from %s\n", filepath, url)
	return err
}

func unzipSource(source, destination string) (path string, err error) {
	reader, err := zip.OpenReader(source)
	if err != nil {
		return
	}
	defer reader.Close()

	destination, err = filepath.Abs(destination)
	if err != nil {
		return
	}

	for _, f := range reader.File {
		err = unzipFile(f, destination)
		if err != nil {
			return
		}
	}

	if reader.File[0] != nil {
		path = reader.File[0].Name
		return
	}

	return
}

func unzipFile(f *zip.File, destination string) error {
	filePath := filepath.Join(destination, f.Name)
	if !strings.HasPrefix(filePath, filepath.Clean(destination)+string(os.PathSeparator)) {
		return fmt.Errorf("invalid file path: %s", filePath)
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(filePath, 0750)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0750); err != nil {
		return err
	}

	destinationFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	zippedFile, err := f.Open()
	if err != nil {
		return err
	}
	defer zippedFile.Close()

	if _, err := io.Copy(destinationFile, zippedFile); err != nil {
		return err
	}
	return nil
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
		return "", fmt.Errorf("cannot get the assets URL for the provided version: %q", version)
	}
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
		return "", fmt.Errorf("cannot get the assets URL for the provided version: %q", version)
	}
}
