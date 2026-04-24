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

	bench "github.com/gardener/scaling-advisor/bench/cmd"

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

var PrometheusPort = 2112

// Flag variables — bound by cobra, read once in execCmd.RunE, then passed
// explicitly to all callees so that no other function touches these globals.
var (
	skipCleanup       bool
	snapshotFile      string
	scalerVersion     string
	monitorInterval   time.Duration
	scalerPodName     string
	prometheusVersion string
)

// execScaler is the interface that every scaler backend must implement to
// participate in a benchmark run.
type execScaler interface {
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

// execCmd runs a scaler inside a KWOK cluster populated with data from the
// cluster snapshot. It is the counterpart to "setup", which prepares the
// resources and deploys the scaler image that this command consumes.
var execCmd = &cobra.Command{
	Use:   "exec <scaler> <options>",
	Short: "Run the scaler by utilizing the data and produce the report",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) (err error) {
		scalerName := args[0]
		var scaler execScaler
		scaler, err = getScaler(scalerName)
		if err != nil {
			return
		}

		scenarioDir := path.Dir(snapshotFile)

		err = scaler.CheckRequiredDataPresent(scenarioDir, scalerVersion)
		if err != nil {
			return fmt.Errorf("please run 'setup' before running 'exec': %v", err)
		}

		ctx := setupSignalHandler()
		kwokClusterName := envconf.RandomName("kwok-cluster", 17)

		ctx, cfg, promConfigPath, err := setupClusterForScaling(ctx, scaler, kwokClusterName, scenarioDir, scalerVersion)
		if err != nil {
			return err
		}
		defer os.Remove(promConfigPath)
		defer cleanupCluster(ctx, cfg, kwokClusterName, scenarioDir)

		clusterSnapshot, err := bench.LoadJSONFromFile[planner.ClusterSnapshot](snapshotFile)
		if err != nil {
			return fmt.Errorf("cannot load cluster snapshot: %v", err)
		}

		if err := deployObjects(ctx, cfg, clusterSnapshot); err != nil {
			return fmt.Errorf("error running KWOK cluster: %v", err)
		}
		if err := scaler.DeployScalerData(ctx, cfg, scenarioDir); err != nil {
			return fmt.Errorf("error deploying the scaler data: %v", err)
		}
		if err := deployPods(ctx, clusterSnapshot, cfg); err != nil {
			return fmt.Errorf("error deploying pods: %v", err)
		}

		scheduled, unscheduled := partitionPods(clusterSnapshot.Pods)
		meta := RunMetadata{
			StartTime:     time.Now(),
			ScalerName:    scalerName,
			ScalerVersion: scalerVersion,
			SnapshotFile:  snapshotFile,
			Before: ClusterState{
				NodeCount:       len(clusterSnapshot.Nodes),
				ScheduledPods:   len(scheduled),
				UnscheduledPods: len(unscheduled),
			},
		}

		if err := monitorScaler(ctx, cfg, kwokClusterName, scenarioDir, meta); err != nil {
			log.Printf("Warning: Monitoring failed: %v", err)
		}

		log.Println("Successfully completed!")
		<-ctx.Done()

		return
	},
}

func init() {
	// Register apiextensionsv1 types (CustomResourceDefinition) with the
	// global client-go scheme so that the e2e-framework client can create
	// CRD objects.
	_ = apiextensionsv1.AddToScheme(scheme.Scheme)

	bench.RootCmd.AddCommand(execCmd)

	execCmd.PersistentFlags().StringVar(
		&snapshotFile,
		"snap",
		"",
		"cluster snapshot file",
	)
	_ = execCmd.MarkFlagRequired("snap")

	execCmd.PersistentFlags().BoolVarP(
		&skipCleanup,
		"skip-cleanup", "s",
		false,
		"delete cluster with all data upon finishing",
	)

	execCmd.PersistentFlags().StringVarP(
		&scalerVersion,
		"scaler-version", "v",
		"main",
		"version of the scaler to fetch",
	)

	execCmd.PersistentFlags().DurationVar(
		&monitorInterval,
		"monitor-interval",
		100*time.Millisecond,
		"interval for collecting metrics",
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
}

func getScaler(scalerName string) (execScaler, error) {
	switch scalerName {
	case bench.ScalerKarpenter:
		return &karpenterExec{}, nil
	case bench.ScalerClusterAutoscaler:
		return &caExec{}, nil
	default:
		return nil, fmt.Errorf("unknown scaler %q", scalerName)
	}
}

// KwokCfgTmplParams stores all the parameters needed for the
// kwokctl configuration template.
type KwokCfgTmplParams struct {
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
	scaler execScaler,
	clusterName string,
	scenarioDir string,
	imageTag string,
) (context.Context, *envconf.Config, string, error) {
	outputFile := path.Join(scenarioDir, "kwok-config.yaml")

	log.Printf("Setting up KWOK cluster %q...\n", clusterName)

	kubeSchedulerConfigPath, err := writeEmbeddedKubeSchedulerConfig()
	if err != nil {
		return ctx, nil, "", fmt.Errorf("cannot write kube-scheduler config: %w", err)
	}
	defer os.Remove(kubeSchedulerConfigPath)

	promConfigPath, err := writePrometheusConfig(PrometheusPort)
	if err != nil {
		return ctx, nil, "", fmt.Errorf("cannot write prometheus config: %w", err)
	}

	tmplParams := KwokCfgTmplParams{
		HomeDir:                 os.Getenv("HOME"),
		ClusterName:             clusterName,
		KubeSchedulerConfigPath: kubeSchedulerConfigPath,
		OutputPath:              outputFile,
		ScenarioDirectory:       scenarioDir,
		ImageTag:                imageTag,
		ContainerName:           scaler.GetScalerContainerName(),
		PrometheusConfigPath:    promConfigPath,
		PrometheusImage:         "prom/prometheus:" + prometheusVersion,
	}

	err = generateKwokConfig(tmplParams, scaler.GetScalerKWOKTemplatePath())
	if err != nil {
		return ctx, nil, "", fmt.Errorf("cannot create kwok config: %w", err)
	}
	log.Printf("Wrote kwok config template to %q\n", tmplParams.OutputPath)

	testenv := env.New()
	createClusterFunc := envfuncs.CreateClusterWithConfig(kwok.NewProvider(), clusterName, outputFile)
	cfg := testenv.EnvConf()

	ctx, err = createClusterFunc(ctx, cfg)
	if err != nil {
		return ctx, nil, "", fmt.Errorf("failed to create cluster: %w", err)
	}

	return ctx, cfg, promConfigPath, nil
}

// cleanupCluster exports pod/container logs and (unless --skip-cleanup is set)
// it destroys the KWOK cluster.
func cleanupCluster(ctx context.Context, cfg *envconf.Config, kwokClusterName, scenarioDir string) {
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
	defaultNamespaces := []string{
		corev1.NamespaceDefault,
		"kube-system",
		"kube-public",
		corev1.NamespaceNodeLease,
	}
	for _, ns := range defaultNamespaces {
		err = createDefaultServiceAccount(ctx, cfg, ns)
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

// deployPods partitions the snapshot pods into scheduled and unscheduled sets
// and creates them in that order. It also creates the namespace used by the
// pod and the 'default' ServiceAccount for that namespace. Scheduled pods are
// deployed first so that the scheduler does not attempt to place unscheduled
// pods on existing nodes.
func deployPods(ctx context.Context, clusterSnapshot planner.ClusterSnapshot, cfg *envconf.Config) error {
	scheduled, unscheduled := partitionPods(clusterSnapshot.Pods)

	log.Printf("Deploying scheduled pods, count %d...", len(scheduled))
	for _, podInfo := range scheduled {
		if err := createNamespaceAndDefaultSA(ctx, cfg, podInfo.Namespace); err != nil {
			return err
		}
		if err := createPod(ctx, cfg, podInfo); err != nil {
			return err
		}
	}

	log.Printf("Deploying unscheduled pods, count %d...", len(unscheduled))
	for _, podInfo := range unscheduled {
		log.Printf("Deploying unscheduled pod %q\n", podInfo.Name)
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
	// This is done to prevent containers name that can have more than 63 chars
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

func generateKwokConfig(params KwokCfgTmplParams, templateConfigPath string) error {
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

// monitorScaler starts a Prometheus metrics server, waits for the scaler
// container to be ready, then streams resource-usage metrics to a JSON file
// and to the Prometheus gauges. It also measures the time until all pods are
// scheduled (recommendation time).
func monitorScaler(ctx context.Context, cfg *envconf.Config, clusterName, scenarioDir string, meta RunMetadata) error {
	log.Printf("Starting monitoring for scaler docker container %s...\n", scalerPodName)

	mon := NewDockerMonitor(scalerPodName, monitorInterval)

	log.Println("Waiting for scaler container to be ready...")
	if err := mon.WaitForReady(ctx); err != nil {
		return fmt.Errorf("scaler container %q did not become ready: %w", scalerPodName, err)
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
				cpu := container.Usage[corev1.ResourceCPU]
				mem := container.Usage[corev1.ResourceMemory]
				ScalerCPUUsage.WithLabelValues(container.Name).Set(float64(cpu.MilliValue()))
				ScalerMemoryUsage.WithLabelValues(container.Name).Set(float64(mem.Value()) / (1024 * 1024))
			}
		}
	}()

	// producer: stream metrics from Docker
	go func() {
		defer wg.Done()
		defer close(metricsChan)
		if err := mon.StreamMetrics(ctx, metricsChan); err != nil {
			log.Printf("Error collecting metrics: %v", err)
		}
	}()

	log.Println("Measuring time to produce recommendation...")

	recommendationCondition := func() (bool, error) {
		pods := &corev1.PodList{}
		if err := cfg.Client().Resources().List(ctx, pods); err != nil {
			return false, err
		}
		for _, pod := range pods.Items {
			if pod.Spec.NodeName == "" {
				return false, nil
			}
		}
		return true, nil
	}

	duration, err := MeasureRecommendationTime(ctx, meta.StartTime, recommendationCondition)
	if err != nil {
		log.Printf("Failed to measure recommendation time: %v", err)
	} else {
		log.Printf("Recommendation produced in: %v\n", duration)
	}

	after := clusterStateAfter(ctx, cfg)

	wg.Wait()

	meta.After = after
	meta.RecommendationDuration = duration.String()

	reportPath := path.Join(scenarioDir, "logs", "kwok-"+clusterName, "scaler-report.json")
	if err := os.MkdirAll(path.Dir(reportPath), 0755); err != nil {
		log.Printf("Failed to create logs directory: %v\n", err)
	} else {
		report := RunReport{
			Metadata: meta,
			Metrics:  collectedMetrics,
		}
		if err := writeReport(reportPath, report); err != nil {
			log.Printf("Failed to write report: %v\n", err)
		} else {
			log.Printf("Wrote report to %s\n", reportPath)
		}
	}

	return nil
}

func clusterStateAfter(ctx context.Context, cfg *envconf.Config) ClusterState {
	var state ClusterState

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
