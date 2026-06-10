// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"

	"github.com/gardener/scaling-advisor/api/planner"
	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var dummyContainers = []corev1.Container{
	{
		Name:  "dummy-container",
		Image: "dummy-image",
	},
}

// deployObjects creates the non-pod/node Kubernetes objects that form the
// cluster state: priority classes, defaultNamespaces' service account,
// pod owners and storage classes, volumes and claims
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

	err = deployPodOwners(ctx, clusterSnapshot, cfg)
	if err != nil {
		return err
	}

	err = deployVolumesAndClaims(ctx, clusterSnapshot, cfg)
	if err != nil {
		return err
	}

	return nil
}

// deployPods deploys the specified pods into the cluster.
// It also creates the namespace used by the pod and the 'default' ServiceAccount
// for that namespace.
func deployPods(ctx context.Context, cfg *envconf.Config, pods []planner.PodInfo) error {
	for _, podInfo := range pods {
		if err := createNamespaceAndDefaultSA(ctx, cfg, podInfo.Namespace); err != nil {
			return err
		}
		if err := createPod(ctx, cfg, podInfo); err != nil {
			return err
		}
	}
	return nil
}

func partitionPods(pods []planner.PodInfo) (scheduled, unscheduled []planner.PodInfo, daemonSetPodCount int) {
	for _, podInfo := range pods {
		if podInfo.NodeName == "" {
			if isOwner(podInfo.GetOwnerReferences(), benchutil.OwnerDaemonSet) {
				daemonSetPodCount++
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

func deployPodOwners(
	ctx context.Context,
	clusterSnapshot planner.ClusterSnapshot,
	cfg *envconf.Config,
) error {
	log.Println("Deploying pod owners...")
	for _, owner := range clusterSnapshot.PodOwners {
		if err := createNamespaceAndDefaultSA(ctx, cfg, owner.Namespace); err != nil {
			return err
		}
		switch owner.Kind {
		case benchutil.OwnerStatefulSet:
			if err := createSS(ctx, cfg, owner); err != nil {
				return err
			}
		case benchutil.OwnerReplicaSet:
			if err := createRS(ctx, cfg, owner); err != nil {
				return err
			}
		case benchutil.OwnerJob:
			if err := createJob(ctx, cfg, owner); err != nil {
				return err
			}
		default:
			fmt.Printf("WARN: Unknown owner kind %s for '%s/%s'\n", owner.Kind, owner.Namespace, owner.Name)
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
		if err := cfg.Client().Resources().Create(ctx, &storageClass); err != nil {
			return fmt.Errorf("failed to create storage class: %w", err)
		}
	}
	for _, pvcInfo := range clusterSnapshot.PVCs {
		if err := createNamespaceAndDefaultSA(ctx, cfg, pvcInfo.Namespace); err != nil {
			return err
		}
		if err := cfg.Client().Resources().Create(ctx, volutil.AsPVC(pvcInfo)); err != nil {
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

// createPod converts a PodInfo to a corev1.Pod, applies the fixups needed
// for KWOK (dummy image, cleared identity fields) and creates it.
func createPod(ctx context.Context, cfg *envconf.Config, podInfo planner.PodInfo) error {
	p := podutil.AsPod(podInfo)
	if p.Spec.Containers[0].Image == "" {
		p.Spec.Containers[0].Image = "dummy-image"
	}
	// This is done to prevent container names that can have more than 63 chars
	p.Spec.Containers[0].Name = "dummy-container"
	p.ResourceVersion = ""

	// HACK: remove once https://github.com/gardener/scaling-advisor/pull/164 is merged
	// This is needed to clean up pods that cannot be deployed due to failures:
	// metadata.annotations[container.apparmor.security.beta.kubernetes.io/install-cni]:
	// Invalid value: "install-cni": container not found
	maps.DeleteFunc(p.Annotations, func(k, _ string) bool {
		return strings.HasSuffix(k, "install-cni")
	})

	// This is needed to fix invalid pods having same keys in 'matchLabelKeys'
	// and 'labelSelector' for `spec.topologySpreadConstraints`
	for i, tsc := range p.Spec.TopologySpreadConstraints {
		if tsc.LabelSelector == nil || len(tsc.MatchLabelKeys) == 0 {
			continue
		}

		matchLabelKeysSet := sets.New(tsc.MatchLabelKeys...)
		// Remove conflicting keys from matchLabels
		for key := range tsc.LabelSelector.MatchLabels {
			if matchLabelKeysSet.Has(key) {
				delete(p.Spec.TopologySpreadConstraints[i].LabelSelector.MatchLabels, key)
			}
		}
		// Remove conflicting keys from matchExpressions
		p.Spec.TopologySpreadConstraints[i].LabelSelector.MatchExpressions = slices.DeleteFunc(
			tsc.LabelSelector.MatchExpressions,
			func(expr metav1.LabelSelectorRequirement) bool {
				return matchLabelKeysSet.Has(expr.Key)
			},
		)
	}
	// p.Spec.Priority = ptr.To(int32(i))

	if err := cfg.Client().Resources().Create(ctx, p); err != nil {
		return fmt.Errorf("failed to create pod %q: %w", p.Name, err)
	}

	return nil
}

func createRS(ctx context.Context, cfg *envconf.Config, owner planner.PodOwnerInfo) error {
	rs := appsv1.ReplicaSet{}
	rs.Name = owner.Name
	rs.Namespace = owner.Namespace
	rs.Spec.Selector = owner.Selector
	rs.Spec.Template.Labels = owner.Selector.MatchLabels
	if rs.Spec.Template.Labels == nil {
		rs.Spec.Template.Labels = make(map[string]string)
	}
	for _, req := range owner.Selector.MatchExpressions {
		for _, v := range req.Values {
			rs.Spec.Template.Labels[req.Key] = v
		}
	}
	rs.Spec.Template.Spec.Containers = dummyContainers
	rs.Spec.Replicas = owner.TargetReplicas
	rs.Status.Replicas = owner.CurrentReplicas
	err := cfg.Client().Resources().Create(ctx, &rs)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create replicaset \"%s/%s\": %w", owner.Namespace, owner.Name, err)
	}
	return nil
}

func createSS(ctx context.Context, cfg *envconf.Config, owner planner.PodOwnerInfo) error {
	ss := appsv1.StatefulSet{}
	ss.Name = owner.Name
	ss.Namespace = owner.Namespace
	ss.Spec.Selector = owner.Selector
	ss.Spec.Template.Labels = owner.Selector.MatchLabels
	if ss.Spec.Template.Labels == nil {
		ss.Spec.Template.Labels = make(map[string]string)
	}
	for _, req := range owner.Selector.MatchExpressions {
		for _, v := range req.Values {
			ss.Spec.Template.Labels[req.Key] = v
		}
	}
	ss.Spec.Template.Spec.Containers = dummyContainers
	ss.Spec.Replicas = owner.TargetReplicas
	ss.Status.Replicas = owner.CurrentReplicas
	err := cfg.Client().Resources().Create(ctx, &ss)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create statefulset \"%s/%s\": %w", owner.Namespace, owner.Name, err)
	}
	return nil
}

func createJob(ctx context.Context, cfg *envconf.Config, owner planner.PodOwnerInfo) error {
	job := batchv1.Job{}
	job.Name = owner.Name
	job.Namespace = owner.Namespace
	job.Spec.Template.Spec.Containers = dummyContainers
	job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
	job.Spec.Completions = owner.TargetReplicas
	job.Status.Active = owner.CurrentReplicas
	err := cfg.Client().Resources().Create(ctx, &job)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create job \"%s/%s\": %w", owner.Namespace, owner.Name, err)
	}
	return nil
}
