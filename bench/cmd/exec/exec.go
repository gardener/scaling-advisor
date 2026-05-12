// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"
	"text/template"
	"time"

	pricingapi "github.com/gardener/scaling-advisor/api/pricing"
	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"
	"github.com/gardener/scaling-advisor/pricing"

	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kwok"
)

//go:embed templates/*.yaml
var content embed.FS

const prometheusPort = 2112

// ScalerEventConfig describes the events a scaler emits and which ones
// indicate a pod has been deemed unschedulable.
type ScalerEventConfig struct {
	Source                string   // Event source to match (e.g. "karpenter", "cluster-autoscaler")
	EventNames            []string // Event names to watch for (e.g. "FailedScheduling", "NodeCreated", "PodScheduled")
	MarksPodUnschedulable []string // Subset of EventNames that mark a pod as unschedulable
}

// ExecScaler is the interface that every scaler backend must implement to
// participate in a benchmark run.
type ExecScaler interface {
	// DeployScalerData creates the scaler-specific Kubernetes objects (CRDs,
	// ConfigMaps, NodePools, etc.) in the KWOK cluster.
	DeployScalerData(ctx context.Context, cfg *envconf.Config, scenarioDir string) error

	// GetScalerKWOKTemplatePath returns the embedded-FS path to the
	// kwokctl configuration template for this scaler.
	GetScalerKWOKTemplatePath() string

	// CheckRequiredDataPresent verifies that everything produced by
	// "setup" (files + Docker images) is available before the cluster
	// is created.
	CheckRequiredDataPresent(scenarioDir, version string) error

	// EventConfig returns the scaler-specific event configuration.
	EventConfig() ScalerEventConfig
}

// ExecArgs has the flag variables — bound by cobra, read once in execCmd.RunE, then
// passed explicitly to all callees so that no other function touches these globals.
type ExecArgs struct {
	Scaler        string
	SnapshotFile  string
	ScalerVersion string
	PricingFile   string
	SkipCleanup   bool
	WaitForCancel bool
}

// NewExecCommand runs a scaler inside a KWOK cluster populated with data
// from the cluster snapshot. It is the counterpart to "setup", which prepares
// the resources and deploys the scaler image that this command consumes.
func NewExecCommand(ctx context.Context) *cobra.Command {
	var execArgs ExecArgs
	var execCmd = &cobra.Command{
		Use:   "exec <scaler> <options>",
		Short: "Run the scaler by utilizing the data and produce the report",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, cmdArgs []string) (err error) {
			// Only the scaler is passed as an argument to the command, rest are all flags
			execArgs.Scaler = cmdArgs[0]

			// FIXME: this might need checking
			execCtx := benchutil.SetupSignalHandler()
			execCtx, err = Run(execCtx, execArgs)
			if err != nil {
				return
			}
			return
		},
	}

	// Initialise the exec args with the passed flag values,
	// falling back to default if nothing specified
	execCmd.PersistentFlags().StringVar(
		&execArgs.SnapshotFile,
		"snap", "",
		"cluster snapshot file",
	)
	_ = execCmd.MarkFlagRequired("snap")
	_ = execCmd.MarkFlagFilename("snap", "json")

	execCmd.PersistentFlags().BoolVarP(
		&execArgs.SkipCleanup,
		"skip-cleanup", "s", false,
		"skip deleting cluster with all data upon finishing",
	)

	execCmd.PersistentFlags().BoolVarP(
		&execArgs.WaitForCancel,
		"wait-for-cancel", "w", false,
		"wait for cancel signal after scaling completes before writing report",
	)

	execCmd.PersistentFlags().StringVarP(
		&execArgs.ScalerVersion,
		"scaler-version", "v", "main",
		"version of the scaler to fetch",
	)

	execCmd.PersistentFlags().StringVarP(
		&execArgs.PricingFile,
		"pricing-data", "p", "",
		"pricing data file",
	)
	_ = execCmd.MarkFlagRequired("pricing-data")

	return execCmd
}

// Run sets up the cluster for executing the benchmarking operation by creating a docker
// runtime cluster using 'kwokctl' (this includes control plane components and the scaler).
// Then it deploys all the objects present in the snapshot in the cluster alongwith the
// artefacts produced by the "setup" subcommand.
func Run(execCtx context.Context, args ExecArgs) (ctx context.Context, err error) {
	scenarioDir, err := filepath.Abs(path.Dir(args.SnapshotFile))
	if err != nil {
		return nil, fmt.Errorf("cannot resolve scenario directory: %w", err)
	}

	scaler, err := getScaler(args.Scaler)
	if err != nil {
		return nil, fmt.Errorf("cannot get scaler: %w", err)
	}

	pricingData, err := pricing.GetInstancePricingAccess("dummy-provider", args.PricingFile)
	if err != nil {
		return nil, fmt.Errorf("error parsing pricing data: %v", err)
	}

	err = scaler.CheckRequiredDataPresent(scenarioDir, args.ScalerVersion)
	if err != nil {
		return nil, fmt.Errorf("please run 'setup' before running 'exec': %v", err)
	}

	kwokClusterName := envconf.RandomName("kwok-cluster", 17)

	// FIXME: what is with this arguments blow-up
	execCtx, cfg, promConfigPath, err := setupClusterForScaling(execCtx, scaler, kwokClusterName, scenarioDir, args.ScalerVersion)
	if err != nil {
		return nil, err
	}
	// FIXME: aaaaaaaaaaaaaargggggggggggghhhhhhh
	defer cleanupCluster(execCtx, cfg, kwokClusterName, scenarioDir, promConfigPath, args.SkipCleanup)

	clusterSnapshot, err := benchutil.LoadJSONFromFile[planner.ClusterSnapshot](args.SnapshotFile)
	if err != nil {
		return nil, fmt.Errorf("cannot load cluster snapshot: %v", err)
	}

	if err := deployObjects(execCtx, cfg, clusterSnapshot); err != nil {
		return nil, fmt.Errorf("error running KWOK cluster: %v", err)
	}
	if err := scaler.DeployScalerData(execCtx, cfg, scenarioDir); err != nil {
		return nil, fmt.Errorf("error deploying the scaler data: %v", err)
	}

	scheduled, unscheduled := partitionPods(clusterSnapshot.Pods)

	log.Printf("Deploying scheduled pods, count %d...", len(scheduled))
	if err := deployPods(execCtx, cfg, scheduled); err != nil {
		return nil, fmt.Errorf("error deploying scheduled pods: %v", err)
	}
	log.Printf("Deployed all %d scheduled pods", len(scheduled))

	meta := RunMetadata{
		StartTime:     time.Now(),
		ScalerName:    args.Scaler,
		ScalerVersion: args.ScalerVersion,
		SnapshotFile:  args.SnapshotFile,
	}
	meta.Summary.ClusterState.Before = ClusterStats{
		NodeCount:       len(clusterSnapshot.Nodes),
		ScheduledPods:   len(scheduled),
		UnscheduledPods: len(unscheduled),
	}

	mon, err := newMonitor(execCtx, cfg, &meta, kwokClusterName, scenarioDir)
	if err != nil {
		return nil, fmt.Errorf("monitoring setup failed: %v", err)
	}
	mon.waitForCancel = args.WaitForCancel

	if err := mon.start(execCtx, scaler.EventConfig()); err != nil {
		return nil, fmt.Errorf("monitoring start failed: %v", err)
	}
	defer mon.ec.Stop()

	log.Printf("Deploying unscheduled pods, count %d...", len(unscheduled))
	if err := deployPods(execCtx, cfg, unscheduled); err != nil {
		return nil, fmt.Errorf("error deploying unscheduled pods: %v", err)
	}
	log.Printf("Deployed all %d unscheduled pods", len(unscheduled))

	mon.stop(execCtx, pricingData)

	log.Println("Successfully completed!")
	// TODO: revert and fix this
	// <-execCtx.Done()
	return execCtx, nil
}

func init() {
	// Register apiextensionsv1 types (CustomResourceDefinition) with the
	// global client-go scheme so that the e2e-framework client can create
	// CRD objects.

	_ = apiextensionsv1.AddToScheme(scheme.Scheme)
}

func getScaler(scalerName string) (ExecScaler, error) {
	switch scalerName {
	case benchutil.ScalerKarpenter:
		return &karpenterExec{}, nil
	case benchutil.ScalerClusterAutoscaler:
		return &caExec{}, nil
	default:
		return nil, fmt.Errorf("unknown scaler %q", scalerName)
	}
}

// KwokctlConfigTemplateParams stores all the parameters
// needed for the kwokctl configuration template.
type KwokctlConfigTemplateParams struct {
	HomeDir                 string
	ClusterName             string
	KubeSchedulerConfigPath string
	OutputPath              string
	ScenarioDirectory       string
	ImageTag                string
	PrometheusConfigPath    string
}

// setupClusterForScaling creates a fresh KWOK cluster configured for the
// given scaler.
func setupClusterForScaling(
	ctx context.Context,
	scaler ExecScaler,
	clusterName string,
	scenarioDir string,
	imageTag string,
) (context.Context, *envconf.Config, string, error) {
	log.Printf("Setting up KWOK cluster %q...\n", clusterName)

	kubeSchedulerConfigPath, err := writeEmbeddedKubeSchedulerConfig()
	if err != nil {
		return ctx, nil, "", fmt.Errorf("cannot write kube-scheduler config: %w", err)
	}
	// TODO: Can this cause problems?
	defer os.Remove(kubeSchedulerConfigPath)

	promConfigPath, err := writePrometheusConfig(prometheusPort)
	if err != nil {
		return ctx, nil, "", fmt.Errorf("cannot write prometheus config: %w", err)
	}

	kwokctlCfgFile := path.Join(scenarioDir, "kwok-config.yaml")
	templateParams := KwokctlConfigTemplateParams{
		HomeDir:                 os.Getenv("HOME"),
		ClusterName:             clusterName,
		KubeSchedulerConfigPath: kubeSchedulerConfigPath,
		OutputPath:              kwokctlCfgFile,
		ScenarioDirectory:       scenarioDir,
		ImageTag:                imageTag,
		PrometheusConfigPath:    promConfigPath,
	}

	err = generateKwokctlConfig(templateParams, scaler.GetScalerKWOKTemplatePath())
	if err != nil {
		return ctx, nil, "", fmt.Errorf("cannot create kwok config: %w", err)
	}
	log.Printf("Wrote kwok config template to %q\n", templateParams.OutputPath)

	testenv := env.New()
	createClusterFunc := envfuncs.CreateClusterWithConfig(kwok.NewProvider(), clusterName, kwokctlCfgFile)
	cfg := testenv.EnvConf()

	ctx, err = createClusterFunc(ctx, cfg)
	if err != nil {
		return ctx, nil, "", fmt.Errorf("failed to create cluster: %w", err)
	}

	return ctx, cfg, promConfigPath, nil
}

// cleanupCluster exports pod/container logs and (unless --skip-cleanup is set)
// it destroys the KWOK cluster.
func cleanupCluster(
	ctx context.Context,
	cfg *envconf.Config,
	kwokClusterName string,
	scenarioDir string,
	promConfigPath string,
	skipCleanup bool,
) {
	os.Remove(promConfigPath)

	logsDir := path.Join(scenarioDir, "logs")

	if err := os.MkdirAll(logsDir, 0750); err != nil {
		fmt.Printf("Warning: Failed to create logs directory %q: %v\n", logsDir, err)
	} else {
		exportLogsFunc := envfuncs.ExportClusterLogs(kwokClusterName, logsDir)
		if _, err := exportLogsFunc(ctx, cfg); err != nil {
			fmt.Printf("Warning: Failed to export logs: %v\n", err)
		} else {
			fmt.Printf("\nExported logs to %q\n", logsDir)
		}
	}

	if !skipCleanup {
		log.Println("Cleaning up...")
		destroyClusterFunc := envfuncs.DestroyCluster(kwokClusterName)
		if _, err := destroyClusterFunc(ctx, cfg); err != nil {
			fmt.Printf("Warning: Failed to destroy cluster: %v\n", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Object deployment
// ---------------------------------------------------------------------------

// deployObjects creates the non-pod Kubernetes objects that form the cluster
// state: priority classes, nodes and defaultNamespaces' service account
func deployObjects(ctx context.Context, cfg *envconf.Config, clusterSnapshot planner.ClusterSnapshot) (err error) {
	err = deployPriorityClasses(ctx, clusterSnapshot, cfg)
	if err != nil {
		return
	}
	err = deployNodes(ctx, clusterSnapshot, cfg)
	if err != nil {
		return
	}
	// Just to be safe, create default NS if it doesn't exist
	defaultNamespaces := []string{
		corev1.NamespaceDefault,
		"kube-system",
		"kube-public",
		corev1.NamespaceNodeLease,
	}
	for _, ns := range defaultNamespaces {
		err := createNamespaceAndDefaultSA(ctx, cfg, ns)
		if err != nil {
			return err
		}
	}
	return
}

func deployPriorityClasses(ctx context.Context, clusterSnapshot planner.ClusterSnapshot, cfg *envconf.Config) error {
	log.Println("Deploying priority classes...")
	for _, pClass := range clusterSnapshot.PriorityClasses {
		pClass.ResourceVersion = ""
		if err := cfg.Client().Resources().Create(ctx, &pClass); err != nil {
			return fmt.Errorf("failed to create priorityClass: %w", err)
		}
	}
	return nil
}

func deployNodes(ctx context.Context, clusterSnapshot planner.ClusterSnapshot, cfg *envconf.Config) error {
	log.Printf("Deploying nodes, count %d...\n", len(clusterSnapshot.Nodes))
	for _, nodeInfo := range clusterSnapshot.Nodes {
		node := nodeutil.AsNode(nodeInfo)
		node.Spec.ProviderID = "kwok://" + node.Name // required so KWOK recognises node
		node.ResourceVersion = ""
		if err := cfg.Client().Resources().Create(ctx, node); err != nil {
			return fmt.Errorf("failed to create node: %w", err)
		}
	}
	return nil
}

// deployPods deploys the specified pods into the cluster.
// It also creates the namespace used by the pod and the 'default' ServiceAccount
// for that namespace.
func deployPods(ctx context.Context, cfg *envconf.Config, pods []planner.PodInfo) error {
	for _, podInfo := range pods {
		if err := createNamespaceAndDefaultSA(ctx, cfg, podInfo.Namespace); err != nil {
			return err
		}
		if err := createPod(ctx, cfg, podInfo); err != nil {
			return err
		}
	}
	return nil
}

func createNamespaceAndDefaultSA(ctx context.Context, cfg *envconf.Config, name string) error {
	ns := &corev1.Namespace{}
	ns.Name = name
	err := cfg.Client().Resources().Create(ctx, ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace: %w", err)
	}
	return createDefaultServiceAccount(ctx, cfg, name)
}

// KWOK does not auto-create service accounts, so we must
// create a "default" SA in every namespace ourselves.
func createDefaultServiceAccount(ctx context.Context, cfg *envconf.Config, name string) error {
	sa := corev1.ServiceAccount{}
	sa.Name = "default"
	sa.Namespace = name
	if err := cfg.Client().Resources().Create(ctx, &sa); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create default serviceAccount in namespace %q: %w", name, err)
	}
	return nil
}

// createPod converts a PodInfo to a corev1.Pod, applies the fixups needed
// for KWOK (dummy image, cleared identity fields) and creates it.
func createPod(ctx context.Context, cfg *envconf.Config, podInfo planner.PodInfo) error {
	p := podutil.AsPod(podInfo)
	if p.Spec.Containers[0].Image == "" {
		p.Spec.Containers[0].Image = "dummy-image"
	}
	// This is done to prevent container names that can have more than 63 chars
	p.Spec.Containers[0].Name = "dummy-container"
	p.ResourceVersion = ""
	p.UID = ""
	if err := cfg.Client().Resources().Create(ctx, p); err != nil {
		return fmt.Errorf("failed to create pod %q: %w", p.Name, err)
	}
	return nil
}

func partitionPods(pods []planner.PodInfo) (scheduled, unscheduled []planner.PodInfo) {
	for _, podInfo := range pods {
		if podInfo.NodeName == "" {
			unscheduled = append(unscheduled, podInfo)
		} else {
			scheduled = append(scheduled, podInfo)
		}
	}
	return
}

// ---------------------------------------------------------------------------
// Template & config helpers
// ---------------------------------------------------------------------------

func generateKwokctlConfig(params KwokctlConfigTemplateParams, templateConfigPath string) error {
	data, err := content.ReadFile(templateConfigPath)
	if err != nil {
		return fmt.Errorf("cannot read %s from content FS: %w", templateConfigPath, err)
	}
	templateConfig, err := template.New(templateConfigPath).Parse(string(data))
	if err != nil {
		return fmt.Errorf("cannot parse %s template: %w", templateConfigPath, err)
	}

	var buf bytes.Buffer
	if err := templateConfig.Execute(&buf, params); err != nil {
		return fmt.Errorf("cannot render %q template with params %q: %w", templateConfig.Name(), params, err)
	}
	if err := os.WriteFile(params.OutputPath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("cannot write kwok config to %q: %w", params.OutputPath, err)
	}
	return nil
}

func writeEmbeddedKubeSchedulerConfig() (string, error) {
	kubeSchedulerConfigData, err := content.ReadFile("templates/kube-scheduler-config.yaml")
	if err != nil {
		return "", fmt.Errorf("cannot read kube-scheduler-config.yaml from embedded FS: %w", err)
	}

	tempFile, err := os.CreateTemp("", "kube-scheduler-config.yaml")
	if err != nil {
		return "", fmt.Errorf("cannot create temporary file: %w", err)
	}
	defer tempFile.Close()

	if _, err := tempFile.Write(kubeSchedulerConfigData); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", fmt.Errorf("cannot write to temporary file: %w", err)
	}

	return tempFile.Name(), nil
}

// ---------------------------------------------------------------------------
// Monitoring and metrics collection
// ---------------------------------------------------------------------------

// monitorState groups the resources needed for metrics collection and
// event watching during a benchmark run.
type monitorState struct {
	monitors      []*DockerMonitor
	ec            *EventCollector
	metrics       map[string][]ContainerStats
	wg            *sync.WaitGroup
	cancelStream  context.CancelFunc
	server        *http.Server
	cfg           *envconf.Config
	meta          *RunMetadata
	clusterName   string
	scenarioDir   string
	waitForCancel bool
}

// newMonitor creates a monitorState by discovering Docker containers and
// preparing the metrics infrastructure. Call start() to begin streaming.
func newMonitor(ctx context.Context, cfg *envconf.Config, meta *RunMetadata, clusterName, scenarioDir string) (*monitorState, error) {
	containers := []string{meta.ScalerName, "kube-apiserver", "etcd", "kube-scheduler", "kwok-controller"}

	var monitors []*DockerMonitor
	for _, name := range containers {
		m := NewDockerMonitor(clusterName + "-" + name)
		if err := m.WaitForReady(ctx); err != nil {
			if name == meta.ScalerName {
				return nil, fmt.Errorf("scaler container %q did not become ready: %w", name, err)
			}
			log.Printf("Warning: component %q not found, skipping metrics collection", name)
			continue
		}
		monitors = append(monitors, m)
	}

	return &monitorState{
		monitors:    monitors,
		metrics:     make(map[string][]ContainerStats),
		cfg:         cfg,
		meta:        meta,
		clusterName: clusterName,
		scenarioDir: scenarioDir,
	}, nil
}

// start begins Docker stats streaming, serves Prometheus metrics, and
// starts the EventCollector. It should be called after newMonitor.
func (mon *monitorState) start(ctx context.Context, eventConfig ScalerEventConfig) error {
	mon.server = ServeMetrics(prometheusPort)

	streamCtx, cancelStream := context.WithCancel(ctx)
	mon.cancelStream = cancelStream

	metricsChan := make(chan PodMetrics, 100)

	var wg sync.WaitGroup
	wg.Go(func() {
		for m := range metricsChan {
			ts := m.Timestamp.Time
			for _, container := range m.Containers {
				stats := container.Stats
				stats.Timestamp = ts
				mon.metrics[container.Name] = append(mon.metrics[container.Name], stats)
				SetContainerMetrics(container.Name, stats)
			}
		}
	})
	mon.wg = &wg

	var producerWg sync.WaitGroup
	producerWg.Add(len(mon.monitors))
	for _, m := range mon.monitors {
		go func() {
			defer producerWg.Done()
			if err := m.StreamMetrics(streamCtx, metricsChan); err != nil {
				log.Printf("Error collecting metrics for %s: %v", m.containerNamePrefix, err)
			}
		}()
	}
	go func() {
		producerWg.Wait()
		close(metricsChan)
	}()

	log.Println("Starting event collection and measuring scaling timeline...")
	mon.ec = NewEventCollector(mon.cfg.Client().Resources(), mon.meta.Summary.ClusterState.Before.UnscheduledPods, eventConfig)
	if err := mon.ec.Start(ctx); err != nil {
		return fmt.Errorf("failed to start event collector: %w", err)
	}
	return nil
}

// stop waits for scaling to complete, then writes the final report
// and shuts down the metrics server.
func (mon *monitorState) stop(ctx context.Context, pricingData pricingapi.InstancePricingAccess) {
	err := mon.ec.Wait(ctx)
	if err != nil {
		log.Printf("Event collector wait interrupted: %v", err)
	}

	events, timing, eventsSummary := mon.ec.Results(pricingData)
	mon.ec.Stop()      // stop event watches
	mon.cancelStream() // stop all metric streams
	mon.meta.EndTime = time.Now()
	log.Printf("Scaling complete. Total time: %s, Reaction time: %s, Scheduling time: %s\n",
		valueOrNA(timing.TotalDuration), valueOrNA(timing.ReactionTime), valueOrNA(timing.SchedulingTime))

	if mon.waitForCancel {
		for _, m := range mon.monitors {
			ResetContainerMetrics(m.containerNamePrefix)
		}
		log.Println("Waiting for Ctrl+C...")
		<-ctx.Done()
	}

	mon.clusterStateAfter(context.Background())

	mon.wg.Wait()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mon.server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Metrics server shutdown error: %v", err)
	}

	eventsSummary.ScalingTimeline = timing
	eventsSummary.ClusterState = mon.meta.Summary.ClusterState
	mon.meta.Summary = eventsSummary
	mon.meta.TotalRunDuration = mon.meta.EndTime.Sub(mon.meta.StartTime).String()
	log.Printf("Total benchmarking run time: %s\n", mon.meta.TotalRunDuration)

	logsDir := path.Join(mon.scenarioDir, "logs", "kwok-"+mon.clusterName)
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		log.Printf("Failed to create logs directory: %v\n", err)
	} else {
		writeReports(logsDir, *mon.meta, events)
		writeMetricsCSV(path.Join(logsDir, "metrics"), mon.metrics)
	}
}

func valueOrNA(s string) string {
	if s == "" {
		return "N/A"
	}
	return s
}

func (mon *monitorState) clusterStateAfter(ctx context.Context) {
	nodes := &corev1.NodeList{}
	if err := mon.cfg.Client().Resources().List(ctx, nodes); err == nil {
		mon.meta.Summary.ClusterState.After.NodeCount = len(nodes.Items)
	}

	pods := &corev1.PodList{}
	if err := mon.cfg.Client().Resources().List(ctx, pods); err == nil {
		for _, pod := range pods.Items {
			if pod.Spec.NodeName == "" {
				mon.meta.Summary.ClusterState.After.UnscheduledPods++
			} else {
				mon.meta.Summary.ClusterState.After.ScheduledPods++
			}
		}
	}
}
