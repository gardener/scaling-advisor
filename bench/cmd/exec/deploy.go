// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"fmt"
	"log"

	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"

	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	dummyContainers = []corev1.Container{
		{
			Name:  "dummy-container",
			Image: "dummy-image",
		},
	}
	dummySelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app.kubernetes.io/name": "dummy",
		},
	}
)

type ownerMeta struct {
	Name      string
	Namespace string
	UID       apitypes.UID
}

// deployObjects creates the non-pod/node Kubernetes objects that form the
// cluster state: priority classes, defaultNamespaces' service account,
// and storage classes, volumes and claims
func deployObjects(
	ctx context.Context,
	cfg *envconf.Config,
	clusterSnapshot planner.ClusterSnapshot,
) error {
	err := deployPriorityClasses(ctx, clusterSnapshot, cfg)
	if err != nil {
		return err
	}
	defaultNamespaces := []string{
		corev1.NamespaceDefault,
		"kube-system",
		"kube-public",
		corev1.NamespaceNodeLease,
	}
	for _, ns := range defaultNamespaces {
		err = createNamespaceAndDefaultSA(ctx, cfg, ns)
		if err != nil {
			return err
		}
	}

	err = deployVolumesAndClaims(ctx, clusterSnapshot, cfg)
	if err != nil {
		return err
	}

	return nil
}

// deployPodAndOwnerss deploys the specified pods into the cluster.
// It creates the required pod owner as well using the OwnerRef to allow for
// CA scale-down activities.
// It also creates the namespace used by the pod and the 'default' ServiceAccount
// for that namespace.
func deployPodsAndOwners(ctx context.Context, cfg *envconf.Config, pods []planner.PodInfo) error {
	for _, podInfo := range pods {
		if err := createNamespaceAndDefaultSA(ctx, cfg, podInfo.Namespace); err != nil {
			return err
		}

		p := podutil.AsPod(podInfo)
		// This is done to prevent container names that can have more than 63 chars
		p.Spec.Containers[0].Name = "dummy-container"

		if p.Spec.Containers[0].Image == "" {
			p.Spec.Containers[0].Image = "dummy-image"
		}

		// Ensures that CA considers this as a replicated pod
		if p.GetOwnerReferences() != nil {
			p.OwnerReferences[0].Controller = ptr.To(true)
		}
		p.ResourceVersion = ""

		if err := createPodOwner(ctx, cfg, p); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}

		if err := cfg.Client().Resources().Create(ctx, p); err != nil {
			return fmt.Errorf("failed to create pod %q: %w", p.Name, err)
		}
	}
	return nil
}

func createPodOwner(ctx context.Context, cfg *envconf.Config, pod *corev1.Pod) error {
	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef == nil {
		return nil
	}

	owner := ownerMeta{
		Name:      controllerRef.Name,
		Namespace: pod.Namespace,
		UID:       controllerRef.UID,
	}

	switch controllerRef.Kind {
	case benchutil.OwnerStatefulSet:
		if err := createStatefulSet(ctx, cfg, owner); err != nil {
			return err
		}
	case benchutil.OwnerReplicaSet:
		if err := createReplicaSet(ctx, cfg, owner); err != nil {
			return err
		}
	case benchutil.OwnerJob:
		if err := createJob(ctx, cfg, owner); err != nil {
			return err
		}
	default:
		fmt.Printf("WARN: Unknown owner kind %s for '%s/%s'\n",
			controllerRef.Kind, pod.Namespace, controllerRef.Name,
		)
	}
	return nil
}

func partitionPods(pods []planner.PodInfo) (scheduled, unscheduled []planner.PodInfo, daemonSetPods sets.Set[string]) {
	daemonSetPods = sets.New[string]()
	for _, podInfo := range pods {
		if podInfo.NodeName == "" {
			if isOwner(podInfo.GetOwnerReferences(), benchutil.OwnerDaemonSet) {
				daemonSetPods.Insert(podInfo.Namespace + "/" + podInfo.Name)
			}
			unscheduled = append(unscheduled, podInfo)
		} else {
			scheduled = append(scheduled, podInfo)
		}
	}
	return
}

func deployPriorityClasses(
	ctx context.Context,
	clusterSnapshot planner.ClusterSnapshot,
	cfg *envconf.Config,
) error {
	log.Println("Deploying priority classes...")
	for _, pClass := range clusterSnapshot.PriorityClasses {
		pClass.ResourceVersion = ""
		err := cfg.Client().Resources().Create(ctx, &pClass)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create priorityClass: %w", err)
		}
	}
	return nil
}

func createNamespaceAndDefaultSA(ctx context.Context, cfg *envconf.Config, name string) error {
	ns := &corev1.Namespace{}
	ns.Name = name
	err := cfg.Client().Resources().Create(ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace: %w", err)
	}
	return createDefaultServiceAccount(ctx, cfg, name)
}

// KWOK does not auto-create service accounts because kube-controller-manager
// isn't running, this creates a "default" service account in the specified namespace.
func createDefaultServiceAccount(ctx context.Context, cfg *envconf.Config, name string) error {
	sa := corev1.ServiceAccount{}
	sa.Name = "default"
	sa.Namespace = name
	err := cfg.Client().Resources().Create(ctx, &sa)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create default serviceAccount in namespace %q: %w", name, err)
	}
	return nil
}

func deployVolumesAndClaims(
	ctx context.Context,
	clusterSnapshot planner.ClusterSnapshot,
	cfg *envconf.Config,
) error {
	log.Println("Deploying PVs, PVCs and StorageClasses...")
	for _, storageClass := range clusterSnapshot.StorageClasses {
		err := cfg.Client().Resources().Create(ctx, &storageClass)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create storage class: %w", err)
		}
	}
	for _, pvcInfo := range clusterSnapshot.PVCs {
		if err := createNamespaceAndDefaultSA(ctx, cfg, pvcInfo.Namespace); err != nil {
			return err
		}
		err := cfg.Client().Resources().Create(ctx, volutil.AsPVC(pvcInfo))
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create persistent volume claim: %w", err)
		}
	}
	for _, pvInfo := range clusterSnapshot.PVs {
		pv := volutil.AsPV(pvInfo)
		pv.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/mnt/data",
			},
		}
		err := cfg.Client().Resources().Create(ctx, pv)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create persistent volume: %w", err)
		}
	}
	return nil
}

func createReplicaSet(ctx context.Context, cfg *envconf.Config, owner ownerMeta) error {
	rs := appsv1.ReplicaSet{}
	rs.Name = owner.Name
	rs.Namespace = owner.Namespace
	rs.UID = owner.UID
	rs.Spec.Selector = dummySelector
	rs.Spec.Template.Labels = dummySelector.MatchLabels
	rs.Spec.Template.Spec.Containers = dummyContainers
	err := cfg.Client().Resources().Create(ctx, &rs)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create replicaset \"%s/%s\": %w", owner.Namespace, owner.Name, err)
	}
	return nil
}

func createStatefulSet(ctx context.Context, cfg *envconf.Config, owner ownerMeta) error {
	ss := appsv1.StatefulSet{}
	ss.Name = owner.Name
	ss.Namespace = owner.Namespace
	ss.UID = owner.UID
	ss.Spec.Selector = dummySelector
	ss.Spec.Template.Labels = dummySelector.MatchLabels
	ss.Spec.Template.Spec.Containers = dummyContainers
	err := cfg.Client().Resources().Create(ctx, &ss)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create statefulset \"%s/%s\": %w", owner.Namespace, owner.Name, err)
	}
	return nil
}

func createJob(ctx context.Context, cfg *envconf.Config, owner ownerMeta) error {
	job := batchv1.Job{}
	job.Name = owner.Name
	job.Namespace = owner.Namespace
	job.UID = owner.UID
	job.Spec.Template.Spec.Containers = dummyContainers
	job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
	err := cfg.Client().Resources().Create(ctx, &job)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create job \"%s/%s\": %w", owner.Namespace, owner.Name, err)
	}
	return nil
}
