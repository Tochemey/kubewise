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
	"github.com/tochemey/kubewise/pkg/collector"
)

func baseProfile() collector.ContainerUsageProfile {
	return collector.ContainerUsageProfile{
		P50Memory:  100_000_000, // 100 MB
		P90Memory:  150_000_000, // 150 MB
		P95Memory:  180_000_000, // 180 MB
		P99Memory:  220_000_000, // 220 MB
		DataPoints: 2000,
	}
}

func TestOOMRiskInsufficientData(t *testing.T) {
	profile := baseProfile()
	profile.DataPoints = 50 // below minimum

	risk := OOMRisk(profile, 200_000_000)
	assert.Equal(t, -1.0, risk)
}

func TestOOMRiskUnboundedLimit(t *testing.T) {
	profile := baseProfile()
	// Zero limit = unbounded, no OOM risk
	assert.Equal(t, 0.0, OOMRisk(profile, 0))
}

func TestOOMRiskWellAboveP99(t *testing.T) {
	profile := baseProfile()
	// Limit well above P99 (220MB) → very low risk
	risk := OOMRisk(profile, 500_000_000) // 500 MB
	assert.Less(t, risk, 0.01)
	assert.GreaterOrEqual(t, risk, 0.0)
}

func TestOOMRiskJustAboveP99(t *testing.T) {
	profile := baseProfile()
	// Limit just above P99 → risk around 1%
	risk := OOMRisk(profile, 221_000_000)
	assert.Less(t, risk, OOMRiskLowThreshold) // < 1%
	assert.GreaterOrEqual(t, risk, 0.0)
}

func TestOOMRiskBetweenP95AndP99(t *testing.T) {
	profile := baseProfile()
	// Midpoint between P95 (180MB) and P99 (220MB) = 200MB
	risk := OOMRisk(profile, 200_000_000)
	assert.Greater(t, risk, OOMRiskLowThreshold)   // > 1%
	assert.Less(t, risk, OOMRiskModerateThreshold) // < 5%
}

func TestOOMRiskBetweenP90AndP95(t *testing.T) {
	profile := baseProfile()
	// Midpoint between P90 (150MB) and P95 (180MB) = 165MB
	risk := OOMRisk(profile, 165_000_000)
	assert.Greater(t, risk, OOMRiskModerateThreshold) // > 5%
	assert.Less(t, risk, 0.10)                        // < 10%
}

func TestOOMRiskBetweenP50AndP90(t *testing.T) {
	profile := baseProfile()
	// Midpoint between P50 (100MB) and P90 (150MB) = 125MB
	risk := OOMRisk(profile, 125_000_000)
	assert.Greater(t, risk, 0.10)
	assert.Less(t, risk, 0.50)
}

func TestOOMRiskAtOrBelowP50(t *testing.T) {
	profile := baseProfile()
	// At P50 → 50% risk
	risk := OOMRisk(profile, 100_000_000)
	assert.InDelta(t, 0.50, risk, 0.01)

	// Below P50 → 50% risk
	risk = OOMRisk(profile, 50_000_000)
	assert.Equal(t, 0.50, risk)
}

func TestOOMRiskMonotonicallyDecreasing(t *testing.T) {
	profile := baseProfile()
	// Risk should decrease as limit increases
	limits := []int64{
		80_000_000,  // below P50
		100_000_000, // P50
		125_000_000, // between P50 and P90
		150_000_000, // P90
		165_000_000, // between P90 and P95
		180_000_000, // P95
		200_000_000, // between P95 and P99
		220_000_000, // P99
		300_000_000, // above P99
	}

	prevRisk := 1.0
	for _, limit := range limits {
		risk := OOMRisk(profile, limit)
		assert.LessOrEqual(t, risk, prevRisk, "risk should decrease as limit increases: limit=%d", limit)
		prevRisk = risk
	}
}

func TestOOMRiskBoundaryAt1Percent(t *testing.T) {
	profile := baseProfile()
	// Find a limit that gives risk just below 1% and just above 1%
	// P99=220MB. Just above P99 should be < 1%
	riskBelow := OOMRisk(profile, 225_000_000)
	assert.Less(t, riskBelow, OOMRiskLowThreshold, "limit above P99 should have <1%% risk")

	// Between P95 and P99 should be > 1%
	riskAbove := OOMRisk(profile, 210_000_000)
	assert.Greater(t, riskAbove, OOMRiskLowThreshold, "limit between P95 and P99 should have >1%% risk")
}

func TestOOMRiskBoundaryAt5Percent(t *testing.T) {
	profile := baseProfile()
	// Between P95 (180MB) and P99 (220MB) midpoint should be 1-5%
	riskBelow := OOMRisk(profile, 195_000_000) // closer to P95 → closer to 5%
	assert.Less(t, riskBelow, OOMRiskModerateThreshold)

	// Between P90 (150MB) and P95 (180MB) should be > 5%
	riskAbove := OOMRisk(profile, 170_000_000)
	assert.Greater(t, riskAbove, OOMRiskModerateThreshold)
}

func TestLerp(t *testing.T) {
	// Midpoint
	assert.InDelta(t, 0.5, lerp(0, 0, 10, 1, 5), 1e-9)
	// At x0
	assert.InDelta(t, 0.0, lerp(0, 0, 10, 1, 0), 1e-9)
	// At x1
	assert.InDelta(t, 1.0, lerp(0, 0, 10, 1, 10), 1e-9)
	// Equal x0 and x1
	assert.InDelta(t, 0.5, lerp(5, 0, 5, 1, 5), 1e-9)
	// Clamped below
	assert.InDelta(t, 0.0, lerp(0, 0, 10, 1, -5), 1e-9)
	// Clamped above
	assert.InDelta(t, 1.0, lerp(0, 0, 10, 1, 15), 1e-9)
}
