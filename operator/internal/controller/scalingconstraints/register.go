// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scalingconstraints

import (
	"context"
	corev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const controllerName = "scaling-constraints-controller"

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return builder.ControllerManagedBy(mgr).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: *r.config.ConcurrentSyncs,
		}).
		For(&corev1alpha1.ClusterScalingConstraint{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&corev1alpha1.ClusterScalingFeedback{},
			handler.EnqueueRequestsFromMapFunc(mapScalingFeedbackToScalingConstraints()),
			builder.WithPredicates(clusterScalingFeedbackPredicate())).
		Complete(r)
}

func mapScalingFeedbackToScalingConstraints() handler.MapFunc {
	return func(_ context.Context, obj client.Object) []ctrl.Request {
		csa, ok := obj.(*corev1alpha1.ClusterScalingFeedback)
		if !ok {
			return nil
		}
		return []ctrl.Request{{NamespacedName: types.NamespacedName{Name: csa.Spec.ConstraintRef.Name, Namespace: csa.Spec.ConstraintRef.Namespace}}}
	}
}

func clusterScalingFeedbackPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(_ event.CreateEvent) bool { return true },
		DeleteFunc:  func(_ event.DeleteEvent) bool { return false },
		UpdateFunc:  func(_ event.UpdateEvent) bool { return true },
		GenericFunc: func(_ event.GenericEvent) bool { return false },
	}
}
