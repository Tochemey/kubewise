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
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tochemey/kubewise/pkg/risk"
)

func newTestReport() Report {
	return Report{
		ScenarioName:   "Right-size simulation",
		ScenarioDesc:   "Right-size simulation (p95 + 20% buffer)",
		BaselineCost:   14230,
		ProjectedCost:  9840,
		Savings:        4390,
		SavingsPercent: 30.8,
		Risk: risk.RiskReport{
			ClusterOOM:   0.008,
			OverallLevel: risk.RiskGreen,
			PerWorkload: map[string]risk.WorkloadRisk{
				"default/web":       {Name: "web", Namespace: "default", OOMRisk: 0.005, Level: risk.RiskGreen},
				"api/api-gateway":   {Name: "api-gateway", Namespace: "api", OOMRisk: 0.003, Level: risk.RiskGreen},
				"data-pipeline/etl": {Name: "etl", Namespace: "data-pipeline", OOMRisk: 0.03, Level: risk.RiskAmber},
			},
		},
		NamespaceBreakdown: []NamespaceSummary{
			{Namespace: "api", Savings: 1200, RiskLevel: risk.RiskGreen},
			{Namespace: "data-pipeline", Savings: 980, RiskLevel: risk.RiskAmber},
			{Namespace: "default", Savings: 640, RiskLevel: risk.RiskGreen},
		},
		NoColor: true,
	}
}

func TestRenderTableContainsCostValues(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := RenderTable(&buf, report)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "$14230")
	assert.Contains(t, output, "$9840")
	assert.Contains(t, output, "$4390")
	assert.Contains(t, output, "30.8%")
}

func TestRenderTableContainsScenarioName(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := RenderTable(&buf, report)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Right-size simulation")
}

func TestRenderTableContainsNamespaces(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := RenderTable(&buf, report)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "api")
	assert.Contains(t, output, "data-pipeline")
	assert.Contains(t, output, "default")
}

func TestRenderTableContainsRiskIndicators(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := RenderTable(&buf, report)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "low")
	assert.Contains(t, output, "moderate")
}

func TestRenderTableContainsOOMRisk(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := RenderTable(&buf, report)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "OOM risk")
	assert.Contains(t, buf.String(), "0.8%")
}

func TestRenderTableSortsByNamespaceSavings(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := RenderTable(&buf, report)
	require.NoError(t, err)

	output := buf.String()
	// api ($1200) should appear before data-pipeline ($980) and default ($640)
	apiIdx := indexOf(output, "api")
	pipelineIdx := indexOf(output, "data-pipeline")
	defaultIdx := indexOf(output, "default")

	assert.Less(t, apiIdx, pipelineIdx)
	assert.Less(t, pipelineIdx, defaultIdx)
}

func TestRenderTableVerboseShowsWorkloads(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()
	report.Verbose = true
	report.NamespaceBreakdown[0].Workloads = []WorkloadSummary{
		{Name: "api-gateway", Savings: 800, RiskLevel: risk.RiskGreen},
		{Name: "api-auth", Savings: 400, RiskLevel: risk.RiskGreen},
	}

	err := RenderTable(&buf, report)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "api-gateway")
	assert.Contains(t, output, "api-auth")
}

func TestRenderTableNonVerboseHidesWorkloads(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()
	report.Verbose = false
	report.NamespaceBreakdown[0].Workloads = []WorkloadSummary{
		{Name: "hidden-workload", Savings: 100, RiskLevel: risk.RiskGreen},
	}

	err := RenderTable(&buf, report)
	require.NoError(t, err)

	assert.NotContains(t, buf.String(), "hidden-workload")
}

func TestRenderTableEmptyNamespaceBreakdown(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()
	report.NamespaceBreakdown = nil

	err := RenderTable(&buf, report)
	require.NoError(t, err)

	assert.NotContains(t, buf.String(), "Top savings by namespace")
}

func TestRenderRiskIndicatorNoColor(t *testing.T) {
	assert.Equal(t, "● low", RenderRiskIndicator(risk.RiskGreen, true))
	assert.Equal(t, "● moderate", RenderRiskIndicator(risk.RiskAmber, true))
	assert.Equal(t, "● high", RenderRiskIndicator(risk.RiskRed, true))
	assert.Equal(t, "? unknown", RenderRiskIndicator(risk.RiskUnknown, true))
}

func TestRenderRiskIndicatorWithColor(t *testing.T) {
	// With color, the output should contain the text plus ANSI codes
	green := RenderRiskIndicator(risk.RiskGreen, false)
	assert.Contains(t, green, "low")

	amber := RenderRiskIndicator(risk.RiskAmber, false)
	assert.Contains(t, amber, "moderate")

	red := RenderRiskIndicator(risk.RiskRed, false)
	assert.Contains(t, red, "high")
}

func TestFormatCost(t *testing.T) {
	assert.Equal(t, "$14230", formatCost(14230))
	assert.Equal(t, "$1000", formatCost(1000))
	assert.Equal(t, "$999.50", formatCost(999.50))
	assert.Equal(t, "$0.19", formatCost(0.192))
}

func TestRenderTableRedRisk(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()
	report.Risk.OverallLevel = risk.RiskRed
	report.Risk.ClusterOOM = 0.08
	report.NoColor = true

	err := RenderTable(&buf, report)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "high")
	assert.Contains(t, output, "8.0%")
}

// helpers

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
