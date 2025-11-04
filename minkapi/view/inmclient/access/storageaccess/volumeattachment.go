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
	_ clientstoragev1.VolumeAttachmentInterface = (*volumeAttachmentAccess)(nil)
)

type volumeAttachmentAccess struct {
	access.GenericResourceAccess[*storagev1.VolumeAttachment, *storagev1.VolumeAttachmentList]
}

// NewVolumeAttachmentAccess creates an access facade for managing VolumeAttachmentList resources using the given minkapi View.
func NewVolumeAttachmentAccess(view mkapi.View) clientstoragev1.VolumeAttachmentInterface {
	return &volumeAttachmentAccess{
		access.GenericResourceAccess[*storagev1.VolumeAttachment, *storagev1.VolumeAttachmentList]{
			View:      view,
			GVK:       typeinfo.VolumeAttachmentDescriptor.GVK,
			Namespace: metav1.NamespaceNone,
		},
	}
}

func (a *volumeAttachmentAccess) Create(ctx context.Context, volumeAttachment *storagev1.VolumeAttachment, opts metav1.CreateOptions) (*storagev1.VolumeAttachment, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, volumeAttachment)
}

func (a *volumeAttachmentAccess) Update(ctx context.Context, volumeAttachment *storagev1.VolumeAttachment, opts metav1.UpdateOptions) (*storagev1.VolumeAttachment, error) {
	return a.UpdateObject(ctx, opts, volumeAttachment)
}

func (a *volumeAttachmentAccess) UpdateStatus(ctx context.Context, volumeAttachment *storagev1.VolumeAttachment, opts metav1.UpdateOptions) (*storagev1.VolumeAttachment, error) {
	return a.UpdateObject(ctx, opts, volumeAttachment)
}

func (a *volumeAttachmentAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *volumeAttachmentAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *volumeAttachmentAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*storagev1.VolumeAttachment, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *volumeAttachmentAccess) List(ctx context.Context, opts metav1.ListOptions) (*storagev1.VolumeAttachmentList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *volumeAttachmentAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *volumeAttachmentAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *storagev1.VolumeAttachment, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *volumeAttachmentAccess) Apply(_ context.Context, _ *v1.VolumeAttachmentApplyConfiguration, _ metav1.ApplyOptions) (result *storagev1.VolumeAttachment, err error) {
	return nil, fmt.Errorf("%w: apply is not implemented for %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *volumeAttachmentAccess) ApplyStatus(_ context.Context, _ *v1.VolumeAttachmentApplyConfiguration, _ metav1.ApplyOptions) (result *storagev1.VolumeAttachment, err error) {
	return nil, fmt.Errorf("%w: applyStatus is not implemented for %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
}
