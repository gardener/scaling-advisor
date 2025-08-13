// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gardener/scaling-advisor/minkapi/core/typeinfo"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

var testPod = corev1.Pod{
	ObjectMeta: metav1.ObjectMeta{
		Name:            "bingo",
		Namespace:       "default",
		ResourceVersion: "2",
	},
}

func TestAdd(t *testing.T) {
	tests := map[string]struct {
		typeMeta                         metav1.TypeMeta
		ignoredFieldsForOutputComparison cmp.Option
		retErr                           error
		expectedNumberOfObjects          int
	}{
		"correct typeMeta": {
			typeMeta:                         metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
			retErr:                           nil,
			expectedNumberOfObjects:          1,
		},
		"missing version in typeMeta": {
			typeMeta:                metav1.TypeMeta{Kind: "Pod"},
			retErr:                  fmt.Errorf("does not match expected objGVK"),
			expectedNumberOfObjects: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p := testPod.DeepCopy()
			p.TypeMeta = tc.typeMeta
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			obj1 := metav1.Object(p.DeepCopy())
			if err := s.Add(obj1); err != nil {
				assertNumberOfItems(t, s, tc.expectedNumberOfObjects)
				assertError(t, err, tc.retErr)
				return
			}
			assertNumberOfItems(t, s, tc.expectedNumberOfObjects)

			key := cache.NewObjectName(p.Namespace, p.Name).String()
			gotObj, err := s.GetByKey(key)
			if err != nil {
				t.Errorf("Error fetching gotObject from store")
			}

			if diff := cmp.Diff(p, gotObj.(*corev1.Pod), tc.ignoredFieldsForOutputComparison); diff != "" {
				t.Errorf("Received object mismatch (-want +got):\n%s", diff)
				return

			}
		})
	}
}

func TestUpdate(t *testing.T) {
	tests := map[string]struct {
		name                             string
		typeMeta                         metav1.TypeMeta
		ignoredFieldsForOutputComparison cmp.Option
		retErr                           error
		expectedNumberOfObjects          int
	}{
		"correct typeMeta": {
			typeMeta:                         metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
			retErr:                           nil,
			expectedNumberOfObjects:          1,
		},
		"missing version in typeMeta": {
			typeMeta:                metav1.TypeMeta{Kind: "Pod"},
			retErr:                  fmt.Errorf("does not match expected objGVK"),
			expectedNumberOfObjects: 1,
		},
		"update non-existent object": { // If object doesn't exist, it creates one
			name:                             "abcd",
			typeMeta:                         metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "Name", "ResourceVersion"),
			retErr:                           nil,
			expectedNumberOfObjects:          2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create object before updating
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			createdPod := testPod.DeepCopy()
			createdPod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
			if err := s.Add(metav1.Object(createdPod)); err != nil {
				t.Errorf("Error adding object to store")
				return
			}

			p := testPod.DeepCopy()
			p.TypeMeta = tc.typeMeta
			if tc.name != "" {
				p.ObjectMeta.Name = tc.name
			}
			obj1 := metav1.Object(p.DeepCopy())
			if err := s.Update(obj1); err != nil {
				assertNumberOfItems(t, s, tc.expectedNumberOfObjects)
				assertError(t, err, tc.retErr)
				return
			}
			assertNumberOfItems(t, s, tc.expectedNumberOfObjects)

			key := cache.NewObjectName(p.Namespace, p.Name).String()
			gotObj, err := s.GetByKey(key)
			if err != nil {
				t.Errorf("Error fetching gotObject from store")
			}

			if diff := cmp.Diff(createdPod, gotObj.(*corev1.Pod), tc.ignoredFieldsForOutputComparison); diff != "" {
				t.Errorf("Received object mismatch (-want +got):\n%s", diff)
				return

			}
			originalRV, err := strconv.ParseInt(createdPod.ResourceVersion, 10, 64)
			gotRV, err := strconv.ParseInt(gotObj.(*corev1.Pod).ResourceVersion, 10, 64)
			if gotRV != originalRV+1 {
				t.Errorf("Expected resourceVersion to increment by 1 (got: %d, want: %d)", gotRV, originalRV+1)
				return
			}
		})
	}
}

func TestDelete(t *testing.T) {
	tests := map[string]struct {
		name                      string
		retErr                    error
		createObjectBeforeTesting bool
		expectedNumberOfObjects   int
	}{
		"correct deletion": {
			name:                      testPod.Name,
			createObjectBeforeTesting: true,
			expectedNumberOfObjects:   0,
			retErr:                    nil,
		},
		"delete non-existent object": {
			name:                      testPod.Name,
			createObjectBeforeTesting: false,
			expectedNumberOfObjects:   0,
			retErr:                    fmt.Errorf("not found"),
		},
		"delete object with wrong key": {
			name:                      "abcd",
			createObjectBeforeTesting: true,
			expectedNumberOfObjects:   1,
			retErr:                    fmt.Errorf("not found"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)

			createdPod := testPod.DeepCopy()
			if tc.createObjectBeforeTesting {
				createdPod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
				if err := s.Add(metav1.Object(createdPod)); err != nil {
					t.Errorf("Error adding object to store")
					return
				}
			}

			key := cache.NewObjectName(createdPod.Namespace, tc.name).String()
			gotObj, _ := s.GetByKey(key)
			if err := s.Delete(key); err != nil {
				assertNumberOfItems(t, s, tc.expectedNumberOfObjects)
				assertError(t, err, tc.retErr)
				return
			}
			assertNumberOfItems(t, s, tc.expectedNumberOfObjects)

			mo, _ := AsMeta(s.log, gotObj)
			if !reflect.DeepEqual(mo.GetDeletionTimestamp().Time, time.Time{}) { // FIXME
				t.Errorf("Expected deletionTimestamp to be set for object that's successfully deleted, got: %v", mo.GetDeletionTimestamp())
				return
			}
		})
	}
}

func TestGetByKey(t *testing.T) {
	tests := map[string]struct {
		key                       string
		errorCheckFunc            func(error) bool
		objectFound               bool
		createObjectBeforeTesting bool
	}{
		"fetch existing object": {
			key:                       fmt.Sprintf("%s/%s", testPod.Namespace, testPod.Name),
			objectFound:               true,
			createObjectBeforeTesting: true,
		},
		"fetch non-existent object": {
			key:                       fmt.Sprintf("%s/%s", testPod.Namespace, testPod.Name),
			objectFound:               false,
			createObjectBeforeTesting: false,
			errorCheckFunc:            apierrors.IsNotFound,
		},
		"fetch object with wrong key": {
			key:                       fmt.Sprintf("%s/%s", testPod.Namespace, "abcd"),
			objectFound:               false,
			createObjectBeforeTesting: true,
			errorCheckFunc:            apierrors.IsNotFound,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)

			createdPod := testPod.DeepCopy()
			if tc.createObjectBeforeTesting {
				createdPod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
				if err := s.Add(metav1.Object(createdPod)); err != nil {
					t.Errorf("Error adding object to store")
					return
				}
			}

			_, err := s.GetByKey(tc.key)
			if err != nil {
				if !tc.errorCheckFunc(err) {
					t.Errorf("Expected error to be %s, got: %v",
						getFunctionName(t, tc.errorCheckFunc), err,
					)
					return
				}
				return
			}

		})
	}
}

func TestList(t *testing.T) {
	s := createStoreForTesting(typeinfo.PodsDescriptor)

	createdPods := make([]corev1.Pod, 3)
	for i := range 3 {
		createdPods[i] = *testPod.DeepCopy()
		createdPods[i].TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
		createdPods[i].Name = fmt.Sprintf("%s-%d", testPod.Name, i)

		createdPods[i].Labels = make(map[string]string)
		for j := range i + 1 {
			createdPods[i].Labels[fmt.Sprintf("k%d", j)] = fmt.Sprintf("v%d", j)
		}

		if err := s.Add(metav1.Object(&createdPods[i])); err != nil {
			t.Errorf("Error adding object to store")
			return
		}
	}

	tests := map[string]struct {
		namespace               string
		labelSelector           labels.Selector
		retErr                  error
		expectedNumberOfObjects int
	}{
		"base": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.SelectorFromSet(labels.Set{"k0": "v0"}),
			retErr:                  nil,
			expectedNumberOfObjects: 3,
		},
		"labels that don't match all objects": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.SelectorFromSet(labels.Set{"k1": "v1"}),
			retErr:                  nil,
			expectedNumberOfObjects: 2,
		},
		"empty labelSelector": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			expectedNumberOfObjects: 3,
		},
		"non-matching namespace": {
			namespace:               "abcd",
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			expectedNumberOfObjects: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			objList, err := s.List(tc.namespace, tc.labelSelector)
			if err != nil {
				assertError(t, err, tc.retErr)
			}
			podList, ok := objList.(*corev1.PodList)
			if !ok {
				t.Errorf("object is not a PodList, got %T", objList)
				return
			}
			if len(podList.Items) != tc.expectedNumberOfObjects {
				t.Errorf("Expected returned number of objects to be %d, got %d",
					tc.expectedNumberOfObjects,
					len(podList.Items),
				)
			}
			for _, p := range podList.Items {
				t.Logf("Pod: %s rV: %s labels %v", p.Name, p.ResourceVersion, p.Labels)
			}
		})
	}
}

func createStoreForTesting(d typeinfo.Descriptor) *InMemResourceStore {
	queueSize := 100
	watchTimeout := time.Duration(2 * time.Second)
	log := klog.NewKlogr().V(4)
	return NewInMemResourceStore(d.GVK, d.ListGVK, d.GVR.GroupResource().Resource, queueSize, watchTimeout, typeinfo.SupportedScheme, log)
}

func assertError(t *testing.T, got error, want error) {
	t.Helper()
	if errors.Is(got, want) || strings.Contains(got.Error(), want.Error()) {
		t.Logf("Expected error: %v", got)
	} else {
		t.Errorf("Unexpected error, got: %v, want: %v", got, want)
	}
}

func assertNumberOfItems(t *testing.T, s *InMemResourceStore, want int) {
	t.Helper()
	got := len(s.delegate.List())
	if got != want {
		t.Errorf("Unexpected number of items, got: %v, want: %v", got, want)
	} else {
		t.Logf("Expected number of items, got: %v", got)
	}
}

func getFunctionName(t *testing.T, fn any) string {
	t.Helper()
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}
