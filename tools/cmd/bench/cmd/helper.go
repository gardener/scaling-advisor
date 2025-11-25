// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bench

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Matches semantic version tags like v0.32.0, v1.0.0-beta.1, etc.
	versionPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[a-zA-Z0-9.-]+)?$`)
	// Matches Git commit SHA (40 character hex string)
	commitPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
)

func getAssets(ctx context.Context, version, scaler, dataDir string) (unzippedPath string, err error) {
	var url string
	switch scaler {
	case "cluster-autoscaler":
		url, err = getCAAssetsURL(version)
	case "karpenter":
		url, err = getKarpenterAssetsURL(version)
	default:
		return "", fmt.Errorf("Scaling solution assets fetch support not added")
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

	if reader.Reader.File[0] != nil {
		return reader.Reader.File[0].Name, nil
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
		return os.MkdirAll(filePath, os.ModePerm)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
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
