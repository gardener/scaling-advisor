package policyaccess

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

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
	access.GenericResourceAccess[*policyv1.PodDisruptionBudget, *policyv1.PodDisruptionBudgetList]
}

// NewPodDisruptionBudgetAccess creates a new access facade for managing PodDisruptionBudget resources within a specific namespace using the given minkapi View.
func NewPodDisruptionBudgetAccess(view mkapi.View, namespace string) clientpolicyv1.PodDisruptionBudgetInterface {
	return &podDisruptionBudgetAccess{
		access.GenericResourceAccess[*policyv1.PodDisruptionBudget, *policyv1.PodDisruptionBudgetList]{
			View:      view,
			GVK:       typeinfo.PodDisruptionBudgetDescriptor.GVK,
			Namespace: namespace,
		},
	}
}

func (a *podDisruptionBudgetAccess) Create(ctx context.Context, podDisruptionBudget *policyv1.PodDisruptionBudget, opts metav1.CreateOptions) (*policyv1.PodDisruptionBudget, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, podDisruptionBudget)
}

func (a *podDisruptionBudgetAccess) Update(ctx context.Context, podDisruptionBudget *policyv1.PodDisruptionBudget, opts metav1.UpdateOptions) (*policyv1.PodDisruptionBudget, error) {
	return a.UpdateObject(ctx, opts, podDisruptionBudget)
}

func (a *podDisruptionBudgetAccess) UpdateStatus(ctx context.Context, podDisruptionBudget *policyv1.PodDisruptionBudget, opts metav1.UpdateOptions) (*policyv1.PodDisruptionBudget, error) {
	return a.UpdateObject(ctx, opts, podDisruptionBudget)
}

func (a *podDisruptionBudgetAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *podDisruptionBudgetAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *podDisruptionBudgetAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*policyv1.PodDisruptionBudget, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *podDisruptionBudgetAccess) List(ctx context.Context, opts metav1.ListOptions) (*policyv1.PodDisruptionBudgetList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *podDisruptionBudgetAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *podDisruptionBudgetAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *policyv1.PodDisruptionBudget, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *podDisruptionBudgetAccess) Apply(_ context.Context, _ *v1.PodDisruptionBudgetApplyConfiguration, _ metav1.ApplyOptions) (result *policyv1.PodDisruptionBudget, err error) {
	return nil, fmt.Errorf("%w: Apply is not implemented for %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *podDisruptionBudgetAccess) ApplyStatus(_ context.Context, _ *v1.PodDisruptionBudgetApplyConfiguration, _ metav1.ApplyOptions) (result *policyv1.PodDisruptionBudget, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus is not implemented for %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
}
