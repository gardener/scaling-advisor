package inmclient

import (
	"context"
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	clientapplyconfigurationsappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	clientapplyconfigurationsautoscalingv1 "k8s.io/client-go/applyconfigurations/autoscaling/v1"
	clientappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/client-go/tools/cache"
)

var (
	_ clientappsv1.ReplicaSetInterface = (*replicaSetAccessImpl)(nil)
)

type replicaSetAccessImpl struct {
	resourceAccessImpl
}

func (r replicaSetAccessImpl) Create(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.CreateOptions) (*v1.ReplicaSet, error) {
	if opts.DryRun != nil {
		return nil, fmt.Errorf("%w: dry run not implemented for ReplicaSet.Create", commonerrors.ErrUnImplemented)
	}
	err := r.view.CreateObject(r.gvk, replicaSet)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, replicaSet.Name, metav1.GetOptions{})
}

func (r replicaSetAccessImpl) Update(ctx context.Context, replicaSet *v1.ReplicaSet, opts metav1.UpdateOptions) (*v1.ReplicaSet, error) {
	panic(commonerrors.ErrUnImplemented)
}

func (r replicaSetAccessImpl) UpdateStatus(ctx context.Context, replicaSet *v1.ReplicaSet, opts metav1.UpdateOptions) (*v1.ReplicaSet, error) {
	panic(commonerrors.ErrUnImplemented)
}

func (r replicaSetAccessImpl) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	panic(commonerrors.ErrUnImplemented)
}

func (r replicaSetAccessImpl) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	panic(commonerrors.ErrUnImplemented)
}

func (r replicaSetAccessImpl) Get(_ context.Context, name string, opts metav1.GetOptions) (*v1.ReplicaSet, error) {
	objName := cache.NewObjectName(r.namespace, name)
	obj, err := r.view.GetObject(r.gvk, objName)
	if err != nil {
		return nil, err
	}
	return obj.(*v1.ReplicaSet), nil
}

func (r replicaSetAccessImpl) List(_ context.Context, opts metav1.ListOptions) (*v1.ReplicaSetList, error) {
	// TODO: map to MatchCriteria correctly
	r.view.ListMetaObjects(r.gvk, mkapi.MatchCriteria{
		LabelSelector: opts.LabelSelector,
	})
	//TODO implement me
	panic("implement me")
}

func (r replicaSetAccessImpl) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	//TODO implement me
	panic("implement me")
}

func (r replicaSetAccessImpl) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.ReplicaSet, err error) {
	//TODO implement me
	panic("implement me")
}

func (r replicaSetAccessImpl) Apply(ctx context.Context, replicaSet *clientapplyconfigurationsappsv1.ReplicaSetApplyConfiguration, opts metav1.ApplyOptions) (result *v1.ReplicaSet, err error) {
	//TODO implement me
	panic("implement me")
}

func (r replicaSetAccessImpl) ApplyStatus(ctx context.Context, replicaSet *clientapplyconfigurationsappsv1.ReplicaSetApplyConfiguration, opts metav1.ApplyOptions) (result *v1.ReplicaSet, err error) {
	//TODO implement me
	panic("implement me")
}

func (r replicaSetAccessImpl) GetScale(ctx context.Context, replicaSetName string, options metav1.GetOptions) (*autoscalingv1.Scale, error) {
	//TODO implement me
	panic("implement me")
}

func (r replicaSetAccessImpl) UpdateScale(ctx context.Context, replicaSetName string, scale *autoscalingv1.Scale, opts metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	//TODO implement me
	panic("implement me")
}

func (r replicaSetAccessImpl) ApplyScale(ctx context.Context, replicaSetName string, scale *clientapplyconfigurationsautoscalingv1.ScaleApplyConfiguration, opts metav1.ApplyOptions) (*autoscalingv1.Scale, error) {
	//TODO implement me
	panic("implement me")
}
