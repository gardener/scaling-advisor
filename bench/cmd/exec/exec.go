// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path"
	"syscall"
	"text/template"

	"github.com/gardener/scaling-advisor/api/planner"
	bench "github.com/gardener/scaling-advisor/bench/cmd"
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

var (
	skipCleanup   bool
	snapshotFile  string
	scalerVersion string
)

type ExecScaler interface {
	DeployScalerData(ctx context.Context, cfg *envconf.Config) error
	GetScalerKWOKTemplatePath() string
	CheckRequiredDataPresent(scenarioDir, version string) error
}

func init() {
	// Register apiextensionsv1 types (CustomResourceDefinition) with the global
	// client-go scheme so that the e2e-framework client can create CRD objects.
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
	// TODO: report stuff
	// TODO: might need this for the compare data
	// execCmd.PersistentFlags().StringVarP(
	// 	&pricingFile,
	// 	"pricing-data", "p",
	// 	"",
	// 	"pricing data file",
	// )
}

var execCmd = &cobra.Command{
	Use:   "exec <scaler> <options>", // data/scenario/report directories
	Short: "Run the scaler by utilizing the data and produce the report",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		scalerName := args[0]
		s, err := getScaler(scalerName)
		if err != nil {
			return
		}

		scenarioDir := path.Dir(snapshotFile)
		err = s.CheckRequiredDataPresent(scenarioDir, scalerVersion)
		if err != nil {
			return fmt.Errorf("please run 'setup' before running 'exec': %v", err)
		}

		ctx := setupSignalHandler()
		kwokClusterName := envconf.RandomName("kwok-cluster", 17)
		// e2e-framework updates the context information, hence we need to update the
		// context with the one that gets returned
		ctx, cfg, err := setupClusterForScaling(ctx, s, kwokClusterName)
		if err != nil {
			return err
		}

		clusterSnapshot, err := getClusterSnapshot(snapshotFile)
		if err != nil {
			return
		}

		if err := deployObjects(ctx, cfg, clusterSnapshot); err != nil {
			return fmt.Errorf("error running KWOK cluster: %v", err)
		}
		if err := s.DeployScalerData(ctx, cfg); err != nil {
			return fmt.Errorf("error deploying the scaler data: %v", err)
		}
		if err := deployPods(ctx, clusterSnapshot, cfg); err != nil {
			return fmt.Errorf("error deploying pods: %v", err)
		}

		defer func() {
			logsDir := path.Join(scenarioDir, "logs")

			if err := os.MkdirAll(logsDir, 0755); err != nil {
				fmt.Printf("Warning: Failed to create logs directory %q: %v\n", logsDir, err)
			} else {
				exportLogsFunc := envfuncs.ExportClusterLogs(kwokClusterName, logsDir)
				if _, err = exportLogsFunc(ctx, cfg); err != nil {
					fmt.Printf("Warning: Failed to export logs: %v\n", err)
				} else {
					log.Printf("Exported logs to %q\n", logsDir)
				}
			}

			if !skipCleanup {
				log.Println("Cleaning up...")
				destroyClusterFunc := envfuncs.DestroyCluster(kwokClusterName)
				if _, err := destroyClusterFunc(ctx, cfg); err != nil {
					fmt.Printf("Warning: Failed to destroy cluster: %v\n", err)
				}
			}
		}()

		log.Println("Successfully completed!")
		<-ctx.Done()

		return
	},
}

type KwokCfgTmplParams struct {
	HomeDir                 string
	ClusterName             string
	KubeSchedulerConfigPath string
	OutputPath              string
	ScenarioDirectory       string
	ImageTag                string
}

func setupClusterForScaling(ctx context.Context, s ExecScaler, kwokClusterName string) (context.Context, *envconf.Config, error) {
	testenv := env.New()
	scenarioDir := path.Dir(snapshotFile)
	outputFile := path.Join(scenarioDir, "kwok-config.yaml")

	log.Printf("Setting up KWOK cluster %q...\n", kwokClusterName)
	// This is needed to provide an absolute path to kwokctlConfiguration
	kubeSchedulerConfigPath, err := writeEmbeddedKubeSchedulerConfig()
	if err != nil {
		return ctx, nil, fmt.Errorf("cannot write kube-scheduler config: %w", err)
	}
	defer os.Remove(kubeSchedulerConfigPath)

	kwokCfgTmplParams := KwokCfgTmplParams{
		HomeDir:                 os.Getenv("HOME"),
		ClusterName:             kwokClusterName,
		KubeSchedulerConfigPath: kubeSchedulerConfigPath,
		OutputPath:              outputFile,
		ScenarioDirectory:       scenarioDir,
		ImageTag:                scalerVersion,
	}

	err = generateKwokConfig(kwokCfgTmplParams, s.GetScalerKWOKTemplatePath())
	if err != nil {
		return ctx, nil, fmt.Errorf("cannot create kwok config: %w", err)
	}
	log.Printf("Wrote kwok config template to %q\n", kwokCfgTmplParams.OutputPath)

	createClusterFunc := envfuncs.CreateClusterWithConfig(kwok.NewProvider(), kwokClusterName, outputFile)
	cfg := testenv.EnvConf()
	ctx, err = createClusterFunc(ctx, cfg)
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to create cluster: %w", err)
	}

	return ctx, cfg, nil
}

func getClusterSnapshot(snapshotFile string) (snapshot planner.ClusterSnapshot, err error) {
	file, err := os.Open(snapshotFile)
	if err != nil {
		err = fmt.Errorf("cannot open the clusterSnapshot file %q: %v", snapshotFile, err)
		return
	}
	defer file.Close()

	clusterSnapshotData, err := io.ReadAll(file)
	if err != nil {
		err = fmt.Errorf("cannot read the clusterSnapshot file %q: %v", file.Name(), err)
		return
	}

	if err = json.Unmarshal(clusterSnapshotData, &snapshot); err != nil {
		err = fmt.Errorf("cannot unmarshal the clusterSnapshot data for %q: %v", file.Name(), err)
		return
	}

	return
}

func deployObjects(ctx context.Context, cfg *envconf.Config, clusterSnapshot planner.ClusterSnapshot) (err error) {
	err = deployPriorityClasses(ctx, clusterSnapshot, cfg)
	if err != nil {
		return
	}

	err = deployNodes(ctx, clusterSnapshot, cfg)
	if err != nil {
		return
	}

	err = createNamespaces(ctx, clusterSnapshot, cfg)
	if err != nil {
		return
	}

	// "runtime: kind" doesn't need this
	err = createDefaultServiceAccount(ctx, cfg)
	if err != nil {
		return
	}

	return
}

func deployPods(ctx context.Context, clusterSnapshot planner.ClusterSnapshot, cfg *envconf.Config) error {
	scheduled, unscheduled := partitionPods(clusterSnapshot.Pods)
	log.Printf("Deploying scheduled pods, count %d...", len(scheduled))
	for _, pInfo := range scheduled {
		p := podutil.AsPod(pInfo)
		if p.Spec.Containers[0].Image == "" {
			p.Spec.Containers[0].Image = p.Name + "-dummy-image"
		}
		p.Spec.Containers[0].Name = "abcdefgh"
		p.ResourceVersion = ""
		p.UID = ""
		if err := cfg.Client().Resources().Create(ctx, p); err != nil {
			return fmt.Errorf("failed to create pod %q: %w", p.Name, err)
		}
	}
	log.Printf("Deploying unscheduled pods, count %d...", len(unscheduled))
	for _, pInfo := range unscheduled {
		p := podutil.AsPod(pInfo)
		if p.Spec.Containers[0].Image == "" {
			p.Spec.Containers[0].Image = p.Name + "-dummy-image"
		}
		p.ResourceVersion = ""
		p.UID = ""
		log.Printf("Deploying unscheduled pod %q\n", p.Name)
		if err := cfg.Client().Resources().Create(ctx, p); err != nil {
			return fmt.Errorf("failed to create pod %q: %w", p.Name, err)
		}
	}
	return nil
}

func createDefaultServiceAccount(ctx context.Context, cfg *envconf.Config) error {
	log.Println("Creating default serviceaccounts...")
	nsList := &corev1.NamespaceList{}
	if err := cfg.Client().Resources().List(ctx, nsList); err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}
	for _, ns := range nsList.Items {
		sa := corev1.ServiceAccount{}
		sa.Name = "default"
		sa.Namespace = ns.Name
		if err := cfg.Client().Resources().Create(ctx, &sa); err != nil {
			return fmt.Errorf("failed to create sa: %w", err)
		}
	}
	return nil
}

func createNamespaces(ctx context.Context, clusterSnapshot planner.ClusterSnapshot, cfg *envconf.Config) error {
	log.Println("Creating missing namespaces...")
	for _, p := range clusterSnapshot.Pods {
		ns := &corev1.Namespace{}
		ns.Name = p.ObjectMeta.Namespace
		if err := cfg.Client().Resources().Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create namespace: %w", err)
			// log.Printf("Namespace %s already exists, skipping\n", p.Namespace)
		}
	}
	return nil
}

func deployNodes(ctx context.Context, clusterSnapshot planner.ClusterSnapshot, cfg *envconf.Config) error {
	log.Printf("Deploying nodes, count %d...\n", len(clusterSnapshot.Nodes))
	for _, nInfo := range clusterSnapshot.Nodes {
		n := nodeutil.AsNode(nInfo)
		n.Spec.ProviderID = "kwok://" + n.Name // fixes not managed by kwok
		n.ResourceVersion = ""
		if err := cfg.Client().Resources().Create(ctx, n); err != nil {
			return fmt.Errorf("failed to create node: %w", err)
		}
	}
	return nil
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

func partitionPods(pods []planner.PodInfo) (scheduled, unscheduled []planner.PodInfo) {
	for _, p := range pods {
		if p.NodeName == "" {
			unscheduled = append(unscheduled, p)
		} else {
			scheduled = append(scheduled, p)
		}
	}
	return
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
	err = templateConfig.Execute(&buf, params)
	if err != nil {
		return fmt.Errorf("cannot render %q template with params %q: %w", templateConfig.Name(), params, err)
	}
	err = os.WriteFile(params.OutputPath, buf.Bytes(), 0600)
	if err != nil {
		return fmt.Errorf("cannot write kwok config to %q: %w", params.OutputPath, err)
	}
	return nil
}

func writeEmbeddedKubeSchedulerConfig() (string, error) {
	kubeSchedulerConfigData, err := content.ReadFile("templates/kube-scheduler-config.yaml")
	if err != nil {
		return "", fmt.Errorf("cannot read kube-scheduler-config.yaml from embedded FS: %w", err)
	}

	// Create a temporary file
	tempFile, err := os.CreateTemp("", "kube-scheduler-config.yaml")
	if err != nil {
		return "", fmt.Errorf("cannot create temporary file: %w", err)
	}
	defer tempFile.Close()

	if _, err := tempFile.Write(kubeSchedulerConfigData); err != nil {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("cannot write to temporary file: %w", err)
	}

	return tempFile.Name(), nil
}

func getScaler(scalerName string) (ExecScaler, error) {
	switch scalerName {
	case bench.ScalerKarpenter:
		return &karpenterExec{}, nil
	case bench.ScalerClusterAutoscaler:
		return &caExec{}, nil
	default:
		return nil, fmt.Errorf("unknown scaler %q", scalerName)
	}
}
