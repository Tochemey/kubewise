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
	"github.com/tochemey/kubewise/pkg/collector"
)

const (
	// minDataPoints is the minimum number of samples required for a meaningful risk estimate.
	minDataPoints = 100
	// insufficientData is the sentinel value returned when data is insufficient.
	insufficientData = -1.0
)

// OOMRisk estimates the probability of an OOM kill given a container usage profile
// and a proposed new memory limit. Returns a value between 0.0 and 1.0, or -1.0
// if there is insufficient data.
//
// The estimate is based on linear interpolation between known percentiles:
//
//	P99 < newLimit         → risk ≈ 0.5%
//	P95 < newLimit <= P99  → risk ≈ 1-5%  (interpolated)
//	P90 < newLimit <= P95  → risk ≈ 5-10% (interpolated)
//	P50 < newLimit <= P90  → risk ≈ 10-50% (interpolated)
//	newLimit <= P50        → risk ≈ 50%+
func OOMRisk(profile collector.ContainerUsageProfile, newLimitMemory int64) float64 {
	if profile.DataPoints < minDataPoints {
		return insufficientData
	}

	if newLimitMemory <= 0 {
		// No limit (unbounded) — no OOM risk from limits
		return 0.0
	}

	p50 := profile.P50Memory
	p90 := profile.P90Memory
	p95 := profile.P95Memory
	p99 := profile.P99Memory

	switch {
	case newLimitMemory > p99:
		// Well above P99 — very low risk
		// Interpolate from 0.005 (at 2*P99) to 0.01 (at P99)
		if p99 > 0 {
			return lerp(float64(p99), 0.01, float64(p99*2), 0.001, float64(newLimitMemory))
		}
		return 0.005

	case newLimitMemory > p95:
		// Between P95 and P99
		return lerp(float64(p95), 0.05, float64(p99), 0.01, float64(newLimitMemory))

	case newLimitMemory > p90:
		// Between P90 and P95
		return lerp(float64(p90), 0.10, float64(p95), 0.05, float64(newLimitMemory))

	case newLimitMemory > p50:
		// Between P50 and P90
		return lerp(float64(p50), 0.50, float64(p90), 0.10, float64(newLimitMemory))

	default:
		// At or below P50 — extremely risky
		return 0.50
	}
}

// lerp performs linear interpolation between two points.
// Given (x0, y0) and (x1, y1), returns y at position x.
// y0 is the value at x0, y1 is the value at x1.
// As x increases from x0 to x1, y decreases from y0 to y1.
func lerp(x0, y0, x1, y1, x float64) float64 {
	if x1 == x0 {
		return (y0 + y1) / 2
	}
	t := (x - x0) / (x1 - x0)
	// Clamp t to [0, 1]
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return y0 + t*(y1-y0)
}
