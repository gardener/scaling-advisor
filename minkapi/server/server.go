// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	rt "runtime"
	"strconv"
	"time"

	"github.com/gardener/scaling-advisor/minkapi/cli"
	"github.com/gardener/scaling-advisor/minkapi/server/configtmpl"
	"github.com/gardener/scaling-advisor/minkapi/server/store"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
	"github.com/gardener/scaling-advisor/minkapi/server/view"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/webutil"
	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	runtimejson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	kjson "k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

var _ mkapi.Server = (*InMemoryKAPI)(nil)

// InMemoryKAPI holds the in-memory stores, watch channels, and version tracking for simple implementation of mkapi.APIServer
type InMemoryKAPI struct {
	cfg                 mkapi.Config
	listenerAddr        net.Addr
	scheme              *runtime.Scheme
	rootMux             *http.ServeMux
	server              *http.Server
	baseView            mkapi.View
	createSandboxViewFn mkapi.CreateSandboxViewFunc
	sandboxViews        map[string]mkapi.View
}

// LaunchApp is a helper function used to parse cli args, construct, and start the MinKAPI server,
// embed this inside an App representing the binary process along with an application context and application cancel func.
//
// On success, returns an initialized App which holds the minkapi Server, the App Context (which has been setup for SIGINT and SIGTERM cancellation and holds a logger),
// and the Cancel func which callers are expected to defer in their main routines.
//
// On error, it will log the error to standard error and return the exitCode that callers are expected to exit the process with.
func LaunchApp(ctx context.Context) (app mkapi.App, exitCode int) {
	app.Ctx, app.Cancel = commoncli.CreateAppContext(ctx)
	log := logr.FromContextOrDiscard(app.Ctx).WithValues("program", mkapi.ProgramName)
	commoncli.PrintVersion(mkapi.ProgramName)
	cliOpts, err := cli.ParseProgramFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
		_, _ = fmt.Fprintf(os.Stderr, "Err: %v\n", err)
		exitCode = commoncli.ExitErrParseOpts
		return
	}
	app.Server, err = NewDefaultInMemory(log, cliOpts.Config)
	if err != nil {
		log.Error(err, "failed to initialize InMemoryKAPI")
		exitCode = commoncli.ExitErrStart
		return
	}
	// Begin the service in a goroutine
	go func() {
		if err := app.Server.Start(app.Ctx); err != nil {
			if errors.Is(err, mkapi.ErrStartFailed) {
				log.Error(err, "failed to start service")
			} else {
				log.Error(err, fmt.Sprintf("%s start failed", mkapi.ProgramName))
			}
		}
	}()
	return
}

func ShutdownApp(app *mkapi.App) (exitCode int) {
	// Create a context with a 5-second timeout for shutdown
	shutDownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	log := logr.FromContextOrDiscard(app.Ctx)

	// Perform shutdown
	if err := app.Server.Stop(shutDownCtx); err != nil {
		log.Error(err, fmt.Sprintf(" %s shutdown failed", mkapi.ProgramName))
		exitCode = commoncli.ExitErrShutdown
		return
	}
	log.Info(fmt.Sprintf("%s shutdown gracefully.", mkapi.ProgramName))
	exitCode = commoncli.ExitSuccess
	return
}

// NewDefaultInMemory constructs a KAPI server with default implementations of sub-components.
func NewDefaultInMemory(log logr.Logger, cfg mkapi.Config) (mkapi.Server, error) {
	scheme := typeinfo.SupportedScheme
	baseView, err := view.New(log, &mkapi.ViewArgs{
		Name:           mkapi.DefaultBasePrefix,
		KubeConfigPath: cfg.KubeConfigPath,
		Scheme:         scheme,
		WatchConfig:    cfg.WatchConfig,
	})
	// TODO: wrap errors with sentinel error code here.
	if err != nil {
		return nil, err
	}
	err = baseView.CreateObject(typeinfo.NamespacesDescriptor.GVK, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: corev1.NamespaceDefault,
		},
	})
	if err != nil {
		return nil, err
	}
	return NewInMemoryUsingViews(cfg, baseView, view.NewSandbox)
}

// NewInMemoryUsingViews constructs a KAPI server with the given base view and the sandbox view creation function.
func NewInMemoryUsingViews(cfg mkapi.Config, baseView mkapi.View, sandboxViewCreateFn mkapi.CreateSandboxViewFunc) (k mkapi.Server, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", mkapi.ErrInitFailed, err)
		}
	}()
	setMinKAPIConfigDefaults(&cfg)
	scheme := typeinfo.SupportedScheme
	rootMux := http.NewServeMux()
	s := &InMemoryKAPI{
		cfg:     cfg,
		scheme:  scheme,
		rootMux: rootMux,
		server: &http.Server{
			Addr: net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
			// G112 (CWE-400): Potential Slowloris Attack: kept it same as the one defined for http server started in the actual kube-apiserver.
			// See: https://github.com/kubernetes/kubernetes/blob/ad82c3d39f5e9f21e173ffeb8aa57953a0da4601/staging/src/k8s.io/apiserver/pkg/server/secure_serving.go#L172
			ReadHeaderTimeout: 32 * time.Second,
		},
		baseView:            baseView,
		createSandboxViewFn: sandboxViewCreateFn,
		sandboxViews:        make(map[string]mkapi.View),
	}
	// DO NOT REMOVE: Single route registration crap needed for kubectl compatability as it ignores server path prefixes
	// and always makes a call to http://localhost:8084/api/v1/?timeout=32s
	rootMux.HandleFunc("GET /api/v1/", s.handleAPIResources(typeinfo.SupportedCoreAPIResourceList))
	rootMux.HandleFunc("POST /views/{name}", s.handleCreateSandboxView)
	k = s
	return
}

// Start begins the MinKAPI server
func (k *InMemoryKAPI) Start(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	k.server.BaseContext = func(_ net.Listener) context.Context {
		return ctx
	}
	baseViewMux := http.NewServeMux()
	k.registerRoutes(log, baseViewMux, k.baseView)
	// Wrap the entire mux with the logger middleware
	serverHandler := webutil.LoggerMiddleware(log, k.rootMux)
	k.server.Handler = serverHandler
	// We do this because we want the bind address
	listener, err := net.Listen("tcp", k.server.Addr)
	if err != nil {
		return fmt.Errorf("%w: cannot listen on TCP Address %q: %w", mkapi.ErrStartFailed, k.server.Addr, err)
	}
	k.listenerAddr = listener.Addr()
	kapiURL := fmt.Sprintf("http://%s:%d/%s", k.cfg.Host, k.cfg.Port, k.cfg.BasePrefix)
	err = configtmpl.GenKubeConfig(configtmpl.KubeConfigParams{
		Name:           mkapi.DefaultBasePrefix,
		KubeConfigPath: k.cfg.KubeConfigPath,
		URL:            kapiURL,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", mkapi.ErrStartFailed, err)
	}
	log.Info("kubeconfig generated", "path", k.cfg.KubeConfigPath)

	schedulerTmplParams := configtmpl.KubeSchedulerTmplParams{
		KubeConfigPath:          k.cfg.KubeConfigPath,
		KubeSchedulerConfigPath: fmt.Sprintf("/tmp/%s-kube-scheduler-config.yaml", mkapi.ProgramName),
		QPS:                     100,
		Burst:                   50,
	}
	err = configtmpl.GenKubeSchedulerConfig(schedulerTmplParams)
	if err != nil {
		return fmt.Errorf("%w: %w", mkapi.ErrStartFailed, err)
	}
	log.Info("sample kube-scheduler-config generated", "path", schedulerTmplParams.KubeSchedulerConfigPath)
	log.Info(fmt.Sprintf("%s service listening", mkapi.ProgramName), "address", k.server.Addr, "kapiURL", kapiURL)
	if err := k.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%w: %w", mkapi.ErrServiceFailed, err)
	}
	return nil
}

// Stop shuts down the HTTP server and closes the base view
func (k *InMemoryKAPI) Stop(ctx context.Context) (err error) {
	var errs []error
	var cancel context.CancelFunc
	if k.cfg.GracefulShutdownTimeout.Duration > 0 {
		// It is possible that ctx is already a shutdown context where minkapi is embedded intoa  higher-level service
		// whose Stop has already created a shutdown context prior to invoking minkapi Stop
		// In such a case, it is expected that cfg.GracefulShutdownTimeout for minkapi would not be explicitly specified.
		ctx, cancel = context.WithTimeout(ctx, k.cfg.GracefulShutdownTimeout.Duration)
		defer cancel()
	}
	err = k.server.Shutdown(ctx) // shutdown server first to avoid accepting new requests.
	if err != nil {
		errs = append(errs, err)
	}
	err = k.baseView.Close()
	if err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		err = errors.Join(errs...)
	}
	return
}

func (k *InMemoryKAPI) GetBaseView() mkapi.View {
	return k.baseView
}

func (k *InMemoryKAPI) GetSandboxView(log logr.Logger, name string) (mkapi.View, error) {
	sandboxView, ok := k.sandboxViews[name] // TODO: protected with mutex.
	if ok {
		return sandboxView, nil
	}
	kapiURL := fmt.Sprintf("http://%s:%d/%s", k.cfg.Host, k.cfg.Port, name)
	_, err := url.Parse(kapiURL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid sandbox-kapi URI for view %q: %w", mkapi.ErrCreateSandbox, name, err)
	}
	baseKubeConfigDir := filepath.Dir(k.cfg.KubeConfigPath)
	kubeConfigPath := filepath.Join(baseKubeConfigDir, fmt.Sprintf("%s-%s.yaml", mkapi.ProgramName, name))
	log.Info("generating kubeconfig for sandbox", "name", name, "path", kubeConfigPath)
	err = configtmpl.GenKubeConfig(configtmpl.KubeConfigParams{
		Name:           name,
		KubeConfigPath: kubeConfigPath,
		URL:            kapiURL,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: cannot generate kubeconfig for view %q: %w", mkapi.ErrCreateSandbox, name, err)
	}
	log.Info("sandbox kubeconfig generated", "name", name, "path", k.cfg.KubeConfigPath)

	kubeSchedulerConfigPath := filepath.Join(baseKubeConfigDir, fmt.Sprintf("%s-%s-kube-scheduler-config.yaml", mkapi.ProgramName, name))
	schedulerTmplParams := configtmpl.KubeSchedulerTmplParams{
		KubeConfigPath:          kubeConfigPath,
		KubeSchedulerConfigPath: kubeSchedulerConfigPath,
		QPS:                     100, //TODO: pass this as param ?
		Burst:                   50,
	}
	err = configtmpl.GenKubeSchedulerConfig(schedulerTmplParams)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot generate kube-scheduler config for view %q: %w", mkapi.ErrStartFailed, name, err)
	}

	sandboxView, err = k.createSandboxViewFn(log, k.baseView, &mkapi.ViewArgs{
		Name:           name,
		KubeConfigPath: kubeConfigPath,
		Scheme:         k.scheme,
		WatchConfig:    k.cfg.WatchConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: cannot create sandbox view for view %q: %w", mkapi.ErrCreateSandbox, name, err)
	}
	sandboxViewMux := http.NewServeMux()
	k.registerRoutes(log, sandboxViewMux, sandboxView)
	k.sandboxViews[name] = sandboxView
	return sandboxView, nil
}

func (k *InMemoryKAPI) registerRoutes(log logr.Logger, viewMux *http.ServeMux, view mkapi.View) {
	// TODO: Design: Discuss this since this is not necessary when running as operator since operator has its own profiling enablement.
	if k.cfg.ProfilingEnabled {
		log.Info("profiling enabled - registering /debug/pprof/* handlers")
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
		k.registerResourceRoutes(viewMux, d, view)
	}
	// Register the view's mux under the pathPrefix, stripping the pathPrefix
	k.rootMux.Handle("/"+view.GetName()+"/", http.StripPrefix("/"+view.GetName(), viewMux))

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

func (k *InMemoryKAPI) registerResourceRoutes(viewMux *http.ServeMux, d typeinfo.Descriptor, view mkapi.View) {
	g := d.GVK.Group
	r := d.GVR.Resource
	if d.GVK.Group == "" {
		viewMux.HandleFunc(fmt.Sprintf("POST /api/v1/namespaces/{namespace}/%s", r), handleCreate(d, view))
		viewMux.HandleFunc(fmt.Sprintf("GET /api/v1/namespaces/{namespace}/%s", r), handleListOrWatch(d, view))
		viewMux.HandleFunc(fmt.Sprintf("GET /api/v1/namespaces/{namespace}/%s/{name}", r), handleGet(d, view))
		viewMux.HandleFunc(fmt.Sprintf("PATCH /api/v1/namespaces/{namespace}/%s/{name}", r), handlePatch(d, view))
		viewMux.HandleFunc(fmt.Sprintf("PATCH /api/v1/namespaces/{namespace}/%s/{name}/status", r), handlePatchStatus(d, view))
		viewMux.HandleFunc(fmt.Sprintf("DELETE /api/v1/namespaces/{namespace}/%s/{name}", r), handleDelete(d, view))
		viewMux.HandleFunc(fmt.Sprintf("PUT /api/v1/namespaces/{namespace}/%s/{name}", r), handlePut(d, view))        // Update
		viewMux.HandleFunc(fmt.Sprintf("PUT /api/v1/namespaces/{namespace}/%s/{name}/status", r), handlePut(d, view)) // UpdateStatus

		if d.GetKind() == typeinfo.PodsDescriptor.GetKind() {
			viewMux.HandleFunc("POST /api/v1/namespaces/{namespace}/pods/{name}/binding", handleCreatePodBinding(view))
		}

		viewMux.HandleFunc(fmt.Sprintf("POST /api/v1/%s", r), handleCreate(d, view))
		viewMux.HandleFunc(fmt.Sprintf("GET /api/v1/%s", r), handleListOrWatch(d, view))
		viewMux.HandleFunc(fmt.Sprintf("GET /api/v1/%s/{name}", r), handleGet(d, view))
		viewMux.HandleFunc(fmt.Sprintf("PATCH /api/v1/%s/{name}", r), handlePatch(d, view))
		viewMux.HandleFunc(fmt.Sprintf("DELETE /api/v1/%s/{name}", r), handleDelete(d, view))
		viewMux.HandleFunc(fmt.Sprintf("PUT /api/v1/%s/{name}", r), handlePut(d, view))        // Update
		viewMux.HandleFunc(fmt.Sprintf("PUT /api/v1/%s/{name}/status", r), handlePut(d, view)) // UpdateStatus
	} else {
		viewMux.HandleFunc(fmt.Sprintf("POST /apis/%s/v1/namespaces/{namespace}/%s", g, r), handleCreate(d, view))
		viewMux.HandleFunc(fmt.Sprintf("GET /apis/%s/v1/namespaces/{namespace}/%s", g, r), handleListOrWatch(d, view))
		viewMux.HandleFunc(fmt.Sprintf("GET /apis/%s/v1/namespaces/{namespace}/%s/{name}", g, r), handleGet(d, view))
		viewMux.HandleFunc(fmt.Sprintf("PATCH /apis/%s/v1/namespaces/{namespace}/%s/{name}", g, r), handlePatch(d, view))
		viewMux.HandleFunc(fmt.Sprintf("DELETE /apis/%s/v1/namespaces/{namespace}/%s/{name}", g, r), handleDelete(d, view))
		viewMux.HandleFunc(fmt.Sprintf("PUT /apis/%s/v1/namespaces/{namespace}/%s/{name}", g, r), handlePut(d, view))

		viewMux.HandleFunc(fmt.Sprintf("POST /apis/%s/v1/%s", g, r), handleCreate(d, view))
		viewMux.HandleFunc(fmt.Sprintf("GET /apis/%s/v1/%s", g, r), handleListOrWatch(d, view))
		viewMux.HandleFunc(fmt.Sprintf("GET /apis/%s/v1/%s/{name}", g, r), handleGet(d, view))
		viewMux.HandleFunc(fmt.Sprintf("DELETE /apis/%s/v1/%s/{name}", g, r), handleDelete(d, view))
	}
}

// handleAPIGroups returns the list of supported API groups
func (k *InMemoryKAPI) handleAPIGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJsonResponse(w, r, &typeinfo.SupportedAPIGroups)
}

// handleAPIVersions returns the list of versions for the core API group
func (k *InMemoryKAPI) handleAPIVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJsonResponse(w, r, &typeinfo.SupportedAPIVersions)
}

func (k *InMemoryKAPI) handleAPIResources(apiResourceList metav1.APIResourceList) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJsonResponse(w, r, apiResourceList)
	}
}

func (k *InMemoryKAPI) handleCreateSandboxView(w http.ResponseWriter, r *http.Request) {
	viewName := r.PathValue("name")
	if viewName == "" {
		handleStatusError(w, r, apierrors.NewBadRequest("sandbox view name is required"))
		return
	}
	log := logr.FromContextOrDiscard(r.Context())
	_, err := k.GetSandboxView(log, viewName)
	if err != nil {
		handleInternalServerError(w, r, err)
		return
	}
	log.Info("sandbox view created and sandbox view API Server routes registered", "viewName", viewName)
	statusOK := &metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status"},
		Status:   metav1.StatusSuccess,
		Code:     http.StatusCreated,
		Message:  fmt.Sprintf("sandbox view %q created and routes registered", viewName),
	}
	writeJsonResponse(w, r, statusOK)
}

func handleGet(d typeinfo.Descriptor, view mkapi.View) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := GetObjectName(r, d)
		obj, err := view.GetObject(d.GVK, name)
		if err != nil {
			handleError(w, r, err)
			return
		}
		writeJsonResponse(w, r, obj)
	}
}

func handleCreate(d typeinfo.Descriptor, view mkapi.View) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			mo  metav1.Object
			err error
		)
		mo, err = d.CreateObject()
		if err != nil {
			err = fmt.Errorf("cannot create object from objGvk %q: %v", d.GVK, err)
			handleInternalServerError(w, r, err)
			return
		}

		if !readBodyIntoObj(w, r, mo) {
			return
		}

		var namespace string
		if mo.GetNamespace() == "" {
			namespace = GetObjectName(r, d).Namespace
			mo.SetNamespace(namespace)
		}
		err = view.CreateObject(d.GVK, mo)
		if err != nil {
			handleError(w, r, err)
			return
		}
		writeJsonResponse(w, r, mo)
	}
}

// handlePut Ref: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#considerations-for-put-operations (TODO ensure handlePut follows this)
// TODO: handlePut is not complete
func handlePut(d typeinfo.Descriptor, view mkapi.View) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := GetObjectName(r, d)
		obj, err := view.GetObject(d.GVK, name)
		if err != nil {
			handleError(w, r, err)
			return
		}
		if !readBodyIntoObj(w, r, obj) {
			return
		}
		metaObj := obj.(metav1.Object)
		err = view.UpdateObject(d.GVK, metaObj)
		if err != nil {
			handleError(w, r, err)
			return
		}
		writeJsonResponse(w, r, obj)
	}
}

func handleDelete(d typeinfo.Descriptor, view mkapi.View) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		objName := GetObjectName(r, d)
		obj, err := view.GetObject(d.GVK, objName)
		if err != nil {
			handleError(w, r, err)
			return
		}
		mo, err := meta.Accessor(obj)
		if err != nil {
			handleError(w, r, fmt.Errorf("stored object with key %q is not metav1.Object: %w", objName, err))
			return
		}
		err = view.DeleteObject(d.GVK, objName)
		if err != nil {
			handleError(w, r, err)
			return
		}
		status := metav1.Status{
			TypeMeta: metav1.TypeMeta{ //No idea why this is explicitly needed just for this payload, but kubectl complains if missing.
				Kind:       "Status",
				APIVersion: "v1",
			},
			Status: metav1.StatusSuccess,
			Details: &metav1.StatusDetails{
				Name: objName.String(),
				Kind: d.GVR.GroupResource().Resource,
				UID:  mo.GetUID(),
			},
		}
		writeJsonResponse(w, r, &status)
	}
}

func handleListOrWatch(d typeinfo.Descriptor, view mkapi.View) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		isWatch := query.Get("watch")
		var delegate http.HandlerFunc

		labelSelector, err := parseLabelSelector(r)
		if err != nil {
			handleBadRequest(w, r, err)
			return
		}

		if isWatch == "true" || isWatch == "1" {
			delegate = handleWatch(d, view, labelSelector)
		} else {
			delegate = handleList(d, view, labelSelector)
		}
		delegate.ServeHTTP(w, r)
	}
}

func handleList(d typeinfo.Descriptor, view mkapi.View, labelSelector labels.Selector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		namespace := r.PathValue("namespace")
		c := mkapi.MatchCriteria{Namespace: namespace, LabelSelector: labelSelector}
		listObj, err := view.ListObjects(d.GVK, c) //s.List(c)
		if err != nil {
			return
		}
		writeJsonResponse(w, r, listObj)
	}
}

func handlePatch(d typeinfo.Descriptor, view mkapi.View) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := GetObjectName(r, d)
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/strategic-merge-patch+json" && contentType != "application/merge-patch+json" {
			err := fmt.Errorf("unsupported content type %q for object %q", contentType, name)
			handleBadRequest(w, r, err)
			return
		}
		patchData, err := io.ReadAll(r.Body)
		if err != nil {
			statusErr := apierrors.NewInternalError(err)
			writeStatusError(w, r, statusErr)
			return
		}
		patchedObj, err := view.PatchObject(d.GVK, name, types.PatchType(contentType), patchData)
		if err != nil {
			handleError(w, r, err)
			return
		}
		writeJsonResponse(w, r, patchedObj)
	}
}

func handlePatchStatus(d typeinfo.Descriptor, view mkapi.View) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		objName := GetObjectName(r, d)
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/strategic-merge-patch+json" {
			err := fmt.Errorf("unsupported content type %q for o %q", contentType, objName)
			handleBadRequest(w, r, err)
			return
		}

		patchData, err := io.ReadAll(r.Body)
		if err != nil {
			err = fmt.Errorf("failed to read patch body for o %q", objName)
			handleInternalServerError(w, r, err)
			return
		}

		patchedObj, err := view.PatchObjectStatus(d.GVK, objName, patchData)
		if err != nil {
			handleError(w, r, err)
			return
		}
		writeJsonResponse(w, r, patchedObj)
	}
}

// handleWatch implements watch request/response handling. It delegates watch functionality to the given mkapi.View, only
// passing a callback which encodes the watch event and flushed it to the response stream.
func handleWatch(d typeinfo.Descriptor, view mkapi.View, labelSelector labels.Selector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			ok           bool
			startVersion int64
			namespace    string
		)

		namespace = r.PathValue("namespace")
		startVersion, ok = getParseResourceVersion(w, r)
		if !ok {
			return
		}

		flusher := getFlusher(w)
		if flusher == nil {
			return
		}
		flusher.Flush() // 🚨important! unblocks client-go I/O so that it can construct a watcher!

		log := logr.FromContextOrDiscard(r.Context())
		err := view.WatchObjects(r.Context(), d.GVK, startVersion, namespace, labelSelector, func(event watch.Event) error {
			metaObj, err := store.AsMeta(event.Object)
			if err != nil {
				return err
			}
			eventJson, err := buildWatchEventJsonAlt(log, &event)
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
			log.Error(err, "watch failed", "gvk", d.GVK, "namespace", namespace, "startVersion", startVersion, "labelSelector", labelSelector)
		}
	}
}

// handleCreatePodBinding is meant to handle creation for a Pod binding.
// Ex: POST http://localhost:8080/api/v1/namespaces/default/pods/a-mc6zl/binding
// This endpoint is invoked by the scheduler, and it is expected that the API HostPort sets the `pod.Spec.NodeName`
//
// Example Payload
// {"kind":"Binding","apiVersion":"v1","metadata":{"name":"a-p4r2l","namespace":"default","uid":"b8124ee8-a0c7-4069-930d-fc5e901675d3"},"target":{"kind":"Node","name":"a-kl827"}}
func handleCreatePodBinding(view mkapi.View) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logr.FromContextOrDiscard(r.Context())
		d := typeinfo.PodsDescriptor
		binding := corev1.Binding{}
		if !readBodyIntoObj(w, r, &binding) {
			return
		}
		podName := GetObjectName(r, d)
		//obj, err := view.GetObject(d.GVK, objName)
		//if err != nil {
		//	handleError(w, r, err)
		//	return
		//}
		//pod := obj.(*corev1.Pod)
		//pod.Spec.NodeName = binding.Target.InstanceType
		//podutil.UpdatePodCondition(&pod.Status, &corev1.PodCondition{
		//	Type:   corev1.PodScheduled,
		//	Status: corev1.ConditionTrue,
		//})
		pod, err := view.UpdatePodNodeBinding(podName, binding)
		if err != nil {
			log.Error(err, "cannot assign pod to node", "podName", podName, "nodeName", binding.Target.Name)
			handleError(w, r, err)
			return
		}
		log.V(3).Info("assigned pod to node", "podName", podName, "nodeName", pod.Spec.NodeName)
		// Return {"kind":"Status","apiVersion":"v1","metadata":{},"status":"Success","code":201}
		statusOK := &metav1.Status{
			TypeMeta: metav1.TypeMeta{Kind: "Status"},
			Status:   metav1.StatusSuccess,
			Code:     http.StatusCreated,
		}
		writeJsonResponse(w, r, statusOK)
	}
}

func writeStatusError(w http.ResponseWriter, r *http.Request, statusError *apierrors.StatusError) {
	w.WriteHeader(int(statusError.ErrStatus.Code))
	writeJsonResponse(w, r, statusError.ErrStatus)
}

func readBodyIntoObj(w http.ResponseWriter, r *http.Request, obj any) (ok bool) {
	log := logr.FromContextOrDiscard(r.Context())
	data, err := io.ReadAll(r.Body)
	if err != nil {
		handleBadRequest(w, r, err)
		ok = false
		return
	}
	if err := json.Unmarshal(data, obj); err != nil {
		err = fmt.Errorf("cannot unmarshal JSON for request %q: %w", r.RequestURI, err)
		log.Error(err, "cannot unmarshal JSON for request body", "payload", string(data))
		handleBadRequest(w, r, err)
		ok = false
		return
	}
	if log.V(4).Enabled() {
		log.V(4).Info("read payload into object", "payload", string(data))
	}
	ok = true
	return
}

func getParseResourceVersion(w http.ResponseWriter, r *http.Request) (resourceVersion int64, ok bool) {
	paramValue := r.URL.Query().Get("resourceVersion")
	if paramValue == "" {
		ok = true
		resourceVersion = 0
		return
	}
	resourceVersion, err := objutil.ParseResourceVersion(paramValue)
	if err != nil {
		handleBadRequest(w, r, err)
		return
	}
	ok = true
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

func buildWatchEventJsonAlt(log logr.Logger, ev *watch.Event) (string, error) {
	sch := typeinfo.SupportedScheme
	s := runtimejson.NewSerializerWithOptions(
		runtimejson.DefaultMetaFactory, sch, sch,
		runtimejson.SerializerOptions{Yaml: false, Pretty: false, Strict: false})

	mev := &metav1.WatchEvent{
		Type: string(ev.Type),
		Object: runtime.RawExtension{
			Object: ev.Object,
		},
	}
	var buf bytes.Buffer
	err := s.Encode(mev, &buf)
	if err != nil {
		log.Error(err, "cannot encode watch event", "event", ev)
		return "", err
	}
	payload := buf.String()
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

func parseLabelSelector(req *http.Request) (labels.Selector, error) {
	raw := req.URL.Query().Get("labelSelector")
	if raw == "" {
		return labels.Everything(), nil
	}
	return labels.Parse(raw)
}

func setMinKAPIConfigDefaults(cfg *mkapi.Config) {
	if cfg.WatchConfig.QueueSize <= 0 {
		cfg.WatchConfig.QueueSize = mkapi.DefaultWatchQueueSize
	}
	if cfg.WatchConfig.Timeout <= 0 {
		cfg.WatchConfig.Timeout = mkapi.DefaultWatchTimeout
	}
	if cfg.KubeConfigPath == "" {
		cfg.KubeConfigPath = mkapi.DefaultKubeConfigPath
	}
	if cfg.Port == 0 {
		cfg.Port = commonconstants.DefaultMinKAPIPort
	}
}

func handleError(w http.ResponseWriter, r *http.Request, err error) {
	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		handleStatusError(w, r, statusErr)
	} else {
		handleInternalServerError(w, r, err)
	}
}

func handleStatusError(w http.ResponseWriter, r *http.Request, statusErr *apierrors.StatusError) {
	log := logr.FromContextOrDiscard(r.Context())
	log.Error(statusErr, "status error", "gvk", statusErr.ErrStatus.GroupVersionKind, "code", statusErr.ErrStatus.Code, "reason", statusErr.ErrStatus.Reason, "message", statusErr.ErrStatus.Message)
	w.WriteHeader(int(statusErr.ErrStatus.Code))
	w.Header().Set("Content-Type", "application/json")
	writeJsonResponse(w, r, statusErr.ErrStatus)
}

func handleInternalServerError(w http.ResponseWriter, r *http.Request, err error) {
	log := logr.FromContextOrDiscard(r.Context())
	statusErr := apierrors.NewInternalError(err)
	log.Error(err, "internal server error")
	w.WriteHeader(http.StatusInternalServerError)
	w.Header().Set("Content-Type", "application/json")
	writeJsonResponse(w, r, statusErr.ErrStatus)
}

func handleBadRequest(w http.ResponseWriter, r *http.Request, err error) {
	log := logr.FromContextOrDiscard(r.Context())
	err = fmt.Errorf("cannot handle request %q: %w", r.Method+" "+r.RequestURI, err)
	log.Error(err, "bad request", "method", r.Method, "requestURI", r.RequestURI)
	statusErr := apierrors.NewBadRequest(err.Error())
	w.WriteHeader(http.StatusBadRequest)
	w.Header().Set("Content-Type", "application/json")
	writeJsonResponse(w, r, statusErr.ErrStatus)
}

// writeJsonResponse sets Content-Type to application/json  and encodes the object to the response writer.
func writeJsonResponse(w http.ResponseWriter, r *http.Request, obj any) {
	log := logr.FromContextOrDiscard(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(obj); err != nil {
		log.Error(err, "cannot  encode response", "obj", obj)
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}
