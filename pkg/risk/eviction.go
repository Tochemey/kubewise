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
	"math"
	"strings"
)

// Instance family prefix constants identify cloud provider instance type families
// used to look up approximate spot/preemptible interruption rates.
const (
	// familyGeneralPurpose is the AWS M-family prefix for general-purpose instances
	// offering a balance of compute, memory, and networking (e.g. m5.xlarge).
	familyGeneralPurpose = "m"
	// familyComputeOptimized is the AWS C-family prefix for compute-optimized instances
	// suited for CPU-intensive workloads (e.g. c5.2xlarge).
	familyComputeOptimized = "c"
	// familyMemoryOptimized is the AWS R-family prefix for memory-optimized instances
	// designed for large in-memory datasets (e.g. r6i.large).
	familyMemoryOptimized = "r"
	// familyBurstable is the AWS T-family prefix for burstable instances that provide
	// a baseline CPU with the ability to burst above it (e.g. t3.medium).
	familyBurstable = "t"
	// familyStorageOptimized is the AWS I-family prefix for storage-optimized instances
	// providing high sequential read/write access to large datasets (e.g. i3.xlarge).
	familyStorageOptimized = "i"
	// familyDenseStorage is the AWS D-family prefix for dense-storage instances
	// designed for massively parallel processing and data warehousing (e.g. d2.xlarge).
	familyDenseStorage = "d"
	// familyGPUP is the AWS P-family prefix for GPU-accelerated instances optimized
	// for machine learning training and HPC (e.g. p3.2xlarge).
	familyGPUP = "p"
	// familyGPUG is the AWS G-family prefix for GPU-accelerated instances optimized
	// for graphics-intensive applications and inference (e.g. g4dn.xlarge).
	familyGPUG = "g"
	// familyMemoryIntensive is the AWS X-family prefix for memory-intensive instances
	// providing the highest memory-to-CPU ratio (e.g. x1.16xlarge).
	familyMemoryIntensive = "x"
	// familyHighFrequency is the AWS Z-family prefix for high-frequency instances
	// offering sustained all-core turbo clock speeds (e.g. z1d.large).
	familyHighFrequency = "z"
	// familyARM is the AWS A-family prefix for ARM-based (Graviton) instances
	// providing cost-effective performance for scale-out workloads (e.g. a1.medium).
	familyARM = "a"

	// familyGCPN2 is the GCP N2-family prefix for general-purpose VMs running on
	// Intel Cascade Lake processors.
	familyGCPN2 = "n2"
	// familyGCPN1 is the GCP N1-family prefix for general-purpose VMs, the
	// first generation of GCP machine types.
	familyGCPN1 = "n1"
	// familyGCPE2 is the GCP E2-family prefix for cost-optimized VMs that offer
	// shared-core and whole-core configurations.
	familyGCPE2 = "e2"
	// familyGCPN2D is the GCP N2D-family prefix for general-purpose VMs running on
	// AMD EPYC processors.
	familyGCPN2D = "n2d"

	// defaultInterruptionRate is the fallback monthly interruption probability used
	// when the instance type family is not recognized in spotInterruptionRates.
	defaultInterruptionRate = 0.05
)

// gcpPrefixes lists multi-char GCP prefixes in longest-first order for matching.
var gcpPrefixes = []string{familyGCPN2D, familyGCPN2, familyGCPN1, familyGCPE2}

// spotInterruptionRates maps instance type family prefixes to monthly interruption rates.
// These are approximate historical rates for AWS spot instances.
var spotInterruptionRates = map[string]float64{
	familyGeneralPurpose:   0.05, // general purpose: ~5%
	familyComputeOptimized: 0.08, // compute optimized: ~8%
	familyMemoryOptimized:  0.06, // memory optimized: ~6%
	familyBurstable:        0.15, // burstable: ~15%
	familyStorageOptimized: 0.07, // storage optimized: ~7%
	familyDenseStorage:     0.07, // dense storage: ~7%
	familyGPUP:             0.10, // GPU: ~10%
	familyGPUG:             0.10, // GPU: ~10%
	familyMemoryIntensive:  0.04, // memory intensive: ~4%
	familyHighFrequency:    0.03, // high frequency: ~3%
	familyARM:              0.06, // ARM: ~6%
	familyGCPN2:            0.05, // GCP N2: ~5%
	familyGCPN1:            0.06, // GCP N1: ~6%
	familyGCPE2:            0.04, // GCP E2: ~4%
	familyGCPN2D:           0.05, // GCP N2D: ~5%
}

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
	for _, prefix := range gcpPrefixes {
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
