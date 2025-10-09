package access

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

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
	BasicResourceAccess[*appsv1.ReplicaSet, *appsv1.ReplicaSetList]
}

func NewReplicaSetAccess(view mkapi.View, namespace string) clientappsv1.ReplicaSetInterface {
	return &replicaSetAccess{
		BasicResourceAccess[*appsv1.ReplicaSet, *appsv1.ReplicaSetList]{
			view:            view,
			gvk:             typeinfo.ReplicaSetDescriptor.GVK,
			Namespace:       namespace,
			ResourcePtr:     &appsv1.ReplicaSet{},
			ResourceListPtr: &appsv1.ReplicaSetList{},
		},
	}
}

func (a *replicaSetAccess) Create(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.CreateOptions) (*appsv1.ReplicaSet, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, replicaSet)
}

func (a *replicaSetAccess) Update(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.UpdateOptions) (*appsv1.ReplicaSet, error) {
	return a.updateObject(ctx, opts, replicaSet)
}

func (a *replicaSetAccess) UpdateStatus(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.UpdateOptions) (*appsv1.ReplicaSet, error) {
	return a.updateObject(ctx, opts, replicaSet)
}

func (a *replicaSetAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *replicaSetAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *replicaSetAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*appsv1.ReplicaSet, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *replicaSetAccess) List(ctx context.Context, opts metav1.ListOptions) (*appsv1.ReplicaSetList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *replicaSetAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *replicaSetAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *appsv1.ReplicaSet, err error) {
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *replicaSetAccess) Apply(ctx context.Context, replicaSet *clientapplyconfigurationsappsv1.ReplicaSetApplyConfiguration, opts metav1.ApplyOptions) (result *appsv1.ReplicaSet, err error) {
	return nil, fmt.Errorf("%w: Apply of %q is not implemented", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *replicaSetAccess) ApplyStatus(ctx context.Context, replicaSet *clientapplyconfigurationsappsv1.ReplicaSetApplyConfiguration, opts metav1.ApplyOptions) (result *appsv1.ReplicaSet, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus of %q is not implemented", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *replicaSetAccess) GetScale(ctx context.Context, replicaSetName string, options metav1.GetOptions) (*autoscalingv1.Scale, error) {
	return nil, fmt.Errorf("%w: GetScale of %q is not implemented", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *replicaSetAccess) UpdateScale(ctx context.Context, replicaSetName string, scale *autoscalingv1.Scale, opts metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	return nil, fmt.Errorf("%w: UpdateScale of %q is not implemented", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *replicaSetAccess) ApplyScale(ctx context.Context, replicaSetName string, scale *clientapplyconfigurationsautoscalingv1.ScaleApplyConfiguration, opts metav1.ApplyOptions) (*autoscalingv1.Scale, error) {
	return nil, fmt.Errorf("%w: ApplyScale of %q is not implemented", commonerrors.ErrUnimplemented, a.gvk.Kind)
}
