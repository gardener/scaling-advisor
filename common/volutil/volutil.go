// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package volutil

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/common/objutil"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/viewutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	storagevolume "k8s.io/component-helpers/storage/volume"
	storageutil "k8s.io/kubernetes/pkg/apis/storage/util"
	"k8s.io/utils/ptr"
)

// AsPVInfo converts the given corev1.PersistentVolume to a lean plannerapi PVInfo.
func AsPVInfo(pv corev1.PersistentVolume) plannerapi.PVInfo {
	pvi := plannerapi.PVInfo{
		AccessModes:      pv.Spec.AccessModes,
		Capacity:         pv.Spec.Capacity,
		ObjectMeta:       pv.ObjectMeta,
		NodeAffinity:     pv.Spec.NodeAffinity.Required,
		StorageClassName: pv.Spec.StorageClassName,
		Phase:            pv.Status.Phase,
	}
	if pv.Spec.ClaimRef != nil {
		pvi.ClaimRef.Namespace = pv.Spec.ClaimRef.Namespace

		pvi.ClaimRef.Name = pv.Spec.ClaimRef.Name
	}
	if pv.Spec.VolumeMode != nil {
		pvi.VolumeMode = *pv.Spec.VolumeMode
	} else {
		pvi.VolumeMode = corev1.PersistentVolumeFilesystem // default according to k8s
	}
	return pvi
}

// AsPV converts the given plannerapi PVInfo object to a corev1.PersistentVolume
func AsPV(p plannerapi.PVInfo) *corev1.PersistentVolume {
	var volNodeAffinity *corev1.VolumeNodeAffinity
	if p.NodeAffinity != nil {
		volNodeAffinity = &corev1.VolumeNodeAffinity{Required: p.NodeAffinity}
	}
	return &corev1.PersistentVolume{
		ObjectMeta: p.ObjectMeta,
		Spec: corev1.PersistentVolumeSpec{
			AccessModes:                   p.AccessModes,
			Capacity:                      p.Capacity,
			ClaimRef:                      p.ClaimRef.AsObjectReference(),
			NodeAffinity:                  volNodeAffinity,
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			StorageClassName:              p.StorageClassName,
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: p.Phase,
		},
	}
}

// AsPVCInfo converts the given k8s corev1.PersistentVolumeClaim object to a lean planner PVCInfo.
func AsPVCInfo(pvc corev1.PersistentVolumeClaim) plannerapi.PVCInfo {
	return plannerapi.PVCInfo{
		ObjectMeta:                pvc.ObjectMeta,
		PersistentVolumeClaimSpec: pvc.Spec,
		Phase:                     pvc.Status.Phase,
	}
}

// AsPVC converts the given plannerapi PVCInfo object to a corev1.PersistentVolumeClaim
func AsPVC(p plannerapi.PVCInfo) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: p.ObjectMeta,
		Spec:       p.PersistentVolumeClaimSpec,
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: p.Phase,
		},
	}
}

// GetDefaultStorageClass gets the `StorageClass` annotated with the "storageclass.kubernetes.io/is-default-class"
// annotation in the given classses slice or nil if no such StoreClasss is found
func GetDefaultStorageClass(classes []*storagev1.StorageClass) *storagev1.StorageClass {
	objutil.SortByDecreasingCreationTime(classes)
	for _, sc := range classes {
		if storageutil.IsDefaultAnnotation(sc.ObjectMeta) {
			return sc
		}
	}
	return nil
}

// FindStorageClassWithName finds a storage class with the given name in the given slice of `StorageClass`s or nil if not found
func FindStorageClassWithName(name string, classes []*storagev1.StorageClass) *storagev1.StorageClass {
	for _, sc := range classes {
		if sc.Name == name {
			return sc
		}
	}
	return nil
}

// SortPVCByIncreasingStorage sorts the given slice of `PersistentVolumeClaim`s by increasing storage capacity.
func SortPVCByIncreasingStorage(pvcs []*corev1.PersistentVolumeClaim) {
	slices.SortFunc(pvcs, func(a, b *corev1.PersistentVolumeClaim) int {
		return a.Spec.Resources.Requests.Storage().Cmp(*b.Spec.Resources.Requests.Storage())
	})
}

// BindClaimsForImmediateMode binds the unbound PV and PVC for the "Immediate" VolumeBindingMode in the given
// minkapi.View so that kube-scheduler's VolumeBinding plugin considers the claim satisfied, and the
// kube-scheduler can proceed with pod-node binding. For the kube-scheduler's
// VolumeBinding plugin to succeed, PVC must have:
//   - spec.volumeName set
//   - status.phase = Bound
//   - annotations["pv.kubernetes.io/bind-completed"] = "yes"
//
// PV must have:
//   - spec.claimRef populated
//   - status.phase = Bound
func BindClaimsForImmediateMode(ctx context.Context, view minkapi.View) (claimAssignments []plannerapi.VolumeClaimAssignment, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: cannot bind claims for immediate mode: %w", plannerapi.ErrBindClaimVolume, err)
		}
	}()
	log := logr.FromContextOrDiscard(ctx)
	scs, pvcs, pvs, err := viewutil.ListStorageClassesClaimsAndVolumes(ctx, view)
	if err != nil {
		return
	}
	if len(scs) == 0 || len(pvcs) == 0 {
		return
	}
	var (
		defaultSc = GetDefaultStorageClass(scs)
		boundPVs  = make(map[string]plannerapi.VolumeClaimAssignment) // key is pvName
		chosenPVs = make(map[string]*corev1.PersistentVolume)
		chosenPV  *corev1.PersistentVolume
	)

	// Sort all the claims by increasing size request to get the smallest fits similar to what the PV controller does.
	SortPVCByIncreasingStorage(pvcs)

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
			sc = FindStorageClassWithName(*pvc.Spec.StorageClassName, scs)
			if sc == nil {
				log.V(2).Info("cannot find PVC storage class", "pvcName", pvc.Name, "pvcNamespace", pvc.Namespace, "storageClassName", *pvc.Spec.StorageClassName)
				continue
			}
		}
		if sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
			continue
		}
		chosenPV, err = FindExistingOrCreateSimulatedBindableVolume(ctx, view, sc, pvc, pvs, chosenPVs)
		if err != nil {
			return
		}
		if chosenPV == nil {
			continue
		}
		if err = BindClaimAndVolume(ctx, view, pvc, chosenPV); err != nil {
			return
		}
		boundPVs[chosenPV.Name] = plannerapi.VolumeClaimAssignment{
			ClaimName:  commontypes.NamespacedName{Namespace: pvc.Namespace, Name: pvc.Name},
			VolumeName: chosenPV.Name,
		}
	}
	claimAssignments = slices.Collect(maps.Values(boundPVs))
	log.V(3).Info("BindClaimsForImmediateMode succeeded", "numClaimAssignments", len(claimAssignments))
	return
}

// ProvisionAndBindVolumesFoSelectedClaimsInWFFC performs provisioning of [corev1.PersistentVolume]'s for
// [corev1.PersistentVolumeClaim]'s selected by the `kube-scheduler` for WaitForFirstConsumer volume binding.
// It queries the given [minkapi.View] for PVC's that have been marked with [storagevolume.AnnSelectedNode] which
// indicates that scheduler has triggered the PVC to be dynamically provisioned. It then creates a simulated virtual PV
// that satisfies the PVC and also binds the same.
func ProvisionAndBindVolumesFoSelectedClaimsInWFFC(ctx context.Context, view minkapi.View) (provisionPVs []*corev1.PersistentVolume, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", plannerapi.ErrProvisionVolume, err)
		}
	}()
	var (
		pvcs  []*corev1.PersistentVolumeClaim
		zone  string
		simPV *corev1.PersistentVolume
	)
	if pvcs, err = viewutil.ListPersistentVolumeClaims(ctx, view); err != nil {
		return
	}
	for _, pvc := range pvcs {
		if pvc.Status.Phase != corev1.ClaimPending || !metav1.HasAnnotation(pvc.ObjectMeta, storagevolume.AnnSelectedNode) {
			continue
		}
		zone, err = getSelectedNodeZone(ctx, view, pvc)
		if err != nil {
			return
		}
		if simPV, err = createSimulatedVolumeMatchingClaim(ctx, view, pvc, zone); err != nil {
			return
		}
		if simPV == nil {
			continue
		}
		if err = BindClaimAndVolume(ctx, view, pvc, simPV); err != nil {
			return
		}
		provisionPVs = append(provisionPVs, simPV)
	}
	return
}

// FinalizeStaticBindingsForSelectedClaimsInWFFC completes PVC↔PV bindings for statically provisioned volumes that were
// selected by the kube-scheduler under WaitForFirstConsumer semantics.
//
// This function simulates the PV controller reconciliation step that occurs after the scheduler's VolumeBinding plugin
// has set PersistentVolume.Spec.ClaimRef.
//
// For each PersistentVolume obtained from the view
//
//	If PV.Spec.ClaimRef != nil and the referenced PVC.Spec.VolumeName is empty, the function:
//	  - sets PVC.Spec.VolumeName to PV.Name
//	  - sets PVC.Status.Phase = Bound
//	  - sets PV.Status.Phase = Bound
//	  - sets pvc.Annotations[storagevolume.AnnBindCompleted] = "yes"
func FinalizeStaticBindingsForSelectedClaimsInWFFC(ctx context.Context, view minkapi.View) (numBound int, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", plannerapi.ErrBindClaimVolume, err)
		}
	}()
	var (
		log = logr.FromContextOrDiscard(ctx)
		obj runtime.Object
		pvs []*corev1.PersistentVolume
		pvc *corev1.PersistentVolumeClaim
	)
	pvs, err = viewutil.ListPersistentVolumes(ctx, view)
	if err != nil {
		return
	}
	for _, pv := range pvs {
		ref := pv.Spec.ClaimRef
		if ref == nil || pv.Status.Phase == corev1.VolumeBound {
			continue
		}
		log.V(3).Info("kube-scheduler has bound PV.Spec.ClaimRef", "pvName", pv.Name, "claimRef", ref)
		obj, err = view.GetObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, cache.NewObjectName(ref.Namespace, ref.Name))
		if err != nil {
			log.V(1).Info("cannot get PVC in PV.Spec.ClaimRef", "pvName", pv.Name, "claimRef", ref)
			continue
		}
		pvc = obj.(*corev1.PersistentVolumeClaim)

		pv = pv.DeepCopy() // deepcopy needed to ensure scheduler informer caches see updated objects
		pvc = pvc.DeepCopy()

		pvc.Spec.VolumeName = pv.Name
		pvc.Status.Phase = corev1.ClaimBound
		metav1.SetMetaDataAnnotation(&pvc.ObjectMeta, storagevolume.AnnBindCompleted, "yes")
		pv.Status.Phase = corev1.VolumeBound
		err = view.UpdateObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, pvc)
		if err != nil {
			return
		}
		err = view.UpdateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, pv)
		if err != nil {
			return
		}
		numBound++
		log.V(3).Info("fully bound claim to volume", "pvName", pv.Name, "pvcName", pvc.Name)
	}
	return
}

// FindExistingOrCreateSimulatedBindableVolume attempts to find a matching existing unbound PV within the given slice of pvs
// for the given pvc, excluding ones in chosenPVs. If a chosenPV could not be found, it will simulate one to satisfy
// the PVC and return the same. When creating a simulated PV will pick the first zone from the allowedTopologies (if any)
func FindExistingOrCreateSimulatedBindableVolume(ctx context.Context,
	view minkapi.View,
	sc *storagev1.StorageClass,
	pvc *corev1.PersistentVolumeClaim,
	pvs []*corev1.PersistentVolume,
	chosenPVs map[string]*corev1.PersistentVolume) (chosenPV *corev1.PersistentVolume, err error) {
	log := logr.FromContextOrDiscard(ctx)
	// Leverage shared helper function used by both pv-controller and kube-scheduler and avoid writing our own code here.
	chosenPV, err = storagevolume.FindMatchingVolume(pvc, pvs, nil, chosenPVs, false, true)
	if err != nil {
		err = fmt.Errorf("failed finding chosen PV for PVC %q", objutil.NamespacedName(pvc))
		return
	}
	if chosenPV != nil {
		return
	}
	log.V(3).Info("could not choose an existing PV for PVC - creating simulated matching PV", "pvcName", pvc.Name, "pvcNamespace", pvc.Namespace)
	zone, err := pickZoneFromAllowedTopologies(sc)
	if err != nil {
		return
	}
	if chosenPV, err = createSimulatedVolumeMatchingClaim(ctx, view, pvc, zone); err != nil || chosenPV == nil {
		return
	}
	//safety-check to see if pv-controller helper function actually chooses this simulated PV. It is an error if it does not
	chosenPV, err = storagevolume.FindMatchingVolume(pvc, []*corev1.PersistentVolume{chosenPV}, nil, chosenPVs, false, true)
	if err != nil {
		err = fmt.Errorf("simulated PV was not chosen for PVC %q: %w", objutil.NamespacedName(pvc), err)
	} else if chosenPV == nil {
		err = fmt.Errorf("simulated PV was not chosen for PVC %q", objutil.NamespacedName(pvc))
	}
	return
}

// getSelectedNodeZone gets the [storagevolume.AnnSelectedNode] annotation from the PVC, obtains the Node object from the view
// and gets the node's topology zone. If any of the above do not succeed, returns an error.
func getSelectedNodeZone(ctx context.Context, view minkapi.View, pvc *corev1.PersistentVolumeClaim) (zone string, err error) {
	log := logr.FromContextOrDiscard(ctx)
	selectedNodeName := pvc.Annotations[storagevolume.AnnSelectedNode]
	if selectedNodeName == "" {
		err = fmt.Errorf("annotation %q empty for PVC with name %q", storagevolume.AnnSelectedNode, objutil.NamespacedName(pvc))
		return
	}
	obj, err := view.GetObject(ctx, typeinfo.NodesDescriptor.GVK, cache.NewObjectName(metav1.NamespaceNone, selectedNodeName))
	if err != nil {
		return
	}
	node := obj.(*corev1.Node)
	zone, ok := node.Labels[corev1.LabelTopologyZone]
	if !ok {
		err = fmt.Errorf("%q not found for selected node %q on PVC %q", corev1.LabelTopologyZone, selectedNodeName, objutil.NamespacedName(pvc))
	}
	log.V(3).Info(storagevolume.AnnSelectedNode+"for PVC by kube-scheduler", "pvcName", pvc.Name, "selectedNodeName", selectedNodeName, "zone", zone)
	return
}

// BindClaimAndVolume performs end to end binding between the given PVC and PV via the given minkapi view or returns an error with sentinel plannerapi.ErrBindClaimVolume
func BindClaimAndVolume(ctx context.Context, view minkapi.View, pvc *corev1.PersistentVolumeClaim, pv *corev1.PersistentVolume) error {
	log := logr.FromContextOrDiscard(ctx)
	pv = pv.DeepCopy() // deepcopy needed to ensure scheduler informer caches see updated objects
	pvc = pvc.DeepCopy()

	//  Bind PVC → PV
	pvc.Spec.VolumeName = pv.Name
	pvc.Status.Phase = corev1.ClaimBound
	metav1.SetMetaDataAnnotation(&pvc.ObjectMeta, storagevolume.AnnBindCompleted, "yes")
	metav1.SetMetaDataAnnotation(&pvc.ObjectMeta, storagevolume.AnnBoundByController, "yes") // VERY-IMPORTANT
	delete(pvc.Annotations, storagevolume.AnnSelectedNode)                                   // avoid provisioning again
	if err := view.UpdateObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, pvc); err != nil {
		log.Error(err, "failed to bind pvc<->pv", "pvc", pvc, "pv", pv)
		return fmt.Errorf("%w: failed to bind pvc %q ->pv %q: %w", plannerapi.ErrBindClaimVolume, pvc.Name, pv.Name, err)
	}

	// Bind PV → PVC
	pv.Spec.ClaimRef = &corev1.ObjectReference{
		Kind:       typeinfo.KindPersistentVolumeClaim,
		Namespace:  pvc.Namespace,
		Name:       pvc.Name,
		UID:        pvc.UID,
		APIVersion: "v1",
	}
	pv.Status.Phase = corev1.VolumeBound
	pv.Status.LastPhaseTransitionTime = ptr.To(metav1.Now())
	metav1.SetMetaDataAnnotation(&pv.ObjectMeta, storagevolume.AnnBoundByController, "yes") // VERY-IMPORTANT
	if err := view.UpdateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, pv); err != nil {
		log.Error(err, "failed to bind pv->pvc", "pv", pv, "pvc", pvc)
		return fmt.Errorf("%w: failed to bind pv %q ->pvc %q: %w", plannerapi.ErrBindClaimVolume, pv.Name, pvc.Name, err)
	}

	log.V(3).Info("bound pvc<->pv", "pvcName", pvc.Name, "pvcNamespace", pvc.Namespace, "pvName", pv.Name)
	return nil
}

// createSimulatedVolumeMatchingClaim creates a PV that satisfies the given PVC but will not set any binding information.
func createSimulatedVolumeMatchingClaim(ctx context.Context, view minkapi.View, pvc *corev1.PersistentVolumeClaim, zone string) (pv *corev1.PersistentVolume, err error) {
	var (
		log   = logr.FromContextOrDiscard(ctx)
		pvObj runtime.Object
	)
	pvName := simulatedVolumeNamePrefix + pvc.Namespace + "-" + pvc.Name
	if pvObj, err = view.GetObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, cache.NewObjectName(metav1.NamespaceNone, pvName)); pvObj != nil {
		pv = pvObj.(*corev1.PersistentVolume)
		log.V(4).Info("simulated PV already created for PVC", "pvName", pv.Name, "pvcName", pvc.Name, "pvcNamespace", pvc.Namespace)
		return
	}
	if !apierrors.IsNotFound(err) {
		return
	}
	pv = &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: simulatedVolumeNamePrefix + pvc.Namespace + "-" + pvc.Name,
			Annotations: map[string]string{
				storagevolume.AnnDynamicallyProvisioned: "scaling-advisor",
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: pvc.Spec.Resources.Requests[corev1.ResourceStorage],
			},
			AccessModes:                   pvc.Spec.AccessModes,
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			StorageClassName:              ptr.Deref(pvc.Spec.StorageClassName, ""),
			VolumeMode:                    pvc.Spec.VolumeMode,
			VolumeAttributesClassName:     pvc.Spec.VolumeAttributesClassName,
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeAvailable,
		},
	}
	if zone != "" {
		pv.Spec.NodeAffinity = &corev1.VolumeNodeAffinity{
			Required: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      corev1.LabelTopologyZone,
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{zone},
							},
						},
					},
				},
			},
		}
	}
	if _, err = view.CreateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, pv); err != nil {
		err = fmt.Errorf("could not create simulated PV %q matching PVC %q: %w", pv.Name, objutil.NamespacedName(pvc), err)
		return
	}
	if err = logutil.DumpObjectIfNeeded(ctx, pv); err != nil {
		return
	}
	log.V(3).Info("created simulated PV matching PVC", "pvName", pv.Name,
		"pvcName", pvc.Name, "pvcNamespace", pvc.Namespace, "zone", zone)
	return
}

func pickZoneFromAllowedTopologies(sc *storagev1.StorageClass) (string, error) {
	if sc.AllowedTopologies == nil {
		return "", nil // no topology restriction
	}

	for _, term := range sc.AllowedTopologies {
		for _, expr := range term.MatchLabelExpressions {
			if expr.Key == corev1.LabelTopologyZone && len(expr.Values) > 0 {
				// Deterministic choice: first value
				return expr.Values[0], nil
			}
		}
	}

	return "", fmt.Errorf("allowedTopologies specified but no %q found", corev1.LabelTopologyZone)
}

const simulatedVolumeNamePrefix = "simVol-"
