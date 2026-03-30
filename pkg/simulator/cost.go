// Copyright 2026 KubeWise Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package simulator

import (
	"github.com/tochemey/kubewise/pkg/collector"
	"github.com/tochemey/kubewise/pkg/pricing"
	"k8s.io/klog/v2"
)

// hoursPerMonth aliases pricing.HoursPerMonth for shorter references in this file.
const hoursPerMonth = pricing.HoursPerMonth

// CostReport holds cost calculation results for baseline and scenario.
type CostReport struct {
	// BaselineMonthlyCost is the current total monthly cost.
	BaselineMonthlyCost float64
	// ScenarioMonthlyCost is the projected monthly cost after the scenario.
	ScenarioMonthlyCost float64
	// Savings is the monthly savings (BaselineMonthlyCost - ScenarioMonthlyCost).
	Savings float64
	// SavingsPercent is the savings as a percentage of baseline.
	SavingsPercent float64
	// PerNode lists per-node cost details for the scenario.
	PerNode []NodeCost
	// PerNamespace maps namespace to its cost breakdown.
	PerNamespace map[string]NamespaceCost
}

// NodeCost holds cost details for a single node.
type NodeCost struct {
	// Name is the node name.
	Name string
	// InstanceType is the instance type.
	InstanceType string
	// HourlyCost is the hourly cost (after spot discount if applicable).
	HourlyCost float64
	// MonthlyCost is the monthly cost (HourlyCost × 730).
	MonthlyCost float64
	// IsSpot indicates whether this node runs spot workloads.
	IsSpot bool
}

// NamespaceCost holds per-namespace cost allocation.
type NamespaceCost struct {
	// Namespace is the namespace name.
	Namespace string
	// BaselineCost is the allocated baseline monthly cost.
	BaselineCost float64
	// ScenarioCost is the allocated scenario monthly cost.
	ScenarioCost float64
	// Savings is the per-namespace monthly savings.
	Savings float64
}

// CalculateCost computes the baseline and scenario monthly costs.
// It uses proportional allocation based on CPU+memory requests for per-namespace costs.
func CalculateCost(original *collector.ClusterSnapshot, simResult *SimulationResult, pricingProvider pricing.PricingProvider, spotDiscount float64) *CostReport {
	report := &CostReport{
		PerNamespace: make(map[string]NamespaceCost),
	}

	if pricingProvider == nil {
		return report
	}

	region := original.Pricing.Region

	// 1. Baseline cost: sum hourly cost of all original nodes × 730
	report.BaselineMonthlyCost = calculateNodesCost(original.Nodes, pricingProvider, region, false, 0)

	// 2. Scenario cost: active nodes in simulation result
	// Determine which nodes are active and whether they host spot pods
	spotNodes := IdentifySpotNodesFromSnapshot(original)
	var scenarioCost float64
	for _, node := range original.Nodes {
		if _, active := simResult.NodeUtilization[node.Name]; !active {
			continue
		}
		hourly := lookupHourlyCost(pricingProvider, node.InstanceType, region)
		isSpot := spotNodes[node.Name]
		if isSpot && spotDiscount > 0 {
			hourly *= 1 - spotDiscount
		}
		monthly := hourly * hoursPerMonth
		scenarioCost += monthly
		report.PerNode = append(report.PerNode, NodeCost{
			Name:         node.Name,
			InstanceType: node.InstanceType,
			HourlyCost:   hourly,
			MonthlyCost:  monthly,
			IsSpot:       isSpot,
		})
	}

	// Account for virtual nodes (from autoscaler) not in original
	for nodeName, util := range simResult.NodeUtilization {
		if util.PodCount == 0 {
			continue
		}
		found := false
		for _, nc := range report.PerNode {
			if nc.Name == nodeName {
				found = true
				break
			}
		}
		if !found {
			// Virtual node — find it in the snapshot
			for _, node := range original.Nodes {
				if node.Name == nodeName {
					hourly := lookupHourlyCost(pricingProvider, node.InstanceType, region)
					isSpot := spotNodes[nodeName]
					if isSpot && spotDiscount > 0 {
						hourly *= 1 - spotDiscount
					}
					monthly := hourly * hoursPerMonth
					scenarioCost += monthly
					report.PerNode = append(report.PerNode, NodeCost{
						Name:         node.Name,
						InstanceType: node.InstanceType,
						HourlyCost:   hourly,
						MonthlyCost:  monthly,
						IsSpot:       isSpot,
					})
					break
				}
			}
		}
	}

	report.ScenarioMonthlyCost = scenarioCost

	// 3. Savings
	report.Savings = report.BaselineMonthlyCost - report.ScenarioMonthlyCost
	if report.BaselineMonthlyCost > 0 {
		report.SavingsPercent = (report.Savings / report.BaselineMonthlyCost) * 100
	}

	// 4. Per-namespace cost allocation (proportional to resource weight)
	report.PerNamespace = allocateNamespaceCosts(
		original.Pods, report.BaselineMonthlyCost,
		original.Pods, report.ScenarioMonthlyCost,
	)

	return report
}

// calculateNodesCost computes total monthly cost for a set of nodes.
func calculateNodesCost(nodes []collector.NodeSnapshot, provider pricing.PricingProvider, region string, applySpot bool, spotDiscount float64) float64 {
	var total float64
	for _, node := range nodes {
		hourly := lookupHourlyCost(provider, node.InstanceType, region)
		if applySpot && spotDiscount > 0 {
			hourly *= 1 - spotDiscount
		}
		total += hourly * hoursPerMonth
	}
	return total
}

// lookupHourlyCost returns the on-demand hourly cost for the given instance type.
// Returns 0 and logs a warning if the instance type has no pricing data.
func lookupHourlyCost(provider pricing.PricingProvider, instanceType, region string) float64 {
	if instanceType == "" {
		return 0
	}
	cost, err := provider.HourlyCost(instanceType, region, false)
	if err != nil {
		klog.V(2).InfoS("No pricing data for instance type", "instanceType", instanceType, "err", err)
		return 0
	}
	return cost
}

// IdentifySpotNodesFromSnapshot determines which nodes host only spot-tagged pods.
func IdentifySpotNodesFromSnapshot(snap *collector.ClusterSnapshot) map[string]bool {
	nodeHasOnDemand := make(map[string]bool)
	nodeHasSpot := make(map[string]bool)

	for _, pod := range snap.Pods {
		if pod.NodeName == "" {
			continue
		}
		if pod.IsSpot {
			nodeHasSpot[pod.NodeName] = true
		} else {
			nodeHasOnDemand[pod.NodeName] = true
		}
	}

	spotNodes := make(map[string]bool)
	for name := range nodeHasSpot {
		if !nodeHasOnDemand[name] {
			spotNodes[name] = true
		}
	}
	return spotNodes
}

// resourceWeight computes a simple weight for proportional cost allocation.
// Uses CPU millicores + memory bytes normalized to a comparable scale.
func resourceWeight(pod collector.PodSnapshot) float64 {
	var cpu, mem int64
	for _, c := range pod.Containers {
		cpu += c.Requests.CPU
		mem += c.Requests.Memory
	}
	// Normalize memory: 1 GiB ≈ 1000 millicores for weighting purposes
	memWeight := float64(mem) / (1024 * 1024 * 1024) * 1000
	return float64(cpu) + memWeight
}

// allocateNamespaceCosts distributes costs proportionally by resource weight.
func allocateNamespaceCosts(baselinePods []collector.PodSnapshot, baselineTotal float64, scenarioPods []collector.PodSnapshot, scenarioTotal float64) map[string]NamespaceCost {
	baselineWeights := computeNamespaceWeights(baselinePods)
	scenarioWeights := computeNamespaceWeights(scenarioPods)

	// Merge all namespace keys
	allNS := make(map[string]bool)
	for ns := range baselineWeights {
		allNS[ns] = true
	}
	for ns := range scenarioWeights {
		allNS[ns] = true
	}

	result := make(map[string]NamespaceCost, len(allNS))
	for ns := range allNS {
		bc := baselineTotal * baselineWeights[ns]
		sc := scenarioTotal * scenarioWeights[ns]
		result[ns] = NamespaceCost{
			Namespace:    ns,
			BaselineCost: bc,
			ScenarioCost: sc,
			Savings:      bc - sc,
		}
	}
	return result
}

// computeNamespaceWeights computes the fraction of total resource weight per namespace.
func computeNamespaceWeights(pods []collector.PodSnapshot) map[string]float64 {
	nsWeight := make(map[string]float64)
	var totalWeight float64
	for _, pod := range pods {
		w := resourceWeight(pod)
		nsWeight[pod.Namespace] += w
		totalWeight += w
	}

	if totalWeight == 0 {
		return nsWeight
	}
	for ns := range nsWeight {
		nsWeight[ns] /= totalWeight
	}
	return nsWeight
}
