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
	applyconfigv1 "k8s.io/client-go/applyconfigurations/core/v1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	_ clientcorev1.PersistentVolumeClaimInterface = (*pvcAccess)(nil)
)

type pvcAccess struct {
	access.GenericResourceAccess[*corev1.PersistentVolumeClaim, *corev1.PersistentVolumeClaimList]
}

// NewPersistentVolumeClaimAccess creates a new access facade for managing PersistentVolumeClaim resources within a specific namespace using the given minkapi View.
func NewPersistentVolumeClaimAccess(view mkapi.View, namespace string) clientcorev1.PersistentVolumeClaimInterface {
	return &pvcAccess{
		access.GenericResourceAccess[*corev1.PersistentVolumeClaim, *corev1.PersistentVolumeClaimList]{
			View:      view,
			GVK:       typeinfo.PersistentVolumeClaimsDescriptor.GVK,
			Namespace: namespace,
		},
	}
}

func (a *pvcAccess) Create(ctx context.Context, pvc *corev1.PersistentVolumeClaim, opts metav1.CreateOptions) (*corev1.PersistentVolumeClaim, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, pvc)
}

func (a *pvcAccess) CreateWithPersistentVolumeClaimNamespaceWithContext(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	return a.CreateObject(ctx, metav1.CreateOptions{}, pvc)
}

func (a *pvcAccess) CreateWithPersistentVolumeClaimNamespace(pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	return a.CreateObject(context.Background(), metav1.CreateOptions{}, pvc)
}

func (a *pvcAccess) Update(ctx context.Context, pvc *corev1.PersistentVolumeClaim, opts metav1.UpdateOptions) (*corev1.PersistentVolumeClaim, error) {
	return a.UpdateObject(ctx, opts, pvc)
}

func (a *pvcAccess) UpdateStatus(ctx context.Context, pvc *corev1.PersistentVolumeClaim, opts metav1.UpdateOptions) (*corev1.PersistentVolumeClaim, error) {
	return a.UpdateObject(ctx, opts, pvc)
}

func (a *pvcAccess) UpdateWithPersistentVolumeClaimNamespaceWithContext(_ context.Context, _ *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	return nil, fmt.Errorf("%w: update of pvc with namespace is not supported", commonerrors.ErrUnimplemented)
}

func (a *pvcAccess) UpdateWithPersistentVolumeClaimNamespace(_ *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	return nil, fmt.Errorf("%w: update of pvc with namespace is not supported", commonerrors.ErrUnimplemented)
}

func (a *pvcAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *pvcAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *pvcAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.PersistentVolumeClaim, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *pvcAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.PersistentVolumeClaimList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *pvcAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *pvcAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *corev1.PersistentVolumeClaim, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *pvcAccess) PatchWithPersistentVolumeClaimNamespace(_ *corev1.PersistentVolumeClaim, _ []byte) (*corev1.PersistentVolumeClaim, error) {
	return nil, fmt.Errorf("%w: patch of pvc with namespace is not supported", commonerrors.ErrUnimplemented)
}

func (a *pvcAccess) PatchWithPersistentVolumeClaimNamespaceWithContext(_ context.Context, _ *corev1.PersistentVolumeClaim, _ []byte) (*corev1.PersistentVolumeClaim, error) {
	return nil, fmt.Errorf("%w: patch of pvc with namespace is not supported", commonerrors.ErrUnimplemented)
}

func (a *pvcAccess) Apply(_ context.Context, _ *applyconfigv1.PersistentVolumeClaimApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.PersistentVolumeClaim, err error) {
	return nil, fmt.Errorf("%w: apply of pvc is not supported", commonerrors.ErrUnimplemented)
}

func (a *pvcAccess) ApplyStatus(_ context.Context, _ *applyconfigv1.PersistentVolumeClaimApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.PersistentVolumeClaim, err error) {
	return nil, fmt.Errorf("%w: apply of pvc is not supported", commonerrors.ErrUnimplemented)
}
