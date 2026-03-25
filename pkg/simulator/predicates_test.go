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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tochemey/kubewise/pkg/collector"
)

// ── FitsResources ────────────────────────────────────────────────────────

func TestFitsResourcesExactFit(t *testing.T) {
	pod := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 1000, Memory: 1024}},
		},
	}
	node := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 1000, Memory: 1024},
	}
	assert.True(t, FitsResources(pod, node))
}

func TestFitsResourcesSlightOverflow(t *testing.T) {
	pod := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 1001, Memory: 1024}},
		},
	}
	node := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 1000, Memory: 1024},
	}
	assert.False(t, FitsResources(pod, node))
}

func TestFitsResourcesMemoryOverflow(t *testing.T) {
	pod := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 500, Memory: 2048}},
		},
	}
	node := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 1000, Memory: 1024},
	}
	assert.False(t, FitsResources(pod, node))
}

func TestFitsResourcesZeroRequests(t *testing.T) {
	pod := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 0, Memory: 0}},
		},
	}
	node := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 1000, Memory: 1024},
	}
	assert.True(t, FitsResources(pod, node))
}

func TestFitsResourcesMultipleContainers(t *testing.T) {
	pod := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 300, Memory: 256}},
			{Requests: collector.ResourcePair{CPU: 300, Memory: 256}},
			{Requests: collector.ResourcePair{CPU: 300, Memory: 256}},
		},
	}
	// 900m CPU, 768 memory — fits in 1000/1024
	node := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 1000, Memory: 1024},
	}
	assert.True(t, FitsResources(pod, node))

	// Add a 4th container — 1200m CPU, won't fit
	pod.Containers = append(pod.Containers, collector.ContainerSnapshot{
		Requests: collector.ResourcePair{CPU: 300, Memory: 256},
	})
	assert.False(t, FitsResources(pod, node))
}

func TestFitsResourcesAccountsForUsedResources(t *testing.T) {
	pod := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 500, Memory: 512}},
		},
	}
	node := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 1000, Memory: 1024},
		UsedCPU:     600, // only 400m available
		UsedMemory:  0,
	}
	assert.False(t, FitsResources(pod, node))
}

// ── MatchesAffinity ──────────────────────────────────────────────────────

func TestMatchesAffinityNoAffinity(t *testing.T) {
	pod := collector.PodSnapshot{}
	node := &NodeState{Labels: map[string]string{"zone": "us-east-1a"}}
	assert.True(t, MatchesAffinity(pod, node))
}

func TestMatchesAffinityNilNodeAffinity(t *testing.T) {
	pod := collector.PodSnapshot{
		Affinity: &collector.Affinity{},
	}
	node := &NodeState{Labels: map[string]string{"zone": "us-east-1a"}}
	assert.True(t, MatchesAffinity(pod, node))
}

func TestMatchesAffinityNilRequired(t *testing.T) {
	pod := collector.PodSnapshot{
		Affinity: &collector.Affinity{
			NodeAffinity: &collector.NodeAffinity{},
		},
	}
	node := &NodeState{Labels: map[string]string{"zone": "us-east-1a"}}
	assert.True(t, MatchesAffinity(pod, node))
}

func TestMatchesAffinitySingleTermMatch(t *testing.T) {
	pod := collector.PodSnapshot{
		Affinity: &collector.Affinity{
			NodeAffinity: &collector.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &collector.NodeSelector{
					NodeSelectorTerms: []collector.NodeSelectorTerm{
						{MatchExpressions: []collector.NodeSelectorRequirement{
							{Key: "zone", Operator: "In", Values: []string{"us-east-1a", "us-east-1b"}},
						}},
					},
				},
			},
		},
	}
	nodeMatch := &NodeState{Labels: map[string]string{"zone": "us-east-1a"}}
	assert.True(t, MatchesAffinity(pod, nodeMatch))

	nodeNoMatch := &NodeState{Labels: map[string]string{"zone": "us-west-2a"}}
	assert.False(t, MatchesAffinity(pod, nodeNoMatch))
}

func TestMatchesAffinityMultipleTermsOR(t *testing.T) {
	pod := collector.PodSnapshot{
		Affinity: &collector.Affinity{
			NodeAffinity: &collector.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &collector.NodeSelector{
					NodeSelectorTerms: []collector.NodeSelectorTerm{
						{MatchExpressions: []collector.NodeSelectorRequirement{
							{Key: "zone", Operator: "In", Values: []string{"us-east-1a"}},
						}},
						{MatchExpressions: []collector.NodeSelectorRequirement{
							{Key: "tier", Operator: "In", Values: []string{"gpu"}},
						}},
					},
				},
			},
		},
	}
	// Matches first term
	assert.True(t, MatchesAffinity(pod, &NodeState{Labels: map[string]string{"zone": "us-east-1a"}}))
	// Matches second term
	assert.True(t, MatchesAffinity(pod, &NodeState{Labels: map[string]string{"tier": "gpu"}}))
	// Matches neither
	assert.False(t, MatchesAffinity(pod, &NodeState{Labels: map[string]string{"zone": "us-west-2a"}}))
}

func TestMatchesAffinityOperatorNotIn(t *testing.T) {
	pod := collector.PodSnapshot{
		Affinity: &collector.Affinity{
			NodeAffinity: &collector.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &collector.NodeSelector{
					NodeSelectorTerms: []collector.NodeSelectorTerm{
						{MatchExpressions: []collector.NodeSelectorRequirement{
							{Key: "zone", Operator: "NotIn", Values: []string{"us-east-1a"}},
						}},
					},
				},
			},
		},
	}
	// Not in the exclusion list — matches
	assert.True(t, MatchesAffinity(pod, &NodeState{Labels: map[string]string{"zone": "us-west-2a"}}))
	// In the exclusion list — doesn't match
	assert.False(t, MatchesAffinity(pod, &NodeState{Labels: map[string]string{"zone": "us-east-1a"}}))
	// Key doesn't exist — NotIn is satisfied
	assert.True(t, MatchesAffinity(pod, &NodeState{Labels: map[string]string{}}))
}

func TestMatchesAffinityOperatorExists(t *testing.T) {
	pod := collector.PodSnapshot{
		Affinity: &collector.Affinity{
			NodeAffinity: &collector.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &collector.NodeSelector{
					NodeSelectorTerms: []collector.NodeSelectorTerm{
						{MatchExpressions: []collector.NodeSelectorRequirement{
							{Key: "gpu", Operator: "Exists"},
						}},
					},
				},
			},
		},
	}
	assert.True(t, MatchesAffinity(pod, &NodeState{Labels: map[string]string{"gpu": "true"}}))
	assert.True(t, MatchesAffinity(pod, &NodeState{Labels: map[string]string{"gpu": ""}}))
	assert.False(t, MatchesAffinity(pod, &NodeState{Labels: map[string]string{"cpu": "true"}}))
}

func TestMatchesAffinityOperatorDoesNotExist(t *testing.T) {
	pod := collector.PodSnapshot{
		Affinity: &collector.Affinity{
			NodeAffinity: &collector.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &collector.NodeSelector{
					NodeSelectorTerms: []collector.NodeSelectorTerm{
						{MatchExpressions: []collector.NodeSelectorRequirement{
							{Key: "gpu", Operator: "DoesNotExist"},
						}},
					},
				},
			},
		},
	}
	assert.True(t, MatchesAffinity(pod, &NodeState{Labels: map[string]string{"cpu": "true"}}))
	assert.False(t, MatchesAffinity(pod, &NodeState{Labels: map[string]string{"gpu": "true"}}))
}

// ── ToleratesTaints ──────────────────────────────────────────────────────

func TestToleratesTaintsNoTaints(t *testing.T) {
	pod := collector.PodSnapshot{}
	node := &NodeState{}
	assert.True(t, ToleratesTaints(pod, node))
}

func TestToleratesTaintsSingleTaintTolerated(t *testing.T) {
	pod := collector.PodSnapshot{
		Tolerations: []collector.Toleration{
			{Key: "dedicated", Operator: "Equal", Value: "gpu", Effect: "NoSchedule"},
		},
	}
	node := &NodeState{
		Taints: []collector.Taint{
			{Key: "dedicated", Value: "gpu", Effect: "NoSchedule"},
		},
	}
	assert.True(t, ToleratesTaints(pod, node))
}

func TestToleratesTaintsUntoleratedTaint(t *testing.T) {
	pod := collector.PodSnapshot{} // no tolerations
	node := &NodeState{
		Taints: []collector.Taint{
			{Key: "dedicated", Value: "gpu", Effect: "NoSchedule"},
		},
	}
	assert.False(t, ToleratesTaints(pod, node))
}

func TestToleratesTaintsWrongValue(t *testing.T) {
	pod := collector.PodSnapshot{
		Tolerations: []collector.Toleration{
			{Key: "dedicated", Operator: "Equal", Value: "cpu", Effect: "NoSchedule"},
		},
	}
	node := &NodeState{
		Taints: []collector.Taint{
			{Key: "dedicated", Value: "gpu", Effect: "NoSchedule"},
		},
	}
	assert.False(t, ToleratesTaints(pod, node))
}

func TestToleratesTaintsExistsOperator(t *testing.T) {
	pod := collector.PodSnapshot{
		Tolerations: []collector.Toleration{
			{Key: "dedicated", Operator: "Exists", Effect: "NoSchedule"},
		},
	}
	node := &NodeState{
		Taints: []collector.Taint{
			{Key: "dedicated", Value: "gpu", Effect: "NoSchedule"},
		},
	}
	assert.True(t, ToleratesTaints(pod, node))
}

func TestToleratesTaintsWildcardToleration(t *testing.T) {
	pod := collector.PodSnapshot{
		Tolerations: []collector.Toleration{
			{Operator: "Exists"}, // empty key + Exists = tolerates all
		},
	}
	node := &NodeState{
		Taints: []collector.Taint{
			{Key: "dedicated", Value: "gpu", Effect: "NoSchedule"},
			{Key: "another", Value: "taint", Effect: "NoSchedule"},
		},
	}
	assert.True(t, ToleratesTaints(pod, node))
}

func TestToleratesTaintsIgnoresNonNoSchedule(t *testing.T) {
	pod := collector.PodSnapshot{} // no tolerations
	node := &NodeState{
		Taints: []collector.Taint{
			{Key: "node.kubernetes.io/not-ready", Effect: "NoExecute"},
			{Key: "prefer", Effect: "PreferNoSchedule"},
		},
	}
	// NoExecute and PreferNoSchedule are ignored
	assert.True(t, ToleratesTaints(pod, node))
}

func TestToleratesTaintsEmptyEffectMatches(t *testing.T) {
	pod := collector.PodSnapshot{
		Tolerations: []collector.Toleration{
			{Key: "dedicated", Operator: "Equal", Value: "gpu"}, // empty effect matches all
		},
	}
	node := &NodeState{
		Taints: []collector.Taint{
			{Key: "dedicated", Value: "gpu", Effect: "NoSchedule"},
		},
	}
	assert.True(t, ToleratesTaints(pod, node))
}

func TestToleratesTaintsMultipleTaints(t *testing.T) {
	pod := collector.PodSnapshot{
		Tolerations: []collector.Toleration{
			{Key: "dedicated", Operator: "Equal", Value: "gpu", Effect: "NoSchedule"},
			// Missing toleration for "critical" taint
		},
	}
	node := &NodeState{
		Taints: []collector.Taint{
			{Key: "dedicated", Value: "gpu", Effect: "NoSchedule"},
			{Key: "critical", Value: "true", Effect: "NoSchedule"},
		},
	}
	assert.False(t, ToleratesTaints(pod, node))
}

// ── SatisfiesTopologySpread ──────────────────────────────────────────────

func TestTopologySpreadNoConstraints(t *testing.T) {
	pod := collector.PodSnapshot{}
	node := &NodeState{Labels: map[string]string{"zone": "a"}}
	state := &SchedulingState{Nodes: map[string]*NodeState{"n1": node}}
	assert.True(t, SatisfiesTopologySpread(pod, node, state))
}

func TestTopologySpreadScheduleAnyway(t *testing.T) {
	pod := collector.PodSnapshot{
		TopologySpreadConstraints: []collector.TopologySpreadConstraint{
			{MaxSkew: 1, TopologyKey: "zone", WhenUnsatisfiable: "ScheduleAnyway", LabelSelector: map[string]string{"app": "web"}},
		},
	}
	node := &NodeState{Labels: map[string]string{"zone": "a"}}
	state := &SchedulingState{Nodes: map[string]*NodeState{"n1": node}}
	// ScheduleAnyway is always satisfied
	assert.True(t, SatisfiesTopologySpread(pod, node, state))
}

func TestTopologySpreadBalancedPlacement(t *testing.T) {
	nodeA := &NodeState{
		Name:   "node-a",
		Labels: map[string]string{"zone": "a"},
		Pods: []collector.PodSnapshot{
			{Labels: map[string]string{"app": "web"}},
		},
	}
	nodeB := &NodeState{
		Name:   "node-b",
		Labels: map[string]string{"zone": "b"},
		// No pods yet
	}

	state := &SchedulingState{
		Nodes: map[string]*NodeState{"node-a": nodeA, "node-b": nodeB},
	}

	pod := collector.PodSnapshot{
		Labels: map[string]string{"app": "web"},
		TopologySpreadConstraints: []collector.TopologySpreadConstraint{
			{MaxSkew: 1, TopologyKey: "zone", WhenUnsatisfiable: "DoNotSchedule", LabelSelector: map[string]string{"app": "web"}},
		},
	}

	// Placing on zone-b: a=1, b=0+1=1. Skew=0 <= 1. OK.
	assert.True(t, SatisfiesTopologySpread(pod, nodeB, state))

	// Placing on zone-a: a=1+1=2, b=0. Skew=2 > 1. Violation!
	assert.False(t, SatisfiesTopologySpread(pod, nodeA, state))
}

func TestTopologySpreadMissingTopologyKey(t *testing.T) {
	node := &NodeState{
		Name:   "node-no-zone",
		Labels: map[string]string{"rack": "r1"}, // no "zone" label
	}
	state := &SchedulingState{
		Nodes: map[string]*NodeState{"node-no-zone": node},
	}

	pod := collector.PodSnapshot{
		TopologySpreadConstraints: []collector.TopologySpreadConstraint{
			{MaxSkew: 1, TopologyKey: "zone", WhenUnsatisfiable: "DoNotSchedule", LabelSelector: map[string]string{"app": "web"}},
		},
	}

	assert.False(t, SatisfiesTopologySpread(pod, node, state))
}

// ── SchedulingState ──────────────────────────────────────────────────────

func TestNewSchedulingState(t *testing.T) {
	nodes := []collector.NodeSnapshot{
		{Name: "n1", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8000}, Labels: map[string]string{"zone": "a"}},
		{Name: "n2", Allocatable: collector.ResourcePair{CPU: 2000, Memory: 4000}, Labels: map[string]string{"zone": "b"}},
	}

	state := NewSchedulingState(nodes)
	assert.Equal(t, 2, len(state.Nodes))
	assert.Equal(t, int64(4000), state.Nodes["n1"].Allocatable.CPU)
	assert.Equal(t, "a", state.Nodes["n1"].Labels["zone"])
}

func TestSchedulingStatePlace(t *testing.T) {
	nodes := []collector.NodeSnapshot{
		{Name: "n1", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8000}},
	}
	state := NewSchedulingState(nodes)

	pod := collector.PodSnapshot{
		Name: "p1", Namespace: "default",
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 500, Memory: 1024}},
		},
	}

	state.Place(pod, "n1")

	assert.Equal(t, int64(500), state.Nodes["n1"].UsedCPU)
	assert.Equal(t, int64(1024), state.Nodes["n1"].UsedMemory)
	assert.Equal(t, 1, len(state.Nodes["n1"].Pods))
	assert.Equal(t, 1, len(state.Placements))
	assert.Equal(t, "n1", state.Placements[0].NodeName)
}

func TestSchedulingStateActiveNodes(t *testing.T) {
	nodes := []collector.NodeSnapshot{
		{Name: "n1", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8000}},
		{Name: "n2", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8000}},
	}
	state := NewSchedulingState(nodes)

	assert.Equal(t, 0, len(state.ActiveNodes()))

	state.Place(collector.PodSnapshot{Name: "p1", Namespace: "default"}, "n1")
	assert.Equal(t, 1, len(state.ActiveNodes()))
}

func TestSchedulingStateUtilization(t *testing.T) {
	nodes := []collector.NodeSnapshot{
		{Name: "n1", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8000}},
	}
	state := NewSchedulingState(nodes)
	state.Place(collector.PodSnapshot{
		Name: "p1", Namespace: "default",
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 1000, Memory: 2000}},
		},
	}, "n1")

	util := state.Utilization()
	assert.Equal(t, int64(1000), util["n1"].CPUUsed)
	assert.Equal(t, int64(4000), util["n1"].CPUTotal)
	assert.Equal(t, 1, util["n1"].PodCount)
}
