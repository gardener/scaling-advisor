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
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/cli"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
	"github.com/gardener/scaling-advisor/minkapi/server/view"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestPatchPodStatus(t *testing.T) {
	var testPodPatchStatus = `{
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
	var testIncorrectPatch = `{
"status" : {
	"conditions" : "not-an-array"
  }
}`

	tests := map[string]struct {
		patch      string
		key        string
		patchErr   error
		passNilObj bool
	}{
		"correct patch": {
			key:      "default/bingo",
			patchErr: nil,
			patch:    testPodPatchStatus,
		},
		"incorrect patch": {
			key:      "default/bingo",
			patchErr: fmt.Errorf("failed to unmarshal patched status"),
			patch:    testIncorrectPatch,
		},
		"nil Object": {
			key:        "default/bingo",
			patchErr:   fmt.Errorf("non-nil pointer"),
			patch:      testPodPatchStatus,
			passNilObj: true,
		},
		"patch with no status": {
			key:      "default/bingo",
			patchErr: fmt.Errorf("does not contain a 'status' key"),
			patch:    `{}`,
		},
		"corrupted patch": {
			key:      "default/bingo",
			patchErr: fmt.Errorf("failed to parse patch"),
			patch:    `{{}`,
		},
		"incorrect key": { // TODO Is key only utilized for error messages
			key:      "default/abc",
			patchErr: nil,
			patch:    testPodPatchStatus,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			obj, err := typeinfo.PodsDescriptor.CreateObject()
			if err != nil {
				t.Errorf("Failed to create pod: %v", err)
				return
			}
			pod := obj.(*corev1.Pod)
			if tc.passNilObj {
				err = patchStatus(nil, tc.key, []byte(tc.patch))
			} else {
				err = patchStatus(obj.(runtime.Object), tc.key, []byte(tc.patch))
			}
			if err != nil {
				if !strings.Contains(err.Error(), tc.patchErr.Error()) {
					t.Errorf("Unexpected error type when patching status, got: %v", err)
					return
				} else {
					t.Logf("Expected error type: %v", err)
					return
				}
			}
			t.Logf("Patched pod status: %#v", pod.Status.Conditions)
			if pod.Status.Conditions == nil {
				t.Errorf("Failed to set pod conditions")
			}
		})
	}
}

func TestPatchObjectUsingEvent(t *testing.T) {
	var patchEventSeries = `
{
  "series": {
	"count": 2,
	"lastObservedTime": "2025-05-08T09:05:57.028064Z"
  }
}
`
	var corruptedPatch = `{}}`
	var invalidPatch = `{ "metadata": "abcdefgh"}`
	contentTypeTests := map[string]struct {
		contentType string
		patchData   string
		patchErr    error
		passNilObj  bool
	}{
		"Strategic Merge Patch":           {contentType: "application/strategic-merge-patch+json", patchData: patchEventSeries, patchErr: nil},
		"Merge Patch":                     {contentType: "application/merge-patch+json", patchData: patchEventSeries, patchErr: nil},
		"Unsupported ContentType":         {contentType: "application/json-patch+json", patchData: patchEventSeries, patchErr: fmt.Errorf("unsupported patch content type")},
		"Corrupted Strategic Merge Patch": {contentType: "application/strategic-merge-patch+json", patchData: corruptedPatch, patchErr: fmt.Errorf("invalid JSON")},
		"Corrupted Merge Patch":           {contentType: "application/merge-patch+json", patchData: corruptedPatch, patchErr: fmt.Errorf("Invalid JSON")},
		"invalid Patch":                   {contentType: "application/merge-patch+json", patchData: invalidPatch, patchErr: fmt.Errorf("failed to unmarshal patched JSON")},
		"Nil object Patch":                {contentType: "application/merge-patch+json", patchData: patchEventSeries, patchErr: fmt.Errorf("non-nil pointer"), passNilObj: true},
	}

	for name, tc := range contentTypeTests {
		t.Run(name, func(t *testing.T) {
			key := "default/a-bingo.aaabbb"
			obj, err := typeinfo.EventsDescriptor.CreateObject()
			if err != nil {
				t.Errorf("Failed to create event: %v", err)
				return
			}
			event := obj.(*eventsv1.Event)
			if tc.passNilObj {
				err = patchObject(nil, key, tc.contentType, []byte(tc.patchData))
			} else {
				err = patchObject(obj.(runtime.Object), key, tc.contentType, []byte(tc.patchData))
			}
			if err != nil {
				if !strings.Contains(err.Error(), tc.patchErr.Error()) {
					t.Errorf("Unexpected error type when patching object, got: %v", err)
					return
				} else {
					t.Logf("Expected error type: %v", err)
					return
				}
			}
			t.Logf("Patched event series: %v", event.Series)
			if event.Series == nil {
				t.Errorf("Failed to patch event series")
			}
		})
	}
}

// ---------------------------------------------------------------------------------------------
func TestHTTPHandlers(t *testing.T) {
	s, mux, err := startMinkapiService(t)
	if err != nil {
		t.Errorf("Can not start minkapi service: %v", err)
		return
	}

	tests := map[string]struct {
		filePath                         string
		reqMethod                        string
		reqTarget                        string
		reqContentType                   string
		expectedStatus                   int
		createObjectBeforeRequest        bool
		ignoredFieldsForOutputComparison cmp.Option
	}{
		"pod creation": {
			filePath:                         "./testdata/pod-a.json",
			reqMethod:                        http.MethodPost,
			reqTarget:                        "/api/v1/namespaces/default/pods",
			reqContentType:                   "application/json",
			expectedStatus:                   http.StatusOK,
			createObjectBeforeRequest:        false,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		},
		"invalid request target": {
			filePath:                  "./testdata/pod-a.json",
			reqMethod:                 http.MethodPost,
			reqTarget:                 "/api/v1/namespaces/default/pods/bingo",
			reqContentType:            "application/json",
			expectedStatus:            http.StatusMethodNotAllowed,
			createObjectBeforeRequest: false,
		},
		"create corrupted pod": {
			filePath:                  "./testdata/corrupt-pod-a.json",
			reqMethod:                 http.MethodPost,
			reqTarget:                 "/api/v1/namespaces/default/pods",
			reqContentType:            "application/json",
			expectedStatus:            http.StatusBadRequest,
			createObjectBeforeRequest: false,
		},
		"create pod without namespace in request target": {
			filePath:                         "./testdata/pod-a.json",
			reqMethod:                        http.MethodPost,
			reqTarget:                        "/api/v1/pods",
			reqContentType:                   "application/json",
			expectedStatus:                   http.StatusOK,
			createObjectBeforeRequest:        false,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		},
		"create pod missing name and generateName": { // TODO Patch pod-a rather than creating files
			filePath:                         "./testdata/name-miss-pod-a.json",
			reqMethod:                        http.MethodPost,
			reqTarget:                        "/api/v1/namespaces/default/pods",
			reqContentType:                   "application/json",
			expectedStatus:                   http.StatusBadRequest,
			createObjectBeforeRequest:        false,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		},
		"create pod missing name, UID and creationTimestamp": { // TODO Patch pod-a rather than creating files
			filePath:                         "./testdata/uid-ts-pod-a.json",
			reqMethod:                        http.MethodPost,
			reqTarget:                        "/api/v1/namespaces/default/pods",
			reqContentType:                   "application/json",
			expectedStatus:                   http.StatusOK,
			createObjectBeforeRequest:        false,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion", "Name", "Namespace", "UID", "CreationTimestamp"),
		},
		"fetch existing pod": {
			filePath:                         "./testdata/pod-a.json",
			reqMethod:                        http.MethodGet,
			reqTarget:                        "/api/v1/namespaces/default/pods/bingo",
			reqContentType:                   "application/json",
			expectedStatus:                   http.StatusOK,
			createObjectBeforeRequest:        true,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		},
		"delete existing pod": {
			filePath:                  "./testdata/pod-a.json",
			reqMethod:                 http.MethodDelete,
			reqTarget:                 "/api/v1/namespaces/default/pods/bingo",
			reqContentType:            "application/json",
			expectedStatus:            http.StatusOK,
			createObjectBeforeRequest: true,
		},
		"fetch non-existent pod": {
			filePath:                  "./testdata/pod-a.json",
			reqMethod:                 http.MethodGet,
			reqTarget:                 "/api/v1/namespaces/default/pods/bingo",
			reqContentType:            "application/json",
			expectedStatus:            http.StatusNotFound,
			createObjectBeforeRequest: false,
		},
		"delete non-existent pod": {
			filePath:                  "./testdata/pod-a.json",
			reqMethod:                 http.MethodDelete,
			reqTarget:                 "/api/v1/namespaces/default/pods/bingo",
			reqContentType:            "application/json",
			expectedStatus:            http.StatusNotFound,
			createObjectBeforeRequest: false,
		}, // TODO: add delete error
		"update non-existent pod": {
			filePath:                  "./testdata/pod-a.json",
			reqMethod:                 http.MethodPut,
			reqTarget:                 "/api/v1/namespaces/default/pods/bingo",
			reqContentType:            "application/json",
			expectedStatus:            http.StatusNotFound,
			createObjectBeforeRequest: false,
		},
		"watch all pods": {
			filePath:                         "./testdata/pod-a.json",
			reqMethod:                        http.MethodGet,
			reqTarget:                        "/api/v1/pods?watch=1&resourceVersion=0",
			reqContentType:                   "application/json",
			expectedStatus:                   http.StatusOK,
			createObjectBeforeRequest:        true,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		},
		// "watch high resVer": { //
		// 	filePath:                         "./testdata/pod-a.json",
		// 	reqMethod:                        http.MethodGet,
		// 	reqTarget:                        "/api/v1/pods?watch=1&resourceVersion=10000",
		// 	reqContentType:                   "application/json",
		// 	expectedStatus:                   http.StatusOK,
		// 	createObjectBeforeRequest:        true,
		// 	ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		// },
		"erroneous label selector for pods": {
			filePath:                  "./testdata/pod-a.json",
			reqMethod:                 http.MethodGet,
			reqTarget:                 "/api/v1/pods?labelSelector=app.kubernetes.io/name=*?",
			reqContentType:            "application/json",
			expectedStatus:            http.StatusBadRequest,
			createObjectBeforeRequest: true,
		}, // TODO: check correct label selector working
		"fetch pod list": {
			filePath:                         "./testdata/pod-a.json",
			reqMethod:                        http.MethodGet,
			reqTarget:                        "/api/v1/namespaces/default/pods",
			reqContentType:                   "application/json",
			expectedStatus:                   http.StatusOK,
			createObjectBeforeRequest:        true,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		},
		"matching label selector for pods": {
			filePath:                         "./testdata/pod-a.json",
			reqMethod:                        http.MethodGet,
			reqTarget:                        "/api/v1/pods?labelSelector=app.kubernetes.io/component=minkapitest",
			reqContentType:                   "application/json",
			expectedStatus:                   http.StatusOK,
			createObjectBeforeRequest:        true,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		},
		"non-matching label selector for pods": {
			filePath:                         "./testdata/pod-a.json",
			reqMethod:                        http.MethodGet,
			reqTarget:                        "/api/v1/pods?labelSelector=app.kubernetes.io/component=abcdefgh",
			reqContentType:                   "application/json",
			expectedStatus:                   http.StatusOK,
			createObjectBeforeRequest:        true,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		},
		"create pod binding": {
			filePath:                         "./testdata/binding-pod-a.json",
			reqMethod:                        http.MethodPost,
			reqTarget:                        "/api/v1/namespaces/default/pods/bingo/binding",
			reqContentType:                   "application/json",
			expectedStatus:                   http.StatusOK,
			createObjectBeforeRequest:        true,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
		},
	}

	t.Cleanup(func() { s.Stop(context.TODO()) })
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() {
				cleanupTestPod(t, s, api.MatchCriteria{
					Labels: map[string]string{"app.kubernetes.io/component": "minkapitest"},
				})
			})

			if tc.createObjectBeforeRequest {
				if _, err := createObjectFromFileName[corev1.Pod](t, s, "./testdata/pod-a.json", typeinfo.PodsDescriptor.GVK); err != nil {
					t.Errorf("Error creating test object: %v", err)
				}
			}

			jsonData, err := os.ReadFile(tc.filePath)
			if err != nil {
				t.Logf("failed to read: %v", err)
				return
			}

			requestData := bytes.NewReader(jsonData)
			req := httptest.NewRequest(tc.reqMethod, tc.reqTarget, requestData)
			req.Header.Set("Content-Type", tc.reqContentType)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			resp := w.Result()
			defer resp.Body.Close()

			reqType := getRequestType(t, tc.reqMethod, tc.reqTarget, "pods")
			if reqType == "WATCH" {
				if err := handleTestWatchResponse(t, resp); err != nil {
					t.Errorf("Could not get watch response: %v", err)
				}
				return
			}

			responseData, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("expected error to be nil got %v", err)
				return
			}
			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Unexpected status code, got: %d, expected: %d", resp.StatusCode, tc.expectedStatus)
				t.Logf(">>> Got response: %s\n", string(responseData))
				return
			} else if resp.StatusCode != http.StatusOK {
				t.Logf("Expected status error: %s", resp.Status)
				return
			}
			// if reqType == "PATCH" || reqType == "PUT" {
			// 	jsonData, _ = os.ReadFile("./testdata/pod-a.json")
			// }
			// Got status OK, now check the output
			if err = compareHTTPHandlerResponse(t, s, responseData, reqType, tc.ignoredFieldsForOutputComparison, jsonData); err != nil {
				t.Errorf("Failed: %v", err)
			} else {
				want, _ := convertJSONtoObject[corev1.Pod](t, jsonData)
				t.Logf("%s object %s successful", reqType, want.Name)
			}
		})
	}
}

func TestAPIHandlerMethods(t *testing.T) {
	s, _, err := startMinkapiService(t)
	if err != nil {
		t.Errorf("Can not start minkapi service: %v", err)
		return
	}

	tests := map[string]struct {
		reqMethod      string
		reqTarget      string
		reqContentType string
		expectedStatus int
		want           any
		handlerFunc    http.HandlerFunc
	}{
		"invalid request for api groups": {
			reqMethod:      http.MethodPost,
			reqTarget:      "/apis",
			reqContentType: "application/json",
			expectedStatus: http.StatusMethodNotAllowed,
			want:           typeinfo.SupportedAPIGroups,
			handlerFunc:    s.handleAPIGroups,
		},
		"get request for api groups": {
			reqMethod:      http.MethodGet,
			reqTarget:      "/apis",
			reqContentType: "application/json",
			expectedStatus: http.StatusOK,
			want:           typeinfo.SupportedAPIGroups,
			handlerFunc:    s.handleAPIGroups,
		},
		"invalid request for api versions": {
			reqMethod:      http.MethodPost,
			reqTarget:      "/api",
			reqContentType: "application/json",
			expectedStatus: http.StatusMethodNotAllowed,
			want:           typeinfo.SupportedAPIVersions,
			handlerFunc:    s.handleAPIVersions,
		},
		"get request for api versions": {
			reqMethod:      http.MethodGet,
			reqTarget:      "/api",
			reqContentType: "application/json",
			expectedStatus: http.StatusOK,
			want:           typeinfo.SupportedAPIVersions,
			handlerFunc:    s.handleAPIVersions,
		},
		"invalid request for api resources": {
			reqMethod:      http.MethodPost,
			reqTarget:      "/api/v1/",
			reqContentType: "application/json",
			expectedStatus: http.StatusMethodNotAllowed,
			want:           typeinfo.SupportedCoreAPIResourceList,
		},
		"get request for api resources": {
			reqMethod:      http.MethodGet,
			reqTarget:      "/api/v1/",
			reqContentType: "application/json",
			expectedStatus: http.StatusOK,
			want:           typeinfo.SupportedCoreAPIResourceList,
		},
	}
	t.Cleanup(func() { s.Stop(context.TODO()) })
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(tc.reqMethod, tc.reqTarget, nil)
			req.Header.Set("Content-Type", tc.reqContentType)
			w := httptest.NewRecorder()
			if tc.reqTarget == "/api/v1/" {
				testFunc := s.handleAPIResources(tc.want.(metav1.APIResourceList))
				testFunc(w, req)
			} else {
				tc.handlerFunc(w, req)
			}
			resp := w.Result()
			defer resp.Body.Close()

			responseData, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("expected error to be nil got %v", err)
				return
			}
			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Unexpected status code, got: %d, expected: %d", resp.StatusCode, tc.expectedStatus)
				t.Logf(">>> Got response: %s\n", string(responseData))
				return
			} else if resp.StatusCode != http.StatusOK {
				t.Logf("Expected status error: %s", resp.Status)
				return
			}
			var got any
			switch tc.reqTarget {
			case "/apis":
				got, _ = convertJSONtoObject[metav1.APIGroupList](t, responseData)
			case "/api":
				got, _ = convertJSONtoObject[metav1.APIVersions](t, responseData)
			case "/api/v1/":
				got, _ = convertJSONtoObject[metav1.APIResourceList](t, responseData)
			}
			if diff := cmp.Diff(tc.want, got, nil); diff != "" {
				t.Errorf("%s object mismatch (-want +got):\n%s", tc.reqMethod, diff)
				return
			} else {
				t.Logf("Got expected output")
			}
		})
	}
}

func TestPatchPutHTTPHandlers(t *testing.T) {
	s, mux, err := startMinkapiService(t)
	if err != nil {
		t.Errorf("Can not start minkapi service: %v", err)
		return
	}
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
	var corruptedPatch = `{}}`
	data, _ := os.ReadFile("./testdata/corrupt-pod-a.json")
	var corruptedPodResource = string(data)
	data, _ = os.ReadFile("./testdata/update-pod-a.json")
	var updatedPodResource = string(data)

	patchTests := map[string]struct {
		patchData                        string
		reqMethod                        string
		reqTarget                        string
		reqContentType                   string
		expectedStatus                   int
		createObjectBeforeRequest        bool
		ignoredFieldsForOutputComparison cmp.Option
	}{
		"patch pod status": {
			patchData:                        testPodPatchStatus,
			reqMethod:                        http.MethodPatch,
			reqTarget:                        "/api/v1/namespaces/default/pods/bingo/status",
			reqContentType:                   "application/strategic-merge-patch+json",
			expectedStatus:                   http.StatusOK,
			createObjectBeforeRequest:        true,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion", "Status.Conditions"),
		},
		"patch pod status with unsupported content type": {
			patchData:                        testPodPatchStatus,
			reqMethod:                        http.MethodPatch,
			reqTarget:                        "/api/v1/namespaces/default/pods/bingo/status",
			reqContentType:                   "application/json-patch+json",
			expectedStatus:                   http.StatusInternalServerError, // FIXME why internal
			createObjectBeforeRequest:        true,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion", "Status.Conditions"),
		},
		"patch pod": {
			patchData:                        testPatchName,
			reqMethod:                        http.MethodPatch,
			reqTarget:                        "/api/v1/namespaces/default/pods/bingo",
			reqContentType:                   "application/strategic-merge-patch+json",
			expectedStatus:                   http.StatusOK,
			createObjectBeforeRequest:        true,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion", "Name"),
		}, // TODO: patch pod with readAll patchData error
		"patch pod with unsupported content type": {
			patchData:                        testPatchName,
			reqMethod:                        http.MethodPatch,
			reqTarget:                        "/api/v1/namespaces/default/pods/bingo",
			reqContentType:                   "application/json-patch+json",
			expectedStatus:                   http.StatusBadRequest,
			createObjectBeforeRequest:        true,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion", "Name"),
		},
		"corrupted patch pod": {
			patchData:                 corruptedPatch,
			reqMethod:                 http.MethodPatch,
			reqTarget:                 "/api/v1/namespaces/default/pods/bingo",
			reqContentType:            "application/strategic-merge-patch+json",
			expectedStatus:            http.StatusInternalServerError, // FIXME why should this return internal server error
			createObjectBeforeRequest: true,
		},
		"corrupted patch pod status": {
			patchData:                 corruptedPatch,
			reqMethod:                 http.MethodPatch,
			reqTarget:                 "/api/v1/namespaces/default/pods/bingo/status",
			reqContentType:            "application/strategic-merge-patch+json",
			expectedStatus:            http.StatusInternalServerError, // FIXME why should this return internal server error
			createObjectBeforeRequest: true,
		},
		"update with corrupted object": {
			patchData:                 corruptedPodResource,
			reqMethod:                 http.MethodPut,
			reqTarget:                 "/api/v1/namespaces/default/pods/bingo",
			reqContentType:            "application/json",
			expectedStatus:            http.StatusBadRequest,
			createObjectBeforeRequest: true,
		},
		"update with new object": {
			patchData:                        updatedPodResource,
			reqMethod:                        http.MethodPut,
			reqTarget:                        "/api/v1/namespaces/default/pods/bingo",
			reqContentType:                   "application/json",
			expectedStatus:                   http.StatusOK,
			createObjectBeforeRequest:        true,
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion", "Name"),
		},
		"patch status of non-existent pod": {
			patchData:                 testPodPatchStatus,
			reqMethod:                 http.MethodPatch,
			reqTarget:                 "/api/v1/namespaces/default/pods/bingoz/status",
			reqContentType:            "application/strategic-merge-patch+json",
			expectedStatus:            http.StatusNotFound,
			createObjectBeforeRequest: false,
		},
		"patch non-existent pod": {
			patchData:                 testPatchName,
			reqMethod:                 http.MethodPatch,
			reqTarget:                 "/api/v1/namespaces/default/pods/bingoz",
			reqContentType:            "application/strategic-merge-patch+json",
			expectedStatus:            http.StatusNotFound,
			createObjectBeforeRequest: false,
		},
	}

	t.Cleanup(func() { s.Stop(context.TODO()) })
	for name, tc := range patchTests {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() { cleanupTestPod(t, s, api.MatchCriteria{}) })

			jsonData, err := os.ReadFile("./testdata/pod-a.json")
			if err != nil {
				t.Logf("failed to read: %v", err)
				return
			}
			if tc.createObjectBeforeRequest {
				if _, err := createObjectFromFileName[corev1.Pod](t, s, "./testdata/pod-a.json", typeinfo.PodsDescriptor.GVK); err != nil {
					t.Errorf("Error creating test object: %v", err)
				}
			}

			testObj := bytes.NewReader([]byte(tc.patchData))
			req := httptest.NewRequest(tc.reqMethod, tc.reqTarget, testObj)
			req.Header.Set("Content-Type", tc.reqContentType)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()
			responseData, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("expected error to be nil got %v", err)
				return
			}
			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Unexpected status code, got: %d, expected: %d", resp.StatusCode, tc.expectedStatus)
				return
			} else if resp.StatusCode != http.StatusOK {
				t.Logf("Expected status error: %s", resp.Status)
				return
			}

			want, _ := convertJSONtoObject[corev1.Pod](t, jsonData)
			t.Logf("%s object %s successful", tc.reqMethod, want.Name)
			got, _ := convertJSONtoObject[corev1.Pod](t, responseData)
			if diff := cmp.Diff(want, got, tc.ignoredFieldsForOutputComparison); diff != "" {
				t.Errorf("%s object mismatch (-want +got):\n%s", tc.reqMethod, diff)
			}
			t.Cleanup(func() { cleanupTestPod(t, s, api.MatchCriteria{Names: sets.New(got.Name)}) })
		})
	}
}

// -- Helper functions ------------------------------------------------------------------------
func handleTestWatchResponse(t *testing.T, resp *http.Response) error {
	t.Helper()
	scanner := bufio.NewScanner(resp.Body)
	eventCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		pod, eventType, err := parseWatchEvent(t, line)
		if err != nil {
			t.Logf("Failed to parse watch event: %v", err)
			continue
		}

		t.Logf("Watch event: %s pod %s/%s, resourceVersion: %s", eventType, pod.Namespace, pod.Name, pod.ResourceVersion)
		eventCount++
		if eventType == "ADDED" && eventCount >= 1 {
			break
		}
	}
	if scanner.Err() != nil {
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
		return nil, "", fmt.Errorf("failed to unmarshal pod: %w", err)
	}

	return &pod, rawEvent.Type, nil
}

func getRequestType(t *testing.T, reqMethod, reqTarget, resourceName string) string {
	t.Helper()
	if idx := strings.Index(reqTarget, "watch="); idx != -1 {
		return "WATCH"
	}
	if idx := strings.Index(reqTarget, "/binding"); idx != -1 {
		return "BIND"
	}
	if idx := strings.Index(reqTarget, "/"+resourceName); idx != -1 && (reqTarget == reqTarget[:idx+1+len(resourceName)] || strings.Contains(reqTarget, resourceName+"?")) { // FIXME what about label selector reqTarget
		if reqMethod == http.MethodGet {
			return "LIST"
		} else if reqMethod != http.MethodPost {
			return "UNKNOWN"
		}
	}
	return reqMethod
}

func compareHTTPHandlerResponse(t *testing.T, s *InMemoryKAPI, responseData []byte, reqType string, ignoredFieldsForOutputComparison cmp.Option, jsonData []byte) error {
	var (
		got  corev1.Pod
		err  error
		want any
	)

	switch reqType {
	case "DELETE":
		wantPod, _ := convertJSONtoObject[corev1.Pod](t, jsonData)
		p, err := s.baseView.ListPods(wantPod.Namespace, []string{wantPod.Name}...)
		if err != nil {
			err := fmt.Errorf("Error listing pods")
			return err
		}
		if len(p) != 0 {
			err := fmt.Errorf("Pod deletion unsuccesful")
			return err
		}
		return nil

	case "BIND":
		gotStatus, _ := convertJSONtoObject[metav1.Status](t, responseData)
		if gotStatus.Status != metav1.StatusSuccess {
			err := fmt.Errorf("Pod binding unsuccessful")
			return err
		}
		wantPodBind, _ := convertJSONtoObject[corev1.Binding](t, jsonData)
		p, err := s.baseView.ListPods(wantPodBind.Namespace, []string{wantPodBind.Name}...)
		if err != nil {
			err := fmt.Errorf("Error listing pods")
			return err
		}
		if len(p) == 0 {
			err := fmt.Errorf("Pod not found")
			return err
		}
		if p[0].Spec.NodeName == wantPodBind.Target.Name {
			t.Logf("Pod binding successful: nodeName is %s", p[0].Spec.NodeName)
			return nil
		}

	case "LIST":
		gotList, err := convertJSONtoObject[corev1.PodList](t, responseData)
		if err != nil {
			err := fmt.Errorf("error converting response body to podlist: %v", err)
			return err
		}
		if len(gotList.Items) > 0 {
			got = gotList.Items[0]
		} else {
			t.Logf("No elements found for the requested LIST")
			return nil
		}

	default:
		got, err = convertJSONtoObject[corev1.Pod](t, responseData)
		if err != nil {
			err := fmt.Errorf("error converting response body to pod object: %v", err)
			return err
		}
	}
	want, _ = convertJSONtoObject[corev1.Pod](t, jsonData)
	if diff := cmp.Diff(want, got, ignoredFieldsForOutputComparison); diff != "" {
		t.Logf(">>> want\n%s\n", string(jsonData))
		t.Logf(">>> got\n%s\n", string(responseData))
		t.Errorf("%s object mismatch (-want +got):\n%s", reqType, diff)
		return err
	}
	t.Cleanup(func() { cleanupTestPod(t, s, api.MatchCriteria{Names: sets.New(got.Name)}) })
	// t.Cleanup(func() {
	// 	cleanupTestPod(t, s, map[string]string{"app.kubernetes.io/component": "minkapitest"})
	// })

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
	err = svc.baseView.CreateObject(gvk, objInterface)
	if err != nil {
		return obj, err
	}
	t.Logf("Creating %s %s", gvk.Kind, objInterface.GetName())
	return obj, nil
}

func startMinkapiService(t *testing.T) (*InMemoryKAPI, *http.ServeMux, error) {
	t.Helper()

	mainOpts, err := cli.ParseProgramFlags([]string{
		"-k", "/tmp/minkapi-test.yaml",
		"-H", "localhost",
		"-P", "9892",
		"-t", "0.5s",
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Err: %v\n", err)
		return nil, nil, err
	}
	cfg := mainOpts.MinKAPIConfig
	log := logr.FromContextOrDiscard(context.TODO())

	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", api.ErrInitFailed, err)
		}
	}()
	setMinKAPIConfigDefaults(&cfg)
	scheme := typeinfo.SupportedScheme
	baseView, err := view.New(log, cfg.KubeConfigPath, scheme, cfg.WatchQueueSize, cfg.WatchTimeout)
	if err != nil {
		return nil, nil, err
	}
	rootMux := http.NewServeMux()
	s := &InMemoryKAPI{
		cfg:     cfg,
		scheme:  scheme,
		rootMux: rootMux,
		server: &http.Server{
			Addr:    net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
			Handler: rootMux,
		},
		baseView: baseView,
		log:      log,
	}
	baseViewMux := http.NewServeMux()
	s.registerRoutes(baseViewMux, cfg.BasePrefix)
	return s, baseViewMux, err
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

func cleanupTestPod(t *testing.T, s *InMemoryKAPI, c api.MatchCriteria) {
	t.Helper()
	err := s.baseView.DeleteObjects(typeinfo.PodsDescriptor.GVK, c)
	if err != nil {
		t.Errorf("Error while performing cleanup of pods: %v", err)
		return
	}
	t.Logf(">>> Cleanup: Deleting Pod")
}
