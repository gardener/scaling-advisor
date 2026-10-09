// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"testing"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	"github.com/gardener/scaling-advisor/api/core/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidateInstancePrice(t *testing.T) {
	fldPath := field.NewPath("price")

	tests := []struct {
		name      string
		wantField string
		wantType  field.ErrorType
		price     v1alpha1.InstancePrice
	}{
		{
			name:  "valid - no resource unit prices",
			price: validInstancePrice(),
		},
		{
			name: "valid - with resource unit prices",
			price: func() v1alpha1.InstancePrice {
				p := validInstancePrice()
				p.ResourceUnitPrice = []v1alpha1.ResourceUnitPrice{
					{Name: corev1.ResourceCPU, Value: "0.048"},
					{Name: corev1.ResourceMemory, Value: "0.006"},
				}
				return p
			}(),
		},
		{
			name: "missing instanceType",
			price: func() v1alpha1.InstancePrice {
				p := validInstancePrice()
				p.InstanceType = ""
				return p
			}(),
			wantField: "price.instanceType",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "missing region",
			price: func() v1alpha1.InstancePrice {
				p := validInstancePrice()
				p.Region = ""
				return p
			}(),
			wantField: "price.region",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "missing capacityType",
			price: func() v1alpha1.InstancePrice {
				p := validInstancePrice()
				p.CapacityType = ""
				return p
			}(),
			wantField: "price.capacityType",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "zero price",
			price: func() v1alpha1.InstancePrice {
				p := validInstancePrice()
				p.Price = "0"
				return p
			}(),
			wantField: "price.price",
			wantType:  field.ErrorTypeInvalid,
		},
		{
			name: "non-numeric price",
			price: func() v1alpha1.InstancePrice {
				p := validInstancePrice()
				p.Price = "free"
				return p
			}(),
			wantField: "price.price",
			wantType:  field.ErrorTypeInvalid,
		},
		{
			name: "scientific notation price rejected",
			price: func() v1alpha1.InstancePrice {
				p := validInstancePrice()
				p.Price = "1e2"
				return p
			}(),
			wantField: "price.price",
			wantType:  field.ErrorTypeInvalid,
		},
		{
			name: "negative price",
			price: func() v1alpha1.InstancePrice {
				p := validInstancePrice()
				p.Price = "-1.0"
				return p
			}(),
			wantField: "price.price",
			wantType:  field.ErrorTypeInvalid,
		},
		{
			name: "resourceUnitPrice - empty name",
			price: func() v1alpha1.InstancePrice {
				p := validInstancePrice()
				p.ResourceUnitPrice = []v1alpha1.ResourceUnitPrice{
					{Name: "", Value: "0.048"},
				}
				return p
			}(),
			wantField: "price.resourceUnitPrice[0].name",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "resourceUnitPrice - duplicate name",
			price: func() v1alpha1.InstancePrice {
				p := validInstancePrice()
				p.ResourceUnitPrice = []v1alpha1.ResourceUnitPrice{
					{Name: corev1.ResourceCPU, Value: "0.048"},
					{Name: corev1.ResourceCPU, Value: "0.050"},
				}
				return p
			}(),
			wantField: "price.resourceUnitPrice[1].name",
			wantType:  field.ErrorTypeDuplicate,
		},
		{
			name: "resourceUnitPrice - zero value",
			price: func() v1alpha1.InstancePrice {
				p := validInstancePrice()
				p.ResourceUnitPrice = []v1alpha1.ResourceUnitPrice{
					{Name: corev1.ResourceCPU, Value: "0"},
				}
				return p
			}(),
			wantField: "price.resourceUnitPrice[0].value",
			wantType:  field.ErrorTypeInvalid,
		},
		{
			name: "resourceUnitPrice - negative value",
			price: func() v1alpha1.InstancePrice {
				p := validInstancePrice()
				p.ResourceUnitPrice = []v1alpha1.ResourceUnitPrice{
					{Name: corev1.ResourceCPU, Value: "-1.0"},
				}
				return p
			}(),
			wantField: "price.resourceUnitPrice[0].value",
			wantType:  field.ErrorTypeInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFieldError(t, ValidateInstancePrice(&tt.price, fldPath), tt.wantField, tt.wantType)
		})
	}
}

func TestValidateInstanceTypeDefSet(t *testing.T) {
	fldPath := field.NewPath("instanceTypeDefSet")

	tests := []struct {
		name      string
		wantField string
		wantType  field.ErrorType
		defSet    v1alpha1.InstanceTypeDefSet
	}{
		{
			name:   "valid",
			defSet: validInstanceTypeDefSet(),
		},
		{
			name: "missing provider",
			defSet: func() v1alpha1.InstanceTypeDefSet {
				s := validInstanceTypeDefSet()
				s.Spec.Provider = ""
				return s
			}(),
			wantField: "instanceTypeDefSet.spec.provider",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "empty instanceTypeDefs",
			defSet: func() v1alpha1.InstanceTypeDefSet {
				s := validInstanceTypeDefSet()
				s.Spec.InstanceTypeDefs = nil
				return s
			}(),
			wantField: "instanceTypeDefSet.spec.instanceTypeDefs",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "duplicate name",
			defSet: func() v1alpha1.InstanceTypeDefSet {
				s := validInstanceTypeDefSet()
				s.Spec.InstanceTypeDefs = append(s.Spec.InstanceTypeDefs, s.Spec.InstanceTypeDefs[0])
				return s
			}(),
			wantField: "instanceTypeDefSet.spec.instanceTypeDefs[1].name",
			wantType:  field.ErrorTypeDuplicate,
		},
		{
			name: "instanceTypeDef - missing name",
			defSet: func() v1alpha1.InstanceTypeDefSet {
				s := validInstanceTypeDefSet()
				s.Spec.InstanceTypeDefs[0].Name = ""
				return s
			}(),
			wantField: "instanceTypeDefSet.spec.instanceTypeDefs[0].name",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "instanceTypeDef - missing architecture",
			defSet: func() v1alpha1.InstanceTypeDefSet {
				s := validInstanceTypeDefSet()
				s.Spec.InstanceTypeDefs[0].Architecture = ""
				return s
			}(),
			wantField: "instanceTypeDefSet.spec.instanceTypeDefs[0].architecture",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "instanceTypeDef - empty baseResources",
			defSet: func() v1alpha1.InstanceTypeDefSet {
				s := validInstanceTypeDefSet()
				s.Spec.InstanceTypeDefs[0].BaseResources = nil
				return s
			}(),
			wantField: "instanceTypeDefSet.spec.instanceTypeDefs[0].baseResources",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "instanceTypeDef - negative maxVolumes",
			defSet: func() v1alpha1.InstanceTypeDefSet {
				s := validInstanceTypeDefSet()
				s.Spec.InstanceTypeDefs[0].MaxVolumes = -1
				return s
			}(),
			wantField: "instanceTypeDefSet.spec.instanceTypeDefs[0].maxVolumes",
			wantType:  field.ErrorTypeInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFieldError(t, ValidateInstanceTypeDefSet(&tt.defSet, fldPath), tt.wantField, tt.wantType)
		})
	}
}

func TestValidateNodeTemplateSetAgainstInstanceTypeDefSet(t *testing.T) {
	fldPath := field.NewPath("nodeTemplateSet")
	defSet := validInstanceTypeDefSet()

	tests := []struct {
		name        string
		wantField   string
		wantType    field.ErrorType
		templateSet v1alpha1.NodeTemplateSet
	}{
		{
			name: "valid - instanceType resolves",
			templateSet: v1alpha1.NodeTemplateSet{
				Spec: v1alpha1.NodeTemplateSetSpec{
					Templates: []v1alpha1.NodeTemplate{
						{Name: "t1", InstanceType: "m5.xlarge", ReservedResources: resourceList(corev1.ResourceCPU, "100m")},
					},
				},
			},
		},
		{
			name: "unknown instanceType",
			templateSet: v1alpha1.NodeTemplateSet{
				Spec: v1alpha1.NodeTemplateSetSpec{
					Templates: []v1alpha1.NodeTemplate{
						{Name: "t1", InstanceType: "x99.unknown", ReservedResources: resourceList(corev1.ResourceCPU, "100m")},
					},
				},
			},
			wantField: "nodeTemplateSet.spec.templates[0].instanceType",
			wantType:  field.ErrorTypeNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFieldError(t, ValidateNodeTemplateSetAgainstInstanceTypeDefSet(&tt.templateSet, &defSet, fldPath), tt.wantField, tt.wantType)
		})
	}
}

func TestValidateInstancePriceSetAgainstInstanceTypeDefSet(t *testing.T) {
	fldPath := field.NewPath("instancePriceSet")
	defSet := validInstanceTypeDefSet()

	tests := []struct {
		name      string
		wantField string
		wantType  field.ErrorType
		priceSet  v1alpha1.InstancePriceSet
	}{
		{
			name: "valid - instanceType resolves",
			priceSet: v1alpha1.InstancePriceSet{
				Spec: v1alpha1.InstancePriceSetSpec{
					Provider: apicommon.CloudProviderAWS,
					Prices: []v1alpha1.InstancePrice{
						{InstancePriceKey: v1alpha1.InstancePriceKey{InstanceType: "m5.xlarge", Region: "us-east-1", CapacityType: "on-demand"}, Price: "0.192"},
					},
				},
			},
		},
		{
			name: "unknown instanceType",
			priceSet: v1alpha1.InstancePriceSet{
				Spec: v1alpha1.InstancePriceSetSpec{
					Provider: apicommon.CloudProviderAWS,
					Prices: []v1alpha1.InstancePrice{
						{InstancePriceKey: v1alpha1.InstancePriceKey{InstanceType: "x99.unknown", Region: "us-east-1", CapacityType: "on-demand"}, Price: "0.192"},
					},
				},
			},
			wantField: "instancePriceSet.spec.prices[0].instanceType",
			wantType:  field.ErrorTypeNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFieldError(t, ValidateInstancePriceSetAgainstInstanceTypeDefSet(&tt.priceSet, &defSet, fldPath), tt.wantField, tt.wantType)
		})
	}
}

func TestValidateInstancePriceSetSpec(t *testing.T) {
	fldPath := field.NewPath("spec")

	tests := []struct {
		name      string
		wantField string
		wantType  field.ErrorType
		spec      v1alpha1.InstancePriceSetSpec
	}{
		{
			name: "valid",
			spec: v1alpha1.InstancePriceSetSpec{
				Provider: apicommon.CloudProviderAWS,
				Prices:   []v1alpha1.InstancePrice{validInstancePrice()},
			},
		},
		{
			name: "individual price entry validated - missing instanceType",
			spec: v1alpha1.InstancePriceSetSpec{
				Provider: apicommon.CloudProviderAWS,
				Prices: []v1alpha1.InstancePrice{
					func() v1alpha1.InstancePrice {
						p := validInstancePrice()
						p.InstanceType = ""
						return p
					}(),
				},
			},
			wantField: "spec.prices[0].instanceType",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "duplicate key",
			spec: v1alpha1.InstancePriceSetSpec{
				Provider: apicommon.CloudProviderAWS,
				Prices:   []v1alpha1.InstancePrice{validInstancePrice(), validInstancePrice()},
			},
			wantField: "spec.prices[1]",
			wantType:  field.ErrorTypeDuplicate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFieldError(t, ValidateInstancePriceSetSpec(&tt.spec, fldPath), tt.wantField, tt.wantType)
		})
	}
}

func TestValidateResourceLimits(t *testing.T) {
	fldPath := field.NewPath("quota")

	tests := []struct {
		name      string
		rl        v1alpha1.ResourceLimits
		wantField string
		wantType  field.ErrorType
	}{
		{
			name: "valid - min less than max",
			rl: v1alpha1.ResourceLimits{
				Min: resourceList(corev1.ResourceCPU, "2"),
				Max: resourceList(corev1.ResourceCPU, "4"),
			},
		},
		{
			name: "valid - min equal to max",
			rl: v1alpha1.ResourceLimits{
				Min: resourceList(corev1.ResourceCPU, "4"),
				Max: resourceList(corev1.ResourceCPU, "4"),
			},
		},
		{
			name: "valid - min only, no max",
			rl: v1alpha1.ResourceLimits{
				Min: resourceList(corev1.ResourceCPU, "2"),
			},
		},
		{
			name: "invalid - min exceeds max",
			rl: v1alpha1.ResourceLimits{
				Min: resourceList(corev1.ResourceCPU, "8"),
				Max: resourceList(corev1.ResourceCPU, "4"),
			},
			wantField: "quota.min[cpu]",
			wantType:  field.ErrorTypeInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFieldError(t, ValidateResourceLimits(&tt.rl, fldPath), tt.wantField, tt.wantType)
		})
	}
}

func TestValidateScalingAdvice(t *testing.T) {
	fldPath := field.NewPath("scalingAdvice")

	tests := []struct {
		name      string
		wantField string
		wantType  field.ErrorType
		advice    v1alpha1.ScalingAdvice
	}{
		{
			name:   "valid - no scale plans",
			advice: validEmptyScalingAdvice(),
		},
		{
			name: "valid - with scaleOut and scaleIn",
			advice: func() v1alpha1.ScalingAdvice {
				a := validEmptyScalingAdvice()
				a.Spec.ScaleOut = &v1alpha1.ScaleOutPlan{Items: []v1alpha1.ScaleOutItem{validScaleOutItem()}}
				a.Spec.ScaleIn = &v1alpha1.ScaleInPlan{Items: []v1alpha1.ScaleInItem{validScaleInItem()}}
				return a
			}(),
		},
		{
			name: "missing constraintRef.name",
			advice: func() v1alpha1.ScalingAdvice {
				a := validEmptyScalingAdvice()
				a.Spec.ConstraintRef.Name = ""
				return a
			}(),
			wantField: "scalingAdvice.spec.constraintRef.name",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "missing constraintRef.namespace",
			advice: func() v1alpha1.ScalingAdvice {
				a := validEmptyScalingAdvice()
				a.Spec.ConstraintRef.Namespace = ""
				return a
			}(),
			wantField: "scalingAdvice.spec.constraintRef.namespace",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "scaleOut - empty items",
			advice: func() v1alpha1.ScalingAdvice {
				a := validEmptyScalingAdvice()
				a.Spec.ScaleOut = &v1alpha1.ScaleOutPlan{Items: nil}
				return a
			}(),
			wantField: "scalingAdvice.spec.scaleOut.items",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "scaleOut - item missing name",
			advice: func() v1alpha1.ScalingAdvice {
				a := validEmptyScalingAdvice()
				item := validScaleOutItem()
				item.Name = ""
				a.Spec.ScaleOut = &v1alpha1.ScaleOutPlan{Items: []v1alpha1.ScaleOutItem{item}}
				return a
			}(),
			wantField: "scalingAdvice.spec.scaleOut.items[0].name",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "scaleOut - item delta zero",
			advice: func() v1alpha1.ScalingAdvice {
				a := validEmptyScalingAdvice()
				item := validScaleOutItem()
				item.Delta = 0
				a.Spec.ScaleOut = &v1alpha1.ScaleOutPlan{Items: []v1alpha1.ScaleOutItem{item}}
				return a
			}(),
			wantField: "scalingAdvice.spec.scaleOut.items[0].delta",
			wantType:  field.ErrorTypeInvalid,
		},
		{
			name: "scaleOut - item negative currentReplicas",
			advice: func() v1alpha1.ScalingAdvice {
				a := validEmptyScalingAdvice()
				item := validScaleOutItem()
				item.CurrentReplicas = -1
				a.Spec.ScaleOut = &v1alpha1.ScaleOutPlan{Items: []v1alpha1.ScaleOutItem{item}}
				return a
			}(),
			wantField: "scalingAdvice.spec.scaleOut.items[0].currentReplicas",
			wantType:  field.ErrorTypeInvalid,
		},
		{
			name: "scaleOut - item missing poolName",
			advice: func() v1alpha1.ScalingAdvice {
				a := validEmptyScalingAdvice()
				item := validScaleOutItem()
				item.NodePlacement.PoolName = ""
				a.Spec.ScaleOut = &v1alpha1.ScaleOutPlan{Items: []v1alpha1.ScaleOutItem{item}}
				return a
			}(),
			wantField: "scalingAdvice.spec.scaleOut.items[0].nodePlacement.poolName",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "scaleIn - empty items",
			advice: func() v1alpha1.ScalingAdvice {
				a := validEmptyScalingAdvice()
				a.Spec.ScaleIn = &v1alpha1.ScaleInPlan{Items: nil}
				return a
			}(),
			wantField: "scalingAdvice.spec.scaleIn.items",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "scaleIn - item missing nodeName",
			advice: func() v1alpha1.ScalingAdvice {
				a := validEmptyScalingAdvice()
				item := validScaleInItem()
				item.NodeName = ""
				a.Spec.ScaleIn = &v1alpha1.ScaleInPlan{Items: []v1alpha1.ScaleInItem{item}}
				return a
			}(),
			wantField: "scalingAdvice.spec.scaleIn.items[0].nodeName",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "diagnostic - missing traceLogName",
			advice: func() v1alpha1.ScalingAdvice {
				a := validEmptyScalingAdvice()
				a.Spec.Diagnostic = &v1alpha1.ScalingAdviceDiagnostic{RunResultsLogName: "run-log"}
				return a
			}(),
			wantField: "scalingAdvice.spec.diagnostic.traceLogName",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "diagnostic - missing runResultsLogName",
			advice: func() v1alpha1.ScalingAdvice {
				a := validEmptyScalingAdvice()
				a.Spec.Diagnostic = &v1alpha1.ScalingAdviceDiagnostic{TraceLogName: "trace-log"}
				return a
			}(),
			wantField: "scalingAdvice.spec.diagnostic.runResultsLogName",
			wantType:  field.ErrorTypeRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFieldError(t, ValidateScalingAdvice(&tt.advice, fldPath), tt.wantField, tt.wantType)
		})
	}
}

func TestValidateScalingConstraint(t *testing.T) {
	fldPath := field.NewPath("scalingConstraint")

	validTemplateSet := v1alpha1.NodeTemplateSet{
		Spec: v1alpha1.NodeTemplateSetSpec{
			Templates: []v1alpha1.NodeTemplate{
				{Name: "m5-standard", InstanceType: "m5.xlarge", ReservedResources: resourceList(corev1.ResourceCPU, "100m", corev1.ResourceMemory, "256Mi")},
			},
		},
	}

	tests := []struct {
		name       string
		wantField  string
		wantType   field.ErrorType
		constraint v1alpha1.ScalingConstraint
	}{
		{
			name:       "valid",
			constraint: validScalingConstraint(),
		},
		{
			name: "missing metadata.name",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Name = ""
				return c
			}(),
			wantField: "scalingConstraint.metadata.name",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "missing metadata.namespace",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Namespace = ""
				return c
			}(),
			wantField: "scalingConstraint.metadata.namespace",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "missing plannerStrategy",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.PlannerStrategy = ""
				return c
			}(),
			wantField: "scalingConstraint.spec.plannerStrategy",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "invalid plannerStrategy",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.PlannerStrategy = "bogus-strategy"
				return c
			}(),
			wantField: "scalingConstraint.spec.plannerStrategy",
			wantType:  field.ErrorTypeNotSupported,
		},
		{
			name: "missing nodePools",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.NodePools = nil
				return c
			}(),
			wantField: "scalingConstraint.spec.nodePools",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "nodePool - empty nodeTemplateRefs",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.NodePools[0].NodeTemplateRefs = nil
				return c
			}(),
			wantField: "scalingConstraint.spec.nodePools[0].nodeTemplateRefs",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "nodePool - nodeTemplateRef empty name",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.NodePools[0].NodeTemplateRefs = []v1alpha1.NodeTemplateRef{{Name: ""}}
				return c
			}(),
			wantField: "scalingConstraint.spec.nodePools[0].nodeTemplateRefs[0].name",
			wantType:  field.ErrorTypeRequired,
		},
		{
			name: "nodePool - scalingParams threshold zero",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.NodePools[0].ScalingParams = &v1alpha1.ScalingParams{
					UtilizationThresholds: []v1alpha1.ResourceUtilizationThreshold{
						{Name: corev1.ResourceCPU, Threshold: "0"},
					},
				}
				return c
			}(),
			wantField: "scalingConstraint.spec.nodePools[0].scalingParams.utilizationThresholds[0].threshold",
			wantType:  field.ErrorTypeInvalid,
		},
		{
			name: "nodePool - scalingParams threshold out of range",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.NodePools[0].ScalingParams = &v1alpha1.ScalingParams{
					UtilizationThresholds: []v1alpha1.ResourceUtilizationThreshold{
						{Name: corev1.ResourceCPU, Threshold: "1.5"},
					},
				}
				return c
			}(),
			wantField: "scalingConstraint.spec.nodePools[0].scalingParams.utilizationThresholds[0].threshold",
			wantType:  field.ErrorTypeInvalid,
		},
		{
			name: "nodePool - scalingParams duplicate threshold name",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.NodePools[0].ScalingParams = &v1alpha1.ScalingParams{
					UtilizationThresholds: []v1alpha1.ResourceUtilizationThreshold{
						{Name: corev1.ResourceCPU, Threshold: "0.5"},
						{Name: corev1.ResourceCPU, Threshold: "0.8"},
					},
				}
				return c
			}(),
			wantField: "scalingConstraint.spec.nodePools[0].scalingParams.utilizationThresholds[1].name",
			wantType:  field.ErrorTypeDuplicate,
		},
		{
			name: "nodePool - scalingParams valid threshold",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.NodePools[0].ScalingParams = &v1alpha1.ScalingParams{
					UtilizationThresholds: []v1alpha1.ResourceUtilizationThreshold{
						{Name: corev1.ResourceCPU, Threshold: "0.5"},
						{Name: corev1.ResourceMemory, Threshold: "1"},
					},
				}
				return c
			}(),
		},
		{
			name: "invalid simulatorStrategy",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.SimulatorStrategy = "bogus-strategy"
				return c
			}(),
			wantField: "scalingConstraint.spec.simulatorStrategy",
			wantType:  field.ErrorTypeNotSupported,
		},
		{
			name: "valid simulatorStrategy",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.SimulatorStrategy = apicommon.SimulatorStrategySingleNodeMultiSim
				return c
			}(),
		},
		{
			name: "invalid scoringStrategy",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.ScoringStrategy = "bogus-scoring"
				return c
			}(),
			wantField: "scalingConstraint.spec.scoringStrategy",
			wantType:  field.ErrorTypeNotSupported,
		},
		{
			name: "valid scoringStrategy",
			constraint: func() v1alpha1.ScalingConstraint {
				c := validScalingConstraint()
				c.Spec.ScoringStrategy = apicommon.NodeScoringStrategyLeastWaste
				return c
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateScalingConstraint(&tc.constraint, &validTemplateSet, fldPath)
			assertFieldError(t, errs, tc.wantField, tc.wantType)
		})
	}
}

func validScalingConstraint() v1alpha1.ScalingConstraint {
	c := v1alpha1.ScalingConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-a",
			Namespace: "tenant-a",
		},
		Spec: v1alpha1.ScalingConstraintSpec{
			PlannerStrategy: apicommon.PlannerStrategyScaleOutThenScaleIn,
			NodePools: []v1alpha1.NodePool{
				{
					Name:              "workers",
					Region:            "eu-west-1",
					AvailabilityZones: []string{"eu-west-1a"},
					NodeTemplateRefs:  []v1alpha1.NodeTemplateRef{{Name: "m5-standard"}},
				},
			},
		},
	}
	return c
}

// validInstanceTypeDefSet returns a minimal valid InstanceTypeDefSet.
func validInstanceTypeDefSet() v1alpha1.InstanceTypeDefSet {
	return v1alpha1.InstanceTypeDefSet{
		Spec: v1alpha1.InstanceTypeDefSetSpec{
			Provider: apicommon.CloudProviderAWS,
			InstanceTypeDefs: []v1alpha1.InstanceTypeDef{
				{
					Name:          "m5.xlarge",
					Architecture:  "amd64",
					BaseResources: resourceList(corev1.ResourceCPU, "4", corev1.ResourceMemory, "16Gi"),
					MaxVolumes:    20,
				},
			},
		},
	}
}

// validInstancePrice returns a minimal valid InstancePrice.
func validInstancePrice() v1alpha1.InstancePrice {
	return v1alpha1.InstancePrice{
		InstancePriceKey: v1alpha1.InstancePriceKey{
			InstanceType: "m5.xlarge",
			Region:       "us-east-1",
			CapacityType: apicommon.CapacityTypeOnDemand,
		},
		Price: "0.192",
	}
}

func validNodePlacement() v1alpha1.NodePlacement {
	return v1alpha1.NodePlacement{
		PoolName:         "workers",
		TemplateName:     "m5-standard",
		InstanceType:     "m5.xlarge",
		Region:           "eu-west-1",
		AvailabilityZone: "eu-west-1a",
	}
}

func validScaleOutItem() v1alpha1.ScaleOutItem {
	return v1alpha1.ScaleOutItem{
		NodePlacement:   validNodePlacement(),
		Name:            "scaleout-0",
		CurrentReplicas: 2,
		Delta:           1,
	}
}

func validScaleInItem() v1alpha1.ScaleInItem {
	return v1alpha1.ScaleInItem{
		NodePlacement: validNodePlacement(),
		NodeName:      "node-abc",
	}
}

func validEmptyScalingAdvice() v1alpha1.ScalingAdvice {
	return v1alpha1.ScalingAdvice{
		Spec: v1alpha1.ScalingAdviceSpec{
			ConstraintRef: apicommon.NamespacedName{Namespace: "tenant-a", Name: "cluster-a"},
		},
	}
}

// assertFieldError checks that errs contains an error of wantType on wantField.
// If wantField is empty it asserts no errors are present.
func assertFieldError(t *testing.T, errs field.ErrorList, wantField string, wantType field.ErrorType) {
	t.Helper()
	if wantField == "" {
		if len(errs) != 0 {
			t.Errorf("expected no errors, got: %v", errs)
		}
		return
	}
	if len(errs) == 0 {
		t.Fatalf("expected error on field %q, got none", wantField)
	}
	for _, e := range errs {
		if e.Field == wantField && e.Type == wantType {
			return
		}
	}
	t.Errorf("expected error type %v on field %q, got: %v", wantType, wantField, errs)
}

// resourceList builds a corev1.ResourceList from alternating (corev1.ResourceName, string) pairs.
func resourceList(pairs ...any) corev1.ResourceList {
	rl := corev1.ResourceList{}
	for i := 0; i < len(pairs); i += 2 {
		rl[pairs[i].(corev1.ResourceName)] = apiresource.MustParse(pairs[i+1].(string))
	}
	return rl
}
