// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"

	bench "github.com/gardener/scaling-advisor/bench/cmd"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	karpenterkwokapis "sigs.k8s.io/karpenter/kwok/apis"
	karpenterkwokv1alpha1 "sigs.k8s.io/karpenter/kwok/apis/v1alpha1"
	karpenterapis "sigs.k8s.io/karpenter/pkg/apis"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	sigyaml "sigs.k8s.io/yaml"
)

var _ ExecScaler = (*karpenterExec)(nil)

type karpenterExec struct{}

const karpKwokTemplatePath = "templates/kwok-karp-tmpl.yaml"

func (ke *karpenterExec) DeployScalerData(ctx context.Context, cfg *envconf.Config) (err error) {
	snapshotDir := path.Dir(snapshotFile)
	err = deployKarpenterCRDs(ctx, cfg)
	if err != nil {
		return
	}

	classesFilePath := path.Join(snapshotDir, bench.FileNameKarpenterNodeClasses)
	poolsFilePath := path.Join(snapshotDir, bench.FileNameKarpenterNodePools)
	err = deployKarpenterPools(ctx, poolsFilePath, cfg)
	if err != nil {
		return
	}
	err = deployKarpenterClasses(ctx, classesFilePath, cfg)
	if err != nil {
		return
	}
	return
}

func (ke *karpenterExec) GetScalerKWOKTemplatePath() string {
	return karpKwokTemplatePath
}

func (ke *karpenterExec) CheckRequiredDataPresent(scenarioDir, scalerVersion string) error {
	// Check files and image with tag present in docker
	imageName := fmt.Sprintf("karpenter.local/kwok:%s", scalerVersion)
	if exists := bench.CheckIfImageExists(imageName); !exists {
		return fmt.Errorf("required image %q not found", imageName)
	}

	instanceTypesFile := path.Join(scenarioDir, bench.FileNameKarpenterInstanceTypes)
	nodePoolsFile := path.Join(scenarioDir, bench.FileNameKarpenterNodePools)
	nodeClassesFile := path.Join(scenarioDir, bench.FileNameKarpenterNodeClasses)

	for _, filePath := range []string{instanceTypesFile, nodePoolsFile, nodeClassesFile} {
		if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("required file %q not found", filePath)
		}
	}
	return nil
}

// deployKarpenterCRDs installs the Karpenter and KWOK CRDs into the cluster so that the
// API server recognises NodePool, NodeClaim, and KWOKNodeClass resources.
func deployKarpenterCRDs(ctx context.Context, cfg *envconf.Config) error {
	log.Println("Deploying Karpenter CRDs...")

	allCRDs := append(karpenterapis.CRDs, karpenterkwokapis.CRDs...)
	for _, crd := range allCRDs {
		crd = crd.DeepCopy()
		crd.ResourceVersion = ""
		if err := cfg.Client().Resources().Create(ctx, crd); err != nil {
			if !k8serrors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create CRD %q: %w", crd.Name, err)
			}
			log.Printf("CRD %q already exists, skipping\n", crd.Name)
		} else {
			log.Printf("Created CRD %q\n", crd.Name)
		}
	}
	return nil
}

func deployKarpenterPools(ctx context.Context, poolsFilePath string, cfg *envconf.Config) error {
	log.Printf("Deploying karpenter nodePools %q...\n", poolsFilePath)
	file, err := os.Open(poolsFilePath)
	if err != nil {
		return fmt.Errorf("cannot open the node pools file %q: %v", poolsFilePath, err)
	}
	defer file.Close()

	karpPoolData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("cannot read the node pools file %q: %v", file.Name(), err)
	}
	nodePools := karpenterv1.NodePoolList{}
	if err := sigyaml.Unmarshal(karpPoolData, &nodePools); err != nil {
		return fmt.Errorf("cannot unmarshal the pool data for %q: %v", file.Name(), err)
	}
	for _, pool := range nodePools.Items {
		if err := cfg.Client().Resources().Create(ctx, &pool); err != nil {
			return fmt.Errorf("failed to create node pool: %w", err)
		}
	}
	return nil
}

func deployKarpenterClasses(ctx context.Context, classesFilePath string, cfg *envconf.Config) error {
	log.Printf("Deploying karpenter nodeClasses %q...\n", classesFilePath)
	file, err := os.Open(classesFilePath)
	if err != nil {
		return fmt.Errorf("cannot open the node classes file %q: %v", classesFilePath, err)
	}
	defer file.Close()

	karpClassData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("cannot read the node classes file %q: %v", file.Name(), err)
	}
	nodeClasses := karpenterkwokv1alpha1.KWOKNodeClassList{}
	if err := sigyaml.Unmarshal(karpClassData, &nodeClasses); err != nil {
		return fmt.Errorf("cannot unmarshal the class data for %q: %v", file.Name(), err)
	}
	for _, class := range nodeClasses.Items {
		if err := cfg.Client().Resources().Create(ctx, &class); err != nil {
			return fmt.Errorf("failed to create node class: %w", err)
		}
	}
	return nil
}
