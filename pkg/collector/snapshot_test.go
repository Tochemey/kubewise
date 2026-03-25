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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

func int32Ptr(i int32) *int32 { return &i }

func newFakeClientset() *fake.Clientset {
	return fake.NewSimpleClientset(
		// Nodes
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "m6i.xlarge",
					"topology.kubernetes.io/region":    "us-east-1",
					"topology.kubernetes.io/zone":      "us-east-1a",
					"eks.amazonaws.com/nodegroup":      "default-pool",
				},
			},
			Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					{Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule},
				},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "m6i.xlarge",
					"topology.kubernetes.io/region":    "us-east-1",
					"topology.kubernetes.io/zone":      "us-east-1b",
					"eks.amazonaws.com/nodegroup":      "default-pool",
				},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		// Pods
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-abc123",
				Namespace: "default",
				Labels:    map[string]string{"app": "web"},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "web-abc"},
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:1.25",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					},
				},
				Affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{Key: "topology.kubernetes.io/zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-east-1a"}},
									},
								},
							},
						},
					},
				},
				Tolerations: []corev1.Toleration{
					{Key: "dedicated", Operator: corev1.TolerationOpEqual, Value: "gpu", Effect: corev1.TaintEffectNoSchedule},
				},
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					{
						MaxSkew:           1,
						TopologyKey:       "topology.kubernetes.io/zone",
						WhenUnsatisfiable: corev1.DoNotSchedule,
						LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "data",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "web-data",
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-def456",
				Namespace: "api",
				Labels:    map[string]string{"app": "api"},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "api-def"},
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "node-2",
				Containers: []corev1.Container{
					{
						Name:  "api",
						Image: "api:latest",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("250m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// Deployment
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(3),
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 3},
		},
		// StatefulSet
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"},
			Spec: appsv1.StatefulSetSpec{
				Replicas:       int32Ptr(1),
				UpdateStrategy: appsv1.StatefulSetUpdateStrategy{Type: appsv1.RollingUpdateStatefulSetStrategyType},
			},
			Status: appsv1.StatefulSetStatus{ReadyReplicas: 1},
		},
		// DaemonSet
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "monitor", Namespace: "kube-system"},
			Spec: appsv1.DaemonSetSpec{
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
			},
			Status: appsv1.DaemonSetStatus{NumberReady: 2, DesiredNumberScheduled: 2},
		},
		// HPA
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "web-hpa", Namespace: "default"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "web"},
				MinReplicas:    int32Ptr(2),
				MaxReplicas:    10,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name:   corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{AverageUtilization: int32Ptr(80)},
						},
					},
				},
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{CurrentReplicas: 3},
		},
		// PDB
		&policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: "web-pdb", Namespace: "default"},
			Spec: policyv1.PodDisruptionBudgetSpec{
				Selector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
			},
		},
		// PVC
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "web-data", Namespace: "default"},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: strPtr("gp3"),
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	)
}

func strPtr(s string) *string { return &s }

func TestCollectSnapshotAllNamespaces(t *testing.T) {
	clientset := newFakeClientset()
	ctx := context.Background()

	snap, err := CollectSnapshot(ctx, clientset, nil, CollectorOptions{})
	require.NoError(t, err)
	require.NotNil(t, snap)

	// Nodes
	assert.Equal(t, 2, len(snap.Nodes))
	node1 := findNode(snap, "node-1")
	require.NotNil(t, node1)
	assert.Equal(t, "m6i.xlarge", node1.InstanceType)
	assert.Equal(t, "us-east-1", node1.Region)
	assert.Equal(t, "us-east-1a", node1.Zone)
	assert.Equal(t, int64(4000), node1.Allocatable.CPU)
	assert.Equal(t, int64(16*1024*1024*1024), node1.Allocatable.Memory)
	assert.Equal(t, "default-pool", node1.NodePool)
	assert.Equal(t, 1, len(node1.Taints))
	assert.Equal(t, "dedicated", node1.Taints[0].Key)
	assert.Equal(t, "NoSchedule", node1.Taints[0].Effect)
	assert.Equal(t, 1, len(node1.Conditions))
	assert.Equal(t, "Ready", node1.Conditions[0].Type)

	// Pods
	assert.Equal(t, 2, len(snap.Pods))
	webPod := findPod(snap, "default", "web-abc123")
	require.NotNil(t, webPod)
	assert.Equal(t, "node-1", webPod.NodeName)
	assert.Equal(t, "ReplicaSet", webPod.OwnerRef.Kind)
	assert.Equal(t, "web-abc", webPod.OwnerRef.Name)
	assert.Equal(t, "Running", webPod.Phase)
	assert.Equal(t, 1, len(webPod.Containers))
	assert.Equal(t, "nginx", webPod.Containers[0].Name)
	assert.Equal(t, int64(500), webPod.Containers[0].Requests.CPU)
	assert.Equal(t, int64(256*1024*1024), webPod.Containers[0].Requests.Memory)
	assert.Equal(t, int64(1000), webPod.Containers[0].Limits.CPU)
	assert.Equal(t, int64(512*1024*1024), webPod.Containers[0].Limits.Memory)

	// Affinity
	require.NotNil(t, webPod.Affinity)
	require.NotNil(t, webPod.Affinity.NodeAffinity)
	require.NotNil(t, webPod.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution)
	terms := webPod.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	assert.Equal(t, 1, len(terms))
	assert.Equal(t, "topology.kubernetes.io/zone", terms[0].MatchExpressions[0].Key)
	assert.Equal(t, "In", terms[0].MatchExpressions[0].Operator)
	assert.Equal(t, []string{"us-east-1a"}, terms[0].MatchExpressions[0].Values)

	// Tolerations
	assert.Equal(t, 1, len(webPod.Tolerations))
	assert.Equal(t, "dedicated", webPod.Tolerations[0].Key)

	// Topology spread
	assert.Equal(t, 1, len(webPod.TopologySpreadConstraints))
	assert.Equal(t, int32(1), webPod.TopologySpreadConstraints[0].MaxSkew)
	assert.Equal(t, "topology.kubernetes.io/zone", webPod.TopologySpreadConstraints[0].TopologyKey)

	// PVC mounts
	assert.Equal(t, []string{"web-data"}, webPod.PVCNames)

	// Controllers (1 deployment + 1 statefulset + 1 daemonset = 3, no standalone replicasets)
	assert.Equal(t, 3, len(snap.Controllers))
	deploy := findController(snap, "Deployment", "default", "web")
	require.NotNil(t, deploy)
	assert.Equal(t, int32(3), deploy.Replicas)
	assert.Equal(t, int32(3), deploy.DesiredReplicas)
	assert.Equal(t, "RollingUpdate", deploy.UpdateStrategy)

	ds := findController(snap, "DaemonSet", "kube-system", "monitor")
	require.NotNil(t, ds)
	assert.Equal(t, int32(2), ds.Replicas)
	assert.Equal(t, int32(2), ds.DesiredReplicas)

	// HPAs
	assert.Equal(t, 1, len(snap.HPAs))
	assert.Equal(t, "web-hpa", snap.HPAs[0].Name)
	assert.Equal(t, int32(2), snap.HPAs[0].MinReplicas)
	assert.Equal(t, int32(10), snap.HPAs[0].MaxReplicas)
	assert.Equal(t, int32(3), snap.HPAs[0].CurrentReplicas)
	assert.Equal(t, 1, len(snap.HPAs[0].MetricTargets))
	assert.Equal(t, "cpu", snap.HPAs[0].MetricTargets[0].Name)
	assert.Equal(t, int64(80), snap.HPAs[0].MetricTargets[0].TargetValue)

	// PDBs
	assert.Equal(t, 1, len(snap.PDBs))
	assert.Equal(t, "web-pdb", snap.PDBs[0].Name)
	assert.Equal(t, int32(2), snap.PDBs[0].MinAvailable)
	assert.Equal(t, map[string]string{"app": "web"}, snap.PDBs[0].SelectorLabels)

	// PVCs
	assert.Equal(t, 1, len(snap.PVCs))
	assert.Equal(t, "web-data", snap.PVCs[0].Name)
	assert.Equal(t, "gp3", snap.PVCs[0].StorageClass)
	assert.Equal(t, int64(10*1024*1024*1024), snap.PVCs[0].CapacityBytes)
	assert.Equal(t, []string{"ReadWriteOnce"}, snap.PVCs[0].AccessModes)

	// Metrics not available (no metrics client)
	assert.False(t, snap.MetricsAvailable)
}

func TestCollectSnapshotNamespaceFiltering(t *testing.T) {
	clientset := newFakeClientset()
	ctx := context.Background()

	snap, err := CollectSnapshot(ctx, clientset, nil, CollectorOptions{Namespace: "default"})
	require.NoError(t, err)

	// Should still collect all nodes (nodes are not namespaced)
	assert.Equal(t, 2, len(snap.Nodes))

	// Only default namespace pods
	assert.Equal(t, 1, len(snap.Pods))
	assert.Equal(t, "default", snap.Pods[0].Namespace)

	// Only default namespace controllers (deployment, statefulset, replicasets in default)
	for _, c := range snap.Controllers {
		assert.Equal(t, "default", c.Namespace)
	}
}

func TestCollectSnapshotSaveToFile(t *testing.T) {
	clientset := newFakeClientset()
	ctx := context.Background()

	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "snapshot.json")

	snap, err := CollectSnapshot(ctx, clientset, nil, CollectorOptions{SavePath: savePath})
	require.NoError(t, err)

	// File should exist
	data, err := os.ReadFile(savePath)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Should unmarshal back to a valid snapshot
	var restored ClusterSnapshot
	require.NoError(t, json.Unmarshal(data, &restored))
	assert.Equal(t, len(snap.Nodes), len(restored.Nodes))
	assert.Equal(t, len(snap.Pods), len(restored.Pods))
	assert.Equal(t, snap.Nodes[0].Name, restored.Nodes[0].Name)
}

func TestCollectSnapshotNoMetricsClient(t *testing.T) {
	clientset := newFakeClientset()
	ctx := context.Background()

	snap, err := CollectSnapshot(ctx, clientset, nil, CollectorOptions{})
	require.NoError(t, err)
	assert.False(t, snap.MetricsAvailable)
	assert.Empty(t, snap.UsageProfile)
}

func TestConvertNodeDetectsNodePool(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{"EKS", map[string]string{"eks.amazonaws.com/nodegroup": "eks-pool"}, "eks-pool"},
		{"GKE", map[string]string{"cloud.google.com/gke-nodepool": "gke-pool"}, "gke-pool"},
		{"AKS", map[string]string{"agentpool": "aks-pool"}, "aks-pool"},
		{"unknown", map[string]string{"other": "label"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, detectNodePool(tt.labels))
		})
	}
}

func TestConvertPodWithoutOwnerRef(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "standalone", Namespace: "default"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "app:v1"}}},
	}
	ps := convertPod(pod)
	assert.Equal(t, "", ps.OwnerRef.Kind)
	assert.Equal(t, "", ps.OwnerRef.Name)
}

func TestConvertPodWithoutAffinity(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "simple", Namespace: "default"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "app:v1"}}},
	}
	ps := convertPod(pod)
	assert.Nil(t, ps.Affinity)
}

// helpers

func findNode(snap *ClusterSnapshot, name string) *NodeSnapshot {
	for i := range snap.Nodes {
		if snap.Nodes[i].Name == name {
			return &snap.Nodes[i]
		}
	}
	return nil
}

func findPod(snap *ClusterSnapshot, ns, name string) *PodSnapshot {
	for i := range snap.Pods {
		if snap.Pods[i].Namespace == ns && snap.Pods[i].Name == name {
			return &snap.Pods[i]
		}
	}
	return nil
}

func findController(snap *ClusterSnapshot, kind, ns, name string) *ControllerSnapshot {
	for i := range snap.Controllers {
		if snap.Controllers[i].Kind == kind && snap.Controllers[i].Namespace == ns && snap.Controllers[i].Name == name {
			return &snap.Controllers[i]
		}
	}
	return nil
}
