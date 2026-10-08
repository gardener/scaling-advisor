// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package simulator provides types and helper functions for all simulator implementations
package simulator

import (
	"context"

	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/common/volutil"
)

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
	return nil
}
