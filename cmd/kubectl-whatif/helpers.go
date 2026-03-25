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
	"github.com/tochemey/kubewise/pkg/collector"
	"github.com/tochemey/kubewise/pkg/output"
	"github.com/tochemey/kubewise/pkg/risk"
	"github.com/tochemey/kubewise/pkg/scenario"
	"github.com/tochemey/kubewise/pkg/simulator"
)

// buildCostReport constructs an output.Report from cost, risk, and simulation data.
func buildCostReport(meta scenario.ScenarioMetadata, snap *collector.ClusterSnapshot, costReport *simulator.CostReport, riskReport *risk.RiskReport, simResult *simulator.SimulationResult) output.Report {
	report := output.Report{
		ScenarioName: meta.Name,
		ScenarioDesc: meta.Description,
		Verbose:      verbose,
		NoColor:      noColor,
	}

	if riskReport != nil {
		report.Risk = *riskReport
	} else {
		report.Risk = risk.RiskReport{
			PerWorkload:  make(map[string]risk.WorkloadRisk),
			OverallLevel: risk.RiskGreen,
		}
	}

	if costReport != nil {
		report.BaselineCost = costReport.BaselineMonthlyCost
		report.ProjectedCost = costReport.ScenarioMonthlyCost
		report.Savings = costReport.Savings
		report.SavingsPercent = costReport.SavingsPercent

		for ns, nc := range costReport.PerNamespace {
			nsRisk := risk.RiskGreen
			// Find worst workload risk in this namespace
			for _, wr := range riskReport.PerWorkload {
				if wr.Namespace == ns && wr.Level > nsRisk {
					nsRisk = wr.Level
				}
			}

			summary := output.NamespaceSummary{
				Namespace: ns,
				Savings:   nc.Savings,
				RiskLevel: nsRisk,
			}

			if verbose {
				for _, wr := range riskReport.PerWorkload {
					if wr.Namespace == ns {
						summary.Workloads = append(summary.Workloads, output.WorkloadSummary{
							Name:      wr.Name,
							RiskLevel: wr.Level,
						})
					}
				}
			}

			report.NamespaceBreakdown = append(report.NamespaceBreakdown, summary)
		}
	}

	return report
}
