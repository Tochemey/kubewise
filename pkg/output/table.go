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
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"

	"github.com/tochemey/kubewise/pkg/risk"
)

// Styles used for terminal rendering.
var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	amberStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
)

// Layout controls how the table renderer displays the report.
const (
	// LayoutFlat renders a single bordered table (default).
	LayoutFlat = ""
	// LayoutPanel renders each namespace as a separate bordered panel.
	LayoutPanel = "panel"
)

// Report captures everything needed to render simulation output.
type Report struct {
	// ScenarioName is the name of the scenario.
	ScenarioName string
	// ScenarioDesc is a human-readable description.
	ScenarioDesc string
	// BaselineCost is the current monthly cost.
	BaselineCost float64
	// ProjectedCost is the simulated monthly cost.
	ProjectedCost float64
	// Savings is the monthly savings (BaselineCost - ProjectedCost).
	Savings float64
	// SavingsPercent is the savings as a percentage.
	SavingsPercent float64
	// Risk is the risk assessment.
	Risk risk.RiskReport
	// NamespaceBreakdown is the per-namespace cost breakdown.
	NamespaceBreakdown []NamespaceSummary
	// Verbose enables per-workload breakdown.
	Verbose bool
	// NoColor disables terminal colors.
	NoColor bool
	// Layout controls the table rendering style ("" = flat, "panel" = stacked panels).
	Layout string
}

// NamespaceSummary holds per-namespace cost and risk data.
type NamespaceSummary struct {
	// Namespace is the namespace name.
	Namespace string
	// MonthlyCost is the allocated monthly cost for this namespace.
	MonthlyCost float64
	// Savings is the monthly savings for this namespace.
	Savings float64
	// CPURequested is the total CPU requested in millicores.
	CPURequested int64
	// MemoryRequested is the total memory requested in bytes.
	MemoryRequested int64
	// RiskLevel is the worst risk level in this namespace.
	RiskLevel risk.RiskLevel
	// Workloads are the per-workload details (populated when verbose=true).
	Workloads []WorkloadSummary
}

// WorkloadSummary holds per-workload cost and risk data.
type WorkloadSummary struct {
	// Name is the workload name.
	Name string
	// MonthlyCost is the allocated monthly cost for this workload.
	MonthlyCost float64
	// Savings is the monthly savings for this workload.
	Savings float64
	// RiskLevel is the risk level for this workload.
	RiskLevel risk.RiskLevel
}

// RenderTable renders a rich terminal table to the writer.
// When report.Layout is LayoutPanel, it delegates to RenderPanels.
func RenderTable(w io.Writer, report Report) error {
	if report.Layout == LayoutPanel {
		return RenderPanels(w, report)
	}
	return renderFlatTable(w, report)
}

func renderFlatTable(w io.Writer, report Report) error {
	var sb strings.Builder

	// Header
	title := fmt.Sprintf("KubeWise: %s", report.ScenarioName)
	if report.ScenarioDesc != "" {
		title = fmt.Sprintf("KubeWise: %s", report.ScenarioDesc)
	}
	if report.NoColor {
		sb.WriteString(title)
	} else {
		sb.WriteString(headerStyle.Render(title))
	}
	sb.WriteString("\n\n")

	// Cost summary
	fmt.Fprintf(&sb, "  Current monthly cost:    %s\n", formatCost(report.BaselineCost))
	fmt.Fprintf(&sb, "  Projected monthly cost:  %s\n", formatCost(report.ProjectedCost))

	savingsLine := fmt.Sprintf("  Savings:                 %s/mo (%.1f%%)", formatCost(report.Savings), report.SavingsPercent)
	savingsLine += "  " + RenderRiskIndicator(report.Risk.OverallLevel, report.NoColor)
	sb.WriteString(savingsLine + "\n")

	// Cluster risk
	oomPct := report.Risk.ClusterOOM * 100
	oomLevel := classifyOOMLevel(report.Risk.ClusterOOM)
	fmt.Fprintf(&sb, "  Cluster OOM risk:        %.1f%%  %s\n", oomPct, RenderRiskIndicator(oomLevel, report.NoColor))

	sb.WriteString("\n")

	// Namespace breakdown
	if len(report.NamespaceBreakdown) > 0 {
		sb.WriteString("  Top savings by namespace:\n")

		// Sort by savings descending
		sorted := make([]NamespaceSummary, len(report.NamespaceBreakdown))
		copy(sorted, report.NamespaceBreakdown)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Savings > sorted[j].Savings
		})

		var tbl strings.Builder
		tw := tabwriter.NewWriter(&tbl, 0, 0, 4, ' ', 0)
		for _, ns := range sorted {
			fmt.Fprintf(tw, "    %s\t%s/mo saved\trisk: %s\n",
				ns.Namespace,
				formatCost(ns.Savings),
				RenderRiskIndicator(ns.RiskLevel, report.NoColor),
			)

			if report.Verbose && len(ns.Workloads) > 0 {
				for _, wl := range ns.Workloads {
					fmt.Fprintf(tw, "      %s\t%s/mo saved\trisk: %s\n",
						wl.Name,
						formatCost(wl.Savings),
						RenderRiskIndicator(wl.RiskLevel, report.NoColor),
					)
				}
			}
		}
		tw.Flush()
		sb.WriteString(tbl.String())
	}

	// Wrap in border
	content := sb.String()
	if !report.NoColor {
		content = borderStyle.Render(content)
	}

	_, err := fmt.Fprint(w, content+"\n")
	return err
}

// RenderRiskIndicator returns a styled risk indicator string.
func RenderRiskIndicator(level risk.RiskLevel, noColor bool) string {
	switch level {
	case risk.RiskGreen:
		indicator := "● low"
		if noColor {
			return indicator
		}
		return greenStyle.Render(indicator)
	case risk.RiskAmber:
		indicator := "● moderate"
		if noColor {
			return indicator
		}
		return amberStyle.Render(indicator)
	case risk.RiskRed:
		indicator := "● high"
		if noColor {
			return indicator
		}
		return redStyle.Render(indicator)
	case risk.RiskUnknown:
		indicator := "? unknown"
		if noColor {
			return indicator
		}
		return dimStyle.Render(indicator)
	default:
		return "? unknown"
	}
}

func formatCost(cost float64) string {
	if cost >= 1000 {
		return fmt.Sprintf("$%.0f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}

func classifyOOMLevel(oomRisk float64) risk.RiskLevel {
	switch {
	case oomRisk < 0:
		return risk.RiskUnknown
	case oomRisk < risk.OOMRiskLowThreshold:
		return risk.RiskGreen
	case oomRisk < risk.OOMRiskModerateThreshold:
		return risk.RiskAmber
	default:
		return risk.RiskRed
	}
}
