// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package simulation provides types and helper functions for all simulation implementations
package simulation

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/minkapi/viewutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	storagevolume "k8s.io/component-helpers/storage/volume"
)

// BindClaimsAndVolumesWithNonNilClaimRefs fully completes the PVC<->PV binding after the kube-scheduler VolumeBinding plugin
// has set the PersistentVolume.Spec.ClaimRef. It obtains the PersistentVolume's from the view and does the following for each PV.
//
// if PV.spec.claimRef != nil and AND claimRef PVC.spec.volumeName == ""
//   - update PVC.spec.volumeName
//   - update PVC.status.phase = Bound
//   - update PV.status.phase = Bound
func BindClaimsAndVolumesWithNonNilClaimRefs(ctx context.Context, view minkapi.View) (numBound int, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", plannerapi.ErrBindClaimVolume, err)
		}
	}()
	var (
		log = logr.FromContextOrDiscard(ctx)
		obj runtime.Object
		pvs []corev1.PersistentVolume
		pvc *corev1.PersistentVolumeClaim
	)
	pvs, err = viewutil.ListPersistentVolumes(ctx, view)
	if err != nil {
		return
	}
	for _, pv := range pvs {
		ref := pv.Spec.ClaimRef
		if ref == nil {
			continue
		}
		if pv.Status.Phase == corev1.VolumeBound {
			continue
		}
		log.V(3).Info("kube-scheduler has bound PV.Spec.ClaimRef", "pvName", pv.Name, "claimRef", ref)
		obj, err = view.GetObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, cache.NewObjectName(ref.Namespace, ref.Name))
		if err != nil {
			return
		}
		pvc = obj.(*corev1.PersistentVolumeClaim)
		if pvc.Spec.VolumeName != "" {
			continue
		}
		pvc.Spec.VolumeName = pv.Name
		pvc.Status.Phase = corev1.ClaimBound
		if pvc.Annotations == nil {
			pvc.Annotations = map[string]string{}
		}
		pvc.Annotations[storagevolume.AnnBindCompleted] = "yes"
		pv.Status.Phase = corev1.VolumeBound
		err = view.UpdateObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, pvc)
		if err != nil {
			return
		}
		err = view.UpdateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, &pv)
		if err != nil {
			return
		}
		numBound++
		log.V(3).Info("fully bound claim to volume", "pvName", pv.Name, "pvcName", pvc.Name)
	}
	return
}
