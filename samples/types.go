package samples

import (
	"github.com/gardener/scaling-advisor/common/objutil"
	corev1 "k8s.io/api/core/v1"
)

// PoolCardinality is the enum type for representing pool cardinalities of a scaling scenario.
type PoolCardinality string

const (
	// PoolCardinalityOne is the pool cardinality variant associated with a basic scenario.
	PoolCardinalityOne PoolCardinality = "one-pool"

	// PoolCardinalityTwo is the pool cardinality variant associated with a two-pool scenario.
	PoolCardinalityTwo PoolCardinality = "two-pool"

	// PoolCardinalityThree is the pool cardinality variant associated with a three-pool scenario.
	PoolCardinalityThree PoolCardinality = "three-pool"

	// PoolCardinalityMany is the pool cardinality variant associated with many pools (used for greater than three)
	PoolCardinalityMany PoolCardinality = "multi-pool"
)

type ScenarioVariant string

const (
	// ScenarioBasic represents scaling constraints where no pool has specified any taints, application pod's of the cluster snapshot have specified no tolerations and any application pod that fits any of the pool's nodeTemplates
	// can be scheduled on the pool's nodes.
	ScenarioBasic ScenarioVariant = "basic"
)

//

// ResourcePairsName is the enum type for naming some common variations of resource pairs.
type ResourcePairsName string

const (
	// Pea is a name for resource pairs that specify 1cpu and 1Gi.
	Pea ResourcePairsName = "pea"

	// Berry is a name for resource pairs that nearly fit an AWS m5.large instance type / GCP n2-standard-2 / Azure Standard_D2
	// leaving buffer to account for provider variance and kube and system reserved.
	Berry ResourcePairsName = "berry"

	// HalfBerry is a name for resource pairs that when doubled nearly fit an AWS m5.large instance type / GCP n2-standard-2 / Azure Standard_D2
	// leaving buffer to account for provider variance and kube and system reserved.
	HalfBerry ResourcePairsName = "half-berry"

	// Grape is a name for resource pairs that when doubled nearly fits an AWS m5.xlarge / GCP n2-standard-4 / Azure Standard_D3
	// leaving buffer to account for provider variance and kube and system reserved.
	Grape ResourcePairsName = "grape"

	// HalfGrape is a name for resource pairs that when doubled nearly fits an AWS m5.xlarge / GCP n2-standard-4 / Azure Standard_D3
	// leaving buffer to account for provider variance and kube and system reserved.
	HalfGrape ResourcePairsName = "half-grape"
)

func (c ResourcePairsName) AsResourceList() corev1.ResourceList {
	return objutil.ResourceNameStringValueMapToResourceList(resourcePairsLabelToResourcePairsMap[c])
}
func (c ResourcePairsName) AsResourcePairs() ResourcePairs {
	return resourcePairsLabelToResourcePairsMap[c]
}

type ResourcePairs map[corev1.ResourceName]string

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
	SimplePodMetadata
	Resources map[corev1.ResourceName]string
}

var (
	allResourcePairsLabels = []ResourcePairsName{
		Pea, Berry, HalfBerry, Grape, HalfGrape,
	}
	resourcePairsLabelToResourcePairsMap = map[ResourcePairsName]ResourcePairs{
		Pea: {
			corev1.ResourceCPU:    "1",
			corev1.ResourceMemory: "1Gi",
		},
		Berry: {
			corev1.ResourceCPU:    "1000m",
			corev1.ResourceMemory: "5100Mi",
		},
		HalfBerry: {
			corev1.ResourceCPU:    "500m",
			corev1.ResourceMemory: "2500Mi",
		},
		Grape: {
			corev1.ResourceCPU:    "3",
			corev1.ResourceMemory: "13Gi",
		},
		HalfGrape: {
			corev1.ResourceCPU:    "1500m",
			corev1.ResourceMemory: "6400Mi",
		},
	}
)
