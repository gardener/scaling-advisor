// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"os"
	"path/filepath"

	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

const (
	testdataPath = "testutil/testdata"
	BindingPodA  = "binding-pod-a.json"
	CorruptPodA  = "corrupt-pod-a.json"
	EventA       = "event-a.json"
	NameMissPodA = "name-miss-pod-a.json"
	NameNodeA    = "name-node-a.json"
	NodeA        = "node-a.json"
	PatchPodA    = "patch-pod-a.json"
	PodA         = "pod-a.json"
	PodDefaultNS = "pod-defaultns.json"
	PodTestNS    = "pod-testns.json"
	UidTSPodA    = "uid-ts-pod-a.json"
	UpdatePodA   = "update-pod-a.json"
)

var (
	defaultBaseViewArgs = &mkapi.ViewArgs{
		Name:           mkapi.DefaultBasePrefix,
		KubeConfigPath: "/tmp/minkapi-test.yaml",
		Scheme:         typeinfo.SupportedScheme,
		WatchConfig: mkapi.WatchConfig{
			QueueSize: mkapi.DefaultWatchQueueSize,
			Timeout:   mkapi.DefaultWatchTimeout,
		},
	}
	defaultSandboxViewArgs = &mkapi.ViewArgs{
		Name:           "sandbox",
		KubeConfigPath: "sandbox",
		Scheme:         typeinfo.SupportedScheme,
		WatchConfig: mkapi.WatchConfig{
			QueueSize: mkapi.DefaultWatchQueueSize,
			Timeout:   mkapi.DefaultWatchTimeout,
		},
	}
	testObjects map[string]metav1.Object
)

func init() {
	decoder := serializer.NewCodecFactory(typeinfo.SupportedScheme).UniversalDeserializer()
	entries, err := os.ReadDir(testdataPath)
	if err != nil {
		return
	}
	testObjects = make(map[string]metav1.Object, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		data, err := os.ReadFile(filepath.Join(testdataPath, name))
		if err != nil {
			continue
		}
		obj, _, err := decoder.Decode(data, nil, nil)
		if err != nil {
			continue
		}
		mo, ok := obj.(metav1.Object)
		if !ok {
			continue
		}
		testObjects[name] = mo
	}
}

func GetDefaultBaseViewArgs() *mkapi.ViewArgs {
	return defaultBaseViewArgs
}

func GetDefaultSandboxViewArgs() *mkapi.ViewArgs {
	return defaultSandboxViewArgs
}

// GetObject returns the object stored under the given filename and whether it was found.
// The directory component of filename is ignored — only the base name is used for lookup.
func GetObject(filename string) (metav1.Object, bool) {
	obj, ok := testObjects[filepath.Base(filename)]
	return obj, ok
}
