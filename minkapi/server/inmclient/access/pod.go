package access

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/common/podutil"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/applyconfigurations/core/v1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

var (
	_ clientcorev1.PodInterface = (*podAccess)(nil)
)

type podAccess struct {
	BasicResourceAccess[*corev1.Pod, *corev1.PodList]
}

func NewPodAccess(view mkapi.View, namespace string) clientcorev1.PodInterface {
	return &podAccess{
		BasicResourceAccess[*corev1.Pod, *corev1.PodList]{
			view:            view,
			gvk:             typeinfo.PodsDescriptor.GVK,
			Namespace:       namespace,
			ResourcePtr:     &corev1.Pod{},
			ResourceListPtr: &corev1.PodList{},
		},
	}
}

func (a *podAccess) Create(ctx context.Context, pod *corev1.Pod, opts metav1.CreateOptions) (*corev1.Pod, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, pod)
}

func (a *podAccess) Update(ctx context.Context, pod *corev1.Pod, opts metav1.UpdateOptions) (*corev1.Pod, error) {
	return a.updateObject(ctx, opts, pod)
}

func (a *podAccess) UpdateStatus(ctx context.Context, pod *corev1.Pod, opts metav1.UpdateOptions) (*corev1.Pod, error) {
	return a.updateObject(ctx, opts, pod)
}

func (a *podAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *podAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *podAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.Pod, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *podAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.PodList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *podAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *podAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *corev1.Pod, err error) {
	if len(subresources) > 0 {
		if subresources[0] != "status" {
			return nil, fmt.Errorf("%w: patch of subresources %q is invalid for Pod", commonerrors.ErrInvalidOptVal, subresources)
		}
		return a.patchObjectStatus(ctx, name, data)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *podAccess) Apply(ctx context.Context, pod *v1.PodApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.Pod, err error) {
	return nil, fmt.Errorf("%w: Apply of %q is not supported", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *podAccess) ApplyStatus(ctx context.Context, pod *v1.PodApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.Pod, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus of %q is not supported", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *podAccess) UpdateEphemeralContainers(ctx context.Context, podName string, pod *corev1.Pod, opts metav1.UpdateOptions) (*corev1.Pod, error) {
	if opts.DryRun != nil {
		return nil, fmt.Errorf("%w: dry run not implemented for %T.UpdateEphemeralContainers", commonerrors.ErrUnimplemented, pod)
	}
	p, err := a.Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	updatedPod := p.DeepCopy()
	updatedPod.Spec.EphemeralContainers = pod.Spec.EphemeralContainers
	return a.Update(ctx, updatedPod, opts)
}

func (a *podAccess) UpdateResize(ctx context.Context, podName string, pod *corev1.Pod, opts metav1.UpdateOptions) (*corev1.Pod, error) {
	if opts.DryRun != nil {
		return nil, fmt.Errorf("%w: dry run not implemented for %T.UpdateResize", commonerrors.ErrUnimplemented, pod)
	}
	p, err := a.Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	updatedPod := p.DeepCopy()
	podutil.UpdatePodResources(updatedPod, pod)
	return a.Update(ctx, updatedPod, opts)
}

func (a *podAccess) Bind(ctx context.Context, binding *corev1.Binding, opts metav1.CreateOptions) error {
	podName := cache.NewObjectName(a.Namespace, binding.Name)
	_, err := a.view.UpdatePodNodeBinding(podName, *binding)
	return err
}

func (a *podAccess) Evict(ctx context.Context, eviction *policyv1beta1.Eviction) error {
	return fmt.Errorf("%w: Evict of %q is not supported", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *podAccess) EvictV1(ctx context.Context, eviction *policyv1.Eviction) error {
	return fmt.Errorf("%w: EvictV1 of %q is not supported", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *podAccess) EvictV1beta1(ctx context.Context, eviction *policyv1beta1.Eviction) error {
	return fmt.Errorf("%w: EvictV1beta1 of %q is not supported", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *podAccess) GetLogs(name string, opts *corev1.PodLogOptions) *rest.Request {
	panic(commonerrors.ErrUnimplemented)
}

func (a *podAccess) ProxyGet(scheme, name, port, path string, params map[string]string) rest.ResponseWrapper {
	panic(commonerrors.ErrUnimplemented)
}
