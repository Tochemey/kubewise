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
	spotMinReplicas     int
	spotDiscount        float64
	spotExcludeNS       string
	spotControllerTypes string
)

func newSpotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spot",
		Short: "Simulate migrating eligible workloads to spot instances",
		Long: `Identifies workloads eligible for spot/preemptible instances and calculates
the cost savings and eviction risk.`,
		RunE: runSpot,
	}

	cmd.Flags().IntVar(&spotMinReplicas, "min-replicas", 2, "Minimum replica count for spot eligibility")
	cmd.Flags().Float64Var(&spotDiscount, "discount", 0.65, "Spot discount fraction (0.65 = 65% off)")
	cmd.Flags().StringVar(&spotExcludeNS, "exclude-namespace", "kube-system", "Comma-separated namespaces to exclude")
	cmd.Flags().StringVar(&spotControllerTypes, "controller-types", "Deployment,ReplicaSet", "Comma-separated eligible controller types")

	return cmd
}

func runSpot(cmd *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
	defer cancel()

	snap, err := collectClusterSnapshot(ctx)
	if err != nil {
		return err
	}

	ss := &scenario.SpotMigrateScenario{
		Meta: scenario.ScenarioMetadata{
			Name:        "Spot migration",
			Description: fmt.Sprintf("Spot migration (min %d replicas, %.0f%% discount)", spotMinReplicas, spotDiscount*100),
		},
		MinReplicas:       spotMinReplicas,
		ControllerTypes:   splitCSV(spotControllerTypes),
		ExcludeNamespaces: splitCSV(spotExcludeNS),
		SpotDiscount:      spotDiscount,
	}

	mutated, err := scenario.ApplyScenario(ss, snap)
	if err != nil {
		return fmt.Errorf("applying spot scenario: %w", err)
	}

	// Calculate costs with spot discount
	providerName, region := pricing.DetectProvider(snap.Nodes)
	var costReport *simulator.CostReport
	if providerName != "" && region != "" {
		pricingProvider, pErr := pricing.NewProvider(ctx, providerName, region)
		if pErr != nil {
			klog.V(1).InfoS("Pricing unavailable", "err", pErr)
		} else {
			// Run simulation to get node utilization
			simResult, simErr := simulator.Simulate(mutated)
			if simErr != nil {
				return fmt.Errorf("running simulation: %w", simErr)
			}
			costReport = simulator.CalculateCost(snap, simResult, pricingProvider, spotDiscount)
		}
	}

	// Score risk (OOM + eviction)
	riskReport := risk.ScoreOOMRisk(mutated)

	// Add eviction risk per workload
	for key, wr := range riskReport.PerWorkload {
		replicas := findControllerReplicas(snap, wr.Namespace, wr.Name)
		if replicas > 0 {
			instanceType := findNodeInstanceTypeForNS(snap, wr.Namespace)
			evRisk := risk.SpotEvictionRisk(instanceType, int(replicas))
			wr.EvictionRisk = evRisk
			newLevel := risk.ClassifyRisk(wr.OOMRisk, evRisk, 0)
			if newLevel > wr.Level {
				wr.Level = newLevel
			}
			riskReport.PerWorkload[key] = wr
		}
	}

	report := buildCostReport(ss.Meta, snap, costReport, riskReport, nil)
	return output.Render(os.Stdout, report, outputFormat)
}

func findControllerReplicas(snap *collector.ClusterSnapshot, ns, name string) int32 {
	for _, ctrl := range snap.Controllers {
		if ctrl.Namespace == ns && ctrl.Name == name {
			return ctrl.DesiredReplicas
		}
	}
	return 0
}

func findNodeInstanceTypeForNS(snap *collector.ClusterSnapshot, ns string) string {
	for _, pod := range snap.Pods {
		if pod.Namespace == ns && pod.NodeName != "" {
			for _, node := range snap.Nodes {
				if node.Name == pod.NodeName && node.InstanceType != "" {
					return node.InstanceType
				}
			}
		}
	}
	return "m6i.xlarge" // fallback
}
