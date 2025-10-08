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

func (a *statefulSetAccess) Create(ctx context.Context, statefulSet *appsv1.StatefulSet, opts metav1.CreateOptions) (*appsv1.StatefulSet, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, statefulSet)
}

func (a *statefulSetAccess) Update(ctx context.Context, statefulSet *appsv1.StatefulSet, opts metav1.UpdateOptions) (*appsv1.StatefulSet, error) {
	return a.updateObject(ctx, opts, statefulSet)
}

func (a *statefulSetAccess) UpdateStatus(ctx context.Context, statefulSet *appsv1.StatefulSet, opts metav1.UpdateOptions) (*appsv1.StatefulSet, error) {
	return a.updateObject(ctx, opts, statefulSet)
}

func (a *statefulSetAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *statefulSetAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *statefulSetAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*appsv1.StatefulSet, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *statefulSetAccess) List(ctx context.Context, opts metav1.ListOptions) (*appsv1.StatefulSetList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *statefulSetAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *statefulSetAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *appsv1.StatefulSet, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (a *statefulSetAccess) Apply(ctx context.Context, statefulSet *clientapplyconfigurationsappsv1.StatefulSetApplyConfiguration, opts metav1.ApplyOptions) (result *appsv1.StatefulSet, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (a *statefulSetAccess) ApplyStatus(ctx context.Context, statefulSet *clientapplyconfigurationsappsv1.StatefulSetApplyConfiguration, opts metav1.ApplyOptions) (result *appsv1.StatefulSet, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (a *statefulSetAccess) GetScale(ctx context.Context, statefulSetName string, options metav1.GetOptions) (*autoscalingv1.Scale, error) {
	panic(commonerrors.ErrUnimplemented)
}

func (a *statefulSetAccess) UpdateScale(ctx context.Context, statefulSetName string, scale *autoscalingv1.Scale, opts metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	panic(commonerrors.ErrUnimplemented)
}

func (a *statefulSetAccess) ApplyScale(ctx context.Context, statefulSetName string, scale *clientapplyconfigurationsautoscalingv1.ScaleApplyConfiguration, opts metav1.ApplyOptions) (*autoscalingv1.Scale, error) {
	panic(commonerrors.ErrUnimplemented)
}
