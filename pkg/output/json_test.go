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

package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tochemey/kubewise/pkg/risk"
)

func TestRenderJSONValidJSON(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := RenderJSON(&buf, report)
	require.NoError(t, err)

	// Must be valid JSON
	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
}

func TestRenderJSONContainsFields(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := RenderJSON(&buf, report)
	require.NoError(t, err)

	var result jsonReport
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	// Scenario
	assert.Equal(t, "Right-size simulation", result.Scenario.Name)
	assert.Equal(t, "Right-size simulation (p95 + 20% buffer)", result.Scenario.Description)

	// Cost
	assert.InDelta(t, 14230.0, result.Cost.BaselineMonthly, 0.01)
	assert.InDelta(t, 9840.0, result.Cost.ProjectedMonthly, 0.01)
	assert.InDelta(t, 4390.0, result.Cost.SavingsMonthly, 0.01)
	assert.InDelta(t, 30.8, result.Cost.SavingsPercent, 0.01)

	// Risk
	assert.Equal(t, "green", result.Risk.OverallLevel)
	assert.InDelta(t, 0.008, result.Risk.ClusterOOM, 0.0001)
}

func TestRenderJSONWorkloadsSorted(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := RenderJSON(&buf, report)
	require.NoError(t, err)

	var result jsonReport
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	// Workloads should be sorted alphabetically by key (namespace/name)
	require.GreaterOrEqual(t, len(result.Risk.Workloads), 2)
	for i := 1; i < len(result.Risk.Workloads); i++ {
		prev := result.Risk.Workloads[i-1].Namespace + "/" + result.Risk.Workloads[i-1].Name
		curr := result.Risk.Workloads[i].Namespace + "/" + result.Risk.Workloads[i].Name
		assert.LessOrEqual(t, prev, curr, "workloads should be sorted")
	}
}

func TestRenderJSONRiskLevelsAsStrings(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()
	report.Risk.OverallLevel = risk.RiskAmber
	report.NamespaceBreakdown[1].RiskLevel = risk.RiskRed

	err := RenderJSON(&buf, report)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, `"amber"`)
	assert.Contains(t, output, `"green"`)
	assert.Contains(t, output, `"red"`)
}

func TestRenderJSONNamespaceBreakdown(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := RenderJSON(&buf, report)
	require.NoError(t, err)

	var result jsonReport
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, 3, len(result.NamespaceBreakdown))
	assert.Equal(t, "api", result.NamespaceBreakdown[0].Namespace)
	assert.InDelta(t, 1200.0, result.NamespaceBreakdown[0].Savings, 0.01)
}

func TestRenderJSONIndented(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := RenderJSON(&buf, report)
	require.NoError(t, err)

	// Should be indented with 2 spaces
	assert.Contains(t, buf.String(), "  \"scenario\"")
}

func TestRoundTo2(t *testing.T) {
	assert.InDelta(t, 14230.00, roundTo2(14230.001), 0.001)
	assert.InDelta(t, 0.19, roundTo2(0.192), 0.001)
	assert.InDelta(t, 100.12, roundTo2(100.124), 0.001)
	assert.InDelta(t, 100.13, roundTo2(100.125), 0.001)
}

func TestRiskLevelString(t *testing.T) {
	assert.Equal(t, "green", riskLevelString(risk.RiskGreen))
	assert.Equal(t, "amber", riskLevelString(risk.RiskAmber))
	assert.Equal(t, "red", riskLevelString(risk.RiskRed))
	assert.Equal(t, "unknown", riskLevelString(risk.RiskUnknown))
}
