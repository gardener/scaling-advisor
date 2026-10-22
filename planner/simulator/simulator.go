// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package simulator provides types and helper functions for all simulator implementations
package simulator

import (
	"context"

	plannerapi "github.com/gardener/scaling-advisor/planner/api"
	plannerhelper "github.com/gardener/scaling-advisor/planner/api/helper"

	"github.com/gardener/scaling-advisor/common/nodeutil"
	minkapi "github.com/gardener/scaling-advisor/minkapi/api"
)

// PopulateView populates the given minkapi.View with the objects in the given ClusterSnapshot.
func PopulateView(ctx context.Context, view minkapi.View, cs *plannerapi.ClusterSnapshot) error {
	if err := view.Reset(); err != nil {
		return err
	}
	for _, pc := range cs.PriorityClasses {
		if _, err := view.CreateObject(ctx, minkapi.PriorityClassesDescriptor.GVK, &pc, minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, rc := range cs.RuntimeClasses {
		if _, err := view.CreateObject(ctx, minkapi.RuntimeClassDescriptor.GVK, &rc, minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, sc := range cs.StorageClasses {
		if _, err := view.CreateObject(ctx, minkapi.StorageClassDescriptor.GVK, &sc, minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, nodeInfo := range cs.Nodes {
		createdObj, err := view.CreateObject(ctx, minkapi.NodesDescriptor.GVK, plannerhelper.AsNode(nodeInfo), minkapi.ObjectOptions{})
		if err != nil {
			return err
		}
		if nodeInfo.CSINodeSpec == nil {
			continue
		}
		csiNode := nodeutil.NewCSINode(nodeInfo.Name, createdObj.GetUID(), *nodeInfo.CSINodeSpec)
		if _, err = view.CreateObject(ctx, minkapi.CSINodeDescriptor.GVK, csiNode, minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, pvc := range cs.PVCs {
		if _, err := view.CreateObject(ctx, minkapi.PersistentVolumeClaimsDescriptor.GVK, plannerhelper.AsPVC(pvc), minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, pv := range cs.PVs {
		if _, err := view.CreateObject(ctx, minkapi.PersistentVolumesDescriptor.GVK, plannerhelper.AsPV(pv), minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	for _, pod := range cs.Pods {
		if _, err := view.CreateObject(ctx, minkapi.PodsDescriptor.GVK, plannerhelper.AsPod(pod), minkapi.ObjectOptions{}); err != nil {
			return err
		}
	}
	return nil
}
