// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package simulator provides types and helper functions for all simulator implementations
package simulator

import (
	"context"
	"fmt"
	"maps"
	"slices"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/gardener/scaling-advisor/minkapi/viewutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	storagevolume "k8s.io/component-helpers/storage/volume"
	"k8s.io/utils/ptr"
)

// PopulateView populates the given minkapi.View with the objects in the given ClusterSnapshot.
func PopulateView(ctx context.Context, view minkapi.View, cs *plannerapi.ClusterSnapshot) error {
	if err := view.Reset(); err != nil {
		return err
	}
	for _, pc := range cs.PriorityClasses {
		if _, err := view.CreateObject(ctx, typeinfo.PriorityClassesDescriptor.GVK, &pc); err != nil {
			return err
		}
	}
	for _, rc := range cs.RuntimeClasses {
		if _, err := view.CreateObject(ctx, typeinfo.RuntimeClassDescriptor.GVK, &rc); err != nil {
			return err
		}
	}
	for _, sc := range cs.StorageClasses {
		if _, err := view.CreateObject(ctx, typeinfo.StorageClassDescriptor.GVK, &sc); err != nil {
			return err
		}
	}
	for _, nodeInfo := range cs.Nodes {
		createdObj, err := view.CreateObject(ctx, typeinfo.NodesDescriptor.GVK, nodeutil.AsNode(nodeInfo))
		if err != nil {
			return err
		}
		if nodeInfo.CSINodeSpec == nil {
			continue
		}
		csiNode := nodeutil.NewCSINode(nodeInfo.Name, createdObj.GetUID(), *nodeInfo.CSINodeSpec)
		if _, err = view.CreateObject(ctx, typeinfo.CSINodeDescriptor.GVK, csiNode); err != nil {
			return err
		}
	}
	for _, pvc := range cs.PVCs {
		if _, err := view.CreateObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, volutil.AsPVC(pvc)); err != nil {
			return err
		}
	}
	for _, pv := range cs.PVs {
		if _, err := view.CreateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, volutil.AsPV(pv)); err != nil {
			return err
		}
	}
	for _, pod := range cs.Pods {
		if _, err := view.CreateObject(ctx, typeinfo.PodsDescriptor.GVK, podutil.AsPod(pod)); err != nil {
			return err
		}
	}
	return nil
}

// BindClaimsAndVolumesForImmediateMode binds the unbound PV and PVC for the
// "Immediate" VolumeBindingMode in the given minkapi.View so that
// kube-scheduler's VolumeBinding plugin considers the claim satisfied, and the
// kube-scheduler can proceed with pod-node binding. For the kube-scheduler's
// VolumeBinding plugin to succeed, PVC must have:
//   - spec.volumeName set
//   - status.phase = Bound
//   - annotations["pv.kubernetes.io/bind-completed"] = "yes"
//
// PV must have:
//   - spec.claimRef populated
//   - status.phase = Bound
func BindClaimsAndVolumesForImmediateMode(ctx context.Context, view minkapi.View) ([]plannerapi.VolumeClaimAssignment, error) {
	log := logr.FromContextOrDiscard(ctx)
	scs, pvcs, pvs, err := viewutil.ListStorageClassesClaimsAndVolumes(ctx, view)
	if err != nil {
		return nil, err
	}
	if len(scs) == 0 || len(pvcs) == 0 || len(pvs) == 0 {
		return nil, nil
	}
	var defaultSc = volutil.GetDefaultStorageClass(scs)
	var boundPVs = make(map[string]plannerapi.VolumeClaimAssignment) // key is pvName

	volutil.SortPersistentVolumesByIncreasingStorage(pvs)
	for _, pvc := range pvcs {
		if pvc.Status.Phase != corev1.ClaimPending {
			continue
		}

		var sc *storagev1.StorageClass
		if pvc.Spec.StorageClassName == nil {
			if defaultSc == nil {
				log.V(2).Info("pvc does not have storage class, skipping", "pvcName", pvc.Name, "pvcNamespace", pvc.Namespace)
				continue
			}
			pvc.Spec.StorageClassName = ptr.To(defaultSc.Name)
			sc = defaultSc
		} else {
			sc = volutil.FindStorageClassWithName(*pvc.Spec.StorageClassName, scs)
			if sc == nil {
				log.V(2).Info("cannot find PVC storage class", "pvcName", pvc.Name, "pvcNamespace", pvc.Namespace, "storageClassName", *pvc.Spec.StorageClassName)
				continue
			}
		}

		if sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
			continue
		}

		var selectedPV corev1.PersistentVolume
		for _, pv := range pvs {
			if _, alreadyBound := boundPVs[pv.Name]; alreadyBound {
				continue
			}
			if !IsPVBindCandidate(ctx, &pvc, &pv, sc.Name) {
				continue
			}
			selectedPV = pv
			break
		}
		if selectedPV.Name == "" {
			continue
		}

		if err = BindClaimAndVolume(ctx, view, &pvc, &selectedPV); err != nil {
			return nil, err
		}
		boundPVs[selectedPV.Name] = plannerapi.VolumeClaimAssignment{
			ClaimName:  commontypes.NamespacedName{Namespace: pvc.Namespace, Name: pvc.Name},
			VolumeName: selectedPV.Name,
		}
	}
	claimAssignments := slices.Collect(maps.Values(boundPVs))
	if len(claimAssignments) >= 0 {
		log.V(3).Info("BindClaimsAndVolumesForImmediateMode succeeded", "numClaimAssignments", len(claimAssignments))
	}
	return claimAssignments, nil
}

// BindClaimAndVolume binds the given PVC and PV via the given minkapi view or returns an error with sentinel plannerapi.ErrBindClaimVolume
func BindClaimAndVolume(ctx context.Context, view minkapi.View, pvc *corev1.PersistentVolumeClaim, pv *corev1.PersistentVolume) error {
	log := logr.FromContextOrDiscard(ctx)

	// Bind PV → PVC
	pv.Spec.ClaimRef = &corev1.ObjectReference{
		Kind:            typeinfo.KindPersistentVolumeClaim,
		Namespace:       pvc.Namespace,
		Name:            pvc.Name,
		UID:             pvc.UID,
		APIVersion:      "v1",
		ResourceVersion: pvc.ResourceVersion,
	}
	pv.Status.Phase = corev1.VolumeBound
	pv.Status.LastPhaseTransitionTime = ptr.To(metav1.Now())
	if err := view.UpdateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, pv); err != nil {
		log.Error(err, "failed to bind pv->pvc", "pv", pv, "pvc", pvc)
		return fmt.Errorf("%w: failed to bind pv %q ->pvc %q: %w", plannerapi.ErrBindClaimVolume, pv.Name, pvc.Name, err)
	}

	//  Bind PVC → PV
	pvc.Spec.VolumeName = pv.Name
	pvc.Status.Phase = corev1.ClaimBound
	if pvc.Annotations == nil {
		pvc.Annotations = map[string]string{}
	}
	pvc.Annotations[storagevolume.AnnBindCompleted] = "yes"
	if err := view.UpdateObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, pvc); err != nil {
		log.Error(err, "failed to bind pvc->pv", "pvc", pvc, "pv", pv)
		return fmt.Errorf("%w: failed to bind pvc %q ->pv %q: %w", plannerapi.ErrBindClaimVolume, pvc.Name, pv.Name, err)
	}

	log.V(3).Info("bound pvc<->pv", "pvcName", pvc.Name, "pvName", pv.Name)
	return nil
}

// IsPVBindCandidate checks whether the given PV can be selected as a bindable candidate for the given PVC.
func IsPVBindCandidate(ctx context.Context, pvc *corev1.PersistentVolumeClaim, pv *corev1.PersistentVolume, storageClassName string) bool {
	log := logr.FromContextOrDiscard(ctx)
	if pv.Status.Phase != corev1.VolumeAvailable {
		return false
	}
	if pv.Spec.StorageClassName != storageClassName {
		return false
	}
	if pvc.Spec.VolumeMode == nil {
		pvc.Spec.VolumeMode = ptr.To(corev1.PersistentVolumeFilesystem)
	}
	if pv.Spec.VolumeMode == nil {
		pv.Spec.VolumeMode = ptr.To(corev1.PersistentVolumeFilesystem)
	}
	if *pvc.Spec.VolumeMode != *pv.Spec.VolumeMode {
		return false
	}
	requested := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	capacity := pv.Spec.Capacity[corev1.ResourceStorage]
	if capacity.Cmp(requested) < 0 {
		log.V(4).Info("PV capacity less than PVC request",
			"pvcName", pvc.Name,
			"requested", requested,
			"pvName", pv.Name,
			"capacity", capacity)
		return false
	}
	if !areAccessModesCompatible(pvc.Spec.AccessModes, pv.Spec.AccessModes) {
		return false
	}
	matches, err := objutil.SelectorMatchesLabels(pvc.Spec.Selector, pv.Labels)
	if err != nil {
		log.V(3).Info("PVC selector conversion error", "pvcName", pvc.Name, "error", err)
		return false
	}
	return matches
}

func areAccessModesCompatible(requested, available []corev1.PersistentVolumeAccessMode) bool {
	availableSet := sets.New[corev1.PersistentVolumeAccessMode](available...)
	return availableSet.HasAll(requested...)
}
