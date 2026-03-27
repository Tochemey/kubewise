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

package scenario

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tochemey/kubewise/pkg/collector"
)

func newRightSizeTestSnapshot() *collector.ClusterSnapshot {
	return &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "node-1", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16 * 1024 * 1024 * 1024}},
		},
		Pods: []collector.PodSnapshot{
			{
				Name: "web-1", Namespace: "default",
				Labels: map[string]string{"app": "web"},
				Containers: []collector.ContainerSnapshot{
					{
						Name:     "nginx",
						Requests: collector.ResourcePair{CPU: 500, Memory: 512 * 1024 * 1024},
						Limits:   collector.ResourcePair{CPU: 1000, Memory: 1024 * 1024 * 1024},
					},
				},
			},
			{
				Name: "api-1", Namespace: "api",
				Labels: map[string]string{"app": "api"},
				Containers: []collector.ContainerSnapshot{
					{
						Name:     "api",
						Requests: collector.ResourcePair{CPU: 250, Memory: 256 * 1024 * 1024},
						Limits:   collector.ResourcePair{CPU: 500, Memory: 512 * 1024 * 1024},
					},
				},
			},
			{
				Name: "system-1", Namespace: "kube-system",
				Labels: map[string]string{"app": "coredns"},
				Containers: []collector.ContainerSnapshot{
					{
						Name:     "coredns",
						Requests: collector.ResourcePair{CPU: 100, Memory: 128 * 1024 * 1024},
						Limits:   collector.ResourcePair{CPU: 200, Memory: 256 * 1024 * 1024},
					},
				},
			},
			{
				Name: "skip-1", Namespace: "default",
				Labels: map[string]string{"kubewise.io/skip": "true"},
				Containers: []collector.ContainerSnapshot{
					{
						Name:     "skipped",
						Requests: collector.ResourcePair{CPU: 100, Memory: 128 * 1024 * 1024},
						Limits:   collector.ResourcePair{CPU: 200, Memory: 256 * 1024 * 1024},
					},
				},
			},
			{
				Name: "no-usage", Namespace: "default",
				Labels: map[string]string{"app": "no-metrics"},
				Containers: []collector.ContainerSnapshot{
					{
						Name:     "app",
						Requests: collector.ResourcePair{CPU: 300, Memory: 300 * 1024 * 1024},
						Limits:   collector.ResourcePair{CPU: 600, Memory: 600 * 1024 * 1024},
					},
				},
			},
			{
				Name: "tiny-1", Namespace: "default",
				Labels: map[string]string{"app": "tiny"},
				Containers: []collector.ContainerSnapshot{
					{
						Name:     "tiny",
						Requests: collector.ResourcePair{CPU: 50, Memory: 64 * 1024 * 1024},
						Limits:   collector.ResourcePair{CPU: 100, Memory: 128 * 1024 * 1024},
					},
				},
			},
		},
		UsageProfile: map[string]collector.ContainerUsageProfile{
			// web-1/nginx: significantly over-provisioned
			"default/web-1/nginx": {
				P50CPU: 50, P90CPU: 80, P95CPU: 100, P99CPU: 130,
				P50Memory: 100 * 1024 * 1024, P90Memory: 150 * 1024 * 1024,
				P95Memory: 180 * 1024 * 1024, P99Memory: 220 * 1024 * 1024,
				DataPoints: 2000,
			},
			// api-1/api: moderately over-provisioned
			"api/api-1/api": {
				P50CPU: 100, P90CPU: 150, P95CPU: 180, P99CPU: 210,
				P50Memory: 80 * 1024 * 1024, P90Memory: 120 * 1024 * 1024,
				P95Memory: 140 * 1024 * 1024, P99Memory: 170 * 1024 * 1024,
				DataPoints: 2000,
			},
			// system-1/coredns (should be excluded by scope)
			"kube-system/system-1/coredns": {
				P50CPU: 30, P90CPU: 50, P95CPU: 60, P99CPU: 80,
				P50Memory: 40 * 1024 * 1024, P90Memory: 60 * 1024 * 1024,
				P95Memory: 70 * 1024 * 1024, P99Memory: 90 * 1024 * 1024,
				DataPoints: 2000,
			},
			// skip-1/skipped (should be excluded by label)
			"default/skip-1/skipped": {
				P50CPU: 10, P90CPU: 20, P95CPU: 25, P99CPU: 30,
				P50Memory: 20 * 1024 * 1024, P90Memory: 30 * 1024 * 1024,
				P95Memory: 35 * 1024 * 1024, P99Memory: 40 * 1024 * 1024,
				DataPoints: 2000,
			},
			// no profile for "no-usage" pod
			// tiny-1/tiny: very low usage (should hit floors)
			"default/tiny-1/tiny": {
				P50CPU: 1, P90CPU: 3, P95CPU: 5, P99CPU: 7,
				P50Memory: 1 * 1024 * 1024, P90Memory: 2 * 1024 * 1024,
				P95Memory: 3 * 1024 * 1024, P99Memory: 5 * 1024 * 1024,
				DataPoints: 2000,
			},
		},
	}
}

func TestRightSizeBasicP95With20Buffer(t *testing.T) {
	snap := newRightSizeTestSnapshot()
	scenario := &RightSizeScenario{
		Percentile:    "p95",
		Buffer:        20,
		Scope:         Scope{Namespaces: []string{"*"}, ExcludeNamespaces: []string{"kube-system"}, ExcludeLabels: map[string]string{"kubewise.io/skip": "true"}},
		LimitStrategy: "ratio",
	}

	result, err := ApplyScenario(scenario, snap)
	require.NoError(t, err)

	// web-1/nginx: p95 CPU=100, +20% = 120m; p95 mem=180Mi, +20% = 216Mi
	// Original ratio: request 500 limit 1000 = 2.0x
	webPod := findPodInSnap(result, "default", "web-1")
	require.NotNil(t, webPod)
	assert.Equal(t, int64(120), webPod.Containers[0].Requests.CPU)
	assert.Equal(t, int64(240), webPod.Containers[0].Limits.CPU) // 120 * 2.0
	expectedMem := int64(float64(180*1024*1024) * 1.2)
	assert.Equal(t, expectedMem, webPod.Containers[0].Requests.Memory)

	// api-1/api: p95 CPU=180, +20% = 216m; p95 mem=140Mi, +20% = 168Mi
	// Original ratio: request 250 limit 500 = 2.0x
	apiPod := findPodInSnap(result, "api", "api-1")
	require.NotNil(t, apiPod)
	assert.Equal(t, int64(216), apiPod.Containers[0].Requests.CPU)
	assert.Equal(t, int64(432), apiPod.Containers[0].Limits.CPU) // 216 * 2.0

	// Original should be unchanged
	origWeb := findPodInSnap(snap, "default", "web-1")
	assert.Equal(t, int64(500), origWeb.Containers[0].Requests.CPU)
}

func TestRightSizeMinimumFloors(t *testing.T) {
	snap := newRightSizeTestSnapshot()
	scenario := &RightSizeScenario{
		Percentile:    "p95",
		Buffer:        20,
		Scope:         DefaultScope(),
		LimitStrategy: "fixed",
	}

	result, err := ApplyScenario(scenario, snap)
	require.NoError(t, err)

	// tiny-1/tiny: p95 CPU=5, +20% = 6 → floor at 10m
	// p95 mem=3Mi, +20% = 3.6Mi → floor at 32Mi
	tinyPod := findPodInSnap(result, "default", "tiny-1")
	require.NotNil(t, tinyPod)
	assert.Equal(t, int64(10), tinyPod.Containers[0].Requests.CPU)
	assert.Equal(t, int64(33554432), tinyPod.Containers[0].Requests.Memory) // 32Mi
}

func TestRightSizeLimitStrategyFixed(t *testing.T) {
	snap := newRightSizeTestSnapshot()
	scenario := &RightSizeScenario{
		Percentile:    "p95",
		Buffer:        20,
		Scope:         Scope{Namespaces: []string{"default"}, ExcludeLabels: map[string]string{"kubewise.io/skip": "true"}},
		LimitStrategy: "fixed",
	}

	result, err := ApplyScenario(scenario, snap)
	require.NoError(t, err)

	webPod := findPodInSnap(result, "default", "web-1")
	require.NotNil(t, webPod)
	// Fixed: limit = request
	assert.Equal(t, webPod.Containers[0].Requests.CPU, webPod.Containers[0].Limits.CPU)
	assert.Equal(t, webPod.Containers[0].Requests.Memory, webPod.Containers[0].Limits.Memory)
}

func TestRightSizeLimitStrategyUnbounded(t *testing.T) {
	snap := newRightSizeTestSnapshot()
	scenario := &RightSizeScenario{
		Percentile:    "p95",
		Buffer:        20,
		Scope:         Scope{Namespaces: []string{"default"}, ExcludeLabels: map[string]string{"kubewise.io/skip": "true"}},
		LimitStrategy: "unbounded",
	}

	result, err := ApplyScenario(scenario, snap)
	require.NoError(t, err)

	webPod := findPodInSnap(result, "default", "web-1")
	require.NotNil(t, webPod)
	// Unbounded: limit = 0
	assert.Equal(t, int64(0), webPod.Containers[0].Limits.CPU)
	assert.Equal(t, int64(0), webPod.Containers[0].Limits.Memory)
}

func TestRightSizeNoUsageDataSkipped(t *testing.T) {
	snap := newRightSizeTestSnapshot()
	scenario := &RightSizeScenario{
		Percentile:    "p95",
		Buffer:        20,
		Scope:         DefaultScope(),
		LimitStrategy: "ratio",
	}

	result, err := ApplyScenario(scenario, snap)
	require.NoError(t, err)

	// no-usage pod should be unchanged (no usage profile)
	noUsagePod := findPodInSnap(result, "default", "no-usage")
	require.NotNil(t, noUsagePod)
	assert.Equal(t, int64(300), noUsagePod.Containers[0].Requests.CPU)
	assert.Equal(t, int64(300*1024*1024), noUsagePod.Containers[0].Requests.Memory)
}

func TestRightScopeScopeExcludesNamespace(t *testing.T) {
	snap := newRightSizeTestSnapshot()
	scenario := &RightSizeScenario{
		Percentile:    "p95",
		Buffer:        20,
		Scope:         Scope{Namespaces: []string{"*"}, ExcludeNamespaces: []string{"kube-system"}},
		LimitStrategy: "ratio",
	}

	result, err := ApplyScenario(scenario, snap)
	require.NoError(t, err)

	// kube-system pod should be unchanged
	systemPod := findPodInSnap(result, "kube-system", "system-1")
	require.NotNil(t, systemPod)
	assert.Equal(t, int64(100), systemPod.Containers[0].Requests.CPU)
}

func TestRightSizeScopeExcludesLabel(t *testing.T) {
	snap := newRightSizeTestSnapshot()
	scenario := &RightSizeScenario{
		Percentile:    "p95",
		Buffer:        20,
		Scope:         Scope{Namespaces: []string{"*"}, ExcludeLabels: map[string]string{"kubewise.io/skip": "true"}},
		LimitStrategy: "ratio",
	}

	result, err := ApplyScenario(scenario, snap)
	require.NoError(t, err)

	// skip-1 pod should be unchanged
	skipPod := findPodInSnap(result, "default", "skip-1")
	require.NotNil(t, skipPod)
	assert.Equal(t, int64(100), skipPod.Containers[0].Requests.CPU)
}

func TestRightSizeP50Percentile(t *testing.T) {
	snap := newRightSizeTestSnapshot()
	scenario := &RightSizeScenario{
		Percentile:    "p50",
		Buffer:        20,
		Scope:         Scope{Namespaces: []string{"default"}, ExcludeLabels: map[string]string{"kubewise.io/skip": "true"}},
		LimitStrategy: "fixed",
	}

	result, err := ApplyScenario(scenario, snap)
	require.NoError(t, err)

	webPod := findPodInSnap(result, "default", "web-1")
	require.NotNil(t, webPod)
	// p50 CPU=50, +20% = 60m
	assert.Equal(t, int64(60), webPod.Containers[0].Requests.CPU)
}

func TestRightSizeP99Percentile(t *testing.T) {
	snap := newRightSizeTestSnapshot()
	scenario := &RightSizeScenario{
		Percentile:    "p99",
		Buffer:        10,
		Scope:         Scope{Namespaces: []string{"default"}, ExcludeLabels: map[string]string{"kubewise.io/skip": "true"}},
		LimitStrategy: "fixed",
	}

	result, err := ApplyScenario(scenario, snap)
	require.NoError(t, err)

	webPod := findPodInSnap(result, "default", "web-1")
	require.NotNil(t, webPod)
	// p99 CPU=130, +10% = 143m
	assert.Equal(t, int64(143), webPod.Containers[0].Requests.CPU)
}

func TestRightSizeInvalidPercentile(t *testing.T) {
	snap := newRightSizeTestSnapshot()
	scenario := &RightSizeScenario{
		Percentile:    "p75",
		Buffer:        20,
		Scope:         DefaultScope(),
		LimitStrategy: "ratio",
	}

	_, err := ApplyScenario(scenario, snap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid percentile")
}

func TestRightSizeInvalidLimitStrategy(t *testing.T) {
	snap := newRightSizeTestSnapshot()
	scenario := &RightSizeScenario{
		Percentile:    "p95",
		Buffer:        20,
		Scope:         DefaultScope(),
		LimitStrategy: "magic",
	}

	_, err := ApplyScenario(scenario, snap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid limit strategy")
}

func TestRightSizeApplyWithChanges(t *testing.T) {
	snap := newRightSizeTestSnapshot().DeepCopy()
	scenario := &RightSizeScenario{
		Percentile:    "p95",
		Buffer:        20,
		Scope:         Scope{Namespaces: []string{"default"}, ExcludeLabels: map[string]string{"kubewise.io/skip": "true"}},
		LimitStrategy: "ratio",
	}

	_, result, err := scenario.ApplyWithChanges(snap)
	require.NoError(t, err)

	// Should have changes for web-1/nginx and tiny-1/tiny
	// no-usage is skipped (no profile), skip-1 excluded by label
	assert.Equal(t, 2, len(result.Changes))
	assert.Equal(t, 1, result.Skipped) // no-usage

	// Verify a change record
	var webChange *RightSizeChange
	for i := range result.Changes {
		if result.Changes[i].Container == "nginx" {
			webChange = &result.Changes[i]
			break
		}
	}
	require.NotNil(t, webChange)
	assert.Equal(t, int64(500), webChange.OriginalCPU)
	assert.Equal(t, int64(120), webChange.NewCPU)
}

func TestRightSizeRatioWithZeroOriginalLimit(t *testing.T) {
	snap := &collector.ClusterSnapshot{
		Pods: []collector.PodSnapshot{
			{
				Name: "p1", Namespace: "default",
				Containers: []collector.ContainerSnapshot{
					{
						Name:     "c1",
						Requests: collector.ResourcePair{CPU: 100, Memory: 100 * 1024 * 1024},
						Limits:   collector.ResourcePair{CPU: 0, Memory: 0}, // no limits
					},
				},
			},
		},
		UsageProfile: map[string]collector.ContainerUsageProfile{
			"default/p1/c1": {P95CPU: 50, P95Memory: 60 * 1024 * 1024, DataPoints: 2000},
		},
	}

	scenario := &RightSizeScenario{
		Percentile:    "p95",
		Buffer:        20,
		Scope:         DefaultScope(),
		LimitStrategy: "ratio",
	}

	result, err := ApplyScenario(scenario, snap)
	require.NoError(t, err)

	pod := result.Pods[0]
	// When original limit is 0, ratio strategy should set limit = request
	assert.Equal(t, pod.Containers[0].Requests.CPU, pod.Containers[0].Limits.CPU)
}

func TestRightSizeZeroBuffer(t *testing.T) {
	snap := newRightSizeTestSnapshot()
	scenario := &RightSizeScenario{
		Percentile:    "p95",
		Buffer:        0,
		Scope:         Scope{Namespaces: []string{"default"}, ExcludeLabels: map[string]string{"kubewise.io/skip": "true"}},
		LimitStrategy: "fixed",
	}

	result, err := ApplyScenario(scenario, snap)
	require.NoError(t, err)

	webPod := findPodInSnap(result, "default", "web-1")
	require.NotNil(t, webPod)
	// p95 CPU=100, 0% buffer = 100m
	assert.Equal(t, int64(100), webPod.Containers[0].Requests.CPU)
}

// helpers

func findPodInSnap(snap *collector.ClusterSnapshot, ns, name string) *collector.PodSnapshot {
	for i := range snap.Pods {
		if snap.Pods[i].Namespace == ns && snap.Pods[i].Name == name {
			return &snap.Pods[i]
		}
	}
	return nil
}
