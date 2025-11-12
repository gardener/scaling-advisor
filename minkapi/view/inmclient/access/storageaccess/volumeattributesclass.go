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
	_ clientstoragev1.VolumeAttributesClassInterface = (*volumeAttributesClassAccess)(nil)
)

type volumeAttributesClassAccess struct {
	access.GenericResourceAccess[*storagev1.VolumeAttributesClass, *storagev1.VolumeAttributesClassList]
}

// NewVolumeAttributesClassAccess creates an access facade for managing VolumeAttributesClass resources using the given minkapi View.
func NewVolumeAttributesClassAccess(view mkapi.View) clientstoragev1.VolumeAttributesClassInterface {
	return &volumeAttributesClassAccess{
		access.GenericResourceAccess[*storagev1.VolumeAttributesClass, *storagev1.VolumeAttributesClassList]{
			View:      view,
			GVK:       typeinfo.VolumeAttributesClassDescriptor.GVK,
			Namespace: metav1.NamespaceNone,
		},
	}
}

func (a *volumeAttributesClassAccess) Create(ctx context.Context, volumeAttributesClass *storagev1.VolumeAttributesClass, opts metav1.CreateOptions) (*storagev1.VolumeAttributesClass, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, volumeAttributesClass)
}

func (a *volumeAttributesClassAccess) Update(ctx context.Context, volumeAttributesClass *storagev1.VolumeAttributesClass, opts metav1.UpdateOptions) (*storagev1.VolumeAttributesClass, error) {
	return a.UpdateObject(ctx, opts, volumeAttributesClass)
}

func (a *volumeAttributesClassAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *volumeAttributesClassAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *volumeAttributesClassAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*storagev1.VolumeAttributesClass, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *volumeAttributesClassAccess) List(ctx context.Context, opts metav1.ListOptions) (*storagev1.VolumeAttributesClassList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *volumeAttributesClassAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *volumeAttributesClassAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *storagev1.VolumeAttributesClass, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *volumeAttributesClassAccess) Apply(_ context.Context, _ *v1.VolumeAttributesClassApplyConfiguration, _ metav1.ApplyOptions) (result *storagev1.VolumeAttributesClass, err error) {
	return nil, fmt.Errorf("%w: apply is not implemented for %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
}
