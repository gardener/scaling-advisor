// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package resourceaccess

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"github.com/gardener/scaling-advisor/api/minkapi"
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
	access.GenericResourceAccess[*resourcev1.ResourceClaim, *resourcev1.ResourceClaimList]
}

// NewResourceClaimAccess creates a new access facade for managing ResourceClaim resources within a specific namespace using the given minkapi View.
func NewResourceClaimAccess(view minkapi.View, namespace string) clientresourcev1.ResourceClaimInterface {
	return &resourceClaimAccess{
		access.GenericResourceAccess[*resourcev1.ResourceClaim, *resourcev1.ResourceClaimList]{
			View:      view,
			GVK:       typeinfo.ResourceClaimDescriptor.GVK,
			Namespace: namespace,
		},
	}
}

func (a *resourceClaimAccess) Create(ctx context.Context, resourceClaim *resourcev1.ResourceClaim, opts metav1.CreateOptions) (*resourcev1.ResourceClaim, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, resourceClaim)
}

func (a *resourceClaimAccess) Update(ctx context.Context, resourceClaim *resourcev1.ResourceClaim, opts metav1.UpdateOptions) (*resourcev1.ResourceClaim, error) {
	return a.UpdateObject(ctx, opts, resourceClaim)
}

func (a *resourceClaimAccess) UpdateStatus(ctx context.Context, resourceClaim *resourcev1.ResourceClaim, opts metav1.UpdateOptions) (*resourcev1.ResourceClaim, error) {
	return a.UpdateObject(ctx, opts, resourceClaim)
}

func (a *resourceClaimAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *resourceClaimAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *resourceClaimAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*resourcev1.ResourceClaim, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *resourceClaimAccess) List(ctx context.Context, opts metav1.ListOptions) (*resourcev1.ResourceClaimList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *resourceClaimAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *resourceClaimAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *resourcev1.ResourceClaim, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *resourceClaimAccess) Apply(_ context.Context, _ *v1.ResourceClaimApplyConfiguration, _ metav1.ApplyOptions) (result *resourcev1.ResourceClaim, err error) {
	return nil, fmt.Errorf("%w: Apply is not implemented for %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *resourceClaimAccess) ApplyStatus(_ context.Context, _ *v1.ResourceClaimApplyConfiguration, _ metav1.ApplyOptions) (result *resourcev1.ResourceClaim, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus is not implemented for %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
}
