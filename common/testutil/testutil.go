// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	sigyaml "sigs.k8s.io/yaml"
)

//go:embed testdata/*
var testDataFS embed.FS

// AssertError compares the received error with the wanted one and
// checks for equality first by comparing them otherwise by checking
// if the received error is a substring of the wanted error.
func AssertError(t *testing.T, got error, want error) {
	t.Helper()
	if isNil(got) && isNil(want) {
		return
	}
	if (isNil(got) && !isNil(want)) || (!isNil(got) && isNil(want)) {
		t.Errorf("Unexpected error, got: %v, want: %v", got, want)
		return
	}
	if errors.Is(got, want) || strings.Contains(got.Error(), want.Error()) {
		t.Logf("Expected error: %v", got)
	} else {
		t.Errorf("Unexpected error, got: %v, want: %v", got, want)
	}
}

// isNil checks if v is nil. (source: https://antonz.org/do-not-testify/)
func isNil(v any) bool {
	if v == nil {
		return true
	}
	// A non-nil interface can still hold a nil value, so we must check the underlying value.
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Pointer, reflect.Slice,
		reflect.UnsafePointer:
		return rv.IsNil()
	default:
		return false
	}
}

// GetFunctionName returns the name of the function passed
func GetFunctionName(t *testing.T, fn any) string {
	t.Helper()
	if fn == nil {
		return "<nil>"
	}
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

// LoadTestNodes returns the node object from the node resource file
func LoadTestNodes() (nodes []corev1.Node, err error) {
	var nodeA corev1.Node
	data, err := testDataFS.ReadFile("testdata/node-a.yaml")
	if err != nil {
		return
	}
	err = sigyaml.Unmarshal(data, &nodeA)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal object node-a data into node obj: %w", err)
		return
	}
	nodes = append(nodes, nodeA)
	return
}

// LoadTestPods returns the pod object from the pod resource file
func LoadTestPods() (pods []corev1.Pod, err error) {
	var podA corev1.Pod
	data, err := testDataFS.ReadFile("testdata/pod-a.yaml")
	if err != nil {
		return
	}
	err = sigyaml.Unmarshal(data, &podA)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal object pod-a data into pod obj: %w", err)
		return
	}
	pods = append(pods, podA)
	return
}

// LoggerContext wraps the given context with a logr logger based on the klog backend.
func LoggerContext(ctx context.Context) context.Context {
	log := klog.NewKlogr()
	return logr.NewContext(ctx, log)
}
