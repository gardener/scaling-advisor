// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"fmt"
	"strings"

	apicommon "github.com/gardener/scaling-advisor/api/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={scad},scope=Namespaced
// +kubebuilder:printcolumn:name="Constraint",type=string,JSONPath=`.spec.constraintRef.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ScalingAdvice is the top-level object containing scaling advice produced by the scaling-advisor
// from a [ScalingConstraint] applied against a k8s cluster.
//
// The lifecycle manager's response is written to a companion [ScalingFeedback] resource that shares
// the same name and namespace and carries an owner reference back to this object.
//
//nolint:govet // fieldalignment: intentional layout for k8s CRD / wire compatibility
type ScalingAdvice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec defines the specification of ScalingAdvice.
	// +required
	Spec ScalingAdviceSpec `json:"spec"`
	// Status defines the observed state of ScalingAdvice.
	// +optional
	Status ScalingAdviceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ScalingAdviceList is a list of ScalingAdvice.
type ScalingAdviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a slice of ScalingAdvice.
	// +required
	Items []ScalingAdvice `json:"items"`
}

// ScalingAdviceSpec contains the generated scaling advice which holds [ScaleOut] and/or [ScaleIn] plans.
type ScalingAdviceSpec struct {
	// ScaleOut is the plan for scaling out across node pools.
	// +optional
	ScaleOut *ScaleOutPlan `json:"scaleOut,omitempty"`
	// ScaleIn is the plan for scaling in across node pools.
	// +optional
	ScaleIn *ScaleInPlan `json:"scaleIn,omitempty"`
	// Diagnostic provides diagnostics information for the scaling advice.
	// +optional
	Diagnostic *ScalingAdviceDiagnostic `json:"diagnostic,omitempty"`
	// ConstraintRef references the [ScalingConstraint] for which this [ScalingAdvice] was generated.
	// +required
	ConstraintRef apicommon.NamespacedName `json:"constraintRef"`
}

// ScalingAdviceStatus contains the observed status from the scaling-advisor controller.
// Lifecycle manager feedback is written to the companion [ScalingFeedback] resource
// (same name and namespace, owner reference pointing to this object).
type ScalingAdviceStatus struct {
	// Conditions contains the [metav1.Condition]'s for this ScalingAdvice.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ScaleOutPlan is the plan for scaling out a node pool.
type ScaleOutPlan struct {
	// UnsatisfiedPodNames contains pod names in "namespace/name" format that could not be scheduled by this scale-out
	// plan.
	UnsatisfiedPodNames []string `json:"unsatisfiedPodNames,omitempty"`
	// Items is the slice of scaling-out advice for a node pool.
	// +kubebuilder:validation:MinItems=1
	// +required
	Items []ScaleOutItem `json:"items"`
}

// ScaleInPlan is the plan for scaling in a node pool and/or targeted set of nodes.
type ScaleInPlan struct {
	// Items is the slice of scaling-in advice for a node pool.
	// +kubebuilder:validation:MinItems=1
	// +required
	Items []ScaleInItem `json:"items"`
}

// ScaleInItem is the unit of scaling-in advice for a specific node.
type ScaleInItem struct {
	// NodePlacement identifies the target pool, template, and cloud location for this scale-in.
	// +required
	NodePlacement NodePlacement `json:"nodePlacement"`
	// NodeName is the name of the node to be scaled in.
	// +required
	NodeName string `json:"nodeName"`
}

// ScaleOutItem is the unit of scaling advice for a node pool.
type ScaleOutItem struct {
	// NodePlacement identifies the target pool, template, and cloud location for this scale-out.
	// +required
	NodePlacement NodePlacement `json:"nodePlacement"`
	// Name represents a logical name for this scale-out item. This is generated name unique within this ScalingAdvice.
	// Ex: of the form: scaleout-<index>
	// +required
	Name string `json:"name"`
	// CurrentReplicas is the current number of replicas for the NodePlacement.
	// +kubebuilder:validation:Minimum=0
	// +required
	CurrentReplicas int32 `json:"currentReplicas"`
	// Delta is the delta change in the number of nodes for the NodePlacement.
	// +kubebuilder:validation:Minimum=1
	// +required
	Delta int32 `json:"delta"`
}

// NodePlacement identifies the target node pool, node template, and cloud location associated with a scaling operation.
type NodePlacement struct {
	// PoolName is the name of the node pool.
	// +required
	PoolName string `json:"poolName"`
	// TemplateName is the name of the node template.
	// +required
	TemplateName string `json:"templateName"`
	// InstanceType is the instance type of the Node
	// +required
	InstanceType string `json:"instanceType"`
	// Region is the region of the instance
	// +required
	Region string `json:"region"`
	// AvailabilityZone is the availability zone of the node pool.
	// +required
	AvailabilityZone string `json:"availabilityZone"`
}

// MarshalText encodes NodePlacement as a |-separated string for use as a JSON map key.
// Value receiver is intentional: encoding/json requires TextMarshaler on the value type (not *T) to marshal map keys.
//
//nolint:recvcheck
func (n NodePlacement) MarshalText() ([]byte, error) {
	return []byte(n.PoolName + "|" + n.TemplateName + "|" + n.InstanceType + "|" + n.Region + "|" + n.AvailabilityZone), nil
}

// UnmarshalText decodes a |-separated string produced by [NodePlacement.MarshalText].
func (n *NodePlacement) UnmarshalText(text []byte) error {
	parts := strings.SplitN(string(text), "|", 5)
	if len(parts) != 5 {
		return fmt.Errorf("invalid NodePlacement text %q: expected 5 |-separated fields", string(text))
	}
	n.PoolName = parts[0]
	n.TemplateName = parts[1]
	n.InstanceType = parts[2]
	n.Region = parts[3]
	n.AvailabilityZone = parts[4]
	return nil
}

// ScalingAdviceDiagnostic provides diagnostics information for the scaling advice.
type ScalingAdviceDiagnostic struct {
	// TraceLogName is the name of the trace log. This can be used to fetch the trace log.
	// +required
	TraceLogName string `json:"traceLogName"`
	// RunResultsLogName is the name of the run results log.  This can be used to fetch the run results log.
	// +required
	RunResultsLogName string `json:"runResultsLogName"`
}
