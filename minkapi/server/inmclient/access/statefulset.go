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
	_ clientappsv1.StatefulSetInterface = (*statefulSetAccess)(nil)
)

type statefulSetAccess struct {
	BasicResourceAccess[*appsv1.StatefulSet, *appsv1.StatefulSetList]
}

func NewStatefulSetAccess(view mkapi.View, namespace string) *statefulSetAccess {
	return &statefulSetAccess{
		BasicResourceAccess[*appsv1.StatefulSet, *appsv1.StatefulSetList]{
			view:            view,
			gvk:             typeinfo.StatefulSetDescriptor.GVK,
			Namespace:       namespace,
			ResourcePtr:     &appsv1.StatefulSet{},
			ResourceListPtr: &appsv1.StatefulSetList{},
		},
	}
}

func (r *statefulSetAccess) Create(ctx context.Context, replicaSet *appsv1.StatefulSet, opts metav1.CreateOptions) (*appsv1.StatefulSet, error) {
	return r.createObject(ctx, opts, replicaSet)
}

func (r *statefulSetAccess) Update(ctx context.Context, replicaSet *appsv1.StatefulSet, opts metav1.UpdateOptions) (*appsv1.StatefulSet, error) {
	return r.updateObject(ctx, opts, replicaSet)
}

func (r *statefulSetAccess) UpdateStatus(ctx context.Context, replicaSet *appsv1.StatefulSet, opts metav1.UpdateOptions) (*appsv1.StatefulSet, error) {
	return r.updateObject(ctx, opts, replicaSet)
}

func (r *statefulSetAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return r.deleteObject(ctx, opts, r.Namespace, name)
}

func (r *statefulSetAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return r.deleteObjectCollection(ctx, r.Namespace, opts, listOpts)
}

func (r *statefulSetAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*appsv1.StatefulSet, error) {
	return r.getObject(ctx, r.Namespace, name)
}

func (r *statefulSetAccess) List(ctx context.Context, opts metav1.ListOptions) (*appsv1.StatefulSetList, error) {
	return r.getObjectList(ctx, r.Namespace, opts)
}

func (r *statefulSetAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return r.getWatcher(ctx, r.Namespace, opts)
}

func (r *statefulSetAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *appsv1.StatefulSet, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (r *statefulSetAccess) Apply(ctx context.Context, replicaSet *clientapplyconfigurationsappsv1.StatefulSetApplyConfiguration, opts metav1.ApplyOptions) (result *appsv1.StatefulSet, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (r *statefulSetAccess) ApplyStatus(ctx context.Context, replicaSet *clientapplyconfigurationsappsv1.StatefulSetApplyConfiguration, opts metav1.ApplyOptions) (result *appsv1.StatefulSet, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (r *statefulSetAccess) GetScale(ctx context.Context, replicaSetName string, options metav1.GetOptions) (*autoscalingv1.Scale, error) {
	panic(commonerrors.ErrUnimplemented)
}

func (r *statefulSetAccess) UpdateScale(ctx context.Context, replicaSetName string, scale *autoscalingv1.Scale, opts metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	panic(commonerrors.ErrUnimplemented)
}

func (r *statefulSetAccess) ApplyScale(ctx context.Context, replicaSetName string, scale *clientapplyconfigurationsautoscalingv1.ScaleApplyConfiguration, opts metav1.ApplyOptions) (*autoscalingv1.Scale, error) {
	panic(commonerrors.ErrUnimplemented)
}
