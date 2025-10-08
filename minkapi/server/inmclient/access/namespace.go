package access

import (
	"context"
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
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
	BasicResourceAccess[*corev1.Namespace, *corev1.NamespaceList]
}

func NewNamespaceAccess(view mkapi.View) clientcorev1.NamespaceInterface {
	return &namespaceAccess{
		BasicResourceAccess[*corev1.Namespace, *corev1.NamespaceList]{
			view:            view,
			gvk:             typeinfo.NamespacesDescriptor.GVK,
			Namespace:       metav1.NamespaceNone, //TODO: check if ok.
			ResourcePtr:     &corev1.Namespace{},
			ResourceListPtr: &corev1.NamespaceList{},
		},
	}
}

func (a *namespaceAccess) Create(ctx context.Context, namespace *corev1.Namespace, opts metav1.CreateOptions) (*corev1.Namespace, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, namespace)
}

func (a *namespaceAccess) Update(ctx context.Context, namespace *corev1.Namespace, opts metav1.UpdateOptions) (*corev1.Namespace, error) {
	return a.updateObject(ctx, opts, namespace)
}

func (a *namespaceAccess) UpdateStatus(ctx context.Context, namespace *corev1.Namespace, opts metav1.UpdateOptions) (*corev1.Namespace, error) {
	return a.updateObject(ctx, opts, namespace)
}

func (a *namespaceAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *namespaceAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.Namespace, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *namespaceAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.NamespaceList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *namespaceAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *namespaceAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *corev1.Namespace, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for namespace", commonerrors.ErrInvalidOptVal, subresources)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a namespaceAccess) Apply(ctx context.Context, namespace *v1.NamespaceApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.Namespace, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (a namespaceAccess) ApplyStatus(ctx context.Context, namespace *v1.NamespaceApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.Namespace, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (a *namespaceAccess) Finalize(ctx context.Context, item *corev1.Namespace, opts metav1.UpdateOptions) (*corev1.Namespace, error) {
	return a.updateObject(ctx, opts, item)
}
