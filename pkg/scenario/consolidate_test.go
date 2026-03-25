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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tochemey/kubewise/pkg/collector"
)

func newConsolidateSnapshot() *collector.ClusterSnapshot {
	return &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", InstanceType: "m5.large", Allocatable: collector.ResourcePair{CPU: 2000, Memory: 8_000_000_000},
				NodePool: "default-pool", Labels: map[string]string{"eks.amazonaws.com/nodegroup": "default-pool"}},
			{Name: "n2", InstanceType: "m5.large", Allocatable: collector.ResourcePair{CPU: 2000, Memory: 8_000_000_000},
				NodePool: "default-pool", Labels: map[string]string{"eks.amazonaws.com/nodegroup": "default-pool"}},
			{Name: "n3", InstanceType: "c5.large", Allocatable: collector.ResourcePair{CPU: 2000, Memory: 4_000_000_000},
				NodePool: "compute-pool", Labels: map[string]string{"eks.amazonaws.com/nodegroup": "compute-pool"}},
			{Name: "gpu-1", InstanceType: "p3.2xlarge", Allocatable: collector.ResourcePair{CPU: 8000, Memory: 64_000_000_000},
				NodePool: "gpu-pool", Labels: map[string]string{"eks.amazonaws.com/nodegroup": "gpu-pool"},
				Taints: []collector.Taint{{Key: "nvidia.com/gpu", Value: "true", Effect: "NoSchedule"}}},
			{Name: "n4", InstanceType: "m5.xlarge", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000},
				NodePool: "default-pool", Labels: map[string]string{"eks.amazonaws.com/nodegroup": "default-pool"}},
		},
		Pods: []collector.PodSnapshot{
			{Name: "web-1", Namespace: "default", NodeName: "n1",
				OwnerRef:   collector.OwnerReference{Kind: "Deployment", Name: "web"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 500, Memory: 1_000_000_000}}}},
			{Name: "web-2", Namespace: "default", NodeName: "n2",
				OwnerRef:   collector.OwnerReference{Kind: "Deployment", Name: "web"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 500, Memory: 1_000_000_000}}}},
			{Name: "api-1", Namespace: "api", NodeName: "n3",
				OwnerRef:   collector.OwnerReference{Kind: "Deployment", Name: "api"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 1000, Memory: 2_000_000_000}}}},
			{Name: "gpu-job", Namespace: "default", NodeName: "gpu-1",
				OwnerRef:    collector.OwnerReference{Kind: "Job", Name: "train"},
				Tolerations: []collector.Toleration{{Key: "nvidia.com/gpu", Operator: "Exists"}},
				Containers:  []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 4000, Memory: 32_000_000_000}}}},
			{Name: "worker-1", Namespace: "default", NodeName: "n4",
				OwnerRef:   collector.OwnerReference{Kind: "Deployment", Name: "worker"},
				Containers: []collector.ContainerSnapshot{{Requests: collector.ResourcePair{CPU: 2000, Memory: 4_000_000_000}}}},
		},
	}
}

func TestConsolidateBasic(t *testing.T) {
	snap := newConsolidateSnapshot()
	cs := &ConsolidateScenario{
		TargetNodeType:    "m6i.xlarge",
		TargetAllocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000},
	}

	result, err := ApplyScenario(cs, snap)
	require.NoError(t, err)

	// All original nodes should be removed, replaced by 1 virtual node
	assert.Equal(t, 1, len(result.Nodes))
	assert.Equal(t, "m6i.xlarge", result.Nodes[0].InstanceType)
	assert.Contains(t, result.Nodes[0].Name, "virtual")

	// All pods should be present, but those from removed nodes are unscheduled
	assert.Equal(t, 5, len(result.Pods))
	for _, pod := range result.Pods {
		assert.Empty(t, pod.NodeName, "pod %s should be unscheduled", pod.Name)
	}

	// Original should be unchanged
	assert.Equal(t, 5, len(snap.Nodes))
}

func TestConsolidateKeepNodePools(t *testing.T) {
	snap := newConsolidateSnapshot()
	cs := &ConsolidateScenario{
		TargetNodeType:    "m6i.xlarge",
		TargetAllocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000},
		KeepNodePools:     []string{"gpu-pool"},
	}

	result, err := ApplyScenario(cs, snap)
	require.NoError(t, err)

	// Should have: gpu-1 (kept) + 1 virtual node = 2 nodes
	assert.Equal(t, 2, len(result.Nodes))

	nodeNames := make(map[string]bool)
	for _, n := range result.Nodes {
		nodeNames[n.Name] = true
	}
	assert.True(t, nodeNames["gpu-1"], "gpu-pool node should be kept")

	// gpu-job should still be on gpu-1 (kept node)
	for _, pod := range result.Pods {
		if pod.Name == "gpu-job" {
			assert.Equal(t, "gpu-1", pod.NodeName, "gpu-job should stay on kept node")
		}
	}

	// Other pods should be unscheduled
	for _, pod := range result.Pods {
		if pod.Name != "gpu-job" {
			assert.Empty(t, pod.NodeName, "pod %s should be unscheduled", pod.Name)
		}
	}
}

func TestConsolidateMissingTargetType(t *testing.T) {
	snap := newConsolidateSnapshot()
	cs := &ConsolidateScenario{
		TargetAllocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000},
	}

	_, err := ApplyScenario(cs, snap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target node type")
}

func TestConsolidateMissingAllocatable(t *testing.T) {
	snap := newConsolidateSnapshot()
	cs := &ConsolidateScenario{
		TargetNodeType: "m6i.xlarge",
	}

	_, err := ApplyScenario(cs, snap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allocatable")
}

func TestConsolidateCreateVirtualNodes(t *testing.T) {
	cs := &ConsolidateScenario{
		TargetNodeType:    "m6i.xlarge",
		TargetAllocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000},
	}

	nodes := cs.CreateVirtualNodes(3)
	assert.Equal(t, 3, len(nodes))
	for i, node := range nodes {
		assert.Contains(t, node.Name, "virtual-m6i.xlarge")
		assert.Equal(t, "m6i.xlarge", node.InstanceType)
		assert.Equal(t, int64(4000), node.Allocatable.CPU)
		assert.Equal(t, "true", node.Labels["kubewise.io/virtual"])
		_ = i
	}
}

func TestConsolidateDetectNodePoolFromLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{"EKS", map[string]string{"eks.amazonaws.com/nodegroup": "pool-a"}, "pool-a"},
		{"GKE", map[string]string{"cloud.google.com/gke-nodepool": "pool-b"}, "pool-b"},
		{"AKS", map[string]string{"agentpool": "pool-c"}, "pool-c"},
		{"none", map[string]string{"other": "label"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, detectNodePoolFromLabels(tt.labels))
		})
	}
}
