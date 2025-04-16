// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crds

import (
	_ "embed"
)

var (
	//go:embed sr.gardener.cloud_clusterscalingconstraints.yaml
	clusterScalingConstraintsCRD string
	//go:embed sr.gardener.cloud_clusterscalingadvices.yaml
	clusterScalingAdviceCRD string
)

// GetClusterScalingConstraintsCRD returns the ClusterScalingConstraints CRD as YAML string.
func GetClusterScalingConstraintsCRD() string {
	return clusterScalingConstraintsCRD
}

// GetClusterScalingAdviceCRD returns the ClusterScalingAdvice CRD as YAML string.
func GetClusterScalingAdviceCRD() string {
	return clusterScalingAdviceCRD
}
