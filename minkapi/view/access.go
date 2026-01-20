// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ minkapi.ViewAccess = (*viewAccess)(nil)

type viewAccess struct {
	baseViewArgs *minkapi.ViewArgs
	baseView     minkapi.View
	sandboxViews map[string]minkapi.View
	mu           sync.Mutex
}

// NewAccess creates a new ViewAccess instance with a default base minkapi.View using the provided context and ViewArgs.
func NewAccess(ctx context.Context, baseViewArgs *minkapi.ViewArgs) (va minkapi.ViewAccess, err error) {
	log := logr.FromContextOrDiscard(ctx)
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", minkapi.ErrCreateView, err)
		}
	}()
	bv, err := createBaseView(ctx, baseViewArgs)
	if err != nil {
		return nil, err
	}
	log.Info("created base view", "name", bv.GetName())
	va = &viewAccess{
		baseView:     bv,
		baseViewArgs: baseViewArgs,
		sandboxViews: make(map[string]minkapi.View),
	}
	return
}

func (v *viewAccess) GetBaseView() minkapi.View {
	return v.baseView
}

func (v *viewAccess) GetSandboxView(ctx context.Context, name string) (minkapi.View, error) {
	return v.GetSandboxViewOverDelegate(ctx, name, v.baseView)
}

// GetSandboxViewOverDelegate is the viewAccess implementation for minkapi.ViewAccess.GetSandboxViewOverDelegate
func (v *viewAccess) GetSandboxViewOverDelegate(ctx context.Context, name string, delegateView minkapi.View) (minkapi.View, error) {
	log := logr.FromContextOrDiscard(ctx)
	v.mu.Lock()
	defer v.mu.Unlock()
	sv, ok := v.sandboxViews[name]
	if ok {
		return sv, nil
	}

	sv, err := NewSandbox(delegateView, &minkapi.ViewArgs{
		Name:        name,
		Scheme:      v.baseViewArgs.Scheme,
		WatchConfig: v.baseViewArgs.WatchConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: cannot create sandbox view %q over delegate view %q: %w", minkapi.ErrCreateView, name, delegateView.GetName(), err)
	}
	v.sandboxViews[name] = sv
	log.Info("created sandbox view", "name", name, "delegateView", delegateView.GetName())
	return sv, nil
}

func (v *viewAccess) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	var errs []error
	for _, sv := range v.sandboxViews {
		if err := sv.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := v.baseView.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func createBaseView(ctx context.Context, viewArgs *minkapi.ViewArgs) (minkapi.View, error) {
	bv, err := NewBase(viewArgs)
	if err != nil {
		return nil, err
	}
	_, err = bv.CreateObject(ctx, typeinfo.NamespacesDescriptor.GVK, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: corev1.NamespaceDefault,
		},
	})
	if err != nil {
		return nil, err
	}
	return bv, nil
}
