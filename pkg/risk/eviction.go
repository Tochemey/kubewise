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
	"math"
	"strings"
)

// spotInterruptionRates maps instance type family prefixes to monthly interruption rates.
// These are approximate historical rates for AWS spot instances.
var spotInterruptionRates = map[string]float64{
	"m":   0.05, // general purpose: ~5%
	"c":   0.08, // compute optimized: ~8%
	"r":   0.06, // memory optimized: ~6%
	"t":   0.15, // burstable: ~15%
	"i":   0.07, // storage optimized: ~7%
	"d":   0.07, // dense storage: ~7%
	"p":   0.10, // GPU: ~10%
	"g":   0.10, // GPU: ~10%
	"x":   0.04, // memory intensive: ~4%
	"z":   0.03, // high frequency: ~3%
	"a":   0.06, // ARM: ~6%
	"n2":  0.05, // GCP N2: ~5%
	"n1":  0.06, // GCP N1: ~6%
	"e2":  0.04, // GCP E2: ~4%
	"n2d": 0.05, // GCP N2D: ~5%
}

// defaultInterruptionRate is used when the instance type family is not recognized.
const defaultInterruptionRate = 0.05

// SpotEvictionRisk calculates the risk that ALL replicas of a workload
// are interrupted simultaneously on spot instances.
// Returns: interruptionRate ^ replicaCount
func SpotEvictionRisk(instanceType string, replicaCount int) float64 {
	if replicaCount <= 0 {
		return 0
	}

	rate := lookupInterruptionRate(instanceType)
	// Risk of ALL replicas being interrupted simultaneously
	return math.Pow(rate, float64(replicaCount))
}

// lookupInterruptionRate returns the monthly interruption rate for an instance type.
func lookupInterruptionRate(instanceType string) float64 {
	// Try longest prefix first (e.g., "n2d" before "n")
	it := strings.ToLower(instanceType)

	// Try multi-char prefixes first
	for _, prefix := range []string{"n2d", "n2", "n1", "e2"} {
		if strings.HasPrefix(it, prefix) {
			return spotInterruptionRates[prefix]
		}
	}

	// Try single-char prefix (instance family letter)
	if len(it) > 0 {
		family := string(it[0])
		if rate, ok := spotInterruptionRates[family]; ok {
			return rate
		}
	}

	return defaultInterruptionRate
}
