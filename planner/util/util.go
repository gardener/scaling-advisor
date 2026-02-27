// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	"github.com/gardener/scaling-advisor/minkapi/viewutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	storagevolume "k8s.io/component-helpers/storage/volume"
	"k8s.io/utils/ptr"
)

// SendErrorResponse wraps the given error with the sentinel error plannerapi.ErrGenScalingPlan, embeds the wrapped error
// within a plannerapi.Response and sends the response to the given results channel.
func SendErrorResponse(resultsCh chan<- plannerapi.Response, requestRef plannerapi.RequestRef, err error) {
	err = plannerapi.AsGenError(requestRef.ID, requestRef.CorrelationID, err)
	resultsCh <- plannerapi.Response{
		ID:    objutil.GenerateName("plan-error"),
		Error: err,
	}
}

// SendScaleOutPlanError wraps the given error within a sentinel error plannerapi.ErrGenScalingPlan, creates a ScaleOutPlanResult and
// sends the result on the planResultCh.
func SendScaleOutPlanError(planResultCh chan<- plannerapi.ScaleOutPlanResult, requestRef plannerapi.RequestRef, err error) {
	err = plannerapi.AsGenError(requestRef.ID, requestRef.CorrelationID, err)
	planResultCh <- plannerapi.ScaleOutPlanResult{
		Error: err,
	}
}

// SendScaleOutPlanResult creates a plannerapi.ScaleOutPlanResult from the given plannerapi.Request and plannerapi.SimulationGroupCycleResults
// and sends this result to the resultCh.
func SendScaleOutPlanResult(ctx context.Context, resultCh chan<- plannerapi.ScaleOutPlanResult,
	req *plannerapi.Request, simulationRunCount uint32, // TODO: introduce a plannerapi.Metrics.
	groupCycleResults []plannerapi.ScaleOutSimGroupCycleResult) error {
	log := logr.FromContextOrDiscard(ctx)
	existingNodeCountByPlacement, err := req.Snapshot.GetNodeCountByPlacement()
	if err != nil {
		return err
	}
	planGenerateDuration := time.Since(req.CreationTime)
	numUnscheduledPods := len(req.Snapshot.GetUnscheduledPods())
	labels := map[string]string{
		commonconstants.LabelRequestID:                  req.ID,
		commonconstants.LabelCorrelationID:              req.CorrelationID,
		commonconstants.LabelTotalSimulationRuns:        fmt.Sprintf("%d", simulationRunCount),
		commonconstants.LabelPlanGenerateDuration:       planGenerateDuration.String(),
		commonconstants.LabelSnapshotNumUnscheduledPods: strconv.Itoa(numUnscheduledPods),
		commonconstants.LabelConstraintNumPools:         strconv.Itoa(len(req.Constraint.Spec.NodePools)),
	}
	var allWinnerNodeScores []plannerapi.NodeScore
	var leftOverUnscheduledPods []commontypes.NamespacedName
	for _, gcr := range groupCycleResults {
		allWinnerNodeScores = append(allWinnerNodeScores, gcr.WinnerNodeScores...)
		leftOverUnscheduledPods = gcr.LeftoverUnscheduledPods
	}
	scaleOutPlan := createScaleOutPlan(allWinnerNodeScores, existingNodeCountByPlacement, leftOverUnscheduledPods)
	planResult := plannerapi.ScaleOutPlanResult{
		Labels:       labels,
		ScaleOutPlan: &scaleOutPlan,
	}
	log.V(2).Info("Sent Planner Success Response", "response", planResult)
	resultCh <- planResult
	return nil
}

// PopulateView populates the given view with the objects in the given cluster snapshot.
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

// createScaleOutPlan creates a ScaleOutPlan based on the given winningNodeScores, existingNodeCountByPlacement and leftoverUnscheduledPods.
func createScaleOutPlan(winningNodeScores []plannerapi.NodeScore, existingNodeCountByPlacement map[sacorev1alpha1.NodePlacement]int32, leftoverUnscheduledPods []commontypes.NamespacedName) sacorev1alpha1.ScaleOutPlan {
	scaleItems := make([]sacorev1alpha1.ScaleOutItem, 0, len(winningNodeScores))
	nodeScoresByPlacement := groupNodeScoresByNodePlacement(winningNodeScores)
	for placement, nodeScores := range nodeScoresByPlacement {
		delta := int32(len(nodeScores)) // #nosec G115 -- length of nodeScores cannot be greater than max int32.
		currentReplicas := existingNodeCountByPlacement[placement]
		scaleItems = append(scaleItems, sacorev1alpha1.ScaleOutItem{
			NodePlacement:   placement,
			CurrentReplicas: currentReplicas,
			Delta:           delta,
		})
	}
	return sacorev1alpha1.ScaleOutPlan{
		UnsatisfiedPodNames: objutil.GetFullNames(leftoverUnscheduledPods),
		Items:               scaleItems,
	}
}

// groupNodeScoresByNodePlacement groups the given nodeScores by their NodePlacement and returns a map of NodePlacement to slice of NodeScores.
func groupNodeScoresByNodePlacement(nodeScores []plannerapi.NodeScore) map[sacorev1alpha1.NodePlacement][]plannerapi.NodeScore {
	groupByPlacement := make(map[sacorev1alpha1.NodePlacement][]plannerapi.NodeScore)
	for _, ns := range nodeScores {
		groupByPlacement[ns.Placement] = append(groupByPlacement[ns.Placement], ns)
	}
	return groupByPlacement
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

func areAccessModesCompatible(requested, available []corev1.PersistentVolumeAccessMode) bool {
	availableSet := sets.New[corev1.PersistentVolumeAccessMode](available...)
	return availableSet.HasAll(requested...)
}
