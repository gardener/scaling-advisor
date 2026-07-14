// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

var testPod = corev1.Pod{
	ObjectMeta: metav1.ObjectMeta{
		Name:            "bingo",
		Namespace:       metav1.NamespaceDefault,
		ResourceVersion: "2",
	},
}

func TestReset(t *testing.T) {
	tests := map[string]struct {
		tombstoneBeforeReset    bool
		expectedNumberOfObjects int
	}{
		"reset clears live objects": {
			expectedNumberOfObjects: 0,
		},
		"reset clears tombstones": {
			tombstoneBeforeReset:    true,
			expectedNumberOfObjects: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })
			_, _ = createPodsForTesting(t, s, mkapi.ObjectOptions{})

			if tc.tombstoneBeforeReset {
				p := testPod.DeepCopy()
				p.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
				if err := s.Add(t.Context(), p, mkapi.ObjectOptions{}); err != nil {
					t.Fatalf("setup: failed to add: %v", err)
				}
				if err := s.Delete(t.Context(), cache.NewObjectName(testPod.Namespace, testPod.Name), mkapi.ObjectOptions{MarkAsDeleted: true}); err != nil {
					t.Fatalf("setup: failed to tombstone: %v", err)
				}
			}

			if err := s.Reset(); err != nil {
				t.Fatalf("Reset failed: %v", err)
			}
			assertNumberOfItems(t, s, tc.expectedNumberOfObjects)

			_, err := s.GetByKey(t.Context(), cache.NewObjectName(testPod.Namespace, testPod.Name).String())
			if !apierrors.IsNotFound(err) {
				t.Errorf("expected NotFound after Reset, got: %v", err)
			}
		})
	}
}

func TestGetObjAndListGVK(t *testing.T) {
	s := createStoreForTesting(typeinfo.PodsDescriptor)
	t.Cleanup(func() { s.Close() })

	objGVK, listGVK := s.GetObjAndListGVK()
	if objGVK != typeinfo.PodsDescriptor.GVK {
		t.Errorf("expected objGVK %v, got %v", typeinfo.PodsDescriptor.GVK, objGVK)
	}
	if listGVK != typeinfo.PodsDescriptor.ListGVK {
		t.Errorf("expected listGVK %v, got %v", typeinfo.PodsDescriptor.ListGVK, listGVK)
	}
}

func TestAdd(t *testing.T) {
	tests := map[string]struct {
		ignoredFieldsForOutputComparison cmp.Option
		retErr                           error
		typeMeta                         metav1.TypeMeta
		opts                             mkapi.ObjectOptions
		tombstoneBeforeAdd               bool
		expectedNumberOfObjects          int
	}{
		"correct typeMeta": {
			typeMeta:                         metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
			retErr:                           nil,
			expectedNumberOfObjects:          1,
			opts:                             mkapi.ObjectOptions{},
		},
		"missing version in typeMeta": {
			typeMeta:                metav1.TypeMeta{Kind: "Pod"},
			retErr:                  fmt.Errorf("does not match expected objGVK"),
			expectedNumberOfObjects: 0,
			opts:                    mkapi.ObjectOptions{},
		},
		"resurrect tombstoned name": {
			typeMeta:                         metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
			retErr:                           nil,
			expectedNumberOfObjects:          1,
			opts:                             mkapi.ObjectOptions{},
			tombstoneBeforeAdd:               true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p := testPod.DeepCopy()
			p.TypeMeta = tc.typeMeta
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })

			if tc.tombstoneBeforeAdd {
				initial := testPod.DeepCopy()
				initial.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
				if err := s.Add(t.Context(), initial, mkapi.ObjectOptions{}); err != nil {
					t.Fatalf("setup: failed to add pod before tombstone: %v", err)
				}
				if err := s.Delete(t.Context(), cache.NewObjectName(testPod.Namespace, testPod.Name), mkapi.ObjectOptions{MarkAsDeleted: true}); err != nil {
					t.Fatalf("setup: failed to tombstone pod: %v", err)
				}
			}

			obj1 := metav1.Object(p.DeepCopy())
			if err := s.Add(t.Context(), obj1, tc.opts); err != nil {
				assertNumberOfItems(t, s, tc.expectedNumberOfObjects)
				testutil.AssertError(t, err, tc.retErr)
				return
			}
			assertNumberOfItems(t, s, tc.expectedNumberOfObjects)

			key := cache.NewObjectName(p.Namespace, p.Name).String()
			gotObj, err := s.GetByKey(t.Context(), key)
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

func TestGetByKey(t *testing.T) {
	tests := map[string]struct {
		errorCheckFunc            func(error) bool
		key                       string
		opts                      mkapi.ObjectOptions
		tombstoneBeforeFetch      bool
		objectFound               bool
		createObjectBeforeTesting bool
	}{
		"fetch existing object": {
			key:                       fmt.Sprintf("%s/%s", testPod.Namespace, testPod.Name),
			objectFound:               true,
			createObjectBeforeTesting: true,
			opts:                      mkapi.ObjectOptions{},
		},
		"fetch non-existent object": {
			key:                       fmt.Sprintf("%s/%s", testPod.Namespace, testPod.Name),
			objectFound:               false,
			createObjectBeforeTesting: false,
			errorCheckFunc:            apierrors.IsNotFound,
			opts:                      mkapi.ObjectOptions{},
		},
		"fetch object with wrong key": {
			key:                       fmt.Sprintf("%s/%s", testPod.Namespace, "abcd"),
			objectFound:               false,
			createObjectBeforeTesting: true,
			errorCheckFunc:            apierrors.IsNotFound,
			opts:                      mkapi.ObjectOptions{},
		},
		"fetch tombstoned object returns ErrObjectDeleted": {
			key:                       fmt.Sprintf("%s/%s", testPod.Namespace, testPod.Name),
			objectFound:               false,
			createObjectBeforeTesting: true,
			tombstoneBeforeFetch:      true,
			errorCheckFunc:            func(err error) bool { return errors.Is(err, mkapi.ErrObjectDeleted) },
			opts:                      mkapi.ObjectOptions{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })

			createdPod := testPod.DeepCopy()
			if tc.createObjectBeforeTesting {
				createdPod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
				if err := s.Add(t.Context(), metav1.Object(createdPod), tc.opts); err != nil {
					t.Errorf("Error adding object to store")
					return
				}
			}

			if tc.tombstoneBeforeFetch {
				if err := s.Delete(t.Context(), cache.NewObjectName(testPod.Namespace, testPod.Name), mkapi.ObjectOptions{MarkAsDeleted: true}); err != nil {
					t.Fatalf("setup: failed to tombstone pod: %v", err)
				}
			}

			_, err := s.GetByKey(t.Context(), tc.key)
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

func TestGet(t *testing.T) {
	tests := map[string]struct {
		errorCheckFunc            func(error) bool
		objName                   cache.ObjectName
		createObjectBeforeTesting bool
		tombstoneBeforeFetch      bool
	}{
		"fetch existing object": {
			objName:                   cache.NewObjectName(testPod.Namespace, testPod.Name),
			createObjectBeforeTesting: true,
		},
		"fetch non-existent object": {
			objName:        cache.NewObjectName(testPod.Namespace, testPod.Name),
			errorCheckFunc: apierrors.IsNotFound,
		},
		"fetch tombstoned object returns ErrObjectDeleted": {
			objName:                   cache.NewObjectName(testPod.Namespace, testPod.Name),
			createObjectBeforeTesting: true,
			tombstoneBeforeFetch:      true,
			errorCheckFunc:            func(err error) bool { return errors.Is(err, mkapi.ErrObjectDeleted) },
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })

			if tc.createObjectBeforeTesting {
				p := testPod.DeepCopy()
				p.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
				if err := s.Add(t.Context(), p, mkapi.ObjectOptions{}); err != nil {
					t.Fatalf("failed to add pod: %v", err)
				}
			}
			if tc.tombstoneBeforeFetch {
				if err := s.Delete(t.Context(), tc.objName, mkapi.ObjectOptions{MarkAsDeleted: true}); err != nil {
					t.Fatalf("setup: failed to tombstone: %v", err)
				}
			}

			_, err := s.Get(t.Context(), tc.objName)
			if tc.errorCheckFunc != nil {
				if !tc.errorCheckFunc(err) {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	tests := map[string]struct {
		ignoredFieldsForOutputComparison cmp.Option
		retErr                           error
		typeMeta                         metav1.TypeMeta
		name                             string
		opts                             mkapi.ObjectOptions
		tombstoneBeforeUpdate            bool
		expectedNumberOfObjects          int
	}{
		"correct typeMeta": {
			typeMeta:                         metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ignoredFieldsForOutputComparison: cmpopts.IgnoreFields(corev1.Pod{}, "ResourceVersion"),
			retErr:                           nil,
			expectedNumberOfObjects:          1,
			opts:                             mkapi.ObjectOptions{},
		},
		"missing version in typeMeta": {
			typeMeta:                metav1.TypeMeta{Kind: "Pod"},
			retErr:                  fmt.Errorf("does not match expected objGVK"),
			expectedNumberOfObjects: 1,
			opts:                    mkapi.ObjectOptions{},
		},
		"update tombstoned object returns ErrObjectDeleted": {
			typeMeta:                metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			retErr:                  fmt.Errorf("object was deleted"),
			expectedNumberOfObjects: 0,
			opts:                    mkapi.ObjectOptions{},
			tombstoneBeforeUpdate:   true,
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
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })

			createdPod := testPod.DeepCopy()
			createdPod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
			if err := s.Add(t.Context(), metav1.Object(createdPod), tc.opts); err != nil {
				t.Errorf("Error adding object to store")
				return
			}

			if tc.tombstoneBeforeUpdate {
				if err := s.Delete(t.Context(), cache.NewObjectName(testPod.Namespace, testPod.Name), mkapi.ObjectOptions{MarkAsDeleted: true}); err != nil {
					t.Fatalf("setup: failed to tombstone pod: %v", err)
				}
			}

			p := testPod.DeepCopy()
			p.TypeMeta = tc.typeMeta
			if tc.name != "" {
				p.Name = tc.name
			}
			obj1 := metav1.Object(p.DeepCopy())
			if err := s.Update(t.Context(), obj1, tc.opts); err != nil {
				assertNumberOfItems(t, s, tc.expectedNumberOfObjects)
				testutil.AssertError(t, err, tc.retErr)
				return
			}
			assertNumberOfItems(t, s, tc.expectedNumberOfObjects)

			key := cache.NewObjectName(p.Namespace, p.Name).String()
			gotObj, err := s.GetByKey(t.Context(), key)
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
		retErr                    error
		name                      string
		opts                      mkapi.ObjectOptions
		tombstoneBeforeDelete     bool
		createObjectBeforeTesting bool
		expectedNumberOfObjects   int
	}{
		"correct deletion": {
			name:                      testPod.Name,
			createObjectBeforeTesting: true,
			expectedNumberOfObjects:   0,
			retErr:                    nil,
			opts:                      mkapi.ObjectOptions{},
		},
		"delete non-existent object": {
			name:                      testPod.Name,
			createObjectBeforeTesting: false,
			expectedNumberOfObjects:   0,
			retErr:                    fmt.Errorf("not found"),
			opts:                      mkapi.ObjectOptions{},
		},
		"delete object with wrong key": {
			name:                      "abcd",
			createObjectBeforeTesting: true,
			expectedNumberOfObjects:   1,
			retErr:                    fmt.Errorf("not found"),
			opts:                      mkapi.ObjectOptions{},
		},
		"tombstone live object": {
			name:                      testPod.Name,
			createObjectBeforeTesting: true,
			expectedNumberOfObjects:   0,
			retErr:                    nil,
			opts:                      mkapi.ObjectOptions{MarkAsDeleted: true},
		},
		"tombstone already-tombstoned object returns NotFound": {
			name:                      testPod.Name,
			createObjectBeforeTesting: true,
			tombstoneBeforeDelete:     true,
			expectedNumberOfObjects:   0,
			retErr:                    fmt.Errorf("not found"),
			opts:                      mkapi.ObjectOptions{MarkAsDeleted: true},
		},
		"tombstone never-created object returns NotFound": {
			name:                      testPod.Name,
			createObjectBeforeTesting: false,
			expectedNumberOfObjects:   0,
			retErr:                    fmt.Errorf("not found"),
			opts:                      mkapi.ObjectOptions{MarkAsDeleted: true},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })

			createdPod := testPod.DeepCopy()
			if tc.createObjectBeforeTesting {
				createdPod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
				if err := s.Add(t.Context(), metav1.Object(createdPod), mkapi.ObjectOptions{}); err != nil {
					t.Errorf("Error adding object to store")
					return
				}
			}

			if tc.tombstoneBeforeDelete {
				if err := s.Delete(t.Context(), cache.NewObjectName(createdPod.Namespace, tc.name), mkapi.ObjectOptions{MarkAsDeleted: true}); err != nil {
					t.Fatalf("setup: failed to tombstone pod: %v", err)
				}
			}

			key := cache.NewObjectName(createdPod.Namespace, tc.name).String()
			gotObj, _ := s.GetByKey(t.Context(), key)
			if err := s.Delete(t.Context(), cache.NewObjectName(createdPod.Namespace, tc.name), tc.opts); err != nil {
				assertNumberOfItems(t, s, tc.expectedNumberOfObjects)
				testutil.AssertError(t, err, tc.retErr)
				return
			}
			assertNumberOfItems(t, s, tc.expectedNumberOfObjects)

			if !tc.opts.MarkAsDeleted {
				mo, _ := objutil.AsMeta(gotObj)
				if mo.GetDeletionTimestamp() == nil {
					t.Errorf("Expected deletionTimestamp to be set for object that's successfully deleted, got: %v", mo.GetDeletionTimestamp())
					return
				}
			}
		})
	}
}

func TestDeleteObjects(t *testing.T) {
	tests := map[string]struct {
		labelSelector         labels.Selector
		namespace             string
		opts                  mkapi.ObjectOptions
		expectedDelCount      int
		expectedRemainingLive int
	}{
		"delete all matching namespace": {
			namespace:             testPod.Namespace,
			labelSelector:         labels.NewSelector(),
			opts:                  mkapi.ObjectOptions{},
			expectedDelCount:      3,
			expectedRemainingLive: 0,
		},
		"delete by label filter": {
			namespace:             testPod.Namespace,
			labelSelector:         labels.SelectorFromSet(labels.Set{"k1": "v1"}),
			opts:                  mkapi.ObjectOptions{},
			expectedDelCount:      2,
			expectedRemainingLive: 1,
		},
		"delete with tombstone": {
			namespace:             testPod.Namespace,
			labelSelector:         labels.NewSelector(),
			opts:                  mkapi.ObjectOptions{MarkAsDeleted: true},
			expectedDelCount:      3,
			expectedRemainingLive: 0,
		},
		"delete non-matching namespace yields zero": {
			namespace:             "other",
			labelSelector:         labels.NewSelector(),
			opts:                  mkapi.ObjectOptions{},
			expectedDelCount:      0,
			expectedRemainingLive: 3,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })
			_, _ = createPodsForTesting(t, s, mkapi.ObjectOptions{})

			c := mkapi.MatchCriteria{Namespace: tc.namespace, LabelSelector: tc.labelSelector}
			delCount, err := s.DeleteObjects(t.Context(), c, tc.opts)
			if err != nil {
				t.Fatalf("DeleteObjects failed: %v", err)
			}
			if delCount != tc.expectedDelCount {
				t.Errorf("expected %d deletions, got %d", tc.expectedDelCount, delCount)
			}
			assertNumberOfItems(t, s, tc.expectedRemainingLive)
		})
	}
}

func TestList(t *testing.T) {
	tests := map[string]struct {
		labelSelector           labels.Selector
		retErr                  error
		namespace               string
		tombstoneOneObject      bool
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
		"tombstoned object excluded": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			expectedNumberOfObjects: 2,
			tombstoneOneObject:      true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })
			pods, _ := createPodsForTesting(t, s, mkapi.ObjectOptions{})

			if tc.tombstoneOneObject {
				target := pods[0]
				if err := s.Delete(t.Context(), cache.NewObjectName(target.Namespace, target.Name), mkapi.ObjectOptions{MarkAsDeleted: true}); err != nil {
					t.Fatalf("setup: failed to tombstone pod: %v", err)
				}
			}

			c := mkapi.MatchCriteria{Namespace: tc.namespace, LabelSelector: tc.labelSelector}
			objList, err := s.List(t.Context(), c)
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

func TestListMetaObjects(t *testing.T) {
	tests := map[string]struct {
		labelSelector           labels.Selector
		namespace               string
		tombstoneOneObject      bool
		expectedNumberOfObjects int
	}{
		"base": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			expectedNumberOfObjects: 3,
		},
		"label filter": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.SelectorFromSet(labels.Set{"k1": "v1"}),
			expectedNumberOfObjects: 2,
		},
		"non-matching namespace": {
			namespace:               "other",
			labelSelector:           labels.NewSelector(),
			expectedNumberOfObjects: 0,
		},
		"tombstoned object excluded": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			tombstoneOneObject:      true,
			expectedNumberOfObjects: 2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })
			pods, _ := createPodsForTesting(t, s, mkapi.ObjectOptions{})

			if tc.tombstoneOneObject {
				target := pods[0]
				if err := s.Delete(t.Context(), cache.NewObjectName(target.Namespace, target.Name), mkapi.ObjectOptions{MarkAsDeleted: true}); err != nil {
					t.Fatalf("setup: failed to tombstone: %v", err)
				}
			}

			c := mkapi.MatchCriteria{Namespace: tc.namespace, LabelSelector: tc.labelSelector}
			objs, _, err := s.ListMetaObjects(t.Context(), c)
			if err != nil {
				t.Fatalf("ListMetaObjects failed: %v", err)
			}
			if len(objs) != tc.expectedNumberOfObjects {
				t.Errorf("expected %d objects, got %d", tc.expectedNumberOfObjects, len(objs))
			}
		})
	}
}

func TestListTombstonedKeys(t *testing.T) {
	tests := map[string]struct {
		numTombstones int
	}{
		"no tombstones":    {numTombstones: 0},
		"one tombstone":    {numTombstones: 1},
		"three tombstones": {numTombstones: 3},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })
			pods, _ := createPodsForTesting(t, s, mkapi.ObjectOptions{})

			for i := range tc.numTombstones {
				target := pods[i]
				if err := s.Delete(t.Context(), cache.NewObjectName(target.Namespace, target.Name), mkapi.ObjectOptions{MarkAsDeleted: true}); err != nil {
					t.Fatalf("setup: failed to tombstone pod %d: %v", i, err)
				}
			}

			keys, err := s.ListTombstonedKeys(t.Context())
			if err != nil {
				t.Fatalf("ListTombstonedKeys failed: %v", err)
			}
			if keys.Len() != tc.numTombstones {
				t.Errorf("expected %d tombstoned keys, got %d: %v", tc.numTombstones, keys.Len(), keys)
			}
		})
	}
}

func TestWatch(t *testing.T) {
	tests := map[string]struct {
		labelSelector           labels.Selector
		retErr                  error
		namespace               string
		opts                    mkapi.ObjectOptions
		tombstoneAfterWatch     bool
		modifyObjectAfterWatch  bool
		startVersion            int64
		expectedNumberOfObjects int
	}{
		"base": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.SelectorFromSet(labels.Set{"k0": "v0"}),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 3,
			opts:                    mkapi.ObjectOptions{},
		},
		"high resource version": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            1000,
			expectedNumberOfObjects: 0,
			opts:                    mkapi.ObjectOptions{},
		},
		"resource version that doesn't match all objects": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            2,
			expectedNumberOfObjects: 1,
			opts:                    mkapi.ObjectOptions{},
		},
		"labels that don't match all objects": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.SelectorFromSet(labels.Set{"k1": "v1"}),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 2,
			opts:                    mkapi.ObjectOptions{},
		},
		"empty labelSelector": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 3,
			opts:                    mkapi.ObjectOptions{},
		},
		"non-matching namespace": {
			namespace:               "abcd",
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 0,
			opts:                    mkapi.ObjectOptions{},
		},
		"modify object after watch starts": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 4,
			modifyObjectAfterWatch:  true,
			opts:                    mkapi.ObjectOptions{},
		},
		"tombstone after watch starts delivers Deleted event": {
			namespace:               testPod.Namespace,
			labelSelector:           labels.NewSelector(),
			retErr:                  nil,
			startVersion:            0,
			expectedNumberOfObjects: 4, // 3 Added (initial list) + 1 Deleted (tombstone broadcast)
			tombstoneAfterWatch:     true,
			opts:                    mkapi.ObjectOptions{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })

			createdPods, _ := createPodsForTesting(t, s, tc.opts)
			var (
				receivedEvents []watch.Event
				eventsMutex    sync.Mutex
				wg             sync.WaitGroup
				watchErr       error
			)

			eventCallback := func(event watch.Event) error {
				evt, err := objutil.AsMeta(event.Object)
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
				if err := s.Update(t.Context(), metav1.Object(modifiedPod), tc.opts); err != nil {
					t.Errorf("Error updating object in store: %v", err)
					cancel()
					wg.Wait()
					return
				}
			}

			if tc.tombstoneAfterWatch {
				time.Sleep(100 * time.Millisecond)
				target := createdPods[0]
				if err := s.Delete(t.Context(), cache.NewObjectName(target.Namespace, target.Name), mkapi.ObjectOptions{MarkAsDeleted: true}); err != nil {
					t.Errorf("Error tombstoning object in store: %v", err)
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

func TestGetWatcher(t *testing.T) {
	tests := map[string]struct {
		modifyAfterWatch  bool
		expectedMinEvents int
	}{
		"receives initial list as Added events": {
			expectedMinEvents: 3,
		},
		"receives Modified event after update": {
			modifyAfterWatch:  true,
			expectedMinEvents: 4, // 3 Added + 1 Modified
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })
			createdPods, _ := createPodsForTesting(t, s, mkapi.ObjectOptions{})

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			w, err := s.GetWatcher(ctx, testPod.Namespace, metav1.ListOptions{})
			if err != nil {
				t.Fatalf("GetWatcher failed: %v", err)
			}
			t.Cleanup(w.Stop)

			if tc.modifyAfterWatch {
				time.Sleep(50 * time.Millisecond)
				mod := createdPods[0].DeepCopy()
				mod.Labels["modified"] = "true"
				if err = s.Update(t.Context(), metav1.Object(mod), mkapi.ObjectOptions{}); err != nil {
					t.Fatalf("Update failed: %v", err)
				}
			}

			var received []watch.Event
			timeout := time.After(500 * time.Millisecond)
		drain:
			for {
				select {
				case ev, ok := <-w.ResultChan():
					if !ok {
						break drain
					}
					received = append(received, ev)
				case <-timeout:
					break drain
				}
			}
			if len(received) < tc.expectedMinEvents {
				t.Errorf("expected at least %d events, got %d", tc.expectedMinEvents, len(received))
			}
		})
	}
}

func TestGetVersionCounter(t *testing.T) {
	tests := map[string]struct {
		mutate func(t *testing.T, s *InMemResourceStore, p *corev1.Pod)
	}{
		"increments after Add": {
			mutate: func(t *testing.T, s *InMemResourceStore, p *corev1.Pod) {
				if err := s.Add(t.Context(), p, mkapi.ObjectOptions{}); err != nil {
					t.Fatalf("Add failed: %v", err)
				}
			},
		},
		"increments after Update": {
			mutate: func(t *testing.T, s *InMemResourceStore, p *corev1.Pod) {
				if err := s.Add(t.Context(), p, mkapi.ObjectOptions{}); err != nil {
					t.Fatalf("Add failed: %v", err)
				}
				before := s.GetVersionCounter().Load()
				p.Labels = map[string]string{"updated": "true"}
				if err := s.Update(t.Context(), p, mkapi.ObjectOptions{}); err != nil {
					t.Fatalf("Update failed: %v", err)
				}
				if s.GetVersionCounter().Load() <= before {
					t.Errorf("expected counter to increment after Update, got %d (was %d)",
						s.GetVersionCounter().Load(), before)
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := createStoreForTesting(typeinfo.PodsDescriptor)
			t.Cleanup(func() { s.Close() })

			counter := s.GetVersionCounter()
			if counter == nil {
				t.Fatal("expected non-nil version counter")
			}
			before := counter.Load()

			p := testPod.DeepCopy()
			p.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
			tc.mutate(t, s, p)

			if counter.Load() <= before {
				t.Errorf("expected counter to increment, got %d (was %d)", counter.Load(), before)
			}
		})
	}
}

func createStoreForTesting(d typeinfo.Descriptor) *InMemResourceStore {
	queueSize := 100
	watchTimeout := 2 * time.Second

	return NewInMemResourceStore(&mkapi.ResourceStoreArgs{
		Name:          d.GVR.Resource,
		ObjectGVK:     d.GVK,
		ObjectListGVK: d.ListGVK,
		Scheme:        typeinfo.SupportedScheme,
		WatchConfig:   mkapi.WatchConfig{QueueSize: queueSize, Timeout: watchTimeout},
	})
}

func createPodsForTesting(t *testing.T, s *InMemResourceStore, opts mkapi.ObjectOptions) ([]corev1.Pod, error) {
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

		if err := s.Add(t.Context(), metav1.Object(&createdPods[i]), opts); err != nil {
			t.Errorf("Error adding object to store")
			return nil, err
		}
	}
	return createdPods, nil
}

func assertNumberOfItems(t *testing.T, s *InMemResourceStore, want int) {
	t.Helper()
	got := len(filterOutTombstones(s.cache.List()))
	if got != want {
		t.Errorf("Unexpected number of items, got: %v, want: %v", got, want)
	} else {
		t.Logf("Expected number of items, got: %v", got)
	}
}
