package drainutil

import (
	"fmt"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	corev1 "k8s.io/api/core/v1"
)

type DrainBlockingPod struct {
	Pod *corev1.Pod
	// Reason is the reason why the pod is blocking the drain.
	Reason BlockingPodReason
}

// BlockingPodReason represents a reason why a pod is blocking the scale down of a node.
type BlockingPodReason int

const (
	// NoReason - sanity check, this should never be set explicitly. If this is found in the wild, it means that it was
	// implicitly initialized and might indicate a bug.
	NoReason BlockingPodReason = iota
	// ControllerNotFound - pod is blocking scale down because its controller can't be found.
	ControllerNotFound
	// MinReplicasReached - pod is blocking scale down because its controller already has the minimum number of replicas.
	MinReplicasReached
	// NotReplicated - pod is blocking scale down because it's not replicated.
	NotReplicated
	// LocalStorageRequested - pod is blocking scale down because it requests local storage.
	LocalStorageRequested
	// NotSafeToEvictAnnotation - pod is blocking scale down because it has a "not safe to evict" annotation.
	NotSafeToEvictAnnotation
	// UnmovableKubeSystemPod - pod is blocking scale down because it's a non-daemonset, non-mirrored, non-pdb-assigned kube-system pod.
	UnmovableKubeSystemPod
	// NotEnoughPdb - pod is blocking scale down because it doesn't have enough PDB left.
	NotEnoughPdb
	// UnexpectedError - pod is blocking scale down because of an unexpected error.
	UnexpectedError
)

func (e BlockingPodReason) String() string {
	switch e {
	case NoReason:
		return "NoReason"
	case ControllerNotFound:
		return "ControllerNotFound"
	case MinReplicasReached:
		return "MinReplicasReached"
	case NotReplicated:
		return "NotReplicated"
	case LocalStorageRequested:
		return "LocalStorageRequested"
	case NotSafeToEvictAnnotation:
		return "NotSafeToEvictAnnotation"
	case UnmovableKubeSystemPod:
		return "UnmovableKubeSystemPod"
	case NotEnoughPdb:
		return "NotEnoughPdb"
	case UnexpectedError:
		return "UnexpectedError"
	default:
		return fmt.Sprintf("unrecognized reason: %d", int(e))
	}
}

// HasNonEvictablePod returns true if any pod in the slice has the `sa.gardener.cloud/safe-to-evict` annotation set to "false".
func HasNonEvictablePod(pods []*corev1.Pod) bool {
	for _, pod := range pods {
		if val, ok := pod.GetAnnotations()[commonconstants.AnnotationSafeToEvict]; ok && val == "false" {
			return true
		}
	}
	return false
}
