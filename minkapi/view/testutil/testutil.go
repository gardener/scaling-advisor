// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
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

const testdataPath = "testutil/testdata"

const (
	// BindingPodA is a Binding object targeting a node.
	BindingPodA = "binding-pod-a.json"
	// CorruptPodA is a Pod with a syntax error in the metadata block.
	CorruptPodA = "corrupt-pod-a.json"
	// EventA is a scheduling Event (events.k8s.io/v1) reporting `FailedScheduling` for a pod.
	EventA = "event-a.json"
	// NameMissPodA is a Pod whose metadata has no name and no generateName.
	NameMissPodA = "name-miss-pod-a.json"
	// NameNodeA is a Node with no generateName, used to test name-only (non-generated) node lookups.
	NameNodeA = "name-node-a.json"
	// NodeA is a well-formed Node with generateName, used as the standard node fixture.
	NodeA = "node-a.json"
	// PatchPodA is a partial Pod payload (status.conditions with `PodScheduled` set to `False`).
	PatchPodA = "patch-pod-a.json"
	// PodA is the primary well-formed Pod fixture in the default namespace with a stable UID.
	PodA = "pod-a.json"
	// PodDefaultNS is a Pod in the "default" namespace.
	PodDefaultNS = "pod-defaultns.json"
	// PodTestNS is a Pod in the "test" namespace.
	PodTestNS = "pod-testns.json"
	// UidTSPodA is a Pod that lacks a UID and creationTimestamp.
	UidTSPodA = "uid-ts-pod-a.json"
	// UpdatePodA is a Pod based on PodA with a modified instanceType label.
	UpdatePodA = "update-pod-a.json"
)

var (
	// DefaultBaseViewArgs holds the default ViewArgs used in tests for the base view.
	DefaultBaseViewArgs = mkapi.ViewArgs{
		Name:           mkapi.DefaultBasePrefix,
		KubeConfigPath: "/tmp/minkapi-base-test.yaml",
		Scheme:         typeinfo.SupportedScheme,
		WatchConfig: mkapi.WatchConfig{
			QueueSize: mkapi.DefaultWatchQueueSize,
			Timeout:   mkapi.DefaultWatchTimeout,
		},
	}
	// DefaultSandboxViewArgs holds the default ViewArgs used in tests for the sandbox view.
	DefaultSandboxViewArgs = mkapi.ViewArgs{
		Name:           "sandbox",
		KubeConfigPath: "/tmp/minkapi-sandbox-test.yaml",
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
		data, err := os.ReadFile(filepath.Join(testdataPath, name)) // #nosec G304 -- path is constructed from os.ReadDir entries within a known testdata directory
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

// GetObject returns the object stored under the given filename and whether it was found.
// The directory component of filename is ignored — only the base name is used for lookup.
func GetObject(filename string) (metav1.Object, bool) {
	obj, ok := testObjects[filepath.Base(filename)]
	return obj, ok
}
