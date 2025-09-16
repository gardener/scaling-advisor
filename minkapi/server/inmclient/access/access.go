package access

import (
	"context"
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"github.com/gardener/scaling-advisor/api/minkapi"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

type BasicResourceAccess[T metav1.Object, L metav1.ListInterface] struct {
	view            mkapi.View
	gvk             schema.GroupVersionKind
	Namespace       string
	ResourcePtr     T
	ResourceListPtr L
}

type ResourceWithStatusAccess[T metav1.Object, L metav1.ListInterface, S metav1.Object] struct {
	BasicResourceAccess[T, L]
	StatusResourcePtr S
}

func (r *BasicResourceAccess[T, L]) createObject(ctx context.Context, opts metav1.CreateOptions, obj T) (t T, err error) {
	if opts.DryRun != nil {
		err = fmt.Errorf("%w: dry run not implemented for %T.Create", commonerrors.ErrUnimplemented, obj)
		return
	}
	err = r.view.CreateObject(r.gvk, obj)
	if err != nil {
		return
	}
	t, err = r.getObject(ctx, obj.GetName(), obj.GetNamespace())
	return
}

func (r *BasicResourceAccess[T, L]) updateObject(ctx context.Context, opts metav1.UpdateOptions, obj T) (t T, err error) {
	if opts.DryRun != nil {
		err = fmt.Errorf("%w: dry run not implemented for %T.Update", commonerrors.ErrUnimplemented, obj)
		return
	}
	err = r.view.UpdateObject(r.gvk, obj)
	if err != nil {
		return
	}
	t, err = r.getObject(ctx, obj.GetName(), obj.GetNamespace())
	return
}

func (r *BasicResourceAccess[T, L]) deleteObject(_ context.Context, opts metav1.DeleteOptions, namespace, name string) error {
	if opts.DryRun != nil {
		return fmt.Errorf("%w: dry run not implemented for Delete of %q", commonerrors.ErrUnimplemented, r.gvk.Kind)
	}
	return r.view.DeleteObject(r.gvk, cache.NewObjectName(namespace, name))
}

func (r *BasicResourceAccess[T, L]) deleteObjectCollection(ctx context.Context, namespace string, delOpts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	if delOpts.DryRun != nil {
		return fmt.Errorf("%w: dry run not implemented for DeleteCollection of %q", commonerrors.ErrUnimplemented, r.gvk.Kind)
	}
	if delOpts.PropagationPolicy != nil {
		return fmt.Errorf("%w: propagationPolicy is unimplemented for DeleteCollection of %q", commonerrors.ErrUnimplemented, r.gvk.Kind)
	}
	if delOpts.PropagationPolicy != nil {
		return fmt.Errorf("%w: gracePeriodSeconds is unimplemented for DeleteCollection of %q", commonerrors.ErrUnimplemented, r.gvk.Kind)
	}
	err := checkLogListOptions(ctx, listOpts)
	if err != nil {
		return err
	}
	c, err := asMatchCriteria(namespace, listOpts)
	if err != nil {
		return err
	}
	return r.view.DeleteObjects(r.gvk, c)
}

func (r *BasicResourceAccess[T, L]) getObject(_ context.Context, namespace, name string) (t T, err error) {
	objName := cache.NewObjectName(namespace, name)
	obj, err := r.view.GetObject(r.gvk, objName)
	if err != nil {
		return
	}
	return objutil.Cast[T](obj)
}

func (r *BasicResourceAccess[T, L]) getObjectList(ctx context.Context, namespace string, opts metav1.ListOptions) (l L, err error) {
	err = checkLogListOptions(ctx, opts)
	if err != nil {
		return
	}
	c, err := asMatchCriteria(namespace, opts)
	if err != nil {
		return
	}
	listObj, err := r.view.ListObjects(r.gvk, c)
	if err != nil {
		return
	}
	return objutil.Cast[L](listObj)
}
func (r *BasicResourceAccess[T, L]) getWatcher(ctx context.Context, namespace string, opts metav1.ListOptions) (w watch.Interface, err error) {
	err = checkLogListOptions(ctx, opts)
	if err != nil {
		return
	}
	return r.view.GetWatcher(ctx, r.gvk, namespace, opts)
}
func (r *BasicResourceAccess[T, L]) patchObject(_ context.Context, name string, pt types.PatchType, patchData []byte, opts metav1.PatchOptions) (t T, err error) {
	if opts.DryRun != nil {
		err = fmt.Errorf("%w: dry run not implemented for Patch of %q", commonerrors.ErrUnimplemented, r.gvk.Kind)
		return
	}
	obj, err := r.view.PatchObject(r.gvk, cache.NewObjectName(r.Namespace, name), pt, patchData)
	if err != nil {
		return
	}
	t, err = objutil.Cast[T](obj)
	return
}

func checkLogListOptions(ctx context.Context, opts metav1.ListOptions) error {
	log := logr.FromContextOrDiscard(ctx)
	logUnimplementedOptionalListOptions(log, opts)
	return checkUnimplementedRequiredListOptions(opts)
}
func logUnimplementedOptionalListOptions(log logr.Logger, listOptions metav1.ListOptions) {
	if listOptions.AllowWatchBookmarks {
		log.V(4).Info("WatchBookmarks is unimplemented")
	}
	if listOptions.Limit > 0 {
		log.V(4).Info("Limit is unimplemented", "limit", listOptions.Limit)
	}
}

func checkUnimplementedRequiredListOptions(listOptions metav1.ListOptions) error {
	if listOptions.FieldSelector != "" {
		return fmt.Errorf("%w: listOptions.FieldSelector is unimplemented", commonerrors.ErrUnimplemented)
	}
	return nil
}

func asMatchCriteria(namespace string, listOptions metav1.ListOptions) (c minkapi.MatchCriteria, err error) {
	var selector = labels.Everything()
	if listOptions.LabelSelector != "" {
		selector, err = labels.Parse(listOptions.LabelSelector)
	}
	if err != nil {
		return
	}
	c.LabelSelector = selector
	c.Namespace = namespace
	return
}
