// Package api defines the types and interfaces for the scaling-advisor simulation layer.
//
// A [Simulation] models a single scale-out or scale-in run against a [github.com/gardener/scaling-advisor/minkapi/api.View].
// Simulations at the same priority are grouped into a [SimulationGroup]; the group
// runs them concurrently and returns all [SimulationResult]s to the caller for
// winner-selection.
//
// The [SimulationFactory] creates [Simulation] instances from [SimulationArgs].
// Scheduling is delegated to a [SchedulerLauncher] which manages the lifecycle of
// embedded kube-scheduler instances.
package api
