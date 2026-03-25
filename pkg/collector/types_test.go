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
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileKey(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		pod       string
		container string
		expected  string
	}{
		{
			name:      "standard key",
			namespace: "default",
			pod:       "web-7b4f9",
			container: "nginx",
			expected:  "default/web-7b4f9/nginx",
		},
		{
			name:      "kube-system",
			namespace: "kube-system",
			pod:       "coredns-abc123",
			container: "coredns",
			expected:  "kube-system/coredns-abc123/coredns",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ProfileKey(tt.namespace, tt.pod, tt.container))
		})
	}
}

func TestClusterSnapshotJSONRoundTrip(t *testing.T) {
	snap := ClusterSnapshot{
		Timestamp: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		Nodes: []NodeSnapshot{
			{
				Name:         "node-1",
				InstanceType: "m6i.xlarge",
				Region:       "us-east-1",
				Zone:         "us-east-1a",
				Allocatable:  ResourcePair{CPU: 4000, Memory: 16 * 1024 * 1024 * 1024},
				Capacity:     ResourcePair{CPU: 4000, Memory: 16 * 1024 * 1024 * 1024},
				Labels:       map[string]string{"node.kubernetes.io/instance-type": "m6i.xlarge"},
				Taints:       []Taint{{Key: "dedicated", Value: "gpu", Effect: "NoSchedule"}},
				Conditions:   []NodeCondition{{Type: "Ready", Status: "True"}},
				NodePool:     "default-pool",
			},
		},
		Pods: []PodSnapshot{
			{
				Name:      "web-abc123",
				Namespace: "default",
				OwnerRef:  OwnerReference{Kind: "Deployment", Name: "web"},
				Containers: []ContainerSnapshot{
					{
						Name:     "nginx",
						Image:    "nginx:1.25",
						Requests: ResourcePair{CPU: 500, Memory: 256 * 1024 * 1024},
						Limits:   ResourcePair{CPU: 1000, Memory: 512 * 1024 * 1024},
					},
				},
				NodeName: "node-1",
				Labels:   map[string]string{"app": "web"},
				Affinity: &Affinity{
					NodeAffinity: &NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &NodeSelector{
							NodeSelectorTerms: []NodeSelectorTerm{
								{
									MatchExpressions: []NodeSelectorRequirement{
										{Key: "zone", Operator: "In", Values: []string{"us-east-1a"}},
									},
								},
							},
						},
					},
				},
				Tolerations: []Toleration{
					{Key: "dedicated", Operator: "Equal", Value: "gpu", Effect: "NoSchedule"},
				},
				TopologySpreadConstraints: []TopologySpreadConstraint{
					{MaxSkew: 1, TopologyKey: "topology.kubernetes.io/zone", WhenUnsatisfiable: "DoNotSchedule", LabelSelector: map[string]string{"app": "web"}},
				},
				PVCNames: []string{"data-vol"},
				Phase:    "Running",
			},
		},
		Controllers: []ControllerSnapshot{
			{Kind: "Deployment", Name: "web", Namespace: "default", Replicas: 3, DesiredReplicas: 3, UpdateStrategy: "RollingUpdate"},
		},
		HPAs: []HPASnapshot{
			{
				Name: "web-hpa", Namespace: "default",
				TargetRef:   OwnerReference{Kind: "Deployment", Name: "web"},
				MinReplicas: 2, MaxReplicas: 10, CurrentReplicas: 3,
				MetricTargets: []MetricTarget{{Type: "Resource", Name: "cpu", TargetValue: 80}},
			},
		},
		PDBs: []PDBSnapshot{
			{Name: "web-pdb", Namespace: "default", SelectorLabels: map[string]string{"app": "web"}, MinAvailable: 2},
		},
		PVCs: []PVCSnapshot{
			{Name: "data-vol", Namespace: "default", StorageClass: "gp3", CapacityBytes: 10 * 1024 * 1024 * 1024, AccessModes: []string{"ReadWriteOnce"}},
		},
		Pricing: PricingData{
			Provider: "aws",
			Region:   "us-east-1",
			InstancePricing: map[string]InstancePricing{
				"m6i.xlarge": {OnDemandHourly: 0.192, SpotHourly: 0.058},
			},
		},
		UsageProfile: map[string]ContainerUsageProfile{
			"default/web-abc123/nginx": {
				CurrentCPU: 120, CurrentMemory: 128 * 1024 * 1024,
				P50CPU: 100, P90CPU: 150, P95CPU: 180, P99CPU: 220,
				P50Memory: 100 * 1024 * 1024, P90Memory: 140 * 1024 * 1024,
				P95Memory: 160 * 1024 * 1024, P99Memory: 200 * 1024 * 1024,
				DataPoints: 2016,
			},
		},
		MetricsAvailable:    true,
		PrometheusAvailable: true,
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Unmarshal back
	var restored ClusterSnapshot
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	// Verify round-trip fidelity
	assert.Equal(t, snap.Timestamp, restored.Timestamp)
	assert.Equal(t, len(snap.Nodes), len(restored.Nodes))
	assert.Equal(t, snap.Nodes[0].Name, restored.Nodes[0].Name)
	assert.Equal(t, snap.Nodes[0].Allocatable.CPU, restored.Nodes[0].Allocatable.CPU)
	assert.Equal(t, snap.Nodes[0].Labels, restored.Nodes[0].Labels)
	assert.Equal(t, snap.Nodes[0].Taints, restored.Nodes[0].Taints)

	assert.Equal(t, len(snap.Pods), len(restored.Pods))
	assert.Equal(t, snap.Pods[0].Name, restored.Pods[0].Name)
	assert.Equal(t, snap.Pods[0].Containers[0].Requests, restored.Pods[0].Containers[0].Requests)
	assert.Equal(t, snap.Pods[0].Affinity, restored.Pods[0].Affinity)
	assert.Equal(t, snap.Pods[0].Tolerations, restored.Pods[0].Tolerations)
	assert.Equal(t, snap.Pods[0].TopologySpreadConstraints, restored.Pods[0].TopologySpreadConstraints)

	assert.Equal(t, snap.Controllers, restored.Controllers)
	assert.Equal(t, snap.HPAs, restored.HPAs)
	assert.Equal(t, snap.PDBs, restored.PDBs)
	assert.Equal(t, snap.PVCs, restored.PVCs)
	assert.Equal(t, snap.Pricing, restored.Pricing)
	assert.Equal(t, snap.UsageProfile, restored.UsageProfile)
	assert.Equal(t, snap.MetricsAvailable, restored.MetricsAvailable)
	assert.Equal(t, snap.PrometheusAvailable, restored.PrometheusAvailable)
}

func TestClusterSnapshotJSONOmitsNilAffinity(t *testing.T) {
	pod := PodSnapshot{
		Name:      "simple-pod",
		Namespace: "default",
	}

	data, err := json.Marshal(pod)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "nodeAffinity")
}

func TestResourcePairZeroValues(t *testing.T) {
	rp := ResourcePair{}
	assert.Equal(t, int64(0), rp.CPU)
	assert.Equal(t, int64(0), rp.Memory)

	data, err := json.Marshal(rp)
	require.NoError(t, err)

	var restored ResourcePair
	require.NoError(t, json.Unmarshal(data, &restored))
	assert.Equal(t, rp, restored)
}
