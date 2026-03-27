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

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	"github.com/tochemey/kubewise/pkg/collector"
	"github.com/tochemey/kubewise/pkg/output"
	"github.com/tochemey/kubewise/pkg/pricing"
	"github.com/tochemey/kubewise/pkg/risk"
	"github.com/tochemey/kubewise/pkg/scenario"
	"github.com/tochemey/kubewise/pkg/simulator"
)

var (
	consNodeType     string
	consMaxNodes     int
	consKeepPool     string
	consTargetCPU    int64
	consTargetMemory int64
)

func newConsolidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "consolidate",
		Short: "Simulate node pool consolidation to a target instance type",
		Long: `Replaces current node types with a target instance type and simulates
bin-packing to determine the minimum number of nodes needed.`,
		RunE: runConsolidate,
	}

	cmd.Flags().StringVar(&consNodeType, "node-type", "", "Target instance type (required)")
	cmd.Flags().IntVar(&consMaxNodes, "max-nodes", 0, "Maximum number of nodes (0 = unlimited)")
	cmd.Flags().StringVar(&consKeepPool, "keep-pool", "", "Comma-separated node pool names to keep")
	cmd.Flags().Int64Var(&consTargetCPU, "target-cpu", 0, "Allocatable CPU millicores for target type")
	cmd.Flags().Int64Var(&consTargetMemory, "target-memory", 0, "Allocatable memory bytes for target type")
	_ = cmd.MarkFlagRequired("node-type")

	return cmd
}

func runConsolidate(cmd *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
	defer cancel()

	snap, err := collectClusterSnapshot(ctx)
	if err != nil {
		return err
	}

	// Determine target allocatable resources
	targetAlloc := collector.ResourcePair{CPU: consTargetCPU, Memory: consTargetMemory}
	if targetAlloc.CPU == 0 || targetAlloc.Memory == 0 {
		detected := detectAllocatableFromNodes(snap.Nodes, consNodeType)
		if detected.CPU > 0 {
			targetAlloc = detected
		} else {
			return fmt.Errorf("target CPU/memory not specified and could not auto-detect for %s (use --target-cpu and --target-memory)", consNodeType)
		}
	}

	// Build keep pools
	var keepPools []string
	if consKeepPool != "" {
		keepPools = splitCSV(consKeepPool)
	}

	// Apply consolidation scenario
	cs := &scenario.ConsolidateScenario{
		Meta: scenario.ScenarioMetadata{
			Name:        "Node consolidation",
			Description: fmt.Sprintf("Consolidate to %s (max %d nodes)", consNodeType, consMaxNodes),
		},
		TargetNodeType:    consNodeType,
		MaxNodes:          consMaxNodes,
		KeepNodePools:     keepPools,
		TargetAllocatable: targetAlloc,
	}

	mutated, err := scenario.ApplyScenario(cs, snap)
	if err != nil {
		return fmt.Errorf("applying consolidation scenario: %w", err)
	}

	// Run simulation with autoscaler
	simResult, err := simulator.SimulateWithAutoscaler(mutated, consNodeType, targetAlloc, consMaxNodes)
	if err != nil {
		return fmt.Errorf("running simulation: %w", err)
	}

	// Calculate costs
	providerName, region := pricing.DetectProvider(snap.Nodes)
	var costReport *simulator.CostReport
	if providerName != "" && region != "" {
		pricingProvider, pErr := pricing.NewProvider(ctx, providerName, region)
		if pErr != nil {
			klog.V(1).InfoS("Pricing unavailable", "err", pErr)
		} else {
			costReport = simulator.CalculateCost(snap, simResult, pricingProvider, 0)
		}
	}

	// Score risk
	schedulingRiskVal := risk.SchedulingRisk(len(simResult.UnschedulablePods), len(mutated.Pods))
	riskReport := risk.RiskReport{
		PerWorkload:    make(map[string]risk.WorkloadRisk),
		SchedulingRisk: schedulingRiskVal,
		OverallLevel:   risk.ClassifyRisk(0, 0, schedulingRiskVal),
	}

	report := buildCostReport(cs.Meta, snap, costReport, &riskReport, simResult)
	return output.Render(os.Stdout, report, outputFormat)
}

// detectAllocatableFromNodes tries to find allocatable resources for a node type
// by looking at existing nodes of that type.
func detectAllocatableFromNodes(nodes []collector.NodeSnapshot, instanceType string) collector.ResourcePair {
	for _, node := range nodes {
		if node.InstanceType == instanceType {
			return node.Allocatable
		}
	}
	return collector.ResourcePair{}
}
