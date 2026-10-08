# API Reference

## Packages
- [operator.config.sa.gardener.cloud/v1alpha1](#operatorconfigsagardenercloudv1alpha1)
- [sa.gardener.cloud/v1alpha1](#sagardenercloudv1alpha1)


## operator.config.sa.gardener.cloud/v1alpha1




#### ClientConnectionConfig



ClientConnectionConfig contains details for constructing a client.



_Appears in:_
- [OperatorConfig](#operatorconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kubeConfigPath` _string_ | KubeConfigPath is the path to kube-config. |  |  |
| `contentType` _string_ | ContentType is the content type used when sending data to the server from this client. |  |  |
| `acceptContentTypes` _string_ | AcceptContentTypes defines the Accept header sent by clients when connecting to the server,<br />overriding the default value of 'application/json'. This field will control all connections<br />to the server used by a particular client. |  |  |
| `burst` _integer_ | Burst allows extra queries to accumulate when a client is exceeding its rate. |  |  |
| `qps` _float_ | QPS controls the number of queries per second allowed for this connection. |  |  |


#### ControllersConfig



ControllersConfig defines the configuration for controllers that are run as part of the scaling-advisor.



_Appears in:_
- [OperatorConfig](#operatorconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `scalingConstraints` _[ScalingConstraintsControllerConfig](#scalingconstraintscontrollerconfig)_ | ScalingConstraints is the configuration for then controller that reconciles ScalingConstraints. |  |  |


#### LeaderElectionConfig



LeaderElectionConfig defines the configuration for the leader election.



_Appears in:_
- [OperatorConfig](#operatorconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `resourceLock` _string_ | ResourceLock determines which resource lock to use for leader election.<br />This is only applicable if leader election is enabled. |  |  |
| `resourceName` _string_ | ResourceName determines the name of the resource that leader election<br />will use for holding the leader lock.<br />This is only applicable if leader election is enabled. |  |  |
| `resourceNamespace` _string_ | ResourceNamespace determines the namespace in which the leader<br />election resource will be created.<br />This is only applicable if leader election is enabled. |  |  |
| `leaseDuration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | LeaseDuration is the duration that non-leader candidates will wait<br />after observing a leadership renewal until attempting to acquire<br />leadership of the occupied but un-renewed leader slot. This is effectively the<br />maximum duration that a leader can be stopped before it is replaced<br />by another candidate. This is only applicable if leader election is<br />enabled. |  |  |
| `renewDeadline` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | RenewDeadline is the interval between attempts by the acting leader to<br />renew its leadership before it stops leading. This must be less than or<br />equal to the lease duration.<br />This is only applicable if leader election is enabled. |  |  |
| `retryPeriod` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | RetryPeriod is the duration leader elector clients should wait<br />between attempting acquisition and renewal of leadership.<br />This is only applicable if leader election is enabled. |  |  |
| `enabled` _boolean_ | Enabled specifies whether leader election is enabled. Set this<br />to true when running replicated instances of the operator for high availability. |  |  |




#### ScalingAdviceGenerationConfig



ScalingAdviceGenerationConfig contains configuration for scaling advice generation.



_Appears in:_
- [OperatorConfig](#operatorconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `mode` _[ScalingAdviceGenerationMode](#scalingadvicegenerationmode)_ | Mode defines the mode in which scaling advice is generated. |  |  |
| `simulatorStrategy` _[SimulatorStrategy](#simulatorstrategy)_ | SimulatorStrategy defines the simulator strategy used by the ScaleOutSimulator implementation. |  |  |
| `scoringStrategy` _[NodeScoringStrategy](#nodescoringstrategy)_ | ScoringStrategy defines the node scoring strategy to use for scaling decisions. |  |  |


#### ScalingAdvisorServerConfig



ScalingAdvisorServerConfig is the configuration for Scaling Advisor server.



_Appears in:_
- [OperatorConfig](#operatorconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `healthProbeBindAddress` _string_ | HealthProbesBindAddress is the host and port for serving health probes. |  |  |
| `metricsBindAddress` _string_ | Metrics is the host and port for serving metrics. |  |  |
| `profilingBindAddress` _string_ | ProfilingBindAddress is the host and port for serving profiling data. |  |  |
| `profilingEnabled` _boolean_ | ProfilingEnable indicates whether profiling is enabled. |  |  |


#### ScalingConstraintsControllerConfig



ScalingConstraintsControllerConfig is the configuration for then controller that reconciles ScalingConstraints.



_Appears in:_
- [ControllersConfig](#controllersconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `concurrentSyncs` _integer_ | ConcurrentSyncs is the maximum number concurrent reconciliations that can be run for this controller. |  |  |



## sa.gardener.cloud/v1alpha1


### Resource Types
- [ScalingAdvice](#scalingadvice)
- [ScalingConstraint](#scalingconstraint)





#### NodePlacement



NodePlacement provides information about the placement of a node.



_Appears in:_
- [ScaleInItem](#scaleinitem)
- [ScaleOutItem](#scaleoutitem)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `poolName` _string_ | PoolName is the name of the node pool. |  |  |
| `templateName` _string_ | TemplateName is the name of the node template. |  |  |
| `instanceType` _string_ | InstanceType is the instance type of the Node |  |  |
| `region` _string_ | Region is the region of the instance |  |  |
| `availabilityZone` _string_ | AvailabilityZone is the availability zone of the node pool. |  |  |


#### NodePool



NodePool defines a node pool configuration for a cluster.



_Appears in:_
- [ScalingConstraintSpec](#scalingconstraintspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `labels` _object (keys:string, values:string)_ | Labels is a map of key/value pairs for labels applied to all the nodes in this node pool. |  |  |
| `annotations` _object (keys:string, values:string)_ | Annotations is a map of key/value pairs for annotations applied to all the nodes in this node pool. |  |  |
| `name` _string_ | Name is the name of the node pool. It must be unique within the cluster. |  |  |
| `region` _string_ | Region is the name of the region. |  |  |
| `taints` _[Taint](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#taint-v1-core) array_ | Taints is a list of taints applied to all the nodes in this node pool. |  |  |
| `availabilityZones` _string array_ | AvailabilityZones is a list of availability zones for the node pool. |  |  |
| `requirements` _[NodePoolRequirement](#nodepoolrequirement) array_ | Requirements encapsulates the slice of requirement selectors for this NodePool |  |  |
| `priority` _integer_ | Priority is the priority of the node pool. |  |  |


#### NodePoolRequirement



NodePoolRequirement is a requirement selector that encapsulates values, a key, and an operator
that relates the key and values.



_Appears in:_
- [NodePool](#nodepool)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `key` _string_ | Key is the label key that the selector applies to. |  |  |
| `operator` _[NodePoolRequirementOperator](#nodepoolrequirementoperator)_ | Operator represents a key's relationship to a set of values.<br />Valid operators are In, NotIn, Exists, DoesNotExist. Gt, and Lt. |  |  |
| `values` _string array_ | Values is an array of string values. If the operator is "In" or "NotIn",<br />the values array must be non-empty. If the operator is "Exists" or "DoesNotExist",<br />the values array must be empty. If the operator is "Gt" or "Lt", the values<br />array must have a single element, which will be interpreted as an integer.<br />This array is replaced during a strategic merge patch. |  |  |
| `priority` _integer_ | Priority represents the priority of this requirement. Higher values have greater priority. |  |  |


#### NodePoolRequirementOperator

_Underlying type:_ _string_

NodePoolRequirementOperator is the set of operators that can be used in a [NodePoolRequirement]



_Appears in:_
- [NodePoolRequirement](#nodepoolrequirement)

| Field | Description |
| --- | --- |
| `In` | NodePoolRequirementOpIn is the enum constant for the "In" operator used within a [NodePoolRequirement].<br /> |
| `NotIn` | NodePoolRequirementOpNotIn is the enum constant for the "NotIn" operator used within a [NodePoolRequirement].<br /> |
| `Exists` | NodePoolRequirementOpExists is the enum constant for the "Exist" operator used within a [NodePoolRequirement].<br /> |
| `DoesNotExist` | NodePoolRequirementOpDoesNotExist is the enum constant for the "DoesNotExist" operator used within a [NodePoolRequirement].<br /> |
| `Gt` | NodePoolRequirementOpGt is the enum constant for the "Gt" operator used within a [NodePoolRequirement].<br /> |
| `Lt` | NodePoolRequirementOpLt is the enum constant for the "Lt" operator used within a [NodePoolRequirement].<br /> |


#### NodeTemplate



NodeTemplate defines a node template configuration for an instance type.
There can be different NodeTemplate's for a [ScalingConstraintSpec] for the same instance type.
This is permitted to allow the opportunity for different SystemReserved.



_Appears in:_
- [ScalingConstraintSpec](#scalingconstraintspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the node template. Name is unique within a particular [ScalingConstraintSpec] |  |  |
| `architecture` _string_ | Architecture is the architecture of the instance type. |  |  |
| `instanceType` _string_ | InstanceType is the instance type of the node template. |  |  |
| `maxVolumes` _integer_ | MaxVolumes is the max number of volumes that can be attached to a node of this instance type. |  |  |


#### ScaleInFeedback



ScaleInFeedback is  the feedback from the life cycle manager after applying [ScaleInPlan]



_Appears in:_
- [ScalingFeedback](#scalingfeedback)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `acceptedNodeNames` _string array_ | AcceptedNodeNames holds the slice of node names that were accepted for scale-in by the lifecycle controller.<br />Required to be specified, since if empty, the ScaleInFeedback itself should not be populated. |  |  |


#### ScaleInItem



ScaleInItem is the unit of scaling-in advice for a specific node.



_Appears in:_
- [ScaleInPlan](#scaleinplan)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `poolName` _string_ | PoolName is the name of the node pool. |  |  |
| `templateName` _string_ | TemplateName is the name of the node template. |  |  |
| `instanceType` _string_ | InstanceType is the instance type of the Node |  |  |
| `region` _string_ | Region is the region of the instance |  |  |
| `availabilityZone` _string_ | AvailabilityZone is the availability zone of the node pool. |  |  |
| `nodeName` _string_ | NodeName is the name of the node to be scaled in. |  |  |


#### ScaleInPlan



ScaleInPlan is the plan for scaling in a node pool and/or targeted set of nodes.



_Appears in:_
- [ScalingAdviceSpec](#scalingadvicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `items` _[ScaleInItem](#scaleinitem) array_ | Items is the slice of scaling-in advice for a node pool. |  |  |


#### ScaleOutFeedback



ScaleOutFeedback is the feedback from the life cycle manager when applying an [ScaleOutPlan] of a [ScalingAdviceSpec]



_Appears in:_
- [ScalingFeedback](#scalingfeedback)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `items` _[ScaleOutItemFeedback](#scaleoutitemfeedback) array_ |  |  |  |


#### ScaleOutItem



ScaleOutItem is the unit of scaling advice for a node pool.



_Appears in:_
- [ScaleOutPlan](#scaleoutplan)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `poolName` _string_ | PoolName is the name of the node pool. |  |  |
| `templateName` _string_ | TemplateName is the name of the node template. |  |  |
| `instanceType` _string_ | InstanceType is the instance type of the Node |  |  |
| `region` _string_ | Region is the region of the instance |  |  |
| `availabilityZone` _string_ | AvailabilityZone is the availability zone of the node pool. |  |  |
| `currentReplicas` _integer_ | CurrentReplicas is the current number of replicas for the NodePlacement. |  |  |
| `delta` _integer_ | Delta is the delta change in the number of nodes for the NodePlacement. |  |  |


#### ScaleOutItemFeedback



ScaleOutItemFeedback is the feedback from the life cycle manager when applying an individual [ScaleOutItem]



_Appears in:_
- [ScaleOutFeedback](#scaleoutfeedback)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `creationDeadline` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | CreationDeadline represents the time after which the scaling-advisor can expect real nodes to be created and available<br />for the corresponding [ScaleOutItem]'s [NodePlacement]. When the [ScalingFeedback] is constructed by the life-cycle manager,<br />this field is mandatory to be set inside all [ScaleOutItemFeedback] |  |  |
| `backoffUntil` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | BackoffUntil if populated, represents the time until the scaling-advisor will not consider the corresponding<br />[ScaleOutItem]'s [NodePlacement] when running simulations and generating subsequent [ScaleOutPlan]'s |  |  |
| `errorType` _[ScalingErrorType](#scalingerrortype)_ | ErrorType is the type of error that occurred during scale-out. |  |  |
| `index` _integer_ | Index represents the item index in [ScaleOutPlan.Items] |  |  |
| `failCount` _integer_ | FailCount is the number of nodes that have failed creation. |  |  |


#### ScaleOutPlan



ScaleOutPlan is the plan for scaling out a node pool.



_Appears in:_
- [ScalingAdviceSpec](#scalingadvicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `unsatisfiedPodNames` _string array_ | UnsatisfiedPodNames is the list of all pods (namespace/name) that could not be satisfied by the scale out plan. |  |  |
| `items` _[ScaleOutItem](#scaleoutitem) array_ | Items is the slice of scaling-out advice for a node pool. |  |  |


#### ScalingAdvice



ScalingAdvice is the schema to define cluster scaling advice for a cluster.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `sa.gardener.cloud/v1alpha1` | | |
| `kind` _string_ | `ScalingAdvice` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ScalingAdviceSpec](#scalingadvicespec)_ | Spec defines the specification of ScalingAdvice. |  |  |
| `status` _[ScalingAdviceStatus](#scalingadvicestatus)_ | Status defines the status of ScalingAdvice. |  |  |


#### ScalingAdviceDiagnostic



ScalingAdviceDiagnostic provides diagnostics information for the scaling advice.



_Appears in:_
- [ScalingAdviceStatus](#scalingadvicestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `traceLogName` _string_ | TraceLogName is the name of the trace log. This can be used to fetch the trace log from the scaling advisor core. |  |  |
| `simRunResults` _[ScalingSimRunResult](#scalingsimrunresult) array_ | SimRunResults is the list of simulation run results for the scaling advice. |  |  |


#### ScalingAdviceSpec



ScalingAdviceSpec defines the desired state of ScalingAdvice.



_Appears in:_
- [ScalingAdvice](#scalingadvice)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `scaleOut` _[ScaleOutPlan](#scaleoutplan)_ | ScaleOut is the plan for scaling out across node pools. |  |  |
| `scaleIn` _[ScaleInPlan](#scaleinplan)_ | ScaleIn is the plan for scaling in across node pools. |  |  |
| `constraintRef` _[NamespacedName](#namespacedname)_ | ConstraintRef is a reference to the ScalingConstraint that this advice is based on. |  |  |


#### ScalingAdviceStatus



ScalingAdviceStatus defines the observed state of ScalingAdvice.



_Appears in:_
- [ScalingAdvice](#scalingadvice)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `diagnostic` _[ScalingAdviceDiagnostic](#scalingadvicediagnostic)_ | Diagnostic provides diagnostics information for the scaling advice.<br />This is only set by the scaling advisor controller if the constants.AnnotationEnableScalingDiagnostics annotation is<br />set on the corresponding ScalingConstraint resource. |  |  |
| `feedback` _[ScalingFeedback](#scalingfeedback)_ | Feedback represents the [ScalingFeedback] from the lifecycle manager applying the [ScalingAdvice] |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represents additional information |  |  |


#### ScalingConstraint



ScalingConstraint is a schema to define constraints that will be used to create cluster scaling advises for a cluster.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `sa.gardener.cloud/v1alpha1` | | |
| `kind` _string_ | `ScalingConstraint` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ScalingConstraintSpec](#scalingconstraintspec)_ | Spec defines the specification of the ScalingConstraint. |  |  |
| `status` _[ScalingConstraintStatus](#scalingconstraintstatus)_ | Status defines the status of the ScalingConstraint. |  |  |


#### ScalingConstraintSpec



ScalingConstraintSpec defines the specification of the ScalingConstraint.



_Appears in:_
- [ScalingConstraint](#scalingconstraint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodePools` _[NodePool](#nodepool) array_ | NodePools is the list of node pools to choose from when creating scaling advice. |  |  |
| `nodeTemplates` _[NodeTemplate](#nodetemplate) array_ | NodeTemplates is the slice of all NodeTemplates that can be used for selecting instances associated with each NodePool. |  |  |


#### ScalingConstraintStatus



ScalingConstraintStatus defines the observed state of ScalingConstraint.



_Appears in:_
- [ScalingConstraint](#scalingconstraint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions contains the conditions for the ScalingConstraint. |  |  |


#### ScalingErrorType

_Underlying type:_ _string_

ScalingErrorType defines the type of scaling error.



_Appears in:_
- [ScaleOutItemFeedback](#scaleoutitemfeedback)

| Field | Description |
| --- | --- |
| `ResourceExhaustedError` | ScalingErrorTypeResourceExhausted indicates that the lifecycle manager could not create the instance due to resource exhaustion for an instance type in an availability zone.<br /> |
| `CreationTimeoutError` | ScalingErrorTypeCreationTimeout indicates that the lifecycle manager could not create the instance within its configured timeout despite multiple attempts.<br /> |


#### ScalingFeedback



ScalingFeedback provides scale-in and scale-out feedback from the lifecycle manager.
Scaling advisor can refine its future scaling advice based on this feedback.



_Appears in:_
- [ScalingAdviceStatus](#scalingadvicestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `scaleOut` _[ScaleOutFeedback](#scaleoutfeedback)_ | ScaleOut is the scale-out feedback from the lifecycle manager when applying [ScaleOutPlan]<br />[ScalingAdviceSpec]. |  |  |
| `scaleIn` _[ScaleInFeedback](#scaleinfeedback)_ | ScaleIn is the scale-in feedback from life-cycle manager when applying [ScaleInPlan] |  |  |


#### ScalingSimRunResult



ScalingSimRunResult is the result of a simulation run in the scaling advisor.



_Appears in:_
- [ScalingAdviceDiagnostic](#scalingadvicediagnostic)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodePoolName` _string_ | NodePoolName is the name of the node pool. |  |  |
| `nodeTemplateName` _string_ | NodeTemplateName is the name of the node template. |  |  |
| `availabilityZone` _string_ | AvailabilityZone is the availability zone of the node pool. |  |  |
| `scheduledPodNames` _string array_ | ScheduledPodNames is the list of pod names that were scheduled in this simulation run. |  |  |
| `nodeScore` _integer_ | NodeScore is the score of the node in the simulation run. |  |  |
| `numUnscheduledPods` _integer_ | NumUnscheduledPods is the number of pods that could not be scheduled in this simulation run. |  |  |


