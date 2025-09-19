package access

import (
	"context"
	"fmt"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
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
	BasicResourceAccess[*storagev1.CSIStorageCapacity, *storagev1.CSIStorageCapacityList]
}

func NewCSIStorageCapacityAccess(view mkapi.View, namespace string) clientstoragev1.CSIStorageCapacityInterface {
	return &csiStorageCapacityAccess{
		BasicResourceAccess[*storagev1.CSIStorageCapacity, *storagev1.CSIStorageCapacityList]{
			view:            view,
			gvk:             typeinfo.CSIStorageCapacityDescriptor.GVK,
			Namespace:       namespace,
			ResourcePtr:     &storagev1.CSIStorageCapacity{},
			ResourceListPtr: &storagev1.CSIStorageCapacityList{},
		},
	}
}

func (a *csiStorageCapacityAccess) Create(ctx context.Context, csiStorageCapacity *storagev1.CSIStorageCapacity, opts metav1.CreateOptions) (*storagev1.CSIStorageCapacity, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, csiStorageCapacity)
}
func (a *csiStorageCapacityAccess) CreateWithCsiStorageCapacityNamespaceWithContext(ctx context.Context, csiStorageCapacity *storagev1.CSIStorageCapacity) (*storagev1.CSIStorageCapacity, error) {
	return a.createObject(ctx, metav1.CreateOptions{}, csiStorageCapacity)
}
func (a *csiStorageCapacityAccess) CreateWithCsiStorageCapacityNamespace(csiStorageCapacity *storagev1.CSIStorageCapacity) (*storagev1.CSIStorageCapacity, error) {
	return a.createObject(context.Background(), metav1.CreateOptions{}, csiStorageCapacity)
}

func (a *csiStorageCapacityAccess) Update(ctx context.Context, csiStorageCapacity *storagev1.CSIStorageCapacity, opts metav1.UpdateOptions) (*storagev1.CSIStorageCapacity, error) {
	return a.updateObject(ctx, opts, csiStorageCapacity)
}
func (a *csiStorageCapacityAccess) UpdateWithCsiStorageCapacityNamespaceWithContext(ctx context.Context, csiStorageCapacity *storagev1.CSIStorageCapacity) (*storagev1.CSIStorageCapacity, error) {
	//TODO implement me
	panic("implement me")
}
func (a *csiStorageCapacityAccess) UpdateWithCsiStorageCapacityNamespace(csiStorageCapacity *storagev1.CSIStorageCapacity) (*storagev1.CSIStorageCapacity, error) {
	//TODO implement me
	panic("implement me")
}

func (a *csiStorageCapacityAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *csiStorageCapacityAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *csiStorageCapacityAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*storagev1.CSIStorageCapacity, error) {
	return a.getObject(ctx, a.Namespace, name)
}

func (a *csiStorageCapacityAccess) List(ctx context.Context, opts metav1.ListOptions) (*storagev1.CSIStorageCapacityList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *csiStorageCapacityAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *csiStorageCapacityAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *storagev1.CSIStorageCapacity, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for csiStorageCapacitys", commonerrors.ErrInvalidOptVal, subresources)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}
func (a *csiStorageCapacityAccess) PatchWithCsiStorageCapacityNamespace(csiStorageCapacity *storagev1.CSIStorageCapacity, data []byte) (*storagev1.CSIStorageCapacity, error) {
	//TODO implement me
	panic("implement me")
}

func (a *csiStorageCapacityAccess) PatchWithCsiStorageCapacityNamespaceWithContext(ctx context.Context, csiStorageCapacity *storagev1.CSIStorageCapacity, data []byte) (*storagev1.CSIStorageCapacity, error) {
	//TODO implement me
	panic("implement me")
}

func (a *csiStorageCapacityAccess) Apply(ctx context.Context, csiStorageCapacity *v1.CSIStorageCapacityApplyConfiguration, opts metav1.ApplyOptions) (result *storagev1.CSIStorageCapacity, err error) {
	panic(commonerrors.ErrUnimplemented)
}
