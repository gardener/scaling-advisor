package access

import (
	"context"
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/applyconfigurations/core/v1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	_ clientcorev1.ReplicationControllerInterface = (*replicationControllerAccess)(nil)
)

type replicationControllerAccess struct {
	BasicResourceAccess[*corev1.ReplicationController, *corev1.ReplicationControllerList]
}

func NewReplicationControllerAccess(view mkapi.View, namespace string) clientcorev1.ReplicationControllerInterface {
	return &replicationControllerAccess{
		BasicResourceAccess[*corev1.ReplicationController, *corev1.ReplicationControllerList]{
			view:            view,
			gvk:             typeinfo.ReplicationControllersDescriptor.GVK,
			Namespace:       namespace,
			ResourcePtr:     &corev1.ReplicationController{},
			ResourceListPtr: &corev1.ReplicationControllerList{},
		},
	}
}

func (a *replicationControllerAccess) Create(ctx context.Context, replicationController *corev1.ReplicationController, opts metav1.CreateOptions) (*corev1.ReplicationController, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, replicationController)
}

func (a *replicationControllerAccess) Update(ctx context.Context, replicationController *corev1.ReplicationController, opts metav1.UpdateOptions) (*corev1.ReplicationController, error) {
	return a.updateObject(ctx, opts, replicationController)
}

func (a *replicationControllerAccess) UpdateStatus(ctx context.Context, replicationController *corev1.ReplicationController, opts metav1.UpdateOptions) (*corev1.ReplicationController, error) {
	return a.updateObject(ctx, opts, replicationController)
}

func (a *replicationControllerAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *replicationControllerAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *replicationControllerAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.ReplicationController, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *replicationControllerAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.ReplicationControllerList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *replicationControllerAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *replicationControllerAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *corev1.ReplicationController, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for %q", commonerrors.ErrInvalidOptVal, subresources, a.gvk.Kind)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *replicationControllerAccess) Apply(ctx context.Context, replicationController *v1.ReplicationControllerApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.ReplicationController, err error) {
	return nil, fmt.Errorf("%w: Apply of %q is not supported", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *replicationControllerAccess) ApplyStatus(ctx context.Context, replicationController *v1.ReplicationControllerApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.ReplicationController, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus of %q is not supported", commonerrors.ErrUnimplemented, a.gvk.Kind)
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

func (a *replicationControllerAccess) UpdateScale(ctx context.Context, replicationControllerName string, scale *autoscalingv1.Scale, opts metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	return nil, fmt.Errorf("%w: UpdateScale of %q is not supported", commonerrors.ErrUnimplemented, a.gvk.Kind)
}
