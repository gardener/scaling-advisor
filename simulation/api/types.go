package api

import (
	"context"
	"io"
	"sync/atomic"
	"time"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	minkapi "github.com/gardener/scaling-advisor/minkapi/api"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/tools/events"
)

// SimulationFactory is a factory facade for creating [Simulation] objects
type SimulationFactory interface {
	// NewScaleOut creates a ScaleOutSimulation instance with the given name and arguments.
	NewScaleOut(args SimulationArgs) (Simulation, error)
}

// SimulationArgs represents the arguments necessary for creating a [Simulation] instance.
type SimulationArgs struct {
	// SchedulerLauncher is used to launch scheduler instances for the simulation.
	SchedulerLauncher SchedulerLauncher
	// StorageMetaAccess is interrogated for metadata to create CSINodes for the simulation
	StorageMetaAccess StorageMetaAccess
	// RunCounter is an atomic counter for tracking simulation runs.
	RunCounter *atomic.Uint32
	// Name is the name of the simulation instance
	Name string
	// TraceDir is the base directory for storing trace logs and other dump data by the simulation
	TraceDir string
	// Strategy is the strategy being used by the parent [Simulator] that is running this simulation.
	Strategy apicommon.SimulatorStrategy
	// NodeTemplates is a slice of [ScaleOutNodeTemplate] representing information needed to create scale-out simulated nodes.
	NodeTemplates []ScaleOutNodeTemplate
	// Config is the simulation configuration.
	Config SimulationConfig
}

// SimulationConfig holds the configuration for the [Simulation].
type SimulationConfig struct {
	// TrackPollInterval is the polling interval for tracking pod scheduling in the view of the simulation
	TrackPollInterval time.Duration
	// MaxUnchangedTrackAttempts is the maximum number of unchanged simulation track attempts after which a simulation run is
	// considered as stabilized.
	MaxUnchangedTrackAttempts int
	// BindVolumeClaimsForImmediateMode should be set if simulation is expected to bind unbound PVC<->PV for
	// [corev1.VolumeBindingImmediate], also creating a simulated PV if a matching existing PV doesn't exist.
	BindVolumeClaimsForImmediateMode bool
}

// SchedulerLauncher defines the interface for launching a kube-scheduler instance.
// There will be a limited number of kube-scheduler instances that can be launched at a time.
type SchedulerLauncher interface {
	// Launch launches and runs an embedded scheduler instance asynchronously.
	// If the limit of running schedulers is reached, it will block.
	// An error is returned if the scheduler fails to start.
	Launch(ctx context.Context, params *SchedulerLaunchParams) (SchedulerHandle, error)
}

// SchedulerLaunchParams holds the parameters required to launch a kube-scheduler instance.
type SchedulerLaunchParams struct {
	// EventSink is the event sink used to send events from the kube-scheduler.
	EventSink events.EventSink
	minkapi.ClientFacades
}

// SchedulerHandle defines the interface for managing a kube-scheduler instance.
type SchedulerHandle interface {
	io.Closer
	// GetParams returns the parameters used to launch the scheduler instance.
	GetParams() SchedulerLaunchParams
}

// StorageMetaAccess defines an interface for querying misc storage metadata
type StorageMetaAccess interface {
	// GetFallbackCSINodeSpec gets the default storagev1.CSINodeSpec which is suitable for the given instanceType.
	// Used as a fallback when there is no CSINodeSpec associated with the NodeInfo or in a scale-from-zero
	// scenario.
	GetFallbackCSINodeSpec(instanceType string) (storagev1.CSINodeSpec, error)
}

// Simulation represents a simulation that can depending on the implementation can scale-in nodes or scale-out virtual node(s)
// or do both and perform valid unscheduled pod to ready node bindings against a minkapi View.
// The default Simulation implementation uses an embedded k8s scheduler to perform this work.
// More exotic implementations could form a SAT (Satisfiability Testing) /MIP (Mixed Integer Programming)
// constraint model from the pod/node data and run a tool that solves the model.
type Simulation interface {
	apicommon.Resettable
	// Name returns the logical simulation name
	Name() string
	// Status returns the current ActivityStatus of the simulation
	Status() ActivityStatus
	// PriorityKey returns the PriorityKey for the simulation which is the key by which simulations are grouped and determines
	// the order in which simulations are run.
	PriorityKey() apicommon.PriorityKey
	// Run executes the simulation against the given simulation [minkapi.View] to completion and returns the [SimulationResult]
	// or any encountered error.
	// This is a blocking call, and callers are expected to manage concurrency and ScaleOutSimResult consumption.
	Run(ctx context.Context, view minkapi.View) (SimulationResult, error)
}

// ActivityStatus represents the operational status of an activity.
type ActivityStatus string

const (
	// ActivityStatusPending indicates the activity is pending execution.
	ActivityStatusPending ActivityStatus = "Pending"
	// ActivityStatusRunning indicates the activity is currently running.
	ActivityStatusRunning ActivityStatus = "Running"
	// ActivityStatusSuccess indicates the activity completed successfully.
	ActivityStatusSuccess ActivityStatus = metav1.StatusSuccess
	// ActivityStatusFailure indicates the activity failed.
	ActivityStatusFailure ActivityStatus = metav1.StatusFailure
)

// ScaleOutNodeTemplate is a superset of the [sacorev1alpha1.NodePlacement] consisting of enough information to create
// a simulated scale-out [corev1.Node] within a [minkapi.View] such that the `kube-scheduler` can bind pods to nodes.
//
// Depending on the choice of [commontypes.SimulatorStrategy], a scale-out [Simulation] can either:
//   - Execute multiple concurrent simulations scaling a node for each [ScaleOutNodeTemplate] at the same priority and choosing
//     a winner among the concurrent simulations to determine chosen [sacorev1alpha1.NodePlacement]'s in the [sacorev1alpha1.ScaleOutPlan]
//   - Or it may execute a single simulation scaling multiple nodes for all ScaleOutNodeTemplate's at same priority, choosing nodes
//     with successful pod-assignments to determine [sacorev1alpha1.NodePlacement]'s in the [sacorev1alpha1.ScaleOutPlan]
type ScaleOutNodeTemplate struct {
	// Labels is a map of key/value pairs for labels applied to all the nodes in this node pool.
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations is a map of key/value pairs for annotations applied to all the nodes in this node pool.
	Annotations map[string]string `json:"annotations,omitempty"`
	// Quota defines the resource quota for the node pool.
	Quota corev1.ResourceList `json:"quota,omitempty"`
	// Capacity defines the capacity for node resources that are available for the node's instance type.
	Capacity corev1.ResourceList `json:"capacity"`
	// KubeReserved defines the capacity for kube reserved resources.
	// See https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#kube-reserved for additional information.
	KubeReserved corev1.ResourceList `json:"kubeReservedCapacity,omitempty"`
	// SystemReserved defines the capacity for system reserved resources.
	// See https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#system-reserved for additional information.
	SystemReserved               corev1.ResourceList `json:"systemReservedCapacity,omitempty"`
	sacorev1alpha1.NodePlacement `json:",inline"`
	// Architecture is the CPU architecture of the node's instance type.
	Architecture string `json:"architecture"`
	// Taints is a list of taints applied to all the nodes in this node pool.
	Taints []corev1.Taint `json:"taints,omitempty"`
	// PriorityKey is the priority key for this ScaleOutNodeTemplate.
	PriorityKey apicommon.PriorityKey
}

// SimulationResult contains the results of a completed simulation run.
type SimulationResult struct {
	// Name of the ScaleOutSimulation that produced this result.
	Name string
	// View is the minkapi View against which the simulation was run.
	View minkapi.View
	// ScaleOutItems is the slice of [sacorev1alpha1.ScaleOutItem] where each item encapsulates the
	// [sacorev1alpha1.NodePlacement] and associated delta.
	ScaleOutItems []sacorev1alpha1.ScaleOutItem
	// ScaleInItems is the slice of [sacorev1alpha1.ScaleInItem] where each item encapsulates the
	// [sacorev1alpha1.NodePlacement] and node name that was successfully scaled-in.
	ScaleInItems []sacorev1alpha1.ScaleInItem
	// NodePodAssignments represents the assignment of Pods to simulated scale-out Nodes.
	NodePodAssignments []NodePodAssignment
	// OtherNodePodAssignments represent the assignment of unscheduled Pods to either an existing Node which is part of
	// the ClusterSnapshot, or it is a simulated scale-out Node from a previous run.
	OtherNodePodAssignments []NodePodAssignment
	// LeftoverUnscheduledPods is the slice of unscheduled pods that remain unscheduled after the simulation Run is
	// completed.
	LeftoverUnscheduledPods []apicommon.NamespacedName
}

// SimulationGroup is a group of [Simulation]'s at the same priority level (ie a partition of simulations).
type SimulationGroup interface {
	apicommon.Resettable
	// Name returns the name of the simulation group.
	Name() string
	// PriorityKey returns the simulation group key.
	PriorityKey() apicommon.PriorityKey
	// GetSimulations returns all simulations in this group.
	GetSimulations() []Simulation
	// AddSimulation adds a simulation to the group.
	AddSimulation(simulation Simulation)
	// Run executes all simulations in the group and returns all the simulation run results or any error.
	Run(ctx context.Context, getViewFn minkapi.GetViewFunc) ([]SimulationResult, error)
}

// NodePodAssignment represents the assignment of pods to a node for simulation purposes.
type NodePodAssignment struct {
	// NodeResources contains the resource information for the node.
	NodeResources NodeResourceInfo
	// ScheduledPods contains the list of pods scheduled to this node.
	ScheduledPods []PodResourceInfo
}

// PodResourceInfo contains resource information for a pod used by the [Simulation] and given by the [SimulationResult].
type PodResourceInfo struct {
	// AggregatedRequests is an aggregated resource requests for all containers of the Pod.
	AggregatedRequests       corev1.ResourceList `json:"aggregatedRequests,omitempty"`
	apicommon.NamespacedName `json:",inline"`
}

// NodeResourceInfo represents the instance type and resource information about a node given by the [SimulationResult].
type NodeResourceInfo struct {
	// Capacity is the total resource capacity of the node.
	Capacity corev1.ResourceList
	// Allocatable is the allocatable resource capacity of the node.
	Allocatable corev1.ResourceList
	// Name is the node name.
	Name string
	// InstanceType is the cloud instance type of the node.
	InstanceType string
}

// VolumeClaimAssignment represents the assignment of a PersistentVolumeClaim to a PersistentVolume
type VolumeClaimAssignment struct {
	// ClaimName is the PVC namespaced name.
	ClaimName apicommon.NamespacedName
	// VolumeName is the name of the bound PV
	VolumeName string
}

// CmpSimulationDecreasingPriority is a cmp function for [ScaleOutSimulation] that compares by decreasing PriorityKey.
func CmpSimulationDecreasingPriority(s1, s2 Simulation) int {
	return apicommon.CmpPriorityKeyDecreasing(s1.PriorityKey(), s2.PriorityKey())
}

// CmpSimulationGroup is a cmp function for [SimulationGroup] that compares by decreasing PriorityKey.
func CmpSimulationGroup(s1, s2 SimulationGroup) int {
	return apicommon.CmpPriorityKeyDecreasing(s1.PriorityKey(), s2.PriorityKey())
}

// CmpScaleOutNodeTemplate is a cmp function for [ScaleOutNodeTemplate] that compares by decreasing PriorityKey.
func CmpScaleOutNodeTemplate(a, b ScaleOutNodeTemplate) int {
	return apicommon.CmpPriorityKeyDecreasing(a.PriorityKey, b.PriorityKey)
}
