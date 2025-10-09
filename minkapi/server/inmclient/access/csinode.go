package access

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/applyconfigurations/storage/v1"
	clientstoragev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
)

var (
	_ clientstoragev1.CSINodeInterface = (*csiNodeAccess)(nil)
)

type csiNodeAccess struct {
	BasicResourceAccess[*storagev1.CSINode, *storagev1.CSINodeList]
}

func NewCSINodeAccess(view mkapi.View) clientstoragev1.CSINodeInterface {
	return &csiNodeAccess{
		BasicResourceAccess[*storagev1.CSINode, *storagev1.CSINodeList]{
			view:            view,
			gvk:             typeinfo.CSINodeDescriptor.GVK,
			Namespace:       metav1.NamespaceNone,
			ResourcePtr:     &storagev1.CSINode{},
			ResourceListPtr: &storagev1.CSINodeList{},
		},
	}
}

func (a *csiNodeAccess) Create(ctx context.Context, csiNode *storagev1.CSINode, opts metav1.CreateOptions) (*storagev1.CSINode, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, csiNode)
}

func (a *csiNodeAccess) Update(ctx context.Context, csiNode *storagev1.CSINode, opts metav1.UpdateOptions) (*storagev1.CSINode, error) {
	return a.updateObject(ctx, opts, csiNode)
}

func (a *csiNodeAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *csiNodeAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *csiNodeAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*storagev1.CSINode, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *csiNodeAccess) List(ctx context.Context, opts metav1.ListOptions) (*storagev1.CSINodeList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *csiNodeAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *csiNodeAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *storagev1.CSINode, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for csiNodes", commonerrors.ErrInvalidOptVal, subresources)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *csiNodeAccess) Apply(ctx context.Context, csiNode *v1.CSINodeApplyConfiguration, opts metav1.ApplyOptions) (result *storagev1.CSINode, err error) {
	return nil, fmt.Errorf("%w: apply is not implemented for csiNodes", commonerrors.ErrUnimplemented)
}

func (a *csiNodeAccess) ApplyStatus(ctx context.Context, csiNode *v1.CSINodeApplyConfiguration, opts metav1.ApplyOptions) (result *storagev1.CSINode, err error) {
	return nil, fmt.Errorf("%w: applyStatus is not implemented for csiNodes", commonerrors.ErrUnimplemented)
}
