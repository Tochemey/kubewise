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
	"time"

	"github.com/spf13/cobra"
	"github.com/tochemey/kubewise/internal/kube"
	"github.com/tochemey/kubewise/pkg/collector"
	"github.com/tochemey/kubewise/pkg/output"
	"github.com/tochemey/kubewise/pkg/pricing"
	"github.com/tochemey/kubewise/pkg/risk"
	"k8s.io/klog/v2"
)

var snapshotSavePath string

func newSnapshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Snapshot cluster state and show current cost breakdown",
		Long:  "Collects a cluster snapshot, fetches pricing data, and displays the current cost breakdown by namespace.",
		RunE:  runSnapshot,
	}

	cmd.Flags().StringVar(&snapshotSavePath, "save", "", "Save snapshot to JSON file")
	return cmd
}

func runSnapshot(cmd *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
	defer cancel()

	snap, err := collectClusterSnapshot(ctx)
	if err != nil {
		return err
	}

	// Detect provider and fetch pricing
	providerName, region := pricing.DetectProvider(snap.Nodes)
	if providerName != "" && region != "" {
		pricingProvider, pErr := pricing.NewProvider(ctx, providerName, region)
		if pErr != nil {
			klog.V(1).InfoS("Could not fetch pricing data", "err", pErr)
		} else {
			populateSnapshotPricing(snap, pricingProvider, region)
		}
	} else {
		klog.V(1).InfoS("Could not detect cloud provider from node labels")
	}

	// Build report showing current cost
	report := buildSnapshotReport(snap)

	return output.Render(os.Stdout, report, outputFormat)
}

func collectClusterSnapshot(ctx context.Context) (*collector.ClusterSnapshot, error) {
	clientset, err := kube.NewClientSet(kubeconfig, kubeContext)
	if err != nil {
		return nil, fmt.Errorf("connecting to cluster: %w", err)
	}

	metricsClient, err := kube.NewMetricsClientSet(kubeconfig, kubeContext)
	if err != nil {
		klog.V(1).InfoS("Could not create metrics client, continuing without metrics", "err", err)
		metricsClient = nil
	}

	opts := collector.CollectorOptions{
		Namespace: namespace,
		SavePath:  snapshotSavePath,
	}

	snap, err := collector.CollectSnapshot(ctx, clientset, metricsClient, opts)
	if err != nil {
		return nil, fmt.Errorf("collecting snapshot: %w", err)
	}

	// Collect Prometheus data if available
	if prometheusURL == "" {
		prometheusURL = collector.DiscoverPrometheusURL(ctx, clientset)
	}
	if prometheusURL != "" {
		profiles, pErr := collector.CollectPrometheusUsage(ctx, prometheusURL, snap.Pods, 14*24*time.Hour)
		if pErr != nil {
			klog.V(1).InfoS("Prometheus collection failed, continuing without historical data", "err", pErr)
		} else {
			for k, v := range profiles {
				existing := snap.UsageProfile[k]
				// Merge: keep current usage from metrics-server, add percentiles from Prometheus
				existing.P50CPU = v.P50CPU
				existing.P90CPU = v.P90CPU
				existing.P95CPU = v.P95CPU
				existing.P99CPU = v.P99CPU
				existing.P50Memory = v.P50Memory
				existing.P90Memory = v.P90Memory
				existing.P95Memory = v.P95Memory
				existing.P99Memory = v.P99Memory
				existing.DataPoints = v.DataPoints
				snap.UsageProfile[k] = existing
			}
			snap.PrometheusAvailable = true
		}
	}

	return snap, nil
}

func populateSnapshotPricing(snap *collector.ClusterSnapshot, provider pricing.PricingProvider, region string) {
	snap.Pricing = collector.PricingData{
		Provider:        provider.Provider(),
		Region:          region,
		InstancePricing: make(map[string]collector.InstancePricing),
	}

	for _, node := range snap.Nodes {
		if node.InstanceType == "" {
			continue
		}
		if _, exists := snap.Pricing.InstancePricing[node.InstanceType]; exists {
			continue
		}
		onDemand, err := provider.HourlyCost(node.InstanceType, region, false)
		if err != nil {
			klog.V(2).InfoS("No pricing for instance type", "instanceType", node.InstanceType, "err", err)
			continue
		}
		spot, _ := provider.HourlyCost(node.InstanceType, region, true)
		snap.Pricing.InstancePricing[node.InstanceType] = collector.InstancePricing{
			OnDemandHourly: onDemand,
			SpotHourly:     spot,
		}
	}
}

func buildSnapshotReport(snap *collector.ClusterSnapshot) output.Report {
	// Calculate current cost
	var totalMonthlyCost float64
	for _, node := range snap.Nodes {
		if ip, ok := snap.Pricing.InstancePricing[node.InstanceType]; ok {
			totalMonthlyCost += ip.OnDemandHourly * 730
		}
	}

	// Per-namespace cost allocation (proportional to CPU requests)
	nsCosts := computeNamespaceCosts(snap, totalMonthlyCost)

	var nsSummaries []output.NamespaceSummary
	for ns, cost := range nsCosts {
		nsSummaries = append(nsSummaries, output.NamespaceSummary{
			Namespace: ns,
			Savings:   0, // snapshot shows current state, no savings
			RiskLevel: risk.RiskGreen,
		})
		_ = cost // used for future detailed output
	}

	return output.Report{
		ScenarioName:       "Current cluster state",
		ScenarioDesc:       "Snapshot of current cluster cost breakdown",
		BaselineCost:       totalMonthlyCost,
		ProjectedCost:      totalMonthlyCost,
		Savings:            0,
		SavingsPercent:     0,
		Risk:               risk.RiskReport{OverallLevel: risk.RiskGreen, PerWorkload: make(map[string]risk.WorkloadRisk)},
		NamespaceBreakdown: nsSummaries,
		Verbose:            verbose,
		NoColor:            noColor,
	}
}

func computeNamespaceCosts(snap *collector.ClusterSnapshot, totalCost float64) map[string]float64 {
	nsCPU := make(map[string]int64)
	var totalCPU int64

	for _, pod := range snap.Pods {
		for _, c := range pod.Containers {
			nsCPU[pod.Namespace] += c.Requests.CPU
			totalCPU += c.Requests.CPU
		}
	}

	result := make(map[string]float64)
	if totalCPU == 0 {
		return result
	}
	for ns, cpu := range nsCPU {
		result[ns] = totalCost * float64(cpu) / float64(totalCPU)
	}
	return result
}
