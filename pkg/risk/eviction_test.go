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

package risk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSpotEvictionRiskSingleReplica(t *testing.T) {
	// Single replica: risk = interruptionRate^1 = interruptionRate
	risk := SpotEvictionRisk("m6i.xlarge", 1)
	assert.InDelta(t, 0.05, risk, 1e-9) // m-series = 5%
}

func TestSpotEvictionRiskThreeReplicas(t *testing.T) {
	// 3 replicas: risk = 0.05^3 = 0.000125 = 0.0125%
	risk := SpotEvictionRisk("m6i.xlarge", 3)
	assert.InDelta(t, 0.000125, risk, 1e-9)
	assert.Less(t, risk, EvictionRiskLowThreshold) // < 0.1% = green
}

func TestSpotEvictionRiskTwoReplicas(t *testing.T) {
	// 2 replicas: risk = 0.05^2 = 0.0025 = 0.25%
	risk := SpotEvictionRisk("m6i.xlarge", 2)
	assert.InDelta(t, 0.0025, risk, 1e-9)
	assert.Greater(t, risk, EvictionRiskLowThreshold) // > 0.1% = amber
	assert.Less(t, risk, EvictionRiskModThreshold)    // < 1%
}

func TestSpotEvictionRiskBurstable(t *testing.T) {
	// t-series has 15% interruption rate
	risk := SpotEvictionRisk("t3.medium", 1)
	assert.InDelta(t, 0.15, risk, 1e-9)
	assert.Greater(t, risk, EvictionRiskModThreshold) // > 1% = red
}

func TestSpotEvictionRiskComputeOptimized(t *testing.T) {
	// c-series has 8% interruption rate
	risk := SpotEvictionRisk("c5.xlarge", 1)
	assert.InDelta(t, 0.08, risk, 1e-9)
}

func TestSpotEvictionRiskGPU(t *testing.T) {
	risk := SpotEvictionRisk("p3.2xlarge", 1)
	assert.InDelta(t, 0.10, risk, 1e-9)
}

func TestSpotEvictionRiskGCPInstances(t *testing.T) {
	assert.InDelta(t, 0.05, SpotEvictionRisk("n2-standard-4", 1), 1e-9)
	assert.InDelta(t, 0.06, SpotEvictionRisk("n1-standard-8", 1), 1e-9)
	assert.InDelta(t, 0.04, SpotEvictionRisk("e2-standard-2", 1), 1e-9)
	assert.InDelta(t, 0.05, SpotEvictionRisk("n2d-standard-4", 1), 1e-9)
}

func TestSpotEvictionRiskUnknownType(t *testing.T) {
	risk := SpotEvictionRisk("unknown.type", 1)
	assert.InDelta(t, defaultInterruptionRate, risk, 1e-9)
}

func TestSpotEvictionRiskZeroReplicas(t *testing.T) {
	assert.Equal(t, 0.0, SpotEvictionRisk("m6i.xlarge", 0))
}

func TestSpotEvictionRiskClassification(t *testing.T) {
	// 3 replicas m-series: 0.000125 < 0.001 → green
	assert.Equal(t, RiskGreen, classifyEviction(SpotEvictionRisk("m6i.xlarge", 3)))

	// 2 replicas m-series: 0.0025 → amber (0.1% < 0.25% < 1%)
	assert.Equal(t, RiskAmber, classifyEviction(SpotEvictionRisk("m6i.xlarge", 2)))

	// 1 replica t-series: 0.15 → red (> 1%)
	assert.Equal(t, RiskRed, classifyEviction(SpotEvictionRisk("t3.medium", 1)))
}
