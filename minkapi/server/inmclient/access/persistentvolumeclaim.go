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
	_ clientcorev1.PersistentVolumeClaimInterface = (*pvcAccess)(nil)
)

type pvcAccess struct {
	BasicResourceAccess[*corev1.PersistentVolumeClaim, *corev1.PersistentVolumeClaimList]
}

func NewPersistentVolumeClaimAccess(view mkapi.View, namespace string) clientcorev1.PersistentVolumeClaimInterface {
	return &pvcAccess{
		BasicResourceAccess[*corev1.PersistentVolumeClaim, *corev1.PersistentVolumeClaimList]{
			view:            view,
			gvk:             typeinfo.PersistentVolumeClaimsDescriptor.GVK,
			Namespace:       namespace,
			ResourcePtr:     &corev1.PersistentVolumeClaim{},
			ResourceListPtr: &corev1.PersistentVolumeClaimList{},
		},
	}
}

func (a *pvcAccess) Create(ctx context.Context, pvc *corev1.PersistentVolumeClaim, opts metav1.CreateOptions) (*corev1.PersistentVolumeClaim, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, pvc)
}

func (a *pvcAccess) CreateWithPersistentVolumeClaimNamespaceWithContext(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	return a.createObject(ctx, metav1.CreateOptions{}, pvc)
}

func (a *pvcAccess) CreateWithPersistentVolumeClaimNamespace(pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	return a.createObject(context.Background(), metav1.CreateOptions{}, pvc)
}

func (a *pvcAccess) Update(ctx context.Context, pvc *corev1.PersistentVolumeClaim, opts metav1.UpdateOptions) (*corev1.PersistentVolumeClaim, error) {
	return a.updateObject(ctx, opts, pvc)
}

func (a *pvcAccess) UpdateStatus(ctx context.Context, pvc *corev1.PersistentVolumeClaim, opts metav1.UpdateOptions) (*corev1.PersistentVolumeClaim, error) {
	return a.updateObject(ctx, opts, pvc)
}

func (a *pvcAccess) UpdateWithPersistentVolumeClaimNamespaceWithContext(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	return nil, fmt.Errorf("%w: update of pvc with namespace is not supported", commonerrors.ErrUnimplemented)
}

func (a *pvcAccess) UpdateWithPersistentVolumeClaimNamespace(pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	return nil, fmt.Errorf("%w: update of pvc with namespace is not supported", commonerrors.ErrUnimplemented)
}

func (a *pvcAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *pvcAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *pvcAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.PersistentVolumeClaim, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *pvcAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.PersistentVolumeClaimList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *pvcAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *pvcAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *corev1.PersistentVolumeClaim, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for pvcs", commonerrors.ErrInvalidOptVal, subresources)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *pvcAccess) PatchWithPersistentVolumeClaimNamespace(pvc *corev1.PersistentVolumeClaim, data []byte) (*corev1.PersistentVolumeClaim, error) {
	return nil, fmt.Errorf("%w: patch of pvc with namespace is not supported", commonerrors.ErrUnimplemented)
}

func (a *pvcAccess) PatchWithPersistentVolumeClaimNamespaceWithContext(ctx context.Context, pvc *corev1.PersistentVolumeClaim, data []byte) (*corev1.PersistentVolumeClaim, error) {
	return nil, fmt.Errorf("%w: patch of pvc with namespace is not supported", commonerrors.ErrUnimplemented)
}

func (a *pvcAccess) Apply(ctx context.Context, pvc *v1.PersistentVolumeClaimApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.PersistentVolumeClaim, err error) {
	return nil, fmt.Errorf("%w: apply of pvc is not supported", commonerrors.ErrUnimplemented)
}

func (a *pvcAccess) ApplyStatus(ctx context.Context, pvc *v1.PersistentVolumeClaimApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.PersistentVolumeClaim, err error) {
	return nil, fmt.Errorf("%w: apply of pvc is not supported", commonerrors.ErrUnimplemented)
}
