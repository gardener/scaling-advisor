// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package genscenario

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	apiv1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ShootCoordinate is a struct comprising of the information needed to uniquely
// identify a gardener Shoot cluster: Landscape, Project and Shoot name.
type ShootCoordinate struct {
	Landscape string
	Project   string
	Shoot     string
}

var (
	shootCoords           ShootCoordinate
	scenarioDir           string
	excludeKubeSystemPods bool
)

// shootGVR is the GroupVersionResource of a gardener shoot
var shootGVR = schema.GroupVersionResource{
	Group:    "core.gardener.cloud",
	Version:  "v1beta1",
	Resource: "shoots",
}

// ShootAccess defines methods used to fetch and access resources needed
// for creating ClusterSnapshot and ClusterScalingConstraints.
type ShootAccess interface {
	// ListNodes fetches the nodes on a shoot cluster matching the given criteria.
	ListNodes(ctx context.Context, criteria minkapi.MatchCriteria) ([]corev1.Node, error)
	// ListPods fetches the pods on a shoot cluster matching the given criteria.
	ListPods(ctx context.Context, criteria minkapi.MatchCriteria, excludeKubeSystemPods bool) ([]corev1.Pod, error)
	// ListPriorityClasses fetches all the priority classes present on a shoot cluster.
	ListPriorityClasses(ctx context.Context, excludeKubeSystemPods bool) ([]schedulingv1.PriorityClass, error)
	// ListRuntimeClasses fetches all the runtime classes present on a shoot cluster.
	ListRuntimeClasses(ctx context.Context) ([]nodev1.RuntimeClass, error)
	// GetCSIDriverToVolCount returns a map of CSI driver names to maximum number of
	// volumes managed by the driver on the nodes present on a shoot cluster.
	GetCSIDriverToVolCount(ctx context.Context) (map[string]int32, error)
	// GetShootWorker fetches the extension worker objects present in the shoot
	// namespace of the control (seed) cluster.
	GetShootWorker(ctx context.Context) (map[string]any, error)
}

var _ ShootAccess = (*access)(nil)

type access struct {
	seedClient      client.Client
	shootClient     client.Client
	scheme          *runtime.Scheme
	landscapeClient *dynamic.DynamicClient
	shootCoord      ShootCoordinate
}

func init() {
	genscenarioCmd.AddCommand(gardenerCmd)

	gardenerCmd.Flags().StringVarP(
		&shootCoords.Landscape,
		"landscape", "l",
		"",
		"gardener landscape name (required)",
	)
	_ = gardenerCmd.MarkFlagRequired("landscape")

	gardenerCmd.Flags().StringVarP(
		&shootCoords.Project,
		"project", "p",
		"",
		"gardener project name (required)",
	)
	_ = gardenerCmd.MarkFlagRequired("project")

	gardenerCmd.Flags().StringVarP(
		&shootCoords.Shoot,
		"shoot", "s",
		"",
		"gardener shoot name (required)",
	)
	_ = gardenerCmd.MarkFlagRequired("shoot")
	gardenerCmd.Flags().BoolVar(
		&excludeKubeSystemPods,
		"exclude-kube-system-pods",
		false,
		"exclude kube-system pods from the snapshot",
	)
}

// gardenerCmd represents the gardener sub-command of genscenario
// for generating scaling scenario(s) for a gardener cluster.
var gardenerCmd = &cobra.Command{
	Use:   "gardener <scenario-dir>",
	Short: "generate scaling data into <scenario-dir> for the gardener cluster manager (needs landscape oidc-kubeconfig to be present on the system)",
	PreRunE: func(_ *cobra.Command, _ []string) (err error) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		trimmedLandscape := strings.TrimPrefix(shootCoords.Landscape, "sap-landscape-")
		landscapeKubeconfigPath := path.Join(homeDir, ".garden", "landscapes", trimmedLandscape, "oidc-kubeconfig.yaml")
		_, err = os.Stat(landscapeKubeconfigPath)
		if err != nil {
			return fmt.Errorf("cannot find kubeconfig for landscape %q: %w", shootCoords.Landscape, err)
		}
		return
	},
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		// Create scaling scenario directory
		if len(args) == 0 {
			scenarioDir = "/tmp/" + shootCoords.getFullyQualifiedName()
		} else {
			scenarioDir = path.Join(args[0], shootCoords.getFullyQualifiedName())
		}
		fmt.Printf("Generating scaling data for shoot in %s\n", scenarioDir)
		err = os.MkdirAll(scenarioDir, 0750)
		if err != nil {
			return fmt.Errorf("error creating scenario directory: %v", err)
		}

		// Create shoot access with shoot and control plane clients
		ctx := cmd.Context()
		shootAccess, err := createShootAccess(ctx)
		if err != nil {
			return fmt.Errorf("error creating shoot access: %v", err)
		}

		// Generate cluster snapshot
		snap, err := createClusterSnapshot(ctx, shootAccess)
		if err != nil {
			return fmt.Errorf("error creating cluster snapshot: %v", err)
		}
		fmt.Printf("Created cluster snapshot with %d nodes and %d pods\n", len(snap.Nodes), len(snap.Pods))
		if err = genSnapshotVariants(snap, scenarioDir); err != nil {
			return fmt.Errorf("error creating snapshot variants: %v", err)
		}

		// Generate cluster scaling constraint
		extensionWorker, err := shootAccess.GetShootWorker(ctx)
		if err != nil {
			return fmt.Errorf("error getting shoot worker: %v", err)
		}

		csc, err := createScalingConstraint(extensionWorker)
		if err != nil {
			return fmt.Errorf("error creating cluster scaling constraint: %v", err)
		}
		clusterScalingConstraintFileName := path.Join(
			scenarioDir,
			"cluster-scaling-constraints-"+time.Now().UTC().Format("20060102T150405Z")+".json",
		)
		if err := saveDataToFile(csc, clusterScalingConstraintFileName); err != nil {
			return fmt.Errorf("error saving cluster scaling constraint: %v", err)
		}
		fmt.Printf("Created cluster scaling constraints at %s\n", clusterScalingConstraintFileName)
		return nil
	},
}

func (sc *ShootCoordinate) getFullyQualifiedName() string {
	trimmedLandscape := strings.TrimPrefix(sc.Landscape, "sap-landscape-")
	return fmt.Sprintf("%s:%s:%s", trimmedLandscape, sc.Project, sc.Shoot)
}

// ---------------------------------------------------------------------------------
// Shoot Access
// ---------------------------------------------------------------------------------

func createShootAccess(ctx context.Context) (*access, error) {
	clientScheme := typeinfo.SupportedScheme

	landscapeClient, err := createLandscapeDynamicClient(shootCoords)
	if err != nil {
		return nil, err
	}

	seedName, err := getSeedName(ctx, landscapeClient, shootCoords)
	if err != nil {
		return nil, err
	}
	seedCoords := ShootCoordinate{
		Landscape: strings.TrimPrefix(shootCoords.Landscape, "sap-landscape-"),
		Project:   "garden",
		Shoot:     seedName,
	}
	seedViewerKubeconfig, err := getViewerKubeconfig(ctx, landscapeClient, seedCoords)
	if err != nil {
		return nil, err
	}
	seedClient, err := getClient(seedViewerKubeconfig, clientScheme)
	if err != nil {
		return nil, err
	}

	targetShootCoords := ShootCoordinate{
		Landscape: strings.TrimPrefix(shootCoords.Landscape, "sap-landscape-"),
		Project:   "garden-" + shootCoords.Project,
		Shoot:     shootCoords.Shoot,
	}
	shootViewerKubeconfig, err := getViewerKubeconfig(ctx, landscapeClient, targetShootCoords)
	if err != nil {
		return nil, err
	}
	shootClient, err := getClient(shootViewerKubeconfig, clientScheme)
	if err != nil {
		return nil, err
	}

	return &access{
		shootCoord:      shootCoords,
		scheme:          clientScheme,
		landscapeClient: landscapeClient,
		seedClient:      seedClient,
		shootClient:     shootClient,
	}, nil
}

func createLandscapeDynamicClient(shootCoord ShootCoordinate) (*dynamic.DynamicClient, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	landscapeKubeconfigPath := path.Join(homeDir, ".garden", "landscapes", shootCoord.Landscape, "oidc-kubeconfig.yaml")

	restCfg, err := clientcmd.BuildConfigFromFlags("", landscapeKubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create rest.Config from kubeconfig %q: %w", landscapeKubeconfigPath, err)
	}

	return dynamic.NewForConfig(restCfg)
}

func getViewerKubeconfig(ctx context.Context, landscapeClient *dynamic.DynamicClient, shootCoords ShootCoordinate) (string, error) {
	expirationSecs := 600
	viewerKubeconfigObject := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "authentication.gardener.cloud/v1alpha1",
			"metadata": map[string]any{
				"name": shootCoords.Shoot,
			},
			"kind": "ViewerKubeconfigRequest",
			"spec": map[string]any{
				"expirationSeconds": expirationSecs,
			},
		},
	}

	result, err := landscapeClient.Resource(shootGVR).
		Namespace(shootCoords.Project).
		Create(ctx, &viewerKubeconfigObject, metav1.CreateOptions{}, "viewerkubeconfig")
	if err != nil {
		return "", fmt.Errorf("could not create viewerkubeconfig request: %w", err)
	}

	status, found, err := unstructured.NestedStringMap(result.Object, "status")
	if found {
		kubeconfigBytes, err := base64.StdEncoding.DecodeString(status["kubeconfig"])
		if err != nil {
			return "", fmt.Errorf("error decoding kubeconfig: %w", err)
		}

		kubeConfigFileName := fmt.Sprintf("%s_%s_%s_viewer-kubeconfig.yaml",
			shootCoords.Landscape, shootCoords.Project, shootCoords.Shoot,
		)
		kubeConfigPath := path.Join("/tmp/" + kubeConfigFileName)
		err = os.WriteFile(kubeConfigPath, kubeconfigBytes, 0600)
		if err != nil {
			return "", err
		}
		fmt.Printf("Saving shoot %q viewerkubeconfig at %q\n", shootCoords.Shoot, kubeConfigPath)
		return kubeConfigPath, nil
	}
	return "", fmt.Errorf("kubeconfig not found: %w", err)
}

func getSeedName(ctx context.Context, landscapeClient *dynamic.DynamicClient, shootCoord ShootCoordinate) (string, error) {
	shoot, err := landscapeClient.Resource(shootGVR).
		Namespace("garden-"+shootCoord.Project).
		Get(ctx, shootCoord.Shoot, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("error fetching the shoot object: %w", err)
	}

	shootSpec, found, err := unstructured.NestedMap(shoot.Object, "spec")
	if found {
		return shootSpec["seedName"].(string), nil
	}
	return "", fmt.Errorf("seedName not found: %w", err)
}

func getClient(kubeConfigPath string, scheme *runtime.Scheme) (client.Client, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create rest.Config from kubeconfig %q: %w", kubeConfigPath, err)
	}

	return client.New(restCfg, client.Options{Scheme: scheme})
}

// ---------------------------------------------------------------------------------
// Cluster Snapshot
// ---------------------------------------------------------------------------------

func (a *access) ListNodes(ctx context.Context, criteria minkapi.MatchCriteria) (nodes []corev1.Node, err error) {
	var nodeList corev1.NodeList
	err = a.shootClient.List(ctx, &nodeList, &client.ListOptions{
		LabelSelector: criteria.LabelSelector,
	})
	if err != nil {
		return nil, err
	}
	if criteria.Names.Len() <= 0 {
		return nodeList.Items, nil
	}
	for _, n := range nodeList.Items {
		if criteria.Names.Has(n.Name) {
			nodes = append(nodes, n)
		}
	}
	return nodes, err
}

func (a *access) ListPods(ctx context.Context, criteria minkapi.MatchCriteria, excludeKubeSystemPods bool) (pods []corev1.Pod, err error) {
	var podList corev1.PodList
	err = a.shootClient.List(ctx, &podList, &client.ListOptions{
		Namespace:     criteria.Namespace,
		LabelSelector: criteria.LabelSelector,
	})
	if err != nil {
		return nil, err
	}
	checkPodName := criteria.Names.Len() > 0
	for _, p := range podList.Items {
		// Filter pods having scheduling gates (check PodInfo docstring)
		if len(p.Spec.SchedulingGates) != 0 {
			continue
		}
		// pod name doesn't match the required names
		if checkPodName && !criteria.Names.Has(p.Name) {
			continue
		}

		if excludeKubeSystemPods && p.Namespace == metav1.NamespaceSystem {
			continue
		}

		pods = append(pods, p)
	}
	return pods, err
}

func (a *access) ListPriorityClasses(ctx context.Context, excludeKubeSystemPods bool) ([]schedulingv1.PriorityClass, error) {
	var priorityClassList schedulingv1.PriorityClassList
	err := a.shootClient.List(ctx, &priorityClassList)
	if err != nil {
		return nil, err
	}
	if excludeKubeSystemPods {
		priorityClassList.Items = slices.DeleteFunc(priorityClassList.Items,
			func(pc schedulingv1.PriorityClass) bool {
				return strings.HasPrefix(pc.Name, "gardener-shoot-system")
			},
		)
	}
	return priorityClassList.Items, nil
}

func (a *access) ListRuntimeClasses(ctx context.Context) ([]nodev1.RuntimeClass, error) {
	var runtimeClassList nodev1.RuntimeClassList
	err := a.shootClient.List(ctx, &runtimeClassList)
	return runtimeClassList.Items, err
}

func (a *access) GetCSIDriverToVolCount(ctx context.Context) (map[string]int32, error) {
	var csiNodeList storagev1.CSINodeList
	err := a.shootClient.List(ctx, &csiNodeList)
	if err != nil {
		return nil, fmt.Errorf("error listing CSI nodes: %v", err)
	}

	if len(csiNodeList.Items) == 0 {
		return nil, fmt.Errorf("no CSI nodes found")
	}

	volMap := make(map[string]int32)
	for _, csiNode := range csiNodeList.Items {
		for _, d := range csiNode.Spec.Drivers {
			if d.Allocatable != nil {
				allocatableSize := ptr.Deref(d.Allocatable.Count, 0)
				if _, present := volMap[d.Name]; present {
					volMap[d.Name] = max(volMap[d.Name], allocatableSize)
				} else {
					volMap[d.Name] = allocatableSize
				}
			}
		}
	}

	return volMap, nil
}

func createClusterSnapshot(ctx context.Context, a *access) (planner.ClusterSnapshot, error) {
	var snap planner.ClusterSnapshot

	nodes, err := a.ListNodes(ctx, minkapi.MatchCriteria{})
	if err != nil {
		return snap, fmt.Errorf("failed to list nodes: %w", err)
	}
	// Nodes are sorted in order of recently created first (used for creating snapshot variants)
	slices.SortFunc(nodes, func(nodeA, nodeB corev1.Node) int {
		return nodeB.CreationTimestamp.Compare(nodeA.CreationTimestamp.Time)
	})
	snap.Nodes = make([]planner.NodeInfo, 0, len(nodes))
	volMap, err := a.GetCSIDriverToVolCount(ctx)
	if err != nil {
		return snap, fmt.Errorf("failed to create volume map: %w", err)
	}
	pods, err := a.ListPods(ctx, minkapi.MatchCriteria{}, excludeKubeSystemPods)
	if err != nil {
		return snap, fmt.Errorf("failed to list pods: %w", err)
	}
	snap.Pods = make([]planner.PodInfo, 0, len(pods))
	for i, pod := range pods {
		sanitizePod(&pod, i)
		snap.Pods = append(snap.Pods, podutil.AsPodInfo(pod))
	}

	for _, node := range nodes {
		sanitizeNode(&node)
		snap.Nodes = append(snap.Nodes, nodeutil.AsNodeInfo(node, volMap))
	}

	snap.PriorityClasses, err = a.ListPriorityClasses(ctx, excludeKubeSystemPods)
	if err != nil {
		return snap, fmt.Errorf("failed to list priority classes: %w", err)
	}
	for i := range snap.PriorityClasses {
		sanitizePriorityClass(&snap.PriorityClasses[i])
	}

	snap.RuntimeClasses, err = a.ListRuntimeClasses(ctx)
	if err != nil {
		return snap, fmt.Errorf("failed to list runtime classes: %w", err)
	}

	err = obfuscateNodeNames(&snap)
	if err != nil {
		return snap, fmt.Errorf("failed to obfuscate node names: %w", err)
	}
	return snap, nil
}

// genSnapshotVariants takes the cluster snapshot and generates variants
// of the snapshot without a few of the most recent scaled nodes and
// unbinds the pods scheduled on those nodes. This is useful to compare
// the removed nodes with the nodes scaled up by autoscaling component.
func genSnapshotVariants(snap planner.ClusterSnapshot, dir string) error {
	formattedTime := time.Now().UTC().Format("20060102T150405Z")
	baseSnapshotFileName := path.Join(dir, "snapshot-"+formattedTime+"-baseline.json")
	if err := saveDataToFile(snap, baseSnapshotFileName); err != nil {
		return err
	}
	fmt.Printf("> Generated snapshot at %s\n", baseSnapshotFileName)

	for _, numNodesToRemove := range []int{1, 5, 10, 20} {
		numNodesToRemove = min(numNodesToRemove, len(snap.Nodes))
		countNodeRemovedSnapFileName := path.Join(
			dir, "snapshot-"+formattedTime+"-latest-"+strconv.Itoa(numNodesToRemove)+".json",
		)
		if _, err := os.Stat(countNodeRemovedSnapFileName); !os.IsNotExist(err) {
			continue // Snapshot already created
		}
		newSnap := removeNodesFromSnapshot(snap, numNodesToRemove)
		if err := saveDataToFile(newSnap, countNodeRemovedSnapFileName); err != nil {
			return err
		}
		fmt.Printf("> Generated variant at %s\n", countNodeRemovedSnapFileName)
	}
	return nil
}

func removeNodesFromSnapshot(snap planner.ClusterSnapshot, count int) planner.ClusterSnapshot {
	newSnap := snap

	newSnap.Nodes = snap.Nodes[count:]
	removedNodesName := make([]string, count)
	for i := range count {
		removedNodesName[i] = snap.Nodes[i].Name
	}

	for idx, p := range newSnap.Pods {
		if slices.Contains(removedNodesName, p.NodeName) {
			podPtr := &newSnap.Pods[idx]
			podPtr.NodeName = ""
		}
	}
	return newSnap
}

// ---------------------------------------------------------------------------------
// ScalingConstraint
// ---------------------------------------------------------------------------------

// GetShootWorker retrieves the shoot worker extension objects from control plane
func (a *access) GetShootWorker(ctx context.Context) (map[string]any, error) {
	var worker unstructured.Unstructured
	worker.SetAPIVersion("extensions.gardener.cloud/v1alpha1")
	worker.SetKind("Worker")

	key := client.ObjectKey{
		Name:      a.shootCoord.Shoot,
		Namespace: fmt.Sprintf("shoot--%s--%s", a.shootCoord.Project, a.shootCoord.Shoot),
	}

	if err := a.seedClient.Get(ctx, key, &worker); err != nil {
		return nil, fmt.Errorf("failed to get required Worker: %w", err)
	}

	return worker.Object, nil
}

func createScalingConstraint(extensionWorker map[string]any) (csc *apiv1alpha1.ScalingConstraint, err error) {
	nodePools, err := createNodePools(extensionWorker)
	if err != nil {
		err = fmt.Errorf("error creating node pools: %v", err)
		return
	}

	csc = &apiv1alpha1.ScalingConstraint{}
	csc.Spec.NodePools = nodePools
	// TODO csc.Spec.ConsumerID = "abcd", backoffpolicy, scaleinpolicy
	return
}

func createNodePools(worker map[string]any) ([]apiv1alpha1.NodePool, error) {
	var nodePools []apiv1alpha1.NodePool
	region, _, err := unstructured.NestedString(worker, "spec", "region")
	if err != nil {
		return nil, fmt.Errorf("worker is missing region: %v", err)
	}

	pools, _, err := unstructured.NestedSlice(worker, "spec", "pools")
	if err != nil {
		return nil, fmt.Errorf("worker is missing pools: %v", err)
	}

	for _, pool := range pools {
		var nodePool apiv1alpha1.NodePool
		poolObj, ok := pool.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("error getting pool object from the worker: %v", err)
		}

		nodePool.Name = poolObj["name"].(string)
		nodePool.Region = region
		if priority, _, err := unstructured.NestedInt64(poolObj, "priority"); err != nil {
			return nil, fmt.Errorf("error getting node pool priority: %v", err)
		} else {
			nodePool.Priority = int32(priority) // #nosec G115 -- priority cannot be greater than max int32.
		}
		if labels, found, err := unstructured.NestedStringMap(poolObj, "labels"); err != nil {
			return nil, fmt.Errorf("error getting node pool labels: %v", err)
		} else if found {
			nodePool.Labels = labels
		}
		if annotations, found, err := unstructured.NestedStringMap(poolObj, "annotations"); err != nil {
			return nil, fmt.Errorf("error getting node pool annotations: %v", err)
		} else if found {
			nodePool.Annotations = annotations
		}
		if taints, found, err := unstructured.NestedSlice(poolObj, "taints"); err != nil {
			return nil, fmt.Errorf("error getting node pool taints")
		} else if found {
			taintsJSON, err := json.Marshal(taints)
			if err != nil {
				return nil, fmt.Errorf("error getting the JSON encoding of taints slice: %v", err)
			}
			if err = json.Unmarshal(taintsJSON, &nodePool.Taints); err != nil {
				return nil, fmt.Errorf("error converting taints JSON to Taints object: %v", err)
			}
		}
		if availZones, _, err := unstructured.NestedStringSlice(poolObj, "zones"); err != nil {
			return nil, fmt.Errorf("error getting node pool availability zones: %v", err)
		} else {
			nodePool.AvailabilityZones = availZones
		}
		if nodeTemplate, err := constructNodeTemplate(poolObj, nodePool.Name, nodePool.Priority); err != nil {
			return nil, fmt.Errorf("error constructing the node template for %s: %v", nodePool.Name, err)
		} else {
			nodePool.NodeTemplates = append(nodePool.NodeTemplates, *nodeTemplate)
		}
		// TODO nP.Quota, nP.ScaleInPolicy, nP.BackoffPolicy

		nodePools = append(nodePools, nodePool)
	}

	return nodePools, nil
}

func constructNodeTemplate(pool map[string]any, name string, priority int32) (*apiv1alpha1.NodeTemplate, error) {
	var (
		capacity, kubeReserved map[string]any
		ok                     bool
	)
	if capacityObj, _, err := unstructured.NestedFieldCopy(pool, "nodeTemplate", "capacity"); err != nil {
		return nil, fmt.Errorf("error getting node template capacity: %v", err)
	} else {
		if capacity, ok = capacityObj.(map[string]any); !ok {
			return nil, fmt.Errorf("could not get capacity")
		}
	}
	if kubeReservedObj, found, err := unstructured.NestedFieldCopy(pool, "kubeletConfig", "kubeReserved"); err != nil {
		return nil, fmt.Errorf("error getting kubeletConfig kubeReserved: %v", err)
	} else if found {
		if kubeReserved, ok = kubeReservedObj.(map[string]any); !ok {
			return nil, fmt.Errorf("could not get kubeReserved")
		}
	}
	nodeTemplate := apiv1alpha1.NodeTemplate{
		Name:         name,
		Architecture: pool["architecture"].(string),
		InstanceType: pool["machineType"].(string),
		Priority:     priority,
		Capacity:     objutil.StringKeyValueMapToResourceList(capacity),
		KubeReserved: objutil.StringKeyValueMapToResourceList(kubeReserved),
		// SystemReserved is not part of gardener shoots from k8s v1.31, these reservations are part of KubeReserved
		// TODO:
		// MaxVolumes:     0,
	}
	return &nodeTemplate, nil
}

// ---------------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------------

func saveDataToFile(data any, path string) error {
	file, err := os.Create(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer ioutil.CloseQuietly(file)

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func obfuscateNodeNames(snap *planner.ClusterSnapshot) error {
	snapData, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("could not marshal snapshot: %w", err)
	}
	snapStr := string(snapData)

	for i, node := range snap.Nodes {
		originalName := node.Name
		newName := "node-" + strconv.Itoa(i)
		snapStr = strings.ReplaceAll(snapStr, originalName, newName)
	}
	err = json.Unmarshal([]byte(snapStr), snap)
	if err != nil {
		return fmt.Errorf("could not unmarshal snapshot string to snapshot object: %w", err)
	}

	return nil
}

func sanitizePod(pod *corev1.Pod, index int) {
	pod.Name = "pod-" + strconv.Itoa(index)
	maps.DeleteFunc(pod.Labels, sanitizeDeleteFunc)
	maps.DeleteFunc(pod.Annotations, sanitizeDeleteFunc)
	pod.ManagedFields = nil
	for i := range pod.Spec.Volumes {
		pod.Spec.Volumes[i].Projected = nil
	}
	pod.OwnerReferences = nil
}

func sanitizeNode(node *corev1.Node) {
	node.Namespace = ""
	maps.DeleteFunc(node.Labels, sanitizeDeleteFunc)
	maps.DeleteFunc(node.Annotations, sanitizeDeleteFunc)
	node.ManagedFields = nil
	requiredConditions := []corev1.NodeConditionType{
		corev1.NodeReady, corev1.NodeDiskPressure, corev1.NodePIDPressure, corev1.NodeMemoryPressure,
		// Node Problem Detector conditions
		"ReadonlyFilesystem", "KernelDeadlock",
	}
	node.Status.Conditions = slices.DeleteFunc(node.Status.Conditions,
		func(cond corev1.NodeCondition) bool {
			return !slices.Contains(requiredConditions, cond.Type)
		})
}

func sanitizePriorityClass(pc *schedulingv1.PriorityClass) {
	pc.ManagedFields = nil
	maps.DeleteFunc(pc.Labels, sanitizeDeleteFunc)
	maps.DeleteFunc(pc.Annotations, sanitizeDeleteFunc)
}

// TODO Can this removal cause issues with selectors?
func sanitizeDeleteFunc(k, _ string) bool {
	for _, prefix := range removePrefixes {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

var (
	removePrefixes = []string{
		"beta.", "failure-domain.beta.", "node.alpha.kubernetes.io", "checksum/",
		"node-agent.gardener.cloud", "worker.gardener.cloud/gardener-node-agent-secret-name",
		"resources.gardener.cloud", "shoot.gardener.cloud", "node.gardener.cloud/machine-name",
		"node.machine.sapcloud.io/last-applied-anno-labels-taints", "cni.",
		"controller-revision-hash", "gardener.cloud/role", "networking.gardener.cloud/", "node.gardener.cloud/critical-component",
		"pod-template-generation", "reference.resources.gardener.cloud/",
	}
)
