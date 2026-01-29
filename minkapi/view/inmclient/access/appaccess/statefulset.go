// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package appaccess

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"github.com/gardener/scaling-advisor/api/minkapi"
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
	access.GenericResourceAccess[*appsv1.StatefulSet, *appsv1.StatefulSetList]
}

// NewStatefulSetAccess creates a new access facade for managing StatefulSet resources within a specific namespace using the given minkapi View.
func NewStatefulSetAccess(view minkapi.View, namespace string) clientappsv1.StatefulSetInterface {
	return &statefulSetAccess{
		access.GenericResourceAccess[*appsv1.StatefulSet, *appsv1.StatefulSetList]{
			View:      view,
			GVK:       typeinfo.StatefulSetDescriptor.GVK,
			Namespace: namespace,
		},
	}
}

func (a *statefulSetAccess) Create(ctx context.Context, statefulSet *appsv1.StatefulSet, opts metav1.CreateOptions) (*appsv1.StatefulSet, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, statefulSet)
}

func (a *statefulSetAccess) Update(ctx context.Context, statefulSet *appsv1.StatefulSet, opts metav1.UpdateOptions) (*appsv1.StatefulSet, error) {
	return a.UpdateObject(ctx, opts, statefulSet)
}

func (a *statefulSetAccess) UpdateStatus(ctx context.Context, statefulSet *appsv1.StatefulSet, opts metav1.UpdateOptions) (*appsv1.StatefulSet, error) {
	return a.UpdateObject(ctx, opts, statefulSet)
}

func (a *statefulSetAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *statefulSetAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *statefulSetAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*appsv1.StatefulSet, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *statefulSetAccess) List(ctx context.Context, opts metav1.ListOptions) (*appsv1.StatefulSetList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *statefulSetAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *statefulSetAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *appsv1.StatefulSet, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *statefulSetAccess) Apply(_ context.Context, _ *clientapplyconfigurationsappsv1.StatefulSetApplyConfiguration, _ metav1.ApplyOptions) (result *appsv1.StatefulSet, err error) {
	return nil, fmt.Errorf("%w: Apply of %q is not implemented", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *statefulSetAccess) ApplyStatus(_ context.Context, _ *clientapplyconfigurationsappsv1.StatefulSetApplyConfiguration, _ metav1.ApplyOptions) (result *appsv1.StatefulSet, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus of %q is not implemented", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *statefulSetAccess) GetScale(_ context.Context, _ string, _ metav1.GetOptions) (*autoscalingv1.Scale, error) {
	return nil, fmt.Errorf("%w: GetScale of %q is not implemented", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *statefulSetAccess) UpdateScale(_ context.Context, _ string, _ *autoscalingv1.Scale, _ metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	return nil, fmt.Errorf("%w: UpdateScale of %q is not implemented", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *statefulSetAccess) ApplyScale(_ context.Context, _ string, _ *clientapplyconfigurationsautoscalingv1.ScaleApplyConfiguration, _ metav1.ApplyOptions) (*autoscalingv1.Scale, error) {
	return nil, fmt.Errorf("%w: ApplyScale of %q is not implemented", commonerrors.ErrUnimplemented, a.GVK.Kind)
}
