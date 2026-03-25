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
	"sort"

	"github.com/tochemey/kubewise/pkg/collector"
)

// Simulate runs the bin-packing scheduling simulation on a snapshot.
// It pre-places DaemonSet pods, then packs remaining pods using
// first-fit-decreasing with MostRequestedPriority scoring.
func Simulate(snap *collector.ClusterSnapshot) (*SimulationResult, error) {
	state := NewSchedulingState(snap.Nodes)

	daemonSetPods, regularPods := separatePods(snap.Pods)

	// Phase 1: Pre-place DaemonSet pods on all matching nodes
	prePlaceDaemonSets(daemonSetPods, state)

	// Phase 2: Sort remaining pods by total resource request descending (first-fit-decreasing)
	sort.Sort(podsByResource(regularPods))

	// Phase 3: Bin-pack regular pods
	// Pre-allocate unschedulable slice (worst case = all pods)
	unschedulable := make([]collector.PodSnapshot, 0, len(regularPods)/4)

	// Pre-compute pod resource totals to avoid recomputing in the inner loop
	podCPU := make([]int64, len(regularPods))
	podMem := make([]int64, len(regularPods))
	for i := range regularPods {
		podCPU[i] = totalPodCPURequest(regularPods[i])
		podMem[i] = totalPodMemoryRequest(regularPods[i])
	}

	for i, pod := range regularPods {
		nodeName := findBestNode(pod, state, podCPU[i], podMem[i])
		if nodeName == "" {
			unschedulable = append(unschedulable, pod)
			continue
		}
		state.PlaceWithResources(pod, nodeName, podCPU[i], podMem[i])
	}

	// Build result
	utilization := state.Utilization()
	activeNodes := state.ActiveNodes()

	return &SimulationResult{
		Placements:        state.Placements,
		UnschedulablePods: unschedulable,
		NodeUtilization:   utilization,
		TotalNodes:        len(state.Nodes),
		ActiveNodes:       len(activeNodes),
	}, nil
}

// separatePods splits pods into DaemonSet pods and regular pods.
// Pre-allocates slices based on estimated proportions.
func separatePods(pods []collector.PodSnapshot) (daemonSet, regular []collector.PodSnapshot) {
	// Estimate: ~5% DaemonSet pods in a typical cluster
	daemonSet = make([]collector.PodSnapshot, 0, len(pods)/20+1)
	regular = make([]collector.PodSnapshot, 0, len(pods))
	for _, pod := range pods {
		if pod.OwnerRef.Kind == "DaemonSet" {
			daemonSet = append(daemonSet, pod)
		} else {
			regular = append(regular, pod)
		}
	}
	return
}

// prePlaceDaemonSets places DaemonSet pods on every matching node.
// A DaemonSet pod matches a node if it tolerates the node's taints
// and satisfies node affinity.
func prePlaceDaemonSets(pods []collector.PodSnapshot, state *SchedulingState) {
	// Group DaemonSet pods by their owner name to avoid duplicates per node
	seen := make(map[string]map[string]bool, len(pods))

	for _, pod := range pods {
		ownerKey := pod.OwnerRef.Name
		if seen[ownerKey] == nil {
			seen[ownerKey] = make(map[string]bool, len(state.Nodes))
		}

		for _, node := range state.Nodes {
			if seen[ownerKey][node.Name] {
				continue
			}
			if !ToleratesTaints(pod, node) {
				continue
			}
			if !MatchesAffinity(pod, node) {
				continue
			}
			state.Place(pod, node.Name)
			seen[ownerKey][node.Name] = true
		}
	}
}

// podsByResource implements sort.Interface for sorting pods by total resource
// request in descending order. Uses a named type to avoid closure allocation.
type podsByResource []collector.PodSnapshot

func (p podsByResource) Len() int      { return len(p) }
func (p podsByResource) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p podsByResource) Less(i, j int) bool {
	ri := totalPodCPURequest(p[i]) + totalPodMemoryRequest(p[i])
	rj := totalPodCPURequest(p[j]) + totalPodMemoryRequest(p[j])
	return ri > rj
}

// findBestNode finds the highest-scoring node that passes all predicates.
// Returns empty string if no node can fit the pod.
// podCPU and podMem are pre-computed to avoid recomputing per-node.
func findBestNode(pod collector.PodSnapshot, state *SchedulingState, podCPU, podMem int64) string {
	bestNode := ""
	bestScore := -1

	for _, node := range state.Nodes {
		if !fitsResourcesPrecomputed(podCPU, podMem, node) {
			continue
		}
		if !MatchesAffinity(pod, node) {
			continue
		}
		if !ToleratesTaints(pod, node) {
			continue
		}
		if !SatisfiesTopologySpread(pod, node, state) {
			continue
		}

		score := scoreNodePrecomputed(podCPU, podMem, node)
		if score > bestScore {
			bestScore = score
			bestNode = node.Name
		}
	}

	return bestNode
}

// fitsResourcesPrecomputed checks resource fit using pre-computed pod totals.
func fitsResourcesPrecomputed(podCPU, podMem int64, node *NodeState) bool {
	return podCPU <= node.Allocatable.CPU-node.UsedCPU &&
		podMem <= node.Allocatable.Memory-node.UsedMemory
}

// scoreNodePrecomputed scores a node using pre-computed pod resource totals.
func scoreNodePrecomputed(podCPU, podMem int64, node *NodeState) int {
	if node.Allocatable.CPU == 0 || node.Allocatable.Memory == 0 {
		return 0
	}

	cpuUsed := node.UsedCPU + podCPU
	memUsed := node.UsedMemory + podMem

	cpuRatio := float64(cpuUsed) / float64(node.Allocatable.CPU)
	memRatio := float64(memUsed) / float64(node.Allocatable.Memory)

	score := int((cpuRatio + memRatio) / 2.0 * 100)

	imbalance := cpuRatio - memRatio
	if imbalance < 0 {
		imbalance = -imbalance
	}
	score -= int(imbalance * 20)

	if score < 0 {
		return 0
	}
	return score
}
