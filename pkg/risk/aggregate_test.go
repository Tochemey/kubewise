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

package risk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tochemey/kubewise/pkg/collector"
)

func TestRiskLevelString(t *testing.T) {
	assert.Equal(t, "low", RiskGreen.String())
	assert.Equal(t, "moderate", RiskAmber.String())
	assert.Equal(t, "high", RiskRed.String())
	assert.Equal(t, "unknown", RiskUnknown.String())
}

func TestClassifyRiskOOMOnly(t *testing.T) {
	assert.Equal(t, RiskGreen, ClassifyRisk(0.005, 0, 0)) // 0.5%
	assert.Equal(t, RiskAmber, ClassifyRisk(0.03, 0, 0))  // 3%
	assert.Equal(t, RiskRed, ClassifyRisk(0.06, 0, 0))    // 6%
}

func TestClassifyRiskEvictionOnly(t *testing.T) {
	assert.Equal(t, RiskGreen, ClassifyRisk(0, 0.0005, 0)) // 0.05%
	assert.Equal(t, RiskAmber, ClassifyRisk(0, 0.005, 0))  // 0.5%
	assert.Equal(t, RiskRed, ClassifyRisk(0, 0.02, 0))     // 2%
}

func TestClassifyRiskSchedulingOnly(t *testing.T) {
	assert.Equal(t, RiskGreen, ClassifyRisk(0, 0, 0))
	assert.Equal(t, RiskAmber, ClassifyRisk(0, 0, 0.005)) // 0.5%
	assert.Equal(t, RiskRed, ClassifyRisk(0, 0, 0.02))    // 2%
}

func TestClassifyRiskWorstWins(t *testing.T) {
	// OOM green, eviction green, scheduling red → red
	assert.Equal(t, RiskRed, ClassifyRisk(0.005, 0.0005, 0.02))
	// All amber
	assert.Equal(t, RiskAmber, ClassifyRisk(0.02, 0.005, 0.005))
	// Mixed: OOM red overrides everything
	assert.Equal(t, RiskRed, ClassifyRisk(0.10, 0, 0))
}

func TestClassifyRiskBoundaries(t *testing.T) {
	// Exact boundaries for OOM
	assert.Equal(t, RiskGreen, ClassifyRisk(0.009, 0, 0)) // 0.9% < 1%
	assert.Equal(t, RiskAmber, ClassifyRisk(0.011, 0, 0)) // 1.1% >= 1%
	assert.Equal(t, RiskAmber, ClassifyRisk(0.049, 0, 0)) // 4.9% < 5%
	assert.Equal(t, RiskRed, ClassifyRisk(0.051, 0, 0))   // 5.1% >= 5%
}

func TestScoreOOMRiskSingleWorkload(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Pods: []collector.PodSnapshot{
			{
				Name: "web-1", Namespace: "default",
				OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "web"},
				Containers: []collector.ContainerSnapshot{
					{
						Name:   "nginx",
						Limits: collector.ResourcePair{Memory: 250_000_000}, // above P99
					},
				},
			},
		},
		UsageProfile: map[string]collector.ContainerUsageProfile{
			"default/web-1/nginx": {
				P50Memory: 100_000_000, P90Memory: 150_000_000,
				P95Memory: 180_000_000, P99Memory: 220_000_000,
				DataPoints: 2000,
			},
		},
	}

	report := ScoreOOMRisk(snap)
	require.NotNil(t, report)

	wr, ok := report.PerWorkload["default/web"]
	require.True(t, ok)
	assert.Less(t, wr.OOMRisk, OOMRiskLowThreshold) // limit above P99 → green
	assert.Equal(t, RiskGreen, wr.Level)
	assert.Equal(t, RiskGreen, report.OverallLevel)
}

func TestScoreOOMRiskMultiContainerWorkload(t *testing.T) {
	// Workload with 2 containers, each with some OOM risk
	snap := &collector.ClusterSnapshot{
		Pods: []collector.PodSnapshot{
			{
				Name: "app-1", Namespace: "default",
				OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "app"},
				Containers: []collector.ContainerSnapshot{
					{Name: "main", Limits: collector.ResourcePair{Memory: 200_000_000}},    // between P95-P99
					{Name: "sidecar", Limits: collector.ResourcePair{Memory: 200_000_000}}, // between P95-P99
				},
			},
		},
		UsageProfile: map[string]collector.ContainerUsageProfile{
			"default/app-1/main": {
				P50Memory: 100_000_000, P90Memory: 150_000_000,
				P95Memory: 180_000_000, P99Memory: 220_000_000,
				DataPoints: 2000,
			},
			"default/app-1/sidecar": {
				P50Memory: 100_000_000, P90Memory: 150_000_000,
				P95Memory: 180_000_000, P99Memory: 220_000_000,
				DataPoints: 2000,
			},
		},
	}

	report := ScoreOOMRisk(snap)
	wr := report.PerWorkload["default/app"]

	// Each container has ~3% risk (between P95-P99, limit=200M)
	// Workload risk = 1 - (1-r1)(1-r2) > each individual risk
	mainRisk := OOMRisk(snap.UsageProfile["default/app-1/main"], 200_000_000)
	expectedWorkloadRisk := 1 - (1-mainRisk)*(1-mainRisk)

	assert.InDelta(t, expectedWorkloadRisk, wr.OOMRisk, 0.001)
	assert.Greater(t, wr.OOMRisk, mainRisk) // workload risk > individual container risk
}

func TestScoreOOMRiskInsufficientData(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Pods: []collector.PodSnapshot{
			{
				Name: "new-1", Namespace: "default",
				OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "new"},
				Containers: []collector.ContainerSnapshot{
					{Name: "app", Limits: collector.ResourcePair{Memory: 200_000_000}},
				},
			},
		},
		UsageProfile: map[string]collector.ContainerUsageProfile{
			"default/new-1/app": {
				P50Memory: 100_000_000, P90Memory: 150_000_000,
				P95Memory: 180_000_000, P99Memory: 220_000_000,
				DataPoints: 50, // below minimum
			},
		},
	}

	report := ScoreOOMRisk(snap)
	wr := report.PerWorkload["default/new"]
	assert.Equal(t, -1.0, wr.OOMRisk)
	assert.Equal(t, RiskUnknown, wr.Level)
}

func TestScoreOOMRiskNoUsageProfile(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Pods: []collector.PodSnapshot{
			{
				Name: "orphan-1", Namespace: "default",
				OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "orphan"},
				Containers: []collector.ContainerSnapshot{
					{Name: "app", Limits: collector.ResourcePair{Memory: 200_000_000}},
				},
			},
		},
		UsageProfile: map[string]collector.ContainerUsageProfile{},
	}

	report := ScoreOOMRisk(snap)
	wr := report.PerWorkload["default/orphan"]
	assert.Equal(t, -1.0, wr.OOMRisk)
	assert.Equal(t, RiskUnknown, wr.Level)
}

func TestScoreOOMRiskClusterWideWorstCase(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Pods: []collector.PodSnapshot{
			{
				Name: "safe-1", Namespace: "default",
				OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "safe"},
				Containers: []collector.ContainerSnapshot{
					{Name: "app", Limits: collector.ResourcePair{Memory: 300_000_000}}, // well above P99
				},
			},
			{
				Name: "risky-1", Namespace: "default",
				OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "risky"},
				Containers: []collector.ContainerSnapshot{
					{Name: "app", Limits: collector.ResourcePair{Memory: 160_000_000}}, // between P90-P95
				},
			},
		},
		UsageProfile: map[string]collector.ContainerUsageProfile{
			"default/safe-1/app": {
				P50Memory: 100_000_000, P90Memory: 150_000_000,
				P95Memory: 180_000_000, P99Memory: 220_000_000,
				DataPoints: 2000,
			},
			"default/risky-1/app": {
				P50Memory: 100_000_000, P90Memory: 150_000_000,
				P95Memory: 180_000_000, P99Memory: 220_000_000,
				DataPoints: 2000,
			},
		},
	}

	report := ScoreOOMRisk(snap)

	safeRisk := report.PerWorkload["default/safe"]
	riskyRisk := report.PerWorkload["default/risky"]

	assert.Less(t, safeRisk.OOMRisk, riskyRisk.OOMRisk)

	// Cluster OOM should equal the worst workload
	assert.InDelta(t, riskyRisk.OOMRisk, report.ClusterOOM, 0.001)
	assert.Equal(t, RiskRed, report.OverallLevel) // risky workload is > 5%
}

func TestScoreOOMRiskPodWithoutOwnerRef(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Pods: []collector.PodSnapshot{
			{
				Name: "standalone", Namespace: "default",
				// No owner ref
				Containers: []collector.ContainerSnapshot{
					{Name: "app", Limits: collector.ResourcePair{Memory: 250_000_000}},
				},
			},
		},
		UsageProfile: map[string]collector.ContainerUsageProfile{
			"default/standalone/app": {
				P50Memory: 100_000_000, P90Memory: 150_000_000,
				P95Memory: 180_000_000, P99Memory: 220_000_000,
				DataPoints: 2000,
			},
		},
	}

	report := ScoreOOMRisk(snap)
	// Should use pod name as workload key
	_, ok := report.PerWorkload["default/standalone"]
	assert.True(t, ok)
}
