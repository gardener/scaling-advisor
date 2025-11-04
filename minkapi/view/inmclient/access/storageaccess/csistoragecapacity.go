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
	v1 "k8s.io/client-go/applyconfigurations/storage/v1"
	clientstoragev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
)

var (
	_ clientstoragev1.CSIStorageCapacityInterface = (*csiStorageCapacityAccess)(nil)
)

type csiStorageCapacityAccess struct {
	access.GenericResourceAccess[*storagev1.CSIStorageCapacity, *storagev1.CSIStorageCapacityList]
}

// NewCSIStorageCapacityAccess creates a new access facade for managing CSIStorageCapacity resources within a specific namespace using the given minkapi View.
func NewCSIStorageCapacityAccess(view mkapi.View, namespace string) clientstoragev1.CSIStorageCapacityInterface {
	return &csiStorageCapacityAccess{
		access.GenericResourceAccess[*storagev1.CSIStorageCapacity, *storagev1.CSIStorageCapacityList]{
			View:      view,
			GVK:       typeinfo.CSIStorageCapacityDescriptor.GVK,
			Namespace: namespace,
		},
	}
}

func (a *csiStorageCapacityAccess) Create(ctx context.Context, csiStorageCapacity *storagev1.CSIStorageCapacity, opts metav1.CreateOptions) (*storagev1.CSIStorageCapacity, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, csiStorageCapacity)
}

func (a *csiStorageCapacityAccess) CreateWithCsiStorageCapacityNamespaceWithContext(ctx context.Context, csiStorageCapacity *storagev1.CSIStorageCapacity) (*storagev1.CSIStorageCapacity, error) {
	return a.CreateObject(ctx, metav1.CreateOptions{}, csiStorageCapacity)
}

func (a *csiStorageCapacityAccess) CreateWithCsiStorageCapacityNamespace(csiStorageCapacity *storagev1.CSIStorageCapacity) (*storagev1.CSIStorageCapacity, error) {
	return a.CreateObject(context.Background(), metav1.CreateOptions{}, csiStorageCapacity)
}

func (a *csiStorageCapacityAccess) Update(ctx context.Context, csiStorageCapacity *storagev1.CSIStorageCapacity, opts metav1.UpdateOptions) (*storagev1.CSIStorageCapacity, error) {
	return a.UpdateObject(ctx, opts, csiStorageCapacity)
}

func (a *csiStorageCapacityAccess) UpdateWithCsiStorageCapacityNamespaceWithContext(_ context.Context, _ *storagev1.CSIStorageCapacity) (*storagev1.CSIStorageCapacity, error) {
	return nil, fmt.Errorf("%w: update of csiStorageCapacitys is not supported", commonerrors.ErrInvalidOptVal)
}

func (a *csiStorageCapacityAccess) UpdateWithCsiStorageCapacityNamespace(_ *storagev1.CSIStorageCapacity) (*storagev1.CSIStorageCapacity, error) {
	return nil, fmt.Errorf("%w: update of csiStorageCapacitys is not supported", commonerrors.ErrInvalidOptVal)
}

func (a *csiStorageCapacityAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *csiStorageCapacityAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *csiStorageCapacityAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*storagev1.CSIStorageCapacity, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *csiStorageCapacityAccess) List(ctx context.Context, opts metav1.ListOptions) (*storagev1.CSIStorageCapacityList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *csiStorageCapacityAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *csiStorageCapacityAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *storagev1.CSIStorageCapacity, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *csiStorageCapacityAccess) PatchWithCsiStorageCapacityNamespace(_ *storagev1.CSIStorageCapacity, _ []byte) (*storagev1.CSIStorageCapacity, error) {
	return nil, fmt.Errorf("%w: patch of csiStorageCapacitys is not supported", commonerrors.ErrInvalidOptVal)
}

func (a *csiStorageCapacityAccess) PatchWithCsiStorageCapacityNamespaceWithContext(_ context.Context, _ *storagev1.CSIStorageCapacity, _ []byte) (*storagev1.CSIStorageCapacity, error) {
	return nil, fmt.Errorf("%w: patch of csiStorageCapacitys is not supported", commonerrors.ErrInvalidOptVal)
}

func (a *csiStorageCapacityAccess) Apply(_ context.Context, _ *v1.CSIStorageCapacityApplyConfiguration, _ metav1.ApplyOptions) (result *storagev1.CSIStorageCapacity, err error) {
	return nil, fmt.Errorf("%w: apply of csiStorageCapacitys is not supported", commonerrors.ErrInvalidOptVal)
}
