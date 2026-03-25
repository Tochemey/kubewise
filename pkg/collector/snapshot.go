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
	"fmt"
	"maps"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// CollectorOptions configures snapshot collection.
type CollectorOptions struct {
	// Namespace limits collection to a specific namespace. Empty means all.
	Namespace string
	// SavePath is the file path to save the snapshot as JSON. Empty means don't save.
	SavePath string
}

// CollectSnapshot collects a full cluster snapshot from the Kubernetes API.
func CollectSnapshot(ctx context.Context, clientset kubernetes.Interface, metricsClient metricsv.Interface, opts CollectorOptions) (*ClusterSnapshot, error) {
	snap := &ClusterSnapshot{
		Timestamp:    time.Now().UTC(),
		UsageProfile: make(map[string]ContainerUsageProfile),
	}

	klog.V(1).InfoS("Collecting cluster snapshot")

	if err := collectNodes(ctx, clientset, snap); err != nil {
		return nil, fmt.Errorf("collecting nodes: %w", err)
	}

	if err := collectPods(ctx, clientset, snap, opts.Namespace); err != nil {
		return nil, fmt.Errorf("collecting pods: %w", err)
	}

	if err := collectControllers(ctx, clientset, snap, opts.Namespace); err != nil {
		return nil, fmt.Errorf("collecting controllers: %w", err)
	}

	if err := collectHPAs(ctx, clientset, snap, opts.Namespace); err != nil {
		return nil, fmt.Errorf("collecting HPAs: %w", err)
	}

	if err := collectPDBs(ctx, clientset, snap, opts.Namespace); err != nil {
		return nil, fmt.Errorf("collecting PDBs: %w", err)
	}

	if err := collectPVCs(ctx, clientset, snap, opts.Namespace); err != nil {
		return nil, fmt.Errorf("collecting PVCs: %w", err)
	}

	collectMetrics(ctx, metricsClient, snap, opts.Namespace)

	klog.InfoS("Snapshot collected",
		"nodes", len(snap.Nodes),
		"pods", len(snap.Pods),
		"controllers", len(snap.Controllers),
		"metricsAvailable", snap.MetricsAvailable,
	)

	if opts.SavePath != "" {
		if err := saveSnapshot(snap, opts.SavePath); err != nil {
			return nil, fmt.Errorf("saving snapshot: %w", err)
		}
		klog.V(1).InfoS("Snapshot saved", "path", opts.SavePath)
	}

	return snap, nil
}

func collectNodes(ctx context.Context, clientset kubernetes.Interface, snap *ClusterSnapshot) error {
	klog.V(1).InfoS("Collecting nodes")
	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}

	snap.Nodes = make([]NodeSnapshot, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		snap.Nodes = append(snap.Nodes, convertNode(node))
	}
	return nil
}

func collectPods(ctx context.Context, clientset kubernetes.Interface, snap *ClusterSnapshot, namespace string) error {
	klog.V(1).InfoS("Collecting pods", "namespace", namespaceOrAll(namespace))
	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing pods: %w", err)
	}

	snap.Pods = make([]PodSnapshot, 0, len(podList.Items))
	for _, pod := range podList.Items {
		snap.Pods = append(snap.Pods, convertPod(pod))
	}
	return nil
}

func collectControllers(ctx context.Context, clientset kubernetes.Interface, snap *ClusterSnapshot, namespace string) error {
	klog.V(1).InfoS("Collecting controllers", "namespace", namespaceOrAll(namespace))

	deployments, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing deployments: %w", err)
	}
	for _, d := range deployments.Items {
		snap.Controllers = append(snap.Controllers, convertDeployment(d))
	}

	statefulsets, err := clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing statefulsets: %w", err)
	}
	for _, s := range statefulsets.Items {
		snap.Controllers = append(snap.Controllers, convertStatefulSet(s))
	}

	daemonsets, err := clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing daemonsets: %w", err)
	}
	for _, ds := range daemonsets.Items {
		snap.Controllers = append(snap.Controllers, convertDaemonSet(ds))
	}

	replicasets, err := clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing replicasets: %w", err)
	}
	for _, rs := range replicasets.Items {
		snap.Controllers = append(snap.Controllers, convertReplicaSet(rs))
	}

	return nil
}

func collectHPAs(ctx context.Context, clientset kubernetes.Interface, snap *ClusterSnapshot, namespace string) error {
	klog.V(1).InfoS("Collecting HPAs", "namespace", namespaceOrAll(namespace))
	hpaList, err := clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing HPAs: %w", err)
	}

	for _, hpa := range hpaList.Items {
		snap.HPAs = append(snap.HPAs, convertHPA(hpa))
	}
	return nil
}

func collectPDBs(ctx context.Context, clientset kubernetes.Interface, snap *ClusterSnapshot, namespace string) error {
	klog.V(1).InfoS("Collecting PDBs", "namespace", namespaceOrAll(namespace))
	pdbList, err := clientset.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing PDBs: %w", err)
	}

	for _, pdb := range pdbList.Items {
		snap.PDBs = append(snap.PDBs, convertPDB(pdb))
	}
	return nil
}

func collectPVCs(ctx context.Context, clientset kubernetes.Interface, snap *ClusterSnapshot, namespace string) error {
	klog.V(1).InfoS("Collecting PVCs", "namespace", namespaceOrAll(namespace))
	pvcList, err := clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing PVCs: %w", err)
	}

	for _, pvc := range pvcList.Items {
		snap.PVCs = append(snap.PVCs, convertPVC(pvc))
	}
	return nil
}

func collectMetrics(ctx context.Context, metricsClient metricsv.Interface, snap *ClusterSnapshot, namespace string) {
	if metricsClient == nil {
		klog.V(1).InfoS("Metrics client not provided, skipping metrics collection")
		return
	}

	klog.V(1).InfoS("Collecting metrics from metrics-server", "namespace", namespaceOrAll(namespace))
	podMetricsList, err := metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to collect metrics from metrics-server, continuing without metrics")
		return
	}

	snap.MetricsAvailable = true
	for _, pm := range podMetricsList.Items {
		for _, cm := range pm.Containers {
			key := ProfileKey(pm.Namespace, pm.Name, cm.Name)
			profile := snap.UsageProfile[key]
			profile.CurrentCPU = cm.Usage.Cpu().MilliValue()
			profile.CurrentMemory = cm.Usage.Memory().Value()
			profile.DataPoints = 1
			snap.UsageProfile[key] = profile
		}
	}

	klog.V(1).InfoS("Metrics collected", "containers", len(snap.UsageProfile))
}

// convertNode maps a corev1.Node to a NodeSnapshot.
func convertNode(node corev1.Node) NodeSnapshot {
	ns := NodeSnapshot{
		Name:         node.Name,
		InstanceType: node.Labels["node.kubernetes.io/instance-type"],
		Region:       node.Labels["topology.kubernetes.io/region"],
		Zone:         node.Labels["topology.kubernetes.io/zone"],
		Allocatable: ResourcePair{
			CPU:    node.Status.Allocatable.Cpu().MilliValue(),
			Memory: node.Status.Allocatable.Memory().Value(),
		},
		Capacity: ResourcePair{
			CPU:    node.Status.Capacity.Cpu().MilliValue(),
			Memory: node.Status.Capacity.Memory().Value(),
		},
		Labels:   copyStringMap(node.Labels),
		NodePool: detectNodePool(node.Labels),
	}

	for _, taint := range node.Spec.Taints {
		ns.Taints = append(ns.Taints, Taint{
			Key:    taint.Key,
			Value:  taint.Value,
			Effect: string(taint.Effect),
		})
	}

	for _, cond := range node.Status.Conditions {
		ns.Conditions = append(ns.Conditions, NodeCondition{
			Type:   string(cond.Type),
			Status: string(cond.Status),
		})
	}

	return ns
}

// convertPod maps a corev1.Pod to a PodSnapshot.
func convertPod(pod corev1.Pod) PodSnapshot {
	ps := PodSnapshot{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		NodeName:  pod.Spec.NodeName,
		Labels:    copyStringMap(pod.Labels),
		Phase:     string(pod.Status.Phase),
	}

	// Owner reference (first one)
	if len(pod.OwnerReferences) > 0 {
		ps.OwnerRef = OwnerReference{
			Kind: pod.OwnerReferences[0].Kind,
			Name: pod.OwnerReferences[0].Name,
		}
	}

	// Containers
	for _, c := range pod.Spec.Containers {
		ps.Containers = append(ps.Containers, convertContainer(c))
	}

	// Affinity
	if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil {
		ps.Affinity = convertAffinity(pod.Spec.Affinity)
	}

	// Tolerations
	for _, t := range pod.Spec.Tolerations {
		ps.Tolerations = append(ps.Tolerations, Toleration{
			Key:      t.Key,
			Operator: string(t.Operator),
			Value:    t.Value,
			Effect:   string(t.Effect),
		})
	}

	// Topology spread constraints
	for _, tsc := range pod.Spec.TopologySpreadConstraints {
		constraint := TopologySpreadConstraint{
			MaxSkew:           tsc.MaxSkew,
			TopologyKey:       tsc.TopologyKey,
			WhenUnsatisfiable: string(tsc.WhenUnsatisfiable),
		}
		if tsc.LabelSelector != nil {
			constraint.LabelSelector = copyStringMap(tsc.LabelSelector.MatchLabels)
		}
		ps.TopologySpreadConstraints = append(ps.TopologySpreadConstraints, constraint)
	}

	// PVC names
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			ps.PVCNames = append(ps.PVCNames, vol.PersistentVolumeClaim.ClaimName)
		}
	}

	return ps
}

func convertContainer(c corev1.Container) ContainerSnapshot {
	return ContainerSnapshot{
		Name:  c.Name,
		Image: c.Image,
		Requests: ResourcePair{
			CPU:    c.Resources.Requests.Cpu().MilliValue(),
			Memory: c.Resources.Requests.Memory().Value(),
		},
		Limits: ResourcePair{
			CPU:    c.Resources.Limits.Cpu().MilliValue(),
			Memory: c.Resources.Limits.Memory().Value(),
		},
	}
}

func convertAffinity(aff *corev1.Affinity) *Affinity {
	if aff == nil || aff.NodeAffinity == nil {
		return nil
	}

	result := &Affinity{
		NodeAffinity: &NodeAffinity{},
	}

	req := aff.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if req != nil {
		ns := &NodeSelector{}
		for _, term := range req.NodeSelectorTerms {
			nst := NodeSelectorTerm{}
			for _, expr := range term.MatchExpressions {
				nst.MatchExpressions = append(nst.MatchExpressions, NodeSelectorRequirement{
					Key:      expr.Key,
					Operator: string(expr.Operator),
					Values:   append([]string(nil), expr.Values...),
				})
			}
			ns.NodeSelectorTerms = append(ns.NodeSelectorTerms, nst)
		}
		result.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = ns
	}

	return result
}

func convertDeployment(d appsv1.Deployment) ControllerSnapshot {
	cs := ControllerSnapshot{
		Kind:            "Deployment",
		Name:            d.Name,
		Namespace:       d.Namespace,
		Replicas:        d.Status.ReadyReplicas,
		DesiredReplicas: 1,
		UpdateStrategy:  string(d.Spec.Strategy.Type),
	}
	if d.Spec.Replicas != nil {
		cs.DesiredReplicas = *d.Spec.Replicas
	}
	return cs
}

func convertStatefulSet(s appsv1.StatefulSet) ControllerSnapshot {
	cs := ControllerSnapshot{
		Kind:            "StatefulSet",
		Name:            s.Name,
		Namespace:       s.Namespace,
		Replicas:        s.Status.ReadyReplicas,
		DesiredReplicas: 1,
		UpdateStrategy:  string(s.Spec.UpdateStrategy.Type),
	}
	if s.Spec.Replicas != nil {
		cs.DesiredReplicas = *s.Spec.Replicas
	}
	return cs
}

func convertDaemonSet(ds appsv1.DaemonSet) ControllerSnapshot {
	return ControllerSnapshot{
		Kind:            "DaemonSet",
		Name:            ds.Name,
		Namespace:       ds.Namespace,
		Replicas:        ds.Status.NumberReady,
		DesiredReplicas: ds.Status.DesiredNumberScheduled,
		UpdateStrategy:  string(ds.Spec.UpdateStrategy.Type),
	}
}

func convertReplicaSet(rs appsv1.ReplicaSet) ControllerSnapshot {
	cs := ControllerSnapshot{
		Kind:            "ReplicaSet",
		Name:            rs.Name,
		Namespace:       rs.Namespace,
		Replicas:        rs.Status.ReadyReplicas,
		DesiredReplicas: 1,
	}
	if rs.Spec.Replicas != nil {
		cs.DesiredReplicas = *rs.Spec.Replicas
	}
	return cs
}

func convertHPA(hpa autoscalingv2.HorizontalPodAutoscaler) HPASnapshot {
	hs := HPASnapshot{
		Name:            hpa.Name,
		Namespace:       hpa.Namespace,
		TargetRef:       OwnerReference{Kind: hpa.Spec.ScaleTargetRef.Kind, Name: hpa.Spec.ScaleTargetRef.Name},
		MinReplicas:     1,
		MaxReplicas:     hpa.Spec.MaxReplicas,
		CurrentReplicas: hpa.Status.CurrentReplicas,
	}
	if hpa.Spec.MinReplicas != nil {
		hs.MinReplicas = *hpa.Spec.MinReplicas
	}

	for _, metric := range hpa.Spec.Metrics {
		mt := MetricTarget{
			Type: string(metric.Type),
		}
		switch metric.Type {
		case autoscalingv2.ResourceMetricSourceType:
			if metric.Resource != nil {
				mt.Name = string(metric.Resource.Name)
				if metric.Resource.Target.AverageUtilization != nil {
					mt.TargetValue = int64(*metric.Resource.Target.AverageUtilization)
				}
			}
		case autoscalingv2.PodsMetricSourceType:
			if metric.Pods != nil {
				mt.Name = metric.Pods.Metric.Name
			}
		case autoscalingv2.ObjectMetricSourceType:
			if metric.Object != nil {
				mt.Name = metric.Object.Metric.Name
			}
		}
		hs.MetricTargets = append(hs.MetricTargets, mt)
	}

	return hs
}

func convertPDB(pdb policyv1.PodDisruptionBudget) PDBSnapshot {
	ps := PDBSnapshot{
		Name:      pdb.Name,
		Namespace: pdb.Namespace,
	}
	if pdb.Spec.Selector != nil {
		ps.SelectorLabels = copyStringMap(pdb.Spec.Selector.MatchLabels)
	}
	if pdb.Spec.MinAvailable != nil {
		ps.MinAvailable = pdb.Spec.MinAvailable.IntVal
	}
	if pdb.Spec.MaxUnavailable != nil {
		ps.MaxUnavailable = pdb.Spec.MaxUnavailable.IntVal
	}
	return ps
}

func convertPVC(pvc corev1.PersistentVolumeClaim) PVCSnapshot {
	ps := PVCSnapshot{
		Name:      pvc.Name,
		Namespace: pvc.Namespace,
	}
	if pvc.Spec.StorageClassName != nil {
		ps.StorageClass = *pvc.Spec.StorageClassName
	}
	if storage, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
		ps.CapacityBytes = storage.Value()
	}
	for _, mode := range pvc.Spec.AccessModes {
		ps.AccessModes = append(ps.AccessModes, string(mode))
	}
	return ps
}

func saveSnapshot(snap *ClusterSnapshot, path string) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling snapshot: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing snapshot file: %w", err)
	}
	return nil
}

// detectNodePool extracts the node pool name from node labels.
func detectNodePool(labels map[string]string) string {
	// GKE
	if pool, ok := labels["cloud.google.com/gke-nodepool"]; ok {
		return pool
	}
	// EKS
	if pool, ok := labels["eks.amazonaws.com/nodegroup"]; ok {
		return pool
	}
	// AKS
	if pool, ok := labels["agentpool"]; ok {
		return pool
	}
	return ""
}

func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	maps.Copy(out, m)
	return out
}

func namespaceOrAll(ns string) string {
	if ns == "" {
		return "all"
	}
	return ns
}
