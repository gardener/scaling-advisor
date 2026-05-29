// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package simulator provides types and helper functions for all simulator implementations
package simulator

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/gardener/scaling-advisor/minkapi/viewutil"
)

// InitializeRequestView creates a sandbox view over the base view, populates it from the request snapshot,
// and optionally binds PVCs for Immediate volume binding mode. It returns the initialized view.
func InitializeRequestView(ctx context.Context, req *plannerapi.Request, viewAccess minkapi.ViewAccess, simConfig plannerapi.SimulatorConfig) (minkapi.View, error) {
	log := logr.FromContextOrDiscard(ctx)
	requestView, err := viewAccess.GetSandboxViewOverDelegate(ctx, "Request-"+req.ID, viewAccess.GetBaseView())
	if err != nil {
		return nil, err
	}
	if err = PopulateView(ctx, requestView, &req.Snapshot); err != nil {
		return nil, fmt.Errorf("%w: %w", plannerapi.ErrPopulateRequestView, err)
	}
	if simConfig.BindVolumeClaimsForImmediateMode {
		if _, err = volutil.BindClaimsForImmediateMode(ctx, requestView); err != nil {
			return nil, err
		}
	}
	if err = viewutil.LogObjects(ctx, "requestView", requestView); err != nil {
		log.Info("failed to dump requestView objects", "requestView", requestView.GetName(), "error", err)
	}
	return requestView, nil
}

// PopulateView populates the given minkapi.View with the objects in the given ClusterSnapshot.
func PopulateView(ctx context.Context, view minkapi.View, cs *plannerapi.ClusterSnapshot) error {
	if err := view.Reset(); err != nil {
		return err
	}
	for _, pc := range cs.PriorityClasses {
		if _, err := view.CreateObject(ctx, typeinfo.PriorityClassesDescriptor.GVK, &pc, minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, rc := range cs.RuntimeClasses {
		if _, err := view.CreateObject(ctx, typeinfo.RuntimeClassDescriptor.GVK, &rc, minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, sc := range cs.StorageClasses {
		if _, err := view.CreateObject(ctx, typeinfo.StorageClassDescriptor.GVK, &sc, minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, nodeInfo := range cs.Nodes {
		createdObj, err := view.CreateObject(ctx, typeinfo.NodesDescriptor.GVK, nodeutil.AsNode(nodeInfo), minkapi.ObjectOptions{})
		if err != nil {
			return err
		}
		if nodeInfo.CSINodeSpec == nil {
			continue
		}
		csiNode := nodeutil.NewCSINode(nodeInfo.Name, createdObj.GetUID(), *nodeInfo.CSINodeSpec)
		if _, err = view.CreateObject(ctx, typeinfo.CSINodeDescriptor.GVK, csiNode, minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, pvc := range cs.PVCs {
		if _, err := view.CreateObject(ctx, typeinfo.PersistentVolumeClaimsDescriptor.GVK, volutil.AsPVC(pvc), minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, pv := range cs.PVs {
		if _, err := view.CreateObject(ctx, typeinfo.PersistentVolumesDescriptor.GVK, volutil.AsPV(pv), minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, pod := range cs.Pods {
		if _, err := view.CreateObject(ctx, typeinfo.PodsDescriptor.GVK, podutil.AsPod(pod), minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, pdb := range cs.PDBs {
		if _, err := view.CreateObject(ctx, typeinfo.PodDisruptionBudgetDescriptor.GVK, &pdb); err != nil {
			return err
		}
	}
	return nil
}
