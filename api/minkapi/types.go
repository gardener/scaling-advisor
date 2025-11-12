// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package minkapi

import (
	"context"
	"github.com/go-logr/logr"
	"io"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sync/atomic"
	"time"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/events"
)

const (
	// ProgramName is the name of the program.
	ProgramName           = "minkapi"
	DefaultWatchQueueSize = 100
	DefaultWatchTimeout   = 5 * time.Minute
	// DefaultKubeConfigPath is the default kubeconfig path if none is specified.
	DefaultKubeConfigPath = "/tmp/minkapi.yaml"
	// DefaultBasePrefix is the default path prefix for the base minkapi server
	DefaultBasePrefix = "base"
)

// WatchConfig holds config parameters relevant for watchers.
type WatchConfig struct {
	// QueueSize is the maximum number of events to queue per watcher
	QueueSize int
	// Timeout represents the timeout for watches following which MinKAPI service will close the connection and ends the watch.
	Timeout time.Duration
}

// Config holds the configuration for MinKAPI.
type Config struct {
	// BasePrefix is the path prefix at which the base View of the minkapi service is served. ie KAPI-Service at http://<MinKAPIHost>:<MinKAPIPort>/BasePrefix
	// Defaults to [DefaultBasePrefix]
	BasePrefix string
}

// Resettable defines types that can reset their state to a default or initial configuration.
type Resettable interface {
	Reset()
}

type WatchEventCallback func(watch.Event) (err error)

type ResourceStore interface {
	Resettable
	io.Closer
	// GetObjAndListGVK gets the object GVK and object list GVK associated with this resource store.
	GetObjAndListGVK() (objKind schema.GroupVersionKind, objListKind schema.GroupVersionKind)
	// Add adds a new object to the store.
	Add(ctx context.Context, mo metav1.Object) error
	// GetByKey retrieves an object from the store by its key.
	GetByKey(ctx context.Context, key string) (o runtime.Object, err error)
	// Get retrieves an object from the store by its name.
	Get(ctx context.Context, objName cache.ObjectName) (o runtime.Object, err error)
	// Update updates an existing object in the store.
	Update(ctx context.Context, mo metav1.Object) error
	// DeleteByKey deletes an object from the store by its key.
	DeleteByKey(ctx context.Context, key string) error
	// Delete deletes an object from the store by its name.
	Delete(ctx context.Context, objName cache.ObjectName) error
	// DeleteObjects deletes objects matching the given criteria and returns the count of deleted objects.
	DeleteObjects(ctx context.Context, c MatchCriteria) (delCount int, err error)
	// List lists objects matching the given criteria.
	List(ctx context.Context, c MatchCriteria) (listObj runtime.Object, err error)
	// ListMetaObjects lists metadata objects matching the given criteria.
	ListMetaObjects(ctx context.Context, c MatchCriteria) (metaObjs []metav1.Object, maxVersion int64, err error)
	// Watch watches object changes in this store starting from the given startVersion, belonging to the given namespace and matching the given labelSelector and then constructs a watch.Event followed by invoking eventCallback.
	Watch(ctx context.Context, startVersion int64, namespace string, labelSelector labels.Selector, eventCallback WatchEventCallback) error

	// GetVersionCounter returns the atomic counter for generating monotonically increasing resource versions
	GetVersionCounter() *atomic.Int64
}

type ResourceStoreArgs struct {
	Name          string
	ObjectGVK     schema.GroupVersionKind
	ObjectListGVK schema.GroupVersionKind
	// Scheme is the runtime Scheme used by the KAPI objects storable in this store.
	Scheme      *runtime.Scheme
	WatchConfig WatchConfig
	// VersionCounter is the atomic counter for generating monotonically increasing resource versions
	VersionCounter *atomic.Int64 //optional
	Log            logr.Logger
}

type EventSink interface {
	Resettable
	events.EventSink
	List() []eventsv1.Event
}

// View is the high-level facade to a repository of objects of different types (GVK).
// TODO: Think of a better name. Rename this to Repository or RepoView, also add godoc ?
type View interface {
	Resettable
	io.Closer
	GetName() string
	GetType() ViewType
	// SetKubeConfigPath sets the path to the kubeconfig file for this view used to create network client facades.
	SetKubeConfigPath(path string)
	// GetClientFacades gets a ClientFacades populated according to the given accessMode that can be used by code to interact with this view
	// via standard k8s client and informer interfaces
	GetClientFacades(ctx context.Context, accessMode commontypes.ClientAccessMode) (commontypes.ClientFacades, error)
	// GetResourceStore returns the resource store for the specified GroupVersionKind.
	GetResourceStore(gvk schema.GroupVersionKind) (ResourceStore, error)
	GetEventSink() EventSink
	// CreateObject creates a new object of the specified GVK in this view.
	CreateObject(ctx context.Context, gvk schema.GroupVersionKind, obj metav1.Object) (metav1.Object, error)
	// GetObject retrieves an object of the specified GVK by name.
	GetObject(ctx context.Context, gvk schema.GroupVersionKind, objName cache.ObjectName) (runtime.Object, error)
	// UpdateObject updates an existing object of the specified GVK.
	UpdateObject(ctx context.Context, gvk schema.GroupVersionKind, obj metav1.Object) error
	// UpdatePodNodeBinding updates a pod's node binding and returns the updated pod.
	UpdatePodNodeBinding(ctx context.Context, podName cache.ObjectName, binding corev1.Binding) (*corev1.Pod, error)
	// PatchObject applies a patch to an object of the specified GVK.
	PatchObject(ctx context.Context, gvk schema.GroupVersionKind, objName cache.ObjectName, patchType types.PatchType, patchData []byte) (patchedObj runtime.Object, err error)
	// PatchObjectStatus applies a patch to an object's status subresource.
	PatchObjectStatus(ctx context.Context, gvk schema.GroupVersionKind, objName cache.ObjectName, patchData []byte) (patchedObj runtime.Object, err error)
	// ListMetaObjects lists metadata objects matching the given criteria.
	ListMetaObjects(ctx context.Context, gvk schema.GroupVersionKind, criteria MatchCriteria) (metaObjs []metav1.Object, maxVersion int64, err error)
	// ListObjects lists objects in the store while matching the criteria and returns the matching objects as a runtime.Object which is actually a *<Kind>List. Ex: *PodList
	// TODO: consider better name for this method.
	ListObjects(ctx context.Context, gvk schema.GroupVersionKind, criteria MatchCriteria) (runtime.Object, error)
	// WatchObjects watches for changes to objects of the specified GVK.
	WatchObjects(ctx context.Context, gvk schema.GroupVersionKind, startVersion int64, namespace string, labelSelector labels.Selector, eventCallback WatchEventCallback) error
	// GetWatcher returns a watcher for objects of the specified GVK.
	GetWatcher(ctx context.Context, gvk schema.GroupVersionKind, namespace string, opts metav1.ListOptions) (watch.Interface, error)
	// DeleteObject deletes an object of the specified GVK by name.
	DeleteObject(ctx context.Context, gvk schema.GroupVersionKind, objName cache.ObjectName) error
	// DeleteObjects deletes objects of the specified GVK matching the criteria.
	DeleteObjects(ctx context.Context, gvk schema.GroupVersionKind, criteria MatchCriteria) error
	// ListNodes returns nodes matching the specified node names, or all nodes if none specified.
	ListNodes(ctx context.Context, matchingNodeNames ...string) ([]corev1.Node, error)
	// ListPods returns pods matching the specified criteria.
	ListPods(ctx context.Context, criteria MatchCriteria) ([]corev1.Pod, error)
	// ListEvents returns events in the specified namespace.
	ListEvents(ctx context.Context, namespace string) ([]eventsv1.Event, error)
	// GetObjectChangeCount returns the current change count made to objects through this view.
	GetObjectChangeCount() int64
	GetKubeConfigPath() string
}

// ViewType represents the type of View.
type ViewType string

const (
	// BaseViewType represents the foundational view of the MinKAPI server.
	BaseViewType ViewType = "base"
	// SandboxViewType represents a sandboxed private view.
	SandboxViewType ViewType = "sandbox"
)

// CreateSandboxViewFunc represents a creator function for constructing sandbox views from the delegate view and given args
type CreateSandboxViewFunc = func(log logr.Logger, delegateView View, args *ViewArgs) (View, error)

// ViewArgs contains arguments for creating a View.
type ViewArgs struct {
	// Name represents name of View
	Name string
	// KubeConfigPath is the path of the kubeconfig file corresponding to this view
	KubeConfigPath string
	// Scheme is the runtime Scheme used by KAPI objects exposed by this view
	Scheme *runtime.Scheme
	// WatchConfig contains configuration for watch operations.
	WatchConfig WatchConfig
}

// ViewAccess is a facade to get or create KAPI Views.
type ViewAccess interface {
	io.Closer
	// GetBaseView returns the foundational View of the KAPI Server which is exposed at http://<MinKAPIHost>:<MinKAPIPort>/basePrefix
	GetBaseView() View
	// GetOrCreateSandboxView creates or returns a sandboxed KAPI View with the given name that is also served as a KAPI Service
	// at http://<MinKAPIHost>:<MinKAPIPort>/sandboxName. A kubeconfig named `minkapi-<name>.yaml` is also generated
	// in the same directory as the base `minkapi.yaml`.  The sandbox name should be a valid path-prefix, ie no-spaces.
	// TODO: discuss whether the above is OK.
	GetOrCreateSandboxView(ctx context.Context, name string) (View, error)
}

// Server represents a MinKAPI server that provides access to a KAPI (kubernetes API) service accessible at http://<MinKAPIHost>:<MinKAPIPort>/base
// It also supports methods to create "sandbox" (private) views accessible at http://<MinKAPIHost>:<MinKAPIPort>/sandboxName
type Server interface {
	commontypes.Service
	ViewAccess
}

// App represents an application process that wraps a minkapi Server, an application context and application cancel func.
//
// `main` entry-point functions taht embed minkapi are expected to construct a new App instance via cli.LaunchApp and shutdown applications via cli.ShutdownApp
type App struct {
	Server Server
	Ctx    context.Context
	Cancel context.CancelFunc
}

type MatchCriteria struct {
	Namespace string
	Names     sets.Set[string]
	// Labels        map[string]string
	LabelSelector labels.Selector
}

var MatchAllCriteria = MatchCriteria{}

func (c MatchCriteria) Matches(obj metav1.Object) bool {
	if c.Namespace != "" && obj.GetNamespace() != c.Namespace {
		return false
	}
	if c.Names.Len() > 0 && !c.Names.Has(obj.GetName()) {
		return false
	}
	if c.LabelSelector != nil && !c.LabelSelector.Matches(labels.Set(obj.GetLabels())) {
		return false
	}
	return true
}
