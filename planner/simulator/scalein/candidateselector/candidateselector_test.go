package scaleincandidateselector

import (
	"context"
	"errors"
	"testing"

	"github.com/gardener/scaling-advisor/planner/testutil"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func TestNextCandidate_NoNodes(t *testing.T) {
	v := testutil.NewTestView(t)
	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, nil)
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}
	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil candidate, got %q", got.Name)
	}
}

func TestNextCandidate_SingleEligibleNode(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))})
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a candidate, got nil")
	}
	if got.Name != "node-a" {
		t.Errorf("expected node-a, got %q", got.Name)
	}
}

func TestNextCandidate_SkipNodesRespected(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})
	testutil.AddNode(t, v, "node-b", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))})
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}
	sel.RemoveCandidateNode("node-a")

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a candidate, got nil")
	}
	if got.Name != "node-b" {
		t.Errorf("expected node-b, got %q", got.Name)
	}
}

func TestNextCandidate_AllNodesSkipped(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))})
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}
	sel.RemoveCandidateNode("node-a")

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil candidate, got %q", got.Name)
	}
}

func TestNextCandidate_PoolMinReached(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 1, 0, testutil.Tmpl("tmpl1", 0))})
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil (pool min reached), got %q", got.Name)
	}
}

func TestNextCandidate_PoolMinNotReached(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})
	testutil.AddNode(t, v, "node-b", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 1, 0, testutil.Tmpl("tmpl1", 0))})
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a candidate, got nil")
	}
}

func TestNextCandidate_ScaleInDisabledAnnotation(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1",
		Annotations: map[string]string{commonconstants.AnnotationScaleInDisabledKey: "true"}})

	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))})
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil (scale-in disabled annotation), got %q", got.Name)
	}
}

func TestNextCandidate_NonEvictablePodBlocksNode(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})
	testutil.AddPod(t, v, "sticky-pod", "default", "node-a",
		testutil.PodOpts{Annotations: map[string]string{commonconstants.AnnotationSafeToEvict: "false"}})

	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))})
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil (non-evictable pod), got %q", got.Name)
	}
}

func TestNextCandidate_HighUtilizationSkipsNode(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	sel := New(testutil.HighUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))})
	args.UtilizationThresholds = map[corev1.ResourceName]float64{corev1.ResourceCPU: 0.5, corev1.ResourceMemory: 0.5}
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil (high utilization), got %q", got.Name)
	}
}

func TestNextCandidate_PicksLowestPriorityPool(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-high", testutil.NodeOpts{Pool: "pool-high", Template: "tmpl1"})
	testutil.AddNode(t, v, "node-low", testutil.NodeOpts{Pool: "pool-low", Template: "tmpl1"})

	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{
		testutil.Pool("pool-high", 0, 10, testutil.Tmpl("tmpl1", 0)),
		testutil.Pool("pool-low", 0, 1, testutil.Tmpl("tmpl1", 0)),
	})
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a candidate, got nil")
	}
	if got.Name != "node-low" {
		t.Errorf("expected node-low (lowest pool priority), got %q", got.Name)
	}
}

func TestNextCandidate_PicksLowestPriorityTemplate(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-tmpl-high", testutil.NodeOpts{Pool: "pool1", Template: "tmpl-high"})
	testutil.AddNode(t, v, "node-tmpl-low", testutil.NodeOpts{Pool: "pool1", Template: "tmpl-low"})

	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{
		testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl-high", 10), testutil.Tmpl("tmpl-low", 1)),
	})
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a candidate, got nil")
	}
	if got.Name != "node-tmpl-low" {
		t.Errorf("expected node-tmpl-low (lowest template priority), got %q", got.Name)
	}
}

func TestNextCandidate_ListNodesError(t *testing.T) {
	listErr := errors.New("node list failed")
	v := &testutil.FailingView{ListNodesErr: listErr}

	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, nil)

	err := sel.Init(context.Background(), args)
	if err == nil {
		t.Fatal("expected error from ListNodes during Init, got nil")
	}
	if !errors.Is(err, listErr) {
		t.Errorf("expected wrapped listErr, got: %v", err)
	}
}

// ---- PDB tests --------------------------------------------------------------

func TestNextCandidate_PDB_BlocksNodeWhenBudgetExhausted(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	testutil.AddPod(t, v, "pod-a", "default", "node-a",
		testutil.PodOpts{Labels: map[string]string{"app": "web"}})

	pdb := testutil.MakePDB("pdb-web", "default", map[string]string{"app": "web"}, 0)
	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))}, pdb)
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil (PDB budget exhausted), got %q", got.Name)
	}
}

func TestNextCandidate_PDB_AllowsNodeWhenBudgetSufficient(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	testutil.AddPod(t, v, "pod-a", "default", "node-a",
		testutil.PodOpts{Labels: map[string]string{"app": "web"}})

	pdb := testutil.MakePDB("pdb-web", "default", map[string]string{"app": "web"}, 1)
	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))}, pdb)
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a candidate (PDB budget sufficient), got nil")
	}
	if got.Name != "node-a" {
		t.Errorf("expected node-a, got %q", got.Name)
	}
}

func TestNextCandidate_PDB_MultiplePodsExceedBudget(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	testutil.AddPod(t, v, "pod-a1", "default", "node-a",
		testutil.PodOpts{Labels: map[string]string{"app": "web"}})
	testutil.AddPod(t, v, "pod-a2", "default", "node-a",
		testutil.PodOpts{Labels: map[string]string{"app": "web"}})

	pdb := testutil.MakePDB("pdb-web", "default", map[string]string{"app": "web"}, 1)
	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))}, pdb)
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil (2 pods exceed PDB budget of 1), got %q", got.Name)
	}
}

func TestNextCandidate_PDB_NamespaceMismatchDoesNotBlock(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	testutil.AddPod(t, v, "pod-a", "other", "node-a",
		testutil.PodOpts{Labels: map[string]string{"app": "web"}})

	pdb := testutil.MakePDB("pdb-web", "default", map[string]string{"app": "web"}, 0)
	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))}, pdb)
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a candidate (PDB namespace mismatch), got nil")
	}
}

func TestNextCandidate_PDB_LabelMismatchDoesNotBlock(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	testutil.AddPod(t, v, "pod-a", "default", "node-a",
		testutil.PodOpts{Labels: map[string]string{"app": "worker"}})

	pdb := testutil.MakePDB("pdb-web", "default", map[string]string{"app": "web"}, 0)
	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))}, pdb)
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a candidate (pod labels don't match PDB selector), got nil")
	}
}

func TestNextCandidate_PDB_BlockedNodeSkippedOtherNodeSelected(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})
	testutil.AddNode(t, v, "node-b", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	testutil.AddPod(t, v, "pod-a", "default", "node-a",
		testutil.PodOpts{Labels: map[string]string{"app": "web"}})
	testutil.AddPod(t, v, "pod-b", "default", "node-b",
		testutil.PodOpts{Labels: map[string]string{"app": "worker"}})

	pdb := testutil.MakePDB("pdb-web", "default", map[string]string{"app": "web"}, 0)
	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))}, pdb)
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected node-b as candidate, got nil")
	}
	if got.Name != "node-b" {
		t.Errorf("expected node-b (node-a is PDB-blocked), got %q", got.Name)
	}
}

func TestNextCandidate_PDB_MultiplePDBsAllMustAllow(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	testutil.AddPod(t, v, "pod-a", "default", "node-a",
		testutil.PodOpts{Labels: map[string]string{"app": "web", "tier": "frontend"}})

	pdb1 := testutil.MakePDB("pdb-web", "default", map[string]string{"app": "web"}, 1)
	pdb2 := testutil.MakePDB("pdb-frontend", "default", map[string]string{"tier": "frontend"}, 0)
	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))}, pdb1, pdb2)
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil (second PDB blocks), got %q", got.Name)
	}
}

func TestNextCandidate_PDB_NoPDBsConfiguredAllowsAll(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	testutil.AddPod(t, v, "pod-a", "default", "node-a",
		testutil.PodOpts{Labels: map[string]string{"app": "web"}})

	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))})
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a candidate (no PDBs configured), got nil")
	}
}

func TestNextCandidate_PDB_NodeWithNoPodsNotBlocked(t *testing.T) {
	v := testutil.NewTestView(t)
	testutil.AddNode(t, v, "node-a", testutil.NodeOpts{Pool: "pool1", Template: "tmpl1"})

	pdb := testutil.MakePDB("pdb-web", "default", map[string]string{"app": "web"}, 0)
	sel := New(testutil.LowUtilCalc())
	args := testutil.MakeCandidateArgs(t, v, []sacorev1alpha1.NodePool{testutil.Pool("pool1", 0, 0, testutil.Tmpl("tmpl1", 0))}, pdb)
	if err := sel.Init(context.Background(), args); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := sel.NextCandidate(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a candidate (node has no pods), got nil")
	}
}
