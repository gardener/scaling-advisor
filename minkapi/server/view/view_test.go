// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package view_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/cli"
	"github.com/gardener/scaling-advisor/minkapi/server"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	"github.com/go-logr/logr"
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
			fileName: "../testdata/node-a.json",
			gvk:      typeinfo.NodesDescriptor.GVK,
			retErr:   nil,
		},
		"Incorrect gvk": {
			fileName: "../testdata/node-a.json",
			gvk:      typeinfo.PodsDescriptor.GVK,
			retErr:   fmt.Errorf("does not match expected objGVK"),
		},
		"Missing name and generateName in file": {
			fileName: "../testdata/name-node-a.json",
			gvk:      typeinfo.NodesDescriptor.GVK,
			retErr:   api.ErrCreateObject,
		},
	}

	svc, err := startMinkapiService(t)
	if err != nil {
		t.Errorf("Can not start minkapi service: %v", err)
		return
	}

	t.Cleanup(func() { svc.Stop(context.TODO()) })
	for name, tc := range objCreationTests {
		t.Run(name, func(t *testing.T) {
			nodes, err := svc.GetBaseView().ListNodes()
			if err != nil {
				assertError(t, err, tc.retErr)
				return
			}
			t.Logf("Number of Nodes before creation is %d", len(nodes))
			_, err = createObjectFromFileName[corev1.Node](t, svc, tc.fileName, tc.gvk)
			if err != nil {
				assertError(t, err, tc.retErr)
				return
			}
			nodes, err = svc.GetBaseView().ListNodes()
			if err != nil {
				assertError(t, err, tc.retErr)
				return
			}
			t.Logf("Number of Nodes after creation is %d", len(nodes))
		})
	}
}

func TestPodListing(t *testing.T) {
	matchCriteria := map[string]struct {
		c         api.MatchCriteria
		namespace string
		names     []string
		retErr    error
	}{
		"No criteria (need ns)":    {retErr: fmt.Errorf("cannot list pods without namespace")},
		"test namespace":           {namespace: "test", retErr: nil},
		"random namespace":         {namespace: "mnbvcxz", retErr: nil},
		"default ns with pod name": {namespace: "default", names: []string{"pod-default"}, retErr: nil},
	}
	svc, err := startMinkapiService(t)
	if err != nil {
		t.Errorf("Can not start minkapi service: %v", err)
		return
	}
	t.Cleanup(func() { svc.Stop(context.TODO()) })
	if err := createTestObjects(t, &svc); err != nil {
		t.Errorf("Can not create test objects: %v", err)
		return
	}
	for name, tc := range matchCriteria {
		t.Run(name, func(t *testing.T) {
			p, err := svc.GetBaseView().ListPods(tc.namespace, tc.names...)
			if err != nil {
				assertError(t, err, tc.retErr)
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
		c      api.MatchCriteria
		gvk    schema.GroupVersionKind
		retErr error
	}{
		"No criteria": {
			c:      api.MatchCriteria{},
			gvk:    typeinfo.EventsDescriptor.GVK,
			retErr: fmt.Errorf("cannot list events without namespace"),
		},
		"test namespace": {
			c:      api.MatchCriteria{Namespace: "test"},
			gvk:    typeinfo.EventsDescriptor.GVK,
			retErr: nil,
		},
		"random namespace": {
			c:      api.MatchCriteria{Namespace: "mnbvcxz"},
			gvk:    typeinfo.EventsDescriptor.GVK,
			retErr: nil,
		},
		"default namespace": {
			c:      api.MatchCriteria{Namespace: "default"},
			gvk:    typeinfo.EventsDescriptor.GVK,
			retErr: nil,
		},
		// TODO GVK is only utilized for checking store existence
		"incorrect gvk when deleting": {
			c:      api.MatchCriteria{Namespace: "default"},
			gvk:    typeinfo.PodsDescriptor.GVK,
			retErr: nil,
		},
		"non-existing name": {
			c:      api.MatchCriteria{Namespace: "default", Names: sets.New("bingo")},
			gvk:    typeinfo.EventsDescriptor.GVK,
			retErr: nil,
		},
	}
	svc, err := startMinkapiService(t)
	if err != nil {
		t.Errorf("Can not start minkapi service: %v", err)
		return
	}
	t.Cleanup(func() { svc.Stop(context.TODO()) })
	for name, tc := range matchCriteria {
		t.Run(name, func(t *testing.T) {
			_, err = createObjectFromFileName[eventsv1.Event](t, svc, "../testdata/event-a.json", typeinfo.EventsDescriptor.GVK)
			if err != nil {
				t.Error(err)
				return
			}
			events, err := svc.GetBaseView().ListEvents(tc.c.Namespace)
			if err != nil {
				assertError(t, err, tc.retErr)
				return
			}
			t.Logf("Number of Events before deletion is %d", len(events))

			t.Logf("Deleting Event")
			err = svc.GetBaseView().DeleteObjects(tc.gvk, tc.c)
			if err != nil {
				assertError(t, err, tc.retErr)
				return
			}

			events, err = svc.GetBaseView().ListEvents(tc.c.Namespace)
			if err != nil {
				assertError(t, err, tc.retErr)
				return
			}
			t.Logf("Number of Events after deletion is %d", len(events))
		})
	}
}

func startMinkapiService(t *testing.T) (api.Server, error) {
	t.Helper()

	mainOpts, err := cli.ParseProgramFlags([]string{
		"-k", "/tmp/minkapi-test.yaml",
		"-H", "localhost",
		"-P", "9892",
		"-t", "0.5s",
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Err: %v\n", err)
		return nil, err
	}
	cfg := mainOpts.MinKAPIConfig
	return server.NewInMemory(logr.FromContextOrDiscard(context.TODO()), cfg)
}

func assertError(t *testing.T, got error, want error) {
	t.Helper()
	if errors.Is(got, want) || strings.Contains(got.Error(), want.Error()) {
		t.Logf("Expected error: %v", got)
	} else {
		t.Errorf("Unexpected error, got: %v, want: %v", got, want)
	}
}

func createTestObjects(t *testing.T, svc *api.Server) (err error) {
	t.Helper()
	_, err = createObjectFromFileName[corev1.Node](t, *svc, "../testdata/node-a.json", typeinfo.NodesDescriptor.GVK)
	if err != nil {
		t.Error(err)
		return err
	}
	for _, file := range []string{"../testdata/pod-a.json", "../testdata/pod-defaultns.json", "../testdata/pod-testns.json"} {
		_, err = createObjectFromFileName[corev1.Pod](t, *svc, file, typeinfo.PodsDescriptor.GVK)
		if err != nil {
			t.Error(err)
			return err
		}
	}

	return nil
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

func createObjectFromFileName[T any](t *testing.T, svc api.Server, fileName string, gvk schema.GroupVersionKind) (T, error) {
	t.Helper()
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
	objInterface, ok := any(&obj).(metav1.Object)
	if !ok {
		return obj, err
	}
	err = svc.GetBaseView().CreateObject(gvk, objInterface)
	if err != nil {
		return obj, err
	}
	t.Logf("Creating %s %s", gvk.Kind, objInterface.GetName())
	return obj, nil
}
