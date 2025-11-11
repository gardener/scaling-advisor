// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
)

var state suiteState

var baseURL string

type suiteState struct {
	app           mkapi.App
	nodeA         corev1.Node
	podA          corev1.Pod
	clientFacades commontypes.ClientFacades
}

// TestMain sets up the MinKAPI server once for all tests in this package, runs tests and then shutdown.
func TestMain(m *testing.M) {
	err := initSuite(context.Background())
	baseURL = fmt.Sprintf("http://localhost:%d/%s", state.app.Server.(*InMemoryKAPI).cfg.Port, mkapi.DefaultBasePrefix)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to initialize suite state: %v\n", err)
		os.Exit(commoncli.ExitErrStart)
	}
	// Run integration tests
	exitCode := m.Run()
	shutdownSuite()
	os.Exit(exitCode)
}

func TestBaseViewCreateGetNodes(t *testing.T) {
	ctx := t.Context()

	nodesFacade := state.clientFacades.Client.CoreV1().Nodes()

	t.Run("checkInitialNodeList", func(t *testing.T) {
		nodeList, err := nodesFacade.List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatal(fmt.Errorf("failed to list nodes: %w", err))
		}
		if len(nodeList.Items) != 0 {
			t.Errorf("len(nodeList)=%d, want %d", len(nodeList.Items), 0)
		}
	})

	t.Run("createGetNode", func(t *testing.T) {
		createdNode, err := nodesFacade.Create(ctx, &state.nodeA, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(fmt.Errorf("failed to create node: %w", err))
		}
		gotNode, err := nodesFacade.Get(ctx, createdNode.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(fmt.Errorf("failed to get node: %w", err))
		}
		checkNodeIsSame(t, gotNode, createdNode)
	})
}

type eventsHolder struct {
	events []watch.Event
	mu     sync.Mutex
}

func (h *eventsHolder) Add(e watch.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, e)
}

func (h *eventsHolder) Events() []watch.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.events)
}

func TestWatchPods(t *testing.T) {
	var h eventsHolder
	client := state.clientFacades.Client
	watcher, err := client.CoreV1().Pods("").Watch(t.Context(), metav1.ListOptions{Watch: true})
	if err != nil {
		t.Fatalf("failed to create pods watcher: %v", err)
		return
	}
	defer watcher.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		listObjects(ctx, t, watcher.ResultChan(), h.Add)
	}()

	createdPod, err := client.CoreV1().Pods("").Create(ctx, &state.podA, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create podA: %v", err)
		return
	}
	t.Logf("Created podA with name %q", createdPod.Name)
	<-time.After(2 * time.Second)
	cancel()

	events := h.Events()
	t.Logf("got numEvents: %d", len(events))
	if len(events) == 0 {
		t.Fatalf("got no events, want at least one")
		return
	}

	if events[0].Type != watch.Added {
		t.Errorf("got event type %v, want %v", events[0].Type, watch.Added)
	}
}

func listObjects(ctx context.Context, t *testing.T, eventCh <-chan watch.Event, addEventFn func(e watch.Event)) {
	t.Logf("Iterating eventCh: %v", eventCh)
	count := 0
outer:
	for {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				break outer
			}
			count++
			mo, err := meta.Accessor(ev.Object)
			if err != nil {
				t.Logf("received #%d event, Type: %s, Object: %v", count, ev.Type, ev.Object)
				continue
			}
			objFullName := objutil.CacheName(mo)
			if err != nil {
				t.Fatalf("failed to get TypeAccessor for event object %q: %v", objFullName, err)
				return
			}
			t.Logf("received #%d event, Type: %s, ObjectName: %s", count, ev.Type, objFullName)
			addEventFn(ev)
		case <-ctx.Done():
			break outer
		}
	}
	t.Log("listObjects done")
}

func checkNodeIsSame(t *testing.T, got, want *corev1.Node) {
	t.Helper()
	if got.Name != want.Name {
		t.Errorf("got.InstanceType=%s, want %s", got.Name, want.Name)
	}
	if got.Spec.ProviderID != want.Spec.ProviderID {
		t.Errorf("got.Spec.ProviderID=%s, want %s", got.Spec.ProviderID, want.Spec.ProviderID)
	}
}

func initSuite(ctx context.Context) error {
	var err error
	var exitCode int

	state.app, exitCode = LaunchApp(ctx)
	if exitCode != commoncli.ExitSuccess {
		os.Exit(exitCode)
	}
	<-time.After(1 * time.Second) // give minmal time for startup

	state.clientFacades, err = state.app.Server.GetBaseView().GetClientFacades(commontypes.ClientAccessNetwork)
	if err != nil {
		return err
	}

	nodes, err := testutil.LoadTestNodes()
	if err != nil {
		return err
	}
	state.nodeA = nodes[0]

	pods, err := testutil.LoadTestPods()
	if err != nil {
		return err
	}
	state.podA = pods[0]

	return nil
}

func shutdownSuite() {
	state.app.Cancel()
	_ = ShutdownApp(&state.app)
}

// --------------------------------------------------------------------------------------

type RequestParams struct {
	Method      string
	Target      string
	ContentType string
}

var ignoreResourceVersion = cmpopts.IgnoreFields(corev1.Pod{}, "Name", "ResourceVersion")

func TestHTTPHandlers(t *testing.T) {
	tests := map[string]struct {
		filePath                         string
		expectedStatus                   int
		reqParams                        RequestParams
		ignoredFieldsForOutputComparison cmp.Option
	}{
		"fetch existing pod": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodGet,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/json",
			},
			expectedStatus:                   http.StatusOK,
			ignoredFieldsForOutputComparison: ignoreResourceVersion,
		},
		"delete existing pod": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodDelete,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusOK,
		},
		"erroneous label selector for pods": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodGet,
				Target:      "/api/v1/pods?labelSelector=state.app.kubernetes.io/name=*?",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusBadRequest,
		},
		"fetch pod list": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodGet,
				Target:      "/api/v1/namespaces/default/pods",
				ContentType: "application/json",
			},
			expectedStatus:                   http.StatusOK,
			ignoredFieldsForOutputComparison: ignoreResourceVersion,
		},
		"matching label selector for pods": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodGet,
				Target:      "/api/v1/pods?labelSelector=state.app.kubernetes.io/component=minkapitest",
				ContentType: "application/json",
			},
			expectedStatus:                   http.StatusOK,
			ignoredFieldsForOutputComparison: ignoreResourceVersion,
		},
		"non-matching label selector for pods": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodGet,
				Target:      "/api/v1/pods?labelSelector=state.app.kubernetes.io/component=abcdefgh",
				ContentType: "application/json",
			},
			expectedStatus:                   http.StatusOK,
			ignoredFieldsForOutputComparison: ignoreResourceVersion,
		},
		"create pod binding": {
			filePath: "./testdata/binding-pod-a.json",
			reqParams: RequestParams{
				Method:      http.MethodPost,
				Target:      "/api/v1/namespaces/default/pods/bingo/binding",
				ContentType: "application/json",
			},
			expectedStatus:                   http.StatusOK,
			ignoredFieldsForOutputComparison: ignoreResourceVersion,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() { state.app.Server.GetBaseView().Reset() })

			if _, err := createObjectFromFileName[corev1.Pod](t, state.app.Server.(*InMemoryKAPI), "./testdata/pod-bingo.json", typeinfo.PodsDescriptor.GVK); err != nil {
				t.Errorf("Error creating test object: %v", err)
			}

			jsonData, err := os.ReadFile(tc.filePath)
			if err != nil {
				t.Errorf("failed to read test data: %v", err)
				return
			}

			resp, err := makeHTTPRequest(baseURL+tc.reqParams.Target, tc.reqParams.Method, tc.reqParams.ContentType, jsonData)
			if err != nil {
				t.Errorf("Failed to make HTTP request: %v", err)
				return
			}
			defer resp.Body.Close()

			if err = compareHTTPHandlerResponse(t, state.app.Server.(*InMemoryKAPI), resp, tc.reqParams, tc.ignoredFieldsForOutputComparison, jsonData, tc.expectedStatus); err != nil {
				t.Errorf("Failed: %v", err)
			}
		})
	}
}

func TestAPIHandlerMethods(t *testing.T) {
	tests := map[string]struct {
		reqParams      RequestParams
		expectedStatus int
		want           any
	}{
		"invalid request for api groups": {
			reqParams: RequestParams{
				Method:      http.MethodPost,
				Target:      "/apis",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusMethodNotAllowed,
			want:           typeinfo.SupportedAPIGroups,
		},
		"get request for api groups": {
			reqParams: RequestParams{
				Method:      http.MethodGet,
				Target:      "/apis",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusOK,
			want:           typeinfo.SupportedAPIGroups,
		},
		"invalid request for api versions": {
			reqParams: RequestParams{
				Method:      http.MethodPost,
				Target:      "/api",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusMethodNotAllowed,
			want:           typeinfo.SupportedAPIVersions,
		},
		"get request for api versions": {
			reqParams: RequestParams{
				Method:      http.MethodGet,
				Target:      "/api",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusOK,
			want:           typeinfo.SupportedAPIVersions,
		},
		"invalid request for api resources": {
			reqParams: RequestParams{
				Method:      http.MethodPost,
				Target:      "/api/v1/",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusMethodNotAllowed,
			want:           typeinfo.SupportedCoreAPIResourceList,
		},
		"get request for api resources": {
			reqParams: RequestParams{
				Method:      http.MethodGet,
				Target:      "/api/v1/",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusOK,
			want:           typeinfo.SupportedCoreAPIResourceList,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resp, err := makeHTTPRequest(baseURL+tc.reqParams.Target, tc.reqParams.Method, tc.reqParams.ContentType, nil)
			if err != nil {
				t.Errorf("Failed to make HTTP request: %v", err)
				return
			}
			defer resp.Body.Close()

			responseData, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("expected error to be nil got %v", err)
				return
			}

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Unexpected status code, got: %s, expected: %d", resp.Status, tc.expectedStatus)
				t.Logf(">>> Got response: %s\n", string(responseData))
				return
			} else if resp.StatusCode != http.StatusOK {
				t.Logf("Expected status: %s", resp.Status)
				return
			}

			validateAPIResponse(t, tc.reqParams.Target, tc.want, responseData)
		})
	}
}

func validateAPIResponse(t *testing.T, target string, want any, responseData []byte) {
	var got any
	switch target {
	case "/apis":
		got, _ = convertJSONtoObject[metav1.APIGroupList](t, responseData)
	case "/api":
		got, _ = convertJSONtoObject[metav1.APIVersions](t, responseData)
	case "/api/v1/":
		got, _ = convertJSONtoObject[metav1.APIResourceList](t, responseData)
	}
	if diff := cmp.Diff(want, got, nil); diff != "" {
		t.Errorf("object mismatch (-want +got):\n%s", diff)
		return
	} else {
		t.Logf("Got expected output")
	}
}

func TestPatchPutHTTPHandlers(t *testing.T) {
	var testPodPatchStatus = `
{
  "status" : {
	"conditions" : [ {
	  "lastProbeTime" : null,
	  "lastTransitionTime" : "2025-05-08T08:21:44Z",
	  "message" : "no nodes available to schedule pods",
	  "reason" : "Unschedulable",
	  "status" : "False",
	  "type" : "PodScheduled"
	} ]
  }
}
`
	var testPatchName = `{"metadata":{"name": "pwned"}}`
	var testPatchLabel = `{"metadata":{"labels": {"test-patch": "label"}}}`
	var corruptedPatch = `{}}`
	data, _ := os.ReadFile("./testdata/corrupt-pod-a.json")
	var corruptedPodResource = string(data)
	data, _ = os.ReadFile("./testdata/update-pod-name-a.json")
	var updatedPodName = string(data)
	data, _ = os.ReadFile("./testdata/update-pod-a.json")
	var updatedPodLabel = string(data)

	patchTests := map[string]struct {
		patchData                        string
		reqParams                        RequestParams
		expectedStatus                   int
		ignoredFieldsForOutputComparison cmp.Option
	}{
		"patch pod status": {
			patchData: testPodPatchStatus,
			reqParams: RequestParams{
				Method:      http.MethodPatch,
				Target:      "/api/v1/namespaces/default/pods/bingo/status",
				ContentType: "application/strategic-merge-patch+json",
			},
			expectedStatus:                   http.StatusOK,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion", "Status.Conditions"),
		},
		"patch pod status with unsupported content type": {
			patchData: testPodPatchStatus,
			reqParams: RequestParams{
				Method:      http.MethodPatch,
				Target:      "/api/v1/namespaces/default/pods/bingo/status",
				ContentType: "application/json-patch+json",
			},
			expectedStatus: http.StatusBadRequest,
		},
		"patch pod name": {
			patchData: testPatchName,
			reqParams: RequestParams{
				Method:      http.MethodPatch,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/strategic-merge-patch+json",
			},
			expectedStatus: http.StatusUnprocessableEntity,
		},
		"patch pod label": {
			patchData: testPatchLabel,
			reqParams: RequestParams{
				Method:      http.MethodPatch,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/strategic-merge-patch+json",
			},
			expectedStatus:                   http.StatusOK,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion", "Labels"),
		},
		"patch pod with unsupported content type": {
			patchData: testPatchName,
			reqParams: RequestParams{
				Method:      http.MethodPatch,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/json-patch+json",
			},
			expectedStatus: http.StatusBadRequest,
		},
		"corrupted patch pod": {
			patchData: corruptedPatch,
			reqParams: RequestParams{
				Method:      http.MethodPatch,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/strategic-merge-patch+json",
			},
			expectedStatus: http.StatusInternalServerError, // FIXME why should this return internal server error
		},
		"corrupted patch pod status": {
			patchData: corruptedPatch,
			reqParams: RequestParams{
				Method:      http.MethodPatch,
				Target:      "/api/v1/namespaces/default/pods/bingo/status",
				ContentType: "application/strategic-merge-patch+json",
			},
			expectedStatus: http.StatusInternalServerError, // FIXME why should this return internal server error
		},
		"update with corrupted object": {
			patchData: corruptedPodResource,
			reqParams: RequestParams{
				Method:      http.MethodPut,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusBadRequest,
		},
		"update with new object": {
			patchData: updatedPodLabel,
			reqParams: RequestParams{
				Method:      http.MethodPut,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/json",
			},
			expectedStatus:                   http.StatusOK,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion", "Labels"),
		},
		"update with change in object name": {
			patchData: updatedPodName,
			reqParams: RequestParams{
				Method:      http.MethodPut,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusUnprocessableEntity,
		},
	}

	for name, tc := range patchTests {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() { state.app.Server.GetBaseView().Reset() })

			jsonData, err := os.ReadFile("./testdata/pod-bingo.json")
			if err != nil {
				t.Logf("failed to read: %v", err)
				return
			}
			if _, err := createObjectFromFileName[corev1.Pod](t, state.app.Server.(*InMemoryKAPI), "./testdata/pod-bingo.json", typeinfo.PodsDescriptor.GVK); err != nil {
				t.Errorf("Error creating test object: %v", err)
			}

			resp, err := makeHTTPRequest(baseURL+tc.reqParams.Target, tc.reqParams.Method, tc.reqParams.ContentType, []byte(tc.patchData))
			if err != nil {
				t.Errorf("Failed to make HTTP request: %v", err)
				return
			}
			defer resp.Body.Close()

			if err = compareHTTPHandlerResponse(t, state.app.Server.(*InMemoryKAPI), resp, tc.reqParams, tc.ignoredFieldsForOutputComparison, jsonData, tc.expectedStatus); err != nil {
				t.Errorf("Failed: %v", err)
			}
		})
	}
}

func TestPatchPutNoObject(t *testing.T) {
	var testPodPatchStatus = `
{
  "status" : {
	"conditions" : [ {
	  "lastProbeTime" : null,
	  "lastTransitionTime" : "2025-05-08T08:21:44Z",
	  "message" : "no nodes available to schedule pods",
	  "reason" : "Unschedulable",
	  "status" : "False",
	  "type" : "PodScheduled"
	} ]
  }
}
`
	var testPatchName = `{"metadata":{"name": "pwned"}}`

	patchTests := map[string]struct {
		patchData                        string
		reqParams                        RequestParams
		expectedStatus                   int
		ignoredFieldsForOutputComparison cmp.Option
	}{
		"patch status of non-existent pod": {
			patchData: testPodPatchStatus,
			reqParams: RequestParams{
				Method:      http.MethodPatch,
				Target:      "/api/v1/namespaces/default/pods/bingoz/status",
				ContentType: "application/strategic-merge-patch+json",
			},
			expectedStatus: http.StatusNotFound,
		},
		"patch non-existent pod": {
			patchData: testPatchName,
			reqParams: RequestParams{
				Method:      http.MethodPatch,
				Target:      "/api/v1/namespaces/default/pods/bingoz",
				ContentType: "application/strategic-merge-patch+json",
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for name, tc := range patchTests {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() { state.app.Server.GetBaseView().Reset() })

			jsonData, err := os.ReadFile("./testdata/pod-bingo.json")
			if err != nil {
				t.Logf("failed to read: %v", err)
				return
			}

			resp, err := makeHTTPRequest(baseURL+tc.reqParams.Target, tc.reqParams.Method, tc.reqParams.ContentType, []byte(tc.patchData))
			if err != nil {
				t.Errorf("Failed to make HTTP request: %v", err)
				return
			}
			defer resp.Body.Close()

			if err = compareHTTPHandlerResponse(t, state.app.Server.(*InMemoryKAPI), resp, tc.reqParams, tc.ignoredFieldsForOutputComparison, jsonData, tc.expectedStatus); err != nil {
				t.Errorf("Failed: %v", err)
			}
		})
	}
}

func TestNoObject(t *testing.T) {
	tests := map[string]struct {
		filePath                         string
		expectedStatus                   int
		reqParams                        RequestParams
		ignoredFieldsForOutputComparison cmp.Option
	}{
		"pod creation": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodPost,
				Target:      "/api/v1/namespaces/default/pods",
				ContentType: "application/json",
			},
			expectedStatus:                   http.StatusOK,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		},
		"invalid request target": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodPost,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusMethodNotAllowed,
		},
		"create corrupted pod": {
			filePath: "./testdata/corrupt-pod-a.json",
			reqParams: RequestParams{
				Method:      http.MethodPost,
				Target:      "/api/v1/namespaces/default/pods",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusBadRequest,
		},
		"create pod without namespace in request target": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodPost,
				Target:      "/api/v1/pods",
				ContentType: "application/json",
			},
			expectedStatus:                   http.StatusOK,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		},
		"create pod missing name and generateName": {
			filePath: "./testdata/name-miss-pod-a.json",
			reqParams: RequestParams{
				Method:      http.MethodPost,
				Target:      "/api/v1/namespaces/default/pods",
				ContentType: "application/json",
			},
			expectedStatus:                   http.StatusBadRequest,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		},
		"create pod missing name, UID and creationTimestamp": {
			filePath: "./testdata/uid-ts-pod-a.json",
			reqParams: RequestParams{
				Method:      http.MethodPost,
				Target:      "/api/v1/namespaces/default/pods",
				ContentType: "application/json",
			},
			expectedStatus:                   http.StatusOK,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion", "Name", "Namespace", "UID", "CreationTimestamp"),
		},
		"fetch non-existent pod": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodGet,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusNotFound,
		},
		"delete non-existent pod": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodDelete,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusNotFound,
		},
		"update non-existent pod": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodPut,
				Target:      "/api/v1/namespaces/default/pods/bingo",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() { state.app.Server.GetBaseView().Reset() })

			jsonData, err := os.ReadFile(tc.filePath)
			if err != nil {
				t.Errorf("failed to read test data: %v", err)
				return
			}

			resp, err := makeHTTPRequest(baseURL+tc.reqParams.Target, tc.reqParams.Method, tc.reqParams.ContentType, jsonData)
			if err != nil {
				t.Errorf("Failed to make HTTP request: %v", err)
				return
			}
			defer resp.Body.Close()

			if err = compareHTTPHandlerResponse(t, state.app.Server.(*InMemoryKAPI), resp, tc.reqParams, tc.ignoredFieldsForOutputComparison, jsonData, tc.expectedStatus); err != nil {
				t.Errorf("Failed: %v", err)
			}
		})
	}
}

func TestWatch(t *testing.T) {
	tests := map[string]struct {
		filePath       string
		expectedStatus int
		reqParams      RequestParams
	}{
		"watch all pods": {
			filePath: "./testdata/pod-bingo.json",
			reqParams: RequestParams{
				Method:      http.MethodGet,
				Target:      "/api/v1/pods?watch=1&resourceVersion=0",
				ContentType: "application/json",
			},
			expectedStatus: http.StatusOK,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() { state.app.Server.GetBaseView().Reset() })

			if _, err := createObjectFromFileName[corev1.Pod](t, state.app.Server.(*InMemoryKAPI), "./testdata/pod-bingo.json", typeinfo.PodsDescriptor.GVK); err != nil {
				t.Errorf("Error creating test object: %v", err)
			}

			wantData, err := os.ReadFile(tc.filePath)
			if err != nil {
				t.Errorf("failed to read test data: %v", err)
				return
			}

			resp, err := makeHTTPRequest(baseURL+tc.reqParams.Target, tc.reqParams.Method, tc.reqParams.ContentType, nil)
			if err != nil {
				t.Errorf("Failed to make HTTP request: %v", err)
				return
			}
			defer resp.Body.Close()

			if err = handleTestWatchResponse(t, resp, wantData); err != nil {
				t.Errorf("Could not get watch response: %v", err)
				return
			}

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Unexpected status code, got: %d, expected: %d", resp.StatusCode, tc.expectedStatus)
				return
			} else if resp.StatusCode != http.StatusOK {
				t.Logf("Expected status: %d", tc.expectedStatus)
				return
			}
		})
	}
}

// -- Helper functions ------------------------------------------------------------------------

func handleTestWatchResponse(t *testing.T, resp *http.Response, wantData []byte) error {
	t.Helper()
	scanner := bufio.NewScanner(resp.Body)
	eventCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		got, eventType, err := parseWatchEvent(t, line)
		if err != nil {
			t.Logf("Failed to parse watch event: %v", err)
			continue
		}
		want, _ := convertJSONtoObject[corev1.Pod](t, wantData)
		if diff := cmp.Diff(want, *got, cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion")); diff != "" {
			t.Logf(">>> want\n%s\n", string(wantData))
			t.Logf(">>> got\n%s\n", line)
			t.Errorf("WATCH object mismatch (-want +got):\n%s", diff)
			return err
		}

		t.Logf("Watch event: %s got %s/%s, resourceVersion: %s", eventType, got.Namespace, got.Name, got.ResourceVersion)
		eventCount++
	}
	if scanner.Err() != nil && eventCount == 0 {
		return scanner.Err()
	}
	if eventCount == 0 {
		respData, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("No watch events received, response: %q", string(respData))
	}
	return nil
}

func parseWatchEvent(t *testing.T, line string) (*corev1.Pod, string, error) {
	t.Helper()
	var rawEvent struct {
		Type   string          `json:"type"`
		Object json.RawMessage `json:"object"`
	}

	if err := json.Unmarshal([]byte(line), &rawEvent); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal event: %w", err)
	}
	var pod corev1.Pod
	if err := json.Unmarshal(rawEvent.Object, &pod); err != nil {
		t.Logf("Response event object:\n%s\n", rawEvent)
		return nil, "", fmt.Errorf("failed to unmarshal pod: %w", err)
	}

	return &pod, rawEvent.Type, nil
}

func getRequestType(t *testing.T, reqMethod, reqTarget, resourceName string) string {
	t.Helper()

	if strings.Contains(reqTarget, "watch=") {
		return "WATCH"
	}

	if strings.Contains(reqTarget, "/binding") {
		return "BIND"
	}

	if reqMethod == http.MethodGet {
		// Remove query parameters
		path := reqTarget
		if idx := strings.Index(reqTarget, "?"); idx != -1 {
			path = reqTarget[:idx]
		}
		if strings.HasSuffix(path, "/"+resourceName) {
			return "LIST"
		}
	}

	return reqMethod
}

func handlePodDeletionResponse(t *testing.T, s *InMemoryKAPI, wantData []byte) error {
	t.Helper()
	wantPod, _ := convertJSONtoObject[corev1.Pod](t, wantData)
	c := mkapi.MatchCriteria{Namespace: wantPod.Namespace, Names: sets.New(wantPod.Name)}
	p, err := s.baseView.ListPods(c)
	if err != nil {
		return fmt.Errorf("Error listing pods")
	}
	if len(p) != 0 {
		return fmt.Errorf("Pod deletion unsuccesful")
	}
	return nil
}

func handlePodBindingResponse(t *testing.T, s *InMemoryKAPI, responseData, wantData []byte) error {
	t.Helper()
	gotStatus, _ := convertJSONtoObject[metav1.Status](t, responseData)
	if gotStatus.Status != metav1.StatusSuccess {
		return fmt.Errorf("Pod binding unsuccessful")
	}
	wantPodBind, _ := convertJSONtoObject[corev1.Binding](t, wantData)
	c := mkapi.MatchCriteria{Namespace: wantPodBind.Namespace, Names: sets.New(wantPodBind.Name)}
	p, err := s.baseView.ListPods(c)
	if err != nil {
		return fmt.Errorf("Error listing pods")
	}
	if len(p) == 0 {
		return fmt.Errorf("Pod not found")
	}
	if p[0].Spec.NodeName != wantPodBind.Target.Name {
		return fmt.Errorf("Pod binding unsuccessful")
	}
	t.Logf("Pod binding successful: nodeName is %s", p[0].Spec.NodeName)
	return nil
}

func compareHTTPHandlerResponse(t *testing.T, s *InMemoryKAPI, resp *http.Response, params RequestParams, ignoredFieldsForOutputComparison cmp.Option, wantData []byte, expectedStatus int) (err error) {
	t.Helper()
	var (
		got  corev1.Pod
		want any
	)
	reqType := getRequestType(t, params.Method, params.Target, "pods")

	responseData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Could not get response body: %v", err)
	}

	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("Unexpected status code, got: %s, expected: %d", resp.Status, expectedStatus)
	} else if resp.StatusCode != http.StatusOK {
		t.Logf("Expected status: %s", resp.Status)
		return nil
	}

	switch reqType {
	case "DELETE":
		if err := handlePodDeletionResponse(t, s, wantData); err != nil {
			return err
		}
		return nil

	case "BIND":
		if err := handlePodBindingResponse(t, s, responseData, wantData); err != nil {
			return err
		}
		return nil

	case "LIST":
		gotList, err := convertJSONtoObject[corev1.PodList](t, responseData)
		if err != nil {
			return fmt.Errorf("error converting response body to podlist: %v", err)
		}
		if len(gotList.Items) == 0 {
			t.Logf("No elements found for the requested LIST")
			return nil
		}
		got = gotList.Items[0]

	default:
		got, err = convertJSONtoObject[corev1.Pod](t, responseData)
		if err != nil {
			t.Logf(">>> got\n%s\n", string(responseData))
			return fmt.Errorf("error converting response body to pod object: %v", err)
		}
	}

	want, _ = convertJSONtoObject[corev1.Pod](t, wantData)

	if diff := cmp.Diff(want, got, ignoredFieldsForOutputComparison); diff != "" {
		t.Logf(">>> want\n%s\n", string(wantData))
		t.Logf(">>> got\n%s\n", string(responseData))
		t.Errorf("%s object mismatch (-want +got):\n%s", reqType, diff)
		return err
	}
	t.Logf("Got expected output")
	// t.Cleanup(func() { s.baseView.Reset() })

	return nil
}

func createObjectFromFileName[T any](t *testing.T, svc *InMemoryKAPI, fileName string, gvk schema.GroupVersionKind) (T, error) {
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
	_, err = svc.baseView.CreateObject(gvk, objInterface)
	if err != nil {
		return obj, err
	}
	t.Logf("Creating %s %s", gvk.Kind, objInterface.GetName())
	return obj, nil
}

func makeHTTPRequest(url, method, contentType string, body []byte) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	client := &http.Client{Timeout: 500 * time.Millisecond}
	return client.Do(req)
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
