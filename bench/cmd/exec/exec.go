// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"text/template"
	"time"

	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"

	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/pricing"
	"github.com/spf13/cobra"
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

// ExecArgs has the flag variables and additional common variables required
// by various methods/functions during harness run.
type ExecArgs struct {
	Scaler        string
	SnapshotFile  string
	ScenarioDir   string
	ConfigFile    string
	ScalerVersion string
	WaitPeriod    string
	SkipCleanup   bool
}

// NewExecCommand runs a scaler inside a KWOK cluster populated with data
// from the cluster snapshot. It is the counterpart to "setup", which prepares
// the resources and deploys the scaler image that this command consumes.
func NewExecCommand(_ context.Context) *cobra.Command {
	var execArgs ExecArgs
	var execCmd = &cobra.Command{
		Use:   "exec <scaler> <options>",
		Short: "Run the scaler by utilizing the data and produce the report",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, cmdArgs []string) (err error) {
			cmd.SilenceUsage = true
			// Only the scaler is passed as an argument to the command, rest are all flags
			execArgs.Scaler = cmdArgs[0]

			scaler, err := getScaler(execArgs.Scaler)
			if err != nil {
				return fmt.Errorf("cannot get scaler: %w", err)
			}

			_, err = time.ParseDuration(execArgs.WaitPeriod)
			if err != nil {
				return fmt.Errorf("wait time %q cannot be parsed: %v", execArgs.WaitPeriod, err)
			}

			err = benchutil.CheckIfDockerRunning()
			if err != nil {
				return fmt.Errorf("docker is not running: %v", err)
			}

			execArgs.ScenarioDir, err = filepath.Abs(path.Dir(execArgs.SnapshotFile))
			if err != nil {
				return fmt.Errorf("cannot resolve scenario directory: %w", err)
			}

			// Check if required data is present
			genDir := path.Join(execArgs.ScenarioDir, "gen")
			_, err = os.Stat(genDir)
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("required directory %q not found", genDir)
			}
			_, err = os.Stat(path.Join(genDir, benchutil.FileNamePricingData))
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("pricing file not found in %q", genDir)
			}

			err = scaler.CheckRequiredDataPresent(genDir, execArgs.ScalerVersion)
			if err != nil {
				return fmt.Errorf("run 'setup' before running 'exec': %v", err)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			cmd.SilenceUsage = true

			clusterName := execArgs.Scaler + "-" + time.Now().UTC().Format("20060102T150405Z")
			execCtx := benchutil.SetupSignalHandler()

			execCtx, cfg, err := setupCluster(execCtx, execArgs, clusterName)
			if err != nil {
				return err
			}
			defer cleanupCluster(execCtx, cfg, execArgs, clusterName)

			scaler, err := getScaler(execArgs.Scaler)
			if err != nil {
				return fmt.Errorf("cannot get scaler: %w", err)
			}
			_, err = Run(execCtx, cfg, scaler, execArgs, clusterName)
			return
		},
	}

	initArgs(execCmd, &execArgs)

	return execCmd
}

func initArgs(execCmd *cobra.Command, execArgs *ExecArgs) {
	// Initialise the exec args with the passed flag values,
	// falling back to default if nothing specified
	execCmd.Flags().StringVar(
		&execArgs.SnapshotFile,
		"snap", "",
		"cluster snapshot file (required)",
	)
	_ = execCmd.MarkFlagRequired("snap")
	_ = execCmd.MarkFlagFilename("snap", "json")

	execCmd.Flags().StringVarP(
		&execArgs.ConfigFile,
		"config", "c", "",
		"kwokctl configuration file, fall back to embedded config (optional)",
	)
	_ = execCmd.MarkFlagFilename("config", "yaml")

	execCmd.Flags().BoolVarP(
		&execArgs.SkipCleanup,
		"skip-cleanup", "s", false,
		"skip deleting cluster with all data upon finishing",
	)

	execCmd.Flags().StringVarP(
		&execArgs.WaitPeriod,
		"wait", "w", "1m",
		"wait for this long for any scaling activity before finishing",
	)

	execCmd.Flags().StringVarP(
		&execArgs.ScalerVersion,
		"scaler-version", "v", "main",
		"version of the scaler to fetch",
	)
}

// Run sets up the cluster for executing the benchmarking operation by creating a docker
// runtime cluster using 'kwokctl' (this includes control plane components and the scaler).
// Then it deploys all the objects present in the snapshot in the cluster alongwith the
// artefacts produced by the "setup" subcommand.
func Run(
	execCtx context.Context,
	cfg *envconf.Config,
	scaler ExecScaler,
	args ExecArgs,
	clusterName string,
) (Summary, error) {
	summary := Summary{}
	// Set the log entry prefix time to UTC so that it matches the container
	// log times and the metric timestamps
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC | log.Lshortfile)

	clusterSnapshot, err := benchutil.LoadJSONFromFile[planner.ClusterSnapshot](args.SnapshotFile)
	if err != nil {
		return summary, fmt.Errorf("cannot load cluster snapshot: %v", err)
	}

	if err := deployObjects(execCtx, cfg, clusterSnapshot); err != nil {
		return summary, fmt.Errorf("error running KWOK cluster: %v", err)
	}
	if err := scaler.DeployScalerData(execCtx, cfg, args.ScenarioDir); err != nil {
		return summary, fmt.Errorf("error deploying the scaler data: %v", err)
	}
	if err := scaler.DeployNodes(execCtx, cfg, &clusterSnapshot); err != nil {
		return summary, fmt.Errorf("error deploying nodes: %v", err)
	}

	scheduled, unscheduled, daemonSetPods := partitionPods(clusterSnapshot.Pods)

	log.Printf("Deploying scheduled pods, count %d...", len(scheduled))
	if err := deployPodsAndOwners(execCtx, cfg, scheduled); err != nil {
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
		NodeCount:                   len(clusterSnapshot.Nodes),
		ScheduledPods:               len(scheduled),
		UnscheduledNonDaemonSetPods: len(unscheduled) - len(daemonSetPods),
	}

	mon, err := newMonitor(execCtx, cfg, &meta, clusterName, args.ScenarioDir)
	if err != nil {
		return summary, fmt.Errorf("monitoring setup failed: %v", err)
	}

	if err := mon.start(execCtx, scaler.EventConfig(), daemonSetPods); err != nil {
		return summary, fmt.Errorf("monitoring start failed: %v", err)
	}
	defer mon.ec.Stop()

	log.Printf("Deploying unscheduled pods, count %d...", len(unscheduled))
	if err := deployPodsAndOwners(execCtx, cfg, unscheduled); err != nil {
		return summary, fmt.Errorf("error deploying unscheduled pods: %v", err)
	}
	for _, p := range unscheduled {
		fmt.Printf("Deployed %q\n", p.Name)
	}
	log.Printf("Deployed all %d unscheduled pods", len(unscheduled))

	waitPeriod, _ := time.ParseDuration(args.WaitPeriod)
	err = mon.ec.Poll(execCtx, waitPeriod)
	if err != nil {
		log.Printf("Event collector wait interrupted: %v", err)
	}

	pricingFile := path.Join(args.ScenarioDir, "gen", benchutil.FileNamePricingData)
	pricingData, err := pricing.GetInstancePricingAccess("dummy-provider", pricingFile)
	if err != nil {
		return summary, fmt.Errorf("error parsing pricing data: %v", err)
	}
	mon.stop(pricingData)

	// log.Println("Successfully completed!")
	return mon.meta.Summary, nil
}

func init() {
	// Register apiextensionsv1 so that client can create CRD objects.
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

// setupCluster creates a fresh KWOK cluster configured for the given scaler.
func setupCluster(
	ctx context.Context,
	args ExecArgs,
	clusterName string,
) (context.Context, *envconf.Config, error) {
	log.Printf("Setting up KWOK cluster %q...\n", clusterName)

	// Check if generated directory is present, create output directory if not present
	genDir := path.Join(args.ScenarioDir, "gen")
	outputDir := path.Join(args.ScenarioDir, "out")
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		return ctx, nil, fmt.Errorf("cannot create output data directory: %w", err)
	}

	outputKwokCfgFile, err := generateAllConfigs(args, genDir, outputDir, clusterName)
	if err != nil {
		return ctx, nil, err
	}

	createClusterFunc := envfuncs.CreateClusterWithConfig(kwok.NewProvider(), clusterName, outputKwokCfgFile)

	envCfg := env.New().EnvConf()
	ctx, err = createClusterFunc(ctx, envCfg)
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to create cluster: %w", err)
	}

	restCfg := envCfg.Client().RESTConfig()
	// Create a client with API server warnings suppressed (otherwise there's a
	// lot of noise e.g. due to pod names exceeding the 63-char DNS label limit).
	restCfg.WarningHandler = rest.NoWarnings{}
	// Disable rate limiting to speed up the deployment
	restCfg.QPS = -1
	client, err := klient.New(restCfg)
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to create client: %w", err)
	}
	envCfg.WithClient(client)

	return ctx, envCfg, nil
}

// cleanupCluster exports pod/container logs and (unless --skip-cleanup is set)
// it destroys the KWOK cluster.
func cleanupCluster(
	ctx context.Context,
	cfg *envconf.Config,
	args ExecArgs,
	clusterName string,
) {
	outputDir := path.Join(args.ScenarioDir, "out")

	if err := os.MkdirAll(outputDir, 0750); err != nil {
		fmt.Printf("Warning: Failed to create logs directory %q: %v\n", outputDir, err)
	} else {
		exportLogsFunc := envfuncs.ExportClusterLogs(clusterName, outputDir)
		if _, err := exportLogsFunc(ctx, cfg); err != nil {
			fmt.Printf("Warning: Failed to export logs: %v\n", err)
		} else {
			fmt.Printf("\nExported logs, reports and metrics to %q\n", path.Join(outputDir, "kwok-"+clusterName))
		}
	}

	if !args.SkipCleanup {
		log.Println("Cleaning up the cluster...")
		destroyClusterFunc := envfuncs.DestroyCluster(clusterName)
		if _, err := destroyClusterFunc(ctx, cfg); err != nil {
			fmt.Printf("Warning: Failed to destroy cluster: %v\n", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Template & config helpers
// ---------------------------------------------------------------------------

func generateAllConfigs(args ExecArgs, genDir, outputDir, clusterName string) (string, error) {
	scaler, _ := getScaler(args.Scaler)

	kubeSchedulerConfigPath := path.Join(genDir, "kube-scheduler-config.yaml")
	if err := writeEmbeddedKubeSchedulerConfig(kubeSchedulerConfigPath); err != nil {
		return "", fmt.Errorf("cannot write kube-scheduler config: %w", err)
	}

	promConfigPath := path.Join(genDir, "prometheus-config.yaml")
	scalerPort := scaler.GetPrometheusPort()
	if err := writePrometheusConfig(promConfigPath, clusterName, args.Scaler, scalerPort); err != nil {
		return "", fmt.Errorf("cannot write prometheus config: %w", err)
	}

	prometheusDataPath := path.Join(outputDir, "kwok-"+clusterName)
	if err := os.MkdirAll(prometheusDataPath, 0750); err != nil {
		return "", fmt.Errorf("cannot create prometheus data directory: %w", err)
	}

	outputKwokCfgFile := path.Join(genDir, "kwok-config.yaml")
	templateParams := KwokctlConfigTemplateParams{
		HomeDir:                 os.Getenv("HOME"),
		ClusterName:             clusterName,
		KubeSchedulerConfigPath: kubeSchedulerConfigPath,
		OutputPath:              outputKwokCfgFile,
		ScenarioDirectory:       args.ScenarioDir,
		ImageTag:                args.ScalerVersion,
		PrometheusConfigPath:    promConfigPath,
		PrometheusDataPath:      prometheusDataPath,
	}

	err := generateKwokctlConfig(templateParams, scaler, args.ConfigFile)
	if err != nil {
		return "", fmt.Errorf("cannot create kwok config: %w", err)
	}
	log.Printf("Wrote kwok config template to %q\n", templateParams.OutputPath)
	return outputKwokCfgFile, nil
}

func generateKwokctlConfig(params KwokctlConfigTemplateParams, scaler ExecScaler, configPath string) error {
	var (
		data               []byte
		templateConfigPath string
		err                error
	)
	if configPath != "" {
		templateConfigPath = configPath
		data, err = os.ReadFile(filepath.Clean(templateConfigPath))
		if err != nil {
			return fmt.Errorf("cannot read %s from filesystem: %w", templateConfigPath, err)
		}
	} else {
		templateConfigPath = scaler.GetKWOKTemplatePath()
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

func writeEmbeddedKubeSchedulerConfig(destPath string) error {
	kubeSchedulerConfigData, err := content.ReadFile("templates/kube-scheduler-config.yaml")
	if err != nil {
		return fmt.Errorf("cannot read kube-scheduler-config.yaml from embedded FS: %w", err)
	}
	if err := os.WriteFile(destPath, kubeSchedulerConfigData, 0600); err != nil {
		return fmt.Errorf("cannot write kube-scheduler config to %q: %w", destPath, err)
	}
	return nil
}
