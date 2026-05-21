package pdbtracker

import (
	"fmt"
	"testing"

	apiv1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

var (
	one    = intstr.FromInt(1)
	label1 = "label-1"
	label2 = "label-2"
	pdb1   = &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "ns",
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					label1: "true",
				},
			},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: 1,
		},
	}
	pdb2 = &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "ns",
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					label2: "true",
				},
			},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: 2,
		},
	}
)

func TestBasicCanRemovePods(t *testing.T) {
	testCases := []struct {
		name            string
		podsLabel1      int
		podsLabel2      int
		podsBothLabels  int
		pdbs            []*policyv1.PodDisruptionBudget
		pdbsDisruptions [2]int32
		canDisrupt      bool
	}{
		{
			name:       "No pdbs",
			podsLabel1: 2,
			podsLabel2: 1,
			canDisrupt: true,
		},
		{
			name:            "Not enough pod disruption budgets",
			podsLabel1:      2,
			podsLabel2:      1,
			pdbs:            []*policyv1.PodDisruptionBudget{pdb1, pdb2},
			pdbsDisruptions: [2]int32{1, 0},
			canDisrupt:      false,
		},
		{
			name:            "Enough pod disruption budgets",
			podsLabel1:      2,
			podsLabel2:      3,
			pdbs:            []*policyv1.PodDisruptionBudget{pdb1, pdb2},
			pdbsDisruptions: [2]int32{2, 4},
			canDisrupt:      true,
		},
		{
			name:            "Pod covered with both PDBs can be moved",
			podsLabel1:      1,
			podsLabel2:      1,
			podsBothLabels:  1,
			pdbs:            []*policyv1.PodDisruptionBudget{pdb1, pdb2},
			pdbsDisruptions: [2]int32{1, 1},
			canDisrupt:      true,
		},
	}
	for _, test := range testCases {
		pdb1.Status.DisruptionsAllowed = test.pdbsDisruptions[0]
		pdb2.Status.DisruptionsAllowed = test.pdbsDisruptions[1]
		tracker := New()
		if err := tracker.SetPDBs(test.pdbs); err != nil {
			t.Errorf("SetPdbs failed: %v", err)
		}
		pods := makePodsWithLabel(label1, test.podsLabel1)
		pods2 := makePodsWithLabel(label2, test.podsLabel2-test.podsBothLabels)
		if test.podsBothLabels > 0 {
			addLabelToPods(pods[:test.podsBothLabels], label2)
		}
		pods = append(pods, pods2...)
		gotDisrupt, _ := tracker.CanRemovePods(pods)
		if gotDisrupt != test.canDisrupt {
			t.Errorf("%s: CanDisrupt() return %v, want %v", test.name, gotDisrupt, test.canDisrupt)
		}
	}
}

func makePodsWithLabel(label string, amount int) []*apiv1.Pod {
	pods := []*apiv1.Pod{}
	for i := 0; i < amount; i++ {
		pod := &apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("pod-1-%d", i),
				Namespace:       "ns",
				OwnerReferences: GenerateOwnerReferences("rs", "ReplicaSet", "extensions/v1beta1", ""),
				Labels: map[string]string{
					label: "true",
				},
			},
			Spec: apiv1.PodSpec{},
		}
		pods = append(pods, pod)
	}
	return pods
}

func addLabelToPods(pods []*apiv1.Pod, label string) {
	for _, pod := range pods {
		pod.ObjectMeta.Labels[label] = "true"
	}
}

// GenerateOwnerReferences builds OwnerReferences with a single reference
func GenerateOwnerReferences(name, kind, api string, uid types.UID) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         api,
			Kind:               kind,
			Name:               name,
			BlockOwnerDeletion: ptr.To(true),
			Controller:         ptr.To(true),
			UID:                uid,
		},
	}
}
