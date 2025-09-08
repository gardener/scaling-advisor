// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package minkapi

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestMatchCriteria(t *testing.T) {
	testPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bingo",
			Namespace: "default",
			Labels:    map[string]string{"k1": "v1", "k2": "v2"},
		},
	}

	tests := map[string]struct {
		criteria MatchCriteria
		matches  bool
	}{
		"not matching name": {criteria: MatchCriteria{Names: sets.New("abcd")}, matches: false},
		"matching name":     {criteria: MatchCriteria{Names: sets.New("bingo")}, matches: true},
		"matching name and namespace": {
			criteria: MatchCriteria{Names: sets.New("bingo"), Namespace: "default"},
			matches:  true,
		},
		"matching name but different namespace": {
			criteria: MatchCriteria{Names: sets.New("bingo"), Namespace: "test"},
			matches:  false,
		},
		"matching namespace and label": {
			criteria: MatchCriteria{Namespace: "default", LabelSelector: labels.SelectorFromSet(map[string]string{"k1": "v1"})},
			matches:  true,
		},
		"matching namespace but not label": {
			criteria: MatchCriteria{Namespace: "default", LabelSelector: labels.SelectorFromSet(map[string]string{"k1": "v2"})},
			matches:  false,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tc.criteria.Matches(&testPod); got != tc.matches {
				t.Errorf("Expected %#v to match for criteria %#v", testPod.ObjectMeta, tc.criteria)
			}
		})
	}
}
