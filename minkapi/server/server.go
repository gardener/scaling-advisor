// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package server

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
	"time"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	kjson "k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/tools/cache"

	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/server/configtmpl"
	"github.com/gardener/scaling-advisor/minkapi/server/podutil"
	"github.com/gardener/scaling-advisor/minkapi/server/store"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
	"github.com/gardener/scaling-advisor/minkapi/server/view"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/watch"
)

var _ api.Server = (*InMemoryKAPI)(nil)

// InMemoryKAPI holds the in-memory stores, watch channels, and version tracking for simple implementation of api.APIServer
type InMemoryKAPI struct {
	cfg          api.MinKAPIConfig
	listenerAddr net.Addr
	scheme       *runtime.Scheme
	rootMux      *http.ServeMux
	server       *http.Server
	baseView     api.View
	log          logr.Logger
}

func NewInMemory(log logr.Logger, cfg api.MinKAPIConfig) (k api.Server, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", api.ErrInitFailed, err)
		}
	}()
	setMinKAPIConfigDefaults(&cfg)
	scheme := typeinfo.SupportedScheme
	baseView, err := view.New(log, cfg.KubeConfigPath, scheme, cfg.WatchQueueSize, cfg.WatchTimeout)
	if err != nil {
		return
	}
	rootMux := http.NewServeMux()
	s := &InMemoryKAPI{
		cfg:     cfg,
		scheme:  scheme,
		rootMux: rootMux,
		server: &http.Server{
			Addr:    net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
			Handler: rootMux,
		},
		baseView: baseView,
		log:      log,
	}
	baseViewMux := http.NewServeMux()
	s.registerRoutes(baseViewMux, cfg.BasePrefix)
	k = s
	return
}

// Start begins the HTTP server
func (k *InMemoryKAPI) Start(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	k.server.BaseContext = func(_ net.Listener) context.Context {
		return ctx
	}
	// We do this because we want the bind address
	listener, err := net.Listen("tcp", k.server.Addr)
	if err != nil {
		return fmt.Errorf("%w: cannot listen on TCP Address %q: %w", api.ErrStartFailed, k.server.Addr, err)
	}
	k.listenerAddr = listener.Addr()
	kapiURL := fmt.Sprintf("http://localhost:%d/%s", k.cfg.Port, k.cfg.BasePrefix)
	err = configtmpl.GenKubeConfig(configtmpl.KubeConfigParams{
		KubeConfigPath: k.cfg.KubeConfigPath,
		URL:            kapiURL,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", api.ErrStartFailed, err)
	}
	log.Info("kubeconfig generated", "path", k.cfg.KubeConfigPath)

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
	log.Info("sample kube-scheduler-config generated", "path", schedulerTmplParams.KubeSchedulerConfigPath)
	k.log.Info(fmt.Sprintf("%s service listening", api.ProgramName), "address", k.server.Addr, "kapiURL", kapiURL)
	if err := k.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%w: %w", api.ErrServiceFailed, err)
	}
	return nil
}

// Stop  shuts down the HTTP server and closes resources
func (k *InMemoryKAPI) Stop(ctx context.Context) (err error) {
	err = k.server.Shutdown(ctx) // shutdown server first to avoid accepting new requests.
	k.baseView.Close()
	return
}

func (k *InMemoryKAPI) GetBaseView() api.View {
	return k.baseView
}

func (k *InMemoryKAPI) GetSandboxView(pathPrefix string) (api.View, error) {
	// TODO replace with SchedulerView
	return view.New(k.log, k.cfg.KubeConfigPath, k.scheme, k.cfg.WatchQueueSize, k.cfg.WatchTimeout)
}

func (k *InMemoryKAPI) GetMux() *http.ServeMux {
	return k.rootMux
}

func (k *InMemoryKAPI) registerRoutes(viewMux *http.ServeMux, pathPrefix string) {
	if k.cfg.ProfilingEnabled {
		k.log.Info("profiling enabled - registering /debug/pprof/* handlers")
		viewMux.HandleFunc("/debug/pprof/", pprof.Index)
		viewMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		viewMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		viewMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		viewMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		viewMux.HandleFunc("/trigger-gc", func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintln(w, "GC Triggering")
			rt.GC() // force garbage collection
			_, _ = fmt.Fprintln(w, "GC Triggered")
		})
	}

	viewMux.HandleFunc("GET /api", k.handleAPIVersions)
	viewMux.HandleFunc("GET /apis", k.handleAPIGroups)

	// Core API Group and Other API Groups
	k.registerAPIGroups(viewMux)

	for _, d := range typeinfo.SupportedDescriptors {
		k.registerResourceRoutes(viewMux, d)
	}
	// Register the view's mux under the pathPrefix, stripping the pathPrefix
	k.rootMux.Handle("/"+pathPrefix+"/", http.StripPrefix("/"+pathPrefix, viewMux))

	// DO NOT REMOVE: Crap needed for kubectl compatability as it ignores base paths and always makes a call to http://localhost:8084/api/v1/?timeout=32s
	k.rootMux.HandleFunc("GET /api/v1/", k.handleAPIResources(typeinfo.SupportedCoreAPIResourceList))
}

func (k *InMemoryKAPI) registerAPIGroups(viewMux *http.ServeMux) {
	// Core API
	viewMux.HandleFunc("GET /api/v1/", k.handleAPIResources(typeinfo.SupportedCoreAPIResourceList))

	// API groups
	for _, apiList := range typeinfo.SupportedGroupAPIResourceLists {
		route := fmt.Sprintf("GET /apis/%s/", apiList.APIResources[0].Group)
		viewMux.HandleFunc(route, k.handleAPIResources(apiList))
	}
}

func (k *InMemoryKAPI) registerResourceRoutes(viewMux *http.ServeMux, d typeinfo.Descriptor) {
	g := d.GVK.Group
	r := d.GVR.Resource
	if d.GVK.Group == "" {
		viewMux.HandleFunc(fmt.Sprintf("POST /api/v1/namespaces/{namespace}/%s", r), k.handleCreate(d))
		viewMux.HandleFunc(fmt.Sprintf("GET /api/v1/namespaces/{namespace}/%s", r), k.handleListOrWatch(d))
		viewMux.HandleFunc(fmt.Sprintf("GET /api/v1/namespaces/{namespace}/%s/{name}", r), k.handleGet(d))
		viewMux.HandleFunc(fmt.Sprintf("PATCH /api/v1/namespaces/{namespace}/%s/{name}", r), k.handlePatch(d))
		viewMux.HandleFunc(fmt.Sprintf("PATCH /api/v1/namespaces/{namespace}/%s/{name}/status", r), k.handlePatchStatus(d))
		viewMux.HandleFunc(fmt.Sprintf("DELETE /api/v1/namespaces/{namespace}/%s/{name}", r), k.handleDelete(d))
		viewMux.HandleFunc(fmt.Sprintf("PUT /api/v1/namespaces/{namespace}/%s/{name}", r), k.handlePut(d))        // Update
		viewMux.HandleFunc(fmt.Sprintf("PUT /api/v1/namespaces/{namespace}/%s/{name}/status", r), k.handlePut(d)) // UpdateStatus

		if d.Kind == typeinfo.PodsDescriptor.Kind {
			viewMux.HandleFunc("POST /api/v1/namespaces/{namespace}/pods/{name}/binding", k.handleCreatePodBinding)
		}

		viewMux.HandleFunc(fmt.Sprintf("POST /api/v1/%s", r), k.handleCreate(d))
		viewMux.HandleFunc(fmt.Sprintf("GET /api/v1/%s", r), k.handleListOrWatch(d))
		viewMux.HandleFunc(fmt.Sprintf("GET /api/v1/%s/{name}", r), k.handleGet(d))
		viewMux.HandleFunc(fmt.Sprintf("PATCH /api/v1/%s/{name}", r), k.handlePatch(d))
		viewMux.HandleFunc(fmt.Sprintf("DELETE /api/v1/%s/{name}", r), k.handleDelete(d))
		viewMux.HandleFunc(fmt.Sprintf("PUT /api/v1/%s/{name}", r), k.handlePut(d))        // Update
		viewMux.HandleFunc(fmt.Sprintf("PUT /api/v1/%s/{name}/status", r), k.handlePut(d)) // UpdateStatus
	} else {
		viewMux.HandleFunc(fmt.Sprintf("POST /apis/%s/v1/namespaces/{namespace}/%s", g, r), k.handleCreate(d))
		viewMux.HandleFunc(fmt.Sprintf("GET /apis/%s/v1/namespaces/{namespace}/%s", g, r), k.handleListOrWatch(d))
		viewMux.HandleFunc(fmt.Sprintf("GET /apis/%s/v1/namespaces/{namespace}/%s/{name}", g, r), k.handleGet(d))
		viewMux.HandleFunc(fmt.Sprintf("PATCH /apis/%s/v1/namespaces/{namespace}/%s/{name}", g, r), k.handlePatch(d))
		viewMux.HandleFunc(fmt.Sprintf("DELETE /apis/%s/v1/namespaces/{namespace}/%s/{name}", g, r), k.handleDelete(d))
		viewMux.HandleFunc(fmt.Sprintf("PUT /apis/%s/v1/namespaces/{namespace}/%s/{name}", g, r), k.handlePut(d))

		viewMux.HandleFunc(fmt.Sprintf("POST /apis/%s/v1/%s", g, r), k.handleCreate(d))
		viewMux.HandleFunc(fmt.Sprintf("GET /apis/%s/v1/%s", g, r), k.handleListOrWatch(d))
		viewMux.HandleFunc(fmt.Sprintf("GET /apis/%s/v1/%s/{name}", g, r), k.handleGet(d))
		viewMux.HandleFunc(fmt.Sprintf("DELETE /apis/%s/v1/%s/{name}", g, r), k.handleDelete(d))
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

		if isWatch == "true" || isWatch == "1" { // FIXME : should check "1" as well
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

func (k *InMemoryKAPI) getStoreOrWriteError(gvk schema.GroupVersionKind, w http.ResponseWriter, r *http.Request) (s api.ResourceStore) {
	s, err := k.baseView.GetResourceStore(gvk)
	if err != nil {
		k.log.Error(err, "store error", "gvk", gvk)
		k.handleInternalServerError(w, r, err)
	}
	return s
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

func parseLabelSelector(req *http.Request) (labels.Selector, error) {
	raw := req.URL.Query().Get("labelSelector")
	if raw == "" {
		return labels.Everything(), nil
	}
	return labels.Parse(raw)
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

func setMinKAPIConfigDefaults(cfg *api.MinKAPIConfig) {
	if cfg.WatchQueueSize <= 0 {
		cfg.WatchQueueSize = api.DefaultWatchQueueSize
	}
	if cfg.WatchTimeout <= 0 {
		cfg.WatchTimeout = api.DefaultWatchTimeout
	}
	if cfg.KubeConfigPath == "" {
		cfg.KubeConfigPath = api.DefaultKubeConfigPath
	}
	if cfg.Port == 0 {
		cfg.Port = commonconstants.DefaultMinKAPIPort
	}
}
