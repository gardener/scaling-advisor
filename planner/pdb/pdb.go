package pdb

import (
	"github.com/gardener/scaling-advisor/common/drainutil"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
)

// RemainingPdbTracker is responsible for tracking the remaining PDBs
type RemainingPdbTracker interface {
	// SetPdbs sets PDBs of the remaining PDB tracker.
	SetPdbs(pdbs []*policyv1.PodDisruptionBudget) error
	// GetPdbs returns the current remaining PDBs.
	GetPdbs() []*policyv1.PodDisruptionBudget
	// MatchingPdbs returns all PDBs matching the pod.
	MatchingPdbs(pod *corev1.Pod) []*policyv1.PodDisruptionBudget

	// CanRemovePods checks if the set of pods can be removed.
	// inParallel indicates if the pods can be removed in parallel. If it is false
	// then evicting pods could fail due to drain timeout.
	CanRemovePods(pods []*corev1.Pod) (canRemove bool, drainBlockingPod *drainutil.DrainBlockingPod)
	// RemovePods updates the remaining PDBs after pod removal.
	RemovePods(pods []*corev1.Pod)

	// Clear resets the remaining PDB tracker to empty state.
	Clear()
}
