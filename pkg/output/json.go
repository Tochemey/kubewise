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

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/tochemey/kubewise/pkg/risk"
)

// jsonReport is the JSON-serializable representation of a Report.
type jsonReport struct {
	Scenario           jsonScenario    `json:"scenario"`
	Cost               jsonCost        `json:"cost"`
	Risk               jsonRisk        `json:"risk"`
	NamespaceBreakdown []jsonNamespace `json:"namespaceBreakdown"`
}

type jsonScenario struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type jsonCost struct {
	BaselineMonthly  float64 `json:"baselineMonthly"`
	ProjectedMonthly float64 `json:"projectedMonthly"`
	SavingsMonthly   float64 `json:"savingsMonthly"`
	SavingsPercent   float64 `json:"savingsPercent"`
}

type jsonRisk struct {
	ClusterOOM      float64        `json:"clusterOOM"`
	ClusterEviction float64        `json:"clusterEviction"`
	SchedulingRisk  float64        `json:"schedulingRisk"`
	OverallLevel    string         `json:"overallLevel"`
	Workloads       []jsonWorkload `json:"workloads"`
}

type jsonWorkload struct {
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	OOMRisk      float64 `json:"oomRisk"`
	EvictionRisk float64 `json:"evictionRisk"`
	Level        string  `json:"level"`
}

type jsonNamespace struct {
	Namespace       string           `json:"namespace"`
	MonthlyCost     float64          `json:"monthlyCost,omitempty"`
	Savings         float64          `json:"savings"`
	CPURequested    int64            `json:"cpuRequested,omitempty"`
	MemoryRequested int64            `json:"memoryRequested,omitempty"`
	RiskLevel       string           `json:"riskLevel"`
	Workloads       []jsonNSWorkload `json:"workloads,omitempty"`
}

type jsonNSWorkload struct {
	Name        string  `json:"name"`
	MonthlyCost float64 `json:"monthlyCost,omitempty"`
	Savings     float64 `json:"savings"`
	RiskLevel   string  `json:"riskLevel"`
}

// RenderJSON writes the report as indented JSON to the writer.
// Map keys are sorted alphabetically for stable diff output.
func RenderJSON(w io.Writer, report Report) error {
	jr := toJSONReport(report)

	data, err := json.MarshalIndent(jr, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON report: %w", err)
	}

	_, err = w.Write(append(data, '\n'))
	return err
}

func toJSONReport(report Report) jsonReport {
	jr := jsonReport{
		Scenario: jsonScenario{
			Name:        report.ScenarioName,
			Description: report.ScenarioDesc,
		},
		Cost: jsonCost{
			BaselineMonthly:  roundTo2(report.BaselineCost),
			ProjectedMonthly: roundTo2(report.ProjectedCost),
			SavingsMonthly:   roundTo2(report.Savings),
			SavingsPercent:   roundTo2(report.SavingsPercent),
		},
		Risk: jsonRisk{
			ClusterOOM:      roundTo4(report.Risk.ClusterOOM),
			ClusterEviction: roundTo4(report.Risk.ClusterEviction),
			SchedulingRisk:  roundTo4(report.Risk.SchedulingRisk),
			OverallLevel:    riskLevelString(report.Risk.OverallLevel),
		},
	}

	// Sort workload keys for stable output
	workloadKeys := make([]string, 0, len(report.Risk.PerWorkload))
	for k := range report.Risk.PerWorkload {
		workloadKeys = append(workloadKeys, k)
	}
	sort.Strings(workloadKeys)

	for _, k := range workloadKeys {
		wr := report.Risk.PerWorkload[k]
		jr.Risk.Workloads = append(jr.Risk.Workloads, jsonWorkload{
			Name:         wr.Name,
			Namespace:    wr.Namespace,
			OOMRisk:      roundTo4(wr.OOMRisk),
			EvictionRisk: roundTo4(wr.EvictionRisk),
			Level:        riskLevelString(wr.Level),
		})
	}

	for _, ns := range report.NamespaceBreakdown {
		jns := jsonNamespace{
			Namespace:       ns.Namespace,
			MonthlyCost:     roundTo2(ns.MonthlyCost),
			Savings:         roundTo2(ns.Savings),
			CPURequested:    ns.CPURequested,
			MemoryRequested: ns.MemoryRequested,
			RiskLevel:       riskLevelString(ns.RiskLevel),
		}
		for _, wl := range ns.Workloads {
			jns.Workloads = append(jns.Workloads, jsonNSWorkload{
				Name:        wl.Name,
				MonthlyCost: roundTo2(wl.MonthlyCost),
				Savings:     roundTo2(wl.Savings),
				RiskLevel:   riskLevelString(wl.RiskLevel),
			})
		}
		jr.NamespaceBreakdown = append(jr.NamespaceBreakdown, jns)
	}

	return jr
}

func riskLevelString(level risk.RiskLevel) string {
	switch level {
	case risk.RiskGreen:
		return "green"
	case risk.RiskAmber:
		return "amber"
	case risk.RiskRed:
		return "red"
	case risk.RiskUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

func roundTo2(v float64) float64 {
	return math.Round(v*100) / 100
}

func roundTo4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
