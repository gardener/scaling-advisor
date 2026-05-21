package testutil

import (
	"context"
	"testing"
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/minkapi/view"
	pdbtracker "github.com/gardener/scaling-advisor/planner/pdbtracker"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
)

// ---- view / object helpers --------------------------------------------------

func NewTestView(t *testing.T) minkapi.View {
	t.Helper()
	v, err := view.NewBase(&minkapi.ViewArgs{
		Name:   minkapi.DefaultBasePrefix,
		Scheme: typeinfo.SupportedScheme,
		WatchConfig: minkapi.WatchConfig{
			QueueSize: 100,
			Timeout:   500 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("failed to create test view: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })
	return v
}

func NewTestViewAccess(t *testing.T) minkapi.ViewAccess {
	t.Helper()
	va, err := view.NewAccess(t.Context(), &minkapi.ViewArgs{
		Name:   minkapi.DefaultBasePrefix,
		Scheme: typeinfo.SupportedScheme,
		WatchConfig: minkapi.WatchConfig{
			QueueSize: 100,
			Timeout:   500 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("failed to create ViewAccess: %v", err)
	}
	t.Cleanup(func() { _ = va.Close() })
	return va
}

// NodeOpts configures a test node. Zero value gives a bare node with default allocatable (4 CPU, 8Gi memory).
type NodeOpts struct {
	Allocatable corev1.ResourceList
	Pool        string
	Template    string
	Annotations map[string]string
}

func AddNode(t *testing.T, v minkapi.View, name string, opts ...NodeOpts) {
	t.Helper()
	var o NodeOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: o.Annotations,
		},
	}
	if o.Pool != "" || o.Template != "" {
		node.Labels = map[string]string{
			commonconstants.LabelNodePoolName:     o.Pool,
			commonconstants.LabelNodeTemplateName: o.Template,
		}
	}
	if o.Allocatable != nil {
		node.Status.Allocatable = o.Allocatable
	} else {
		node.Status.Allocatable = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
		}
	}
	if _, err := v.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, node); err != nil {
		t.Fatalf("failed to add node %q: %v", name, err)
	}
}

// PodOpts configures a test pod. Zero value gives a pod with default requests (100m CPU, 128Mi memory).
type PodOpts struct {
	Requests    corev1.ResourceList
	Labels      map[string]string
	Annotations map[string]string
}

func AddPod(t *testing.T, v minkapi.View, name, namespace, nodeName string, opts ...PodOpts) {
	t.Helper()
	var o PodOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	requests := o.Requests
	if requests == nil {
		requests = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		}
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      o.Labels,
			Annotations: o.Annotations,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{Name: "c", Resources: corev1.ResourceRequirements{Requests: requests}},
			},
		},
	}
	if _, err := v.CreateObject(t.Context(), typeinfo.PodsDescriptor.GVK, pod); err != nil {
		t.Fatalf("failed to add pod %q: %v", name, err)
	}
}

func AddPDBToView(t *testing.T, v minkapi.View, name, namespace string, matchLabels map[string]string, disruptionsAllowed int32) {
	t.Helper()
	pdb := MakePDB(name, namespace, matchLabels, disruptionsAllowed)
	if _, err := v.CreateObject(t.Context(), typeinfo.PodDisruptionBudgetDescriptor.GVK, pdb); err != nil {
		t.Fatalf("failed to add PDB %q: %v", name, err)
	}
}

// ---- object constructors ----------------------------------------------------

func MakePDB(name, namespace string, matchLabels map[string]string, disruptionsAllowed int32) *policyv1.PodDisruptionBudget {
	minAvail := intstr.FromInt32(1)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvail,
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: disruptionsAllowed,
		},
	}
}

func Pool(name string, min, priority int32, templates ...sacorev1alpha1.NodeTemplate) sacorev1alpha1.NodePool {
	return sacorev1alpha1.NodePool{Name: name, Min: min, Priority: priority, NodeTemplates: templates}
}

func Tmpl(name string, priority int32) sacorev1alpha1.NodeTemplate {
	return sacorev1alpha1.NodeTemplate{Name: name, Priority: priority}
}

func EmptySkip() *sets.Set[string] {
	s := sets.New[string]()
	return &s
}

func Node(name string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

// ---- request / config constructors ------------------------------------------

// RequestOpts configures optional fields on a test Request's ClusterSnapshot.
type RequestOpts struct {
	Pods []plannerapi.PodInfo
	PDBs []*policyv1.PodDisruptionBudget
}

func MakeRequest(id string, opts ...RequestOpts) *plannerapi.Request {
	var o RequestOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	pdbs := make([]policyv1.PodDisruptionBudget, 0, len(o.PDBs))
	for _, p := range o.PDBs {
		pdbs = append(pdbs, *p)
	}
	return &plannerapi.Request{
		RequestRef: plannerapi.RequestRef{ID: id},
		Constraint: &sacorev1alpha1.ScalingConstraint{},
		Memento: plannerapi.Memento{
			ScaleIn: plannerapi.ScaleInMemento{},
		},
		Snapshot: plannerapi.ClusterSnapshot{
			Pods: o.Pods,
			PDBs: pdbs,
		},
	}
}

func MakeSimulatorConfig(underutilizedDuration time.Duration) plannerapi.ScaleInSimulatorConfig {
	return plannerapi.ScaleInSimulatorConfig{
		UnderutilizedDuration: underutilizedDuration,
	}
}

func MakeCandidateArgs(t *testing.T, v minkapi.View, pools []sacorev1alpha1.NodePool, pdbs ...*policyv1.PodDisruptionBudget) plannerapi.ScaleInCandidateArgs {
	t.Helper()
	tracker := pdbtracker.New()
	if len(pdbs) > 0 {
		if err := tracker.SetPDBs(pdbs); err != nil {
			t.Fatalf("failed to set PDBs: %v", err)
		}
	}
	return plannerapi.ScaleInCandidateArgs{
		View:       v,
		Constraint: sacorev1alpha1.ScalingConstraintSpec{NodePools: pools},
		PDBTracker: tracker,
	}
}

func DrainResult(t *testing.T, ch <-chan plannerapi.ScaleInPlanResult) plannerapi.ScaleInPlanResult {
	t.Helper()
	r, ok := <-ch
	if !ok {
		t.Fatal("result channel closed without delivering a result")
	}
	return r
}

// ---- stub utilization calculators -------------------------------------------

// StubUtilizationCalculator returns fixed utilization ratios for every node.
type StubUtilizationCalculator struct {
	CPURatio float64
	MemRatio float64
	Err      error
}

func (s *StubUtilizationCalculator) GetUtilization(_ context.Context, _ minkapi.View, _ string) (plannerapi.NodeUtilization, error) {
	if s.Err != nil {
		return plannerapi.NodeUtilization{}, s.Err
	}
	return plannerapi.NodeUtilization{
		ResourceRatios: map[corev1.ResourceName]float64{
			corev1.ResourceCPU:    s.CPURatio,
			corev1.ResourceMemory: s.MemRatio,
		},
	}, nil
}

func LowUtilCalc() plannerapi.NodeUtilizationCalculator {
	return &StubUtilizationCalculator{CPURatio: 0.1, MemRatio: 0.1}
}

func HighUtilCalc() plannerapi.NodeUtilizationCalculator {
	return &StubUtilizationCalculator{CPURatio: 0.9, MemRatio: 0.9}
}

func ErrUtilCalc(err error) plannerapi.NodeUtilizationCalculator {
	return &StubUtilizationCalculator{Err: err}
}

// ---- stub candidate selectors -----------------------------------------------

// FixedCandidateSelector yields a pre-set sequence of nodes, then nil.
type FixedCandidateSelector struct {
	Nodes []*corev1.Node
	Idx   int
}

func (f *FixedCandidateSelector) NextCandidate(_ context.Context, _ plannerapi.ScaleInCandidateArgs, _ *sets.Set[string]) (*corev1.Node, error) {
	if f.Idx >= len(f.Nodes) {
		return nil, nil
	}
	n := f.Nodes[f.Idx]
	f.Idx++
	return n, nil
}

// ErrCandidateSelector always returns an error from NextCandidate.
type ErrCandidateSelector struct{ Err error }

func (e *ErrCandidateSelector) NextCandidate(_ context.Context, _ plannerapi.ScaleInCandidateArgs, _ *sets.Set[string]) (*corev1.Node, error) {
	return nil, e.Err
}

// AlwaysCandidateSelector returns the same node forever (until it gets
// added to skipNodes by the caller, at which point it returns nil).
type AlwaysCandidateSelector struct{ N *corev1.Node }

func (a *AlwaysCandidateSelector) NextCandidate(_ context.Context, _ plannerapi.ScaleInCandidateArgs, skip *sets.Set[string]) (*corev1.Node, error) {
	if skip.Has(a.N.Name) {
		return nil, nil
	}
	return a.N, nil
}

// PDBAwareCandidateSelector uses the PDBTracker from args to decide if a node's
// pods can be removed. It yields nodes from a fixed list, skipping those blocked by PDBs.
type PDBAwareCandidateSelector struct {
	Nodes []*corev1.Node
	Pods  map[string][]*corev1.Pod // nodeName -> pods on that node
	Idx   int
}

func (p *PDBAwareCandidateSelector) NextCandidate(_ context.Context, args plannerapi.ScaleInCandidateArgs, skip *sets.Set[string]) (*corev1.Node, error) {
	for p.Idx < len(p.Nodes) {
		n := p.Nodes[p.Idx]
		p.Idx++
		if skip.Has(n.Name) {
			continue
		}
		nodePods := p.Pods[n.Name]
		if len(nodePods) > 0 {
			if canRemove, _ := args.PDBTracker.CanRemovePods(nodePods); !canRemove {
				skip.Insert(n.Name)
				continue
			}
		}
		return n, nil
	}
	return nil, nil
}

// ---- stub simulations / factories -------------------------------------------

// SuccessSimulation: Run succeeds, all pods rescheduled.
type SuccessSimulation struct {
	NodeName string
	SimView  minkapi.View
}

func (s *SuccessSimulation) Reset() error { return nil }
func (s *SuccessSimulation) Name() string { return "stub-success" }
func (s *SuccessSimulation) Status() plannerapi.ActivityStatus {
	return plannerapi.ActivityStatusSuccess
}
func (s *SuccessSimulation) PriorityKey() commontypes.PriorityKey { return commontypes.PriorityKey{} }
func (s *SuccessSimulation) Run(_ context.Context, v minkapi.View, node *corev1.Node) error {
	s.SimView = v
	if s.NodeName == "" {
		s.NodeName = node.Name
	}
	return nil
}
func (s *SuccessSimulation) Result() (plannerapi.ScaleInSimRunResult, error) {
	return plannerapi.ScaleInSimRunResult{
		Name:             "stub-success",
		View:             s.SimView,
		Item:             sacorev1alpha1.ScaleInItem{NodeName: s.NodeName},
		PodsToReschedule: sets.New[commontypes.NamespacedName](),
	}, nil
}

// PendingPodsSimulation: Run succeeds but pods remain unscheduled.
type PendingPodsSimulation struct {
	SimView minkapi.View
}

func (p *PendingPodsSimulation) Reset() error { return nil }
func (p *PendingPodsSimulation) Name() string { return "stub-pending" }
func (p *PendingPodsSimulation) Status() plannerapi.ActivityStatus {
	return plannerapi.ActivityStatusSuccess
}
func (p *PendingPodsSimulation) PriorityKey() commontypes.PriorityKey {
	return commontypes.PriorityKey{}
}
func (p *PendingPodsSimulation) Run(_ context.Context, v minkapi.View, _ *corev1.Node) error {
	p.SimView = v
	return nil
}
func (p *PendingPodsSimulation) Result() (plannerapi.ScaleInSimRunResult, error) {
	remaining := sets.New(commontypes.NamespacedName{Namespace: "default", Name: "stuck-pod"})
	return plannerapi.ScaleInSimRunResult{
		Name:             "stub-pending",
		View:             p.SimView,
		Item:             sacorev1alpha1.ScaleInItem{},
		PodsToReschedule: remaining,
	}, nil
}

// FailingSimulation: Run returns an error.
type FailingSimulation struct{ Err error }

func (f *FailingSimulation) Reset() error { return nil }
func (f *FailingSimulation) Name() string { return "stub-fail" }
func (f *FailingSimulation) Status() plannerapi.ActivityStatus {
	return plannerapi.ActivityStatusFailure
}
func (f *FailingSimulation) PriorityKey() commontypes.PriorityKey { return commontypes.PriorityKey{} }
func (f *FailingSimulation) Run(_ context.Context, _ minkapi.View, _ *corev1.Node) error {
	return f.Err
}
func (f *FailingSimulation) Result() (plannerapi.ScaleInSimRunResult, error) {
	return plannerapi.ScaleInSimRunResult{}, f.Err
}

// StubSimulationFactory is a SimulationFactory that returns pre-configured simulation instances.
type StubSimulationFactory struct {
	Sim plannerapi.ScaleInSimulation
	Err error
}

func (s *StubSimulationFactory) NewScaleOut(_ plannerapi.ScaleOutSimArgs) (plannerapi.ScaleOutSimulation, error) {
	panic("not used in scale-in tests")
}
func (s *StubSimulationFactory) NewScaleIn(_ plannerapi.ScaleInSimArgs) (plannerapi.ScaleInSimulation, error) {
	return s.Sim, s.Err
}

// ---- stub views -------------------------------------------------------------

// FailingView is a minkapi.View stub that returns preset errors for ListNodes and/or ListPods.
// When ListNodesErr is nil, ListNodes returns a single fake node named "any-node".
type FailingView struct {
	minkapi.View
	ListNodesErr error
	ListPodsErr  error
}

func (f *FailingView) ListNodes(_ context.Context, _ ...string) ([]corev1.Node, error) {
	if f.ListNodesErr != nil {
		return nil, f.ListNodesErr
	}
	return []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "any-node"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
			},
		},
	}, nil
}

func (f *FailingView) ListPods(_ context.Context, _ minkapi.MatchCriteria) ([]corev1.Pod, error) {
	return nil, f.ListPodsErr
}
