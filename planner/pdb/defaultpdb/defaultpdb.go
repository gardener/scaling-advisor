package pdb

import (
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type pdbInfo struct {
	pdb      *policyv1.PodDisruptionBudget
	selector labels.Selector
}

// defaultRemainingPdbTracker is the default implementation of RemainingPdbTracker
type defaultPdbTracker struct {
	pdbInfos []*pdbInfo
}

// NewDefaultRemainingPdbTracker returns a new instance of defaultRemainingPdbTracker
func NewDefaultRemainingPdbTracker() *defaultPdbTracker {
	return &defaultPdbTracker{}
}

func (t *defaultPdbTracker) SetPdbs(pdbs []*policyv1.PodDisruptionBudget) error {
	t.Clear()
	for _, pdb := range pdbs {
		pdbCopy := pdb.DeepCopy()
		selector, err := metav1.LabelSelectorAsSelector(pdbCopy.Spec.Selector)
		if err != nil {
			return err
		}
		t.pdbInfos = append(t.pdbInfos, &pdbInfo{
			pdb:      pdbCopy,
			selector: selector,
		})
	}
	return nil
}

func (t *defaultPdbTracker) GetPdbs() []*policyv1.PodDisruptionBudget {
	var pdbs []*policyv1.PodDisruptionBudget
	for _, pdbInfo := range t.pdbInfos {
		pdbs = append(pdbs, pdbInfo.pdb)
	}
	return pdbs
}

func (t *defaultPdbTracker) MatchingPdbs(pod *corev1.Pod) []*policyv1.PodDisruptionBudget {
	var pdbs []*policyv1.PodDisruptionBudget
	for _, pdbInfo := range t.pdbInfos {
		if pod.Namespace == pdbInfo.pdb.Namespace && pdbInfo.selector.Matches(labels.Set(pod.Labels)) {
			pdbs = append(pdbs, pdbInfo.pdb)
		}
	}
	return pdbs
}

func (t *defaultPdbTracker) CanRemovePods(pods []*corev1.Pod) (canRemove bool, blockingPodName string) {
	for _, pdbInfo := range t.pdbInfos {
		count := int32(0)
		for _, pod := range pods {
			if pod.Namespace == pdbInfo.pdb.Namespace && pdbInfo.selector.Matches(labels.Set(pod.Labels)) {
				count += 1
				if pdbInfo.pdb.Status.DisruptionsAllowed < count {
					//TODO: Just log the podname rather than returning here.
					return false, pod.Name
				}
			}
		}
	}
	return true, blockingPodName
}

func (t *defaultPdbTracker) RemovePods(pods []*corev1.Pod) {
	for _, pdbInfo := range t.pdbInfos {
		for _, pod := range pods {
			if pod.Namespace == pdbInfo.pdb.Namespace && pdbInfo.selector.Matches(labels.Set(pod.Labels)) {
				pdbInfo.pdb.Status.DisruptionsAllowed -= 1
			}
		}
	}
}

func (t *defaultPdbTracker) Clear() {
	t.pdbInfos = nil
}
