// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"fmt"
	"slices"
	"testing"

	viewtestutil "github.com/gardener/scaling-advisor/minkapi/view/testutil"

	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestNodeCreation(t *testing.T) {
	objCreationTests := map[string]struct {
		retErr   error
		fileName string
		gvk      schema.GroupVersionKind
		opts     minkapi.ObjectOptions
	}{
		"No error": {
			fileName: viewtestutil.NodeA,
			gvk:      typeinfo.NodesDescriptor.GVK,
			opts:     minkapi.ObjectOptions{},
			retErr:   nil,
		},
		"Incorrect gvk": {
			fileName: viewtestutil.NodeA,
			gvk:      typeinfo.PodsDescriptor.GVK,
			opts:     minkapi.ObjectOptions{},
			retErr:   fmt.Errorf("does not match expected objGVK"),
		},
		"Missing name and generateName in file": {
			fileName: viewtestutil.NameNodeA,
			gvk:      typeinfo.NodesDescriptor.GVK,
			opts:     minkapi.ObjectOptions{},
			retErr:   minkapi.ErrCreateObject,
		},
	}

	baseView, err := NewBase(viewtestutil.GetDefaultBaseViewArgs())
	if err != nil {
		t.Errorf("Can not create baseView: %v", err)
		return
	}

	t.Cleanup(func() { baseView.Close() })
	for name, tc := range objCreationTests {
		t.Run(name, func(t *testing.T) {
			nodes, err := baseView.ListNodes(t.Context())
			if err != nil {
				testutil.AssertError(t, err, tc.retErr)
				return
			}
			t.Logf("Number of Nodes before creation is %d", len(nodes))
			obj, ok := viewtestutil.GetObject(tc.fileName)
			if !ok {
				t.Fatalf("test object %q not found", tc.fileName)
			}
			_, err = baseView.CreateObject(t.Context(), tc.gvk, obj, tc.opts)
			if err != nil {
				testutil.AssertError(t, err, tc.retErr)
				return
			}
			nodes, err = baseView.ListNodes(t.Context())
			if err != nil {
				testutil.AssertError(t, err, tc.retErr)
				return
			}
			t.Logf("Number of Nodes after creation is %d", len(nodes))
		})
	}
}

func TestPodListing(t *testing.T) {
	matchCriteria := map[string]struct {
		retErr    error
		c         minkapi.MatchCriteria
		namespace string
		names     []string
	}{
		"No criteria (need ns)":    {retErr: fmt.Errorf("cannot list pods without namespace")},
		"test namespace":           {namespace: "test", retErr: nil},
		"random namespace":         {namespace: "mnbvcxz", retErr: nil},
		"default ns with pod name": {namespace: metav1.NamespaceDefault, names: []string{"pod-default"}, retErr: nil},
	}
	baseView, err := NewBase(viewtestutil.GetDefaultBaseViewArgs())
	if err != nil {
		t.Errorf("Can not create base view: %v", err)
		return
	}
	t.Cleanup(func() { baseView.Close() })
	nodeA, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	_, err = baseView.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nodeA, minkapi.ObjectOptions{})
	if err != nil {
		return
	}
	for _, file := range []string{viewtestutil.PodA, viewtestutil.PodDefaultNS, viewtestutil.PodTestNS} {
		obj, ok := viewtestutil.GetObject(file)
		if !ok {
			t.Fatalf("test object %q not found", file)
		}
		_, err = baseView.CreateObject(t.Context(), typeinfo.PodsDescriptor.GVK, obj, minkapi.ObjectOptions{})
		if err != nil {
			return
		}
	}
	for name, tc := range matchCriteria {
		t.Run(name, func(t *testing.T) {
			criteria := minkapi.MatchCriteria{Namespace: tc.namespace, Names: sets.New(tc.names...)}
			p, err := baseView.ListPods(t.Context(), criteria)
			if err != nil {
				testutil.AssertError(t, err, tc.retErr)
				return
			}
			for _, pd := range p {
				t.Logf("Pod is %v", pd.Name)
			}
			if len(p) == 0 {
				t.Logf("No pods found for the criteria")
			}
		})
	}
}

// TODO test matching when deleting
func TestEventDeletion(t *testing.T) {
	matchCriteria := map[string]struct {
		retErr error
		gvk    schema.GroupVersionKind
		c      minkapi.MatchCriteria
		opts   minkapi.ObjectOptions
	}{
		"No criteria": {
			c:      minkapi.MatchCriteria{},
			gvk:    typeinfo.EventsDescriptor.GVK,
			opts:   minkapi.ObjectOptions{},
			retErr: fmt.Errorf("cannot list events without namespace"),
		},
		"test namespace": {
			c:      minkapi.MatchCriteria{Namespace: "test"},
			gvk:    typeinfo.EventsDescriptor.GVK,
			opts:   minkapi.ObjectOptions{},
			retErr: nil,
		},
		"random namespace": {
			c:      minkapi.MatchCriteria{Namespace: "mnbvcxz"},
			gvk:    typeinfo.EventsDescriptor.GVK,
			opts:   minkapi.ObjectOptions{},
			retErr: nil,
		},
		"default namespace": {
			c:      minkapi.MatchCriteria{Namespace: metav1.NamespaceDefault},
			gvk:    typeinfo.EventsDescriptor.GVK,
			opts:   minkapi.ObjectOptions{},
			retErr: nil,
		},
		// TODO GVK is only utilized for checking store existence
		"incorrect gvk when deleting": {
			c:      minkapi.MatchCriteria{Namespace: metav1.NamespaceDefault},
			gvk:    typeinfo.PodsDescriptor.GVK,
			opts:   minkapi.ObjectOptions{},
			retErr: nil,
		},
		"non-existing name": {
			c:      minkapi.MatchCriteria{Namespace: metav1.NamespaceDefault, Names: sets.New("bingo")},
			gvk:    typeinfo.EventsDescriptor.GVK,
			opts:   minkapi.ObjectOptions{},
			retErr: nil,
		},
	}
	baseView, err := NewBase(viewtestutil.GetDefaultBaseViewArgs())
	if err != nil {
		t.Errorf("Can not create baseView: %v", err)
		return
	}
	t.Cleanup(func() { baseView.Close() })
	for name, tc := range matchCriteria {
		t.Run(name, func(t *testing.T) {
			eventA, ok := viewtestutil.GetObject(viewtestutil.EventA)
			if !ok {
				t.Fatalf("test object %q not found", viewtestutil.EventA)
			}
			_, err = baseView.CreateObject(t.Context(), typeinfo.EventsDescriptor.GVK, eventA, tc.opts)
			if err != nil {
				t.Error(err)
				return
			}
			events, err := baseView.ListEvents(t.Context(), tc.c.Namespace)
			if err != nil {
				testutil.AssertError(t, err, tc.retErr)
				return
			}
			t.Logf("Number of Events before deletion is %d", len(events))

			t.Logf("Deleting Event with criteria: %s", tc.c)
			err = baseView.DeleteObjects(t.Context(), tc.gvk, tc.c)
			if err != nil {
				testutil.AssertError(t, err, tc.retErr)
				return
			}

			events, err = baseView.ListEvents(t.Context(), tc.c.Namespace)
			if err != nil {
				testutil.AssertError(t, err, tc.retErr)
				return
			}
			t.Logf("Number of Events after deletion is %d", len(events))
		})
	}
}

func TestCombinePrimarySecondary(t *testing.T) {
	primary := []metav1.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-a",
				Labels: map[string]string{
					"category": "primary",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-b",
				Labels: map[string]string{
					"category": "primary",
				},
			},
		},
	}
	secondary := []metav1.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-a",
				Labels: map[string]string{
					"category": "secondary",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-b",
				Labels: map[string]string{
					"category": "secondary",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-c",
				Labels: map[string]string{
					"category": "secondary",
				},
			},
		},
	}
	combined := combinePrimarySecondary(primary, secondary)
	combinedNames := make([]string, len(combined))
	for i, obj := range combined {
		combinedNames[i] = objutil.CacheName(obj).String()
	}
	t.Logf("Combined objects: %v", combinedNames)

	expectedLen := 3
	if len(combined) != expectedLen {
		t.Errorf("Expected %d objects, got %d", expectedLen, len(combined))
	}
	nodeAIdx := slices.IndexFunc(combined, func(obj metav1.Object) bool {
		return obj.GetName() == "node-a"
	})
	if nodeAIdx == -1 {
		t.Errorf("Expected to find node-a in combined list")
		return
	}
	nodeACategory := combined[nodeAIdx].(*corev1.Node).Labels["category"]
	if nodeACategory != "primary" {
		t.Errorf("Expected node-a to have category primary, got %s", nodeACategory)
		return
	}

	nodeBIdx := slices.IndexFunc(combined, func(obj metav1.Object) bool {
		return obj.GetName() == "node-b"
	})
	if nodeBIdx == -1 {
		t.Errorf("Expected to find node-b in combined list")
		return
	}
	nodeBCategory := combined[nodeBIdx].(*corev1.Node).Labels["category"]
	if nodeBCategory != "primary" {
		t.Errorf("Expected node-b to have category primary, got %s", nodeBCategory)
	}

	nodeCIdx := slices.IndexFunc(combined, func(obj metav1.Object) bool {
		return obj.GetName() == "node-c"
	})
	if nodeCIdx == -1 {
		t.Errorf("Expected to find node-c in combined list")
		return
	}
	nodeCCategory := combined[nodeCIdx].(*corev1.Node).Labels["category"]
	if nodeCCategory != "secondary" {
		t.Errorf("Expected node-c to have category secondary, got %s", nodeCCategory)
	}
}
