package scalein

import (
	"testing"

	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/gardener/scaling-advisor/planner/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	storagevolume "k8s.io/component-helpers/storage/volume"
)

// ---- UnbindPodVolumes tests -------------------------------------------------

func TestUnbindPodVolumes_SimulatedPV_DeletedAndPVCReset(t *testing.T) {
	v := testutil.NewTestView(t)
	ctx := t.Context()

	pvc := testutil.MakeBoundPVC("pvc-a", "default", "simVol-default-pvc-a", map[string]string{
		storagevolume.AnnSelectedNode: "node-a",
	})
	if _, err := v.CreateObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, pvc); err != nil {
		t.Fatalf("create PVC: %v", err)
	}

	claimRef := &corev1.ObjectReference{Kind: "PersistentVolumeClaim", Namespace: "default", Name: "pvc-a"}
	pv := testutil.MakePV("simVol-default-pvc-a", claimRef, true)
	if _, err := v.CreateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, pv); err != nil {
		t.Fatalf("create PV: %v", err)
	}

	pod := testutil.MakePodWithPVC("pod-a", "default", "node-a", "pvc-a")

	if err := volutil.UnbindPodVolumes(ctx, v, pod); err != nil {
		t.Fatalf("UnbindPodVolumes: %v", err)
	}

	// Simulated PV must be deleted.
	pvObj, err := v.GetObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, cache.NewObjectName("", "simVol-default-pvc-a"))
	if pvObj != nil || err == nil {
		t.Error("expected simulated PV to be deleted, but it still exists")
	}

	// PVC must be fully reset to Pending.
	pvcObj, err := v.GetObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, cache.NewObjectName("default", "pvc-a"))
	if err != nil {
		t.Fatalf("get PVC: %v", err)
	}
	got := pvcObj.(*corev1.PersistentVolumeClaim)
	if got.Status.Phase != corev1.ClaimPending {
		t.Errorf("expected PVC phase Pending, got %s", got.Status.Phase)
	}
	if got.Spec.VolumeName != "" {
		t.Errorf("expected PVC.Spec.VolumeName empty, got %q", got.Spec.VolumeName)
	}
	for _, ann := range []string{storagevolume.AnnBindCompleted, storagevolume.AnnBoundByController, storagevolume.AnnSelectedNode} {
		if _, present := got.Annotations[ann]; present {
			t.Errorf("expected annotation %q removed from PVC, but it is still present", ann)
		}
	}
}

func TestUnbindPodVolumes_RealPV_KeptOnlySelectedNodeCleared(t *testing.T) {
	v := testutil.NewTestView(t)
	ctx := t.Context()

	pvc := testutil.MakeBoundPVC("pvc-b", "default", "real-pv", map[string]string{
		storagevolume.AnnSelectedNode: "node-a",
	})
	if _, err := v.CreateObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, pvc); err != nil {
		t.Fatalf("create PVC: %v", err)
	}

	claimRef := &corev1.ObjectReference{Kind: "PersistentVolumeClaim", Namespace: "default", Name: "pvc-b"}
	pv := testutil.MakePV("real-pv", claimRef, false)
	if _, err := v.CreateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, pv); err != nil {
		t.Fatalf("create PV: %v", err)
	}

	pod := testutil.MakePodWithPVC("pod-b", "default", "node-a", "pvc-b")

	if err := volutil.UnbindPodVolumes(ctx, v, pod); err != nil {
		t.Fatalf("UnbindPodVolumes: %v", err)
	}

	// Real PV must still exist.
	pvObj, err := v.GetObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, cache.NewObjectName("", "real-pv"))
	if err != nil || pvObj == nil {
		t.Error("expected real PV to still exist, but it is gone")
	}

	// PVC must remain Bound but AnnSelectedNode cleared.
	pvcObj, err := v.GetObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, cache.NewObjectName("default", "pvc-b"))
	if err != nil {
		t.Fatalf("get PVC: %v", err)
	}
	got := pvcObj.(*corev1.PersistentVolumeClaim)
	if got.Status.Phase != corev1.ClaimBound {
		t.Errorf("expected PVC phase Bound, got %s", got.Status.Phase)
	}
	if got.Spec.VolumeName != "real-pv" {
		t.Errorf("expected PVC.Spec.VolumeName %q, got %q", "real-pv", got.Spec.VolumeName)
	}
	if _, present := got.Annotations[storagevolume.AnnSelectedNode]; present {
		t.Error("expected AnnSelectedNode removed from PVC, but it is still present")
	}
	if _, present := got.Annotations[storagevolume.AnnBindCompleted]; !present {
		t.Error("expected AnnBindCompleted to remain on PVC bound to real PV")
	}
}

func TestUnbindPodVolumes_NoPVCVolumes_NoOp(t *testing.T) {
	v := testutil.NewTestView(t)
	ctx := t.Context()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-c", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
			Volumes: []corev1.Volume{
				{Name: "cfg", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{}}},
			},
		},
	}

	if err := volutil.UnbindPodVolumes(ctx, v, pod); err != nil {
		t.Fatalf("expected no error for pod with no PVC volumes, got: %v", err)
	}
}

func TestUnbindPodVolumes_MultiplePVCs_EachHandledCorrectly(t *testing.T) {
	v := testutil.NewTestView(t)
	ctx := t.Context()

	// pvc-sim: bound to simulated PV
	pvcSim := testutil.MakeBoundPVC("pvc-sim", "default", "simVol-default-pvc-sim", nil)
	if _, err := v.CreateObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, pvcSim); err != nil {
		t.Fatalf("create pvc-sim: %v", err)
	}
	simPV := testutil.MakePV("simVol-default-pvc-sim", &corev1.ObjectReference{Namespace: "default", Name: "pvc-sim"}, true)
	if _, err := v.CreateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, simPV); err != nil {
		t.Fatalf("create sim PV: %v", err)
	}

	// pvc-real: bound to real PV
	pvcReal := testutil.MakeBoundPVC("pvc-real", "default", "real-pv-2", map[string]string{
		storagevolume.AnnSelectedNode: "node-a",
	})
	if _, err := v.CreateObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, pvcReal); err != nil {
		t.Fatalf("create pvc-real: %v", err)
	}
	realPV := testutil.MakePV("real-pv-2", &corev1.ObjectReference{Namespace: "default", Name: "pvc-real"}, false)
	if _, err := v.CreateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, realPV); err != nil {
		t.Fatalf("create real PV: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-d", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
			Volumes: []corev1.Volume{
				{Name: "sim", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-sim"}}},
				{Name: "real", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-real"}}},
			},
		},
	}

	if err := volutil.UnbindPodVolumes(ctx, v, pod); err != nil {
		t.Fatalf("UnbindPodVolumes: %v", err)
	}

	// Simulated PV deleted, pvc-sim fully reset to Pending.
	if pvObj, _ := v.GetObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, cache.NewObjectName("", "simVol-default-pvc-sim")); pvObj != nil {
		t.Error("expected simulated PV deleted")
	}
	if pvcObj, err := v.GetObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, cache.NewObjectName("default", "pvc-sim")); err == nil {
		got := pvcObj.(*corev1.PersistentVolumeClaim)
		if got.Status.Phase != corev1.ClaimPending {
			t.Error("expected pvc-sim reset to Pending")
		}
		if got.Spec.VolumeName != "" {
			t.Error("expected pvc-sim VolumeName cleared")
		}
		if _, present := got.Annotations[storagevolume.AnnSelectedNode]; present {
			t.Error("expected AnnSelectedNode removed from pvc-sim")
		}
	}

	// Real PV kept, AnnSelectedNode cleared on pvc-real.
	if pvObj, err := v.GetObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, cache.NewObjectName("", "real-pv-2")); err != nil || pvObj == nil {
		t.Error("expected real PV to still exist")
	}
	if pvcObj, err := v.GetObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, cache.NewObjectName("default", "pvc-real")); err == nil {
		got := pvcObj.(*corev1.PersistentVolumeClaim)
		if got.Status.Phase != corev1.ClaimBound {
			t.Error("expected pvc-real to remain Bound")
		}
		if _, present := got.Annotations[storagevolume.AnnSelectedNode]; present {
			t.Error("expected AnnSelectedNode removed from pvc-real")
		}
	}
}

// ---- RemoveNodeAndUnbindPods volume tests -----------------------------------

func TestRemoveNodeAndUnbindPods_SimulatedPVDeletedAndPVCReset(t *testing.T) {
	v := testutil.NewTestView(t)
	ctx := t.Context()

	testutil.AddNode(t, v, "node-a")

	pvc := testutil.MakeBoundPVC("pvc-a", "default", "simVol-default-pvc-a", map[string]string{
		storagevolume.AnnSelectedNode: "node-a",
	})
	if _, err := v.CreateObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, pvc); err != nil {
		t.Fatalf("create PVC: %v", err)
	}
	pv := testutil.MakePV("simVol-default-pvc-a", &corev1.ObjectReference{Namespace: "default", Name: "pvc-a"}, true)
	if _, err := v.CreateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, pv); err != nil {
		t.Fatalf("create PV: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
			Volumes: []corev1.Volume{
				{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-a"}}},
			},
			Containers: []corev1.Container{{Name: "c", Image: "img"}},
		},
	}
	if _, err := v.CreateObject(ctx, typeinfo.PodsDescriptor.GVK, pod); err != nil {
		t.Fatalf("create pod: %v", err)
	}

	rs := FreshRunState()
	_, err := rs.Init(ctx, "test-sim", 1, v, "", "node-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	err = rs.RemoveNodeAndUnbindPods("node-a")
	if err != nil {
		t.Fatalf("RemoveNodeAndUnbindPods: %v", err)
	}

	// Simulated PV must be deleted.
	pvObj, _ := v.GetObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, cache.NewObjectName("", "simVol-default-pvc-a"))
	if pvObj != nil {
		t.Error("expected simulated PV deleted after RemoveNodeAndUnbindPods")
	}

	// PVC must be fully reset to Pending.
	pvcObj, err := v.GetObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, cache.NewObjectName("default", "pvc-a"))
	if err != nil {
		t.Fatalf("get PVC: %v", err)
	}
	got := pvcObj.(*corev1.PersistentVolumeClaim)
	if got.Status.Phase != corev1.ClaimPending {
		t.Errorf("expected PVC phase Pending, got %s", got.Status.Phase)
	}
	if got.Spec.VolumeName != "" {
		t.Errorf("expected PVC VolumeName cleared, got %q", got.Spec.VolumeName)
	}
}

func TestRemoveNodeAndUnbindPods_RealPVKept(t *testing.T) {
	v := testutil.NewTestView(t)
	ctx := t.Context()

	testutil.AddNode(t, v, "node-a")

	pvc := testutil.MakeBoundPVC("pvc-b", "default", "real-pv", map[string]string{
		storagevolume.AnnSelectedNode: "node-a",
	})
	if _, err := v.CreateObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, pvc); err != nil {
		t.Fatalf("create PVC: %v", err)
	}
	pv := testutil.MakePV("real-pv", &corev1.ObjectReference{Namespace: "default", Name: "pvc-b"}, false)
	if _, err := v.CreateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, pv); err != nil {
		t.Fatalf("create PV: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
			Volumes: []corev1.Volume{
				{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-b"}}},
			},
			Containers: []corev1.Container{{Name: "c", Image: "img"}},
		},
	}
	if _, err := v.CreateObject(ctx, typeinfo.PodsDescriptor.GVK, pod); err != nil {
		t.Fatalf("create pod: %v", err)
	}

	rs := FreshRunState()
	if _, err := rs.Init(ctx, "test-sim", 1, v, "", "node-a"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := rs.RemoveNodeAndUnbindPods("node-a"); err != nil {
		t.Fatalf("RemoveNodeAndUnbindPods: %v", err)
	}

	// Real PV must still exist.
	pvObj, err := v.GetObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, cache.NewObjectName("", "real-pv"))
	if err != nil || pvObj == nil {
		t.Error("expected real PV to still exist after node removal")
	}

	// PVC must remain Bound, AnnSelectedNode cleared.
	pvcObj, err := v.GetObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, cache.NewObjectName("default", "pvc-b"))
	if err != nil {
		t.Fatalf("get PVC: %v", err)
	}
	got := pvcObj.(*corev1.PersistentVolumeClaim)
	if got.Status.Phase != corev1.ClaimBound {
		t.Errorf("expected PVC phase Bound, got %s", got.Status.Phase)
	}
	if _, present := got.Annotations[storagevolume.AnnSelectedNode]; present {
		t.Error("expected AnnSelectedNode cleared on real-PV-bound PVC")
	}
}
