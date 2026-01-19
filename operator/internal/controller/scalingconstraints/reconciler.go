// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scalingconstraints

import (
	"context"

	"github.com/gardener/scaling-advisor/api/config/v1alpha1"
	corev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler is the operator controller type responsible for reconciling ClusterScalingConstraints to produce ScalingAdvice for a cluster.
type Reconciler struct {
	client client.Client
	log    logr.Logger
	config v1alpha1.ScalingConstraintsControllerConfig
}

// NewReconciler creates a new instance of Reconciler with the provided manager and configuration.
func NewReconciler(mgr ctrl.Manager, config v1alpha1.ScalingConstraintsControllerConfig) *Reconciler {
	return &Reconciler{
		config: config,
		client: mgr.GetClient(),
		log:    mgr.GetLogger().WithName(controllerName),
	}
}

// Reconcile handles reconciliation for ScalingConstraint objects and produces ClusterScalingAdvice
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.log.WithValues("namespace", req.Namespace, "name", req.Name)

	scalingConstraints := &corev1alpha1.ScalingConstraint{}
	if err := r.client.Get(ctx, req.NamespacedName, scalingConstraints); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("ClusterScalingConstraints not found. Skipping reconcile", "scalingConstraintsObjectKey", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log.Info("received event for clusterScalingConstraints", "objectKey", client.ObjectKeyFromObject(scalingConstraints))
	return ctrl.Result{}, nil
}
