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
)

var compareFiles []string

func newCompareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare two or more scenarios side by side",
		Long:  "Applies each scenario independently to the same snapshot and renders a comparison.",
		RunE:  runCompare,
	}

	cmd.Flags().StringArrayVarP(&compareFiles, "file", "f", nil, "Scenario YAML files to compare (specify multiple -f flags)")

	return cmd
}

func runCompare(cmd *cobra.Command, _ []string) error {
	if len(compareFiles) < 2 {
		return fmt.Errorf("at least 2 scenario files required for comparison (use -f flag multiple times)")
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Minute)
	defer cancel()

	scenarios := make([]scenario.Scenario, 0, len(compareFiles))
	for _, path := range compareFiles {
		s, err := scenario.ParseScenarioFile(path)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		scenarios = append(scenarios, s)
	}

	snap, err := collectClusterSnapshot(ctx)
	if err != nil {
		return err
	}

	providerName, region := pricing.DetectProvider(snap.Nodes)
	var pricingProvider pricing.PricingProvider
	if providerName != "" && region != "" {
		pricingProvider, err = pricing.NewProvider(ctx, providerName, region)
		if err != nil {
			klog.V(1).InfoS("Pricing unavailable", "err", err)
		}
	}

	var baselineCost float64
	if pricingProvider != nil {
		baselineCost = calculateMonthlyCostFromSnapshot(snap, pricingProvider, region)
	}

	for i, s := range scenarios {
		mutated, applyErr := scenario.ApplyScenario(s, snap)
		if applyErr != nil {
			return fmt.Errorf("applying scenario %d (%s): %w", i+1, s.Kind(), applyErr)
		}

		riskReport := risk.ScoreOOMRisk(mutated)
		riskReport.OverallLevel = risk.ClassifyRisk(riskReport.ClusterOOM, riskReport.ClusterEviction, riskReport.SchedulingRisk)

		meta := scenario.ScenarioMetadata{Name: fmt.Sprintf("Scenario %d: %s", i+1, s.Kind())}

		report := buildCostReport(meta, baselineCost, baselineCost, riskReport)
		if renderErr := output.Render(os.Stdout, report, outputFormat); renderErr != nil {
			return renderErr
		}
		fmt.Fprintln(os.Stdout)
	}

	return nil
}
