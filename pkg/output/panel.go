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
)

// panelTitleStyle renders the namespace name in the panel border.
var panelTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))

// RenderPanels renders each namespace as a separate bordered panel,
// preceded by a cluster summary panel.
func RenderPanels(w io.Writer, report Report) error {
	var sb strings.Builder

	// Cluster summary panel
	renderSummaryPanel(&sb, report)

	// Sort namespaces by cost descending
	sorted := make([]NamespaceSummary, len(report.NamespaceBreakdown))
	copy(sorted, report.NamespaceBreakdown)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].MonthlyCost > sorted[j].MonthlyCost
	})

	// Namespace panels
	for _, ns := range sorted {
		renderNamespacePanel(&sb, ns, report.NoColor)
	}

	_, err := fmt.Fprint(w, sb.String())
	return err
}

func renderSummaryPanel(sb *strings.Builder, report Report) {
	var body strings.Builder

	title := fmt.Sprintf("KubeWise: %s", report.ScenarioName)
	if report.ScenarioDesc != "" {
		title = fmt.Sprintf("KubeWise: %s", report.ScenarioDesc)
	}
	if report.NoColor {
		body.WriteString(title)
	} else {
		body.WriteString(headerStyle.Render(title))
	}
	body.WriteString("\n\n")

	fmt.Fprintf(&body, "  Total monthly cost:    %s\n", formatCost(report.BaselineCost))

	oomPct := report.Risk.ClusterOOM * 100
	oomLevel := classifyOOMLevel(report.Risk.ClusterOOM)
	fmt.Fprintf(&body, "  Cluster OOM risk:      %.1f%%  %s\n", oomPct, RenderRiskIndicator(oomLevel, report.NoColor))
	fmt.Fprintf(&body, "  Overall risk:          %s\n", RenderRiskIndicator(report.Risk.OverallLevel, report.NoColor))
	fmt.Fprintf(&body, "  Namespaces:            %d\n", len(report.NamespaceBreakdown))

	content := body.String()
	if report.NoColor {
		sb.WriteString(content)
		sb.WriteString("\n")
	} else {
		sb.WriteString(borderStyle.Render(content))
		sb.WriteString("\n\n")
	}
}

func renderNamespacePanel(sb *strings.Builder, ns NamespaceSummary, noColor bool) {
	var body strings.Builder

	// Panel title: namespace name
	if noColor {
		fmt.Fprintf(&body, "  %s\n\n", ns.Namespace)
	} else {
		fmt.Fprintf(&body, "  %s\n\n", panelTitleStyle.Render(ns.Namespace))
	}

	// Cost and resource summary
	fmt.Fprintf(&body, "  Monthly cost:    %s\n", formatCost(ns.MonthlyCost))
	fmt.Fprintf(&body, "  CPU requested:   %s\n", formatCPU(ns.CPURequested))
	fmt.Fprintf(&body, "  Mem requested:   %s\n", formatMemory(ns.MemoryRequested))
	fmt.Fprintf(&body, "  Risk:            %s\n", RenderRiskIndicator(ns.RiskLevel, noColor))

	// Workload breakdown
	if len(ns.Workloads) > 0 {
		body.WriteString("\n  Workloads:\n")

		var tbl strings.Builder
		tw := tabwriter.NewWriter(&tbl, 0, 0, 4, ' ', 0)
		for _, wl := range ns.Workloads {
			fmt.Fprintf(tw, "    %s\t%s\t%s\n",
				wl.Name,
				formatCost(wl.MonthlyCost),
				RenderRiskIndicator(wl.RiskLevel, noColor),
			)
		}
		tw.Flush()
		body.WriteString(tbl.String())
	}

	content := body.String()
	if noColor {
		// Plain-text separator
		sb.WriteString(strings.Repeat("-", 44))
		sb.WriteString("\n")
		sb.WriteString(content)
		sb.WriteString("\n")
	} else {
		sb.WriteString(borderStyle.Render(content))
		sb.WriteString("\n\n")
	}
}

// formatCPU formats millicores as a human-readable string.
func formatCPU(millis int64) string {
	if millis <= 0 {
		return "0m"
	}
	if millis >= 1000 && millis%1000 == 0 {
		return fmt.Sprintf("%d cores", millis/1000)
	}
	if millis >= 1000 {
		return fmt.Sprintf("%.1f cores", float64(millis)/1000)
	}
	return fmt.Sprintf("%dm", millis)
}

// formatMemory formats bytes as a human-readable string.
func formatMemory(bytes int64) string {
	if bytes <= 0 {
		return "0"
	}
	const (
		ki = 1024
		mi = ki * 1024
		gi = mi * 1024
	)
	switch {
	case bytes >= gi:
		val := float64(bytes) / float64(gi)
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d Gi", int64(val))
		}
		return fmt.Sprintf("%.1f Gi", val)
	case bytes >= mi:
		val := float64(bytes) / float64(mi)
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d Mi", int64(val))
		}
		return fmt.Sprintf("%.1f Mi", val)
	default:
		return fmt.Sprintf("%d Ki", bytes/ki)
	}
}
