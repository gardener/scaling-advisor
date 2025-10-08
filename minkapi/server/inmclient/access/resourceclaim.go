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
	"k8s.io/client-go/applyconfigurations/resource/v1"
	clientresourcev1 "k8s.io/client-go/kubernetes/typed/resource/v1"
)

var (
	_ clientresourcev1.ResourceClaimInterface = (*resourceClaimAccess)(nil)
)

type resourceClaimAccess struct {
	BasicResourceAccess[*resourcev1.ResourceClaim, *resourcev1.ResourceClaimList]
}

func NewResourceClaimAccess(view mkapi.View, namespace string) clientresourcev1.ResourceClaimInterface {
	return &resourceClaimAccess{
		BasicResourceAccess[*resourcev1.ResourceClaim, *resourcev1.ResourceClaimList]{
			view:            view,
			gvk:             typeinfo.ResourceClaimDescriptor.GVK,
			Namespace:       namespace,
			ResourcePtr:     &resourcev1.ResourceClaim{},
			ResourceListPtr: &resourcev1.ResourceClaimList{},
		},
	}
}
func (a *resourceClaimAccess) Create(ctx context.Context, resourceClaim *resourcev1.ResourceClaim, opts metav1.CreateOptions) (*resourcev1.ResourceClaim, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, resourceClaim)
}

func (a *resourceClaimAccess) Update(ctx context.Context, resourceClaim *resourcev1.ResourceClaim, opts metav1.UpdateOptions) (*resourcev1.ResourceClaim, error) {
	return a.updateObject(ctx, opts, resourceClaim)
}

func (a *resourceClaimAccess) UpdateStatus(ctx context.Context, resourceClaim *resourcev1.ResourceClaim, opts metav1.UpdateOptions) (*resourcev1.ResourceClaim, error) {
	return a.updateObject(ctx, opts, resourceClaim)
}

func (a *resourceClaimAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *resourceClaimAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *resourceClaimAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*resourcev1.ResourceClaim, error) {
	return a.getObject(ctx, a.Namespace, name)
}

func (a *resourceClaimAccess) List(ctx context.Context, opts metav1.ListOptions) (*resourcev1.ResourceClaimList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *resourceClaimAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *resourceClaimAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *resourcev1.ResourceClaim, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for %q", commonerrors.ErrInvalidOptVal, subresources, a.gvk.Kind)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *resourceClaimAccess) Apply(ctx context.Context, resourceClaim *v1.ResourceClaimApplyConfiguration, opts metav1.ApplyOptions) (result *resourcev1.ResourceClaim, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (a *resourceClaimAccess) ApplyStatus(ctx context.Context, resourceClaim *v1.ResourceClaimApplyConfiguration, opts metav1.ApplyOptions) (result *resourcev1.ResourceClaim, err error) {
	panic(commonerrors.ErrUnimplemented)
}
