// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package podutil

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var transitionTime = metav1.NewTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

var testPodStatus = corev1.PodStatus{
	Phase: corev1.PodRunning,
	Conditions: []corev1.PodCondition{
		{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, LastTransitionTime: transitionTime},
		{Type: corev1.ContainersReady, Status: corev1.ConditionTrue, LastTransitionTime: transitionTime},
		{Type: corev1.PodInitialized, Status: corev1.ConditionTrue, LastTransitionTime: transitionTime},
		{Type: corev1.PodInitialized, Status: corev1.ConditionFalse, LastTransitionTime: transitionTime},
	},
}

func TestGetPodCondition(t *testing.T) {
	tests := map[string]struct {
		podStatus     *corev1.PodStatus
		conditionType corev1.PodConditionType
		index         int
	}{
		"multiple instances of condition present": {
			podStatus:     &testPodStatus,
			conditionType: corev1.PodInitialized,
			index:         2,
		},
		"single instance present": {
			podStatus:     &testPodStatus,
			conditionType: corev1.ContainersReady,
			index:         1,
		},
		"missing condition": {
			podStatus:     &testPodStatus,
			conditionType: corev1.PodReady,
			index:         -1,
		},
		"empty status": {
			podStatus:     &corev1.PodStatus{},
			conditionType: "",
			index:         -1,
		},
		"nil status": {
			podStatus:     nil,
			conditionType: "",
			index:         -1,
		},
		"status with no conditions": {
			podStatus:     &corev1.PodStatus{Phase: corev1.PodPending},
			conditionType: "",
			index:         -1,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotIdx, gotCond := GetPodCondition(tc.podStatus, tc.conditionType)

			if gotIdx != tc.index {
				t.Errorf("Expected condition to be at %d index, got: %d", tc.index, gotIdx)
				return
			}

			if tc.index != -1 && (gotCond != &testPodStatus.Conditions[tc.index] || gotCond.Type != tc.conditionType) {
				t.Errorf("Expected condition to be %v, got: %v", testPodStatus.Conditions[tc.index], gotCond)
				return
			}
		})
	}
}

func TestUpdatePodCondition(t *testing.T) {
	tests := map[string]struct {
		ignoredFieldsForComparison cmp.Option
		podCondition               corev1.PodCondition
		statusChanged              bool
	}{
		"update condition status": {
			podCondition: corev1.PodCondition{
				Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Now(),
			},
			ignoredFieldsForComparison: cmpopts.IgnoreFields(corev1.PodCondition{}, "Status", "LastTransitionTime"),
			statusChanged:              true,
		},
		"add condition": {
			podCondition: corev1.PodCondition{
				Type: corev1.PodResizePending, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Now(),
			},
			ignoredFieldsForComparison: cmpopts.IgnoreFields(corev1.PodCondition{}, "Type", "Status", "LastTransitionTime"),
			statusChanged:              true,
		},
		"empty condition": {
			podCondition:               corev1.PodCondition{},
			ignoredFieldsForComparison: cmpopts.IgnoreFields(corev1.PodCondition{}, "LastTransitionTime"),
			statusChanged:              true,
		},
		"no change in condition": {
			podCondition:  testPodStatus.Conditions[0],
			statusChanged: false,
		},
		"no change in condition status": {
			podCondition: corev1.PodCondition{
				Type: corev1.PodScheduled, Status: corev1.ConditionFalse, LastTransitionTime: metav1.Now(),
			},
			statusChanged: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			podStatus := testPodStatus.DeepCopy()
			conditionChanged := UpdatePodCondition(podStatus, &tc.podCondition)
			// FIXME
			_, gotCond := GetPodCondition(&testPodStatus, tc.podCondition.Type)
			if gotCond == nil {
				gotCond = &corev1.PodCondition{}
			}

			if conditionChanged != tc.statusChanged {
				t.Errorf("Expected pod condition to change")
				t.Logf("Got condition: %#v\n", *gotCond)
				t.Logf("Want condition: %#v\n", tc.podCondition)
			}
			if conditionChanged {
				t.Logf("Pod condition changed")
				if diff := cmp.Diff(tc.podCondition, *gotCond, tc.ignoredFieldsForComparison); diff != "" {
					t.Errorf("Diff:\n%s\n", diff)
				}
			} else {
				t.Logf("No pod condition change as expected")
			}
		})
	}
}
