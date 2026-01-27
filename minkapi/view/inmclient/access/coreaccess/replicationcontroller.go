// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package coreaccess

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"github.com/gardener/scaling-advisor/api/minkapi"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	applyconfigv1 "k8s.io/client-go/applyconfigurations/core/v1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	_ clientcorev1.ReplicationControllerInterface = (*replicationControllerAccess)(nil)
)

type replicationControllerAccess struct {
	access.GenericResourceAccess[*corev1.ReplicationController, *corev1.ReplicationControllerList]
}

// NewReplicationControllerAccess creates a new access facade for managing ReplicationController resources within a specific namespace using the given minkapi View.
func NewReplicationControllerAccess(view minkapi.View, namespace string) clientcorev1.ReplicationControllerInterface {
	return &replicationControllerAccess{
		access.GenericResourceAccess[*corev1.ReplicationController, *corev1.ReplicationControllerList]{
			View:      view,
			GVK:       typeinfo.ReplicationControllersDescriptor.GVK,
			Namespace: namespace,
		},
	}
}

func (a *replicationControllerAccess) Create(ctx context.Context, replicationController *corev1.ReplicationController, opts metav1.CreateOptions) (*corev1.ReplicationController, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, replicationController)
}

func (a *replicationControllerAccess) Update(ctx context.Context, replicationController *corev1.ReplicationController, opts metav1.UpdateOptions) (*corev1.ReplicationController, error) {
	return a.UpdateObject(ctx, opts, replicationController)
}

func (a *replicationControllerAccess) UpdateStatus(ctx context.Context, replicationController *corev1.ReplicationController, opts metav1.UpdateOptions) (*corev1.ReplicationController, error) {
	return a.UpdateObject(ctx, opts, replicationController)
}

func (a *replicationControllerAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *replicationControllerAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *replicationControllerAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.ReplicationController, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *replicationControllerAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.ReplicationControllerList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *replicationControllerAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *replicationControllerAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *corev1.ReplicationController, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *replicationControllerAccess) Apply(_ context.Context, _ *applyconfigv1.ReplicationControllerApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.ReplicationController, err error) {
	return nil, fmt.Errorf("%w: Apply of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *replicationControllerAccess) ApplyStatus(_ context.Context, _ *applyconfigv1.ReplicationControllerApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.ReplicationController, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *replicationControllerAccess) GetScale(ctx context.Context, replicationControllerName string, opts metav1.GetOptions) (*autoscalingv1.Scale, error) {
	rc, err := a.Get(ctx, replicationControllerName, opts)
	if err != nil {
		return nil, err
	}
	var selectorStr string
	if len(rc.Spec.Selector) > 0 {
		labelSelector := metav1.LabelSelector{
			MatchLabels: rc.Spec.Selector,
		}
		selectorStr = labelSelector.String()
	}
	scale := &autoscalingv1.Scale{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Scale",
			APIVersion: "autoscaling/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              rc.Name,
			Namespace:         rc.Namespace,
			ResourceVersion:   rc.ResourceVersion,
			CreationTimestamp: rc.CreationTimestamp,
			UID:               rc.UID,
		},
		Spec: autoscalingv1.ScaleSpec{
			Replicas: *rc.Spec.Replicas,
		},
		Status: autoscalingv1.ScaleStatus{
			Replicas: rc.Status.Replicas,
			Selector: selectorStr,
		},
	}
	return scale, nil
}

func (a *replicationControllerAccess) UpdateScale(_ context.Context, _ string, _ *autoscalingv1.Scale, _ metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	return nil, fmt.Errorf("%w: UpdateScale of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}
