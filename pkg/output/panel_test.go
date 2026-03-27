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

func newPanelTestReport() Report {
	return Report{
		ScenarioName: "Current cluster state",
		ScenarioDesc: "Snapshot of current cluster cost breakdown",
		BaselineCost: 14230,
		Risk: risk.RiskReport{
			OverallLevel: risk.RiskGreen,
			PerWorkload:  make(map[string]risk.WorkloadRisk),
		},
		NamespaceBreakdown: []NamespaceSummary{
			{
				Namespace:       "api",
				MonthlyCost:     1200,
				CPURequested:    4000,
				MemoryRequested: 8 * 1024 * 1024 * 1024,
				RiskLevel:       risk.RiskGreen,
				Workloads: []WorkloadSummary{
					{Name: "api-gateway", MonthlyCost: 800, RiskLevel: risk.RiskGreen},
					{Name: "api-auth", MonthlyCost: 400, RiskLevel: risk.RiskGreen},
				},
			},
			{
				Namespace:       "data-pipeline",
				MonthlyCost:     980,
				CPURequested:    2500,
				MemoryRequested: 12 * 1024 * 1024 * 1024,
				RiskLevel:       risk.RiskAmber,
				Workloads: []WorkloadSummary{
					{Name: "etl", MonthlyCost: 980, RiskLevel: risk.RiskAmber},
				},
			},
			{
				Namespace:       "default",
				MonthlyCost:     640,
				CPURequested:    1000,
				MemoryRequested: 2 * 1024 * 1024 * 1024,
				RiskLevel:       risk.RiskGreen,
				Workloads: []WorkloadSummary{
					{Name: "web", MonthlyCost: 640, RiskLevel: risk.RiskGreen},
				},
			},
		},
		NoColor: true,
		Layout:  LayoutPanel,
	}
}

func TestRenderPanelsContainsSummary(t *testing.T) {
	var buf bytes.Buffer
	err := RenderPanels(&buf, newPanelTestReport())
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Snapshot of current cluster cost breakdown")
	assert.Contains(t, output, "$14230")
	assert.Contains(t, output, "Namespaces:")
}

func TestRenderPanelsContainsAllNamespaces(t *testing.T) {
	var buf bytes.Buffer
	err := RenderPanels(&buf, newPanelTestReport())
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "api")
	assert.Contains(t, output, "data-pipeline")
	assert.Contains(t, output, "default")
}

func TestRenderPanelsSortsByCostDescending(t *testing.T) {
	var buf bytes.Buffer
	err := RenderPanels(&buf, newPanelTestReport())
	require.NoError(t, err)

	output := buf.String()
	// api ($1200) > data-pipeline ($980) > default ($640)
	apiIdx := indexOf(output, "$1200")
	pipelineIdx := indexOf(output, "$980")
	defaultIdx := indexOf(output, "$640")

	assert.Less(t, apiIdx, pipelineIdx)
	assert.Less(t, pipelineIdx, defaultIdx)
}

func TestRenderPanelsContainsCostAndResources(t *testing.T) {
	var buf bytes.Buffer
	err := RenderPanels(&buf, newPanelTestReport())
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Monthly cost:")
	assert.Contains(t, output, "CPU requested:")
	assert.Contains(t, output, "Mem requested:")
	assert.Contains(t, output, "4 cores")
	assert.Contains(t, output, "8 Gi")
}

func TestRenderPanelsContainsWorkloads(t *testing.T) {
	var buf bytes.Buffer
	err := RenderPanels(&buf, newPanelTestReport())
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "api-gateway")
	assert.Contains(t, output, "api-auth")
	assert.Contains(t, output, "etl")
	assert.Contains(t, output, "web")
}

func TestRenderPanelsContainsRiskIndicators(t *testing.T) {
	var buf bytes.Buffer
	err := RenderPanels(&buf, newPanelTestReport())
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "low")
	assert.Contains(t, output, "moderate")
}

func TestRenderPanelsNoColorUsesDashSeparators(t *testing.T) {
	var buf bytes.Buffer
	report := newPanelTestReport()
	report.NoColor = true

	err := RenderPanels(&buf, report)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "----")
}

func TestRenderPanelsEmptyNamespaces(t *testing.T) {
	var buf bytes.Buffer
	report := newPanelTestReport()
	report.NamespaceBreakdown = nil

	err := RenderPanels(&buf, report)
	require.NoError(t, err)

	output := buf.String()
	// Should still have summary panel
	assert.Contains(t, output, "Snapshot of current cluster cost breakdown")
	assert.Contains(t, output, "Namespaces:")
}

func TestRenderTableDispatchesToPanels(t *testing.T) {
	var buf bytes.Buffer
	report := newPanelTestReport()

	err := RenderTable(&buf, report)
	require.NoError(t, err)

	output := buf.String()
	// Should use panel layout (contains per-namespace cost, not "Top savings by namespace")
	assert.Contains(t, output, "Monthly cost:")
	assert.NotContains(t, output, "Top savings by namespace")
}

func TestFormatCPU(t *testing.T) {
	assert.Equal(t, "0m", formatCPU(0))
	assert.Equal(t, "500m", formatCPU(500))
	assert.Equal(t, "1 cores", formatCPU(1000))
	assert.Equal(t, "4 cores", formatCPU(4000))
	assert.Equal(t, "2.5 cores", formatCPU(2500))
}

func TestFormatMemory(t *testing.T) {
	assert.Equal(t, "0", formatMemory(0))
	assert.Equal(t, "512 Mi", formatMemory(512*1024*1024))
	assert.Equal(t, "1 Gi", formatMemory(1024*1024*1024))
	assert.Equal(t, "8 Gi", formatMemory(8*1024*1024*1024))
	assert.Equal(t, "1.5 Gi", formatMemory(1536*1024*1024))
	assert.Equal(t, "100 Ki", formatMemory(100*1024))
}
