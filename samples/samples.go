// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	"embed"
	"fmt"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/service/pricing"
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

// GetInstancePricingAccessWithFakeData loads and parses fake instance pricing data from sample data for testing purposes.
// Returns an implementation of planner.InstancePricingAccess or an error if loading or parsing the data fails.
// Errors are wrapped with planner.ErrLoadInstanceTypeInfo sentinel error.
func GetInstancePricingAccessWithFakeData() (access planner.InstancePricingAccess, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", planner.ErrLoadInstanceTypeInfo, err)
		}
	}()
	testData, err := dataFS.ReadFile("data/fake-instance_price_infos.json")
	if err != nil {
		return
	}
	return pricing.GetInstancePricingFromData(commontypes.CloudProviderAWS, testData)
}

// GetInstancePricingAccessForTop20AWSInstanceTypes loads pricing data for the top 20 AWS instance types in eu-west-1 region and
// Returns an implementation of planner.InstancePricingAccess or an error if loading or parsing the data fails.
// Errors are wrapped with planner.ErrLoadInstanceTypeInfo sentinel error.
func GetInstancePricingAccessForTop20AWSInstanceTypes() (access planner.InstancePricingAccess, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", planner.ErrLoadInstanceTypeInfo, err)
		}
	}()
	testData, err := dataFS.ReadFile("data/aws_eu-west-1_top20_instance_pricing.json")
	if err != nil {
		return
	}
	return pricing.GetInstancePricingFromData(commontypes.CloudProviderAWS, testData)
}
