# Scaling Advisor Design 

> [!WARNING]
> Work In Progress 

## High Level Operational View

![High Level Operational View](./assets/high-level.svg)

- `scaling-advisor` is an *advisory-controller* that reconciles `ScalingConstraint` and `ScalingFeedback` and generates next `ScalingAdvice`
- Real actions are taken by a consuming lifecycle management component like the MCM. Scaling advisor does not take actions

## Statics

The scaling-advisor has been structured into several modules, some of which are independently re-useable.

![Modules](./assets/modules.svg)

### API (Base) Module
### MinKAPI API Module
### Simulation API Module
### Planner API Module

## Dynamics

### ScalingPlanner
`ScalingPlanner` takes in `plannerapi.Request` and delivers `plannerapi.Response` on a result channel.

```mermaid
---
title: ScalingPlanner
---
classDiagram
    class ScalingPlanner {
        <<interface>>
        +Plan(Request) chan Response
    }
    class Request {
       +ID
       +CorrelationID
       +SimulationStrategy
       +ScoringStrategy
       +ClusterSnapshot
       +DiagnosticVerbosity
    }
    class Response {
      +ID
      +RequestRef(ID, CorrelationID)
      +Error
      +Labels map[string]string 
      +ScaleOutPlan
      +ScaleInPLan
    }
    ScalingPlanner-->Request:consumes
    ScalingPlanner-->Response:produces
```




### Older Material (To be Revised)
![Node Pool Simulation](./assets/scaling-advisor-simulator.svg)

- The `ScalingAdvisor` leverages a minimal in-memory KAPI (called `minkapi`) to hold cluster snapshot (k8s objects such as existing nodes, pods, volumes, volume-attachments, etc.) of the real cluster.
- The base `minkapi` offers *in-memory* k8s client implementations for [kuberentes.Interface](https://pkg.go.dev/k8s.io/client-go/kubernetes#Interface) and [dynamic.Interface](https://pkg.go.dev/k8s.io/client-go/dynamic#Interface) 
- `minkapi` also supports *sandboxed views* and in-memory clients for the sandboxed views which are consumable by the `kube-scheduler`. 
  - The sandbox clients *decorate* the base clients and maintain their own internal state. Changes made by a scheduler instance are limited to its own sandbox and are not propagated to the master data.
- The simulator creates a sandbox, chooses a NodePool and creates virtual Nodes corresponding to the NodePool inside the sandbox. It then runs an embedded scheduler instance that operates against the sandbox.
- The scheduler assigns pods to the virtual nodes inside the sandbox.
- Node scores are computed from the pod to the virtual node assignment wrt to configurable strategies. Ex: normalize price of assigned pod resources
- The node pool with the max score is chosen and cluster scaling advice is generated.
