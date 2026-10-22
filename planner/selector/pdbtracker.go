// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package selector

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// pdbInfo pairs a compiled label selector with its source PDB and the remaining disruptions budget.
type pdbInfo struct {
	selector        labels.Selector
	pdb             policyv1.PodDisruptionBudget
	disruptionsLeft int32
}

// pdbTracker tracks remaining PDB disruption budgets and allows callers to check and consume
// disruption allowances as scale-in candidates are accepted one by one.
type pdbTracker struct {
	infos []pdbInfo
}

// newPDBTracker returns a new pdbTracker pre-loaded with the given PDBs.
// Returns an error if any PDB has a selector that cannot be compiled.
func newPDBTracker(pdbs []policyv1.PodDisruptionBudget) (*pdbTracker, error) {
	t := &pdbTracker{infos: make([]pdbInfo, 0, len(pdbs))}
	for _, pdb := range pdbs {
		sel, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid selector for PDB %s/%s: %w", pdb.Namespace, pdb.Name, err)
		}
		t.infos = append(t.infos, pdbInfo{
			selector:        sel,
			pdb:             pdb,
			disruptionsLeft: pdb.Status.DisruptionsAllowed,
		})
	}
	return t, nil
}

// CanRemovePods returns true if removing all given pods would not violate any tracked PDB.
// When false, it also returns the name of the pod that would exceed the budget.
//
// Pods are matched against each PDB by namespace and label selector. The check is non-destructive:
// it does not modify the tracker state. Use ConsumePods to actually deduct the budget.
func (t *pdbTracker) CanRemovePods(pods []corev1.Pod) (ok bool, blockingPod string) {
	for i := range t.infos {
		info := &t.infos[i]
		remaining := info.disruptionsLeft
		for _, pod := range pods {
			if pod.Namespace != info.pdb.Namespace {
				continue
			}
			if !info.selector.Matches(labels.Set(pod.Labels)) {
				continue
			}
			remaining--
			if remaining < 0 {
				return false, pod.Name
			}
		}
	}
	return true, ""
}

// ConsumePods deducts the disruption budget consumed by removing the given pods.
// Must only be called after CanRemovePods returns true for the same pod set.
func (t *pdbTracker) ConsumePods(pods []corev1.Pod) {
	for i := range t.infos {
		info := &t.infos[i]
		for _, pod := range pods {
			if pod.Namespace != info.pdb.Namespace {
				continue
			}
			if info.selector.Matches(labels.Set(pod.Labels)) {
				info.disruptionsLeft--
			}
		}
	}
}
