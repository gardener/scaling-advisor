// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package coreaccess

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/applyconfigurations/core/v1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	_ clientcorev1.NamespaceInterface = (*namespaceAccess)(nil)
)

type namespaceAccess struct {
	access.GenericResourceAccess[*corev1.Namespace, *corev1.NamespaceList]
}

// NewNamespaceAccess creates a new namespace access facade for managing namespace resources using the given minkapi View.
func NewNamespaceAccess(view mkapi.View) clientcorev1.NamespaceInterface {
	return &namespaceAccess{
		access.GenericResourceAccess[*corev1.Namespace, *corev1.NamespaceList]{
			View:      view,
			GVK:       typeinfo.NamespacesDescriptor.GVK,
			Namespace: metav1.NamespaceNone,
		},
	}
}

func (a *namespaceAccess) Create(ctx context.Context, namespace *corev1.Namespace, opts metav1.CreateOptions) (*corev1.Namespace, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, namespace)
}

func (a *namespaceAccess) Update(ctx context.Context, namespace *corev1.Namespace, opts metav1.UpdateOptions) (*corev1.Namespace, error) {
	return a.UpdateObject(ctx, opts, namespace)
}

func (a *namespaceAccess) UpdateStatus(ctx context.Context, namespace *corev1.Namespace, opts metav1.UpdateOptions) (*corev1.Namespace, error) {
	return a.UpdateObject(ctx, opts, namespace)
}

func (a *namespaceAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *namespaceAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.Namespace, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *namespaceAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.NamespaceList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *namespaceAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *namespaceAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *corev1.Namespace, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *namespaceAccess) Apply(_ context.Context, _ *v1.NamespaceApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.Namespace, err error) {
	return nil, fmt.Errorf("%w: apply of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *namespaceAccess) ApplyStatus(_ context.Context, _ *v1.NamespaceApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.Namespace, err error) {
	return nil, fmt.Errorf("%w: apply of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *namespaceAccess) Finalize(ctx context.Context, item *corev1.Namespace, opts metav1.UpdateOptions) (*corev1.Namespace, error) {
	return a.UpdateObject(ctx, opts, item)
}
