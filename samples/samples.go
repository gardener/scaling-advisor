// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	"embed"
	"fmt"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
)

const (
	// CategoryBasic is the name associated with a basic scenario.
	CategoryBasic = "basic"
)

//go:embed data/*.*
var dataFS embed.FS

// LoadClusterConstraints loads cluster constraints from the sample data filesystem.
func LoadClusterConstraints(categoryName string) (*sacorev1alpha1.ClusterScalingConstraint, error) {
	var clusterConstraints sacorev1alpha1.ClusterScalingConstraint
	clusterConstraintsPath := fmt.Sprintf("data/%s-cluster-constraints.json", categoryName)
	switch categoryName {
	case CategoryBasic:
		if err := objutil.LoadIntoRuntimeObj(dataFS, clusterConstraintsPath, &clusterConstraints); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: unknown %q", commonerrors.ErrUnimplemented, categoryName)
	}
	return &clusterConstraints, nil
}

// LoadClusterSnapshot loads a cluster snapshot from the sample data filesystem.
func LoadClusterSnapshot(categoryName string) (*planner.ClusterSnapshot, error) {
	var clusterSnapshot planner.ClusterSnapshot
	clusterSnapshotPath := fmt.Sprintf("data/%s-cluster-snapshot.json", categoryName)
	switch categoryName {
	case CategoryBasic:
		if err := objutil.LoadJSONIntoObject(dataFS, clusterSnapshotPath, &clusterSnapshot); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: unknown %q", commonerrors.ErrUnimplemented, categoryName)
	}
	return &clusterSnapshot, nil
}

// LoadBinPackingSchedulerConfig loads the kube-scheduler configuration from the sample data filesystem.
func LoadBinPackingSchedulerConfig() ([]byte, error) {
	return dataFS.ReadFile("data/bin-packing-scheduler-config.yaml")
}
