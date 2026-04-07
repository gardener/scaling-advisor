// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package genscenario

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	gardenerauthv1alpha1 "github.com/gardener/gardener/pkg/apis/authentication/v1alpha1"
	gardenercorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardenerconstantsv1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	gardenerextensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
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
	shootCoords             ShootCoordinate
	scenarioDir             string
	excludeSystemComponents bool
	obfuscateData           bool
)

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
	// GetCSINodeSpecs returns a map of node name to CSINodeSpec if a CSINode was present in the shoot cluster.
	GetCSINodeSpecs(ctx context.Context) (map[string]storagev1.CSINodeSpec, error)
}

var _ ShootAccess = (*access)(nil)

type access struct {
	shoot       gardenercorev1beta1.Shoot
	shootClient client.Client
	scheme      *runtime.Scheme
	instances   map[string]corev1.ResourceList
	shootCoord  ShootCoordinate
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
		&excludeSystemComponents,
		"exclude-system-components",
		false,
		"exclude system components (pods and priority classes) from the snapshot",
	)

	gardenerCmd.Flags().BoolVar(
		&obfuscateData,
		"obfuscate-data",
		false,
		"sanitize and obfuscate cluster sensitive data",
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

		// Create shoot access with shoot client
		ctx := cmd.Context()
		shootAccess, err := createShootAccess(ctx)
		if err != nil {
			return fmt.Errorf("error creating shoot access: %v", err)
		}

		scalingConstraint := shootAccess.createScalingConstraint()

		snap, err := createClusterSnapshot(ctx, scalingConstraint, shootAccess)
		if err != nil {
			return fmt.Errorf("error creating cluster snapshot: %v", err)
		}
		fmt.Printf("Created cluster snapshot with %d nodes and %d pods\n", len(snap.Nodes), len(snap.Pods))
		if err = genSnapshotVariants(snap, scenarioDir); err != nil {
			return fmt.Errorf("error creating snapshot variants: %v", err)
		}
		saveFilename := "scaling-constraints-" + time.Now().UTC().Format("20060102T150405Z") + ".json"
		scalingConstraintSavePath, err := objutil.SaveRuntimeObjAsJSONToPath(scalingConstraint, scenarioDir, saveFilename)
		if err != nil {
			return fmt.Errorf("cannot save scaling constraint at %q: %v", scalingConstraintSavePath, err)
		}
		fmt.Printf("Saved scaling constraints at %s\n", scalingConstraintSavePath)
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
	clientScheme := objutil.ScalingAdvisorScheme
	err := registerSchemes(clientScheme)
	if err != nil {
		return nil, err
	}
	landscapeClient, err := createLandscapeClient(shootCoords, clientScheme)
	if err != nil {
		return nil, err
	}
	shoot, err := getShootObject(ctx, landscapeClient, shootCoords)
	if err != nil {
		return nil, err
	}
	if shoot.Spec.CloudProfile == nil {
		return nil, fmt.Errorf("no cloudprofile associated with the shoot")
	}
	instanceTypes, err := getCloudProfileMachineTypes(ctx, landscapeClient, shoot.Spec.CloudProfile.Name)
	if err != nil {
		return nil, err
	}
	instancesMap := constructInstanceRequirementsMap(instanceTypes)

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
		shootCoord:  shootCoords,
		scheme:      clientScheme,
		shootClient: shootClient,
		shoot:       shoot,
		instances:   instancesMap,
	}, nil
}

func registerSchemes(clientScheme *runtime.Scheme) error {
	if err := k8sscheme.AddToScheme(clientScheme); err != nil {
		return fmt.Errorf("failed to add kubernetes scheme: %w", err)
	}
	if err := gardenercorev1beta1.AddToScheme(clientScheme); err != nil {
		return fmt.Errorf("failed to add gardener core scheme: %w", err)
	}
	if err := gardenerauthv1alpha1.AddToScheme(clientScheme); err != nil {
		return fmt.Errorf("failed to add gardener auth scheme: %w", err)
	}
	if err := gardenerextensionsv1alpha1.AddToScheme(clientScheme); err != nil {
		return fmt.Errorf("failed to add gardener extensions scheme: %w", err)
	}
	return nil
}

func createLandscapeClient(shootCoord ShootCoordinate, scheme *runtime.Scheme) (client.Client, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	landscapeKubeconfigPath := path.Join(homeDir, ".garden", "landscapes", shootCoord.Landscape, "oidc-kubeconfig.yaml")

	restCfg, err := clientcmd.BuildConfigFromFlags("", landscapeKubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create rest.Config from kubeconfig %q: %w", landscapeKubeconfigPath, err)
	}

	return client.New(restCfg, client.Options{Scheme: scheme})
}

func getViewerKubeconfig(ctx context.Context, landscapeClient client.Client, shootCoords ShootCoordinate) (string, error) {
	shoot := &gardenercorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shootCoords.Shoot,
			Namespace: shootCoords.Project,
		},
	}
	viewerKcfgReq := &gardenerauthv1alpha1.ViewerKubeconfigRequest{
		Spec: gardenerauthv1alpha1.ViewerKubeconfigRequestSpec{
			ExpirationSeconds: ptr.To[int64](600),
		},
	}

	if err := landscapeClient.SubResource("viewerkubeconfig").Create(ctx, shoot, viewerKcfgReq); err != nil {
		return "", fmt.Errorf("could not create viewerkubeconfig request: %w", err)
	}

	kubeconfigBytes := viewerKcfgReq.Status.Kubeconfig
	if len(kubeconfigBytes) == 0 {
		return "", fmt.Errorf("kubeconfig not found in ViewerKubeconfigRequest status")
	}

	kubeConfigFileName := fmt.Sprintf("%s_%s_%s_viewer-kubeconfig.yaml",
		shootCoords.Landscape, shootCoords.Project, shootCoords.Shoot,
	)
	kubeConfigPath := path.Join("/tmp/" + kubeConfigFileName)
	if err := os.WriteFile(kubeConfigPath, kubeconfigBytes, 0600); err != nil {
		return "", err
	}
	fmt.Printf("Saving shoot %q viewerkubeconfig at %q\n", shootCoords.Shoot, kubeConfigPath)
	return kubeConfigPath, nil
}

func getShootObject(ctx context.Context, landscapeClient client.Client, shootCoord ShootCoordinate) (shoot gardenercorev1beta1.Shoot, err error) {
	key := client.ObjectKey{
		Namespace: "garden-" + shootCoord.Project,
		Name:      shootCoord.Shoot,
	}
	if err = landscapeClient.Get(ctx, key, &shoot); err != nil {
		err = fmt.Errorf("error fetching the shoot object: %w", err)
		return
	}

	return shoot, nil
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

func (a *access) GetCSINodeSpecs(ctx context.Context) (map[string]storagev1.CSINodeSpec, error) {
	var csiNodeList storagev1.CSINodeList
	err := a.shootClient.List(ctx, &csiNodeList)
	if err != nil {
		return nil, fmt.Errorf("error listing CSI nodes: %v", err)
	}

	if len(csiNodeList.Items) == 0 {
		return nil, fmt.Errorf("no CSI nodes found")
	}

	csiNodeSpecs := make(map[string]storagev1.CSINodeSpec)
	for _, csiNode := range csiNodeList.Items {
		csiNodeSpecs[csiNode.Name] = csiNode.Spec
	}
	return csiNodeSpecs, nil
}

func createClusterSnapshot(ctx context.Context, sc *sacorev1alpha1.ScalingConstraint, a *access) (planner.ClusterSnapshot, error) {
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
	csiNodeSpecs, err := a.GetCSINodeSpecs(ctx)
	if err != nil {
		return snap, fmt.Errorf("failed to obtain csiNodeSpecs: %w", err)
	}
	pods, err := a.ListPods(ctx, minkapi.MatchCriteria{}, excludeSystemComponents)
	if err != nil {
		return snap, fmt.Errorf("failed to list pods: %w", err)
	}
	snap.Pods = make([]planner.PodInfo, 0, len(pods))
	for _, pod := range pods {
		sanitizePod(&pod)
		snap.Pods = append(snap.Pods, podutil.AsPodInfo(pod))
	}

	for _, node := range nodes {
		poolName := node.Labels[gardenerconstantsv1beta1.LabelWorkerPool]
		instanceType := node.Labels[corev1.LabelInstanceTypeStable]
		var matchingNodeTemplateName string
		for _, p := range sc.Spec.NodePools {
			if p.Name == poolName {
				for _, nt := range p.NodeTemplates {
					if nt.InstanceType == instanceType {
						matchingNodeTemplateName = nt.Name
					}
				}
			}
		}
		if matchingNodeTemplateName == "" {
			return snap, fmt.Errorf("failed to find matching node template for pool %q and instance-type %q", poolName, instanceType)
		}
		sanitizeNode(&node)
		ni := nodeutil.AsNodeInfo(node)
		if cns, ok := csiNodeSpecs[ni.Name]; ok {
			ni.CSINodeSpec = &cns
		}
		ni.Labels[commonconstants.LabelNodePoolName] = poolName
		ni.Labels[corev1.LabelInstanceTypeStable] = instanceType
		ni.Labels[commonconstants.LabelNodeTemplateName] = matchingNodeTemplateName
		if err = ni.ValidateLabels(); err != nil {
			return snap, err
		}
		snap.Nodes = append(snap.Nodes, ni)
	}

	snap.PriorityClasses, err = a.ListPriorityClasses(ctx, excludeSystemComponents)
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

	if obfuscateData {
		fmt.Println("Obfuscating snapshot data!!")
		obfuscateMetadata(&snap)
	}
	return snap, nil
}

// genSnapshotVariants takes the cluster snapshot and generates variants
// of the snapshot without a few of the most recent scaled nodes and
// unbinds the pods scheduled on those nodes. This is useful to compare
// the removed nodes with the nodes scaled up by autoscaling component.
func genSnapshotVariants(snap planner.ClusterSnapshot, dir string) error {
	formattedTime := time.Now().UTC().Format("20060102T150405Z")
	baseSnapshotFileName := path.Join(dir, "cluster-snapshot-"+formattedTime+"-baseline.json")
	if err := saveDataToFile(snap, baseSnapshotFileName); err != nil {
		return err
	}
	fmt.Printf("> Generated snapshot at %s\n", baseSnapshotFileName)

	for _, numNodesToRemove := range []int{1, 5, 10, 20} {
		numNodesToRemove = min(numNodesToRemove, len(snap.Nodes))
		countNodeRemovedSnapFileName := path.Join(
			dir, "cluster-snapshot-"+formattedTime+"-latest-"+strconv.Itoa(numNodesToRemove)+".json",
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

// getCloudProfileMachineTypes retreives the CloudProfile MachineTypes for the specified landscape
// This is required for getting the instance capacities
func getCloudProfileMachineTypes(ctx context.Context, landscapeClient client.Client, cloudProfileName string) ([]gardenercorev1beta1.MachineType, error) {
	var cloudProfile gardenercorev1beta1.CloudProfile
	key := client.ObjectKey{
		Name:      cloudProfileName,
		Namespace: "garden",
	}

	if err := landscapeClient.Get(ctx, key, &cloudProfile); err != nil {
		return nil, fmt.Errorf("failed to get required cloudProfile: %w", err)
	}

	return cloudProfile.Spec.MachineTypes, nil
}

func constructInstanceRequirementsMap(instances []gardenercorev1beta1.MachineType) (instanceMap map[string]corev1.ResourceList) {
	instanceMap = make(map[string]corev1.ResourceList, len(instances))
	for _, instance := range instances {
		resources := corev1.ResourceList{}
		resources[corev1.ResourceCPU] = instance.CPU
		resources["gpu"] = instance.GPU // TODO check the key
		resources[corev1.ResourceMemory] = instance.Memory

		instanceMap[instance.Name] = resources
	}
	return
}

func (a *access) createScalingConstraint() (csc *sacorev1alpha1.ScalingConstraint) {
	nodePools := a.createNodePools()
	csc = &sacorev1alpha1.ScalingConstraint{}
	csc.Spec.NodePools = nodePools
	// TODO csc.Spec.ConsumerID = "abcd", backoffpolicy, scaleinpolicy
	return
}

func (a *access) createNodePools() (nodePools []sacorev1alpha1.NodePool) {
	region := a.shoot.Spec.Region

	for _, worker := range a.shoot.Spec.Provider.Workers {
		var nodePool sacorev1alpha1.NodePool

		nodePool.Name = worker.Name
		nodePool.Region = region
		if worker.Priority != nil {
			nodePool.Priority = *worker.Priority
		}
		if len(worker.Labels) > 0 {
			nodePool.Labels = maps.Clone(worker.Labels)
			maps.DeleteFunc(nodePool.Labels, sanitizeDeleteFunc)
		}
		if len(worker.Annotations) > 0 {
			nodePool.Annotations = maps.Clone(worker.Annotations)
		}
		if len(worker.Taints) > 0 {
			nodePool.Taints = worker.Taints
		}
		nodePool.AvailabilityZones = worker.Zones

		nodeTemplate := a.constructNodeTemplate(worker)
		nodePool.NodeTemplates = append(nodePool.NodeTemplates, nodeTemplate)
		// TODO nP.Quota, nP.ScaleInPolicy, nP.BackoffPolicy

		nodePools = append(nodePools, nodePool)
	}
	return
}

func (a *access) constructNodeTemplate(worker gardenercorev1beta1.Worker) sacorev1alpha1.NodeTemplate {
	return sacorev1alpha1.NodeTemplate{
		Name:         worker.Name,
		Architecture: ptr.Deref(worker.Machine.Architecture, ""),
		InstanceType: worker.Machine.Type,
		Priority:     ptr.Deref(worker.Priority, 0),
		// TODO: add pool.NodeTemplate.VirtualCapacity
		Capacity:     a.instances[worker.Machine.Type],
		KubeReserved: kubernetesConfigToResourceList(a.shoot.Spec.Kubernetes),
		// SystemReserved is not part of gardener shoots from k8s v1.31, these reservations are part of KubeReserved
		// TODO:
		// MaxVolumes:     0,
	}
}

func kubernetesConfigToResourceList(kubernetesConfig gardenercorev1beta1.Kubernetes) corev1.ResourceList {
	kubeletConfig := kubernetesConfig.Kubelet

	if kubeletConfig == nil || kubeletConfig.KubeReserved == nil {
		return nil
	}
	reserved := kubeletConfig.KubeReserved

	result := make(corev1.ResourceList)
	if reserved.CPU != nil {
		result[corev1.ResourceCPU] = *reserved.CPU
	}
	if reserved.Memory != nil {
		result[corev1.ResourceMemory] = *reserved.Memory
	}
	if kubeletConfig.MaxPods != nil {
		result[corev1.ResourcePods] = *resource.NewQuantity(
			int64(*kubeletConfig.MaxPods), resource.DecimalSI,
		)
	}
	return result
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

	encoder := stdjson.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func sanitizePod(pod *corev1.Pod) {
	pod.ManagedFields = nil
	pod.ResourceVersion = ""
	for i := range pod.Spec.Volumes {
		pod.Spec.Volumes[i].Projected = nil
	}
}

func sanitizeNode(node *corev1.Node) {
	node.Namespace = ""
	node.ManagedFields = nil
	node.ResourceVersion = ""
	requiredConditions := []corev1.NodeConditionType{
		corev1.NodeReady, // only preserve ready conditions, memory-pressure/disk-pressure/etc are now in taints
	}
	node.Status.Conditions = slices.DeleteFunc(node.Status.Conditions,
		func(cond corev1.NodeCondition) bool {
			return !slices.Contains(requiredConditions, cond.Type)
		})
}

func sanitizePriorityClass(pc *schedulingv1.PriorityClass) {
	pc.ManagedFields = nil
	pc.ResourceVersion = ""
}
