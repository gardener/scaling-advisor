# API Reference

## Packages
- [operator.config.sa.gardener.cloud/v1alpha1](#operatorconfigsagardenercloudv1alpha1)
- [sa.gardener.cloud/v1alpha1](#sagardenercloudv1alpha1)


## operator.config.sa.gardener.cloud/v1alpha1




#### ClientConnectionConfiguration



ClientConnectionConfiguration contains details for constructing a client.



_Appears in:_
- [ScalingAdvisorConfiguration](#scalingadvisorconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `qps` _float_ | QPS controls the number of queries per second allowed for this connection. |  |  |
| `burst` _integer_ | Burst allows extra queries to accumulate when a client is exceeding its rate. |  |  |
| `contentType` _string_ | ContentType is the content type used when sending data to the server from this client. |  |  |
| `acceptContentTypes` _string_ | AcceptContentTypes defines the Accept header sent by clients when connecting to the server,<br />overriding the default value of 'application/json'. This field will control all connections<br />to the server used by a particular client. |  |  |


#### ControllersConfiguration



ControllersConfiguration defines the configuration for controllers that are run as part of the scaling-advisor.



_Appears in:_
- [ScalingAdvisorConfiguration](#scalingadvisorconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `scalingConstraints` _[ScalingConstraintsControllerConfiguration](#scalingconstraintscontrollerconfiguration)_ | ScalingConstraints is the configuration for then controller that reconciles ScalingConstraints. |  |  |


#### LeaderElectionConfiguration



LeaderElectionConfiguration defines the configuration for the leader election.



_Appears in:_
- [ScalingAdvisorConfiguration](#scalingadvisorconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled specifies whether leader election is enabled. Set this<br />to true when running replicated instances of the operator for high availability. |  |  |
| `leaseDuration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | LeaseDuration is the duration that non-leader candidates will wait<br />after observing a leadership renewal until attempting to acquire<br />leadership of the occupied but un-renewed leader slot. This is effectively the<br />maximum duration that a leader can be stopped before it is replaced<br />by another candidate. This is only applicable if leader election is<br />enabled. |  |  |
| `renewDeadline` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | RenewDeadline is the interval between attempts by the acting leader to<br />renew its leadership before it stops leading. This must be less than or<br />equal to the lease duration.<br />This is only applicable if leader election is enabled. |  |  |
| `retryPeriod` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | RetryPeriod is the duration leader elector clients should wait<br />between attempting acquisition and renewal of leadership.<br />This is only applicable if leader election is enabled. |  |  |
| `resourceLock` _string_ | ResourceLock determines which resource lock to use for leader election.<br />This is only applicable if leader election is enabled. |  |  |
| `resourceName` _string_ | ResourceName determines the name of the resource that leader election<br />will use for holding the leader lock.<br />This is only applicable if leader election is enabled. |  |  |
| `resourceNamespace` _string_ | ResourceNamespace determines the namespace in which the leader<br />election resource will be created.<br />This is only applicable if leader election is enabled. |  |  |




#### ScalingAdvisorServerConfiguration



ScalingAdvisorServerConfiguration is the configuration for Scaling Advisor server.



_Appears in:_
- [ScalingAdvisorConfiguration](#scalingadvisorconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `host` _string_ | Host is the IP address on which to listen for the specified port. |  |  |
| `port` _integer_ | Port is the port on which to serve requests. |  |  |
| `kubeConfigPath` _string_ | KubeConfigPath is the path to master kube-config. |  |  |
| `profilingEnabled` _boolean_ | ProfilingEnabled indicates whether this service should register the standard pprof HTTP handlers: /debug/pprof/* |  |  |
| `gracefulShutdownTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | GracefulShutdownTimeout is the time given to the service to gracefully shutdown. |  |  |
| `healthProbes` _[HostPort](#hostport)_ | HealthProbes is the host and port for serving the healthz and readyz endpoints. |  |  |
| `metrics` _[HostPort](#hostport)_ | Metrics is the host and port for serving the metrics endpoint. |  |  |
| `profiling` _[HostPort](#hostport)_ | Profiling is the host and port for serving the profiling endpoints. |  |  |


#### ScalingConstraintsControllerConfiguration



ScalingConstraintsControllerConfiguration is the configuration for then controller that reconciles ScalingConstraints.



_Appears in:_
- [ControllersConfiguration](#controllersconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `concurrentSyncs` _integer_ | ConcurrentSyncs is the maximum number concurrent reconciliations that can be run for this controller. |  |  |
| `scoringStrategy` _[NodeScoringStrategy](#nodescoringstrategy)_ | ScoringStrategy defines the node scoring strategy to use for scaling decisions. |  |  |
| `cloudProvider` _[CloudProvider](#cloudprovider)_ | CloudProvider specifies the cloud provider for which the scaling advisor is configured. |  |  |



## sa.gardener.cloud/v1alpha1


### Resource Types
- [ClusterScalingAdvice](#clusterscalingadvice)
- [ClusterScalingConstraint](#clusterscalingconstraint)
- [ClusterScalingFeedback](#clusterscalingfeedback)



#### BackoffPolicy



BackoffPolicy defines the backoff policy to be used when backing off from suggesting an instance type + zone in subsequence scaling advice upon failed scaling operation.



_Appears in:_
- [ClusterScalingConstraintSpec](#clusterscalingconstraintspec)
- [NodePool](#nodepool)

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
| `constraintRef` _[ConstraintReference](#constraintreference)_ | ConstraintRef is a reference to the ClusterScalingConstraint that this advice is based on. |  |  |
| `scaleOutPlan` _[ScaleOutPlan](#scaleoutplan)_ | ScaleOutPlan is the plan for scaling out across node pools. |  |  |
| `scaleInPlan` _[ScaleInPlan](#scaleinplan)_ | ScaleInPlan is the plan for scaling in across node pools. |  |  |


#### ClusterScalingAdviceStatus



ClusterScalingAdviceStatus defines the observed state of ClusterScalingAdvice.



_Appears in:_
- [ClusterScalingAdvice](#clusterscalingadvice)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `diagnostic` _[ScalingAdviceDiagnostic](#scalingadvicediagnostic)_ | Diagnostic provides diagnostics information for the scaling advice.<br />This is only set by the scaling advisor controller if the constants.AnnotationEnableScalingDiagnostics annotation is<br />set on the corresponding ClusterScalingConstraint resource. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represents additional information |  |  |


#### ClusterScalingConstraint



ClusterScalingConstraint is a schema to define constraints that will be used to create cluster scaling advises for a cluster.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `sa.gardener.cloud/v1alpha1` | | |
| `kind` _string_ | `ClusterScalingConstraint` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ClusterScalingConstraintSpec](#clusterscalingconstraintspec)_ | Spec defines the specification of the ClusterScalingConstraint. |  |  |
| `status` _[ClusterScalingConstraintStatus](#clusterscalingconstraintstatus)_ | Status defines the status of the ClusterScalingConstraint. |  |  |


#### ClusterScalingConstraintSpec



ClusterScalingConstraintSpec defines the specification of the ClusterScalingConstraint.



_Appears in:_
- [ClusterScalingConstraint](#clusterscalingconstraint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `consumerID` _string_ | ConsumerID is the ID of the consumer who creates the scaling constraint and is the target for cluster scaling advises.<br />It allows a consumer to accept or reject the advises by checking the ConsumerID for which the scaling advice has been created. |  |  |
| `adviceGenerationMode` _[ScalingAdviceGenerationMode](#scalingadvicegenerationmode)_ | AdviceGenerationMode defines the mode in which scaling advice is generated. |  |  |
| `nodePools` _[NodePool](#nodepool) array_ | NodePools is the list of node pools to choose from when creating scaling advice. |  |  |
| `defaultBackoffPolicy` _[BackoffPolicy](#backoffpolicy)_ | DefaultBackoffPolicy defines a default backoff policy for all NodePools of a cluster. Backoff policy can be overridden at the NodePool level. |  |  |
| `scaleInPolicy` _[ScaleInPolicy](#scaleinpolicy)_ | ScaleInPolicy defines the default scale in policy to be used when scaling in a node pool. |  |  |


#### ClusterScalingConstraintStatus



ClusterScalingConstraintStatus defines the observed state of ClusterScalingConstraint.



_Appears in:_
- [ClusterScalingConstraint](#clusterscalingconstraint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions contains the conditions for the ClusterScalingConstraint. |  |  |


#### ClusterScalingFeedback



ClusterScalingFeedback provides scale-in and scale-out error feedback from the lifecycle manager.
Scaling advisor can refine its future scaling advice based on this feedback.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `sa.gardener.cloud/v1alpha1` | | |
| `kind` _string_ | `ClusterScalingFeedback` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ClusterScalingFeedbackSpec](#clusterscalingfeedbackspec)_ | Spec defines the specification of ClusterScalingFeedback. |  |  |


#### ClusterScalingFeedbackSpec



ClusterScalingFeedbackSpec defines the specification of the ClusterScalingFeedback.



_Appears in:_
- [ClusterScalingFeedback](#clusterscalingfeedback)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `constraintRef` _[ConstraintReference](#constraintreference)_ | ConstraintRef is a reference to the ClusterScalingConstraint that this advice is based on. |  |  |
| `scaleOutErrorInfos` _[ScaleOutErrorInfo](#scaleouterrorinfo) array_ | ScaleOutErrorInfos is the list of scale-out errors for the scaling advice. |  |  |
| `scaleInErrorInfo` _[ScaleInErrorInfo](#scaleinerrorinfo)_ | ScaleInErrorInfo is the scale-in error information for the scaling advice. |  |  |




#### NodePlacement



NodePlacement provides information about the placement of a node.



_Appears in:_
- [ScaleInItem](#scaleinitem)
- [ScaleOutItem](#scaleoutitem)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodePoolName` _string_ | NodePoolName is the name of the node pool. |  |  |
| `nodeTemplateName` _string_ | NodeTemplateName is the name of the node template. |  |  |
| `instanceType` _string_ | InstanceType is the instance type of the Node. |  |  |
| `region` _string_ | Region is the region of the instance |  |  |
| `availabilityZone` _string_ | AvailabilityZone is the availability zone of the node pool. |  |  |


#### NodePool



NodePool defines a node pool configuration for a cluster.



_Appears in:_
- [ClusterScalingConstraintSpec](#clusterscalingconstraintspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the node pool. It must be unique within the cluster. |  |  |
| `region` _string_ | Region is the name of the region. |  |  |
| `priority` _integer_ | Priority is the priority of the node pool. |  |  |
| `labels` _object (keys:string, values:string)_ | Labels is a map of key/value pairs for labels applied to all the nodes in this node pool. |  |  |
| `annotations` _object (keys:string, values:string)_ | Annotations is a map of key/value pairs for annotations applied to all the nodes in this node pool. |  |  |
| `taints` _[Taint](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#taint-v1-core) array_ | Taints is a list of taints applied to all the nodes in this node pool. |  |  |
| `availabilityZones` _string array_ | AvailabilityZones is a list of availability zones for the node pool. |  |  |
| `nodeTemplates` _[NodeTemplate](#nodetemplate) array_ | NodeTemplates is a slice of NodeTemplate. |  |  |
| `scaleInPolicy` _[ScaleInPolicy](#scaleinpolicy)_ | ScaleInPolicy defines the scale in policy for this node pool. |  |  |
| `defaultBackoffPolicy` _[BackoffPolicy](#backoffpolicy)_ | BackoffPolicy defines the backoff policy applicable to resource exhaustion of any instance type + zone combination in this node pool. |  |  |


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
- [ClusterScalingFeedbackSpec](#clusterscalingfeedbackspec)

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
| `instanceType` _string_ | InstanceType is the instance type of the Node. |  |  |
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
- [ClusterScalingConstraintSpec](#clusterscalingconstraintspec)
- [NodePool](#nodepool)



#### ScaleOutErrorInfo



ScaleOutErrorInfo is the backoff information for each instance type + zone.



_Appears in:_
- [ClusterScalingFeedbackSpec](#clusterscalingfeedbackspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `availabilityZone` _string_ | AvailabilityZone is the availability zone of the node pool. |  |  |
| `instanceType` _string_ | InstanceType is the instance type of the node pool. |  |  |
| `failCount` _integer_ | FailCount is the number of nodes that have failed creation. |  |  |
| `errorType` _[ScalingErrorType](#scalingerrortype)_ | ErrorType is the type of error that occurred during scale-out. |  |  |


#### ScaleOutItem



ScaleOutItem is the unit of scaling advice for a node pool.



_Appears in:_
- [ScaleOutPlan](#scaleoutplan)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodePoolName` _string_ | NodePoolName is the name of the node pool. |  |  |
| `nodeTemplateName` _string_ | NodeTemplateName is the name of the node template. |  |  |
| `instanceType` _string_ | InstanceType is the instance type of the Node. |  |  |
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
| `simRunResults` _[ScalingSimRunResult](#scalingsimrunresult) array_ | SimRunResults is the list of simulation run results for the scaling advice. |  |  |
| `traceLogURL` _string_ | TraceLogURL is the URL to the transient trace log for the scaling simulation run. |  |  |


#### ScalingAdviceGenerationMode

_Underlying type:_ _string_

ScalingAdviceGenerationMode defines the mode in which scaling advice is generated.



_Appears in:_
- [ClusterScalingConstraintSpec](#clusterscalingconstraintspec)



#### ScalingErrorType

_Underlying type:_ _string_

ScalingErrorType defines the type of scaling error.



_Appears in:_
- [ScaleOutErrorInfo](#scaleouterrorinfo)

| Field | Description |
| --- | --- |
| `ResourceExhaustedError` | ErrorTypeResourceExhausted indicates that the lifecycle manager could not create the instance due to resource exhaustion for an instance type in an availability zone.<br /> |
| `CreationTimeoutError` | ErrorTypeCreationTimeout indicates that the lifecycle manager could not create the instance within its configured timeout despite multiple attempts.<br /> |


#### ScalingSimRunResult



ScalingSimRunResult is the result of a simulation run in the scaling advisor.



_Appears in:_
- [ScalingAdviceDiagnostic](#scalingadvicediagnostic)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodePoolName` _string_ | NodePoolName is the name of the node pool. |  |  |
| `nodeTemplateName` _string_ | NodeTemplateName is the name of the node template. |  |  |
| `availabilityZone` _string_ | AvailabilityZone is the availability zone of the node pool. |  |  |
| `nodeScore` _integer_ | NodeScore is the score of the node in the simulation run. |  |  |
| `scheduledPodNames` _string array_ | ScheduledPodNames is the list of pod names that were scheduled in this simulation run. |  |  |
| `numUnscheduledPods` _integer_ | NumUnscheduledPods is the number of pods that could not be scheduled in this simulation run. |  |  |


