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

	"github.com/tochemey/kubewise/pkg/collector"
)

func makeSnapshotForBench(nodeCount, podCount int) *collector.ClusterSnapshot {
	nodes := make([]collector.NodeSnapshot, nodeCount)
	for i := range nodeCount {
		nodes[i] = collector.NodeSnapshot{
			Name:        "node-" + itoa(i),
			Allocatable: collector.ResourcePair{CPU: 8000, Memory: 32_000_000_000},
			Labels:      map[string]string{"zone": zoneForIdx(i), "node.kubernetes.io/instance-type": "m6i.xlarge"},
		}
	}

	pods := make([]collector.PodSnapshot, podCount)
	for i := range podCount {
		pods[i] = collector.PodSnapshot{
			Name:      "pod-" + itoa(i),
			Namespace: "default",
			OwnerRef:  collector.OwnerReference{Kind: "Deployment", Name: "app"},
			Containers: []collector.ContainerSnapshot{
				{Requests: collector.ResourcePair{CPU: 200 + int64(i%300), Memory: 256_000_000 + int64(i%500)*1_000_000}},
			},
			Labels: map[string]string{"app": "bench"},
		}
	}

	return &collector.ClusterSnapshot{Nodes: nodes, Pods: pods}
}

func BenchmarkBinPack(b *testing.B) {
	snap := makeSnapshotForBench(100, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = Simulate(snap)
	}
}

func BenchmarkBinPackLarge(b *testing.B) {
	snap := makeSnapshotForBench(1000, 10000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = Simulate(snap)
	}
}

func BenchmarkPredicateFitsResources(b *testing.B) {
	pod := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 500, Memory: 1_000_000_000}},
		},
	}
	node := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000},
		UsedCPU:     2000,
		UsedMemory:  8_000_000_000,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		FitsResources(pod, node)
	}
}

func BenchmarkPredicateMatchesAffinity(b *testing.B) {
	pod := collector.PodSnapshot{
		Affinity: &collector.Affinity{
			NodeAffinity: &collector.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &collector.NodeSelector{
					NodeSelectorTerms: []collector.NodeSelectorTerm{
						{MatchExpressions: []collector.NodeSelectorRequirement{
							{Key: "zone", Operator: "In", Values: []string{"a", "b", "c"}},
						}},
					},
				},
			},
		},
	}
	node := &NodeState{Labels: map[string]string{"zone": "b"}}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		MatchesAffinity(pod, node)
	}
}

func BenchmarkPredicateToleratesTaints(b *testing.B) {
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
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		ToleratesTaints(pod, node)
	}
}

func BenchmarkScoreNode(b *testing.B) {
	pod := collector.PodSnapshot{
		Containers: []collector.ContainerSnapshot{
			{Requests: collector.ResourcePair{CPU: 500, Memory: 1_000_000_000}},
		},
	}
	node := &NodeState{
		Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000},
		UsedCPU:     2000,
		UsedMemory:  8_000_000_000,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		ScoreNode(pod, node)
	}
}
