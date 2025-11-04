package storageaccess

import (
	"context"
	"fmt"
	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access"

	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	applyconfigv1 "k8s.io/client-go/applyconfigurations/storage/v1"
	clientstoragev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
)

var (
	_ clientstoragev1.CSIDriverInterface = (*csiDriverAccess)(nil)
)

type csiDriverAccess struct {
	access.GenericResourceAccess[*storagev1.CSIDriver, *storagev1.CSIDriverList]
}

// NewCSIDriverAccess creates an access facade for managing CSIDriver resources using the given minkapi View.
func NewCSIDriverAccess(view mkapi.View) clientstoragev1.CSIDriverInterface {
	return &csiDriverAccess{
		access.GenericResourceAccess[*storagev1.CSIDriver, *storagev1.CSIDriverList]{
			View:      view,
			GVK:       typeinfo.CSIDriverDescriptor.GVK,
			Namespace: metav1.NamespaceNone,
		},
	}
}

func (a *csiDriverAccess) Create(ctx context.Context, csiDriver *storagev1.CSIDriver, opts metav1.CreateOptions) (*storagev1.CSIDriver, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, csiDriver)
}

func (a *csiDriverAccess) Update(ctx context.Context, csiDriver *storagev1.CSIDriver, opts metav1.UpdateOptions) (*storagev1.CSIDriver, error) {
	return a.UpdateObject(ctx, opts, csiDriver)
}

func (a *csiDriverAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *csiDriverAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *csiDriverAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*storagev1.CSIDriver, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *csiDriverAccess) List(ctx context.Context, opts metav1.ListOptions) (*storagev1.CSIDriverList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *csiDriverAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *csiDriverAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *storagev1.CSIDriver, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *csiDriverAccess) Apply(_ context.Context, _ *applyconfigv1.CSIDriverApplyConfiguration, _ metav1.ApplyOptions) (result *storagev1.CSIDriver, err error) {
	return nil, fmt.Errorf("%w: apply is not implemented for csiDrivers", commonerrors.ErrUnimplemented)
}
