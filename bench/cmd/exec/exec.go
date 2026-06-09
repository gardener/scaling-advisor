// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"bytes"
	"cmp"
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"sync"
	"text/template"
	"time"

	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"

	"github.com/gardener/scaling-advisor/api/planner"
	pricingapi "github.com/gardener/scaling-advisor/api/pricing"
	"github.com/gardener/scaling-advisor/pricing"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kwok"
)

//go:embed templates/*.yaml
var content embed.FS

var temporaryFiles []string

const prometheusPort = 2112

// NewExecCommand runs a scaler inside a KWOK cluster populated with data
// from the cluster snapshot. It is the counterpart to "setup", which prepares
// the resources and deploys the scaler image that this command consumes.
func NewExecCommand(_ context.Context) *cobra.Command {
	var execArgs ExecArgs
	var execCmd = &cobra.Command{
		Use:   "exec <scaler> <options>",
		Short: "Run the scaler by utilizing the data and produce the report",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, cmdArgs []string) (err error) {
			cmd.SilenceUsage = true
			// Only the scaler is passed as an argument to the command, rest are all flags
			execArgs.Scaler = cmdArgs[0]

			clusterName := envconf.RandomName("cluster", 17)
			execCtx := benchutil.SetupSignalHandler()
			_, err = Run(execCtx, execArgs, clusterName)
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

	execCmd.PersistentFlags().StringVarP(
		&execArgs.ConfigFile,
		"config", "c", "",
		"kwokctl configuration file, fall back to embedded config (optional)",
	)
	_ = execCmd.MarkFlagFilename("config", "yaml")

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
func Run(execCtx context.Context, args ExecArgs, clusterName string) (Summary, error) {
	summary := Summary{}
	// Set the log entry prefix time to UTC so that it matches the container
	// log times and the metric timestamps
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC | log.Lshortfile)

	scenarioDir, err := filepath.Abs(path.Dir(args.SnapshotFile))
	if err != nil {
		return summary, fmt.Errorf("cannot resolve scenario directory: %w", err)
	}

	scaler, err := getScaler(args.Scaler)
	if err != nil {
		return summary, fmt.Errorf("cannot get scaler: %w", err)
	}

	err = benchutil.CheckIfDockerRunning()
	if err != nil {
		return summary, fmt.Errorf("docker is not running: %v", err)
	}

	pricingData, err := pricing.GetInstancePricingAccess("dummy-provider", args.PricingFile)
	if err != nil {
		return summary, fmt.Errorf("error parsing pricing data: %v", err)
	}

	err = scaler.CheckRequiredDataPresent(scenarioDir, args.ScalerVersion)
	if err != nil {
		return summary, fmt.Errorf("please run 'setup' before running 'exec': %v", err)
	}

	execCtx, cfg, err := setupCluster(execCtx, scaler, clusterName, scenarioDir, args.ConfigFile, args.ScalerVersion)
	if err != nil {
		return summary, err
	}
	defer cleanupCluster(execCtx, cfg, clusterName, scenarioDir, args.SkipCleanup)

	clusterSnapshot, err := benchutil.LoadJSONFromFile[planner.ClusterSnapshot](args.SnapshotFile)
	if err != nil {
		return summary, fmt.Errorf("cannot load cluster snapshot: %v", err)
	}

	if err := deployObjects(execCtx, cfg, clusterSnapshot); err != nil {
		return summary, fmt.Errorf("error running KWOK cluster: %v", err)
	}
	if err := scaler.DeployScalerData(execCtx, cfg, scenarioDir); err != nil {
		return summary, fmt.Errorf("error deploying the scaler data: %v", err)
	}
	if err := scaler.DeployNodes(execCtx, cfg, &clusterSnapshot); err != nil {
		return summary, fmt.Errorf("error deploying nodes: %v", err)
	}

	scheduled, unscheduled, daemonSetPodCount := partitionPods(clusterSnapshot.Pods)

	log.Printf("Deploying scheduled pods, count %d...", len(scheduled))
	if err := deployPods(execCtx, cfg, scheduled); err != nil {
		return summary, fmt.Errorf("error deploying scheduled pods: %v", err)
	}
	log.Printf("Deployed all %d scheduled pods", len(scheduled))

	meta := RunMetadata{
		StartTime:     time.Now().UTC(),
		ScalerName:    args.Scaler,
		ScalerVersion: args.ScalerVersion,
		SnapshotFile:  args.SnapshotFile,
	}
	meta.Summary.ClusterState.Before = ClusterStats{
		NodeCount:     len(clusterSnapshot.Nodes),
		ScheduledPods: len(scheduled),
		// FIXME: need to do this for all events as well
		// CA doesn't emit events for DS pods not triggering scale up
		UnscheduledNonDaemonSetPods: len(unscheduled) - daemonSetPodCount,
	}

	mon, err := newMonitor(execCtx, cfg, &meta, clusterName, scenarioDir)
	if err != nil {
		return summary, fmt.Errorf("monitoring setup failed: %v", err)
	}
	mon.waitForCancel = args.WaitForCancel

	if err := mon.start(execCtx, scaler.EventConfig()); err != nil {
		return summary, fmt.Errorf("monitoring start failed: %v", err)
	}
	defer mon.ec.Stop()

	log.Printf("Deploying unscheduled pods, count %d...", len(unscheduled))
	if err := deployPods(execCtx, cfg, unscheduled); err != nil {
		return summary, fmt.Errorf("error deploying unscheduled pods: %v", err)
	}
	for _, p := range unscheduled {
		fmt.Printf("Deployed %q\n", p.Name)
	}
	log.Printf("Deployed all %d unscheduled pods", len(unscheduled))

	mon.stop(execCtx, pricingData)

	log.Println("Successfully completed!")

	return mon.meta.Summary, nil
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

// setupClusterForScaling creates a fresh KWOK cluster configured for the
// given scaler.
func setupCluster(
	ctx context.Context,
	scaler ExecScaler,
	clusterName string,
	scenarioDir string,
	configFile string,
	imageTag string,
) (context.Context, *envconf.Config, error) {
	log.Printf("Setting up KWOK cluster %q...\n", clusterName)

	kubeSchedulerConfigPath, err := writeEmbeddedKubeSchedulerConfig()
	if err != nil {
		return ctx, nil, fmt.Errorf("cannot write kube-scheduler config: %w", err)
	}
	temporaryFiles = append(temporaryFiles, kubeSchedulerConfigPath)

	promConfigPath, err := writePrometheusConfig(prometheusPort)
	if err != nil {
		return ctx, nil, fmt.Errorf("cannot write prometheus config: %w", err)
	}
	temporaryFiles = append(temporaryFiles, promConfigPath)

	outputKwokCfgFile := path.Join(scenarioDir, "kwok-config.yaml")
	templateParams := KwokctlConfigTemplateParams{
		HomeDir:                 os.Getenv("HOME"),
		ClusterName:             clusterName,
		KubeSchedulerConfigPath: kubeSchedulerConfigPath,
		OutputPath:              outputKwokCfgFile,
		ScenarioDirectory:       scenarioDir,
		ImageTag:                imageTag,
		PrometheusConfigPath:    promConfigPath,
	}

	err = generateKwokctlConfig(templateParams, scaler, configFile)
	if err != nil {
		return ctx, nil, fmt.Errorf("cannot create kwok config: %w", err)
	}
	log.Printf("Wrote kwok config template to %q\n", templateParams.OutputPath)

	testenv := env.New()
	createClusterFunc := envfuncs.CreateClusterWithConfig(kwok.NewProvider(), clusterName, outputKwokCfgFile)
	cfg := testenv.EnvConf()

	ctx, err = createClusterFunc(ctx, cfg)
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to create cluster: %w", err)
	}

	restCfg := cfg.Client().RESTConfig()
	// Create a client with API server warnings suppressed (e.g. pod names
	// exceeding the 63-char DNS label limit, this introduces a lot of noise).
	restCfg.WarningHandler = rest.NoWarnings{}
	// Disable rate limiting to speed up the deployment
	restCfg.QPS = -1
	client, err := klient.New(restCfg)
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to create client: %w", err)
	}
	cfg.WithClient(client)

	return ctx, cfg, nil
}

// cleanupCluster exports pod/container logs and (unless --skip-cleanup is set)
// it destroys the KWOK cluster.
func cleanupCluster(
	ctx context.Context,
	cfg *envconf.Config,
	clusterName string,
	scenarioDir string,
	skipCleanup bool,
) {
	for _, file := range temporaryFiles {
		os.Remove(file)
	}

	logsDir := path.Join(scenarioDir, "logs")

	if err := os.MkdirAll(logsDir, 0750); err != nil {
		fmt.Printf("Warning: Failed to create logs directory %q: %v\n", logsDir, err)
	} else {
		exportLogsFunc := envfuncs.ExportClusterLogs(clusterName, logsDir)
		if _, err := exportLogsFunc(ctx, cfg); err != nil {
			fmt.Printf("Warning: Failed to export logs: %v\n", err)
		} else {
			fmt.Printf("\nExported logs to %q\n", path.Join(logsDir, clusterName))
		}
	}

	if !skipCleanup {
		log.Println("Cleaning up...")
		destroyClusterFunc := envfuncs.DestroyCluster(clusterName)
		if _, err := destroyClusterFunc(ctx, cfg); err != nil {
			fmt.Printf("Warning: Failed to destroy cluster: %v\n", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Template & config helpers
// ---------------------------------------------------------------------------

func generateKwokctlConfig(params KwokctlConfigTemplateParams, scaler ExecScaler, configPath string) error {
	var (
		data               []byte
		templateConfigPath string
		err                error
	)
	if configPath != "" {
		templateConfigPath = configPath
		data, err = os.ReadFile(templateConfigPath)
		if err != nil {
			return fmt.Errorf("cannot read %s from filesystem: %w", templateConfigPath, err)
		}
	} else {
		templateConfigPath = scaler.GetScalerKWOKTemplatePath()
		data, err = content.ReadFile(templateConfigPath)
		if err != nil {
			return fmt.Errorf("cannot read %s from content FS: %w", templateConfigPath, err)
		}
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

// newMonitor creates a monitorState by discovering Docker containers and
// preparing the metrics infrastructure. Call start() to begin streaming.
func newMonitor(ctx context.Context, cfg *envconf.Config, meta *RunMetadata, clusterName, scenarioDir string) (*monitorState, error) {
	containers := []string{
		meta.ScalerName, "kube-apiserver", "etcd", "kube-scheduler", "kwok-controller",
	}

	var monitors []DockerMonitor
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
			ts := m.Timestamp.UTC()
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
	mon.ec = NewEventCollector(mon.cfg.Client().Resources(), mon.meta.Summary.ClusterState.Before.UnscheduledNonDaemonSetPods, eventConfig)
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

	if mon.waitForCancel {
		for _, m := range mon.monitors {
			ResetContainerMetrics(m.containerNamePrefix)
		}
		log.Println("Waiting for Ctrl+C...")
		<-ctx.Done()
	}

	events, timing, eventsSummary := mon.ec.Results(pricingData)
	mon.ec.Stop()      // stop event watches
	mon.cancelStream() // stop all metric streams
	mon.meta.EndTime = time.Now().UTC()
	log.Printf("Scaling complete. Total time: %s, Reaction time: %s, Scheduling time: %s\n",
		cmp.Or(timing.TotalDuration, "N/A"), cmp.Or(timing.ReactionTime, "N/A"), cmp.Or(timing.SchedulingTime, "N/A"))

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

func (mon *monitorState) clusterStateAfter(ctx context.Context) {
	nodes := &corev1.NodeList{}
	if err := mon.cfg.Client().Resources().List(ctx, nodes); err == nil {
		mon.meta.Summary.ClusterState.After.NodeCount = len(nodes.Items)
	}

	pods := &corev1.PodList{}
	if err := mon.cfg.Client().Resources().List(ctx, pods); err == nil {
		for _, pod := range pods.Items {
			if pod.Spec.NodeName == "" {
				if owners := pod.GetOwnerReferences(); len(owners) > 0 && owners[0].Kind != "DaemonSet" {
					mon.meta.Summary.ClusterState.After.UnscheduledNonDaemonSetPods++
				}
			} else {
				mon.meta.Summary.ClusterState.After.ScheduledPods++
			}
		}
	}
}
