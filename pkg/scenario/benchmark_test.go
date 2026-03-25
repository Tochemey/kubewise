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
	"strconv"
	"testing"

	"github.com/tochemey/kubewise/pkg/collector"
)

func makeRightSizeBenchSnapshot(podCount int) *collector.ClusterSnapshot {
	pods := make([]collector.PodSnapshot, podCount)
	usageProfiles := make(map[string]collector.ContainerUsageProfile, podCount)

	for i := range podCount {
		ns := "ns-" + strconv.Itoa(i%10)
		name := "pod-" + strconv.Itoa(i)
		pods[i] = collector.PodSnapshot{
			Name:      name,
			Namespace: ns,
			OwnerRef:  collector.OwnerReference{Kind: "Deployment", Name: "app"},
			Containers: []collector.ContainerSnapshot{
				{
					Name:     "main",
					Requests: collector.ResourcePair{CPU: 500, Memory: 512_000_000},
					Limits:   collector.ResourcePair{CPU: 1000, Memory: 1_024_000_000},
				},
			},
			Labels: map[string]string{"app": "bench"},
		}
		key := collector.ProfileKey(ns, name, "main")
		usageProfiles[key] = collector.ContainerUsageProfile{
			P50CPU: 50, P90CPU: 80, P95CPU: 100, P99CPU: 130,
			P50Memory: 100_000_000, P90Memory: 150_000_000,
			P95Memory: 180_000_000, P99Memory: 220_000_000,
			DataPoints: 2016,
		}
	}

	return &collector.ClusterSnapshot{
		Pods:         pods,
		UsageProfile: usageProfiles,
	}
}

func BenchmarkRightSizeApply(b *testing.B) {
	snap := makeRightSizeBenchSnapshot(10000)
	scenario := &RightSizeScenario{
		Percentile:    "p95",
		Buffer:        20,
		Scope:         DefaultScope(),
		LimitStrategy: "ratio",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = ApplyScenario(scenario, snap)
	}
}
