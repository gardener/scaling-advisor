package access

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/applyconfigurations/storage/v1"
	clientstoragev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
)

var (
	_ clientstoragev1.StorageClassInterface = (*storageClassAccess)(nil)
)

type storageClassAccess struct {
	BasicResourceAccess[*storagev1.StorageClass, *storagev1.StorageClassList]
}

func NewStorageClassAccess(view mkapi.View) clientstoragev1.StorageClassInterface {
	return &storageClassAccess{
		BasicResourceAccess[*storagev1.StorageClass, *storagev1.StorageClassList]{
			view:            view,
			gvk:             typeinfo.StorageClassDescriptor.GVK,
			Namespace:       metav1.NamespaceNone,
			ResourcePtr:     &storagev1.StorageClass{},
			ResourceListPtr: &storagev1.StorageClassList{},
		},
	}
}

func (a *storageClassAccess) Create(ctx context.Context, storageClass *storagev1.StorageClass, opts metav1.CreateOptions) (*storagev1.StorageClass, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, storageClass)
}

func (a *storageClassAccess) Update(ctx context.Context, storageClass *storagev1.StorageClass, opts metav1.UpdateOptions) (*storagev1.StorageClass, error) {
	return a.updateObject(ctx, opts, storageClass)
}

func (a *storageClassAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *storageClassAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *storageClassAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*storagev1.StorageClass, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *storageClassAccess) List(ctx context.Context, opts metav1.ListOptions) (*storagev1.StorageClassList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *storageClassAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *storageClassAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *storagev1.StorageClass, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for storageClasss", commonerrors.ErrInvalidOptVal, subresources)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *storageClassAccess) Apply(ctx context.Context, storageClass *v1.StorageClassApplyConfiguration, opts metav1.ApplyOptions) (result *storagev1.StorageClass, err error) {
	return nil, fmt.Errorf("%w: Apply is not implemented for %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *storageClassAccess) ApplyStatus(ctx context.Context, storageClass *v1.StorageClassApplyConfiguration, opts metav1.ApplyOptions) (result *storagev1.StorageClass, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus is not implemented for %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
}
