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
	_ clientstoragev1.VolumeAttributesClassInterface = (*volumeAttributesClassAccess)(nil)
)

type volumeAttributesClassAccess struct {
	BasicResourceAccess[*storagev1.VolumeAttributesClass, *storagev1.VolumeAttributesClassList]
}

func NewVolumeAttributesClassAccess(view mkapi.View) clientstoragev1.VolumeAttributesClassInterface {
	return &volumeAttributesClassAccess{
		BasicResourceAccess[*storagev1.VolumeAttributesClass, *storagev1.VolumeAttributesClassList]{
			view:            view,
			gvk:             typeinfo.VolumeAttributesClassDescriptor.GVK,
			Namespace:       metav1.NamespaceNone,
			ResourcePtr:     &storagev1.VolumeAttributesClass{},
			ResourceListPtr: &storagev1.VolumeAttributesClassList{},
		},
	}
}

func (a *volumeAttributesClassAccess) Create(ctx context.Context, volumeAttributesClass *storagev1.VolumeAttributesClass, opts metav1.CreateOptions) (*storagev1.VolumeAttributesClass, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, volumeAttributesClass)
}

func (a *volumeAttributesClassAccess) Update(ctx context.Context, volumeAttributesClass *storagev1.VolumeAttributesClass, opts metav1.UpdateOptions) (*storagev1.VolumeAttributesClass, error) {
	return a.updateObject(ctx, opts, volumeAttributesClass)
}

func (a *volumeAttributesClassAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *volumeAttributesClassAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *volumeAttributesClassAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*storagev1.VolumeAttributesClass, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *volumeAttributesClassAccess) List(ctx context.Context, opts metav1.ListOptions) (*storagev1.VolumeAttributesClassList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *volumeAttributesClassAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *volumeAttributesClassAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *storagev1.VolumeAttributesClass, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for volumeAttributesClass", commonerrors.ErrInvalidOptVal, subresources)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *volumeAttributesClassAccess) Apply(ctx context.Context, volumeAttributesClass *v1.VolumeAttributesClassApplyConfiguration, opts metav1.ApplyOptions) (result *storagev1.VolumeAttributesClass, err error) {
	return nil, fmt.Errorf("%w: apply is not implemented for %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
}
