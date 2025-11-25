// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path"
	"slices"
	"syscall"

	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kwok"
	"sigs.k8s.io/yaml"
)

var (
	skipCleanup  bool
	snapshotFile string
)

func init() {
	rootCmd.AddCommand(execCmd)

	execCmd.PersistentFlags().StringVar(
		&snapshotFile,
		"snap",
		"/tmp", // FIXME
		"output data directory",
	)

	execCmd.PersistentFlags().BoolVarP(
		&skipCleanup,
		"skip-cleanup", "s",
		false,
		"delete cluster with all data upon finishing",
	)
}

var execCmd = &cobra.Command{
	// Run (deploy objects, possibly cleaning the cluster) and benchmark
	Use:   "exec <scaler> <options>", // data/scenario/report directories
	Short: "Run the scaler by utilizing the data and produce the report",
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		fmt.Println("benchmark exec called")

		return setupTestEnv()
	},
}

// REF: https://medium.com/programming-kubernetes/testing-kubernetes-controllers-with-the-e2e-framework-fac232843dc6
func setupTestEnv() error {
	testenv := env.New()
	// FIXME use template
	kwokClusterName := "kwok-test-cluster" //envconf.RandomName("kwok-cluster", 16)

	ctx := setupSignalHandler()
	var cfg *envconf.Config

	log.Printf("Setting up KWOK cluster %q...\n", kwokClusterName)
	createClusterFunc := envfuncs.CreateClusterWithConfig(kwok.NewProvider(), kwokClusterName, "kwok-config.yaml")
	cfg = testenv.EnvConf()
	ctx, err := createClusterFunc(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}

	defer func() {
		// FIXME use outputDir instead
		pwd, _ := os.Getwd()
		logsDir := path.Join(pwd, "./logs", "kwok-"+kwokClusterName)

		notEmpty, _ := isDirNonEmpty(logsDir)
		if notEmpty {
			err := os.RemoveAll(logsDir)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
			// fmt.Printf("Directory %q deleted successfully\n", logsDir)
		}

		exportLogsFunc := envfuncs.ExportClusterLogs(kwokClusterName, "./logs")
		if _, err := exportLogsFunc(ctx, cfg); err != nil {
			log.Printf("Warning: Failed to export logs: %v", err)
		}

		if !skipCleanup {
			log.Println("Cleaning up...")
			destroyClusterFunc := envfuncs.DestroyCluster(kwokClusterName)
			if _, err := destroyClusterFunc(ctx, cfg); err != nil {
				log.Printf("Warning: Failed to destroy cluster: %v", err)
			}
		}
	}()

	if err := runKwokCluster(ctx, cfg, snapshotFile); err != nil {
		return fmt.Errorf("Error running KWOK cluster test: %v", err)
	}
	log.Println("Successfully completed!")
	<-ctx.Done()

	return nil
}

func runKwokCluster(ctx context.Context, cfg *envconf.Config, snapshotFile string) error {
	file, err := os.Open(snapshotFile)
	if err != nil {
		return fmt.Errorf("Could not open the clusterSnapshot file %q: %v", snapshotFile, err)
	}
	defer file.Close()

	clusterSnapshotData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("Could not read the clusterSnapshot file %q: %v", file.Name(), err)
	}
	clusterSnapshot := svcapi.ClusterSnapshot{}
	if err := json.Unmarshal(clusterSnapshotData, &clusterSnapshot); err != nil {
		return fmt.Errorf("Could not unmarshal the clusterSnapshot data for %q: %v", file.Name(), err)
	}

	err = deployPriorityClasses(ctx, clusterSnapshot, cfg)
	if err != nil {
		return err
	}

	err = deployAndUntaintNodes(ctx, clusterSnapshot, cfg)
	if err != nil {
		return err
	}

	err = createNamespaces(ctx, clusterSnapshot, cfg)
	if err != nil {
		return err
	}

	err = createDefaultServiceAccount(ctx, cfg)
	if err != nil {
		return err
	}

	// TODO toggle based on the scalar
	caKwokCfgFile := path.Join(path.Dir(snapshotFile), "ca-kwok-provider-config.yaml")
	err = deployCAKwokConfig(ctx, caKwokCfgFile, cfg)
	if err != nil {
		return err
	}
	// FIXME
	templateFilePath := path.Join(path.Dir(snapshotFile), "ca-kwok-provider-template.yaml")
	err = deployCAKwokTemplate(ctx, templateFilePath, cfg)
	if err != nil {
		return err
	}

	err = deployPods(ctx, clusterSnapshot, cfg)
	if err != nil {
		return err
	}

	return nil
}

func deployPods(ctx context.Context, clusterSnapshot svcapi.ClusterSnapshot, cfg *envconf.Config) error {
	scheduled, unscheduled := partitionPods(clusterSnapshot.Pods)
	log.Printf("Deploying scheduled pods, count %d...", len(scheduled))
	for _, pInfo := range scheduled {
		p := podutil.AsPod(pInfo)
		if p.Spec.Containers[0].Image == "" {
			p.Spec.Containers[0].Image = p.Name + "-dummy-image"
		}
		p.Spec.Containers[0].Name = "abcdefgh"
		if err := cfg.Client().Resources().Create(ctx, p); err != nil {
			return fmt.Errorf("failed to create pod: %w", err)
		}
	}
	log.Printf("Deploying unscheduled pods, count %d...", len(unscheduled))
	for _, pInfo := range unscheduled {
		p := podutil.AsPod(pInfo)
		if p.Spec.Containers[0].Image == "" {
			p.Spec.Containers[0].Image = p.Name + "-dummy-image"
		}
		// p.Spec.Containers[0].Image = "abcd" // FIXME
		fmt.Printf("Deploying unscheduled pod %q\n", p.Name)
		if err := cfg.Client().Resources().Create(ctx, p); err != nil {
			return fmt.Errorf("failed to create pod: %w", err)
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

func createNamespaces(ctx context.Context, clusterSnapshot svcapi.ClusterSnapshot, cfg *envconf.Config) error {
	log.Println("Creating missing namespaces...")
	for _, p := range clusterSnapshot.Pods {
		ns := &corev1.Namespace{}
		ns.Name = p.Namespace
		if err := cfg.Client().Resources().Create(ctx, ns); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create namespace: %w", err)
			}
			// log.Printf("Namespace %s already exists, skipping\n", p.Namespace)
		}
	}
	return nil
}

func deployAndUntaintNodes(ctx context.Context, clusterSnapshot svcapi.ClusterSnapshot, cfg *envconf.Config) error {
	log.Printf("Deploying nodes, count %d...\n", len(clusterSnapshot.Nodes))
	for _, nInfo := range clusterSnapshot.Nodes {
		n := nodeutil.AsNode(nInfo)
		n.Spec.ProviderID = "kwok://" + n.Name // fixes not managed by kwok
		if err := cfg.Client().Resources().Create(ctx, n); err != nil {
			return fmt.Errorf("failed to create node: %w", err)
		}

		if err := cfg.Client().Resources().Get(ctx, n.Name, "", n); err != nil {
			return fmt.Errorf("failed to fetch node: %w", err)
		}

		n.Spec.Taints = slices.DeleteFunc(n.Spec.Taints, func(item corev1.Taint) bool {
			return item.Key == corev1.TaintNodeNotReady
		})

		if err := cfg.Client().Resources().Update(ctx, n); err != nil {
			return fmt.Errorf("failed to update node: %w", err)
		}
	}
	return nil
}

func deployPriorityClasses(ctx context.Context, clusterSnapshot svcapi.ClusterSnapshot, cfg *envconf.Config) error {
	log.Println("Deploying priority classes...")
	for _, pClass := range clusterSnapshot.PriorityClasses {
		if err := cfg.Client().Resources().Create(ctx, &pClass); err != nil {
			return fmt.Errorf("failed to create priorityClass: %w", err)
		}
	}
	return nil
}

func partitionPods(pods []svcapi.PodInfo) (scheduled, unscheduled []svcapi.PodInfo) {
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

func isDirNonEmpty(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	if !info.IsDir() {
		return false, fmt.Errorf("%s is not a directory", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}

	return len(entries) > 0, nil
}

// CA -----------------------------------------

func deployCAKwokTemplate(ctx context.Context, templateFilePath string, cfg *envconf.Config) error {
	log.Printf("Deploying CA kwok-provider-templates %q...\n", templateFilePath)
	file, err := os.Open(templateFilePath)
	if err != nil {
		return fmt.Errorf("Could not open the kwok cr file %q: %v", templateFilePath, err)
	}
	defer file.Close()

	kwokTemplateData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("Could not read the kwok cr file %q: %v", file.Name(), err)
	}
	kwokProviderTemplate := corev1.ConfigMap{}
	if err := yaml.Unmarshal(kwokTemplateData, &kwokProviderTemplate); err != nil {
		return fmt.Errorf("Could not unmarshal the kwokTemplate data for %q: %v", file.Name(), err)
	}
	if err := cfg.Client().Resources().Create(ctx, &kwokProviderTemplate); err != nil {
		return fmt.Errorf("failed to create kwok provider template: %w", err)
	}
	return nil
}

func deployCAKwokConfig(ctx context.Context, caKwokCfgFile string, cfg *envconf.Config) error {
	// FIXME
	log.Printf("Deploying CA kwok-provider-config %q...\n", caKwokCfgFile)
	file, err := os.Open(caKwokCfgFile)
	if err != nil {
		return fmt.Errorf("Could not open the kwok provider config file %q: %v", caKwokCfgFile, err)
	}
	defer file.Close()

	kwokConfigData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("Could not read the kwok provider config file %q: %v", file.Name(), err)
	}
	kwokProviderConfig := corev1.ConfigMap{}
	if err := yaml.Unmarshal(kwokConfigData, &kwokProviderConfig); err != nil {
		return fmt.Errorf("Could not unmarshal the kwokConfig data for %q: %v", file.Name(), err)
	}
	// fmt.Printf("Kwok provider cfg data is:\n%#v\n", kwokProviderConfig)

	if err := cfg.Client().Resources().Create(ctx, &kwokProviderConfig); err != nil {
		return fmt.Errorf("failed to create kwok provider config: %w", err)
	}
	return nil
}
