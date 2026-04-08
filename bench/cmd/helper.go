// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bench

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	sigyaml "sigs.k8s.io/yaml"
)

var (
	// Matches semantic version tags like v0.32.0, v1.0.0-beta.1, etc.
	versionPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[a-zA-Z0-9.-]+)?$`)
	// Matches Git commit SHA (40 character hex string)
	commitPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
)

const (
	caReleaseAssetsPrefix        = "https://github.com/kubernetes/autoscaler/"
	karpenterReleaseAssetsPrefix = "https://github.com/kubernetes-sigs/karpenter/"
)

// SaveYamlToFile to saves the given yaml data to the file specified by the path
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

// CheckIfImageExists runs "docker image inspect" to find if specified image is already present
func CheckIfImageExists(imageName string) (skipBuild bool) {
	check := exec.Command("docker", "image", "inspect", imageName)
	if err := check.Run(); err == nil {
		fmt.Printf("Docker image %q exists\n", imageName)
		return true
	}
	return false
}

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
	err = downloadAssets(assetsZipFileName, url, version)
	if err != nil {
		return
	}
	unzippedPath, err = unzipSource(assetsZipFileName, dataDir)
	if err != nil {
		return
	}
	return
}

func downloadAssets(filepath, url, version string) error {
	if version != "master" && version != "main" {
		if _, err := os.Stat(filepath); err == nil {
			fmt.Printf("File %q already exists\n", filepath)
			return nil
		}
	}
	out, err := os.Create(filepath) // Check if existing
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
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

func unzipSource(source, destination string) (string, error) {
	reader, err := zip.OpenReader(source)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	destination, err = filepath.Abs(destination)
	if err != nil {
		return "", err
	}

	for _, f := range reader.File {
		err = unzipFile(f, destination)
		if err != nil {
			return "", err
		}
	}

	if reader.File[0] != nil {
		return reader.File[0].Name, nil
	} else {
		return "", nil
	}
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
