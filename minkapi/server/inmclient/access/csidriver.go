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
	_ clientstoragev1.CSIDriverInterface = (*csiDriverAccess)(nil)
)

type csiDriverAccess struct {
	BasicResourceAccess[*storagev1.CSIDriver, *storagev1.CSIDriverList]
}

func NewCSIDriverAccess(view mkapi.View) clientstoragev1.CSIDriverInterface {
	return &csiDriverAccess{
		BasicResourceAccess[*storagev1.CSIDriver, *storagev1.CSIDriverList]{
			view:            view,
			gvk:             typeinfo.CSIDriverDescriptor.GVK,
			Namespace:       metav1.NamespaceNone,
			ResourcePtr:     &storagev1.CSIDriver{},
			ResourceListPtr: &storagev1.CSIDriverList{},
		},
	}
}

func (a *csiDriverAccess) Create(ctx context.Context, csiDriver *storagev1.CSIDriver, opts metav1.CreateOptions) (*storagev1.CSIDriver, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, csiDriver)
}

func (a *csiDriverAccess) Update(ctx context.Context, csiDriver *storagev1.CSIDriver, opts metav1.UpdateOptions) (*storagev1.CSIDriver, error) {
	return a.updateObject(ctx, opts, csiDriver)
}

func (a *csiDriverAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *csiDriverAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *csiDriverAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*storagev1.CSIDriver, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *csiDriverAccess) List(ctx context.Context, opts metav1.ListOptions) (*storagev1.CSIDriverList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *csiDriverAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *csiDriverAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *storagev1.CSIDriver, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for csiDrivers", commonerrors.ErrInvalidOptVal, subresources)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *csiDriverAccess) Apply(ctx context.Context, csiDriver *v1.CSIDriverApplyConfiguration, opts metav1.ApplyOptions) (result *storagev1.CSIDriver, err error) {
	return nil, fmt.Errorf("%w: apply is not implemented for csiDrivers", commonerrors.ErrUnimplemented)
}

func (a *csiDriverAccess) ApplyStatus(ctx context.Context, csiDriver *v1.CSIDriverApplyConfiguration, opts metav1.ApplyOptions) (result *storagev1.CSIDriver, err error) {
	return nil, fmt.Errorf("%w: applyStatus is not implemented for csiDrivers", commonerrors.ErrUnimplemented)
}
