// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	apiv1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ShootCoordinate struct {
	Landscape string
	Project   string
	Shoot     string
}

var scenarioDir string

var shootCoords ShootCoordinate

type GardenerPlane int

const (
	DataPlane          GardenerPlane = 0
	ControlPlane       GardenerPlane = 1
	VirtualGardenPlane GardenerPlane = 2
)

type ShootAccess interface {
	ListNodes(ctx context.Context, criteria mkapi.MatchCriteria) ([]corev1.Node, error)
	ListPods(ctx context.Context, criteria mkapi.MatchCriteria) ([]corev1.Pod, error)
	ListPriorityClasses(ctx context.Context) ([]schedulingv1.PriorityClass, error)
	ListRuntimeClasses(ctx context.Context) ([]nodev1.RuntimeClass, error)
	MakeCSIDriverVolumeMap(ctx context.Context) (map[string]int32, error)
	GetShootWorker(ctx context.Context) (map[string]any, error)
}

var _ ShootAccess = (*access)(nil)

type access struct {
	shootCoord    ShootCoordinate
	scheme        *runtime.Scheme
	shootClient   client.Client
	controlClient client.Client
}

// (TODO) look later into incorporating scalebench with this
type ScalingScenario struct {
	constraintsPath string
	snapshotsPath   []string
	feedback        apiv1alpha1.ClusterScalingFeedback
}

// gardenerCmd represents the gardener sub-command for generating scaling scenario(s) for a gardener cluster.
var gardenerCmd = &cobra.Command{
	Use:   "gardener <scenario-dir>",
	Short: "generate scaling scenarios into <scenario-dir> for the gardener cluster manager",
	Run: func(cmd *cobra.Command, args []string) {
		// Create scaling scenario directory
		fullyQualName := constructFullyQualifiedName(shootCoords)
		if len(args) == 0 {
			scenarioDir = "/tmp/" + fullyQualName
		} else {
			scenarioDir = path.Join(args[0], fullyQualName)
		}
		fmt.Printf("Generating scaling scenarios for shoot in %s\n", scenarioDir)
		err := os.MkdirAll(scenarioDir, 0755)
		if err != nil {
			fmt.Printf("Error creating scenario directory: %v\n", err)
			return
		}

		// Create shoot access with shoot and control plane clients
		ctx := context.Background()
		acc, err := createShootAccess(ctx)
		if err != nil {
			fmt.Printf("Error creating shoot access: %v\n", err)
			os.Exit(1)
		}

		// Generate cluster snapshot
		snap, err := createClusterSnapshot(ctx, acc)
		if err != nil {
			fmt.Printf("Error creating cluster snapshot: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Created cluster snapshot with %d nodes and %d pods\n", len(snap.Nodes), len(snap.Pods))
		if err = genSnapshotVariants(snap, scenarioDir); err != nil {
			fmt.Printf("Error creating snapshot variants: %v\n", err)
			os.Exit(1)
		}

		// Generate cluster scaling constraint
		extensionWorker, err := acc.GetShootWorker(ctx)
		if err != nil {
			fmt.Printf("Error getting shoot worker: %v\n", err)
			os.Exit(1)
		}

		csc, err := createScalingConstraint(extensionWorker)
		if err != nil {
			fmt.Printf("Error creating cluster scaling constraint: %v\n", err)
			os.Exit(1)
		}
		if err := saveDataToFile(csc, path.Join(scenarioDir, "cluster-scaling-constraints.json")); err != nil {
			fmt.Printf("Error saving cluster scaling constraint: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Created cluster scaling constraints")
	},
}

func init() {
	genscenarioCmd.AddCommand(gardenerCmd)

	gardenerCmd.Flags().StringVarP(
		&shootCoords.Landscape,
		"landscape", "l",
		"",
		"gardener landscape name (required)",
	)
	gardenerCmd.MarkFlagRequired("landscape")

	gardenerCmd.Flags().StringVarP(
		&shootCoords.Project,
		"project", "p",
		"",
		"gardener project name (required)",
	)
	gardenerCmd.MarkFlagRequired("project")

	gardenerCmd.Flags().StringVarP(
		&shootCoords.Shoot,
		"shoot", "s",
		"",
		"gardener shoot name (required)",
	)
	gardenerCmd.MarkFlagRequired("shoot")
}

func constructFullyQualifiedName(shootCoords ShootCoordinate) string {
	trimmedLandscape := strings.TrimPrefix(shootCoords.Landscape, "sap-landscape-")
	return fmt.Sprintf("%s:%s:%s", trimmedLandscape, shootCoords.Project, shootCoords.Shoot)
}

// ---------------------------------------------------------------------------------
// Shoot Access
// ---------------------------------------------------------------------------------
func createShootAccess(ctx context.Context) (*access, error) {
	clientScheme := registerSchemes()

	shootClient, err := getClient(ctx, shootCoords, clientScheme, DataPlane)
	if err != nil {
		return nil, err
	}

	controlClient, err := getClient(ctx, shootCoords, clientScheme, ControlPlane)
	if err != nil {
		return nil, err
	}

	return &access{
		shootCoord:    shootCoords,
		scheme:        clientScheme,
		shootClient:   shootClient,
		controlClient: controlClient,
	}, nil
}

func registerSchemes() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(nodev1.AddToScheme(scheme))
	utilruntime.Must(schedulingv1.AddToScheme(scheme))
	utilruntime.Must(storagev1.AddToScheme(scheme))
	return scheme
}

func getClient(ctx context.Context, shootCoord ShootCoordinate, scheme *runtime.Scheme, plane GardenerPlane) (client.Client, error) {
	var targetStr string

	switch plane {
	case VirtualGardenPlane:
		targetStr = fmt.Sprintf("gardenctl target --garden %s", shootCoord.Landscape)
	case DataPlane:
		targetStr = fmt.Sprintf("gardenctl target --garden %s --project %s --shoot %s",
			shootCoord.Landscape, shootCoord.Project, shootCoord.Shoot)
	case ControlPlane:
		targetStr = fmt.Sprintf("gardenctl target --garden %s --project %s --shoot %s --control-plane",
			shootCoord.Landscape, shootCoord.Project, shootCoord.Shoot)
	default:
		return nil, fmt.Errorf("unsupported gardener plane: %d", plane)
	}

	cmdStr := fmt.Sprintf("eval $(gardenctl kubectl-env bash) && %s > /dev/null && gardenctl kubectl-env bash", targetStr)
	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	cmd.Env = append(os.Environ(), "GCTL_SESSION_ID=dev")

	capturedOut, err := invokeCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to execute gardenctl command: %w", err)
	}

	kubeConfigPath, err := extractKubeConfigPath(capturedOut)
	if err != nil {
		return nil, err
	}

	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create rest.Config from kubeconfig %q: %w", kubeConfigPath, err)
	}

	return client.New(restCfg, client.Options{Scheme: scheme})
}

// ---------------------------------------------------------------------------------
// Cluster Snapshot
// ---------------------------------------------------------------------------------
func (a *access) ListNodes(ctx context.Context, criteria mkapi.MatchCriteria) (nodes []corev1.Node, err error) {
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

func (a *access) ListPods(ctx context.Context, criteria mkapi.MatchCriteria) (pods []corev1.Pod, err error) {
	var podList corev1.PodList
	err = a.shootClient.List(ctx, &podList, &client.ListOptions{
		Namespace:     criteria.Namespace,
		LabelSelector: criteria.LabelSelector,
	})
	if err != nil {
		return nil, err
	}
	if criteria.Names.Len() <= 0 {
		return podList.Items, nil
	}
	for _, p := range podList.Items {
		// TODO Check if needed: filter pods having scheduling gates
		if criteria.Names.Has(p.Name) && len(p.Spec.SchedulingGates) != 0 {
			pods = append(pods, p)
		}
	}
	return pods, err
}

func (a *access) ListPriorityClasses(ctx context.Context) ([]schedulingv1.PriorityClass, error) {
	var priorityClassList schedulingv1.PriorityClassList
	err := a.shootClient.List(ctx, &priorityClassList)
	return priorityClassList.Items, err
}

func (a *access) ListRuntimeClasses(ctx context.Context) ([]nodev1.RuntimeClass, error) {
	var runtimeClassList nodev1.RuntimeClassList
	err := a.shootClient.List(ctx, &runtimeClassList)
	return runtimeClassList.Items, err
}

func (a *access) MakeCSIDriverVolumeMap(ctx context.Context) (map[string]int32, error) {
	var csiNodeList storagev1.CSINodeList
	err := a.shootClient.List(ctx, &csiNodeList)
	if len(csiNodeList.Items) == 0 {
		return nil, err
	}

	volMap := make(map[string]int32, 0)
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

// TODO: Add name obfuscation logic toggled via a flag maybe
func createClusterSnapshot(ctx context.Context, a *access) (svcapi.ClusterSnapshot, error) {
	var snap svcapi.ClusterSnapshot

	nodes, err := a.ListNodes(ctx, mkapi.MatchCriteria{})
	if err != nil {
		return snap, fmt.Errorf("failed to list nodes: %w", err)
	}
	// Nodes are sorted in order of recently created first (used for creating snapshot variants)
	slices.SortFunc(nodes, func(nodeA, nodeB corev1.Node) int {
		return nodeB.CreationTimestamp.Compare(nodeA.CreationTimestamp.Time)
	})
	snap.Nodes = make([]svcapi.NodeInfo, 0, len(nodes))
	volMap, err := a.MakeCSIDriverVolumeMap(ctx)
	if err != nil {
		return snap, fmt.Errorf("failed to create volume map: %w", err)
	}
	for _, node := range nodes {
		snap.Nodes = append(snap.Nodes, nodeutil.AsNodeInfo(node, volMap))
	}

	pods, err := a.ListPods(ctx, mkapi.MatchCriteria{})
	if err != nil {
		return snap, fmt.Errorf("failed to list pods: %w", err)
	}
	snap.Pods = make([]svcapi.PodInfo, 0, len(pods))
	for _, pod := range pods {
		snap.Pods = append(snap.Pods, podutil.AsPodInfo(pod))
	}

	// TODO: consider removing managedFields from PC
	snap.PriorityClasses, err = a.ListPriorityClasses(ctx)
	if err != nil {
		return snap, fmt.Errorf("failed to list priority classes: %w", err)
	}

	snap.RuntimeClasses, err = a.ListRuntimeClasses(ctx)
	if err != nil {
		return snap, fmt.Errorf("failed to list runtime classes: %w", err)
	}

	return snap, nil
}

func genSnapshotVariants(snap svcapi.ClusterSnapshot, dir string) error {
	formattedTime := time.Now().UTC().Format("020106-1504") // DDMMYY-HHMM

	incrementalSnapshotFileName := path.Join(dir, "snapshot-"+formattedTime+"-incremental.json")
	if err := saveDataToFile(snap, incrementalSnapshotFileName); err != nil {
		return err
	}
	fmt.Printf("> Generated snapshot at %s\n", incrementalSnapshotFileName)

	for _, count := range []int{1, 5, 10, 20} {
		count = min(count, len(snap.Nodes))
		countNodeRemovedSnapFileName := path.Join(
			dir, "snapshot-"+formattedTime+"-latest-"+strconv.Itoa(count)+".json",
		)
		if _, err := os.Stat(countNodeRemovedSnapFileName); !os.IsNotExist(err) {
			continue // Snapshot already created
		}
		newSnap := removeNodesFromSnapshot(snap, count)
		if err := saveDataToFile(newSnap, countNodeRemovedSnapFileName); err != nil {
			return err
		}
		fmt.Printf("> Generated variant at %s\n", countNodeRemovedSnapFileName)
	}
	return nil
}

func removeNodesFromSnapshot(snap svcapi.ClusterSnapshot, count int) svcapi.ClusterSnapshot {
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
// Get Worker extension objects from control plane
func (a *access) GetShootWorker(ctx context.Context) (map[string]any, error) {
	var worker unstructured.Unstructured
	worker.SetAPIVersion("extensions.gardener.cloud/v1alpha1")
	worker.SetKind("Worker")

	key := client.ObjectKey{
		Name:      a.shootCoord.Shoot,
		Namespace: fmt.Sprintf("shoot--%s--%s", a.shootCoord.Project, a.shootCoord.Shoot),
	}

	if err := a.controlClient.Get(ctx, key, &worker); err != nil {
		return nil, fmt.Errorf("failed to get required Worker: %w", err)
	}

	return worker.Object, nil
}

func createScalingConstraint(extensionWorker map[string]any) (csc apiv1alpha1.ClusterScalingConstraint, err error) {
	csc.Spec.NodePools, err = createNodePools(extensionWorker)
	if err != nil {
		return
	}
	// TODO csc.Spec.ConsumerID = "abcd", backoffpolicy, scaleinpolicy
	csc.Spec.AdviceGenerationMode = apiv1alpha1.ScalingAdviceGenerationModeAllAtOnce // FIXME hardcoded for now
	return
}

func createNodePools(worker map[string]any) (nodePools []apiv1alpha1.NodePool, err error) {
	region, found, err := unstructured.NestedString(worker, "spec", "region")
	if !found || err != nil {
		return nil, fmt.Errorf("worker is missing region: %v", err)
	}

	pools, found, err := unstructured.NestedSlice(worker, "spec", "pools")
	if !found || err != nil {
		return nil, fmt.Errorf("worker is missing pools: %v", err)
	}

	for _, poolInterface := range pools {
		var nP apiv1alpha1.NodePool
		pool, ok := poolInterface.(map[string]any)
		if !ok {
			continue
		}

		nP.Name = pool["name"].(string)
		nP.Region = region
		if priority, found, _ := unstructured.NestedInt64(pool, "priority"); found {
			nP.Priority = int32(priority)
		}
		if labels, found, _ := unstructured.NestedStringMap(pool, "labels"); found {
			nP.Labels = labels
		}
		if annotations, found, _ := unstructured.NestedStringMap(pool, "annotations"); found {
			nP.Annotations = annotations
		}
		if taints, found, _ := unstructured.NestedSlice(pool, "taints"); found {
			taintsJSON, err := json.Marshal(taints)
			if err != nil {
				continue
			}
			if err = json.Unmarshal(taintsJSON, &nP.Taints); err != nil {
				continue
			}
		}
		nP.AvailabilityZones, _, _ = unstructured.NestedStringSlice(pool, "zones")
		nP.NodeTemplates, err = constructNodeTemplates(pool, nP.Name, nP.Priority)
		if err != nil {
			continue
		}
		// TODO nP.Quota, nP.ScaleInPolicy, nP.BackoffPolicy

		nodePools = append(nodePools, nP)
	}

	return nodePools, nil
}

func constructNodeTemplates(pool map[string]any, name string, priority int32) ([]apiv1alpha1.NodeTemplate, error) {
	var nodeTemplates []apiv1alpha1.NodeTemplate
	var cap, kr map[string]any
	var ok bool
	if capacity, found, _ := unstructured.NestedFieldCopy(pool, "nodeTemplate", "capacity"); found {
		if cap, ok = capacity.(map[string]any); !ok {
			return nil, fmt.Errorf("could not get capacity")
		}
	}
	if kubeReserved, found, _ := unstructured.NestedFieldNoCopy(pool, "kubeletConfig", "kubeReserved"); found {
		if kr, ok = kubeReserved.(map[string]any); !ok {
			return nil, fmt.Errorf("could not get kubeReserved")
		}
	}

	nT := apiv1alpha1.NodeTemplate{
		Name:         name,
		Architecture: pool["architecture"].(string),
		InstanceType: pool["machineType"].(string),
		Priority:     priority,
		Capacity:     objutil.StringMapToResourceList(cap),
		KubeReserved: objutil.StringMapToResourceList(kr),
		// TODO:
		// SystemReserved: corev1.ResourceList{},
		// MaxVolumes:     0,
	}
	nodeTemplates = append(nodeTemplates, nT)
	return nodeTemplates, nil
}

// ---------------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------------
func extractKubeConfigPath(output string) (string, error) {
	kubeConfigRe := regexp.MustCompile(`export KUBECONFIG='([^']+)'`)
	matches := kubeConfigRe.FindStringSubmatch(output)
	if len(matches) <= 1 {
		return "", fmt.Errorf("cannot extract kubeconfig path from gardenctl output: %s", output)
	}
	return matches[1], nil
}

// TODO: Move to commons/toolutil.go
func invokeCommand(cmd *exec.Cmd) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// fmt.Printf("Executing command: %s", cmd.String())
	if err := cmd.Run(); err != nil {
		capturedError := strings.TrimSpace(stderr.String())
		if capturedError != "" {
			return "", fmt.Errorf("command failed: %s (stderr: %s)", err, capturedError)
		}
		return "", fmt.Errorf("command failed: %w", err)
	}

	return stdout.String(), nil
}

func saveDataToFile(data any, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
