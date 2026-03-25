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

package scenario

import (
	"fmt"

	"github.com/tochemey/kubewise/pkg/collector"
)

const (
	// minCPUMillicores is the minimum CPU request floor.
	minCPUMillicores int64 = 10
	// minMemoryBytes is the minimum memory request floor (32Mi).
	minMemoryBytes int64 = 33554432
)

// RightSizeScenario adjusts resource requests and limits based on actual usage percentiles.
type RightSizeScenario struct {
	Meta          ScenarioMetadata
	Percentile    string
	Buffer        int
	Scope         Scope
	LimitStrategy string
}

// RightSizeChange records what changed for a single container.
type RightSizeChange struct {
	Namespace           string
	Pod                 string
	Container           string
	OriginalCPU         int64
	OriginalMemory      int64
	NewCPU              int64
	NewMemory           int64
	OriginalLimitCPU    int64
	OriginalLimitMemory int64
	NewLimitCPU         int64
	NewLimitMemory      int64
}

// RightSizeResult holds the metadata about all changes made.
type RightSizeResult struct {
	Changes []RightSizeChange
	Skipped int
}

func (r *RightSizeScenario) Kind() string { return "RightSize" }

// Apply mutates the snapshot by adjusting container requests and limits
// based on actual usage percentiles. The snapshot is already a deep copy.
func (r *RightSizeScenario) Apply(snap *collector.ClusterSnapshot) (*collector.ClusterSnapshot, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}

	for i := range snap.Pods {
		pod := &snap.Pods[i]
		if !r.Scope.Includes(*pod) {
			continue
		}

		for j := range pod.Containers {
			container := &pod.Containers[j]
			key := collector.ProfileKey(pod.Namespace, pod.Name, container.Name)
			profile, ok := snap.UsageProfile[key]
			if !ok {
				continue
			}

			targetCPU := percentileValue(profile, r.Percentile, "cpu")
			targetMem := percentileValue(profile, r.Percentile, "memory")

			// Apply buffer
			newRequestCPU := applyBuffer(targetCPU, r.Buffer)
			newRequestMem := applyBuffer(targetMem, r.Buffer)

			// Enforce minimum floors
			newRequestCPU = max(newRequestCPU, minCPUMillicores)
			newRequestMem = max(newRequestMem, minMemoryBytes)

			// Compute new limits
			newLimitCPU, newLimitMem := computeLimits(
				container.Requests, container.Limits,
				newRequestCPU, newRequestMem,
				r.LimitStrategy,
			)

			container.Requests.CPU = newRequestCPU
			container.Requests.Memory = newRequestMem
			container.Limits.CPU = newLimitCPU
			container.Limits.Memory = newLimitMem
		}
	}

	return snap, nil
}

// ApplyWithChanges is like Apply but also returns metadata about what changed.
func (r *RightSizeScenario) ApplyWithChanges(snap *collector.ClusterSnapshot) (*collector.ClusterSnapshot, *RightSizeResult, error) {
	if err := r.validate(); err != nil {
		return nil, nil, err
	}

	result := &RightSizeResult{}

	for i := range snap.Pods {
		pod := &snap.Pods[i]
		if !r.Scope.Includes(*pod) {
			continue
		}

		for j := range pod.Containers {
			container := &pod.Containers[j]
			key := collector.ProfileKey(pod.Namespace, pod.Name, container.Name)
			profile, ok := snap.UsageProfile[key]
			if !ok {
				result.Skipped++
				continue
			}

			origCPU := container.Requests.CPU
			origMem := container.Requests.Memory
			origLimitCPU := container.Limits.CPU
			origLimitMem := container.Limits.Memory

			targetCPU := percentileValue(profile, r.Percentile, "cpu")
			targetMem := percentileValue(profile, r.Percentile, "memory")

			newRequestCPU := max(applyBuffer(targetCPU, r.Buffer), minCPUMillicores)
			newRequestMem := max(applyBuffer(targetMem, r.Buffer), minMemoryBytes)

			newLimitCPU, newLimitMem := computeLimits(
				container.Requests, container.Limits,
				newRequestCPU, newRequestMem,
				r.LimitStrategy,
			)

			container.Requests.CPU = newRequestCPU
			container.Requests.Memory = newRequestMem
			container.Limits.CPU = newLimitCPU
			container.Limits.Memory = newLimitMem

			result.Changes = append(result.Changes, RightSizeChange{
				Namespace:           pod.Namespace,
				Pod:                 pod.Name,
				Container:           container.Name,
				OriginalCPU:         origCPU,
				OriginalMemory:      origMem,
				NewCPU:              newRequestCPU,
				NewMemory:           newRequestMem,
				OriginalLimitCPU:    origLimitCPU,
				OriginalLimitMemory: origLimitMem,
				NewLimitCPU:         newLimitCPU,
				NewLimitMemory:      newLimitMem,
			})
		}
	}

	return snap, result, nil
}

func (r *RightSizeScenario) validate() error {
	switch r.Percentile {
	case "p50", "p90", "p95", "p99":
	default:
		return fmt.Errorf("invalid percentile %q: must be p50, p90, p95, or p99", r.Percentile)
	}
	switch r.LimitStrategy {
	case "ratio", "fixed", "unbounded", "":
	default:
		return fmt.Errorf("invalid limit strategy %q: must be ratio, fixed, or unbounded", r.LimitStrategy)
	}
	return nil
}

// percentileValue returns the CPU or memory percentile value from a usage profile.
func percentileValue(profile collector.ContainerUsageProfile, percentile, resource string) int64 {
	switch {
	case percentile == "p50" && resource == "cpu":
		return profile.P50CPU
	case percentile == "p90" && resource == "cpu":
		return profile.P90CPU
	case percentile == "p95" && resource == "cpu":
		return profile.P95CPU
	case percentile == "p99" && resource == "cpu":
		return profile.P99CPU
	case percentile == "p50" && resource == "memory":
		return profile.P50Memory
	case percentile == "p90" && resource == "memory":
		return profile.P90Memory
	case percentile == "p95" && resource == "memory":
		return profile.P95Memory
	case percentile == "p99" && resource == "memory":
		return profile.P99Memory
	default:
		return 0
	}
}

// applyBuffer adds a percentage buffer to a value.
func applyBuffer(value int64, bufferPercent int) int64 {
	return int64(float64(value) * (1.0 + float64(bufferPercent)/100.0))
}

// computeLimits calculates new limits based on the chosen strategy.
func computeLimits(origRequests, origLimits collector.ResourcePair, newCPU, newMem int64, strategy string) (int64, int64) {
	switch strategy {
	case "fixed":
		return newCPU, newMem
	case "unbounded":
		return 0, 0
	case "ratio", "":
		cpuLimit := computeRatioLimit(origRequests.CPU, origLimits.CPU, newCPU)
		memLimit := computeRatioLimit(origRequests.Memory, origLimits.Memory, newMem)
		return cpuLimit, memLimit
	default:
		return newCPU, newMem
	}
}

// computeRatioLimit maintains the original request:limit ratio.
func computeRatioLimit(origRequest, origLimit, newRequest int64) int64 {
	if origRequest <= 0 || origLimit <= 0 {
		// No original limit or request — set limit = request
		return newRequest
	}
	ratio := float64(origLimit) / float64(origRequest)
	return int64(float64(newRequest) * ratio)
}
