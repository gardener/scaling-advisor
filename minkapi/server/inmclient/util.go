// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
	"context"
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func createObject[T metav1.Object](ctx context.Context, view minkapi.View, gvk schema.GroupVersionKind, opts metav1.CreateOptions, obj T) (t T, err error) {
	if opts.DryRun != nil {
		err = fmt.Errorf("%w: dry run not implemented for %T.Create", commonerrors.ErrUnimplemented, obj)
		return
	}
	err = view.CreateObject(gvk, obj)
	if err != nil {
		return
	}
	t, err = getObject[T](ctx, view, gvk, obj.GetName(), obj.GetNamespace())
	return
}

func getObject[T metav1.Object](_ context.Context, view minkapi.View, gvk schema.GroupVersionKind, namespace, name string) (t T, err error) {
	objName := cache.NewObjectName(namespace, name)
	obj, err := view.GetObject(gvk, objName)
	if err != nil {
		return
	}
	return objutil.Cast[T](obj)
}

func getObjectList[T metav1.ListInterface](ctx context.Context, view minkapi.View, gvk schema.GroupVersionKind, namespace string, opts metav1.ListOptions) (t T, err error) {
	err = checkLogListOptions(ctx, opts)
	if err != nil {
		return
	}
	c, err := asMatchCriteria(namespace, opts)
	if err != nil {
		return
	}
	listObj, err := view.ListObjects(gvk, c)
	if err != nil {
		return
	}
	return objutil.Cast[T](listObj)
}
func getWatcher(ctx context.Context, view minkapi.View, gvk schema.GroupVersionKind, namespace string, opts metav1.ListOptions) (w watch.Interface, err error) {
	err = checkLogListOptions(ctx, opts)
	if err != nil {
		return
	}
	return view.GetWatcher(ctx, gvk, namespace, opts)
}

func checkLogListOptions(ctx context.Context, opts metav1.ListOptions) error {
	log := logr.FromContextOrDiscard(ctx)
	logUnimplementedOptionalListOptions(log, opts)
	return checkUnimplementedRequiredListOptions(log, opts)
}
func logUnimplementedOptionalListOptions(log logr.Logger, listOptions metav1.ListOptions) {
	if listOptions.AllowWatchBookmarks {
		log.V(4).Info("WatchBookmarks is unimplemented")
	}
	if listOptions.Limit > 0 {
		log.V(4).Info("Limit is unimplemented", "limit", listOptions.Limit)
	}
}

func checkUnimplementedRequiredListOptions(log logr.Logger, listOptions metav1.ListOptions) error {
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
