// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
	"context"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
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
	_ clientappsv1.ReplicaSetInterface = (*replicaSetAccessImpl)(nil)
)

type replicaSetAccessImpl struct {
	resourceAccessImpl
}

func (r replicaSetAccessImpl) Create(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.CreateOptions) (*appsv1.ReplicaSet, error) {
	return createObject[*appsv1.ReplicaSet](ctx, r.view, r.gvk, opts, replicaSet)
}

func (r replicaSetAccessImpl) Update(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.UpdateOptions) (*appsv1.ReplicaSet, error) {
	panic(commonerrors.ErrUnimplemented)
}

func (r replicaSetAccessImpl) UpdateStatus(ctx context.Context, replicaSet *appsv1.ReplicaSet, opts metav1.UpdateOptions) (*appsv1.ReplicaSet, error) {
	panic(commonerrors.ErrUnimplemented)
}

func (r replicaSetAccessImpl) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	panic(commonerrors.ErrUnimplemented)
}

func (r replicaSetAccessImpl) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	panic(commonerrors.ErrUnimplemented)
}

func (r replicaSetAccessImpl) Get(ctx context.Context, name string, opts metav1.GetOptions) (*appsv1.ReplicaSet, error) {
	return getObject[*appsv1.ReplicaSet](ctx, r.view, r.gvk, r.namespace, name)
}

func (r replicaSetAccessImpl) List(ctx context.Context, opts metav1.ListOptions) (*appsv1.ReplicaSetList, error) {
	return getObjectList[*appsv1.ReplicaSetList](ctx, r.view, r.gvk, r.namespace, opts)
}

func (r replicaSetAccessImpl) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	//TODO implement me
	panic("implement me")
}

func (r replicaSetAccessImpl) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *appsv1.ReplicaSet, err error) {
	//TODO implement me
	panic("implement me")
}

func (r replicaSetAccessImpl) Apply(ctx context.Context, replicaSet *clientapplyconfigurationsappsv1.ReplicaSetApplyConfiguration, opts metav1.ApplyOptions) (result *appsv1.ReplicaSet, err error) {
	//TODO implement me
	panic("implement me")
}

func (r replicaSetAccessImpl) ApplyStatus(ctx context.Context, replicaSet *clientapplyconfigurationsappsv1.ReplicaSetApplyConfiguration, opts metav1.ApplyOptions) (result *appsv1.ReplicaSet, err error) {
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
