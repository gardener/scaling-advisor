// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"reflect"
	rt "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/core/configtmpl"
	"github.com/gardener/scaling-advisor/minkapi/core/objutil"
	"github.com/gardener/scaling-advisor/minkapi/core/podutil"
	"github.com/gardener/scaling-advisor/minkapi/core/store"
	"github.com/gardener/scaling-advisor/minkapi/core/typeinfo"

	"github.com/go-logr/logr"
	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kjson "k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

var _ api.MinKAPIAccess = (*InMemoryKAPI)(nil)

// InMemoryKAPI holds the in-memory stores, watch channels, and version tracking for simple implementation of api.MinKAPIAccess
type InMemoryKAPI struct {
	cfg          api.MinKAPIConfig
	mu           sync.RWMutex
	stores       map[schema.GroupVersionKind]*store.InMemResourceStore
	listenerAddr net.Addr
	scheme       *runtime.Scheme
	mux          *http.ServeMux
	server       *http.Server
	log          logr.Logger
}

func NewInMemoryMinKAPI(appCtx context.Context, cfg api.MinKAPIConfig, log logr.Logger) (api.MinKAPIAccess, error) {
	mux := http.NewServeMux()
	stores := map[schema.GroupVersionKind]*store.InMemResourceStore{}
	for _, d := range typeinfo.SupportedDescriptors {
		stores[d.GVK] = store.NewInMemResourceStore(d.GVK, d.ListGVK, d.GVR.GroupResource().Resource, cfg.WatchQueueSize, cfg.WatchTimeout, typeinfo.SupportedScheme, log)
	}
	s := &InMemoryKAPI{
		cfg:    cfg,
		stores: stores,
		scheme: typeinfo.SupportedScheme,
		mux:    mux,
		server: &http.Server{
			Addr:    net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
			Handler: mux,
			BaseContext: func(_ net.Listener) context.Context {
				return appCtx
			},
		},
		log: log,
	}
	s.registerRoutes()
	return s, nil
}

func (k *InMemoryKAPI) CreateObject(gvk schema.GroupVersionKind, obj metav1.Object) (err error) {
	s := k.getStore(gvk)
	if s == nil {
		return fmt.Errorf("%w: no store for GVK %q", api.ErrStoreNotFound, gvk)
	}

	name := obj.GetName()
	namePrefix := obj.GetGenerateName()
	if name == "" {
		if namePrefix == "" {
			return fmt.Errorf("%w: missing both name and generateName in request for creating object of objGvk %q in %q namespace", api.ErrCreateObject, gvk, obj.GetNamespace())
		}
		name = typeinfo.GenerateName(namePrefix)
	}
	obj.SetName(name)

	createTimestamp := obj.GetCreationTimestamp()
	if (&createTimestamp).IsZero() { // only set creationTimestamp if not already set.
		obj.SetCreationTimestamp(metav1.Time{Time: time.Now()})
	}

	if obj.GetUID() == "" {
		obj.SetUID(uuid.NewUUID())
	}

	objutil.SetMetaObjectGVK(obj, gvk)

	err = s.Add(obj)
	if err != nil {
		return fmt.Errorf("%w: %w", api.ErrCreateObject, err)
	}

	return nil
}

func (k *InMemoryKAPI) DeleteObjects(gvk schema.GroupVersionKind, c api.MatchCriteria) error {
	s := k.getStore(gvk)
	if s == nil {
		return fmt.Errorf("%w: store not found for gvk %q", api.ErrDeleteObject, gvk)
	}
	return s.DeleteObjects(c)
}

func (k *InMemoryKAPI) ListPods(namespace string, matchingPodNames ...string) ([]*corev1.Pod, error) {
	if len(strings.TrimSpace(namespace)) == 0 {
		return nil, errors.New("cannot list pods without namespace")
	}
	podNamesSet := sets.New(matchingPodNames...)
	c := api.MatchCriteria{
		Namespace: namespace,
		Names:     podNamesSet,
	}
	gvk := typeinfo.PodsDescriptor.GVK
	s := k.getStore(gvk)
	if s == nil {
		return nil, fmt.Errorf("%w: store not found for gvk %q", api.ErrListObjects, gvk)
	}
	objs, err := s.ListMetaObjects(c)
	if err != nil {
		return nil, err
	}
	pods := make([]*corev1.Pod, 0, len(objs))
	for _, obj := range objs {
		pods = append(pods, obj.(*corev1.Pod))
	}
	return pods, nil
}

func (k *InMemoryKAPI) ListNodes(matchingNodeNames ...string) ([]*corev1.Node, error) {
	// if len(strings.TrimSpace(namespace)) == 0 {
	// 	return nil, errors.New("cannot list nodes without namespace")
	// }
	nodeNamesSet := sets.New(matchingNodeNames...)
	c := api.MatchCriteria{
		// Namespace: namespace,
		Names: nodeNamesSet,
	}
	gvk := typeinfo.NodesDescriptor.GVK
	s := k.getStore(gvk)
	if s == nil {
		return nil, fmt.Errorf("%w: store not found for gvk %q", api.ErrListObjects, gvk)
	}
	objs, err := s.ListMetaObjects(c)
	if err != nil {
		return nil, err
	}
	nodes := make([]*corev1.Node, 0, len(objs))
	for _, obj := range objs {
		nodes = append(nodes, obj.(*corev1.Node))
	}
	return nodes, nil
}

func (k *InMemoryKAPI) ListEvents(namespace string) ([]*eventsv1.Event, error) {
	if len(strings.TrimSpace(namespace)) == 0 {
		return nil, errors.New("cannot list events without namespace")
	}
	c := api.MatchCriteria{
		Namespace: namespace,
	}
	gvk := typeinfo.EventsDescriptor.GVK
	s := k.getStore(gvk)
	if s == nil {
		return nil, fmt.Errorf("%w: store not found for gvk %q", api.ErrListObjects, gvk)
	}
	objs, err := s.ListMetaObjects(c)
	if err != nil {
		return nil, err
	}
	events := make([]*eventsv1.Event, 0, len(objs))
	for _, obj := range objs {
		events = append(events, obj.(*eventsv1.Event))
	}
	return events, nil
}

func (k *InMemoryKAPI) GetMux() *http.ServeMux {
	return k.mux
}

// Start begins the HTTP server
func (k *InMemoryKAPI) Start() error {
	// We do this because we want the bind address
	listener, err := net.Listen("tcp", k.server.Addr)
	if err != nil {
		return fmt.Errorf("%w: cannot listen on TCP Address %q: %w", api.ErrStartFailed, k.server.Addr, err)
	}
	k.listenerAddr = listener.Addr()
	err = configtmpl.GenKubeConfig(configtmpl.KubeConfigParams{
		KubeConfigPath: k.cfg.KubeConfigPath,
		URL:            "http://" + k.listenerAddr.String(),
	})
	if err != nil {
		return fmt.Errorf("%w: %w", api.ErrStartFailed, err)
	}
	k.log.Info("kubeconfig generated", "path", k.cfg.KubeConfigPath)

	schedulerTmplParams := configtmpl.KubeSchedulerTmplParams{
		KubeConfigPath:          k.cfg.KubeConfigPath,
		KubeSchedulerConfigPath: fmt.Sprintf("/tmp/%s-kube-scheduler-config.yaml", api.ProgramName),
		QPS:                     100, //TODO: pass this as param ?
		Burst:                   50,
	}
	err = configtmpl.GenKubeSchedulerConfig(schedulerTmplParams)
	if err != nil {
		return fmt.Errorf("%w: %w", api.ErrStartFailed, err)
	}
	k.log.Info("sample kube-scheduler-config generated", "path", schedulerTmplParams.KubeSchedulerConfigPath)
	k.log.Info(fmt.Sprintf("%s service listening", api.ProgramName), "address", k.server.Addr)
	if err := k.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%w: %w", api.ErrServiceFailed, err)
	}
	return nil
}

// Shutdown shuts down the HTTP server and closes resources
func (k *InMemoryKAPI) Shutdown(ctx context.Context) (err error) {
	err = k.server.Shutdown(ctx) // shutdown server first to avoid accepting new requests.
	k.closeStores()
	return
}

func (k *InMemoryKAPI) closeStores() {
	k.mu.RLock()
	defer k.mu.RUnlock()
	for _, s := range k.stores {
		s.Shutdown()
	}
}

func (k *InMemoryKAPI) registerRoutes() {
	if k.cfg.ProfilingEnabled {
		k.log.Info("profiling enabled - registering /debug/pprof/* handlers")
		k.mux.HandleFunc("/debug/pprof/", pprof.Index)
		k.mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		k.mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		k.mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		k.mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		k.mux.HandleFunc("/trigger-gc", func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintln(w, "GC Triggering")
			rt.GC() // force garbage collection
			_, _ = fmt.Fprintln(w, "GC Triggered")
		})
	}

	k.mux.HandleFunc("GET /api", k.handleAPIVersions)
	k.mux.HandleFunc("GET /apis", k.handleAPIGroups)

	// Core API Group and Other API Groups
	k.registerAPIGroups()

	for _, d := range typeinfo.SupportedDescriptors {
		k.registerResourceRoutes(d)
	}
}

func (k *InMemoryKAPI) registerAPIGroups() {
	// Core API
	k.mux.HandleFunc("GET /api/v1/", k.handleAPIResources(typeinfo.SupportedCoreAPIResourceList))

	// API groups
	for _, apiList := range typeinfo.SupportedGroupAPIResourceLists {
		route := fmt.Sprintf("GET /apis/%s/", apiList.APIResources[0].Group)
		k.mux.HandleFunc(route, k.handleAPIResources(apiList))
	}
}

func (k *InMemoryKAPI) registerResourceRoutes(d typeinfo.Descriptor) {
	g := d.GVK.Group
	r := d.GVR.Resource
	if d.GVK.Group == "" {
		k.mux.HandleFunc(fmt.Sprintf("POST /api/v1/namespaces/{namespace}/%s", r), k.handleCreate(d))
		k.mux.HandleFunc(fmt.Sprintf("GET /api/v1/namespaces/{namespace}/%s", r), k.handleListOrWatch(d))
		k.mux.HandleFunc(fmt.Sprintf("GET /api/v1/namespaces/{namespace}/%s/{name}", r), k.handleGet(d))
		k.mux.HandleFunc(fmt.Sprintf("PATCH /api/v1/namespaces/{namespace}/%s/{name}", r), k.handlePatch(d))
		k.mux.HandleFunc(fmt.Sprintf("PATCH /api/v1/namespaces/{namespace}/%s/{name}/status", r), k.handlePatchStatus(d))
		k.mux.HandleFunc(fmt.Sprintf("DELETE /api/v1/namespaces/{namespace}/%s/{name}", r), k.handleDelete(d))
		k.mux.HandleFunc(fmt.Sprintf("PUT /api/v1/namespaces/{namespace}/%s/{name}", r), k.handlePut(d))        // Update
		k.mux.HandleFunc(fmt.Sprintf("PUT /api/v1/namespaces/{namespace}/%s/{name}/status", r), k.handlePut(d)) // UpdateStatus

		if d.Kind == typeinfo.PodsDescriptor.Kind {
			k.mux.HandleFunc("POST /api/v1/namespaces/{namespace}/pods/{name}/binding", k.handleCreatePodBinding)
		}

		k.mux.HandleFunc(fmt.Sprintf("POST /api/v1/%s", r), k.handleCreate(d))
		k.mux.HandleFunc(fmt.Sprintf("GET /api/v1/%s", r), k.handleListOrWatch(d))
		k.mux.HandleFunc(fmt.Sprintf("GET /api/v1/%s/{name}", r), k.handleGet(d))
		k.mux.HandleFunc(fmt.Sprintf("PATCH /api/v1/%s/{name}", r), k.handlePatch(d))
		k.mux.HandleFunc(fmt.Sprintf("DELETE /api/v1/%s/{name}", r), k.handleDelete(d))
		k.mux.HandleFunc(fmt.Sprintf("PUT /api/v1/%s/{name}", r), k.handlePut(d))        // Update
		k.mux.HandleFunc(fmt.Sprintf("PUT /api/v1/%s/{name}/status", r), k.handlePut(d)) // UpdateStatus
	} else {
		k.mux.HandleFunc(fmt.Sprintf("POST /apis/%s/v1/namespaces/{namespace}/%s", g, r), k.handleCreate(d))
		k.mux.HandleFunc(fmt.Sprintf("GET /apis/%s/v1/namespaces/{namespace}/%s", g, r), k.handleListOrWatch(d))
		k.mux.HandleFunc(fmt.Sprintf("GET /apis/%s/v1/namespaces/{namespace}/%s/{name}", g, r), k.handleGet(d))
		k.mux.HandleFunc(fmt.Sprintf("PATCH /apis/%s/v1/namespaces/{namespace}/%s/{name}", g, r), k.handlePatch(d))
		k.mux.HandleFunc(fmt.Sprintf("DELETE /apis/%s/v1/namespaces/{namespace}/%s/{name}", g, r), k.handleDelete(d))
		k.mux.HandleFunc(fmt.Sprintf("PUT /apis/%s/v1/namespaces/{namespace}/%s/{name}", g, r), k.handlePut(d))

		k.mux.HandleFunc(fmt.Sprintf("POST /apis/%s/v1/%s", g, r), k.handleCreate(d))
		k.mux.HandleFunc(fmt.Sprintf("GET /apis/%s/v1/%s", g, r), k.handleListOrWatch(d))
		k.mux.HandleFunc(fmt.Sprintf("GET /apis/%s/v1/%s/{name}", g, r), k.handleGet(d))
		k.mux.HandleFunc(fmt.Sprintf("DELETE /apis/%s/v1/%s/{name}", g, r), k.handleDelete(d))
	}
}

// handleAPIGroups returns the list of supported API groups
func (k *InMemoryKAPI) handleAPIGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJsonResponse(k.log, w, r, &typeinfo.SupportedAPIGroups)
}

// handleAPIVersions returns the list of versions for the core API group
func (k *InMemoryKAPI) handleAPIVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJsonResponse(k.log, w, r, &typeinfo.SupportedAPIVersions)
}

func (k *InMemoryKAPI) handleAPIResources(apiResourceList metav1.APIResourceList) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJsonResponse(k.log, w, r, apiResourceList)
	}
}

func (k *InMemoryKAPI) handleCreate(d typeinfo.Descriptor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := k.getStoreOrWriteError(d.GVK, w, r)
		if s == nil {
			return
		}
		var (
			mo  metav1.Object
			err error
		)
		mo, err = d.CreateObject()
		if err != nil {
			err = fmt.Errorf("cannot create object from objGvk %q: %v", d.GVK, err)
			k.handleInternalServerError(w, r, err)
			return
		}

		if !k.readBodyIntoObj(w, r, mo) {
			return
		}

		var namespace string
		if mo.GetNamespace() == "" {
			namespace = GetObjectName(r, d).Namespace
			mo.SetNamespace(namespace)
		}
		name := mo.GetName()
		namePrefix := mo.GetGenerateName()
		if name == "" {
			if namePrefix == "" {
				err = fmt.Errorf("missing both name and generateName in request for creating object of objGvk %q in %q namespace", d.GVK, namespace)
				handleBadRequest(k.log, w, r, err)
				return
			}
			name = typeinfo.GenerateName(namePrefix)
		}
		mo.SetName(name)

		createTimestamp := mo.GetCreationTimestamp()
		if (&createTimestamp).IsZero() { // only set creationTimestamp if not already set.
			mo.SetCreationTimestamp(metav1.Time{Time: time.Now()})
		}

		if mo.GetUID() == "" {
			mo.SetUID(uuid.NewUUID())
		}

		err = s.Add(mo)
		if err != nil {
			k.handleError(w, r, err)
			return
		}
		writeJsonResponse(k.log, w, r, mo)
	}
}

// handlePut Ref: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#considerations-for-put-operations (TODO ensure handlePut follows this)
// TODO: handlePut is not complete
func (k *InMemoryKAPI) handlePut(d typeinfo.Descriptor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := k.getStoreOrWriteError(d.GVK, w, r)
		if s == nil {
			return
		}
		key := GetObjectKey(r, d)
		obj, err := s.GetByKey(key)
		if err != nil {
			k.handleError(w, r, err)
			return
		}
		if !k.readBodyIntoObj(w, r, obj) {
			return
		}
		metaObj := obj.(metav1.Object)
		err = s.Update(metaObj)
		if err != nil {
			k.handleError(w, r, err)
			return
		}
		writeJsonResponse(k.log, w, r, obj)
	}
}

func (k *InMemoryKAPI) handleGet(d typeinfo.Descriptor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := k.getStoreOrWriteError(d.GVK, w, r)
		if s == nil {
			return
		}
		key := GetObjectKey(r, d)
		obj, err := s.GetByKey(key)
		if err != nil {
			k.handleError(w, r, err)
			return
		}
		writeJsonResponse(k.log, w, r, obj)
	}
}

func (k *InMemoryKAPI) handleDelete(d typeinfo.Descriptor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := k.getStoreOrWriteError(d.GVK, w, r)
		if s == nil {
			return
		}

		objKey := GetObjectName(r, d).String()
		obj, err := s.GetByKey(objKey)
		if err != nil {
			k.handleError(w, r, err)
			return
		}
		err = s.Delete(objKey)
		if err != nil {
			k.handleError(w, r, err)
			return
		}
		mo, err := meta.Accessor(obj)
		if err != nil {
			k.handleError(w, r, fmt.Errorf("stored object with key %q is not metav1.Object: %w", objKey, err))
			return
		}
		status := metav1.Status{
			TypeMeta: metav1.TypeMeta{ //No idea why this is explicitly needed just for this payload, but kubectl complains
				Kind:       "Status",
				APIVersion: "v1",
			},
			Status: metav1.StatusSuccess,
			Details: &metav1.StatusDetails{
				Name: objKey,
				Kind: d.GVR.GroupResource().Resource,
				UID:  mo.GetUID(),
			},
		}
		writeJsonResponse(k.log, w, r, &status)
	}
}

func (k *InMemoryKAPI) handleListOrWatch(d typeinfo.Descriptor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		isWatch := query.Get("watch")
		var delegate http.HandlerFunc

		labelSelector, err := parseLabelSelector(r)
		if err != nil {
			handleBadRequest(k.log, w, r, err)
			return
		}

		if isWatch == "true" {
			delegate = k.handleWatch(d, labelSelector)
		} else {
			delegate = k.handleList(d, labelSelector)
		}
		delegate.ServeHTTP(w, r)
	}
}

func (k *InMemoryKAPI) handleList(d typeinfo.Descriptor, labelSelector labels.Selector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := k.getStoreOrWriteError(d.GVK, w, r)
		if s == nil {
			return
		}
		namespace := r.PathValue("namespace")
		listObj, err := s.List(namespace, labelSelector)
		if err != nil {
			return
		}
		writeJsonResponse(k.log, w, r, listObj)
	}
}

func (k *InMemoryKAPI) handlePatch(d typeinfo.Descriptor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := k.getStoreOrWriteError(d.GVK, w, r)
		if s == nil {
			return
		}
		key := GetObjectKey(r, d)
		o, err := s.GetByKey(key)
		if err != nil {
			k.handleError(w, r, err)
			return
		}
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/strategic-merge-patch+json" && contentType != "application/merge-patch+json" {
			err = fmt.Errorf("unsupported content type %q for o %q", contentType, key)
			handleBadRequest(k.log, w, r, err)
			return
		}
		patchData, err := io.ReadAll(r.Body)
		if err != nil {
			statusErr := apierrors.NewInternalError(err)
			writeStatusError(k.log, w, r, statusErr)
			return
		}
		err = patchObject(o, key, contentType, patchData)
		if err != nil {
			err = fmt.Errorf("failed to atch o %q: %w", key, err)
			k.handleInternalServerError(w, r, err)
			return
		}
		mo, err := meta.Accessor(o)
		if err != nil {
			k.handleError(w, r, fmt.Errorf("stored object with key %q is not metav1.Object: %w", key, err))
			return
		}
		err = s.Update(mo)
		if err != nil {
			k.handleError(w, r, err)
			return
		}
		writeJsonResponse(k.log, w, r, o)
	}
}

func (k *InMemoryKAPI) handlePatchStatus(d typeinfo.Descriptor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := k.getStoreOrWriteError(d.GVK, w, r)
		if s == nil {
			return
		}

		key := GetObjectKey(r, d)
		o, err := s.GetByKey(key)
		if err != nil {
			k.handleError(w, r, err)
			return
		}
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/strategic-merge-patch+json" {
			err = fmt.Errorf("unsupported content type %q for o %q", contentType, key)
			k.handleInternalServerError(w, r, err)
			return
		}

		patchData, err := io.ReadAll(r.Body)
		if err != nil {
			err = fmt.Errorf("failed to read patch body for o %q", key)
			k.handleInternalServerError(w, r, err)
			return
		}
		err = patchStatus(o, key, patchData)
		if err != nil {
			err = fmt.Errorf("failed to atch status for o %q: %w", key, err)
			k.handleInternalServerError(w, r, err)
			return
		}
		mo, err := meta.Accessor(o)
		if err != nil {
			k.handleError(w, r, fmt.Errorf("stored object with key %q is not metav1.Object: %w", key, err))
			return
		}
		err = s.Update(mo)
		if err != nil {
			k.handleError(w, r, err)
			return
		}
		writeJsonResponse(k.log, w, r, o)
	}
}

func (k *InMemoryKAPI) handleWatch(d typeinfo.Descriptor, labelSelector labels.Selector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := k.getStoreOrWriteError(d.GVK, w, r)
		if s == nil {
			return
		}

		var (
			ok           bool
			startVersion int64
			namespace    string
		)

		namespace = r.PathValue("namespace")
		startVersion, ok = getParseResourceVersion(k.log, w, r)
		if !ok {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Transfer-Encoding", "chunked")
		flusher := getFlusher(w)
		if flusher == nil {
			return
		}

		err := s.Watch(r.Context(), startVersion, namespace, labelSelector, func(event watch.Event) error {
			metaObj, err := store.AsMeta(k.log, event.Object)
			if err != nil {
				return err
			}
			eventJson, err := buildWatchEventJson(k.log, &event)
			if err != nil {
				err = fmt.Errorf("cannot  encode watch %q event for object name %q, namespace %q, resourceVersion %q: %w",
					event.Type, metaObj.GetName(), metaObj.GetNamespace(), metaObj.GetResourceVersion(), err)
				return err
			}
			_, _ = fmt.Fprintln(w, eventJson)
			flusher.Flush()
			return nil
		})

		if err != nil {
			k.log.Error(err, "watch failed", "gvk", d.GVK, "namespace", namespace, "startVersion", startVersion, "labelSelector", labelSelector)
		}
	}
}

// handleCreatePodBinding is meant to handle creation for a Pod binding.
// Ex: POST http://localhost:8080/api/v1/namespaces/default/pods/a-mc6zl/binding
// This endpoint is invoked by the scheduler, and it is expected that the API HostPort sets the `pod.Spec.NodeName`
//
// Example Payload
// {"kind":"Binding","apiVersion":"v1","metadata":{"name":"a-p4r2l","namespace":"default","uid":"b8124ee8-a0c7-4069-930d-fc5e901675d3"},"target":{"kind":"Node","name":"a-kl827"}}
func (k *InMemoryKAPI) handleCreatePodBinding(w http.ResponseWriter, r *http.Request) {
	d := typeinfo.PodsDescriptor
	s := k.getStoreOrWriteError(d.GVK, w, r)
	if s == nil {
		return
	}
	binding := corev1.Binding{}
	if !k.readBodyIntoObj(w, r, &binding) {
		return
	}
	key := GetObjectKey(r, d)
	obj, err := s.GetByKey(key)
	if err != nil {
		k.handleError(w, r, err)
		return
	}
	pod := obj.(*corev1.Pod)
	pod.Spec.NodeName = binding.Target.Name
	podutil.UpdatePodCondition(&pod.Status, &corev1.PodCondition{
		Type:   corev1.PodScheduled,
		Status: corev1.ConditionTrue,
	})
	err = s.Update(pod)
	if err != nil {
		k.log.Error(err, "cannot assign pod to node", "podName", pod.Name, "podNamespace", pod.Namespace, "nodeName", pod.Spec.NodeName)
		k.handleError(w, r, err)
		return
	}
	k.log.V(3).Info("assigned pod to node", "podName", pod.Name, "podNamespace", pod.Namespace, "nodeName", pod.Spec.NodeName)
	// Return {"kind":"Status","apiVersion":"v1","metadata":{},"status":"Success","code":201}
	statusOK := &metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status"},
		Status:   metav1.StatusSuccess,
		Code:     http.StatusCreated,
	}
	writeJsonResponse(k.log, w, r, statusOK)
}

func (k *InMemoryKAPI) getStore(gvk schema.GroupVersionKind) *store.InMemResourceStore {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.stores[gvk]
}

func (k *InMemoryKAPI) getStoreOrWriteError(gvk schema.GroupVersionKind, w http.ResponseWriter, r *http.Request) (s *store.InMemResourceStore) {
	s = k.getStore(gvk)
	if s == nil {
		err := fmt.Errorf("no store initialized for GVK %q", gvk)
		k.log.Error(err, "store error", "gvk", gvk)
		k.handleInternalServerError(w, r, err)
	}
	return
}

func writeStatusError(log logr.Logger, w http.ResponseWriter, r *http.Request, statusError *apierrors.StatusError) {
	w.WriteHeader(int(statusError.ErrStatus.Code))
	writeJsonResponse(log, w, r, statusError.ErrStatus)
}

// writeJsonResponse sets Content-Type to application/json  and encodes the object to the response writer.
func writeJsonResponse(log logr.Logger, w http.ResponseWriter, r *http.Request, obj any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(obj); err != nil {
		log.Error(err, "cannot  encode response", "method", r.Method, "requestURI", r.RequestURI, "obj", obj)
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

func getParseResourceVersion(log logr.Logger, w http.ResponseWriter, r *http.Request) (resourceVersion int64, ok bool) {
	paramValue := r.URL.Query().Get("resourceVersion")
	if paramValue == "" {
		ok = true
		resourceVersion = 0
		return
	}
	resourceVersion, err := parseResourceVersion(paramValue)
	if err != nil {
		handleBadRequest(log, w, r, fmt.Errorf("invalid resource version %q: %w", paramValue, err))
		return
	}
	ok = true
	return
}

func parseResourceVersion(rvStr string) (resourceVersion int64, err error) {
	if rvStr != "" {
		resourceVersion, err = strconv.ParseInt(rvStr, 10, 64)
	}
	return
}

func getFlusher(w http.ResponseWriter) http.Flusher {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Transfer-Encoding", "chunked")
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return nil
	}
	return flusher
}

func buildWatchEventJson(log logr.Logger, event *watch.Event) (string, error) {
	// NOTE: Simple Json serialization does NOT work due to bug in Watch struct
	//if err := json.NewEncoder(w).Encode(event); err != nil {
	//	http.Error(w, fmt.Sprintf("Failed to encode watch event: %v", err), http.StatusInternalServerError)
	//	s.removeWatch(gvr, namespace, ch)
	//	return
	//}
	data, err := kjson.Marshal(event.Object)
	if err != nil {
		log.Error(err, "cannot encode watch event", "event", event)
		return "", err
	}
	payload := fmt.Sprintf("{\"type\":\"%s\",\"object\":%s}", event.Type, string(data))
	return payload, nil
}

func (k *InMemoryKAPI) readBodyIntoObj(w http.ResponseWriter, r *http.Request, obj any) (ok bool) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		handleBadRequest(k.log, w, r, err)
		ok = false
		return
	}
	if err := json.Unmarshal(data, obj); err != nil {
		err = fmt.Errorf("cannot unmarshal JSON for request %q: %w", r.RequestURI, err)
		handleBadRequest(k.log, w, r, err)
		ok = false
		return
	}
	ok = true
	return
}

func (k *InMemoryKAPI) handleError(w http.ResponseWriter, r *http.Request, err error) {
	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		k.handleStatusError(w, r, statusErr)
	} else {
		k.handleInternalServerError(w, r, err)
	}
}

func (k *InMemoryKAPI) handleStatusError(w http.ResponseWriter, r *http.Request, statusErr *apierrors.StatusError) {
	w.WriteHeader(int(statusErr.ErrStatus.Code))
	w.Header().Set("Content-Type", "application/json")
	writeJsonResponse(k.log, w, r, statusErr.ErrStatus)
}

func (k *InMemoryKAPI) handleInternalServerError(w http.ResponseWriter, r *http.Request, err error) {
	statusErr := apierrors.NewInternalError(err)
	k.log.Error(err, "internal server error")
	w.WriteHeader(http.StatusInternalServerError)
	w.Header().Set("Content-Type", "application/json")
	writeJsonResponse(k.log, w, r, statusErr.ErrStatus)
}

func handleBadRequest(log logr.Logger, w http.ResponseWriter, r *http.Request, err error) {
	err = fmt.Errorf("cannot handle request %q: %w", r.Method+" "+r.RequestURI, err)
	log.Error(err, "bad request", "method", r.Method, "requestURI", r.RequestURI)
	statusErr := apierrors.NewBadRequest(err.Error())
	w.WriteHeader(http.StatusBadRequest)
	w.Header().Set("Content-Type", "application/json")
	writeJsonResponse(log, w, r, statusErr.ErrStatus)
}

func GetObjectName(r *http.Request, d typeinfo.Descriptor) cache.ObjectName {
	namespace := r.PathValue("namespace")
	if namespace == "" && d.APIResource.Namespaced {
		namespace = "default"
	}
	name := r.PathValue("name")
	return cache.NewObjectName(namespace, name)
}

func GetObjectKey(r *http.Request, d typeinfo.Descriptor) string {
	return GetObjectName(r, d).String()
}

func patchObject(objPtr runtime.Object, key string, contentType string, patchBytes []byte) error {
	objValuePtr := reflect.ValueOf(objPtr)
	if objValuePtr.Kind() != reflect.Ptr || objValuePtr.IsNil() {
		return fmt.Errorf("object %q must be a non-nil pointer", key)
	}
	objInterface := objValuePtr.Interface()
	originalJSON, err := kjson.Marshal(objInterface)
	if err != nil {
		return fmt.Errorf("failed to marshal object %q: %w", key, err)
	}

	var patchedBytes []byte
	switch contentType {
	case "application/strategic-merge-patch+json":
		patchedBytes, err = strategicpatch.StrategicMergePatch(originalJSON, patchBytes, objInterface)
		if err != nil {
			return fmt.Errorf("failed to apply strategic merge patch for object %q: %w", key, err)
		}
	case "application/merge-patch+json":
		patchedBytes, err = jsonpatch.MergePatch(originalJSON, patchBytes)
		if err != nil {
			return fmt.Errorf("failed to apply merge-patch for object %q: %w", key, err)
		}
	default:
		return fmt.Errorf("unsupported patch content type %q for object %q", contentType, key)
	}
	err = kjson.Unmarshal(patchedBytes, objInterface)
	if err != nil {
		return fmt.Errorf("failed to unmarshal patched JSON back into obj %q: %w", key, err)
	}
	return nil
}

func patchStatus(objPtr runtime.Object, key string, patch []byte) error {
	objValuePtr := reflect.ValueOf(objPtr)
	if objValuePtr.Kind() != reflect.Ptr || objValuePtr.IsNil() {
		return fmt.Errorf("object %q must be a non-nil pointer", key)
	}
	statusField := objValuePtr.Elem().FieldByName("Status")
	if !statusField.IsValid() {
		return fmt.Errorf("object %q of type %T has no Status field", key, objPtr)
	}

	var patchWrapper map[string]json.RawMessage
	err := json.Unmarshal(patch, &patchWrapper)
	if err != nil {
		return fmt.Errorf("failed to parse patch for %q as JSON object: %w", key, err)
	}
	statusPatchRaw, ok := patchWrapper["status"]
	if !ok {
		return fmt.Errorf("patch for %q does not contain a 'status' key", key)
	}

	statusInterface := statusField.Interface()
	originalStatusJSON, err := kjson.Marshal(statusInterface)
	if err != nil {
		return fmt.Errorf("failed to marshal original status for object %q: %w", key, err)
	}
	patchedStatusJSON, err := strategicpatch.StrategicMergePatch(originalStatusJSON, statusPatchRaw, statusInterface)
	if err != nil {
		return fmt.Errorf("failed to apply strategic merge patch for object %q: %w", key, err)
	}

	newStatusVal := reflect.New(statusField.Type())
	newStatusPtr := newStatusVal.Interface()
	if err := json.Unmarshal(patchedStatusJSON, newStatusPtr); err != nil {
		return fmt.Errorf("failed to unmarshal patched status for object %q: %w", key, err)
	}
	statusField.Set(newStatusVal.Elem())
	return nil
}

func parseLabelSelector(req *http.Request) (labels.Selector, error) {
	raw := req.URL.Query().Get("labelSelector")
	if raw == "" {
		return labels.Everything(), nil
	}
	return labels.Parse(raw)
}
