// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/common/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

var testPod = corev1.Pod{
	ObjectMeta: metav1.ObjectMeta{
		Name:            "bingo",
		Namespace:       metav1.NamespaceDefault,
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
			t.Cleanup(func() { s.Close() })

			obj1 := metav1.Object(p.DeepCopy())
			if err := s.Add(obj1); err != nil {
				assertNumberOfItems(t, s, tc.expectedNumberOfObjects)
				testutil.AssertError(t, err, tc.retErr)
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
		//"update non-existent object": { // If object doesn't exist, it creates one
		//	name:                             "abcd",
		//	typeMeta:                         metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		//	ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "InstanceType", "ResourceVersion"),
		//	retErr:                           nil,
		//	expectedNumberOfObjects:          2,
		//},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create object before updating
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })

			createdPod := testPod.DeepCopy()
			createdPod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
			if err := s.Add(metav1.Object(createdPod)); err != nil {
				t.Errorf("Error adding object to store")
				return
			}

			p := testPod.DeepCopy()
			p.TypeMeta = tc.typeMeta
			if tc.name != "" {
				p.Name = tc.name
			}
			obj1 := metav1.Object(p.DeepCopy())
			if err := s.Update(obj1); err != nil {
				assertNumberOfItems(t, s, tc.expectedNumberOfObjects)
				testutil.AssertError(t, err, tc.retErr)
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
			if err != nil {
				t.Errorf("Error converting resourceVersion to integer")
				return
			}
			gotRV, err := strconv.ParseInt(gotObj.(*corev1.Pod).ResourceVersion, 10, 64)
			if err != nil {
				t.Errorf("Error converting resourceVersion to integer")
				return
			}
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
			t.Cleanup(func() { s.Close() })

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
			if err := s.DeleteByKey(key); err != nil {
				assertNumberOfItems(t, s, tc.expectedNumberOfObjects)
				testutil.AssertError(t, err, tc.retErr)
				return
			}
			assertNumberOfItems(t, s, tc.expectedNumberOfObjects)

			mo, _ := AsMeta(gotObj)
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
			t.Cleanup(func() { s.Close() })

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
						testutil.GetFunctionName(t, tc.errorCheckFunc), err,
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
	_, _ = createPodsForTesting(t, s)

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
			c := mkapi.MatchCriteria{Namespace: tc.namespace, LabelSelector: tc.labelSelector}
			objList, err := s.List(c)
			if err != nil {
				testutil.AssertError(t, err, tc.retErr)
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

func TestBuildPendingWatchEvents(t *testing.T) {
	s := createStoreForTesting(typeinfo.PodsDescriptor)
	_, _ = createPodsForTesting(t, s)

	tests := map[string]struct {
		namespace               string
		labelSelector           labels.Selector
		startVersion            int64
		retErr                  error
		expectedNumberOfObjects int
	}{
		"base": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.SelectorFromSet(labels.Set{"k0": "v0"}),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 3,
		},
		"high resource version": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            1000,
			expectedNumberOfObjects: 0,
		},
		"resource version that doesn't match all objects": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            2,
			expectedNumberOfObjects: 1,
		},
		"negative resource version": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            -999,
			expectedNumberOfObjects: 3,
		},
		"labels that don't match all objects": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.SelectorFromSet(labels.Set{"k1": "v1"}),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 2,
		},
		"empty labelSelector": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 3,
		},
		"non-matching namespace": {
			namespace:               "abcd",
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			watchEvents, err := s.buildPendingWatchEvents(tc.startVersion, tc.namespace, tc.labelSelector)
			if err != nil {
				testutil.AssertError(t, err, tc.retErr)
			}
			if len(watchEvents) != tc.expectedNumberOfObjects {
				t.Errorf("Expected returned number of objects to be %d, got %d",
					tc.expectedNumberOfObjects,
					len(watchEvents),
				)
			}
			for _, w := range watchEvents {
				if w.Type != watch.Added {
					t.Errorf("Expected watch event type to be ADDED, got: %s", w.Type)
				}
				obj := w.Object.(metav1.Object)
				t.Logf("Event type: %s, object name: %s, resourceVersion: %s", w.Type, obj.GetName(), obj.GetResourceVersion())
			}
		})
	}
}

func TestWatch(t *testing.T) {
	tests := map[string]struct {
		namespace               string
		labelSelector           labels.Selector
		startVersion            int64
		retErr                  error
		expectedNumberOfObjects int
		modifyObjectAfterWatch  bool
	}{
		"base": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.SelectorFromSet(labels.Set{"k0": "v0"}),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 3,
		},
		"high resource version": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            1000,
			expectedNumberOfObjects: 0,
		},
		"resource version that doesn't match all objects": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            2,
			expectedNumberOfObjects: 1,
		},
		"labels that don't match all objects": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.SelectorFromSet(labels.Set{"k1": "v1"}),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 2,
		},
		"empty labelSelector": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 3,
		},
		"non-matching namespace": {
			namespace:               "abcd",
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 0,
		},
		"modify object after watch starts": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 4,
			modifyObjectAfterWatch:  true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })

			createdPods, _ := createPodsForTesting(t, s)
			var (
				receivedEvents []watch.Event
				eventsMutex    sync.Mutex
				wg             sync.WaitGroup
				watchErr       error
			)

			eventCallback := func(event watch.Event) error {
				evt, err := AsMeta(event.Object)
				if err != nil {
					return err
				}
				t.Logf("Received event: %s for %s with resourceVersion %s",
					event.Type, evt.GetName(), evt.GetResourceVersion(),
				)

				eventsMutex.Lock()
				receivedEvents = append(receivedEvents, event)
				eventsMutex.Unlock()
				return nil
			}

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			time.Sleep(100 * time.Millisecond)

			wg.Add(1)
			go func() {
				defer wg.Done()
				watchErr = s.Watch(ctx, tc.startVersion, tc.namespace, tc.labelSelector, eventCallback)
			}()

			if tc.modifyObjectAfterWatch {
				time.Sleep(100 * time.Millisecond)
				modifiedPod := createdPods[0].DeepCopy()
				modifiedPod.Labels["modified"] = "true"
				if err := s.Update(metav1.Object(modifiedPod)); err != nil {
					t.Errorf("Error updating object in store: %v", err)
					cancel()
					wg.Wait()
					return
				}
			}

			time.Sleep(100 * time.Millisecond)

			cancel()
			wg.Wait()

			if watchErr != nil && watchErr != context.Canceled {
				testutil.AssertError(t, watchErr, tc.retErr)
			}

			eventsMutex.Lock()
			count := len(receivedEvents)
			eventsMutex.Unlock()

			if count != tc.expectedNumberOfObjects {
				t.Errorf("Expected returned number of objects to be %d, got %d",
					tc.expectedNumberOfObjects,
					count,
				)
			}
		})
	}
}

func createStoreForTesting(d typeinfo.Descriptor) *InMemResourceStore {
	queueSize := 100
	watchTimeout := 2 * time.Second
	log := klog.NewKlogr().V(4)

	return NewInMemResourceStore(log, &mkapi.ResourceStoreArgs{
		Name:          d.GVR.Resource,
		ObjectGVK:     d.GVK,
		ObjectListGVK: d.ListGVK,
		Scheme:        typeinfo.SupportedScheme,
		WatchConfig:   mkapi.WatchConfig{QueueSize: queueSize, Timeout: watchTimeout},
	})
}

func createPodsForTesting(t *testing.T, s *InMemResourceStore) ([]corev1.Pod, error) {
	t.Helper()
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
			return nil, err
		}
	}
	return createdPods, nil
}

func assertNumberOfItems(t *testing.T, s *InMemResourceStore, want int) {
	t.Helper()
	got := len(s.cache.ListKeys())
	if got != want {
		t.Errorf("Unexpected number of items, got: %v, want: %v", got, want)
	} else {
		t.Logf("Expected number of items, got: %v", got)
	}
}
