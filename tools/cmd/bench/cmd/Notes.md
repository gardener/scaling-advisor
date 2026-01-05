# Notes and TODOs

## Karpenter (Action Items)

- The below might be useful for incorporation of scalingfeedback backoff.
> If the provider API (e.g. EC2 Fleet’s API) indicates capacity is unavailable, Karpenter caches that result across all attempts to provision EC2 capacity for that instance type and zone for the next 3 minutes.
- Karpenter provisioning is highly parallel. Because of this, limit checking is eventually consistent, which can result in overrun during rapid scale outs.
- [GCP nodePool and nodeClass example](https://github.com/cloudpilot-ai/karpenter-provider-gcp/tree/main/charts#2-create-nodepool-and-nodeclass)
- [How does it dynamically select instance type](https://karpenter.sh/docs/faq/#how-does-karpenter-dynamically-select-instance-types)

### NodePool

The NodePool sets constraints on the nodes that can be created by Karpenter and the pods that can run on those nodes. [Examples](https://github.com/aws/karpenter-provider-aws/tree/main/examples/v1)
- Limits pods via taints.
- Limits node creation via zones, instance types, arch etc (requirements combined with pod affinity/nodeSelector).
- Needs atleast one NodePool.
- Mutually exlusive pools are recommended, `weight` (higher is more priority) is used for tie-breaks.

| ScalingAdvisor      | NodePool Karpenter                |
|---------------------|-----------------------------------|
| NP ALT              | ALT (No startup taints)           |
| ???                 | nodeClassRef                      |
| N/A?                | expireAfter: Never                |
| MachineDrainTimeout | terminationGracePeriod            |
| Labels/Annotations  | requirements                      |
| N/A                 | di.consolidationPolicy: WhenEmpty |
| scaleInPolicy (TBD) | di.consolidateAfter               |
| Priority            | weight                            |
| N/A?                | di.budgets???                     |

### NodeClass

- Specific to cloud provider, so maybe requires the `cloud-provider` to be passed along as well when constructing the node class. (Or see if kwok has its own concept of class: kwok/apis/v1alpha1/kwoknodeclass.go, however that only has `NodeRegistrationDelay, only useful for metadata`)
- TODO: how to utilize the kwokNodeClass, since the `systemReserved/kubeReserved` data is not being passed, maybe when generating instances, remove those from the available resources.

| ScalingAdvisor    | NodeClass              |
|-------------------|------------------------|
| nT.systemReserved | kubelet.systemReserved |
| nT.kubeReserved   | kubelet.kubeReserved   |

### NodeClaim

NodeClaims manageed the lifecycle of k8s nodes with underlying cloud provider, acting as requests for capacity. NodeClaims are created/deleted in accordance with the pod demands. It's a view-only resource managed by karpenter.

### InstanceTypes

1. Pricing data for CSPs: only AWS supported via [genprice](../../scadctl/cmd/gardener/genprice.go) asof now.
2. Need to check how to populate these requirements in instanceTypes (kwok/cloudprovider/helpers.go: newInstanceType()) (Check [wellKnownLabels](https://karpenter.sh/docs/concepts/nodepools/#well-known-labels))
```go
	requirements := scheduling.NewRequirements(
		scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, options.Name),
		scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, options.Architecture),
		scheduling.NewRequirement(corev1.LabelOSStable, corev1.NodeSelectorOpIn, osNames...),
		scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zones...),
		scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityTypes...),
		scheduling.NewRequirement(v1alpha1.InstanceSizeLabelKey, corev1.NodeSelectorOpIn, options.instanceTypeLabels[v1alpha1.InstanceSizeLabelKey]),
		scheduling.NewRequirement(v1alpha1.InstanceFamilyLabelKey, corev1.NodeSelectorOpIn, options.instanceTypeLabels[v1alpha1.InstanceFamilyLabelKey]),
		scheduling.NewRequirement(v1alpha1.InstanceCPULabelKey, corev1.NodeSelectorOpIn, options.instanceTypeLabels[v1alpha1.InstanceCPULabelKey]),
		scheduling.NewRequirement(v1alpha1.InstanceMemoryLabelKey, corev1.NodeSelectorOpIn, options.instanceTypeLabels[v1alpha1.InstanceMemoryLabelKey]),
	)
```
