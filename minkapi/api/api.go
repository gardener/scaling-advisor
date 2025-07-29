// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"time"

	apicommon "github.com/gardener/scaling-advisor/api/common/types"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// ProgramName is the name of the program.
	ProgramName           = "minkapi"
	DefaultWatchQueueSize = 100
	DefaultWatchTimeout   = 5 * time.Minute
	// DefaultKubeConfigPath is the default kubeconfig path if none is specified.
	DefaultKubeConfigPath = "/tmp/minkapi.yaml"
)

// MinKAPIConfig holds the configuration for MinKAPI.
type MinKAPIConfig struct {
	apicommon.ServerConfig
	// WatchTimeout represents the timeout for watches following which MinKAPI service will close the connection and ends the watch.
	WatchTimeout time.Duration
	// WatchQueueSize is the maximum number of events to queue per watcher
	WatchQueueSize int
}

// MinKAPIAccess facade defines methods to
type MinKAPIAccess interface {
	Start() error
	Shutdown(ctx context.Context) error
	CreateObject(gvk schema.GroupVersionKind, obj metav1.Object) error
	DeleteObjects(gvk schema.GroupVersionKind, criteria MatchCriteria) error
	ListNodes(matchingNodeNames ...string) ([]*corev1.Node, error)
	ListPods(namespace string, matchingPodNames ...string) ([]*corev1.Pod, error)
	ListEvents(namespace string) ([]*eventsv1.Event, error)
}

type MatchCriteria struct {
	Namespace string
	Names     sets.Set[string]
	Labels    map[string]string
}

func (c MatchCriteria) Matches(obj metav1.Object) bool {
	if c.Namespace != "" && obj.GetNamespace() != c.Namespace {
		return false
	}
	if c.Names.Len() > 0 && !c.Names.Has(obj.GetName()) {
		return false
	}
	if len(c.Labels) > 0 && !isSubset(c.Labels, obj.GetLabels()) {
		return false
	}
	return true
}

// TODO: think about utilizing stdlib/apimachinery replacements for this
func isSubset(subset, superset map[string]string) bool {
	for k, v := range subset {
		if val, ok := superset[k]; !ok || val != v {
			return false
		}
	}
	return true
}
