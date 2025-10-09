package access

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

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
	BasicResourceAccess[*corev1.Node, *corev1.NodeList]
}

func NewNodeAccess(view mkapi.View) clientcorev1.NodeInterface {
	return &nodeAccess{
		BasicResourceAccess[*corev1.Node, *corev1.NodeList]{
			view:            view,
			gvk:             typeinfo.NodesDescriptor.GVK,
			Namespace:       metav1.NamespaceNone,
			ResourcePtr:     &corev1.Node{},
			ResourceListPtr: &corev1.NodeList{},
		},
	}
}

func (a *nodeAccess) Create(ctx context.Context, node *corev1.Node, opts metav1.CreateOptions) (*corev1.Node, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, node)
}

func (a *nodeAccess) Update(ctx context.Context, node *corev1.Node, opts metav1.UpdateOptions) (*corev1.Node, error) {
	return a.updateObject(ctx, opts, node)
}

func (a *nodeAccess) UpdateStatus(ctx context.Context, node *corev1.Node, opts metav1.UpdateOptions) (*corev1.Node, error) {
	return a.updateObject(ctx, opts, node)
}

func (a *nodeAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *nodeAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *nodeAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.Node, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *nodeAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.NodeList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *nodeAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *nodeAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *corev1.Node, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for node", commonerrors.ErrInvalidOptVal, subresources)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *nodeAccess) PatchStatus(ctx context.Context, nodeName string, data []byte) (*corev1.Node, error) {
	return a.patchObjectStatus(ctx, nodeName, data)
}

func (a nodeAccess) Apply(ctx context.Context, node *v1.NodeApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.Node, err error) {
	return nil, fmt.Errorf("%w: apply of %q is not supported", commonerrors.ErrUnimplemented, a.gvk.Kind)
}

func (a nodeAccess) ApplyStatus(ctx context.Context, node *v1.NodeApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.Node, err error) {
	return nil, fmt.Errorf("%w: apply of %q is not supported", commonerrors.ErrUnimplemented, a.gvk.Kind)
}
