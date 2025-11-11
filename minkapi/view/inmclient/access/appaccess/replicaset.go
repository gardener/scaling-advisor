package appaccess

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	clientapplyconfigurationsappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	clientapplyconfigurationsautoscalingv1 "k8s.io/client-go/applyconfigurations/autoscaling/v1"
	clientappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
)

var (
	_ clientappsv1.ReplicaSetInterface = (*replicaSetAccess)(nil)
)

type replicaSetAccess struct {
	access.GenericResourceAccess[*appsv1.ReplicaSet, *appsv1.ReplicaSetList]
}

// NewReplicaSetAccess creates a new access facade for managing ReplicaSet resources within a specific namespace using the given minkapi View.
func NewReplicaSetAccess(view mkapi.View, namespace string) clientappsv1.ReplicaSetInterface {
	return &replicaSetAccess{
		access.GenericResourceAccess[*appsv1.ReplicaSet, *appsv1.ReplicaSetList]{
			View:      view,
			GVK:       typeinfo.ReplicaSetDescriptor.GVK,
			Namespace: namespace,
		},
	}
}

func (a *replicaSetAccess) Create(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.CreateOptions) (*appsv1.ReplicaSet, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, replicaSet)
}

func (a *replicaSetAccess) Update(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.UpdateOptions) (*appsv1.ReplicaSet, error) {
	return a.UpdateObject(ctx, opts, replicaSet)
}

func (a *replicaSetAccess) UpdateStatus(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.UpdateOptions) (*appsv1.ReplicaSet, error) {
	return a.UpdateObject(ctx, opts, replicaSet)
}

func (a *replicaSetAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *replicaSetAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *replicaSetAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*appsv1.ReplicaSet, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *replicaSetAccess) List(ctx context.Context, opts metav1.ListOptions) (*appsv1.ReplicaSetList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *replicaSetAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *replicaSetAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *appsv1.ReplicaSet, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *replicaSetAccess) Apply(_ context.Context, _ *clientapplyconfigurationsappsv1.ReplicaSetApplyConfiguration, _ metav1.ApplyOptions) (result *appsv1.ReplicaSet, err error) {
	return nil, fmt.Errorf("%w: Apply of %q is not implemented", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *replicaSetAccess) ApplyStatus(_ context.Context, _ *clientapplyconfigurationsappsv1.ReplicaSetApplyConfiguration, _ metav1.ApplyOptions) (result *appsv1.ReplicaSet, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus of %q is not implemented", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *replicaSetAccess) GetScale(_ context.Context, _ string, _ metav1.GetOptions) (*autoscalingv1.Scale, error) {
	return nil, fmt.Errorf("%w: GetScale of %q is not implemented", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *replicaSetAccess) UpdateScale(_ context.Context, _ string, _ *autoscalingv1.Scale, _ metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	return nil, fmt.Errorf("%w: UpdateScale of %q is not implemented", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *replicaSetAccess) ApplyScale(_ context.Context, _ string, _ *clientapplyconfigurationsautoscalingv1.ScaleApplyConfiguration, _ metav1.ApplyOptions) (*autoscalingv1.Scale, error) {
	return nil, fmt.Errorf("%w: ApplyScale of %q is not implemented", commonerrors.ErrUnimplemented, a.GVK.Kind)
}
