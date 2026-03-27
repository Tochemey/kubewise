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

package simulator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tochemey/kubewise/pkg/collector"
)

func TestSimulateSimplePacking(t *testing.T) {
	// 2 nodes with 4000m CPU each, 3 pods with 1000m each
	// All 3 should fit on 1 node (bin-packing prefers tight packing)
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000}},
			{Name: "n2", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000}},
		},
		Pods: []collector.PodSnapshot{
			{Name: "p1", Namespace: "default", OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "web"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 1_000_000_000}}}},
			{Name: "p2", Namespace: "default", OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "web"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 1_000_000_000}}}},
			{Name: "p3", Namespace: "default", OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "web"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 1_000_000_000}}}},
		},
	}

	result, err := Simulate(snap)
	require.NoError(t, err)

	assert.Equal(t, 3, len(result.Placements))
	assert.Empty(t, result.UnschedulablePods)
	assert.Equal(t, 2, result.TotalNodes)

	// Verify bin-packing: pods should be concentrated on fewer nodes
	nodeCounts := countPlacementsPerNode(result)
	// With MostRequestedPriority, all 3 should land on the same node
	maxOnSingleNode := 0
	for _, count := range nodeCounts {
		if count > maxOnSingleNode {
			maxOnSingleNode = count
		}
	}
	assert.Equal(t, 3, maxOnSingleNode, "all 3 pods should be packed onto 1 node")
}

func TestSimulateDaemonSetPrePlacement(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000}, Labels: map[string]string{"zone": "a"}},
			{Name: "n2", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000}, Labels: map[string]string{"zone": "b"}},
		},
		Pods: []collector.PodSnapshot{
			// DaemonSet pod should be placed on both nodes
			{Name: "monitor-1", Namespace: "kube-system",
				OwnerRef:   collector.OwnerReference{Kind: "DaemonSet", Name: "monitor"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 100, Memory: 100_000_000}}}},
			// Regular pod
			{Name: "web-1", Namespace: "default",
				OwnerRef:   collector.OwnerReference{Kind: "Deployment", Name: "web"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 500, Memory: 500_000_000}}}},
		},
	}

	result, err := Simulate(snap)
	require.NoError(t, err)

	assert.Empty(t, result.UnschedulablePods)

	// DaemonSet should be placed on both nodes
	dsPlacedNodes := findPlacementNodes(result, "monitor-1")
	assert.Equal(t, 2, len(dsPlacedNodes), "DaemonSet pod should be on both nodes")

	// Regular pod should also be placed
	webNodes := findPlacementNodes(result, "web-1")
	assert.Equal(t, 1, len(webNodes))
}

func TestSimulateDaemonSetRespectsTolerationsAndAffinity(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000},
				Taints: []collector.Taint{{Key: "gpu", Value: "true", Effect: "NoSchedule"}}},
			{Name: "n2", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000}},
		},
		Pods: []collector.PodSnapshot{
			// DaemonSet without toleration — should only land on n2
			{Name: "agent-1", Namespace: "kube-system",
				OwnerRef:   collector.OwnerReference{Kind: "DaemonSet", Name: "agent"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 100, Memory: 100_000_000}}}},
		},
	}

	result, err := Simulate(snap)
	require.NoError(t, err)

	nodes := findPlacementNodes(result, "agent-1")
	assert.Equal(t, 1, len(nodes))
	assert.Contains(t, nodes, "n2")
}

func TestSimulatePodTooLarge(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", Allocatable: collector.ResourcePair{CPU: 1000, Memory: 1_000_000_000}},
		},
		Pods: []collector.PodSnapshot{
			{Name: "big-pod", Namespace: "default",
				OwnerRef:   collector.OwnerReference{Kind: "Deployment", Name: "big"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 2000, Memory: 2_000_000_000}}}},
		},
	}

	result, err := Simulate(snap)
	require.NoError(t, err)

	assert.Equal(t, 1, len(result.UnschedulablePods))
	assert.Equal(t, "big-pod", result.UnschedulablePods[0].Name)
	assert.Empty(t, result.Placements)
}

func TestSimulateTaintedNodeOnlyReceivesToleratingPods(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "tainted", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000},
				Taints: []collector.Taint{{Key: "dedicated", Value: "gpu", Effect: "NoSchedule"}}},
			{Name: "normal", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000}},
		},
		Pods: []collector.PodSnapshot{
			// No toleration — should go to normal
			{Name: "web-1", Namespace: "default",
				OwnerRef:   collector.OwnerReference{Kind: "Deployment", Name: "web"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 500, Memory: 500_000_000}}}},
			// Has toleration — can go to tainted
			{Name: "gpu-job", Namespace: "default",
				OwnerRef:    collector.OwnerReference{Kind: "Job", Name: "gpu"},
				Tolerations: []collector.Toleration{{Key: "dedicated", Operator: "Equal", Value: "gpu", Effect: "NoSchedule"}},
				Containers:  []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 1_000_000_000}}}},
		},
	}

	result, err := Simulate(snap)
	require.NoError(t, err)
	assert.Empty(t, result.UnschedulablePods)

	webNodes := findPlacementNodes(result, "web-1")
	assert.Equal(t, []string{"normal"}, webNodes)
}

func TestSimulateAffinityConstrainsPlacement(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "gpu-node", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000},
				Labels: map[string]string{"gpu": "true"}},
			{Name: "cpu-node", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000},
				Labels: map[string]string{"gpu": "false"}},
		},
		Pods: []collector.PodSnapshot{
			{Name: "ml-pod", Namespace: "default",
				OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "ml"},
				Affinity: &collector.Affinity{
					NodeAffinity: &collector.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &collector.NodeSelector{
							NodeSelectorTerms: []collector.NodeSelectorTerm{
								{MatchExpressions: []collector.NodeSelectorRequirement{
									{Key: "gpu", Operator: "In", Values: []string{"true"}},
								}},
							},
						},
					},
				},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 1_000_000_000}}}},
		},
	}

	result, err := Simulate(snap)
	require.NoError(t, err)
	assert.Empty(t, result.UnschedulablePods)

	nodes := findPlacementNodes(result, "ml-pod")
	assert.Equal(t, []string{"gpu-node"}, nodes)
}

func TestSimulateLargeCluster(t *testing.T) {
	// 10 nodes, 50 pods — all should be placed
	var nodes []collector.NodeSnapshot
	for i := range 10 {
		nodes = append(nodes, collector.NodeSnapshot{
			Name:        nodeN(i),
			Allocatable: collector.ResourcePair{CPU: 8000, Memory: 32_000_000_000},
			Labels:      map[string]string{"zone": zoneForIdx(i)},
		})
	}

	var pods []collector.PodSnapshot
	for i := range 50 {
		pods = append(pods, collector.PodSnapshot{
			Name:      podN(i),
			Namespace: "default",
			OwnerRef:  collector.OwnerReference{Kind: "Deployment", Name: "app"},
			Containers: []collector.ContainerSnapshot{
				{Requests: collector.ResourcePair{CPU: 500, Memory: 1_000_000_000}},
			},
		})
	}

	snap := &collector.ClusterSnapshot{Nodes: nodes, Pods: pods}
	result, err := Simulate(snap)
	require.NoError(t, err)

	assert.Equal(t, 50, len(result.Placements))
	assert.Empty(t, result.UnschedulablePods)
	assert.Equal(t, 10, result.TotalNodes)

	// Verify utilization: total CPU = 50 * 500 = 25000m across 10 nodes of 8000m each
	// Optimal packing: 4 nodes with ~6000-7000m, rest less used
	totalUsedCPU := int64(0)
	for _, util := range result.NodeUtilization {
		totalUsedCPU += util.CPUUsed
	}
	assert.Equal(t, int64(25000), totalUsedCPU)
}

func TestSimulateSortOrder(t *testing.T) {
	// Verify that larger pods are placed first (better packing)
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", Allocatable: collector.ResourcePair{CPU: 3000, Memory: 10_000_000_000}},
		},
		Pods: []collector.PodSnapshot{
			{Name: "small", Namespace: "default",
				OwnerRef:   collector.OwnerReference{Kind: "Deployment", Name: "a"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 500, Memory: 500_000_000}}}},
			{Name: "large", Namespace: "default",
				OwnerRef:   collector.OwnerReference{Kind: "Deployment", Name: "b"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 2000, Memory: 2_000_000_000}}}},
			{Name: "medium", Namespace: "default",
				OwnerRef:   collector.OwnerReference{Kind: "Deployment", Name: "c"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 1_000_000_000}}}},
		},
	}

	result, err := Simulate(snap)
	require.NoError(t, err)

	// All should fit: 500+2000+1000 = 3500 > 3000, so one should be unschedulable
	// Actually 3500 > 3000, so not all fit
	assert.Equal(t, 2, len(result.Placements))
	assert.Equal(t, 1, len(result.UnschedulablePods))

	// The large pod (2000m) should be placed first, then medium (1000m), small doesn't fit
	placedNames := make(map[string]bool)
	for _, p := range result.Placements {
		placedNames[p.PodName] = true
	}
	assert.True(t, placedNames["large"], "large pod should be placed (sorted first)")
	assert.True(t, placedNames["medium"], "medium pod should be placed")
	assert.Equal(t, "small", result.UnschedulablePods[0].Name, "small pod should be unschedulable")
}

func TestSimulateEmptyCluster(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000}},
		},
	}

	result, err := Simulate(snap)
	require.NoError(t, err)
	assert.Empty(t, result.Placements)
	assert.Empty(t, result.UnschedulablePods)
}

// ── Priorities tests ─────────────────────────────────────────────────────

func TestScoreNodeMostRequested(t *testing.T) {
	pod := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 1000, Memory: 2_000_000_000}},
		},
	}

	// Node already 50% used — should score higher
	busyNode := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000},
		UsedCPU:     2000,
		UsedMemory:  4_000_000_000,
	}

	// Node empty — should score lower
	emptyNode := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000},
	}

	busyScore := ScoreNode(pod, busyNode)
	emptyScore := ScoreNode(pod, emptyNode)

	assert.Greater(t, busyScore, emptyScore, "busier node should score higher")
}

func TestScoreNodeImbalancePenalty(t *testing.T) {
	// Pod with balanced CPU:memory
	balanced := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 1000, Memory: 2_000_000_000}},
		},
	}

	// Node where placing gives balanced utilization
	balancedNode := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000},
		UsedCPU:     1000,
		UsedMemory:  2_000_000_000,
	}

	// Node where placing gives imbalanced utilization
	imbalancedNode := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000},
		UsedCPU:     3000, // almost full on CPU
		UsedMemory:  0,    // empty on memory
	}

	balancedScore := ScoreNode(balanced, balancedNode)
	imbalancedScore := ScoreNode(balanced, imbalancedNode)

	assert.GreaterOrEqual(t, balancedScore, imbalancedScore, "balanced node should score >= imbalanced")
}

func TestScoreNodeZeroAllocatable(t *testing.T) {
	pod := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 100, Memory: 100}},
		},
	}
	node := &NodeState{Allocatable: collector.ResourcePair{CPU: 0, Memory: 0}}
	assert.Equal(t, 0, ScoreNode(pod, node))
}

func TestScoreNodeClampedToZero(t *testing.T) {
	// Extreme imbalance should not produce negative scores
	pod := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 100, Memory: 100}},
		},
	}
	node := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 100000, Memory: 100},
		UsedCPU:     0,
		UsedMemory:  0,
	}
	score := ScoreNode(pod, node)
	assert.GreaterOrEqual(t, score, 0)
}

// ── helpers ──────────────────────────────────────────────────────────────

func countPlacementsPerNode(result *SimulationResult) map[string]int {
	counts := make(map[string]int)
	for _, p := range result.Placements {
		counts[p.NodeName]++
	}
	return counts
}

func findPlacementNodes(result *SimulationResult, podName string) []string {
	var nodes []string
	for _, p := range result.Placements {
		if p.PodName == podName {
			nodes = append(nodes, p.NodeName)
		}
	}
	return nodes
}

func nodeN(i int) string {
	return "node-" + string(rune('a'+i))
}

func podN(i int) string {
	return "pod-" + itoa(i)
}

func itoa(i int) string {
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

func zoneForIdx(i int) string {
	zones := []string{"a", "b", "c"}
	return zones[i%len(zones)]
}
