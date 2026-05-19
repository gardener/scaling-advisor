package defaultSimulator

import (
	"context"
	"errors"
	"testing"
	"time"

	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/planner/testutil"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// ---- helpers -------------------------------------------------

func newSimulator(t *testing.T, sel plannerapi.ScaleInCandidateSelector, cfg plannerapi.ScaleInSimulatorConfig) plannerapi.ScaleInSimulator {
	t.Helper()
	va := testutil.NewTestViewAccess(t)
	sim, err := New(plannerapi.SimulatorArgs{
		ViewAccess:               va,
		ScaleInCandidateSelector: sel,
		ScaleInSimulatorConfig:   cfg,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return sim
}

// ---- Simulate tests ---------------------------------------------------------

func TestSimulate_NoCandidates_EmptyPlan(t *testing.T) {
	sel := &testutil.FixedCandidateSelector{} // immediately returns nil
	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(0))

	result := testutil.DrainResult(t, sim.Simulate(t.Context(), testutil.MakeRequest("r1"), &testutil.StubSimulationFactory{Sim: &testutil.SuccessSimulation{}}))

	if !errors.Is(result.Error, plannerapi.ErrNoScaleInPlan) {
		t.Fatalf("expected ErrNoScaleInPlan, got: %v", result.Error)
	}
	if result.ScaleInPlan != nil {
		t.Errorf("expected nil ScaleInPlan, got %+v", result.ScaleInPlan)
	}
}

func TestSimulate_CandidateSelectorError_ErrorResult(t *testing.T) {
	selErr := errors.New("selector exploded")
	sel := &testutil.ErrCandidateSelector{Err: selErr}
	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(0))

	result := testutil.DrainResult(t, sim.Simulate(t.Context(), testutil.MakeRequest("r2"), &testutil.StubSimulationFactory{Sim: &testutil.SuccessSimulation{}}))

	if result.Error == nil {
		t.Fatal("expected an error result, got nil")
	}
	if !errors.Is(result.Error, plannerapi.ErrGenScalingPlan) {
		t.Errorf("expected ErrGenScalingPlan in chain, got: %v", result.Error)
	}
}

func TestSimulate_SimulationFactoryError_ErrorResult(t *testing.T) {
	factoryErr := errors.New("factory failed")
	// Selector returns one node so the factory is actually called.
	sel := &testutil.FixedCandidateSelector{Nodes: []*corev1.Node{testutil.Node("node-a")}}
	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(0))

	result := testutil.DrainResult(t, sim.Simulate(t.Context(), testutil.MakeRequest("r3"), &testutil.StubSimulationFactory{Err: factoryErr}))

	if result.Error == nil {
		t.Fatal("expected an error result, got nil")
	}
	if !errors.Is(result.Error, plannerapi.ErrGenScalingPlan) {
		t.Errorf("expected ErrGenScalingPlan in chain, got: %v", result.Error)
	}
}

func TestSimulate_SimulationRunError_ErrorResult(t *testing.T) {
	runErr := errors.New("run failed")
	sel := &testutil.FixedCandidateSelector{Nodes: []*corev1.Node{testutil.Node("node-a")}}
	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(0))

	result := testutil.DrainResult(t, sim.Simulate(t.Context(), testutil.MakeRequest("r4"), &testutil.StubSimulationFactory{Sim: &testutil.FailingSimulation{Err: runErr}}))

	if result.Error == nil {
		t.Fatal("expected an error result, got nil")
	}
	if !errors.Is(result.Error, plannerapi.ErrGenScalingPlan) {
		t.Errorf("expected ErrGenScalingPlan in chain, got: %v", result.Error)
	}
}

func TestSimulate_PendingPods_NodeSkippedNotInPlan(t *testing.T) {
	sel := &testutil.FixedCandidateSelector{Nodes: []*corev1.Node{testutil.Node("node-a")}}
	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(0))

	result := testutil.DrainResult(t, sim.Simulate(t.Context(), testutil.MakeRequest("r5"), &testutil.StubSimulationFactory{Sim: &testutil.PendingPodsSimulation{}}))

	if !errors.Is(result.Error, plannerapi.ErrNoScaleInPlan) {
		t.Fatalf("expected ErrNoScaleInPlan, got: %v", result.Error)
	}
	if result.ScaleInPlan != nil {
		t.Errorf("expected nil ScaleInPlan (pods unscheduled → node skipped), got %+v", result.ScaleInPlan)
	}
}

func TestSimulate_SuccessfulCandidate_NodeRecordedInMemento(t *testing.T) {
	sel := &testutil.FixedCandidateSelector{Nodes: []*corev1.Node{testutil.Node("node-a")}}
	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(10*time.Minute))

	req := testutil.MakeRequest("r6")
	result := testutil.DrainResult(t, sim.Simulate(t.Context(), req, &testutil.StubSimulationFactory{Sim: &testutil.SuccessSimulation{NodeName: "node-a"}}))

	// First sighting → no plan items yet, so ErrNoScaleInPlan is returned.
	if !errors.Is(result.Error, plannerapi.ErrNoScaleInPlan) {
		t.Fatalf("expected ErrNoScaleInPlan on first sighting, got: %v", result.Error)
	}
	if result.ScaleInPlan != nil {
		t.Errorf("expected nil ScaleInPlan on first sighting, got %+v", result.ScaleInPlan)
	}
	if _, recorded := result.Memento.LastIdentifiedUnneededNodes["node-a"]; !recorded {
		t.Error("expected node-a to be recorded in memento after first sighting")
	}
}

func TestSimulate_SuccessfulCandidate_EmittedAfterDuration(t *testing.T) {
	req := testutil.MakeRequest("r7")
	req.Memento.ScaleIn.LastIdentifiedUnneededNodes = map[string]time.Time{
		"node-a": time.Now().Add(-10 * time.Minute),
	}

	sel := &testutil.FixedCandidateSelector{Nodes: []*corev1.Node{testutil.Node("node-a")}}
	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(5*time.Minute))

	result := testutil.DrainResult(t, sim.Simulate(t.Context(), req, &testutil.StubSimulationFactory{Sim: &testutil.SuccessSimulation{NodeName: "node-a"}}))

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.ScaleInPlan == nil {
		t.Fatal("expected a ScaleInPlan, got nil")
	}
	if len(result.ScaleInPlan.Items) != 1 || result.ScaleInPlan.Items[0].NodeName != "node-a" {
		t.Errorf("expected plan with node-a, got %+v", result.ScaleInPlan)
	}
}

func TestSimulate_ContextCancelled_ErrorResult(t *testing.T) {
	alwaysReturns := &testutil.AlwaysCandidateSelector{N: testutil.Node("node-x")}
	sim := newSimulator(t, alwaysReturns, testutil.MakeSimulatorConfig(0))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := testutil.DrainResult(t, sim.Simulate(ctx, testutil.MakeRequest("r8"), &testutil.StubSimulationFactory{Sim: &testutil.SuccessSimulation{NodeName: "node-x"}}))

	if result.Error == nil {
		t.Fatal("expected an error from cancelled context, got nil")
	}
	if !errors.Is(result.Error, plannerapi.ErrGenScalingPlan) {
		t.Errorf("expected ErrGenScalingPlan in chain, got: %v", result.Error)
	}
}

func TestSimulate_ResultChannelClosedAfterResult(t *testing.T) {
	sel := &testutil.FixedCandidateSelector{}
	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(0))

	ch := sim.Simulate(t.Context(), testutil.MakeRequest("r9"), &testutil.StubSimulationFactory{Sim: &testutil.SuccessSimulation{}})
	<-ch // consume the result
	if _, open := <-ch; open {
		t.Error("expected channel to be closed after result")
	}
}

// ---- computeScaleInItems unit tests -----------------------------------------

func makeDefaultSimulator(t *testing.T, cfg plannerapi.ScaleInSimulatorConfig) *defaultSimulator {
	t.Helper()
	va := testutil.NewTestViewAccess(t)
	s, err := New(plannerapi.SimulatorArgs{
		ViewAccess:               va,
		ScaleInCandidateSelector: &testutil.FixedCandidateSelector{},
		ScaleInSimulatorConfig:   cfg,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.(*defaultSimulator)
}

func TestComputeScaleInItems_EmptyInput(t *testing.T) {
	d := makeDefaultSimulator(t, testutil.MakeSimulatorConfig(0))
	items, err := d.computeScaleInItems(t.Context(), &plannerapi.ScaleInMemento{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestComputeScaleInItems_FirstSeen_RecordedNotEmitted(t *testing.T) {
	d := makeDefaultSimulator(t, testutil.MakeSimulatorConfig(10*time.Minute))
	memento := &plannerapi.ScaleInMemento{}
	successNodes := map[string]sacorev1alpha1.ScaleInItem{"node-a": {NodeName: "node-a"}}

	items, err := d.computeScaleInItems(t.Context(), memento, successNodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items on first sighting, got %d", len(items))
	}
	if _, ok := memento.LastIdentifiedUnneededNodes["node-a"]; !ok {
		t.Error("expected node-a recorded in memento")
	}
}

func TestComputeScaleInItems_DurationNotExceeded_NotEmitted(t *testing.T) {
	d := makeDefaultSimulator(t, testutil.MakeSimulatorConfig(10*time.Minute))
	memento := &plannerapi.ScaleInMemento{
		LastIdentifiedUnneededNodes: map[string]time.Time{
			"node-a": time.Now().Add(-1 * time.Minute),
		},
	}
	successNodes := map[string]sacorev1alpha1.ScaleInItem{"node-a": {NodeName: "node-a"}}

	items, err := d.computeScaleInItems(t.Context(), memento, successNodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items (duration not exceeded), got %d", len(items))
	}
}

func TestComputeScaleInItems_DurationExceeded_Emitted(t *testing.T) {
	d := makeDefaultSimulator(t, testutil.MakeSimulatorConfig(5*time.Minute))
	memento := &plannerapi.ScaleInMemento{
		LastIdentifiedUnneededNodes: map[string]time.Time{
			"node-a": time.Now().Add(-10 * time.Minute),
		},
	}
	successNodes := map[string]sacorev1alpha1.ScaleInItem{"node-a": {NodeName: "node-a"}}

	items, err := d.computeScaleInItems(t.Context(), memento, successNodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].NodeName != "node-a" {
		t.Errorf("expected [node-a], got %+v", items)
	}
	if _, still := memento.LastIdentifiedUnneededNodes["node-a"]; still {
		t.Error("expected node-a removed from memento after emission")
	}
}

func TestComputeScaleInItems_NilMementoMap_Initialised(t *testing.T) {
	d := makeDefaultSimulator(t, testutil.MakeSimulatorConfig(0))
	memento := &plannerapi.ScaleInMemento{}
	successNodes := map[string]sacorev1alpha1.ScaleInItem{"node-a": {NodeName: "node-a"}}

	_, err := d.computeScaleInItems(t.Context(), memento, successNodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if memento.LastIdentifiedUnneededNodes == nil {
		t.Error("expected memento map to be initialised")
	}
}

func TestComputeScaleInItems_MultipleNodes_MixedState(t *testing.T) {
	d := makeDefaultSimulator(t, testutil.MakeSimulatorConfig(5*time.Minute))
	memento := &plannerapi.ScaleInMemento{
		LastIdentifiedUnneededNodes: map[string]time.Time{
			"node-old":   time.Now().Add(-10 * time.Minute),
			"node-young": time.Now().Add(-1 * time.Minute),
		},
	}
	successNodes := map[string]sacorev1alpha1.ScaleInItem{
		"node-old":   {NodeName: "node-old"},
		"node-young": {NodeName: "node-young"},
		"node-new":   {NodeName: "node-new"},
	}

	items, err := d.computeScaleInItems(t.Context(), memento, successNodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].NodeName != "node-old" {
		t.Errorf("expected only node-old emitted, got %+v", items)
	}
	if _, present := memento.LastIdentifiedUnneededNodes["node-old"]; present {
		t.Error("node-old should be removed from memento")
	}
	if _, present := memento.LastIdentifiedUnneededNodes["node-young"]; !present {
		t.Error("node-young should remain in memento")
	}
	if _, present := memento.LastIdentifiedUnneededNodes["node-new"]; !present {
		t.Error("node-new should be recorded in memento on first sighting")
	}
}

// ---- PDB integration tests --------------------------------------------------

func TestSimulate_PDB_CandidateBlockedByExhaustedBudget(t *testing.T) {
	podsOnNodeA := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Spec:       corev1.PodSpec{NodeName: "node-a"},
		},
	}

	sel := &testutil.PDBAwareCandidateSelector{
		Nodes: []*corev1.Node{testutil.Node("node-a")},
		Pods:  map[string][]*corev1.Pod{"node-a": podsOnNodeA},
	}

	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(0))
	req := testutil.MakeRequest("pdb-blocked", testutil.RequestOpts{
		PDBs: []*policyv1.PodDisruptionBudget{testutil.MakePDB("pdb-web", "default", map[string]string{"app": "web"}, 0)},
	})

	result := testutil.DrainResult(t, sim.Simulate(t.Context(), req, &testutil.StubSimulationFactory{Sim: &testutil.SuccessSimulation{NodeName: "node-a"}}))

	if !errors.Is(result.Error, plannerapi.ErrNoScaleInPlan) {
		t.Fatalf("expected ErrNoScaleInPlan (node blocked by PDB), got: %v", result.Error)
	}
	if result.ScaleInPlan != nil {
		t.Errorf("expected nil ScaleInPlan (node blocked by PDB), got %+v", result.ScaleInPlan)
	}
}

func TestSimulate_PDB_CandidateAllowedBySufficientBudget(t *testing.T) {
	podsOnNodeA := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Spec:       corev1.PodSpec{NodeName: "node-a"},
		},
	}

	sel := &testutil.PDBAwareCandidateSelector{
		Nodes: []*corev1.Node{testutil.Node("node-a")},
		Pods:  map[string][]*corev1.Pod{"node-a": podsOnNodeA},
	}

	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(0))
	req := testutil.MakeRequest("pdb-allowed", testutil.RequestOpts{
		PDBs: []*policyv1.PodDisruptionBudget{testutil.MakePDB("pdb-web", "default", map[string]string{"app": "web"}, 1)},
	})
	req.Memento.ScaleIn.LastIdentifiedUnneededNodes = map[string]time.Time{
		"node-a": time.Now().Add(-10 * time.Minute),
	}

	result := testutil.DrainResult(t, sim.Simulate(t.Context(), req, &testutil.StubSimulationFactory{Sim: &testutil.SuccessSimulation{NodeName: "node-a"}}))

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.ScaleInPlan == nil {
		t.Fatal("expected a ScaleInPlan (PDB allows disruption), got nil")
	}
	if len(result.ScaleInPlan.Items) != 1 || result.ScaleInPlan.Items[0].NodeName != "node-a" {
		t.Errorf("expected plan with node-a, got %+v", result.ScaleInPlan)
	}
}

func TestSimulate_PDB_OnlyUnblockedNodeSelected(t *testing.T) {
	podsOnNodeA := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Spec:       corev1.PodSpec{NodeName: "node-a"},
		},
	}
	podsOnNodeB := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: "default", Labels: map[string]string{"app": "worker"}},
			Spec:       corev1.PodSpec{NodeName: "node-b"},
		},
	}

	sel := &testutil.PDBAwareCandidateSelector{
		Nodes: []*corev1.Node{testutil.Node("node-a"), testutil.Node("node-b")},
		Pods:  map[string][]*corev1.Pod{"node-a": podsOnNodeA, "node-b": podsOnNodeB},
	}

	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(0))
	req := testutil.MakeRequest("pdb-selective", testutil.RequestOpts{
		PDBs: []*policyv1.PodDisruptionBudget{testutil.MakePDB("pdb-web", "default", map[string]string{"app": "web"}, 0)},
	})
	req.Memento.ScaleIn.LastIdentifiedUnneededNodes = map[string]time.Time{
		"node-a": time.Now().Add(-10 * time.Minute),
		"node-b": time.Now().Add(-10 * time.Minute),
	}

	result := testutil.DrainResult(t, sim.Simulate(t.Context(), req, &testutil.StubSimulationFactory{Sim: &testutil.SuccessSimulation{NodeName: "node-b"}}))

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.ScaleInPlan == nil {
		t.Fatal("expected a ScaleInPlan, got nil")
	}
	if len(result.ScaleInPlan.Items) != 1 || result.ScaleInPlan.Items[0].NodeName != "node-b" {
		t.Errorf("expected plan with only node-b (node-a blocked by PDB), got %+v", result.ScaleInPlan)
	}
}

func TestSimulate_PDB_MultiplePodsSameNodeExceedBudget(t *testing.T) {
	podsOnNodeA := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-a1", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Spec:       corev1.PodSpec{NodeName: "node-a"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-a2", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Spec:       corev1.PodSpec{NodeName: "node-a"},
		},
	}

	sel := &testutil.PDBAwareCandidateSelector{
		Nodes: []*corev1.Node{testutil.Node("node-a")},
		Pods:  map[string][]*corev1.Pod{"node-a": podsOnNodeA},
	}

	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(0))
	req := testutil.MakeRequest("pdb-exceed", testutil.RequestOpts{
		PDBs: []*policyv1.PodDisruptionBudget{testutil.MakePDB("pdb-web", "default", map[string]string{"app": "web"}, 1)},
	})

	result := testutil.DrainResult(t, sim.Simulate(t.Context(), req, &testutil.StubSimulationFactory{Sim: &testutil.SuccessSimulation{NodeName: "node-a"}}))

	if !errors.Is(result.Error, plannerapi.ErrNoScaleInPlan) {
		t.Fatalf("expected ErrNoScaleInPlan (2 pods exceed PDB budget of 1), got: %v", result.Error)
	}
	if result.ScaleInPlan != nil {
		t.Errorf("expected nil ScaleInPlan (2 pods exceed PDB budget of 1), got %+v", result.ScaleInPlan)
	}
}

func TestSimulate_PDB_NoPDBsInView_AllCandidatesAllowed(t *testing.T) {
	podsOnNodeA := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Spec:       corev1.PodSpec{NodeName: "node-a"},
		},
	}

	sel := &testutil.PDBAwareCandidateSelector{
		Nodes: []*corev1.Node{testutil.Node("node-a")},
		Pods:  map[string][]*corev1.Pod{"node-a": podsOnNodeA},
	}

	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(0))
	req := testutil.MakeRequest("no-pdbs")
	req.Memento.ScaleIn.LastIdentifiedUnneededNodes = map[string]time.Time{
		"node-a": time.Now().Add(-10 * time.Minute),
	}

	result := testutil.DrainResult(t, sim.Simulate(t.Context(), req, &testutil.StubSimulationFactory{Sim: &testutil.SuccessSimulation{NodeName: "node-a"}}))

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.ScaleInPlan == nil {
		t.Fatal("expected a ScaleInPlan (no PDBs), got nil")
	}
}

func TestSimulate_PDB_NamespaceMismatchDoesNotBlock(t *testing.T) {
	podsOnNodeA := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Spec:       corev1.PodSpec{NodeName: "node-a"},
		},
	}

	sel := &testutil.PDBAwareCandidateSelector{
		Nodes: []*corev1.Node{testutil.Node("node-a")},
		Pods:  map[string][]*corev1.Pod{"node-a": podsOnNodeA},
	}

	sim := newSimulator(t, sel, testutil.MakeSimulatorConfig(0))
	req := testutil.MakeRequest("pdb-ns-mismatch", testutil.RequestOpts{
		PDBs: []*policyv1.PodDisruptionBudget{testutil.MakePDB("pdb-web", "production", map[string]string{"app": "web"}, 0)},
	})
	req.Memento.ScaleIn.LastIdentifiedUnneededNodes = map[string]time.Time{
		"node-a": time.Now().Add(-10 * time.Minute),
	}

	result := testutil.DrainResult(t, sim.Simulate(t.Context(), req, &testutil.StubSimulationFactory{Sim: &testutil.SuccessSimulation{NodeName: "node-a"}}))

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.ScaleInPlan == nil {
		t.Fatal("expected a ScaleInPlan (PDB namespace mismatch), got nil")
	}
}

func TestInitPdbTracker_LoadsPDBsFromSnapshot(t *testing.T) {
	snapshotPDBs := []policyv1.PodDisruptionBudget{
		*testutil.MakePDB("pdb-1", "default", map[string]string{"app": "web"}, 2),
		*testutil.MakePDB("pdb-2", "kube-system", map[string]string{"component": "dns"}, 1),
	}

	tracker, err := initPdbTracker(snapshotPDBs)
	if err != nil {
		t.Fatalf("initPdbTracker: %v", err)
	}

	pdbs := tracker.GetPdbs()
	if len(pdbs) != 2 {
		t.Fatalf("expected 2 PDBs in tracker, got %d", len(pdbs))
	}

	pdbNames := sets.New[string]()
	for _, p := range pdbs {
		pdbNames.Insert(p.Name)
	}
	if !pdbNames.Has("pdb-1") || !pdbNames.Has("pdb-2") {
		t.Errorf("expected PDBs pdb-1 and pdb-2, got %v", pdbNames)
	}
}

func TestInitPdbTracker_EmptySnapshot_EmptyTracker(t *testing.T) {
	tracker, err := initPdbTracker(nil)
	if err != nil {
		t.Fatalf("initPdbTracker: %v", err)
	}

	pdbs := tracker.GetPdbs()
	if len(pdbs) != 0 {
		t.Errorf("expected 0 PDBs, got %d", len(pdbs))
	}
}
