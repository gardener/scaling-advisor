package coreaccess

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	applyconfigv1 "k8s.io/client-go/applyconfigurations/core/v1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

var (
	_ clientcorev1.PodInterface = (*podAccess)(nil)
)

type podAccess struct {
	access.GenericResourceAccess[*corev1.Pod, *corev1.PodList]
}

// NewPodAccess creates a new access facade for managing Pod resources within a specific namespace using the given minkapi View.
func NewPodAccess(view mkapi.View, namespace string) clientcorev1.PodInterface {
	return &podAccess{
		access.GenericResourceAccess[*corev1.Pod, *corev1.PodList]{
			View:      view,
			GVK:       typeinfo.PodsDescriptor.GVK,
			Namespace: namespace,
		},
	}
}

func (a *podAccess) Create(ctx context.Context, pod *corev1.Pod, opts metav1.CreateOptions) (*corev1.Pod, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, pod)
}

func (a *podAccess) Update(ctx context.Context, pod *corev1.Pod, opts metav1.UpdateOptions) (*corev1.Pod, error) {
	return a.UpdateObject(ctx, opts, pod)
}

func (a *podAccess) UpdateStatus(ctx context.Context, pod *corev1.Pod, opts metav1.UpdateOptions) (*corev1.Pod, error) {
	return a.UpdateObject(ctx, opts, pod)
}

func (a *podAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *podAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *podAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.Pod, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *podAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.PodList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *podAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *podAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *corev1.Pod, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *podAccess) Apply(_ context.Context, _ *applyconfigv1.PodApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.Pod, err error) {
	return nil, fmt.Errorf("%w: Apply of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *podAccess) ApplyStatus(_ context.Context, _ *applyconfigv1.PodApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.Pod, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *podAccess) UpdateEphemeralContainers(ctx context.Context, podName string, pod *corev1.Pod, opts metav1.UpdateOptions) (*corev1.Pod, error) {
	p, err := a.Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	updatedPod := p.DeepCopy()
	updatedPod.Spec.EphemeralContainers = pod.Spec.EphemeralContainers
	return a.Update(ctx, updatedPod, opts)
}

func (a *podAccess) UpdateResize(_ context.Context, _ string, _ *corev1.Pod, _ metav1.UpdateOptions) (*corev1.Pod, error) {
	return nil, fmt.Errorf("%w: UpdateResize of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *podAccess) Bind(ctx context.Context, binding *corev1.Binding, _ metav1.CreateOptions) error {
	podName := cache.NewObjectName(a.Namespace, binding.Name)
	_, err := a.View.UpdatePodNodeBinding(ctx, podName, *binding)
	return err
}

func (a *podAccess) Evict(_ context.Context, _ *policyv1beta1.Eviction) error {
	return fmt.Errorf("%w: Evict of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *podAccess) EvictV1(_ context.Context, _ *policyv1.Eviction) error {
	return fmt.Errorf("%w: EvictV1 of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *podAccess) EvictV1beta1(_ context.Context, _ *policyv1beta1.Eviction) error {
	return fmt.Errorf("%w: EvictV1beta1 of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *podAccess) GetLogs(_ string, _ *corev1.PodLogOptions) *rest.Request {
	panic(commonerrors.ErrUnimplemented)
}

func (a *podAccess) ProxyGet(_, _, _, _ string, _ map[string]string) rest.ResponseWrapper {
	panic(commonerrors.ErrUnimplemented)
}
