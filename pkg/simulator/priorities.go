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

package simulator

import (
	"math"

	"github.com/tochemey/kubewise/pkg/collector"
)

// ScoreNode scores a node for a given pod using MostRequestedPriority.
// Higher scores indicate more tightly packed nodes (better for bin-packing).
// Returns a score from 0 to 100.
func ScoreNode(pod collector.PodSnapshot, node *NodeState) int {
	if node.Allocatable.CPU == 0 || node.Allocatable.Memory == 0 {
		return 0
	}

	cpuUsed := node.UsedCPU + totalPodCPURequest(pod)
	memUsed := node.UsedMemory + totalPodMemoryRequest(pod)

	cpuRatio := float64(cpuUsed) / float64(node.Allocatable.CPU)
	memRatio := float64(memUsed) / float64(node.Allocatable.Memory)

	// Score 0-100, higher = more packed = better
	score := int((cpuRatio + memRatio) / 2.0 * 100)

	// Penalty for resource imbalance (prefer balanced CPU:mem ratio)
	imbalance := math.Abs(cpuRatio - memRatio)
	score -= int(imbalance * 20)

	return max(score, 0)
}
