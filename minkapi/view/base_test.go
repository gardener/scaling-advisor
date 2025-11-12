// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/testutil"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestNodeCreation(t *testing.T) {
	objCreationTests := map[string]struct {
		fileName string
		gvk      schema.GroupVersionKind
		retErr   error
	}{
		"No error": {
			fileName: "testdata/node-a.json",
			gvk:      typeinfo.NodesDescriptor.GVK,
			retErr:   nil,
		},
		"Incorrect gvk": {
			fileName: "testdata/node-a.json",
			gvk:      typeinfo.PodsDescriptor.GVK,
			retErr:   fmt.Errorf("does not match expected objGVK"),
		},
		"Missing name and generateName in file": {
			fileName: "testdata/name-node-a.json",
			gvk:      typeinfo.NodesDescriptor.GVK,
			retErr:   minkapi.ErrCreateObject,
		},
	}

	baseView, err := createTestBaseView(t)
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
			_, err = createObjectFromFileName[corev1.Node](t, baseView, tc.fileName, tc.gvk)
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
		c         minkapi.MatchCriteria
		namespace string
		names     []string
		retErr    error
	}{
		"No criteria (need ns)":    {retErr: fmt.Errorf("cannot list pods without namespace")},
		"test namespace":           {namespace: "test", retErr: nil},
		"random namespace":         {namespace: "mnbvcxz", retErr: nil},
		"default ns with pod name": {namespace: metav1.NamespaceDefault, names: []string{"pod-default"}, retErr: nil},
	}
	baseView, err := createTestBaseView(t)
	if err != nil {
		t.Errorf("Can not create base view: %v", err)
		return
	}
	t.Cleanup(func() { baseView.Close() })
	if err := createTestObjects(t, &baseView); err != nil {
		t.Errorf("Can not create test objects: %v", err)
		return
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
		c      minkapi.MatchCriteria
		gvk    schema.GroupVersionKind
		retErr error
	}{
		"No criteria": {
			c:      minkapi.MatchCriteria{},
			gvk:    typeinfo.EventsDescriptor.GVK,
			retErr: fmt.Errorf("cannot list events without namespace"),
		},
		"test namespace": {
			c:      minkapi.MatchCriteria{Namespace: "test"},
			gvk:    typeinfo.EventsDescriptor.GVK,
			retErr: nil,
		},
		"random namespace": {
			c:      minkapi.MatchCriteria{Namespace: "mnbvcxz"},
			gvk:    typeinfo.EventsDescriptor.GVK,
			retErr: nil,
		},
		"default namespace": {
			c:      minkapi.MatchCriteria{Namespace: metav1.NamespaceDefault},
			gvk:    typeinfo.EventsDescriptor.GVK,
			retErr: nil,
		},
		// TODO GVK is only utilized for checking store existence
		"incorrect gvk when deleting": {
			c:      minkapi.MatchCriteria{Namespace: metav1.NamespaceDefault},
			gvk:    typeinfo.PodsDescriptor.GVK,
			retErr: nil,
		},
		"non-existing name": {
			c:      minkapi.MatchCriteria{Namespace: metav1.NamespaceDefault, Names: sets.New("bingo")},
			gvk:    typeinfo.EventsDescriptor.GVK,
			retErr: nil,
		},
	}
	baseView, err := createTestBaseView(t)
	if err != nil {
		t.Errorf("Can not create baseView: %v", err)
		return
	}
	t.Cleanup(func() { baseView.Close() })
	for name, tc := range matchCriteria {
		t.Run(name, func(t *testing.T) {
			_, err = createObjectFromFileName[eventsv1.Event](t, baseView, "testdata/event-a.json", typeinfo.EventsDescriptor.GVK)
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

			t.Logf("Deleting Event")
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

func createTestBaseView(t *testing.T) (minkapi.View, error) {
	t.Helper()
	viewArgs := minkapi.ViewArgs{

		Name:           minkapi.DefaultBasePrefix,
		KubeConfigPath: "/tmp/minkapi-test.yaml",
		Scheme:         typeinfo.SupportedScheme,
		WatchConfig: minkapi.WatchConfig{
			QueueSize: 100,
			Timeout:   500 * time.Millisecond,
		},
	}
	return NewBase(&viewArgs)
}

func createTestObjects(t *testing.T, view *minkapi.View) (err error) {
	_, err = createObjectFromFileName[corev1.Node](t, *view, "testdata/node-a.json", typeinfo.NodesDescriptor.GVK)
	if err != nil {
		return
	}
	for _, file := range []string{"testdata/pod-a.json", "testdata/pod-defaultns.json", "testdata/pod-testns.json"} {
		_, err = createObjectFromFileName[corev1.Pod](t, *view, file, typeinfo.PodsDescriptor.GVK)
		if err != nil {
			return
		}
	}
	return
}

func convertJSONtoObject[T any](t *testing.T, data []byte) (T, error) {
	t.Helper()
	var obj T
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Errorf("error unmarshalling JSON: %v", err)
		return obj, err
	}
	return obj, nil
}

func createObjectFromFileName[T any](t *testing.T, view minkapi.View, fileName string, gvk schema.GroupVersionKind) (T, error) {
	var (
		jsonData []byte
		obj      T
		err      error
	)
	jsonData, err = os.ReadFile(fileName)
	if err != nil {
		return obj, err
	}
	obj, err = convertJSONtoObject[T](t, jsonData)
	if err != nil {
		return obj, err
	}
	metaObj, ok := any(&obj).(metav1.Object)
	if !ok {
		return obj, err
	}
	_, err = view.CreateObject(t.Context(), gvk, metaObj)
	if err != nil {
		return obj, err
	}
	t.Logf("Created %s %q", gvk.Kind, objutil.CacheName(metaObj))
	return obj, nil
}
