package access

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/applyconfigurations/core/v1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	_ clientcorev1.PersistentVolumeInterface = (*pvAccess)(nil)
)

type pvAccess struct {
	BasicResourceAccess[*corev1.PersistentVolume, *corev1.PersistentVolumeList]
}

func NewPersistentVolumeAccess(view mkapi.View) clientcorev1.PersistentVolumeInterface {
	return &pvAccess{
		BasicResourceAccess[*corev1.PersistentVolume, *corev1.PersistentVolumeList]{
			view:            view,
			gvk:             typeinfo.PersistentVolumesDescriptor.GVK,
			Namespace:       metav1.NamespaceNone,
			ResourcePtr:     &corev1.PersistentVolume{},
			ResourceListPtr: &corev1.PersistentVolumeList{},
		},
	}
}

func (a *pvAccess) Create(ctx context.Context, pv *corev1.PersistentVolume, opts metav1.CreateOptions) (*corev1.PersistentVolume, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, pv)
}

func (a *pvAccess) Update(ctx context.Context, pv *corev1.PersistentVolume, opts metav1.UpdateOptions) (*corev1.PersistentVolume, error) {
	return a.updateObject(ctx, opts, pv)
}

func (a *pvAccess) UpdateStatus(ctx context.Context, pv *corev1.PersistentVolume, opts metav1.UpdateOptions) (*corev1.PersistentVolume, error) {
	return a.updateObject(ctx, opts, pv)
}

func (a *pvAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *pvAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *pvAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.PersistentVolume, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *pvAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.PersistentVolumeList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *pvAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *pvAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *corev1.PersistentVolume, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for pvs", commonerrors.ErrInvalidOptVal, subresources)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *pvAccess) Apply(ctx context.Context, pv *v1.PersistentVolumeApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.PersistentVolume, err error) {
	return nil, fmt.Errorf("%w: apply of %q is not supported", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *pvAccess) ApplyStatus(ctx context.Context, pv *v1.PersistentVolumeApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.PersistentVolume, err error) {
	return nil, fmt.Errorf("%w: apply of %q is not supported", commonerrors.ErrUnimplemented, a.gvk.Kind)
}
