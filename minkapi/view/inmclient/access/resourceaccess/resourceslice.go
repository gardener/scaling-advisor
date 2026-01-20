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
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
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
	access.GenericResourceAccess[*resourcev1.ResourceSlice, *resourcev1.ResourceSliceList]
}

// NewResourceSliceAccess creates an access facade for managing ResourceSlice resources using the given minkapi View.
func NewResourceSliceAccess(view mkapi.View) clientresourcev1.ResourceSliceInterface {
	return &resourceSliceAccess{
		access.GenericResourceAccess[*resourcev1.ResourceSlice, *resourcev1.ResourceSliceList]{
			View:      view,
			GVK:       typeinfo.ResourceSliceDescriptor.GVK,
			Namespace: metav1.NamespaceNone,
		},
	}
}

func (a *resourceSliceAccess) Create(ctx context.Context, resourceSlice *resourcev1.ResourceSlice, opts metav1.CreateOptions) (*resourcev1.ResourceSlice, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, resourceSlice)
}

func (a *resourceSliceAccess) Update(ctx context.Context, resourceSlice *resourcev1.ResourceSlice, opts metav1.UpdateOptions) (*resourcev1.ResourceSlice, error) {
	return a.UpdateObject(ctx, opts, resourceSlice)
}

func (a *resourceSliceAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *resourceSliceAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *resourceSliceAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*resourcev1.ResourceSlice, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *resourceSliceAccess) List(ctx context.Context, opts metav1.ListOptions) (*resourcev1.ResourceSliceList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *resourceSliceAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *resourceSliceAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *resourcev1.ResourceSlice, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *resourceSliceAccess) Apply(_ context.Context, _ *v1.ResourceSliceApplyConfiguration, _ metav1.ApplyOptions) (result *resourcev1.ResourceSlice, err error) {
	return nil, fmt.Errorf("%w: Apply is not implemented for %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
}
