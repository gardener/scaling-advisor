package pdbtracker

import (
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ plannerapi.PDBTracker = (*pdbTracker)(nil)

type pdbInfo struct {
	pdb      *policyv1.PodDisruptionBudget
	selector labels.Selector
}

// pdbTracker is the default implementation of PDBTracker interface
type pdbTracker struct {
	pdbInfos []*pdbInfo
}

// New returns a new instance of pdbTracker
func New() *pdbTracker {
	return &pdbTracker{}
}

func (t *pdbTracker) SetPDBs(pdbs []*policyv1.PodDisruptionBudget) error {
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

func (t *pdbTracker) GetPDBs() []*policyv1.PodDisruptionBudget {
	var pdbs []*policyv1.PodDisruptionBudget
	for _, pdbInfo := range t.pdbInfos {
		pdbs = append(pdbs, pdbInfo.pdb)
	}
	return pdbs
}

func (t *pdbTracker) CanRemovePods(pods []*corev1.Pod) (bool, string) {
	for _, pdbInfo := range t.pdbInfos {
		count := int32(0)
		for _, pod := range pods {
			if pod.Namespace == pdbInfo.pdb.Namespace && pdbInfo.selector.Matches(labels.Set(pod.Labels)) {
				count += 1
				if pdbInfo.pdb.Status.DisruptionsAllowed < count {
					return false, pod.Name
				}
			}
		}
	}
	return true, ""
}

func (t *pdbTracker) Clear() {
	t.pdbInfos = nil
}
