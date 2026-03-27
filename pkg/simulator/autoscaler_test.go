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

func TestAutoscalerAddsNodesUntilAllFit(t *testing.T) {
	// 5 pods each needing 1000m CPU, target node has 4000m
	// Need ceil(5000/4000) = 2 nodes
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "virtual-m6i.xlarge-1", InstanceType: "m6i.xlarge",
				Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000},
				Labels:      map[string]string{"node.kubernetes.io/instance-type": "m6i.xlarge"}},
		},
		Pods: makePods(5, 1000, 1_000_000_000),
	}

	target := collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}
	result, err := SimulateWithAutoscaler(snap, "m6i.xlarge", target, 0)
	require.NoError(t, err)

	assert.Empty(t, result.UnschedulablePods)
	assert.Equal(t, 5, len(result.Placements))
	assert.GreaterOrEqual(t, result.TotalNodes, 2)
}

func TestAutoscalerRespectsMaxNodes(t *testing.T) {
	// 10 pods each needing 2000m CPU, target node has 4000m → need 5 nodes
	// But max is 3 → some pods should be unschedulable
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "virtual-1", InstanceType: "m6i.xlarge",
				Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000},
				Labels:      map[string]string{"node.kubernetes.io/instance-type": "m6i.xlarge"}},
		},
		Pods: makePods(10, 2000, 2_000_000_000),
	}

	target := collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}
	result, err := SimulateWithAutoscaler(snap, "m6i.xlarge", target, 3)
	require.NoError(t, err)

	// 3 nodes × 4000m = 12000m capacity, 10 pods × 2000m = 20000m needed
	// 6 pods fit (3 nodes × 2 pods), 4 unschedulable
	assert.Equal(t, 6, len(result.Placements))
	assert.Equal(t, 4, len(result.UnschedulablePods))
	assert.Equal(t, 3, result.TotalNodes)
}

func TestAutoscalerAllFitInitially(t *testing.T) {
	// All pods fit on the initial node — no scaling needed
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "virtual-1", InstanceType: "m6i.xlarge",
				Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000},
				Labels:      map[string]string{"node.kubernetes.io/instance-type": "m6i.xlarge"}},
		},
		Pods: makePods(2, 1000, 2_000_000_000),
	}

	target := collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}
	result, err := SimulateWithAutoscaler(snap, "m6i.xlarge", target, 10)
	require.NoError(t, err)

	assert.Empty(t, result.UnschedulablePods)
	assert.Equal(t, 2, len(result.Placements))
	assert.Equal(t, 1, result.TotalNodes)
}

func TestAutoscalerNeedsMoreNodesThanCurrent(t *testing.T) {
	// Start with 1 node, need many — simulates consolidation to smaller nodes
	// 3 pods each needing 3000m CPU, target node has 4000m → need 3 nodes
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "virtual-1", InstanceType: "small-type",
				Allocatable: collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000},
				Labels:      map[string]string{"node.kubernetes.io/instance-type": "small-type"}},
		},
		Pods: makePods(3, 3000, 4_000_000_000),
	}

	target := collector.ResourcePair{CPU: 4000, Memory: 8_000_000_000}
	result, err := SimulateWithAutoscaler(snap, "small-type", target, 10)
	require.NoError(t, err)

	assert.Empty(t, result.UnschedulablePods)
	assert.Equal(t, 3, len(result.Placements))
	assert.Equal(t, 3, result.TotalNodes)
}

func TestAutoscalerWithDaemonSets(t *testing.T) {
	// DaemonSets consume resources on every node, reducing available capacity
	snap := &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "virtual-1", InstanceType: "m6i.xlarge",
				Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000},
				Labels:      map[string]string{"node.kubernetes.io/instance-type": "m6i.xlarge"}},
		},
		Pods: append(
			// DaemonSet pod that will be placed on every node
			[]collector.PodSnapshot{
				{Name: "monitor-1", Namespace: "kube-system",
					OwnerRef:   collector.OwnerReference{Kind: "DaemonSet", Name: "monitor"},
					Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 500, Memory: 500_000_000}}}},
			},
			// 4 regular pods needing 1000m each
			makePods(4, 1000, 2_000_000_000)...,
		),
	}

	target := collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}
	result, err := SimulateWithAutoscaler(snap, "m6i.xlarge", target, 10)
	require.NoError(t, err)

	assert.Empty(t, result.UnschedulablePods)
	// DaemonSet takes 500m per node, so each node has 3500m for regular pods
	// 4 pods × 1000m = 4000m → needs 2 nodes (3500m + 3500m = 7000m available)
	assert.GreaterOrEqual(t, result.TotalNodes, 2)
}

func makePods(count int, cpuMillis, memBytes int64) []collector.PodSnapshot {
	pods := make([]collector.PodSnapshot, count)
	for i := range count {
		pods[i] = collector.PodSnapshot{
			Name:      "pod-" + itoa(i),
			Namespace: "default",
			OwnerRef:  collector.OwnerReference{Kind: "Deployment", Name: "app"},
			Containers: []collector.ContainerSnapshot{
				{Requests: collector.ResourcePair{CPU: cpuMillis, Memory: memBytes}},
			},
		}
	}
	return pods
}
