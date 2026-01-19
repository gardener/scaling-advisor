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
| `simulationStrategy` _[SimulationStrategy](#simulationstrategy)_ | SimulationStrategy defines the simulation strategy to be used for scaling virtual nodes for generation of scaling advice. |  |  |
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
- [ClusterScalingAdvice](#clusterscalingadvice)
- [ScalingConstraint](#scalingconstraint)
- [ScalingFeedback](#scalingfeedback)



#### BackoffPolicy



BackoffPolicy defines the backoff policy to be used when backing off from suggesting an instance type + zone in subsequence scaling advice upon failed scaling operation.



_Appears in:_
- [NodePool](#nodepool)
- [ScalingConstraintSpec](#scalingconstraintspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `initialBackoff` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | InitialBackoffDuration defines the lower limit of the backoff duration. |  |  |
| `maxBackoff` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | MaxBackoffDuration defines the upper limit of the backoff duration. |  |  |


#### ClusterScalingAdvice



ClusterScalingAdvice is the schema to define cluster scaling advice for a cluster.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `sa.gardener.cloud/v1alpha1` | | |
| `kind` _string_ | `ClusterScalingAdvice` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ClusterScalingAdviceSpec](#clusterscalingadvicespec)_ | Spec defines the specification of ClusterScalingAdvice. |  |  |
| `status` _[ClusterScalingAdviceStatus](#clusterscalingadvicestatus)_ | Status defines the status of ClusterScalingAdvice. |  |  |


#### ClusterScalingAdviceSpec



ClusterScalingAdviceSpec defines the desired state of ClusterScalingAdvice.



_Appears in:_
- [ClusterScalingAdvice](#clusterscalingadvice)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `scaleOutPlan` _[ScaleOutPlan](#scaleoutplan)_ | ScaleOutPlan is the plan for scaling out across node pools. |  |  |
| `scaleInPlan` _[ScaleInPlan](#scaleinplan)_ | ScaleInPlan is the plan for scaling in across node pools. |  |  |
| `constraintRef` _[ConstraintReference](#constraintreference)_ | ConstraintRef is a reference to the ScalingConstraint that this advice is based on. |  |  |


#### ClusterScalingAdviceStatus



ClusterScalingAdviceStatus defines the observed state of ClusterScalingAdvice.



_Appears in:_
- [ClusterScalingAdvice](#clusterscalingadvice)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `diagnostic` _[ScalingAdviceDiagnostic](#scalingadvicediagnostic)_ | Diagnostic provides diagnostics information for the scaling advice.<br />This is only set by the scaling advisor controller if the constants.AnnotationEnableScalingDiagnostics annotation is<br />set on the corresponding ScalingConstraint resource. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represents additional information |  |  |




#### NodePlacement



NodePlacement provides information about the placement of a node.



_Appears in:_
- [ScaleInItem](#scaleinitem)
- [ScaleOutItem](#scaleoutitem)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodePoolName` _string_ | NodePoolName is the name of the node pool. |  |  |
| `nodeTemplateName` _string_ | NodeTemplateName is the name of the node template. |  |  |
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
| `scaleInPolicy` _[ScaleInPolicy](#scaleinpolicy)_ | ScaleInPolicy defines the scale in policy for this node pool. |  |  |
| `defaultBackoffPolicy` _[BackoffPolicy](#backoffpolicy)_ | BackoffPolicy defines the backoff policy applicable to resource exhaustion of any instance type + zone combination in this node pool. |  |  |
| `name` _string_ | Name is the name of the node pool. It must be unique within the cluster. |  |  |
| `region` _string_ | Region is the name of the region. |  |  |
| `taints` _[Taint](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#taint-v1-core) array_ | Taints is a list of taints applied to all the nodes in this node pool. |  |  |
| `availabilityZones` _string array_ | AvailabilityZones is a list of availability zones for the node pool. |  |  |
| `nodeTemplates` _[NodeTemplate](#nodetemplate) array_ | NodeTemplates is a slice of NodeTemplate. |  |  |
| `priority` _integer_ | Priority is the priority of the node pool. |  |  |


#### NodeTemplate



NodeTemplate defines a node template configuration for an instance type.
All nodes of a certain instance type in a node pool will be created using this template.



_Appears in:_
- [NodePool](#nodepool)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the node template. |  |  |
| `architecture` _string_ | Architecture is the architecture of the instance type. |  |  |
| `instanceType` _string_ | InstanceType is the instance type of the node template. |  |  |
| `priority` _integer_ | Priority is the priority of the node template. The lower the number, the higher the priority. |  |  |
| `maxVolumes` _integer_ | MaxVolumes is the max number of volumes that can be attached to a node of this instance type. |  |  |


#### ScaleInErrorInfo



ScaleInErrorInfo is the information about nodes that could not be deleted for scale-in.



_Appears in:_
- [ScalingFeedbackSpec](#scalingfeedbackspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeNames` _string array_ | NodeNames is the list of node names that could not be deleted for scaled in. |  |  |


#### ScaleInItem



ScaleInItem is the unit of scaling-in advice for a specific node.



_Appears in:_
- [ScaleInPlan](#scaleinplan)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodePoolName` _string_ | NodePoolName is the name of the node pool. |  |  |
| `nodeTemplateName` _string_ | NodeTemplateName is the name of the node template. |  |  |
| `instanceType` _string_ | InstanceType is the instance type of the Node |  |  |
| `region` _string_ | Region is the region of the instance |  |  |
| `availabilityZone` _string_ | AvailabilityZone is the availability zone of the node pool. |  |  |
| `nodeName` _string_ | NodeName is the name of the node to be scaled in. |  |  |


#### ScaleInPlan



ScaleInPlan is the plan for scaling in a node pool and/or targeted set of nodes.



_Appears in:_
- [ClusterScalingAdviceSpec](#clusterscalingadvicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `items` _[ScaleInItem](#scaleinitem) array_ | Items is the slice of scaling-in advice for a node pool. |  |  |


#### ScaleInPolicy



ScaleInPolicy defines the scale in policy to be used when scaling in a node pool.



_Appears in:_
- [NodePool](#nodepool)
- [ScalingConstraintSpec](#scalingconstraintspec)



#### ScaleOutErrorInfo



ScaleOutErrorInfo is the backoff information for each instance type + zone.



_Appears in:_
- [ScalingFeedbackSpec](#scalingfeedbackspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `availabilityZone` _string_ | AvailabilityZone is the availability zone of the node pool. |  |  |
| `instanceType` _string_ | InstanceType is the instance type of the node pool. |  |  |
| `errorType` _[ScalingErrorType](#scalingerrortype)_ | ErrorType is the type of error that occurred during scale-out. |  |  |
| `failCount` _integer_ | FailCount is the number of nodes that have failed creation. |  |  |


#### ScaleOutItem



ScaleOutItem is the unit of scaling advice for a node pool.



_Appears in:_
- [ScaleOutPlan](#scaleoutplan)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodePoolName` _string_ | NodePoolName is the name of the node pool. |  |  |
| `nodeTemplateName` _string_ | NodeTemplateName is the name of the node template. |  |  |
| `instanceType` _string_ | InstanceType is the instance type of the Node |  |  |
| `region` _string_ | Region is the region of the instance |  |  |
| `availabilityZone` _string_ | AvailabilityZone is the availability zone of the node pool. |  |  |
| `currentReplicas` _integer_ | CurrentReplicas is the current number of replicas for the NodePlacement. |  |  |
| `delta` _integer_ | Delta is the delta change in the number of nodes for the NodePlacement. |  |  |


#### ScaleOutPlan



ScaleOutPlan is the plan for scaling out a node pool.



_Appears in:_
- [ClusterScalingAdviceSpec](#clusterscalingadvicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `unsatisfiedPodNames` _string array_ | UnsatisfiedPodNames is the list of all pods (namespace/name) that could not be satisfied by the scale out plan. |  |  |
| `Items` _[ScaleOutItem](#scaleoutitem) array_ | Items is the slice of scaling-out advice for a node pool. |  |  |


#### ScalingAdviceDiagnostic



ScalingAdviceDiagnostic provides diagnostics information for the scaling advice.



_Appears in:_
- [ClusterScalingAdviceStatus](#clusterscalingadvicestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `traceLogName` _string_ | TraceLogName is the name of the trace log. This can be used to fetch the trace log from the scaling advisor core. |  |  |
| `simRunResults` _[ScalingSimRunResult](#scalingsimrunresult) array_ | SimRunResults is the list of simulation run results for the scaling advice. |  |  |


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
| `defaultBackoffPolicy` _[BackoffPolicy](#backoffpolicy)_ | DefaultBackoffPolicy defines a default backoff policy for all NodePools of a cluster. Backoff policy can be overridden at the NodePool level. |  |  |
| `scaleInPolicy` _[ScaleInPolicy](#scaleinpolicy)_ | ScaleInPolicy defines the default scale in policy to be used when scaling in a node pool. |  |  |
| `consumerID` _string_ | ConsumerID is the Name of the consumer who creates the scaling constraint and is the target for cluster scaling advises.<br />It allows a consumer to accept or reject the advises by checking the ConsumerID for which the scaling advice has been created. |  |  |
| `nodePools` _[NodePool](#nodepool) array_ | NodePools is the list of node pools to choose from when creating scaling advice. |  |  |


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
- [ScaleOutErrorInfo](#scaleouterrorinfo)

| Field | Description |
| --- | --- |
| `ResourceExhaustedError` | ScalingErrorTypeResourceExhausted indicates that the lifecycle manager could not create the instance due to resource exhaustion for an instance type in an availability zone.<br /> |
| `CreationTimeoutError` | ScalingErrorTypeCreationTimeout indicates that the lifecycle manager could not create the instance within its configured timeout despite multiple attempts.<br /> |


#### ScalingFeedback



ScalingFeedback provides scale-in and scale-out error feedback from the lifecycle manager.
Scaling advisor can refine its future scaling advice based on this feedback.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `sa.gardener.cloud/v1alpha1` | | |
| `kind` _string_ | `ScalingFeedback` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ScalingFeedbackSpec](#scalingfeedbackspec)_ | Spec defines the specification of ScalingFeedback. |  |  |


#### ScalingFeedbackSpec



ScalingFeedbackSpec defines the specification of the ScalingFeedback.



_Appears in:_
- [ScalingFeedback](#scalingfeedback)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `constraintRef` _[ConstraintReference](#constraintreference)_ | ConstraintRef is a reference to the ScalingConstraint that this advice is based on. |  |  |
| `scaleOutErrorInfos` _[ScaleOutErrorInfo](#scaleouterrorinfo) array_ | ScaleOutErrorInfos is the list of scale-out errors for the scaling advice. |  |  |
| `scaleInErrorInfo` _[ScaleInErrorInfo](#scaleinerrorinfo)_ | ScaleInErrorInfo is the scale-in error information for the scaling advice. |  |  |


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


