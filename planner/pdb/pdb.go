package pdb

import (
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
)

// RemainingPdbTracker is responsible for tracking the remaining PDBs
// TODO: Change the name from `RemainingPdbTracker` to `PdbTracker`
type PdbTracker interface {
	// SetPdbs sets PDBs of the remaining PDB tracker.
	SetPdbs(pdbs []*policyv1.PodDisruptionBudget) error
	// GetPdbs returns the current remaining PDBs.
	GetPdbs() []*policyv1.PodDisruptionBudget
	// MatchingPdbs returns all PDBs matching the pod.
	MatchingPdbs(pod *corev1.Pod) []*policyv1.PodDisruptionBudget

	// CanRemovePods checks if the set of pods can be removed.
	CanRemovePods(pods []*corev1.Pod) (canRemove bool, blockingPodName string)
	// RemovePods updates the remaining PDBs after pod removal.
	RemovePods(pods []*corev1.Pod)

	// Clear resets the remaining PDB tracker to empty state.
	Clear()
}
