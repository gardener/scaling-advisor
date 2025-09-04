package view

import (
	"fmt"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/testutil"
	"github.com/gardener/scaling-advisor/minkapi/api"
	mkapi "github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
	gocmp "github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"testing"
)

var (
	log          = klog.NewKlogr()
	baseViewArgs = api.ViewArgs{
		Name:           "base",
		KubeConfigPath: "base",
		Scheme:         typeinfo.SupportedScheme,
		WatchConfig: api.WatchConfig{
			QueueSize: mkapi.DefaultWatchQueueSize,
			Timeout:   mkapi.DefaultWatchTimeout,
		},
	}
	sandboxViewArgs = api.ViewArgs{
		Name:           "sandbox",
		KubeConfigPath: "sandbox",
		Scheme:         typeinfo.SupportedScheme,
		WatchConfig: api.WatchConfig{
			QueueSize: mkapi.DefaultWatchQueueSize,
			Timeout:   mkapi.DefaultWatchTimeout,
		},
	}
	testNodes []corev1.Node
	testPods  []corev1.Pod
)

func TestStoreGetNode(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}

	baseChangeCount := b.GetObjectChangeCount()
	sandboxChangeCount := s.GetObjectChangeCount()
	nA := testNodes[0]
	err = storeNode(t, b, &nA)
	if err != nil {
		return
	}

	t.Run("CheckNodeFromBase", func(t *testing.T) {
		checkNodeInViewIsSame(t, b, &nA)
		if baseChangeCount == b.GetObjectChangeCount() {
			t.Errorf("expected base view to have changed, want %d, got %d", baseChangeCount, b.GetObjectChangeCount())
		}
	})

	t.Run("CheckBaseNodeFromSandbox", func(t *testing.T) {
		checkNodeInViewIsSame(t, s, &nA)
		if sandboxChangeCount != s.GetObjectChangeCount() {
			t.Errorf("expected sandbox view to not have changed, want %d, got %d", sandboxChangeCount, s.GetObjectChangeCount())
		}
	})

	baseChangeCount = b.GetObjectChangeCount()
	nB := *nA.DeepCopy()
	nB.GenerateName = "b-"
	err = storeNode(t, s, &nB)
	if err != nil {
		return
	}
	t.Run("CheckSandboxNodeFromSandbox", func(t *testing.T) {
		checkNodeInViewIsSame(t, s, &nB)
		if baseChangeCount != b.GetObjectChangeCount() {
			t.Errorf("expected base view to not have changed, want %d, got %d", baseChangeCount, b.GetObjectChangeCount())
		}
	})
}

func TestStoreNodeInBaseUpdateInSandbox(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nA := testNodes[0]
	err = storeNode(t, b, &nA)
	if err != nil {
		return
	}
	baseChangeNumAfterStore := b.GetObjectChangeCount() // mark change count of base after storing nA
	sandboxChangeNumAfterStore := s.GetObjectChangeCount()

	// lets make a copy of node A, change conditions and store in sandbox view
	nAWithCondChange := *nA.DeepCopy()
	conditions := []corev1.NodeCondition{
		{
			Type:               corev1.NodeReady,
			Status:             corev1.ConditionFalse,
			Reason:             "NodeNotReady",
			Message:            "Node is not ready due to some issue",
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
		},
	}
	nAWithCondChange.Status.Conditions = conditions
	err = s.UpdateObject(typeinfo.NodesDescriptor.GVK, &nAWithCondChange)
	if err != nil {
		t.Fatalf("in view %q, failed to update node: %v", s.GetName(), err)
		return
	}
	if baseChangeNumAfterStore != b.GetObjectChangeCount() {
		t.Errorf("expected base view to not have changed, want %d, got %d", baseChangeNumAfterStore, b.GetObjectChangeCount())
	}

	// get node from base
	nABase, err := getNode(t, b, nAWithCondChange.GetName())
	if err != nil {
		return
	}
	checkNodeInViewIsSame(t, b, nABase) // check that node in base view has not changed after updating in sandbox
	if t.Failed() {
		return
	}

	// check node in sandbox view is ihe same as the node after changing conditions
	checkNodeInViewIsSame(t, s, &nAWithCondChange)
	if sandboxChangeNumAfterStore == s.GetObjectChangeCount() {
		t.Errorf("expected sandbox view to have diff change count, got same %d", s.GetObjectChangeCount())
		return
	}
}

func TestSandboxPodSandboxNodeBinding(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	initialBaseChangeCount := b.GetObjectChangeCount()
	nA := testNodes[0]
	err = storeNode(t, s, &nA)
	if err != nil {
		return
	}
	pA := testPods[0]
	err = storePod(t, s, &pA)
	if err != nil {
		return
	}
	pAModified, err := updateBinding(t, s, &pA, &nA) // update pod-node binding in the sandbox view
	if err != nil {
		return
	}
	pAModName := objutil.CacheName(pAModified)

	if pAModified.Spec.NodeName != nA.GetName() {
		t.Errorf("in %q view, expected pod to be bound to node %q, got %q", b.GetName(), nA.GetName(), pAModified.Spec.NodeName)
	}
	pASandbox, err := getPod(t, s, pA.GetNamespace(), pA.GetName())
	if err != nil {
		return
	}
	pASandboxName := objutil.CacheName(pASandbox)
	if pASandbox.Spec.NodeName != nA.GetName() {
		t.Errorf("in %q view, expected pod to be bound to node %q, got %q", s.GetName(), nA.GetName(), pASandbox.Spec.NodeName)
	}
	if pASandbox.Name != pA.GetName() && pASandbox.Namespace != pAModified.GetNamespace() {
		t.Errorf("in %q view, expected pod %q,  got %q", b.GetName(), pASandboxName, pAModName)
	}

	pABase, err := getPod(t, b, pA.GetNamespace(), pA.GetName())
	if pABase != nil {
		t.Errorf("in %q view, expected pod %q to not exist, got %q", b.GetName(), pAModName, pABase.GetName())
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("in %q view, expected pod %q to not exist, got %v", b.GetName(), pAModName, err)
	}
	if b.GetObjectChangeCount() != initialBaseChangeCount {
		t.Errorf("expected base view to not have changed, want %d, got %d", initialBaseChangeCount, b.GetObjectChangeCount())
	}
}

func TestSandboxPodBaseNodeBinding(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nA := testNodes[0]
	err = storeNode(t, b, &nA) //store node in base
	if err != nil {
		return
	}
	baseChangeCountAfterNodeStore := b.GetObjectChangeCount()
	pA := testPods[0]
	err = storePod(t, s, &pA) // store pod in sandbox
	if err != nil {
		return
	}
	pAModified, err := updateBinding(t, s, &pA, &nA) // update pod-node binding via sandbox view
	if err != nil {
		return
	}
	pAModName := objutil.CacheName(pAModified)

	if pAModified.Spec.NodeName != nA.GetName() {
		t.Errorf("in %q view, expected pod to be bound to node %q, got %q", b.GetName(), nA.GetName(), pAModified.Spec.NodeName)
	}
	pASandbox, err := getPod(t, s, pA.GetNamespace(), pA.GetName())
	if err != nil {
		return
	}
	pASandboxName := objutil.CacheName(pASandbox)
	if pASandbox.Spec.NodeName != nA.GetName() {
		t.Errorf("in %q view, expected pod to be bound to node %q, got %q", s.GetName(), nA.GetName(), pASandbox.Spec.NodeName)
	}
	if pASandbox.Name != pA.GetName() && pASandbox.Namespace != pAModified.GetNamespace() {
		t.Errorf("In %q view, expected pod %q,  got %q", b.GetName(), pASandboxName, pAModName)
	}

	pABase, err := getPod(t, b, pA.GetNamespace(), pA.GetName())
	if err == nil {
		t.Fatalf("in %q view, expected pod %q to not exist, got no error", b.GetName(), pAModName)
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("in %q view, expected pod %q to not exist, got %v", b.GetName(), pAModName, err)
		return
	}
	if pABase != nil {
		t.Fatalf("in %q view, expected pod %q to not exist, got %q", b.GetName(), pAModName, pABase)
		return
	}
	if b.GetObjectChangeCount() != baseChangeCountAfterNodeStore {
		t.Errorf("expected base view to not have changed, want %d, got %d", baseChangeCountAfterNodeStore, b.GetObjectChangeCount())
	}
}

func TestBasePodSandboxNodeBinding(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nA := testNodes[0]
	err = storeNode(t, s, &nA) //store node in the sandbox view
	if err != nil {
		return
	}
	pA := testPods[0]
	err = storePod(t, b, &pA) // store pod in the base view
	if err != nil {
		return
	}
	baseChangeCountAfterPodStore := b.GetObjectChangeCount()

	pAUpdated, err := updateBinding(t, s, &pA, &nA) // update pod-node binding in the  sandbox view
	if err != nil {
		return
	}
	pAUpdatedName := objutil.CacheName(pAUpdated)

	if pAUpdated.Spec.NodeName != nA.GetName() {
		t.Errorf("in %q view, expected pod to be bound to node %q, got %q", s.GetName(), nA.GetName(), pAUpdated.Spec.NodeName)
	}
	pASandbox, err := getPod(t, s, pA.GetNamespace(), pA.GetName())
	if err != nil {
		return
	}
	pASandboxName := objutil.CacheName(pASandbox)
	if pASandbox.Spec.NodeName != nA.GetName() {
		t.Errorf("in %q view, expected pod to be bound to node %q, got %q", s.GetName(), nA.GetName(), pASandbox.Spec.NodeName)
	}
	if pAUpdatedName != pASandboxName {
		t.Errorf("In %q view, expected pod %q,  got %q", b.GetName(), pAUpdatedName, pASandboxName)
	}

	pABase, err := getPod(t, b, pA.GetNamespace(), pA.GetName())
	if err != nil {
		t.Errorf("in %q view, expected pod %q to exist, got %v", b.GetName(), pAUpdatedName, err)
		return
	}
	if pABase.Spec.NodeName != "" {
		t.Errorf("in %q view, expected pod %q to not be bound to a node, got %q", b.GetName(), pAUpdatedName, pABase.Spec.NodeName)
	}
	if b.GetObjectChangeCount() != baseChangeCountAfterPodStore {
		t.Errorf("expected base view to not have changed, want %d, got %d", baseChangeCountAfterPodStore, b.GetObjectChangeCount())
	}

}
func setup(t *testing.T) (b api.View, s api.View, err error) {
	t.Helper()
	err = loadTestNodes(t)
	if err != nil {
		return
	}
	err = loadTestPods(t)
	if err != nil {
		return
	}
	b, s, err = createViews(t)
	t.Cleanup(func() {
		err = s.Close()
		if err != nil {
			t.Logf("failed to close sandbox view: %v", err)
		}
		err = b.Close()
		if err != nil {
			t.Logf("failed to close base view: %v", err)
		}
	})
	return
}

func loadTestNodes(t *testing.T) error {
	t.Helper()
	var err error
	if testNodes != nil {
		return nil
	}
	testNodes, err = testutil.LoadTestNodes()
	if err != nil {
		t.Fatalf("failed to load test nodes: %v", err)
		return err
	}
	return nil
}

func loadTestPods(t *testing.T) error {
	t.Helper()
	var err error
	if testPods != nil {
		return nil
	}
	testPods, err = testutil.LoadTestPods()
	if err != nil {
		t.Fatalf("failed to load test pods: %v", err)
		return err
	}
	return nil
}

func createViews(t *testing.T) (b api.View, s api.View, err error) {
	t.Helper()
	b, err = New(log, &baseViewArgs)
	if err != nil {
		t.Fatalf("failed to create base view: %v", err)
		return
	}
	s, err = NewSandbox(log, b, &sandboxViewArgs)
	if err != nil {
		t.Fatalf("failed to create sandbox view: %v", err)
		return
	}
	return
}

func updateBinding(t *testing.T, v api.View, p *corev1.Pod, n *corev1.Node) (*corev1.Pod, error) {
	t.Helper()
	binding := createBinding(p, n)
	pMod, err := v.UpdatePodNodeBinding(objutil.CacheName(p), binding)
	if err != nil {
		t.Fatalf("failed to update pod node binding: %v", err)
		return nil, err
	}
	return pMod, nil
}

func createBinding(p *corev1.Pod, n *corev1.Node) corev1.Binding {
	return corev1.Binding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Binding",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.GetName(),
			Namespace: p.GetNamespace(),
			UID:       p.GetUID(),
		},
		Target: corev1.ObjectReference{
			Kind: "Node",
			Name: n.GetName(),
		},
	}
}

func storePod(t *testing.T, v api.View, p *corev1.Pod) error {
	t.Helper()
	err := v.StoreObject(typeinfo.PodsDescriptor.GVK, p)
	if err != nil {
		t.Fatalf("failed to store pod: %v", err)
		return err
	}
	return nil
}

func storeNode(t *testing.T, v api.View, n *corev1.Node) error {
	t.Helper()
	err := v.StoreObject(typeinfo.NodesDescriptor.GVK, n)
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", v.GetName(), err)
		return err
	}
	return nil
}

func getNode(t *testing.T, v api.View, name string) (n *corev1.Node, err error) {
	t.Helper()
	o, err := v.GetObject(typeinfo.NodesDescriptor.GVK, cache.NewObjectName("", name))
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, err
		}
		t.Fatalf("from view %q, failed to get node: %v", v.GetName(), err)
		return
	}
	n, ok := o.(*corev1.Node)
	if !ok {
		err = fmt.Errorf("expected Node, got %T", o)
		t.Fatalf("from view %q, failed to get node: %v", v.GetName(), err)
	}
	return
}

func getPod(t *testing.T, v api.View, namespace, name string) (p *corev1.Pod, err error) {
	t.Helper()
	o, err := v.GetObject(typeinfo.PodsDescriptor.GVK, cache.NewObjectName(namespace, name))
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, err
		}
		t.Fatalf("failed to get Pod: %v", err)
		return
	}
	p, ok := o.(*corev1.Pod)
	if !ok {
		err = fmt.Errorf("expected Pod, got %T", o)
		t.Fatalf("failed to get Pod: %v", err)
	}
	return
}

func checkNodeInViewIsSame(t *testing.T, v mkapi.View, n *corev1.Node) (diff string) {
	t.Helper()
	nAFromView, err := getNode(t, v, n.GetName())
	if err != nil {
		t.Fatalf("From view %q, failed to get node: %v", v.GetName(), err)
		return
	}
	if n.GetName() != nAFromView.GetName() {
		t.Errorf("From view %q, expected node %q, got %q", v.GetName(), n.GetName(), nAFromView.GetName())
	}
	diff = gocmp.Diff(n, nAFromView)
	if diff != "" {
		t.Errorf("From view %q, expected node spec for %q to be same, got diff: %s", v.GetName(), n.GetName(), diff)
	}
	return diff
}
