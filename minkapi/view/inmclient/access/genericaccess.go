package access

import (
	"context"
	"fmt"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

// GenericResourceAccess provides helper methods for basic Create, Get, Update, Delete, Patch of k8s object defined by generic parameter T, and k8s list object
// provided by generic parameter L.
type GenericResourceAccess[T metav1.Object, L metav1.ListInterface] struct {
	View      minkapi.View
	GVK       schema.GroupVersionKind
	Namespace string
}

// CreateObject creates an object corresponding to inputObj of type T through the backing minkapi View and returns the stored object.
func (a *GenericResourceAccess[T, L]) CreateObject(ctx context.Context, _ metav1.CreateOptions, inputObj T) (storedObj T, err error) {
	o, err := a.View.CreateObject(ctx, a.GVK, inputObj)
	if err != nil {
		return
	}
	storedObj, err = objutil.Cast[T](o)
	return
}

// CreateObjectWithAccessNamespace creates an object corresponding to the given inputObj of type T via the backing minkapi View after changing its namespace to the default namespace associated with this facade and returns the stored object.
func (a *GenericResourceAccess[T, L]) CreateObjectWithAccessNamespace(ctx context.Context, _ metav1.CreateOptions, inputObj T) (storedObj T, err error) {
	if inputObj.GetNamespace() != a.Namespace {
		inputObj.SetNamespace(a.Namespace)
	}
	o, err := a.View.CreateObject(ctx, a.GVK, inputObj)
	if err != nil {
		return
	}
	storedObj, err = objutil.Cast[T](o)
	return
}

// UpdateObject updates an object of type T in the backing minkapi View and returns the updated object.
func (a *GenericResourceAccess[T, L]) UpdateObject(ctx context.Context, _ metav1.UpdateOptions, obj T) (updatedObj T, err error) {
	err = a.View.UpdateObject(ctx, a.GVK, obj)
	if err != nil {
		return
	}
	updatedObj, err = a.GetObject(ctx, obj.GetName(), obj.GetNamespace(), metav1.GetOptions{})
	return
}

// DeleteObject deletes the specified object identified by namespace and name. Currently the deletion options are ignored
func (a *GenericResourceAccess[T, L]) DeleteObject(ctx context.Context, namespace, name string, _ metav1.DeleteOptions) error {
	return a.View.DeleteObject(ctx, a.GVK, cache.NewObjectName(namespace, name))
}

// DeleteObjectCollection deletes a collection of objects matching the provided namespace and list options.
// Returns an error if any issues occur during deletion or if unsupported delete options are used.
func (a *GenericResourceAccess[T, L]) DeleteObjectCollection(ctx context.Context, namespace string, delOpts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	if delOpts.PropagationPolicy != nil {
		return fmt.Errorf("%w: propagationPolicy is unimplemented for DeleteCollection of %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
	}
	if delOpts.PropagationPolicy != nil {
		return fmt.Errorf("%w: gracePeriodSeconds is unimplemented for DeleteCollection of %q", commonerrors.ErrUnimplemented, a.GVK.Kind)
	}
	err := checkLogListOptions(ctx, listOpts)
	if err != nil {
		return err
	}
	c, err := asMatchCriteria(namespace, listOpts)
	if err != nil {
		return err
	}
	return a.View.DeleteObjects(ctx, a.GVK, c)
}

// GetObject retrieves an object of type T identified by its namespace and name, validates resourceVersion if specified in options, and returns it.
func (a *GenericResourceAccess[T, L]) GetObject(ctx context.Context, namespace, name string, opts metav1.GetOptions) (targetObj T, err error) {
	objName := cache.NewObjectName(namespace, name)
	obj, err := a.View.GetObject(ctx, a.GVK, objName)
	if err != nil {
		return
	}
	targetObj, err = objutil.Cast[T](obj)
	if err != nil {
		return
	}
	err = validateMinimumResourceVersion(opts.ResourceVersion, targetObj.GetResourceVersion())
	return
}

// GetObjectList retrieves a object list of type L in the specified namespace based on the provided list options.
func (a *GenericResourceAccess[T, L]) GetObjectList(ctx context.Context, namespace string, opts metav1.ListOptions) (listObj L, err error) {
	err = checkLogListOptions(ctx, opts)
	if err != nil {
		return
	}
	c, err := asMatchCriteria(namespace, opts)
	if err != nil {
		return
	}
	lobj, err := a.View.ListObjects(ctx, a.GVK, c)
	if err != nil {
		return
	}
	return objutil.Cast[L](lobj)
}

// GetWatcher returns a watch.Interface for the specified namespace and list options to observe changes to resources.
func (a *GenericResourceAccess[T, L]) GetWatcher(ctx context.Context, namespace string, opts metav1.ListOptions) (w watch.Interface, err error) {
	err = checkLogListOptions(ctx, opts)
	if err != nil {
		return
	}
	return a.View.GetWatcher(ctx, a.GVK, namespace, opts)
}

// PatchObject force patches the object of the given name using the given patch type with the given patchData, possibly applying the patch to one or more subResource.
// It is the responsibility of the consumer to implement conflict resolution, if any.
// Currently, only the status subResource may be patched. This maybe extended in the future.
func (a *GenericResourceAccess[T, L]) PatchObject(ctx context.Context, name string, pt types.PatchType, patchData []byte, subResources ...string) (patchedObj T, err error) {
	if len(subResources) > 0 {
		if subResources[0] != "status" {
			err = fmt.Errorf("%w: patch of subResources %q is unsupported for %q", commonerrors.ErrInvalidOptVal, subResources, a.GVK.Kind)
			if err != nil {
				return
			}
		}
		return a.PatchObjectStatus(ctx, name, patchData)
	}
	obj, err := a.View.PatchObject(ctx, a.GVK, cache.NewObjectName(a.Namespace, name), pt, patchData)
	if err != nil {
		return
	}
	patchedObj, err = objutil.Cast[T](obj)
	return
}

// PatchObjectStatus updates the status subResource of the specified object using the provided patch data and returns the patched object or an error.
func (a *GenericResourceAccess[T, L]) PatchObjectStatus(ctx context.Context, name string, patchData []byte) (patchedObj T, err error) {
	obj, err := a.View.PatchObjectStatus(ctx, a.GVK, cache.NewObjectName(a.Namespace, name), patchData)
	if err != nil {
		return
	}
	patchedObj, err = objutil.Cast[T](obj)
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
		if err != nil {
			return
		}
	}
	c.LabelSelector = selector
	c.Namespace = namespace
	return
}

func validateMinimumResourceVersion(desiredResourceVersion string, currentResourceVersion string) error {
	if desiredResourceVersion == "" {
		return nil
	}
	desiredRV, err := objutil.ParseResourceVersion(desiredResourceVersion)
	if err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("invalid resource version %q: %v", desiredResourceVersion, err))
	}
	currentRV, err := objutil.ParseResourceVersion(currentResourceVersion)
	if err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("invalid resource version %q: %v", currentResourceVersion, err))
	}
	if desiredRV > currentRV {
		return apierrors.NewBadRequest(fmt.Sprintf("too large desired resource version: %q, current: %q", desiredRV, currentResourceVersion))
	}
	return nil
}
