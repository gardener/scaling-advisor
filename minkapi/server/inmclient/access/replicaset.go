package access

import (
	"context"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
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
func (r *replicaSetAccess) Create(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.CreateOptions) (*appsv1.ReplicaSet, error) {
	return r.createObject(ctx, opts, replicaSet)
}

func (r *replicaSetAccess) Update(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.UpdateOptions) (*appsv1.ReplicaSet, error) {
	return r.updateObject(ctx, opts, replicaSet)
}

func (r *replicaSetAccess) UpdateStatus(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.UpdateOptions) (*appsv1.ReplicaSet, error) {
	return r.updateObject(ctx, opts, replicaSet)
}

func (r *replicaSetAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return r.deleteObject(ctx, opts, r.Namespace, name)
}

func (r *replicaSetAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return r.deleteObjectCollection(ctx, r.Namespace, opts, listOpts)
}

func (r *replicaSetAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*appsv1.ReplicaSet, error) {
	return r.getObject(ctx, r.Namespace, name)
}

func (r *replicaSetAccess) List(ctx context.Context, opts metav1.ListOptions) (*appsv1.ReplicaSetList, error) {
	return r.getObjectList(ctx, r.Namespace, opts)
}

func (r *replicaSetAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return r.getWatcher(ctx, r.Namespace, opts)
}

func (r *replicaSetAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *appsv1.ReplicaSet, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (r *replicaSetAccess) Apply(ctx context.Context, replicaSet *clientapplyconfigurationsappsv1.ReplicaSetApplyConfiguration, opts metav1.ApplyOptions) (result *appsv1.ReplicaSet, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (r *replicaSetAccess) ApplyStatus(ctx context.Context, replicaSet *clientapplyconfigurationsappsv1.ReplicaSetApplyConfiguration, opts metav1.ApplyOptions) (result *appsv1.ReplicaSet, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (r *replicaSetAccess) GetScale(ctx context.Context, replicaSetName string, options metav1.GetOptions) (*autoscalingv1.Scale, error) {
	panic(commonerrors.ErrUnimplemented)
}

func (r *replicaSetAccess) UpdateScale(ctx context.Context, replicaSetName string, scale *autoscalingv1.Scale, opts metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	panic(commonerrors.ErrUnimplemented)
}

func (r *replicaSetAccess) ApplyScale(ctx context.Context, replicaSetName string, scale *clientapplyconfigurationsautoscalingv1.ScaleApplyConfiguration, opts metav1.ApplyOptions) (*autoscalingv1.Scale, error) {
	panic(commonerrors.ErrUnimplemented)
}
