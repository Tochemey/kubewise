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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSnapshot() *ClusterSnapshot {
	return &ClusterSnapshot{
		Timestamp: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		Nodes: []NodeSnapshot{
			{
				Name:         "node-1",
				InstanceType: "m6i.xlarge",
				Region:       "us-east-1",
				Zone:         "us-east-1a",
				Allocatable:  ResourcePair{CPU: 4000, Memory: 16 * 1024 * 1024 * 1024},
				Capacity:     ResourcePair{CPU: 4000, Memory: 16 * 1024 * 1024 * 1024},
				Labels:       map[string]string{"zone": "us-east-1a", "pool": "default"},
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
				Labels:   map[string]string{"app": "web", "env": "prod"},
				Affinity: &Affinity{
					NodeAffinity: &NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &NodeSelector{
							NodeSelectorTerms: []NodeSelectorTerm{
								{
									MatchExpressions: []NodeSelectorRequirement{
										{Key: "zone", Operator: "In", Values: []string{"us-east-1a", "us-east-1b"}},
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
}

func TestClusterSnapshotDeepCopy(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	require.NotNil(t, copied)
	assert.Equal(t, original.Timestamp, copied.Timestamp)
	assert.Equal(t, len(original.Nodes), len(copied.Nodes))
	assert.Equal(t, len(original.Pods), len(copied.Pods))
	assert.Equal(t, len(original.Controllers), len(copied.Controllers))
	assert.Equal(t, len(original.HPAs), len(copied.HPAs))
	assert.Equal(t, len(original.PDBs), len(copied.PDBs))
	assert.Equal(t, len(original.PVCs), len(copied.PVCs))
	assert.Equal(t, original.MetricsAvailable, copied.MetricsAvailable)
	assert.Equal(t, original.PrometheusAvailable, copied.PrometheusAvailable)
}

func TestClusterSnapshotDeepCopyNil(t *testing.T) {
	var snap *ClusterSnapshot
	assert.Nil(t, snap.DeepCopy())
}

func TestNodeSnapshotLabelsIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	// Mutate the copy's node labels
	copied.Nodes[0].Labels["new-label"] = "new-value"
	delete(copied.Nodes[0].Labels, "zone")

	// Original should be unchanged
	assert.Equal(t, "us-east-1a", original.Nodes[0].Labels["zone"])
	_, exists := original.Nodes[0].Labels["new-label"]
	assert.False(t, exists)
}

func TestNodeSnapshotTaintsIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	// Mutate the copy's taints
	copied.Nodes[0].Taints[0].Key = "changed"
	copied.Nodes[0].Taints = append(copied.Nodes[0].Taints, Taint{Key: "extra", Value: "v", Effect: "NoSchedule"})

	// Original should be unchanged
	assert.Equal(t, "dedicated", original.Nodes[0].Taints[0].Key)
	assert.Equal(t, 1, len(original.Nodes[0].Taints))
}

func TestPodSnapshotContainersIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	// Mutate the copy's container requests
	copied.Pods[0].Containers[0].Requests.CPU = 9999
	copied.Pods[0].Containers[0].Limits.Memory = 1

	// Append a new container to the copy
	copied.Pods[0].Containers = append(copied.Pods[0].Containers, ContainerSnapshot{
		Name: "sidecar", Requests: ResourcePair{CPU: 100, Memory: 64},
	})

	// Original should be unchanged
	assert.Equal(t, int64(500), original.Pods[0].Containers[0].Requests.CPU)
	assert.Equal(t, int64(512*1024*1024), original.Pods[0].Containers[0].Limits.Memory)
	assert.Equal(t, 1, len(original.Pods[0].Containers))
}

func TestPodSnapshotLabelsIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	copied.Pods[0].Labels["new"] = "label"
	delete(copied.Pods[0].Labels, "app")

	assert.Equal(t, "web", original.Pods[0].Labels["app"])
	_, exists := original.Pods[0].Labels["new"]
	assert.False(t, exists)
}

func TestPodSnapshotAffinityIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	// Mutate the copy's affinity values
	terms := copied.Pods[0].Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	terms[0].MatchExpressions[0].Key = "changed"
	terms[0].MatchExpressions[0].Values = append(terms[0].MatchExpressions[0].Values, "us-east-1c")

	// Original should be unchanged
	origTerms := original.Pods[0].Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	assert.Equal(t, "zone", origTerms[0].MatchExpressions[0].Key)
	assert.Equal(t, 2, len(origTerms[0].MatchExpressions[0].Values))
}

func TestPodSnapshotAffinityNilHandling(t *testing.T) {
	pod := PodSnapshot{Name: "no-affinity", Namespace: "default"}
	copied := pod.DeepCopy()
	assert.Nil(t, copied.Affinity)
}

func TestPodSnapshotTolerationsIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	copied.Pods[0].Tolerations[0].Key = "modified"
	copied.Pods[0].Tolerations = append(copied.Pods[0].Tolerations, Toleration{Key: "extra"})

	assert.Equal(t, "dedicated", original.Pods[0].Tolerations[0].Key)
	assert.Equal(t, 1, len(original.Pods[0].Tolerations))
}

func TestPodSnapshotTopologySpreadIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	// Mutate the copy's topology spread label selector map
	copied.Pods[0].TopologySpreadConstraints[0].LabelSelector["new"] = "value"
	copied.Pods[0].TopologySpreadConstraints[0].MaxSkew = 99

	assert.Equal(t, int32(1), original.Pods[0].TopologySpreadConstraints[0].MaxSkew)
	_, exists := original.Pods[0].TopologySpreadConstraints[0].LabelSelector["new"]
	assert.False(t, exists)
}

func TestPodSnapshotPVCNamesIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	copied.Pods[0].PVCNames = append(copied.Pods[0].PVCNames, "extra-vol")
	copied.Pods[0].PVCNames[0] = "changed"

	assert.Equal(t, "data-vol", original.Pods[0].PVCNames[0])
	assert.Equal(t, 1, len(original.Pods[0].PVCNames))
}

func TestUsageProfileMapIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	key := "default/web-abc123/nginx"

	// Mutate the copy's usage profile
	profile := copied.UsageProfile[key]
	profile.CurrentCPU = 9999
	copied.UsageProfile[key] = profile
	copied.UsageProfile["new/key/here"] = ContainerUsageProfile{CurrentCPU: 1}

	// Original should be unchanged
	assert.Equal(t, int64(120), original.UsageProfile[key].CurrentCPU)
	_, exists := original.UsageProfile["new/key/here"]
	assert.False(t, exists)
}

func TestPricingDataIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	copied.Pricing.InstancePricing["m6i.xlarge"] = InstancePricing{OnDemandHourly: 999.0}
	copied.Pricing.InstancePricing["new-type"] = InstancePricing{OnDemandHourly: 1.0}

	assert.InDelta(t, 0.192, original.Pricing.InstancePricing["m6i.xlarge"].OnDemandHourly, 1e-9)
	_, exists := original.Pricing.InstancePricing["new-type"]
	assert.False(t, exists)
}

func TestHPASnapshotMetricTargetsIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	copied.HPAs[0].MetricTargets[0].TargetValue = 9999
	copied.HPAs[0].MetricTargets = append(copied.HPAs[0].MetricTargets, MetricTarget{Name: "extra"})

	assert.Equal(t, int64(80), original.HPAs[0].MetricTargets[0].TargetValue)
	assert.Equal(t, 1, len(original.HPAs[0].MetricTargets))
}

func TestPDBSnapshotSelectorLabelsIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	copied.PDBs[0].SelectorLabels["new"] = "label"
	delete(copied.PDBs[0].SelectorLabels, "app")

	assert.Equal(t, "web", original.PDBs[0].SelectorLabels["app"])
	_, exists := original.PDBs[0].SelectorLabels["new"]
	assert.False(t, exists)
}

func TestPVCSnapshotAccessModesIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	copied.PVCs[0].AccessModes = append(copied.PVCs[0].AccessModes, "ReadWriteMany")
	copied.PVCs[0].AccessModes[0] = "Changed"

	assert.Equal(t, "ReadWriteOnce", original.PVCs[0].AccessModes[0])
	assert.Equal(t, 1, len(original.PVCs[0].AccessModes))
}

func TestNilSlicesAndMapsPreserved(t *testing.T) {
	snap := &ClusterSnapshot{
		Timestamp: time.Now(),
		// All slices and maps left nil
	}

	copied := snap.DeepCopy()
	assert.Nil(t, copied.Nodes)
	assert.Nil(t, copied.Pods)
	assert.Nil(t, copied.Controllers)
	assert.Nil(t, copied.HPAs)
	assert.Nil(t, copied.PDBs)
	assert.Nil(t, copied.PVCs)
	assert.Nil(t, copied.UsageProfile)
	assert.Nil(t, copied.Pricing.InstancePricing)
}

func TestNodeSnapshotNilSlicesPreserved(t *testing.T) {
	node := NodeSnapshot{Name: "bare-node"}
	copied := node.DeepCopy()
	assert.Nil(t, copied.Labels)
	assert.Nil(t, copied.Taints)
	assert.Nil(t, copied.Conditions)
}

func TestPodSnapshotNilSlicesPreserved(t *testing.T) {
	pod := PodSnapshot{Name: "bare-pod", Namespace: "default"}
	copied := pod.DeepCopy()
	assert.Nil(t, copied.Containers)
	assert.Nil(t, copied.Labels)
	assert.Nil(t, copied.Affinity)
	assert.Nil(t, copied.Tolerations)
	assert.Nil(t, copied.TopologySpreadConstraints)
	assert.Nil(t, copied.PVCNames)
}

func TestNodesSliceIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	// Append a node to the copy
	copied.Nodes = append(copied.Nodes, NodeSnapshot{Name: "node-2"})

	assert.Equal(t, 1, len(original.Nodes))
}

func TestPodsSliceIndependence(t *testing.T) {
	original := newTestSnapshot()
	copied := original.DeepCopy()

	// Append a pod to the copy
	copied.Pods = append(copied.Pods, PodSnapshot{Name: "extra-pod"})

	assert.Equal(t, 1, len(original.Pods))
}
