package testutil

import (
	"embed"
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/objutil"
)

const (
	BasicCluster = "basic-cluster"
)

//go:embed testdata/*
var testDataFS embed.FS

// LoadBasicClusterConstraints loads basic cluster constraints from the testdata filesystem.
func LoadBasicClusterConstraints(name string) (*sacorev1alpha1.ClusterScalingConstraint, error) {
	var clusterConstraints sacorev1alpha1.ClusterScalingConstraint
	clusterConstraintsPath := fmt.Sprintf("testdata/%s-constraints.json", name)
	switch name {
	case BasicCluster:
		if err := objutil.LoadIntoRuntimeObj(testDataFS, clusterConstraintsPath, &clusterConstraints); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: unknown %q", commonerrors.ErrUnimplemented, name)
	}
	return &clusterConstraints, nil
}

// LoadBasicClusterSnapshot loads a basic cluster snapshot from the testdata filesystem.
func LoadBasicClusterSnapshot(name string) (*svcapi.ClusterSnapshot, error) {
	var clusterSnapshot svcapi.ClusterSnapshot
	clusterSnapshotPath := fmt.Sprintf("testdata/%s-snapshot.json", name)
	switch name {
	case BasicCluster:
		if err := objutil.LoadJSONIntoObject(testDataFS, clusterSnapshotPath, &clusterSnapshot); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: unknown %q", commonerrors.ErrUnimplemented, name)
	}
	return &clusterSnapshot, nil
}

// ReadSchedulerConfig reads the kube-scheduler configuration from the testdata filesystem.
func ReadSchedulerConfig() ([]byte, error) {
	return testDataFS.ReadFile("testdata/kube-scheduler-config.yaml")
}
