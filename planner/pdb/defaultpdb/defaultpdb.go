package pdb

import (
	"github.com/gardener/scaling-advisor/common/drainutil"
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
type defaultRemainingPdbTracker struct {
	pdbInfos []*pdbInfo
}

// NewDefaultRemainingPdbTracker returns a new instance of defaultRemainingPdbTracker
func NewDefaultRemainingPdbTracker() *defaultRemainingPdbTracker {
	return &defaultRemainingPdbTracker{}
}

func (t *defaultRemainingPdbTracker) SetPdbs(pdbs []*policyv1.PodDisruptionBudget) error {
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

func (t *defaultRemainingPdbTracker) GetPdbs() []*policyv1.PodDisruptionBudget {
	var pdbs []*policyv1.PodDisruptionBudget
	for _, pdbInfo := range t.pdbInfos {
		pdbs = append(pdbs, pdbInfo.pdb)
	}
	return pdbs
}

func (t *defaultRemainingPdbTracker) MatchingPdbs(pod *corev1.Pod) []*policyv1.PodDisruptionBudget {
	var pdbs []*policyv1.PodDisruptionBudget
	for _, pdbInfo := range t.pdbInfos {
		if pod.Namespace == pdbInfo.pdb.Namespace && pdbInfo.selector.Matches(labels.Set(pod.Labels)) {
			pdbs = append(pdbs, pdbInfo.pdb)
		}
	}
	return pdbs
}

func (t *defaultRemainingPdbTracker) CanRemovePods(pods []*corev1.Pod) (canRemove bool, drainBlockingPod *drainutil.DrainBlockingPod) {
	for _, pdbInfo := range t.pdbInfos {
		count := int32(0)
		for _, pod := range pods {
			if pod.Namespace == pdbInfo.pdb.Namespace && pdbInfo.selector.Matches(labels.Set(pod.Labels)) {
				count += 1
				if pdbInfo.pdb.Status.DisruptionsAllowed < count {
					//TODO: Just log the podname rather than returning here.
					return false, &drainutil.DrainBlockingPod{Pod: pod, Reason: drainutil.NotEnoughPdb}
				}
			}
		}
	}
	return true, drainBlockingPod
}

func (t *defaultRemainingPdbTracker) RemovePods(pods []*corev1.Pod) {
	for _, pdbInfo := range t.pdbInfos {
		for _, pod := range pods {
			if pod.Namespace == pdbInfo.pdb.Namespace && pdbInfo.selector.Matches(labels.Set(pod.Labels)) {
				pdbInfo.pdb.Status.DisruptionsAllowed -= 1
			}
		}
	}
}

func (t *defaultRemainingPdbTracker) Clear() {
	t.pdbInfos = nil
}
