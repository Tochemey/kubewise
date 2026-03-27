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

func newSpotTestSnapshot() *collector.ClusterSnapshot {
	return &collector.ClusterSnapshot{
		Nodes: []collector.NodeSnapshot{
			{Name: "n1", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16_000_000_000}},
		},
		Controllers: []collector.ControllerSnapshot{
			{Kind: "Deployment", Name: "web", Namespace: "default", DesiredReplicas: 3},
			{Kind: "Deployment", Name: "api", Namespace: "api", DesiredReplicas: 2},
			{Kind: "StatefulSet", Name: "db", Namespace: "default", DesiredReplicas: 1},
			{Kind: "Deployment", Name: "critical", Namespace: "payments", DesiredReplicas: 3},
			{Kind: "DaemonSet", Name: "monitor", Namespace: "kube-system", DesiredReplicas: 2},
		},
		Pods: []collector.PodSnapshot{
			{Name: "web-1", Namespace: "default", OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "web"}},
			{Name: "web-2", Namespace: "default", OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "web"}},
			{Name: "web-3", Namespace: "default", OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "web"}},
			{Name: "api-1", Namespace: "api", OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "api"}},
			{Name: "api-2", Namespace: "api", OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "api"}},
			{Name: "db-1", Namespace: "default", OwnerRef: collector.OwnerReference{Kind: "StatefulSet", Name: "db"}},
			{Name: "critical-1", Namespace: "payments", OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "critical"}},
			{Name: "critical-2", Namespace: "payments", OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "critical"}},
			{Name: "critical-3", Namespace: "payments", OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "critical"}},
			{Name: "monitor-1", Namespace: "kube-system", OwnerRef: collector.OwnerReference{Kind: "DaemonSet", Name: "monitor"}},
		},
	}
}

func TestSpotMigrateBasic(t *testing.T) {
	snap := newSpotTestSnapshot()
	s := &SpotMigrateScenario{
		MinReplicas:       2,
		ControllerTypes:   []string{"Deployment"},
		ExcludeNamespaces: []string{"payments", "kube-system"},
		SpotDiscount:      0.65,
	}

	result, err := ApplyScenario(s, snap)
	require.NoError(t, err)

	// web (3 replicas, Deployment, default ns) → eligible
	for _, pod := range result.Pods {
		switch pod.Name {
		case "web-1", "web-2", "web-3":
			assert.True(t, pod.IsSpot, "%s should be spot-tagged", pod.Name)
		case "api-1", "api-2":
			assert.True(t, pod.IsSpot, "%s should be spot-tagged", pod.Name)
		case "db-1":
			// StatefulSet, not in ControllerTypes
			assert.False(t, pod.IsSpot, "db-1 should NOT be spot (StatefulSet)")
		case "critical-1", "critical-2", "critical-3":
			// payments namespace excluded
			assert.False(t, pod.IsSpot, "%s should NOT be spot (excluded ns)", pod.Name)
		case "monitor-1":
			// kube-system excluded + DaemonSet not in types
			assert.False(t, pod.IsSpot, "monitor-1 should NOT be spot")
		}
	}

	// Original should be unchanged
	for _, pod := range snap.Pods {
		assert.False(t, pod.IsSpot, "original pod %s should not be modified", pod.Name)
	}
}

func TestSpotMigrateMinReplicas(t *testing.T) {
	snap := newSpotTestSnapshot()
	s := &SpotMigrateScenario{
		MinReplicas:     3, // only workloads with 3+ replicas
		ControllerTypes: []string{"Deployment"},
		SpotDiscount:    0.65,
	}

	result, err := ApplyScenario(s, snap)
	require.NoError(t, err)

	// web (3 replicas) → eligible
	// api (2 replicas) → NOT eligible
	// critical (3 replicas, no exclusion) → eligible
	for _, pod := range result.Pods {
		switch pod.Name {
		case "web-1", "web-2", "web-3":
			assert.True(t, pod.IsSpot, "%s should be spot", pod.Name)
		case "api-1", "api-2":
			assert.False(t, pod.IsSpot, "%s should NOT be spot (only 2 replicas)", pod.Name)
		case "critical-1", "critical-2", "critical-3":
			assert.True(t, pod.IsSpot, "%s should be spot (3 replicas)", pod.Name)
		}
	}
}

func TestSpotMigrateControllerTypeFilter(t *testing.T) {
	snap := newSpotTestSnapshot()
	s := &SpotMigrateScenario{
		MinReplicas:     1,
		ControllerTypes: []string{"Deployment", "StatefulSet"},
		SpotDiscount:    0.65,
	}

	result, err := ApplyScenario(s, snap)
	require.NoError(t, err)

	// db-1 (StatefulSet) should now be eligible since StatefulSet is included
	dbPod := findPodInSnap(result, "default", "db-1")
	require.NotNil(t, dbPod)
	assert.True(t, dbPod.IsSpot)

	// DaemonSet monitor should NOT be eligible
	monitorPod := findPodInSnap(result, "kube-system", "monitor-1")
	require.NotNil(t, monitorPod)
	assert.False(t, monitorPod.IsSpot)
}

func TestSpotMigrateNoControllerTypeFilter(t *testing.T) {
	snap := newSpotTestSnapshot()
	s := &SpotMigrateScenario{
		MinReplicas:  1,
		SpotDiscount: 0.65,
		// Empty ControllerTypes = all types eligible
	}

	result, err := ApplyScenario(s, snap)
	require.NoError(t, err)

	// All pods should be spot-tagged (no type filter, min replicas = 1)
	spotCount := 0
	for _, pod := range result.Pods {
		if pod.IsSpot {
			spotCount++
		}
	}
	assert.Equal(t, 10, spotCount)
}

func TestSpotMigrateNamespaceExclusion(t *testing.T) {
	snap := newSpotTestSnapshot()
	s := &SpotMigrateScenario{
		MinReplicas:       1,
		ControllerTypes:   []string{"Deployment"},
		ExcludeNamespaces: []string{"api", "payments"},
		SpotDiscount:      0.65,
	}

	result, err := ApplyScenario(s, snap)
	require.NoError(t, err)

	for _, pod := range result.Pods {
		if pod.Namespace == "api" || pod.Namespace == "payments" {
			assert.False(t, pod.IsSpot, "%s in excluded ns should NOT be spot", pod.Name)
		}
	}
}

func TestSpotMigrateUnknownController(t *testing.T) {
	// Pod with controller not in the snapshot's Controllers list
	snap := &collector.ClusterSnapshot{
		Controllers: []collector.ControllerSnapshot{},
		Pods: []collector.PodSnapshot{
			{Name: "orphan", Namespace: "default", OwnerRef: collector.OwnerReference{Kind: "Deployment", Name: "unknown"}},
		},
	}
	s := &SpotMigrateScenario{
		MinReplicas:     2,
		ControllerTypes: []string{"Deployment"},
		SpotDiscount:    0.65,
	}

	result, err := ApplyScenario(s, snap)
	require.NoError(t, err)

	// Can't verify replicas → not eligible
	assert.False(t, result.Pods[0].IsSpot)
}
