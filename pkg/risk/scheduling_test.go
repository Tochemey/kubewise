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

func TestSchedulingRiskZero(t *testing.T) {
	assert.Equal(t, 0.0, SchedulingRisk(0, 100))
}

func TestSchedulingRiskAll(t *testing.T) {
	assert.Equal(t, 1.0, SchedulingRisk(100, 100))
}

func TestSchedulingRiskPartial(t *testing.T) {
	assert.InDelta(t, 0.05, SchedulingRisk(5, 100), 1e-9)
}

func TestSchedulingRiskZeroPods(t *testing.T) {
	assert.Equal(t, 0.0, SchedulingRisk(0, 0))
}

func TestSchedulingRiskClassification(t *testing.T) {
	// 0 unschedulable → green
	assert.Equal(t, RiskGreen, classifyScheduling(SchedulingRisk(0, 100)))

	// 0.5% unschedulable → amber
	risk := SchedulingRisk(1, 200) // 0.5%
	assert.Equal(t, RiskAmber, classifyScheduling(risk))

	// 2% unschedulable → red
	risk = SchedulingRisk(2, 100) // 2%
	assert.Equal(t, RiskRed, classifyScheduling(risk))
}

func TestSchedulingRiskBoundaries(t *testing.T) {
	// Exactly 0 → green
	assert.Equal(t, RiskGreen, classifyScheduling(0.0))

	// Just above 0 → amber
	assert.Equal(t, RiskAmber, classifyScheduling(0.001))

	// Just below 1% → amber
	assert.Equal(t, RiskAmber, classifyScheduling(0.009))

	// At 1% → red
	assert.Equal(t, RiskRed, classifyScheduling(0.01))
}
