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

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tochemey/kubewise/pkg/collector"
	"github.com/tochemey/kubewise/pkg/output"
	"github.com/tochemey/kubewise/pkg/pricing"
	"github.com/tochemey/kubewise/pkg/risk"
	"github.com/tochemey/kubewise/pkg/scenario"
	"k8s.io/klog/v2"
)

var (
	rsPercentile    string
	rsBuffer        int
	rsScope         string
	rsExcludeNS     string
	rsLimitStrategy string
)

func newRightSizeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rightsize",
		Short: "Simulate right-sizing resource requests based on actual usage",
		Long: `Adjusts resource requests and limits based on actual usage percentiles.
Collects a cluster snapshot, applies right-sizing mutations, scores OOM risk,
and renders the projected savings.`,
		RunE: runRightSize,
	}

	cmd.Flags().StringVar(&rsPercentile, "percentile", "p95", "Usage percentile to base new requests on (p50, p90, p95, p99)")
	cmd.Flags().IntVar(&rsBuffer, "buffer", 20, "Buffer percentage above the percentile (e.g., 20 = 20%)")
	cmd.Flags().StringVar(&rsScope, "scope", "", "Comma-separated namespaces to include (default: all)")
	cmd.Flags().StringVar(&rsExcludeNS, "exclude-namespace", "kube-system", "Comma-separated namespaces to exclude")
	cmd.Flags().StringVar(&rsLimitStrategy, "limit-strategy", "ratio", "Limit strategy: ratio, fixed, unbounded")

	return cmd
}

func runRightSize(cmd *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
	defer cancel()

	// Collect snapshot
	snap, err := collectClusterSnapshot(ctx)
	if err != nil {
		return err
	}

	// Build scope
	scope := buildScope()

	// Build and apply right-size scenario
	rs := &scenario.RightSizeScenario{
		Meta: scenario.ScenarioMetadata{
			Name:        "Right-size simulation",
			Description: fmt.Sprintf("Right-size simulation (%s + %d%% buffer)", rsPercentile, rsBuffer),
		},
		Percentile:    rsPercentile,
		Buffer:        rsBuffer,
		Scope:         scope,
		LimitStrategy: rsLimitStrategy,
	}

	mutated, err := scenario.ApplyScenario(rs, snap)
	if err != nil {
		return fmt.Errorf("applying right-size scenario: %w", err)
	}

	// Score OOM risk on the mutated snapshot
	riskReport := risk.ScoreOOMRisk(mutated)

	// Calculate costs
	providerName, region := pricing.DetectProvider(snap.Nodes)
	var baselineCost, projectedCost float64

	if providerName != "" && region != "" {
		pricingProvider, pErr := pricing.NewProvider(ctx, providerName, region)
		if pErr != nil {
			klog.V(1).InfoS("Could not fetch pricing, cost estimates unavailable", "err", pErr)
		} else {
			baselineCost = calculateMonthlyCostFromSnapshot(snap, pricingProvider, region)
			projectedCost = baselineCost // right-sizing doesn't change node count, but changes utilization
			// For right-sizing, projected cost is based on reduced resource needs
			// which could allow consolidation — for now, show same node cost
			// The real savings come from the potential to consolidate
		}
	}

	savings := baselineCost - projectedCost
	savingsPct := 0.0
	if baselineCost > 0 {
		savingsPct = (savings / baselineCost) * 100
	}

	// Build namespace breakdown
	nsSummaries := buildNamespaceBreakdown(snap, mutated, riskReport)

	report := output.Report{
		ScenarioName:       rs.Meta.Name,
		ScenarioDesc:       rs.Meta.Description,
		BaselineCost:       baselineCost,
		ProjectedCost:      projectedCost,
		Savings:            savings,
		SavingsPercent:     savingsPct,
		Risk:               *riskReport,
		NamespaceBreakdown: nsSummaries,
		Verbose:            verbose,
		NoColor:            noColor,
	}

	return output.Render(os.Stdout, report, outputFormat)
}

func buildScope() scenario.Scope {
	scope := scenario.DefaultScope()

	if rsScope != "" {
		scope.Namespaces = splitCSV(rsScope)
	}
	if rsExcludeNS != "" {
		scope.ExcludeNamespaces = splitCSV(rsExcludeNS)
	}

	return scope
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func calculateMonthlyCostFromSnapshot(snap *collector.ClusterSnapshot, provider pricing.PricingProvider, region string) float64 {
	var total float64
	for _, node := range snap.Nodes {
		cost, err := provider.HourlyCost(node.InstanceType, region, false)
		if err != nil {
			continue
		}
		total += cost * 730 // average hours per month
	}
	return total
}

func buildNamespaceBreakdown(_ *collector.ClusterSnapshot, _ *collector.ClusterSnapshot, riskReport *risk.RiskReport) []output.NamespaceSummary {
	var summaries []output.NamespaceSummary

	for key, wr := range riskReport.PerWorkload {
		found := false
		for i := range summaries {
			if summaries[i].Namespace == wr.Namespace {
				found = true
				if wr.Level > summaries[i].RiskLevel {
					summaries[i].RiskLevel = wr.Level
				}
				break
			}
		}
		if !found {
			summaries = append(summaries, output.NamespaceSummary{
				Namespace: wr.Namespace,
				RiskLevel: wr.Level,
			})
		}
		_ = key
	}

	return summaries
}
