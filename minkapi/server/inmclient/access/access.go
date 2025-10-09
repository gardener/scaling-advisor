package access

import (
	"context"
	"fmt"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"github.com/gardener/scaling-advisor/api/minkapi"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

type StatusResourceAccess[T metav1.Object, L metav1.ListInterface, S metav1.Object] struct {
	BasicResourceAccess[T, L]
	StatusResourcePtr S
}

func (a *BasicResourceAccess[T, L]) createObject(ctx context.Context, opts metav1.CreateOptions, obj T) (t T, err error) {
	if opts.DryRun != nil {
		err = fmt.Errorf("%w: dry run not implemented for %T.Create", commonerrors.ErrUnimplemented, obj)
		return
	}
	createdObj, err := a.view.CreateObject(a.gvk, obj)
	if err != nil {
		return
	}
	t, err = a.getObject(ctx, createdObj.GetName(), createdObj.GetNamespace(), metav1.GetOptions{})
	return
}

func (a *BasicResourceAccess[T, L]) createObjectWithAccessNamespace(ctx context.Context, opts metav1.CreateOptions, obj T) (t T, err error) {
	if opts.DryRun != nil {
		err = fmt.Errorf("%w: dry run not implemented for %T.Create", commonerrors.ErrUnimplemented, obj)
		return
	}
	if obj.GetNamespace() != a.Namespace {
		obj.SetNamespace(a.Namespace)
	}
	createdObj, err := a.view.CreateObject(a.gvk, obj)
	if err != nil {
		return
	}
	t, err = a.getObject(ctx, createdObj.GetNamespace(), createdObj.GetName(), metav1.GetOptions{})
	return
}

func (a *BasicResourceAccess[T, L]) updateObject(ctx context.Context, opts metav1.UpdateOptions, obj T) (t T, err error) {
	if opts.DryRun != nil {
		err = fmt.Errorf("%w: dry run not implemented for %T.Update", commonerrors.ErrUnimplemented, obj)
		return
	}
	err = a.view.UpdateObject(a.gvk, obj)
	if err != nil {
		return
	}
	t, err = a.getObject(ctx, obj.GetName(), obj.GetNamespace(), metav1.GetOptions{})
	return
}

func (a *BasicResourceAccess[T, L]) deleteObject(_ context.Context, opts metav1.DeleteOptions, namespace, name string) error {
	if opts.DryRun != nil {
		return fmt.Errorf("%w: dry run not implemented for Delete of %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
	}
	return a.view.DeleteObject(a.gvk, cache.NewObjectName(namespace, name))
}

func (a *BasicResourceAccess[T, L]) deleteObjectCollection(ctx context.Context, namespace string, delOpts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	if delOpts.DryRun != nil {
		return fmt.Errorf("%w: dry run not implemented for DeleteCollection of %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
	}
	if delOpts.PropagationPolicy != nil {
		return fmt.Errorf("%w: propagationPolicy is unimplemented for DeleteCollection of %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
	}
	if delOpts.PropagationPolicy != nil {
		return fmt.Errorf("%w: gracePeriodSeconds is unimplemented for DeleteCollection of %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
	}
	err := checkLogListOptions(ctx, listOpts)
	if err != nil {
		return err
	}
	c, err := asMatchCriteria(namespace, listOpts)
	if err != nil {
		return err
	}
	return a.view.DeleteObjects(a.gvk, c)
}

func (a *BasicResourceAccess[T, L]) getObject(_ context.Context, namespace, name string, opts metav1.GetOptions) (t T, err error) {
	objName := cache.NewObjectName(namespace, name)
	obj, err := a.view.GetObject(a.gvk, objName)
	if err != nil {
		return
	}
	t, err = objutil.Cast[T](obj)
	if err != nil {
		return
	}
	if opts.ResourceVersion != "" && opts.ResourceVersion != t.GetResourceVersion() {
		// TODO: FIXME: I need gvr inside BasicResourceAccess for this error, using gvk is bad.
		err = errors.NewConflict(corev1.Resource(a.gvk.Kind), name, fmt.Errorf("requested ResourceVersion %s does not match current %s", opts.ResourceVersion, t.GetResourceVersion()))
	}
	return
}

func (a *BasicResourceAccess[T, L]) getObjectList(ctx context.Context, namespace string, opts metav1.ListOptions) (l L, err error) {
	err = checkLogListOptions(ctx, opts)
	if err != nil {
		return
	}
	c, err := asMatchCriteria(namespace, opts)
	if err != nil {
		return
	}
	listObj, err := a.view.ListObjects(a.gvk, c)
	if err != nil {
		return
	}
	return objutil.Cast[L](listObj)
}

func (a *BasicResourceAccess[T, L]) getWatcher(ctx context.Context, namespace string, opts metav1.ListOptions) (w watch.Interface, err error) {
	err = checkLogListOptions(ctx, opts)
	if err != nil {
		return
	}
	return a.view.GetWatcher(ctx, a.gvk, namespace, opts)
}

func (a *BasicResourceAccess[T, L]) patchObject(_ context.Context, name string, pt types.PatchType, patchData []byte, opts metav1.PatchOptions) (t T, err error) {
	if opts.DryRun != nil {
		err = fmt.Errorf("%w: dry run not implemented for Patch of %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
		return
	}
	obj, err := a.view.PatchObject(a.gvk, cache.NewObjectName(a.Namespace, name), pt, patchData)
	if err != nil {
		return
	}
	t, err = objutil.Cast[T](obj)
	return
}

func (a *BasicResourceAccess[T, L]) patchObjectStatus(_ context.Context, name string, patchData []byte) (t T, err error) {
	obj, err := a.view.PatchObjectStatus(a.gvk, cache.NewObjectName(a.Namespace, name), patchData)
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
