// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"testing"

	"github.com/gardener/scaling-advisor/minkapi/core/typeinfo"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

//func TestCreateList(t *testing.T) {
//	log := klog.NewKlogr()
//	descriptors := []typeinfo.Descriptor{
//		typeinfo.PodsDescriptor,
//	}
//	objLists := [][]runtime.Object{
//		{
//			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pa"}},
//			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pb"}},
//		},
//	}
//	for i := 0; i < len(descriptors); i++ {
//		d := descriptors[i]
//		ol := objLists[i]
//		listObj, err := createList(log, d, "", "v1", ol, labels.Everything())
//		if err != nil {
//			t.Errorf("Failed to create list: %v", err)
//		}
//		t.Logf("Created list object using %q: %v", d.ListGVK, listObj)
//	}
//}

func TestPatchPodStatus(t *testing.T) {
	data := readFile(t, "testdata/pod-a.json")
	if data == nil {
		return
	}
	obj, err := typeinfo.PodsDescriptor.CreateObject()
	if err != nil {
		t.Errorf("Failed to create pod: %v", err)
		return
	}
	pod := obj.(*corev1.Pod)
	err = patchStatus(obj.(runtime.Object), "default/bingo", []byte(patchPodCond))
	if err != nil {
		t.Errorf("Failed to patch pod: %v", err)
		return
	}
	t.Logf("Patched pod status: %v", pod)
	if pod.Status.Conditions == nil {
		t.Errorf("Failed to set pod conditions")
	}
}

func TestPatchEvent(t *testing.T) {
	data := readFile(t, "testdata/event-a.json")
	if data == nil {
		return
	}
	obj, err := typeinfo.EventsDescriptor.CreateObject()
	if err != nil {
		t.Errorf("Failed to create event: %v", err)
		return
	}
	event := obj.(*eventsv1.Event)
	err = patchObject(obj.(runtime.Object), "default/a-bingo.aaabbb", "application/strategic-merge-patch+json", []byte(patchEventSeries))
	if err != nil {
		t.Errorf("Failed to patch evnt: %v", err)
		return
	}
	t.Logf("Patched event series: %v", event)
	if event.Series == nil {
		t.Errorf("Failed to patch event series")
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("Failed to read file from path %q: %v", path, err)
		return nil
	}
	return data
}

var patchPodCond = `
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

var patchEventSeries = `
{
  "series": {
    "count": 2,
    "lastObservedTime": "2025-05-08T09:05:57.028064Z"
  }
}
`
