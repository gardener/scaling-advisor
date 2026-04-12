// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"errors"
	"fmt"
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
)

var _ execScaler = (*karpenterExec)(nil)

type karpenterExec struct{}

const karpKwokTemplatePath = "templates/kwok-karp-tmpl.yaml"

func (ke *karpenterExec) DeployScalerData(ctx context.Context, cfg *envconf.Config, scenarioDir string) (err error) {
	err = deployKarpenterCRDs(ctx, cfg)
	if err != nil {
		return
	}

	poolsFilePath := path.Join(scenarioDir, bench.FileNameKarpenterNodePools)
	err = deployKarpenterPools(ctx, cfg, poolsFilePath)
	if err != nil {
		return
	}

	classesFilePath := path.Join(scenarioDir, bench.FileNameKarpenterNodeClasses)
	err = deployKarpenterClasses(ctx, cfg, classesFilePath)
	if err != nil {
		return
	}

	return
}

func (ke *karpenterExec) GetScalerKWOKTemplatePath() string {
	return karpKwokTemplatePath
}

func (ke *karpenterExec) CheckRequiredDataPresent(scenarioDir, scalerVersion string) error {
	imageName := fmt.Sprintf("karpenter.local/kwok:%s", scalerVersion)
	if exists := bench.CheckIfImageExists(imageName); !exists {
		return fmt.Errorf("required image %q not found", imageName)
	}

	requiredFiles := []string{
		path.Join(scenarioDir, bench.FileNameKarpenterInstanceTypes),
		path.Join(scenarioDir, bench.FileNameKarpenterNodePools),
		path.Join(scenarioDir, bench.FileNameKarpenterNodeClasses),
	}
	for _, filePath := range requiredFiles {
		if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("required file %q not found", filePath)
		}
	}

	return nil
}

// deployKarpenterCRDs installs the Karpenter and KWOK CRDs into the cluster
// so that the API server recognises NodePool, NodeClaim, and KWOKNodeClass
// resources.
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

// deployKarpenterPools loads a NodePoolList from a YAML file and creates each
// NodePool in the cluster.
// TODO: clean up the kubernetes.io related labels
func deployKarpenterPools(ctx context.Context, cfg *envconf.Config, poolsFilePath string) error {
	log.Printf("Deploying karpenter nodePools %q...\n", poolsFilePath)

	nodePools, err := bench.LoadYAMLFromFile[karpenterv1.NodePoolList](poolsFilePath)
	if err != nil {
		return fmt.Errorf("cannot load node pools from %q: %w", poolsFilePath, err)
	}

	for _, pool := range nodePools.Items {
		if err := cfg.Client().Resources().Create(ctx, &pool); err != nil {
			return fmt.Errorf("failed to create node pool %q: %w", pool.Name, err)
		}
	}

	return nil
}

// deployKarpenterClasses loads a KWOKNodeClassList from a YAML file and
// creates each KWOKNodeClass in the cluster.
func deployKarpenterClasses(ctx context.Context, cfg *envconf.Config, classesFilePath string) error {
	log.Printf("Deploying karpenter nodeClasses %q...\n", classesFilePath)

	nodeClasses, err := bench.LoadYAMLFromFile[karpenterkwokv1alpha1.KWOKNodeClassList](classesFilePath)
	if err != nil {
		return fmt.Errorf("cannot load node classes from %q: %w", classesFilePath, err)
	}

	for _, class := range nodeClasses.Items {
		if err := cfg.Client().Resources().Create(ctx, &class); err != nil {
			return fmt.Errorf("failed to create node class %q: %w", class.Name, err)
		}
	}

	return nil
}
