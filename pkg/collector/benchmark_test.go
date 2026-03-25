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

package collector

import (
	"strconv"
	"testing"
)

func makeSnapshotForDeepCopy(nodeCount, podCount int) *ClusterSnapshot {
	nodes := make([]NodeSnapshot, nodeCount)
	for i := range nodeCount {
		nodes[i] = NodeSnapshot{
			Name:         "node-" + strconv.Itoa(i),
			InstanceType: "m6i.xlarge",
			Region:       "us-east-1",
			Zone:         "us-east-1a",
			Allocatable:  ResourcePair{CPU: 4000, Memory: 16_000_000_000},
			Capacity:     ResourcePair{CPU: 4000, Memory: 16_000_000_000},
			Labels:       map[string]string{"zone": "a", "pool": "default", "instance-type": "m6i.xlarge"},
			Taints:       []Taint{{Key: "dedicated", Value: "gpu", Effect: "NoSchedule"}},
			Conditions:   []NodeCondition{{Type: "Ready", Status: "True"}},
			NodePool:     "default-pool",
		}
	}

	pods := make([]PodSnapshot, podCount)
	for i := range podCount {
		pods[i] = PodSnapshot{
			Name:      "pod-" + strconv.Itoa(i),
			Namespace: "ns-" + strconv.Itoa(i%10),
			OwnerRef:  OwnerReference{Kind: "Deployment", Name: "app-" + strconv.Itoa(i%20)},
			Containers: []ContainerSnapshot{
				{
					Name:     "main",
					Image:    "app:v1",
					Requests: ResourcePair{CPU: 500, Memory: 256_000_000},
					Limits:   ResourcePair{CPU: 1000, Memory: 512_000_000},
				},
			},
			NodeName: "node-" + strconv.Itoa(i%nodeCount),
			Labels:   map[string]string{"app": "bench", "env": "prod"},
			Phase:    "Running",
		}
	}

	usageProfiles := make(map[string]ContainerUsageProfile, podCount)
	for i := range podCount {
		key := ProfileKey("ns-"+strconv.Itoa(i%10), "pod-"+strconv.Itoa(i), "main")
		usageProfiles[key] = ContainerUsageProfile{
			CurrentCPU: 120, CurrentMemory: 128_000_000,
			P50CPU: 100, P90CPU: 150, P95CPU: 180, P99CPU: 220,
			P50Memory: 100_000_000, P90Memory: 140_000_000,
			P95Memory: 160_000_000, P99Memory: 200_000_000,
			DataPoints: 2016,
		}
	}

	return &ClusterSnapshot{
		Nodes:        nodes,
		Pods:         pods,
		UsageProfile: usageProfiles,
	}
}

func BenchmarkDeepCopy(b *testing.B) {
	snap := makeSnapshotForDeepCopy(1000, 10000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = snap.DeepCopy()
	}
}

func BenchmarkDeepCopySmall(b *testing.B) {
	snap := makeSnapshotForDeepCopy(10, 100)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = snap.DeepCopy()
	}
}

func BenchmarkProfileKey(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = ProfileKey("default", "web-abc123-def456", "nginx")
	}
}
