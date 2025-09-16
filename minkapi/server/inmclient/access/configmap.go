package access

import (
	"context"
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/applyconfigurations/core/v1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	_ clientcorev1.ConfigMapInterface = (*configMapAccess)(nil)
)

type configMapAccess struct {
	BasicResourceAccess[*corev1.ConfigMap, *corev1.ConfigMapList]
}

func (c *configMapAccess) Create(ctx context.Context, configMap *corev1.ConfigMap, opts metav1.CreateOptions) (*corev1.ConfigMap, error) {
	return c.createObject(ctx, opts, configMap)
}

func (c *configMapAccess) Update(ctx context.Context, configMap *corev1.ConfigMap, opts metav1.UpdateOptions) (*corev1.ConfigMap, error) {
	return c.updateObject(ctx, opts, configMap)
}

func (c *configMapAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return c.deleteObject(ctx, opts, c.Namespace, name)
}

func (c *configMapAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return c.deleteObjectCollection(ctx, c.Namespace, opts, listOpts)
}

func (c *configMapAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.ConfigMap, error) {
	return c.getObject(ctx, c.Namespace, name)
}

func (c *configMapAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.ConfigMapList, error) {
	return c.getObjectList(ctx, c.Namespace, opts)
}

func (c *configMapAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return c.getWatcher(ctx, c.Namespace, opts)
}

func (c *configMapAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *corev1.ConfigMap, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources is invalid for configmaps", commonerrors.ErrInvalidOptVal)
	}
	return c.patchObject(ctx, name, pt, data, opts)
}

func (c *configMapAccess) Apply(ctx context.Context, configMap *v1.ConfigMapApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.ConfigMap, err error) {
	panic(commonerrors.ErrUnimplemented)
}
