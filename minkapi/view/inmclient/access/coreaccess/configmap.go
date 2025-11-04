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
	_ clientcorev1.ConfigMapInterface = (*configMapAccess)(nil)
)

type configMapAccess struct {
	access.GenericResourceAccess[*corev1.ConfigMap, *corev1.ConfigMapList]
}

// NewConfigMapAccess creates a new access facade for managing ConfigMap resources within a specific namespace using the given minkapi View.
func NewConfigMapAccess(view mkapi.View, namespace string) clientcorev1.ConfigMapInterface {
	return &configMapAccess{
		access.GenericResourceAccess[*corev1.ConfigMap, *corev1.ConfigMapList]{
			View:      view,
			GVK:       typeinfo.ConfigMapsDescriptor.GVK,
			Namespace: namespace,
		},
	}
}

func (a *configMapAccess) Create(ctx context.Context, configMap *corev1.ConfigMap, opts metav1.CreateOptions) (*corev1.ConfigMap, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, configMap)
}

func (a *configMapAccess) Update(ctx context.Context, configMap *corev1.ConfigMap, opts metav1.UpdateOptions) (*corev1.ConfigMap, error) {
	return a.UpdateObject(ctx, opts, configMap)
}

func (a *configMapAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *configMapAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *configMapAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.ConfigMap, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *configMapAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.ConfigMapList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *configMapAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *configMapAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *corev1.ConfigMap, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *configMapAccess) Apply(_ context.Context, _ *v1.ConfigMapApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.ConfigMap, err error) {
	return nil, fmt.Errorf("%w: apply of configmaps is not supported", commonerrors.ErrUnimplemented)
}
