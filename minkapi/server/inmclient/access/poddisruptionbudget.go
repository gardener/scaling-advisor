package access

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/applyconfigurations/policy/v1"
	clientpolicyv1 "k8s.io/client-go/kubernetes/typed/policy/v1"
)

var (
	_ clientpolicyv1.PodDisruptionBudgetInterface = (*podDisruptionBudgetAccess)(nil)
)

type podDisruptionBudgetAccess struct {
	BasicResourceAccess[*policyv1.PodDisruptionBudget, *policyv1.PodDisruptionBudgetList]
}

func NewPodDisruptionBudgetAccess(view mkapi.View, namespace string) clientpolicyv1.PodDisruptionBudgetInterface {
	return &podDisruptionBudgetAccess{
		BasicResourceAccess[*policyv1.PodDisruptionBudget, *policyv1.PodDisruptionBudgetList]{
			view:            view,
			gvk:             typeinfo.PodDisruptionBudgetDescriptor.GVK,
			Namespace:       namespace,
			ResourcePtr:     &policyv1.PodDisruptionBudget{},
			ResourceListPtr: &policyv1.PodDisruptionBudgetList{},
		},
	}
}

func (a *podDisruptionBudgetAccess) Create(ctx context.Context, podDisruptionBudget *policyv1.PodDisruptionBudget, opts metav1.CreateOptions) (*policyv1.PodDisruptionBudget, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, podDisruptionBudget)
}

func (a *podDisruptionBudgetAccess) Update(ctx context.Context, podDisruptionBudget *policyv1.PodDisruptionBudget, opts metav1.UpdateOptions) (*policyv1.PodDisruptionBudget, error) {
	return a.updateObject(ctx, opts, podDisruptionBudget)
}

func (a *podDisruptionBudgetAccess) UpdateStatus(ctx context.Context, podDisruptionBudget *policyv1.PodDisruptionBudget, opts metav1.UpdateOptions) (*policyv1.PodDisruptionBudget, error) {
	return a.updateObject(ctx, opts, podDisruptionBudget)
}

func (a *podDisruptionBudgetAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *podDisruptionBudgetAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *podDisruptionBudgetAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*policyv1.PodDisruptionBudget, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *podDisruptionBudgetAccess) List(ctx context.Context, opts metav1.ListOptions) (*policyv1.PodDisruptionBudgetList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *podDisruptionBudgetAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *podDisruptionBudgetAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *policyv1.PodDisruptionBudget, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for %q", commonerrors.ErrInvalidOptVal, subresources, a.gvk.Kind)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *podDisruptionBudgetAccess) Apply(ctx context.Context, podDisruptionBudget *v1.PodDisruptionBudgetApplyConfiguration, opts metav1.ApplyOptions) (result *policyv1.PodDisruptionBudget, err error) {
	return nil, fmt.Errorf("%w: Apply is not implemented for %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *podDisruptionBudgetAccess) ApplyStatus(ctx context.Context, podDisruptionBudget *v1.PodDisruptionBudgetApplyConfiguration, opts metav1.ApplyOptions) (result *policyv1.PodDisruptionBudget, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus is not implemented for %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
}
