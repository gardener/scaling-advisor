// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package crds

import (
	_ "embed"
)

var (
	//go:embed scad.gardener.cloud_scalingadvices.yaml
	scalingAdviceCRD string
	//go:embed scad.gardener.cloud_scalingfeedbacks.yaml
	scalingFeedbackCRD string
	//go:embed scad.gardener.cloud_scalingconstraints.yaml
	scalingConstraintsCRD string
	//go:embed scad.gardener.cloud_nodetemplatesets.yaml
	nodeTemplateSetsCRD string
	//go:embed scad.gardener.cloud_instancepricesets.yaml
	instancePriceSetsCRD string
	//go:embed scad.gardener.cloud_instancetypedefsets.yaml
	instanceTypeDefSetsCRD string
)

// GetScalingAdviceCRD returns the ScalingAdvice CRD as a YAML string.
func GetScalingAdviceCRD() string { return scalingAdviceCRD }

// GetScalingFeedbackCRD returns the ScalingFeedback CRD as a YAML string.
func GetScalingFeedbackCRD() string { return scalingFeedbackCRD }

// GetScalingConstraintsCRD returns the ScalingConstraints CRD as a YAML string.
func GetScalingConstraintsCRD() string { return scalingConstraintsCRD }

// GetNodeTemplateSetsCRD returns the NodeTemplateSets CRD as a YAML string.
func GetNodeTemplateSetsCRD() string { return nodeTemplateSetsCRD }

// GetInstancePriceSetsCRD returns the InstancePriceSets CRD as a YAML string.
func GetInstancePriceSetsCRD() string { return instancePriceSetsCRD }

// GetInstanceTypeDefSetsCRD returns the InstanceTypeDefSets CRD as a YAML string.
func GetInstanceTypeDefSetsCRD() string { return instanceTypeDefSetsCRD }
