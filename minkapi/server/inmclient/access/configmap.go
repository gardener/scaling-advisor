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
	_ clientcorev1.ConfigMapInterface = (*configMapAccess)(nil)
)

type configMapAccess struct {
	BasicResourceAccess[*corev1.ConfigMap, *corev1.ConfigMapList]
}

func NewConfigMapAccess(view mkapi.View, namespace string) clientcorev1.ConfigMapInterface {
	return &configMapAccess{
		BasicResourceAccess[*corev1.ConfigMap, *corev1.ConfigMapList]{
			view:            view,
			gvk:             typeinfo.ConfigMapsDescriptor.GVK,
			Namespace:       namespace,
			ResourcePtr:     &corev1.ConfigMap{},
			ResourceListPtr: &corev1.ConfigMapList{},
		},
	}
}

func (a *configMapAccess) Create(ctx context.Context, configMap *corev1.ConfigMap, opts metav1.CreateOptions) (*corev1.ConfigMap, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, configMap)
}

func (a *configMapAccess) Update(ctx context.Context, configMap *corev1.ConfigMap, opts metav1.UpdateOptions) (*corev1.ConfigMap, error) {
	return a.updateObject(ctx, opts, configMap)
}

func (a *configMapAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *configMapAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *configMapAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.ConfigMap, error) {
	return a.getObject(ctx, a.Namespace, name)
}

func (a *configMapAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.ConfigMapList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *configMapAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *configMapAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *corev1.ConfigMap, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for configmaps", commonerrors.ErrInvalidOptVal, subresources)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *configMapAccess) Apply(ctx context.Context, configMap *v1.ConfigMapApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.ConfigMap, err error) {
	panic(commonerrors.ErrUnimplemented)
}
