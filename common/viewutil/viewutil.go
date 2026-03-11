// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package viewutil

import (
	"context"
	"fmt"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

// ListUnscheduledPods returns all Pods from the given View that are not scheduled to any Node.
func ListUnscheduledPods(ctx context.Context, view minkapi.View) ([]corev1.Pod, error) {
	allPods, err := view.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return nil, err
	}
	unscheduledPods := make([]corev1.Pod, 0, len(allPods))
	for _, p := range allPods {
		if p.Spec.NodeName == "" {
			unscheduledPods = append(unscheduledPods, p)
		}
	}
	return unscheduledPods, nil
}

// ListPersistentVolumes lists all the persistent volumes in the given minkapi view.
func ListPersistentVolumes(ctx context.Context, view minkapi.View) ([]*corev1.PersistentVolume, error) {
	objs, _, err := view.ListMetaObjects(ctx, typeinfo.PersistentVolumesDescriptor.GVK, minkapi.MatchAllCriteria)
	if err != nil {
		return nil, err
	}
	allPVs := make([]*corev1.PersistentVolume, 0, len(objs))
	for _, o := range objs {
		pv, ok := o.(*corev1.PersistentVolume)
		if !ok {
			return nil, fmt.Errorf("expected PersistentVolume, unexpected object %v", o)
		}
		allPVs = append(allPVs, pv)
	}
	return allPVs, nil
}

// ListPersistentVolumeClaims lists all the persistent volume claims in the given minkapi view.
func ListPersistentVolumeClaims(ctx context.Context, view minkapi.View) ([]*corev1.PersistentVolumeClaim, error) {
	objs, _, err := view.ListMetaObjects(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, minkapi.MatchAllCriteria)
	if err != nil {
		return nil, err
	}
	allPVCs := make([]*corev1.PersistentVolumeClaim, 0, len(objs))
	for _, o := range objs {
		pvc, ok := o.(*corev1.PersistentVolumeClaim)
		if !ok {
			return nil, fmt.Errorf("expected PersistentVolumeClaim, unexpected object %v", o)
		}
		allPVCs = append(allPVCs, pvc)
	}
	return allPVCs, nil
}

// LogObjects logs the node and pod names in the given minkapi view using logger from the given context if any.
// At higher log verbosity, it also dumps all scheduling relevant objects into <tempDir>/<viewName>/ directory.
func LogObjects(ctx context.Context, prefix string, view minkapi.View) error {
	log := logr.FromContextOrDiscard(ctx)
	allPods, err := view.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return err
	}
	allNodes, err := view.ListNodes(ctx)
	if err != nil {
		return err
	}
	log.V(2).Info(prefix+"|count of nodes,pods",
		"viewName", view.GetName(),
		"totalNodes", len(allNodes),
		"totalPods", len(allPods),
		"totalUnscheduledPods", podutil.CountUnscheduledPods(allPods))
	if !log.V(4).Enabled() {
		return nil
	}
	for idx, pod := range allPods {
		log.V(4).Info(prefix+"|pod in view",
			"viewName", view.GetName(), "idx", idx, "podName", pod.Name, "podNamespace", pod.Namespace,
			"assignedNodeName", pod.Spec.NodeName, "pvcNames", podutil.GetPVCNames(pod), "podRequests", pod.Spec.Containers[0].Resources.Requests)
	}
	for _, node := range allNodes {
		log.V(4).Info(prefix+"|node in view",
			"viewName", view.GetName(),
			"nodeName", node.Name,
			"nodePool", node.Labels[commonconstants.LabelNodePoolName],
			"instanceType", node.Labels[corev1.LabelInstanceTypeStable],
			"region", node.Labels[corev1.LabelTopologyRegion],
			"zone", node.Labels[corev1.LabelTopologyZone],
			"allocatable", node.Status.Allocatable)
	}
	pvs, err := ListPersistentVolumes(ctx, view)
	if err != nil {
		return err
	}
	pvcs, err := ListPersistentVolumeClaims(ctx, view)
	if err != nil {
		return err
	}
	for _, pvc := range pvcs {
		log.V(4).Info(prefix+"|pvc in view", "viewName", view.GetName(),
			"pvcName", pvc.Name,
			"pvcNamespace", pvc.Namespace,
			"pvcAnnotations", pvc.Annotations,
			"phase", pvc.Status.Phase,
			"storageClassName", pvc.Spec.StorageClassName,
			"storageRequest", pvc.Spec.Resources.Requests.Storage(),
			"selector", pvc.Spec.Selector,
			"pvcAnnotations", pvc.Annotations,
			"uid", string(pvc.UID))
	}
	for _, pv := range pvs {
		log.V(4).Info(prefix+"|pv in view",
			"viewName", view.GetName(),
			"pvName", pv.GetName(),
			"pvAnnotations", pv.Annotations,
			"phase", pv.Status.Phase,
			"storageClassName", pv.Spec.StorageClassName,
			"storageCapacity", pv.Spec.Capacity.Storage(),
			"claimRef", pv.Spec.ClaimRef,
			"labels", pv.Labels)
	}
	return nil
}

// ListStorageClassesClaimsAndVolumes gets the slice of [storagev1.StorageClass],
// slice of [corev1.PersistentVolumeClaim] and slice of [corev1.PersistentVolume] from the given minkapi view or an error
func ListStorageClassesClaimsAndVolumes(ctx context.Context, view minkapi.View) (scs []*storagev1.StorageClass, pvcs []*corev1.PersistentVolumeClaim, pvs []*corev1.PersistentVolume, err error) {
	scObjs, _, err := view.ListMetaObjects(ctx, typeinfo.StorageClassDescriptor.GVK, minkapi.MatchAllCriteria)
	if err != nil {
		return
	}
	scs = make([]*storagev1.StorageClass, 0, len(scObjs))
	var sc *storagev1.StorageClass
	for _, o := range scObjs {
		if sc, err = objutil.Cast[*storagev1.StorageClass](o); err != nil {
			return
		}
		scs = append(scs, sc)
	}

	if pvcs, err = ListPersistentVolumeClaims(ctx, view); err != nil {
		return
	}

	if pvs, err = ListPersistentVolumes(ctx, view); err != nil {
		return
	}
	return
}

// GetPersistentVolume returns the PersistentVolume with the given name from the given [minkapi.View]
func GetPersistentVolume(ctx context.Context, name string, view minkapi.View) (*corev1.PersistentVolume, error) {
	obj, err := view.GetObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, cache.NewObjectName(metav1.NamespaceNone, name))
	if err != nil {
		return nil, err
	}
	return obj.(*corev1.PersistentVolume), err
}
