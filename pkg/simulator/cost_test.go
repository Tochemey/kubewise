//    Copyright 2026 KubeWise Authors
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package simulator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tochemey/kubewise/pkg/collector"
)

// mockPricing implements pricing.PricingProvider for testing.
type mockPricing struct {
	prices map[string]float64
}

func (m *mockPricing) HourlyCost(instanceType string, _ string, _ bool) (float64, error) {
	if p, ok := m.prices[instanceType]; ok {
		return p, nil
	}
	return 0, nil
}

func (m *mockPricing) Provider() string { return "mock" }

func TestCalculateCostSimple(t *testing.T) {
	// 2 nodes at $0.192/hr → monthly = 2 × 0.192 × 730 = $280.32
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", InstanceType: "m6i.xlarge", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}},
			{Name: "n2", InstanceType: "m6i.xlarge", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}},
		},
		Pods: []collector.PodSnapshot{
			{Name: "p1", Namespace: "default", NodeName: "n1",
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 2_000_000_000}}}},
			{Name: "p2", Namespace: "default", NodeName: "n2",
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 2_000_000_000}}}},
		},
		Pricing: collector.PricingData{Region: "us-east-1"},
	}

	simResult := &SimulationResult{
		NodeUtilization: map[string]NodeUtilization{
			"n1": {NodeName: "n1", CPUUsed: 1000, CPUTotal: 4000, PodCount: 1},
			"n2": {NodeName: "n2", CPUUsed: 1000, CPUTotal: 4000, PodCount: 1},
		},
		TotalNodes: 2,
	}

	provider := &mockPricing{prices: map[string]float64{"m6i.xlarge": 0.192}}
	report := CalculateCost(snap, simResult, provider, 0)

	require.NotNil(t, report)
	assert.InDelta(t, 2*0.192*730, report.BaselineMonthlyCost, 0.01)
	assert.InDelta(t, 2*0.192*730, report.ScenarioMonthlyCost, 0.01)
	assert.InDelta(t, 0, report.Savings, 0.01)
	assert.InDelta(t, 0, report.SavingsPercent, 0.01)
}

func TestCalculateCostConsolidation(t *testing.T) {
	// 3 original nodes, scenario uses only 2 → savings
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", InstanceType: "m6i.xlarge", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}},
			{Name: "n2", InstanceType: "m6i.xlarge", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}},
			{Name: "n3", InstanceType: "m6i.xlarge", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}},
		},
		Pods: []collector.PodSnapshot{
			{Name: "p1", Namespace: "default", NodeName: "n1",
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 2_000_000_000}}}},
		},
		Pricing: collector.PricingData{Region: "us-east-1"},
	}

	// Only n1 is active in the scenario
	simResult := &SimulationResult{
		NodeUtilization: map[string]NodeUtilization{
			"n1": {NodeName: "n1", CPUUsed: 1000, CPUTotal: 4000, PodCount: 1},
		},
		TotalNodes: 1,
	}

	provider := &mockPricing{prices: map[string]float64{"m6i.xlarge": 0.192}}
	report := CalculateCost(snap, simResult, provider, 0)

	baseline := 3 * 0.192 * 730
	scenario := 1 * 0.192 * 730
	assert.InDelta(t, baseline, report.BaselineMonthlyCost, 0.01)
	assert.InDelta(t, scenario, report.ScenarioMonthlyCost, 0.01)
	assert.InDelta(t, baseline-scenario, report.Savings, 0.01)
	assert.InDelta(t, ((baseline-scenario)/baseline)*100, report.SavingsPercent, 0.01)
}

func TestCalculateCostPerNamespaceAllocation(t *testing.T) {
	// 2 namespaces with equal CPU requests → each gets 50%
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", InstanceType: "m6i.xlarge", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}},
		},
		Pods: []collector.PodSnapshot{
			{Name: "web-1", Namespace: "web", NodeName: "n1",
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 4_000_000_000}}}},
			{Name: "api-1", Namespace: "api", NodeName: "n1",
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 4_000_000_000}}}},
		},
		Pricing: collector.PricingData{Region: "us-east-1"},
	}

	simResult := &SimulationResult{
		NodeUtilization: map[string]NodeUtilization{
			"n1": {NodeName: "n1", CPUUsed: 2000, CPUTotal: 4000, PodCount: 2},
		},
		TotalNodes: 1,
	}

	provider := &mockPricing{prices: map[string]float64{"m6i.xlarge": 0.192}}
	report := CalculateCost(snap, simResult, provider, 0)

	totalMonthly := 0.192 * 730
	webCost := report.PerNamespace["web"]
	apiCost := report.PerNamespace["api"]

	// Equal requests → equal allocation
	assert.InDelta(t, totalMonthly/2, webCost.BaselineCost, 0.1)
	assert.InDelta(t, totalMonthly/2, apiCost.BaselineCost, 0.1)
}

func TestCalculateCostPerNamespaceUnequal(t *testing.T) {
	// web has 3x the resources of api → web gets 75%, api gets 25%
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", InstanceType: "m6i.xlarge", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}},
		},
		Pods: []collector.PodSnapshot{
			{Name: "web-1", Namespace: "web", NodeName: "n1",
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 3000, Memory: 12_000_000_000}}}},
			{Name: "api-1", Namespace: "api", NodeName: "n1",
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 4_000_000_000}}}},
		},
		Pricing: collector.PricingData{Region: "us-east-1"},
	}

	simResult := &SimulationResult{
		NodeUtilization: map[string]NodeUtilization{
			"n1": {NodeName: "n1", CPUUsed: 4000, CPUTotal: 4000, PodCount: 2},
		},
		TotalNodes: 1,
	}

	provider := &mockPricing{prices: map[string]float64{"m6i.xlarge": 0.192}}
	report := CalculateCost(snap, simResult, provider, 0)

	totalMonthly := 0.192 * 730
	webCost := report.PerNamespace["web"]
	apiCost := report.PerNamespace["api"]

	// web has 3x resources → gets ~75%
	assert.Greater(t, webCost.BaselineCost, apiCost.BaselineCost)
	assert.InDelta(t, totalMonthly*0.75, webCost.BaselineCost, totalMonthly*0.05)
	assert.InDelta(t, totalMonthly*0.25, apiCost.BaselineCost, totalMonthly*0.05)
}

func TestCalculateCostNilProvider(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", InstanceType: "m6i.xlarge"},
		},
	}
	simResult := &SimulationResult{
		NodeUtilization: map[string]NodeUtilization{},
	}

	report := CalculateCost(snap, simResult, nil, 0)
	assert.InDelta(t, 0, report.BaselineMonthlyCost, 0.01)
}

func TestCalculateCostEmptyCluster(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Pricing: collector.PricingData{Region: "us-east-1"},
	}
	simResult := &SimulationResult{
		NodeUtilization: map[string]NodeUtilization{},
	}

	provider := &mockPricing{prices: map[string]float64{}}
	report := CalculateCost(snap, simResult, provider, 0)

	assert.InDelta(t, 0, report.BaselineMonthlyCost, 0.01)
	assert.InDelta(t, 0, report.ScenarioMonthlyCost, 0.01)
	assert.InDelta(t, 0, report.Savings, 0.01)
}

func TestCalculateCostUnknownInstanceType(t *testing.T) {
	// Unknown instance type → 0 cost, no error
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", InstanceType: "unknown.type", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}},
		},
		Pods: []collector.PodSnapshot{
			{Name: "p1", Namespace: "default", NodeName: "n1",
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 2_000_000_000}}}},
		},
		Pricing: collector.PricingData{Region: "us-east-1"},
	}
	simResult := &SimulationResult{
		NodeUtilization: map[string]NodeUtilization{
			"n1": {NodeName: "n1", PodCount: 1},
		},
	}

	provider := &mockPricing{prices: map[string]float64{"m6i.xlarge": 0.192}}
	report := CalculateCost(snap, simResult, provider, 0)
	assert.InDelta(t, 0, report.BaselineMonthlyCost, 0.01)
}

func TestCalculateCostPerNodeDetails(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", InstanceType: "m6i.xlarge", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}},
			{Name: "n2", InstanceType: "m6i.2xlarge", Allocatable: collector.ResourcePair{CPU: 8000, Memory: 32_000_000_000}},
		},
		Pricing: collector.PricingData{Region: "us-east-1"},
	}
	simResult := &SimulationResult{
		NodeUtilization: map[string]NodeUtilization{
			"n1": {NodeName: "n1", PodCount: 1},
			"n2": {NodeName: "n2", PodCount: 1},
		},
	}

	provider := &mockPricing{prices: map[string]float64{
		"m6i.xlarge":  0.192,
		"m6i.2xlarge": 0.384,
	}}
	report := CalculateCost(snap, simResult, provider, 0)

	assert.Equal(t, 2, len(report.PerNode))
	for _, nc := range report.PerNode {
		switch nc.Name {
		case "n1":
			assert.InDelta(t, 0.192, nc.HourlyCost, 1e-9)
			assert.InDelta(t, 0.192*730, nc.MonthlyCost, 0.01)
		case "n2":
			assert.InDelta(t, 0.384, nc.HourlyCost, 1e-9)
			assert.InDelta(t, 0.384*730, nc.MonthlyCost, 0.01)
		}
	}
}

func TestIdentifySpotNodesFromSnapshot(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Pods: []collector.PodSnapshot{
			{Name: "spot-1", Namespace: "default", NodeName: "n1", IsSpot: true},
			{Name: "spot-2", Namespace: "default", NodeName: "n1", IsSpot: true},
			{Name: "ondemand-1", Namespace: "default", NodeName: "n2", IsSpot: false},
			{Name: "mixed-spot", Namespace: "default", NodeName: "n3", IsSpot: true},
			{Name: "mixed-od", Namespace: "default", NodeName: "n3", IsSpot: false},
		},
	}

	spotNodes := IdentifySpotNodesFromSnapshot(snap)
	assert.True(t, spotNodes["n1"], "n1 has only spot pods")
	assert.False(t, spotNodes["n2"], "n2 has only on-demand pods")
	assert.False(t, spotNodes["n3"], "n3 has mixed pods")
}

func TestResourceWeight(t *testing.T) {
	pod := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 1000, Memory: 1024 * 1024 * 1024}}, // 1000m + 1GiB
		},
	}
	w := resourceWeight(pod)
	// 1000 (CPU) + 1000 (1GiB normalized) = 2000
	assert.InDelta(t, 2000, w, 1)
}
