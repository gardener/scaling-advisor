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
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
	"text/template"
	"time"

	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"

	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion/scheme"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kwok"
)

//go:embed templates/*.yaml
var content embed.FS

var PrometheusPort = 2112

// Flag variables — bound by cobra, read once in execCmd.RunE, then passed
// explicitly to all callees so that no other function touches these globals.
var (
	skipCleanup       bool
	snapshotFile      string
	scalerVersion     string
	scalerPodName     string
	prometheusVersion string
)

// execScaler is the interface that every scaler backend must implement to
// participate in a benchmark run.
type ExecScaler interface {
	// DeployScalerData creates the scaler-specific Kubernetes objects (CRDs,
	// ConfigMaps, NodePools, etc.) in the KWOK cluster.
	DeployScalerData(ctx context.Context, cfg *envconf.Config, scenarioDir string) error

	// GetScalerContainerName returns the name of the container in which the scaler is running.
	// This is used for monitoring the resource usage of the scaler container during the benchmark run.
	GetScalerContainerName() string

	// GetScalerKWOKTemplatePath returns the embedded-FS path to the
	// kwokctl configuration template for this scaler.
	GetScalerKWOKTemplatePath() string

	// CheckRequiredDataPresent verifies that everything produced by
	// "setup" (files + Docker images) is available before the cluster
	// is created.
	CheckRequiredDataPresent(scenarioDir, version string) error
}

// ExecArgs has the flag variables — bound by cobra, read once in execCmd.RunE, then
// passed explicitly to all callees so that no other function touches these globals.
type ExecArgs struct {
	Scaler        string
	SnapshotFile  string
	ScalerVersion string
	SkipCleanup   bool
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
			<-execCtx.Done()
			return
		},
	}

	// Initialise the exec args with the passed flag values,
	// falling back to default if nothing specified
	// TODO: need to add the argument to pass the pricing data for report
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

	execCmd.PersistentFlags().StringVarP(
		&execArgs.ScalerVersion,
		"scaler-version", "v", "main",
		"version of the scaler to fetch",
	)

	execCmd.PersistentFlags().StringVar(
		&scalerPodName,
		"scaler-pod",
		"cluster-autoscaler",
		"name of the scaler pod to monitor",
	)

	execCmd.PersistentFlags().StringVar(
		&prometheusVersion,
		"prometheus-version",
		"latest",
		"prometheus image tag to use",
	)

	return execCmd
}

// Run sets up the cluster for executing the benchmarking operation by creating a docker
// runtime cluster using 'kwokctl' (this includes control plane components and the scaler).
// Then it deploys all the objects present in the snapshot in the cluster alongwith the
// artefacts produced by the "setup" subcommand.
func Run(execCtx context.Context, args ExecArgs) (ctx context.Context, err error) {
	scenarioDir := path.Dir(args.SnapshotFile)

	scaler, err := getScaler(args.Scaler)
	if err != nil {
		return
	}

	err = scaler.CheckRequiredDataPresent(scenarioDir, args.ScalerVersion)
	if err != nil {
		return nil, fmt.Errorf("please run 'setup' before running 'exec': %v", err)
	}

	kwokClusterName := envconf.RandomName("kwok-cluster", 17)

	ctx, cfg, promConfigPath, err := setupClusterForScaling(ctx, scaler, kwokClusterName, scenarioDir, args.scalerVersion)
	if err != nil {
		return err
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
	if err := deployPods(ctx, cfg, scheduled); err != nil {
		return fmt.Errorf("error deploying scheduled pods: %v", err)
	}
	log.Printf("Deployed all %d scheduled pods", len(scheduled))

	meta := RunMetadata{
		StartTime:     time.Now(),
		ScalerName:    scalerName,
		ScalerVersion: scalerVersion,
		SnapshotFile:  snapshotFile,
		ClusterState: ClusterState{
			Before: ClusterPodStats{
				NodeCount:       len(clusterSnapshot.Nodes),
				ScheduledPods:   len(scheduled),
				UnscheduledPods: len(unscheduled),
			},
		},
	}

	ec, collectedMetrics, wg, err := startMonitor(ctx, cfg, meta)
	if err != nil {
		return fmt.Errorf("monitoring setup failed: %v", err)
	}
	defer ec.Stop()

	log.Printf("Deploying unscheduled pods, count %d...", len(unscheduled))
	if err := deployPods(ctx, cfg, unscheduled); err != nil {
		return fmt.Errorf("error deploying unscheduled pods: %v", err)
	}
	log.Printf("Deployed all %d unscheduled pods", len(unscheduled))

	stopMonitor(ctx, cfg, ec, collectedMetrics, wg, &meta, kwokClusterName, scenarioDir)

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
	ContainerName           string
	PrometheusConfigPath    string
	PrometheusImage         string
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

	promConfigPath, err := writePrometheusConfig(PrometheusPort)
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
		ContainerName:           scaler.GetScalerContainerName(),
		PrometheusConfigPath:    promConfigPath,
		PrometheusImage:         "prom/prometheus:" + prometheusVersion,
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
	// FIXME:
	// return createDefaultServiceAccount(ctx, cfg, corev1.NamespaceDefault)
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
	} else if errors.IsAlreadyExists(err) {
		// Do not attempt to create another service account below
		return nil
	}
	err = createDefaultServiceAccount(ctx, cfg, name)
	if err != nil {
		return err
	}
	return nil
}

// KWOK does not auto-create service accounts, so we must
// create a "default" SA in every namespace ourselves.
func createDefaultServiceAccount(ctx context.Context, cfg *envconf.Config, name string) error {
	sa := corev1.ServiceAccount{}
	sa.Name = "default"
	sa.Namespace = name
	if err := cfg.Client().Resources().Create(ctx, &sa); err != nil {
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

func setupSignalHandler() context.Context {
	quit := make(chan os.Signal, 2)
	ctx, cancel := context.WithCancel(context.Background())
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		cancel()
		<-quit
		os.Exit(1)
	}()
	return ctx
}

// startMonitor sets up Docker stats streaming, Prometheus metrics server,
// and the EventCollector.
func startMonitor(ctx context.Context, cfg *envconf.Config, meta RunMetadata) (*EventCollector, *[]PodMetrics, *sync.WaitGroup, error) {
	log.Printf("Starting monitoring for scaler docker container %s...\n", scalerPodName)

	mon := NewDockerMonitor(scalerPodName)

	log.Println("Waiting for scaler container to be ready...")
	if err := mon.WaitForReady(ctx); err != nil {
		return nil, nil, nil, fmt.Errorf("scaler container %q did not become ready: %w", scalerPodName, err)
	}
	log.Println("Scaler container is ready")

	go func() {
		if err := ServeMetrics(PrometheusPort); err != nil {
			log.Printf("Failed to serve metrics: %v", err)
		}
	}()

	metricsChan := make(chan PodMetrics, 100)
	var wg sync.WaitGroup
	var collectedMetrics []PodMetrics
	wg.Add(2)

	// consumer: collect metrics and update Prometheus gauges
	go func() {
		defer wg.Done()
		for m := range metricsChan {
			collectedMetrics = append(collectedMetrics, m)
			for _, container := range m.Containers {
				s := container.Stats
				ScalerCPUUsage.WithLabelValues(container.Name).Set(float64(s.CPUMillicores))
				ScalerMemoryUsage.WithLabelValues(container.Name).Set(float64(s.MemoryMi))
				ScalerMemoryRSS.WithLabelValues(container.Name).Set(float64(s.MemoryRSSMi))
				ScalerMemoryMaxUsage.WithLabelValues(container.Name).Set(float64(s.MemoryMaxUsageMi))
				ScalerMemoryLimit.WithLabelValues(container.Name).Set(float64(s.MemoryLimitMi))
				ScalerCPUThrottledPeriods.WithLabelValues(container.Name).Set(float64(s.CPUThrottledPeriods))
				ScalerCPUTotalPeriods.WithLabelValues(container.Name).Set(float64(s.CPUTotalPeriods))
				ScalerCPUThrottledTime.WithLabelValues(container.Name).Set(float64(s.CPUThrottledTimeNs))
				ScalerPIDs.WithLabelValues(container.Name).Set(float64(s.PIDs))
			}
		}
	}()

	// producer: stream metrics from Docker until ctx is cancelled (Ctrl+C)
	go func() {
		defer wg.Done()
		defer close(metricsChan)
		if err := mon.StreamMetrics(ctx, metricsChan); err != nil {
			log.Printf("Error collecting metrics: %v", err)
		}
	}()

	log.Println("Starting event collection and measuring scaling timeline...")
	ec := NewEventCollector(cfg.Client().Resources(), meta.ClusterState.Before.UnscheduledPods)
	if err := ec.Start(ctx); err != nil {
		log.Printf("Failed to start event collector: %v", err)
	}

	return ec, &collectedMetrics, &wg, nil
}

// finishMonitoring waits for scaling to complete, blocks until cancelled by user
// then writes the final report.
func stopMonitor(ctx context.Context, cfg *envconf.Config, ec *EventCollector, collectedMetrics *[]PodMetrics, wg *sync.WaitGroup, meta *RunMetadata, clusterName, scenarioDir string) {
	if err := ec.Wait(ctx); err != nil {
		log.Printf("Event collector wait interrupted: %v", err)
	}

	events, timing := ec.Results()
	log.Printf("Scaling complete. Total time: %s, Reaction time: %s, Scheduling time: %s\n",
		timing.TotalDuration, timing.ReactionTime, timing.SchedulingTime)

	<-ctx.Done()

	after := clusterStateAfter(context.Background(), cfg)

	wg.Wait()

	meta.ClusterState.After = after
	meta.ScalingTime = timing

	reportPath := path.Join(scenarioDir, "logs", "kwok-"+clusterName, "scaler-report.json")
	if err := os.MkdirAll(path.Dir(reportPath), 0755); err != nil {
		log.Printf("Failed to create logs directory: %v\n", err)
	} else {
		report := RunReport{
			Metadata: *meta,
			Metrics:  *collectedMetrics,
			Events:   events,
		}
		if err := writeReport(reportPath, report); err != nil {
			log.Printf("Failed to write report: %v\n", err)
		} else {
			log.Printf("Wrote report to %s\n", reportPath)
		}
	}
}

func clusterStateAfter(ctx context.Context, cfg *envconf.Config) ClusterPodStats {
	var state ClusterPodStats

	nodes := &corev1.NodeList{}
	if err := cfg.Client().Resources().List(ctx, nodes); err == nil {
		state.NodeCount = len(nodes.Items)
	}

	pods := &corev1.PodList{}
	if err := cfg.Client().Resources().List(ctx, pods); err == nil {
		for _, pod := range pods.Items {
			if pod.Spec.NodeName == "" {
				state.UnscheduledPods++
			} else {
				state.ScheduledPods++
			}
		}
	}

	return state
}
