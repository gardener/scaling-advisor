// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bench

const (
	// ScalerClusterAutoscaler specifies that the scaler being invoked is CA
	ScalerClusterAutoscaler = "cluster-autoscaler"
	// ScalerKarpenter specifies that the scaler being invoked is karpenter
	ScalerKarpenter = "karpenter"

	// FileNameCAKwokProviderConfig is the filename used for storing CA kwok provider configuration
	FileNameCAKwokProviderConfig = "ca-kwok-provider-config.yaml"
	// FileNameCAKwokProviderTemplate is the filename used for storing CA kwok provider node templates
	FileNameCAKwokProviderTemplate = "ca-kwok-provider-template.yaml"

	// FileNameKarpenterInstanceTypes is the filename used for storing all instance types
	FileNameKarpenterInstanceTypes = "instance_types.json"
	// FileNameKarpenterNodePools is used for storing the NodePools deployed during harness execution
	FileNameKarpenterNodePools = "node_pools.yaml"
	// FileNameKarpenterNodeClasses is used for storing the KWOKNodeClasses
	FileNameKarpenterNodeClasses = "node_classes.yaml"
)
