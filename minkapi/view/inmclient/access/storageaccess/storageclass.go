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
	clientconfigstoragev1 "k8s.io/client-go/applyconfigurations/storage/v1"
	clientstoragev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
)

var (
	_ clientstoragev1.StorageClassInterface = (*storageClassAccess)(nil)
)

type storageClassAccess struct {
	access.GenericResourceAccess[*storagev1.StorageClass, *storagev1.StorageClassList]
}

// NewStorageClassAccess creates an access facade for managing StorageClass resources using the given minkapi View.
func NewStorageClassAccess(view mkapi.View) clientstoragev1.StorageClassInterface {
	return &storageClassAccess{
		access.GenericResourceAccess[*storagev1.StorageClass, *storagev1.StorageClassList]{
			View:      view,
			GVK:       typeinfo.StorageClassDescriptor.GVK,
			Namespace: metav1.NamespaceNone,
		},
	}
}

func (a *storageClassAccess) Create(ctx context.Context, storageClass *storagev1.StorageClass, opts metav1.CreateOptions) (*storagev1.StorageClass, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, storageClass)
}

func (a *storageClassAccess) Update(ctx context.Context, storageClass *storagev1.StorageClass, opts metav1.UpdateOptions) (*storagev1.StorageClass, error) {
	return a.UpdateObject(ctx, opts, storageClass)
}

func (a *storageClassAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *storageClassAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *storageClassAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*storagev1.StorageClass, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *storageClassAccess) List(ctx context.Context, opts metav1.ListOptions) (*storagev1.StorageClassList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *storageClassAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *storageClassAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *storagev1.StorageClass, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *storageClassAccess) Apply(_ context.Context, _ *clientconfigstoragev1.StorageClassApplyConfiguration, _ metav1.ApplyOptions) (result *storagev1.StorageClass, err error) {
	return nil, fmt.Errorf("%w: Apply is not implemented for %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *storageClassAccess) ApplyStatus(_ context.Context, _ *clientconfigstoragev1.StorageClassApplyConfiguration, _ metav1.ApplyOptions) (result *storagev1.StorageClass, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus is not implemented for %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
}
