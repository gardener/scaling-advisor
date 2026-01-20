// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
	applyconfigstoragev1 "k8s.io/client-go/applyconfigurations/storage/v1"
	clientstoragev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
)

var (
	_ clientstoragev1.CSINodeInterface = (*csiNodeAccess)(nil)
)

type csiNodeAccess struct {
	access.GenericResourceAccess[*storagev1.CSINode, *storagev1.CSINodeList]
}

// NewCSINodeAccess creates an access facade for managing CSINodeList resources using the given minkapi View.
func NewCSINodeAccess(view mkapi.View) clientstoragev1.CSINodeInterface {
	return &csiNodeAccess{
		access.GenericResourceAccess[*storagev1.CSINode, *storagev1.CSINodeList]{
			View:      view,
			GVK:       typeinfo.CSINodeDescriptor.GVK,
			Namespace: metav1.NamespaceNone,
		},
	}
}

func (a *csiNodeAccess) Create(ctx context.Context, csiNode *storagev1.CSINode, opts metav1.CreateOptions) (*storagev1.CSINode, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, csiNode)
}

func (a *csiNodeAccess) Update(ctx context.Context, csiNode *storagev1.CSINode, opts metav1.UpdateOptions) (*storagev1.CSINode, error) {
	return a.UpdateObject(ctx, opts, csiNode)
}

func (a *csiNodeAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *csiNodeAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *csiNodeAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*storagev1.CSINode, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *csiNodeAccess) List(ctx context.Context, opts metav1.ListOptions) (*storagev1.CSINodeList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *csiNodeAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *csiNodeAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *storagev1.CSINode, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *csiNodeAccess) Apply(_ context.Context, _ *applyconfigstoragev1.CSINodeApplyConfiguration, _ metav1.ApplyOptions) (result *storagev1.CSINode, err error) {
	return nil, fmt.Errorf("%w: apply is not implemented for csiNodes", commonerrors.ErrUnimplemented)
}

func (a *csiNodeAccess) ApplyStatus(_ context.Context, _ *applyconfigstoragev1.CSINodeApplyConfiguration, _ metav1.ApplyOptions) (result *storagev1.CSINode, err error) {
	return nil, fmt.Errorf("%w: applyStatus is not implemented for csiNodes", commonerrors.ErrUnimplemented)
}
