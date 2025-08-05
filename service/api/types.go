package api

import (
	"context"
	corev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ScalingAdvisorService defines the interface for the scaling advisor service.
type ScalingAdvisorService interface {
	// GenerateScalingAdvice generates scaling advice based on the provided constraint, feedback, and snapshot.
	GenerateScalingAdvice(ctx context.Context, constraint corev1alpha1.ClusterScalingConstraint, feedback corev1alpha1.ClusterScalingFeedback, snapshot ClusterSnapshot) (corev1alpha1.ClusterScalingAdvice, error)
}

type ClusterSnapshot struct {
}

// PodInfo contains the minimum set of information about corev1.Pod that will be required by the kube-scheduler.
type PodInfo struct {
	ResourceMeta
	// AggregatedRequests is an aggregated resource requests for all containers of the Pod.
	AggregatedRequests corev1.ResourceList
	Volumes            []corev1.Volume `json:"volumes"`
	NodeSelector       map[string]string
	NodeName           string
	Affinity           *corev1.Affinity
	SchedulerName      string
	Tolerations        []corev1.Toleration
	PriorityClassName  string
	Priority           *int32
	PreemptionPolicy   *corev1.PreemptionPolicy
	RuntimeClassName   *string
	Overhead           corev1.ResourceList
}

type ResourceMeta struct {
	UID string
	types.NamespacedName
	Labels map[string]string
}

func bingo() {
	var _ = corev1.Pod{}
}
