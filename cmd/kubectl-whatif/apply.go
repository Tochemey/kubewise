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

	"github.com/tochemey/kubewise/pkg/output"
	"github.com/tochemey/kubewise/pkg/pricing"
	"github.com/tochemey/kubewise/pkg/risk"
	"github.com/tochemey/kubewise/pkg/scenario"
	"github.com/tochemey/kubewise/pkg/simulator"
)

var applyScenarioFile string

func newApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a scenario from a YAML file",
		Long:  "Parses a scenario YAML file, applies it to the cluster snapshot, and renders the results.",
		RunE:  runApply,
	}

	cmd.Flags().StringVarP(&applyScenarioFile, "file", "f", "", "Path to scenario YAML file (required)")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func runApply(cmd *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
	defer cancel()

	// Parse scenario file
	s, err := scenario.ParseScenarioFile(applyScenarioFile)
	if err != nil {
		return fmt.Errorf("parsing scenario file: %w", err)
	}

	snap, err := collectClusterSnapshot(ctx)
	if err != nil {
		return err
	}

	// Apply scenario
	mutated, err := scenario.ApplyScenario(s, snap)
	if err != nil {
		return fmt.Errorf("applying scenario: %w", err)
	}

	// Run simulation if needed (consolidation scenarios need bin-packing)
	var simResult *simulator.SimulationResult
	if s.Kind() == "Consolidate" {
		cs, ok := s.(*scenario.ConsolidateScenario)
		if ok {
			simResult, err = simulator.SimulateWithAutoscaler(mutated, cs.TargetNodeType, cs.TargetAllocatable, cs.MaxNodes)
			if err != nil {
				return fmt.Errorf("running simulation: %w", err)
			}
		}
	}
	if simResult == nil {
		simResult, err = simulator.Simulate(mutated)
		if err != nil {
			return fmt.Errorf("running simulation: %w", err)
		}
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
	riskReport := risk.ScoreOOMRisk(mutated)
	riskReport.SchedulingRisk = risk.SchedulingRisk(len(simResult.UnschedulablePods), len(mutated.Pods))
	riskReport.OverallLevel = risk.ClassifyRisk(riskReport.ClusterOOM, riskReport.ClusterEviction, riskReport.SchedulingRisk)

	meta := scenario.ScenarioMetadata{Name: s.Kind()}
	if rs, ok := s.(*scenario.RightSizeScenario); ok {
		meta = rs.Meta
	} else if cs, ok := s.(*scenario.ConsolidateScenario); ok {
		meta = cs.Meta
	} else if ss, ok := s.(*scenario.SpotMigrateScenario); ok {
		meta = ss.Meta
	}

	report := buildCostReport(meta, snap, costReport, riskReport, simResult)
	return output.Render(os.Stdout, report, outputFormat)
}
