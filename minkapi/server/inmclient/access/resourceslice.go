package access

import (
	"context"
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/applyconfigurations/resource/v1"
	clientresourcev1 "k8s.io/client-go/kubernetes/typed/resource/v1"
)

var (
	_ clientresourcev1.ResourceSliceInterface = (*resourceSliceAccess)(nil)
)

type resourceSliceAccess struct {
	BasicResourceAccess[*resourcev1.ResourceSlice, *resourcev1.ResourceSliceList]
}

func NewResourceSliceAccess(view mkapi.View) clientresourcev1.ResourceSliceInterface {
	return &resourceSliceAccess{
		BasicResourceAccess[*resourcev1.ResourceSlice, *resourcev1.ResourceSliceList]{
			view:            view,
			gvk:             typeinfo.ResourceSliceDescriptor.GVK,
			Namespace:       metav1.NamespaceNone,
			ResourcePtr:     &resourcev1.ResourceSlice{},
			ResourceListPtr: &resourcev1.ResourceSliceList{},
		},
	}
}
func (a *resourceSliceAccess) Create(ctx context.Context, resourceSlice *resourcev1.ResourceSlice, opts metav1.CreateOptions) (*resourcev1.ResourceSlice, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, resourceSlice)
}

func (a *resourceSliceAccess) Update(ctx context.Context, resourceSlice *resourcev1.ResourceSlice, opts metav1.UpdateOptions) (*resourcev1.ResourceSlice, error) {
	return a.updateObject(ctx, opts, resourceSlice)
}

func (a *resourceSliceAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *resourceSliceAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *resourceSliceAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*resourcev1.ResourceSlice, error) {
	return a.getObject(ctx, a.Namespace, name)
}

func (a *resourceSliceAccess) List(ctx context.Context, opts metav1.ListOptions) (*resourcev1.ResourceSliceList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *resourceSliceAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *resourceSliceAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *resourcev1.ResourceSlice, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for %q", commonerrors.ErrInvalidOptVal, subresources, a.gvk.Kind)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *resourceSliceAccess) Apply(ctx context.Context, resourceSlice *v1.ResourceSliceApplyConfiguration, opts metav1.ApplyOptions) (result *resourcev1.ResourceSlice, err error) {
	panic(commonerrors.ErrUnimplemented)
}
