//go:build integration

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
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tochemey/kubewise/internal/kube"
)

func TestIntegrationCollectSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	clientset, err := kube.NewClientSet("", "kind-kubewise-test")
	require.NoError(t, err, "failed to create clientset — is the kind cluster running?")

	metricsClient, _ := kube.NewMetricsClientSet("", "kind-kubewise-test")

	snap, err := CollectSnapshot(ctx, clientset, metricsClient, CollectorOptions{})
	require.NoError(t, err)

	// Should have nodes (1 control-plane + 2 workers = 3)
	assert.GreaterOrEqual(t, len(snap.Nodes), 2, "should have at least 2 nodes")

	// Should have pods from our deployments
	assert.Greater(t, len(snap.Pods), 0, "should have pods")

	// Check namespace coverage
	namespaces := make(map[string]bool)
	for _, pod := range snap.Pods {
		namespaces[pod.Namespace] = true
	}
	assert.True(t, namespaces["default"], "should have pods in default namespace")
	assert.True(t, namespaces["api"], "should have pods in api namespace")
	assert.True(t, namespaces["data-pipeline"], "should have pods in data-pipeline namespace")
	assert.True(t, namespaces["monitoring"], "should have pods in monitoring namespace")

	// Should have controllers
	assert.Greater(t, len(snap.Controllers), 0, "should have controllers")

	// Should have the HPA
	assert.Greater(t, len(snap.HPAs), 0, "should have HPAs")

	// Should have the PDB
	assert.Greater(t, len(snap.PDBs), 0, "should have PDBs")

	// Check one of the nodes has the taint
	hasTaintedNode := false
	for _, node := range snap.Nodes {
		for _, taint := range node.Taints {
			if taint.Key == "dedicated" && taint.Value == "gpu" {
				hasTaintedNode = true
			}
		}
	}
	assert.True(t, hasTaintedNode, "should have a node with dedicated=gpu taint")
}

func TestIntegrationSnapshotJSONRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	clientset, err := kube.NewClientSet("", "kind-kubewise-test")
	require.NoError(t, err)

	snap, err := CollectSnapshot(ctx, clientset, nil, CollectorOptions{})
	require.NoError(t, err)

	// Serialize
	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Deserialize
	var restored ClusterSnapshot
	require.NoError(t, json.Unmarshal(data, &restored))

	// Verify round-trip fidelity
	assert.Equal(t, len(snap.Nodes), len(restored.Nodes))
	assert.Equal(t, len(snap.Pods), len(restored.Pods))
	assert.Equal(t, len(snap.Controllers), len(restored.Controllers))

	for i, node := range snap.Nodes {
		assert.Equal(t, node.Name, restored.Nodes[i].Name)
		assert.Equal(t, node.Allocatable.CPU, restored.Nodes[i].Allocatable.CPU)
	}
}

func TestIntegrationCollectSnapshotNamespaceFilter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	clientset, err := kube.NewClientSet("", "kind-kubewise-test")
	require.NoError(t, err)

	snap, err := CollectSnapshot(ctx, clientset, nil, CollectorOptions{Namespace: "api"})
	require.NoError(t, err)

	// Should still have all nodes
	assert.GreaterOrEqual(t, len(snap.Nodes), 2)

	// Only api namespace pods
	for _, pod := range snap.Pods {
		assert.Equal(t, "api", pod.Namespace)
	}
}
