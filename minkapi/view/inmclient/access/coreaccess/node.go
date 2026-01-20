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
	_ clientcorev1.NodeInterface = (*nodeAccess)(nil)
)

type nodeAccess struct {
	access.GenericResourceAccess[*corev1.Node, *corev1.NodeList]
}

// NewNodeAccess creates the k8s client core facade for Nodes leveraging the given view.
func NewNodeAccess(view mkapi.View) clientcorev1.NodeInterface {
	return &nodeAccess{
		access.GenericResourceAccess[*corev1.Node, *corev1.NodeList]{
			View:      view,
			GVK:       typeinfo.NodesDescriptor.GVK,
			Namespace: metav1.NamespaceNone,
		},
	}
}

func (a *nodeAccess) Create(ctx context.Context, node *corev1.Node, opts metav1.CreateOptions) (*corev1.Node, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, node)
}

func (a *nodeAccess) Update(ctx context.Context, node *corev1.Node, opts metav1.UpdateOptions) (*corev1.Node, error) {
	return a.UpdateObject(ctx, opts, node)
}

func (a *nodeAccess) UpdateStatus(ctx context.Context, node *corev1.Node, opts metav1.UpdateOptions) (*corev1.Node, error) {
	return a.UpdateObject(ctx, opts, node)
}

func (a *nodeAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *nodeAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *nodeAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.Node, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *nodeAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.NodeList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *nodeAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *nodeAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *corev1.Node, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *nodeAccess) PatchStatus(ctx context.Context, nodeName string, data []byte) (*corev1.Node, error) {
	return a.PatchObjectStatus(ctx, nodeName, data)
}

func (a *nodeAccess) Apply(_ context.Context, _ *v1.NodeApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.Node, err error) {
	return nil, fmt.Errorf("%w: apply of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *nodeAccess) ApplyStatus(_ context.Context, _ *v1.NodeApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.Node, err error) {
	return nil, fmt.Errorf("%w: apply of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}
