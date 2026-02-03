// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// PoolCategory is the enum type for representing pool categories of a sample scaling constraint.
type PoolCategory string

const (
	// PoolCategoryBasicOne is the pool category variant associated with a basic one-pool scaling constraint.
	PoolCategoryBasicOne PoolCategory = "basic-one-pool"

	// PoolCategoryBasicTwo is the pool category variant associated with a basic two-pool scaling constraint.
	PoolCategoryBasicTwo PoolCategory = "basic-two-pool"

	// PoolCategoryBasicThree is the pool category variant associated with a basic three-pool scaling constraint.
	PoolCategoryBasicThree PoolCategory = "basic-three-pool"

	// PoolCategoryBasicMany is the pool category variant associated with a basic many pool scaling constraint
	PoolCategoryBasicMany PoolCategory = "basic-multi-pool"
)

// ResourceCategory is the enum type for categorizing common variations of resource pairs.
type ResourceCategory string

const (
	// ResourceCategoryPea is a category for a resource list that specifies  1cpu and 1Gi.
	ResourceCategoryPea ResourceCategory = "pea"

	// ResourceCategoryBerry is a category for a resource list that nearly fit an AWS m5.large instance type / GCP n2-standard-2 / Azure Standard_D2
	// leaving buffer to account for provider variance and kube and system reserved.
	ResourceCategoryBerry ResourceCategory = "berry"

	// ResourceCategoryHalfBerry is a category for a resource list that when doubled nearly fit an AWS m5.large instance type / GCP n2-standard-2 / Azure Standard_D2
	// leaving buffer to account for provider variance and kube and system reserved.
	ResourceCategoryHalfBerry ResourceCategory = "half-berry"

	// ResourceCategoryGrape is a category for a resource list that when doubled nearly fits an AWS m5.xlarge / GCP n2-standard-4 / Azure Standard_D3
	// leaving buffer to account for provider variance and kube and system reserved.
	ResourceCategoryGrape ResourceCategory = "grape"

	// ResourceCategoryHalfGrape is a category for a resource list that when doubled nearly fits an AWS m5.xlarge / GCP n2-standard-4 / Azure Standard_D3
	// leaving buffer to account for provider variance and kube and system reserved.
	ResourceCategoryHalfGrape ResourceCategory = "half-grape"
)

// AsResourceList creates a corev1.ResourceList for the resources associated with this name
func (c ResourceCategory) AsResourceList() corev1.ResourceList {
	return resourceCategoriesToResourceListMap[c]
}

// AppLabels represents standard k8s app labels
type AppLabels struct {
	Name      string
	Instance  string
	Version   string
	Component string
	PartOf    string
	ManagedBy string
}

// SimplePodMetadata holds the simple pod metadata.
type SimplePodMetadata struct {
	Name      string
	Namespace string
	AppLabels AppLabels
}

// SimplePodTemplateData holds all the pod template data for the simple pod template.
type SimplePodTemplateData struct {
	//Resources map[corev1.ResourceName]string
	Resources corev1.ResourceList
	SimplePodMetadata
}

var (
	allResourceCategories = []ResourceCategory{
		ResourceCategoryPea, ResourceCategoryBerry, ResourceCategoryHalfBerry, ResourceCategoryGrape, ResourceCategoryHalfGrape,
	}
	resourceCategoriesToResourceListMap = map[ResourceCategory]corev1.ResourceList{
		ResourceCategoryPea: {
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		ResourceCategoryBerry: {
			corev1.ResourceCPU:    resource.MustParse("1000m"),
			corev1.ResourceMemory: resource.MustParse("5100Mi"),
		},
		ResourceCategoryHalfBerry: {
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("2500Mi"),
		},
		ResourceCategoryGrape: {
			corev1.ResourceCPU:    resource.MustParse("3"),
			corev1.ResourceMemory: resource.MustParse("13Gi"),
		},
		ResourceCategoryHalfGrape: {
			corev1.ResourceCPU:    resource.MustParse("1500m"),
			corev1.ResourceMemory: resource.MustParse("6400Mi"),
		},
	}
)
