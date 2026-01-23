// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	"embed"
	"fmt"
	"strconv"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// CategoryBasic is the name associated with a basic scenario.
	CategoryBasic = "basic"
)

//go:embed data/*.*
var dataFS embed.FS

// LoadClusterConstraints loads cluster constraints from the sample data filesystem.
func LoadClusterConstraints(categoryName string) (*sacorev1alpha1.ScalingConstraint, error) {
	var clusterConstraints sacorev1alpha1.ScalingConstraint
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

// IncreaseUnscheduledWorkLoad increases the unscheduled pods by delta for the given cluster snapshot
func IncreaseUnscheduledWorkLoad(snapshot *planner.ClusterSnapshot, amount int) error {
	var extra []planner.PodInfo
	for _, upod := range snapshot.GetUnscheduledPods() {
		for i := 1; i <= amount; i++ {
			p := upod
			p.Name = p.Name + "-" + strconv.Itoa(i)
			p.UID = types.UID(fmt.Sprintf("%s-%d", p.UID, i))
			extra = append(extra, p)
		}
	}
	snapshot.Pods = append(snapshot.Pods, extra...)
	return nil
}

// LoadBinPackingSchedulerConfig loads the kube-scheduler configuration from the sample data filesystem.
func LoadBinPackingSchedulerConfig() ([]byte, error) {
	return dataFS.ReadFile("data/bin-packing-scheduler-config.yaml")
}
