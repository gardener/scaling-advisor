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
	"time"

	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"

	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	karpenterkwokapis "sigs.k8s.io/karpenter/kwok/apis"
	karpenterkwokv1alpha1 "sigs.k8s.io/karpenter/kwok/apis/v1alpha1"
	karpenterapis "sigs.k8s.io/karpenter/pkg/apis"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

var _ ExecScaler = (*karpenterExec)(nil)

type karpenterExec struct{}

const (
	karpenterKwokTemplatePath = "templates/kwok-karpenter-tmpl.yaml"
	karpenterPrometheusPort   = 8080
)

// In case of karpenter, the snapshot.Pods are mutated before deployment to update
// the nodeNames with the names created by the kwok cloudprovider using the NodeClaim
// This is required since karpenter only manages nodes which have a corresponding
// 'NodeClaim'; while not strictly necessary for Scale-Outs, claims are needed for
// running 'Scale-In' scenarios where consolidation is active only when 'NodeClaim' is
// present for a node.
func (ke *karpenterExec) DeployNodes(ctx context.Context, cfg *envconf.Config, snapshot *planner.ClusterSnapshot) error {
	log.Printf("Deploying nodes, count %d...\n", len(snapshot.Nodes))
	var (
		nodePools     karpenterv1.NodePoolList
		nodeClaimList karpenterv1.NodeClaimList
	)
	err := cfg.Client().Resources().List(ctx, &nodePools)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	for _, nodeInfo := range snapshot.Nodes {
		node := nodeutil.AsNode(nodeInfo)
		node.ResourceVersion = ""
		if node.Annotations == nil {
			node.Annotations = make(map[string]string)
		}
		node.Annotations["kwok.x-k8s.io/node"] = "fake"
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		node.Labels["karpenter.sh/nodepool"] = node.Labels["worker.gardener.cloud/pool"]

		nodePool, err := findNodePoolForNode(node, nodePools.Items)
		if err != nil {
			fmt.Printf("WARN: nodePool corresponding to node %q not found, cannot create a fake NodeClaim\n", node.Name)
			if err := cfg.Client().Resources().Create(ctx, node); err != nil {
				return fmt.Errorf("failed to create node: %w", err)
			}
			continue
		}

		nodeClaim := constructNodeClaimForNode(node, nodePool)
		if err := cfg.Client().Resources().Create(ctx, &nodeClaim); err != nil {
			return fmt.Errorf("failed to create nodeClaim: %w", err)
		}
	}
	// Wait until len(NodeClaimList.Items) == len(NodeList.Items) or for 5s?
	// TODO: check if this can be done with the nodeList (owner gives the old name)?
	nodeMultDuration := time.Duration(50*len(snapshot.Nodes)) * time.Millisecond
	time.Sleep(max(nodeMultDuration, 5*time.Second))
	err = cfg.Client().Resources().List(ctx, &nodeClaimList)
	if err != nil {
		return err
	}
	for _, claim := range nodeClaimList.Items {
		for i := range snapshot.Pods {
			if snapshot.Pods[i].NodeName == claim.Name {
				// Update NodeName with the name created by the kwok cloudprovider
				// when creating the node for the claim
				snapshot.Pods[i].NodeName = claim.Status.NodeName
			}
		}
	}

	return nil
}

func (ke *karpenterExec) DeployScalerData(ctx context.Context, cfg *envconf.Config, scenarioDir string) (err error) {
	// Need to deploy karpenter CRDs to create 'NodeClaim', 'NodePool' and 'KWOKNodeClass'
	err = deployKarpenterCRDs(ctx, cfg)
	if err != nil {
		return err
	}

	poolsFilePath := path.Join(scenarioDir, "gen", benchutil.FileNameKarpenterNodePools)
	err = deployKarpenterPools(ctx, cfg, poolsFilePath)
	if err != nil {
		return
	}

	classesFilePath := path.Join(scenarioDir, "gen", benchutil.FileNameKarpenterNodeClasses)
	err = deployKarpenterClasses(ctx, cfg, classesFilePath)
	if err != nil {
		return
	}

	return
}

func (ke *karpenterExec) GetKWOKTemplatePath() string {
	return karpenterKwokTemplatePath
}

func (ke *karpenterExec) GetPrometheusPort() int {
	return karpenterPrometheusPort
}

func (ke *karpenterExec) EventConfig() ScalerEventConfig {
	return ScalerEventConfig{
		Source: benchutil.ScalerKarpenter,
		WatchedEvents: []string{
			"Nominated", "Launched", "NoCompatibleInstanceTypes",
			"FailedScheduling", "DisruptionTerminating",
		},
		MarksPodUnschedulable: []string{"FailedScheduling"},
	}
}

func (ke *karpenterExec) CheckRequiredDataPresent(genDir, scalerVersion string) error {
	imageName := fmt.Sprintf("karpenter.local/kwok:%s", scalerVersion)
	if exists := benchutil.CheckIfImageExists(imageName); !exists {
		return fmt.Errorf("required image %q not found", imageName)
	}

	requiredFiles := []string{
		path.Join(genDir, benchutil.FileNameKarpenterInstanceTypes),
		path.Join(genDir, benchutil.FileNameKarpenterNodePools),
		path.Join(genDir, benchutil.FileNameKarpenterNodeClasses),
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
			if !apierrors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create CRD %q: %w", crd.Name, err)
			}
			log.Printf("CRD %q already exists, skipping\n", crd.Name)
		} else {
			log.Printf("Created CRD %q\n", crd.Name)
		}
	}

	return nil
}

func findNodePoolForNode(node *corev1.Node, pools []karpenterv1.NodePool) (*karpenterv1.NodePool, error) {
	poolName, ok := node.Labels["worker.gardener.cloud/pool"]
	if !ok {
		return nil, fmt.Errorf("pool label not present on node %q", node.Name)
	}
	for _, p := range pools {
		if p.Name == poolName {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("no matching pool found for %q", poolName)
}

// constructNodeClaimForNode creates a fake NodeClaim object for existing nodes for
// karpenter to recognize those nodes and register them; to allow for proper consolidation
// and scaling
func constructNodeClaimForNode(
	node *corev1.Node,
	pool *karpenterv1.NodePool,
) karpenterv1.NodeClaim {
	return karpenterv1.NodeClaim{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        node.Name,
			Labels:      node.Labels,
			Annotations: node.Annotations,
		},
		Spec: karpenterv1.NodeClaimSpec{
			NodeClassRef: &karpenterv1.NodeClassReference{
				Kind:  "KWOKNodeClass",
				Name:  pool.Name,
				Group: karpenterkwokapis.Group,
			},
			Resources: karpenterv1.ResourceRequirements{
				Requests: node.Status.Capacity,
			},
			Requirements: pool.Spec.Template.Spec.Requirements,
			Taints:       node.Spec.Taints,
		},
		Status: karpenterv1.NodeClaimStatus{
			Allocatable: node.Status.Allocatable,
			Capacity:    node.Status.Capacity,
		},
	}
}

// deployKarpenterPools loads a NodePoolList from a YAML file and creates each
// NodePool in the cluster.
func deployKarpenterPools(ctx context.Context, cfg *envconf.Config, poolsFilePath string) error {
	log.Printf("Deploying karpenter nodePools %q...\n", poolsFilePath)

	nodePools, err := benchutil.LoadYAMLFromFile[karpenterv1.NodePoolList](poolsFilePath)
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

	nodeClasses, err := benchutil.LoadYAMLFromFile[karpenterkwokv1alpha1.KWOKNodeClassList](classesFilePath)
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
