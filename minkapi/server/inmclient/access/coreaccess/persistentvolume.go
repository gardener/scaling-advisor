package coreaccess

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/server/inmclient/access"
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
	access.GenericResourceAccess[*corev1.PersistentVolume, *corev1.PersistentVolumeList]
}

// NewPersistentVolumeAccess creates a PersistentVolume access facade for managing PersistentVolume resources using the given minkapi View.
func NewPersistentVolumeAccess(view mkapi.View) clientcorev1.PersistentVolumeInterface {
	return &pvAccess{
		access.GenericResourceAccess[*corev1.PersistentVolume, *corev1.PersistentVolumeList]{
			View:      view,
			GVK:       typeinfo.PersistentVolumesDescriptor.GVK,
			Namespace: metav1.NamespaceNone,
		},
	}
}

func (a *pvAccess) Create(ctx context.Context, pv *corev1.PersistentVolume, opts metav1.CreateOptions) (*corev1.PersistentVolume, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, pv)
}

func (a *pvAccess) Update(ctx context.Context, pv *corev1.PersistentVolume, opts metav1.UpdateOptions) (*corev1.PersistentVolume, error) {
	return a.UpdateObject(ctx, opts, pv)
}

func (a *pvAccess) UpdateStatus(ctx context.Context, pv *corev1.PersistentVolume, opts metav1.UpdateOptions) (*corev1.PersistentVolume, error) {
	return a.UpdateObject(ctx, opts, pv)
}

func (a *pvAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *pvAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *pvAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.PersistentVolume, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *pvAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.PersistentVolumeList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *pvAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *pvAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *corev1.PersistentVolume, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *pvAccess) Apply(_ context.Context, _ *v1.PersistentVolumeApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.PersistentVolume, err error) {
	return nil, fmt.Errorf("%w: apply of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *pvAccess) ApplyStatus(_ context.Context, _ *v1.PersistentVolumeApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.PersistentVolume, err error) {
	return nil, fmt.Errorf("%w: apply of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}
