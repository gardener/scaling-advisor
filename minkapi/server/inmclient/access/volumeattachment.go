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
	_ clientstoragev1.VolumeAttachmentInterface = (*volumeAttachmentAccess)(nil)
)

type volumeAttachmentAccess struct {
	BasicResourceAccess[*storagev1.VolumeAttachment, *storagev1.VolumeAttachmentList]
}

func NewVolumeAttachmentAccess(view mkapi.View) clientstoragev1.VolumeAttachmentInterface {
	return &volumeAttachmentAccess{
		BasicResourceAccess[*storagev1.VolumeAttachment, *storagev1.VolumeAttachmentList]{
			view:            view,
			gvk:             typeinfo.VolumeAttachmentDescriptor.GVK,
			Namespace:       metav1.NamespaceNone,
			ResourcePtr:     &storagev1.VolumeAttachment{},
			ResourceListPtr: &storagev1.VolumeAttachmentList{},
		},
	}
}

func (a *volumeAttachmentAccess) Create(ctx context.Context, volumeAttachment *storagev1.VolumeAttachment, opts metav1.CreateOptions) (*storagev1.VolumeAttachment, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, volumeAttachment)
}

func (a *volumeAttachmentAccess) Update(ctx context.Context, volumeAttachment *storagev1.VolumeAttachment, opts metav1.UpdateOptions) (*storagev1.VolumeAttachment, error) {
	return a.updateObject(ctx, opts, volumeAttachment)
}

func (a *volumeAttachmentAccess) UpdateStatus(ctx context.Context, volumeAttachment *storagev1.VolumeAttachment, opts metav1.UpdateOptions) (*storagev1.VolumeAttachment, error) {
	return a.updateObject(ctx, opts, volumeAttachment)
}

func (a *volumeAttachmentAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *volumeAttachmentAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *volumeAttachmentAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*storagev1.VolumeAttachment, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *volumeAttachmentAccess) List(ctx context.Context, opts metav1.ListOptions) (*storagev1.VolumeAttachmentList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *volumeAttachmentAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *volumeAttachmentAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *storagev1.VolumeAttachment, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for volumeAttachments", commonerrors.ErrInvalidOptVal, subresources)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *volumeAttachmentAccess) Apply(ctx context.Context, volumeAttachment *v1.VolumeAttachmentApplyConfiguration, opts metav1.ApplyOptions) (result *storagev1.VolumeAttachment, err error) {
	return nil, fmt.Errorf("%w: apply is not implemented for %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a *volumeAttachmentAccess) ApplyStatus(ctx context.Context, volumeAttachment *v1.VolumeAttachmentApplyConfiguration, opts metav1.ApplyOptions) (result *storagev1.VolumeAttachment, err error) {
	return nil, fmt.Errorf("%w: applyStatus is not implemented for %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
}
