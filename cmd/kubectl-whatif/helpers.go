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
	"github.com/tochemey/kubewise/pkg/output"
	"github.com/tochemey/kubewise/pkg/risk"
	"github.com/tochemey/kubewise/pkg/scenario"
)

// buildCostReport constructs an output.Report from cost and risk data.
func buildCostReport(meta scenario.ScenarioMetadata, baselineCost, projectedCost float64, riskReport *risk.RiskReport) output.Report {
	report := output.Report{
		ScenarioName: meta.Name,
		ScenarioDesc: meta.Description,
		Verbose:      verbose,
		NoColor:      noColor,
		Layout:       output.LayoutPanel,
	}

	if riskReport != nil {
		report.Risk = *riskReport
	} else {
		report.Risk = risk.RiskReport{
			PerWorkload:  make(map[string]risk.WorkloadRisk),
			OverallLevel: risk.RiskGreen,
		}
	}

	report.BaselineCost = baselineCost
	report.ProjectedCost = projectedCost
	report.Savings = baselineCost - projectedCost
	if baselineCost > 0 {
		report.SavingsPercent = (report.Savings / baselineCost) * 100
	}

	if riskReport != nil {
		nsRisk := make(map[string]risk.RiskLevel)
		for _, wr := range riskReport.PerWorkload {
			if level, ok := nsRisk[wr.Namespace]; !ok || wr.Level > level {
				nsRisk[wr.Namespace] = wr.Level
			}
		}

		for ns, level := range nsRisk {
			summary := output.NamespaceSummary{
				Namespace: ns,
				RiskLevel: level,
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
