package resourceaccess

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/applyconfigurations/resource/v1"
	clientresourcev1 "k8s.io/client-go/kubernetes/typed/resource/v1"
)

var (
	_ clientresourcev1.DeviceClassInterface = (*deviceClassAccess)(nil)
)

type deviceClassAccess struct {
	access.GenericResourceAccess[*resourcev1.DeviceClass, *resourcev1.DeviceClassList]
}

// NewDeviceClassAccess creates an access facade for managing DeviceClass resources using the given minkapi View.
func NewDeviceClassAccess(view mkapi.View) clientresourcev1.DeviceClassInterface {
	return &deviceClassAccess{
		access.GenericResourceAccess[*resourcev1.DeviceClass, *resourcev1.DeviceClassList]{
			View:      view,
			GVK:       typeinfo.DeviceClassDescriptor.GVK,
			Namespace: metav1.NamespaceNone,
		},
	}
}

func (a *deviceClassAccess) Create(ctx context.Context, deviceClass *resourcev1.DeviceClass, opts metav1.CreateOptions) (*resourcev1.DeviceClass, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, deviceClass)
}

func (a *deviceClassAccess) Update(ctx context.Context, deviceClass *resourcev1.DeviceClass, opts metav1.UpdateOptions) (*resourcev1.DeviceClass, error) {
	return a.UpdateObject(ctx, opts, deviceClass)
}

func (a *deviceClassAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *deviceClassAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *deviceClassAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*resourcev1.DeviceClass, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *deviceClassAccess) List(ctx context.Context, opts metav1.ListOptions) (*resourcev1.DeviceClassList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *deviceClassAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *deviceClassAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *resourcev1.DeviceClass, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *deviceClassAccess) Apply(_ context.Context, _ *v1.DeviceClassApplyConfiguration, _ metav1.ApplyOptions) (result *resourcev1.DeviceClass, err error) {
	return nil, fmt.Errorf("%w: apply is not implemented for %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
}
