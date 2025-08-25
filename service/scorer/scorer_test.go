package scorer

import (
	"github.com/gardener/scaling-advisor/service/api"
	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	"testing"
)

func TestLeastWasteScoringStrategy(t *testing.T) {
	l := LeastWaste{
		instancePricing: nil,
		weights:         map[v1.ResourceName]float64{v1.ResourceCPU: 5.0, v1.ResourceMemory: 1.0},
	}
	tests := map[string]struct {
		input         api.NodeScoreArgs
		expectedErr   error
		expectedScore api.NodeScore
	}{}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := l.Compute(tc.input)
			scoreDiff := cmp.Diff(tc.expectedScore, got)
			errDiff := cmp.Diff(tc.expectedErr, err)
			if scoreDiff != "" {
				t.Fatalf(scoreDiff)
			}
			if errDiff != "" {
				t.Fatalf(errDiff)
			}
		})
	}
}

func TestLeastCostScoringStrategy(t *testing.T) {
	l := LeastCost{
		instancePricing: nil,
		weights:         map[v1.ResourceName]float64{v1.ResourceCPU: 5.0, v1.ResourceMemory: 1.0},
	}
	tests := map[string]struct {
		input         api.NodeScoreArgs
		expectedErr   error
		expectedScore api.NodeScore
	}{}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := l.Compute(tc.input)
			scoreDiff := cmp.Diff(tc.expectedScore, got)
			errDiff := cmp.Diff(tc.expectedErr, err)
			if scoreDiff != "" {
				t.Fatalf(scoreDiff)
			}
			if errDiff != "" {
				t.Fatalf(errDiff)
			}
		})
	}
}
