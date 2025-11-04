package view

import (
	"context"
	"errors"
	"fmt"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sync"
)

var _ minkapi.ViewAccess = (*viewAccess)(nil)

type viewAccess struct {
	baseViewArgs *minkapi.ViewArgs
	baseView     minkapi.View
	sandboxViews map[string]minkapi.View
	mu           sync.Mutex
}

func NewAccess(baseViewArgs *minkapi.ViewArgs) (va minkapi.ViewAccess, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", minkapi.ErrCreateView, err)
		}
	}()
	bv, err := createBaseView(context.Background(), baseViewArgs)
	if err != nil {
		return nil, err
	}
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

func (v *viewAccess) GetOrCreateSandboxView(_ context.Context, name string) (minkapi.View, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	sv, ok := v.sandboxViews[name]
	if ok {
		return sv, nil
	}

	sv, err := NewSandbox(v.baseView, &minkapi.ViewArgs{
		Name:        name,
		Scheme:      v.baseViewArgs.Scheme,
		WatchConfig: v.baseViewArgs.WatchConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: cannot create sandbox view %q: %w", minkapi.ErrCreateView, name, err)
	}
	v.sandboxViews[name] = sv
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
