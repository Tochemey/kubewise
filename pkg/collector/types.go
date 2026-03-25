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

import "time"

// ClusterSnapshot is the top-level struct representing the full schedulable
// state of a Kubernetes cluster at a point in time.
// Fields ordered largest-to-smallest alignment to minimize padding.
type ClusterSnapshot struct {
	// Timestamp is the time the snapshot was taken.
	Timestamp time.Time `json:"timestamp"`
	// Nodes contains the state of all cluster nodes.
	Nodes []NodeSnapshot `json:"nodes"`
	// Pods contains the state of all pods in scope.
	Pods []PodSnapshot `json:"pods"`
	// Controllers contains deployments, statefulsets, daemonsets, and replicasets.
	Controllers []ControllerSnapshot `json:"controllers"`
	// HPAs contains horizontal pod autoscaler state.
	HPAs []HPASnapshot `json:"hpas"`
	// PDBs contains pod disruption budget state.
	PDBs []PDBSnapshot `json:"pdbs"`
	// PVCs contains persistent volume claim state.
	PVCs []PVCSnapshot `json:"pvcs"`
	// UsageProfile maps "namespace/pod/container" to its usage profile.
	UsageProfile map[string]ContainerUsageProfile `json:"usageProfile"`
	// Pricing holds the cost data for instance types in this cluster.
	Pricing PricingData `json:"pricing"`
	// MetricsAvailable indicates whether metrics-server data was collected.
	MetricsAvailable bool `json:"metricsAvailable"`
	// PrometheusAvailable indicates whether Prometheus historical data was collected.
	PrometheusAvailable bool `json:"prometheusAvailable"`
}

// NodeSnapshot represents a single cluster node.
type NodeSnapshot struct {
	// Name is the node name.
	Name string `json:"name"`
	// InstanceType is the cloud instance type (e.g., "m6i.xlarge").
	InstanceType string `json:"instanceType"`
	// Region is the cloud region (e.g., "us-east-1").
	Region string `json:"region"`
	// Zone is the availability zone (e.g., "us-east-1a").
	Zone string `json:"zone"`
	// Allocatable is the allocatable CPU (millicores) and memory (bytes).
	Allocatable ResourcePair `json:"allocatable"`
	// Capacity is the total node capacity (before system reservations).
	Capacity ResourcePair `json:"capacity"`
	// Labels are the node labels.
	Labels map[string]string `json:"labels"`
	// Taints are the node taints.
	Taints []Taint `json:"taints"`
	// Conditions are the node conditions (e.g., Ready, MemoryPressure).
	Conditions []NodeCondition `json:"conditions"`
	// NodePool is the node pool or node group name, if detectable.
	NodePool string `json:"nodePool"`
}

// PodSnapshot represents a single pod.
// Fields ordered largest-to-smallest alignment to minimize padding.
type PodSnapshot struct {
	// Affinity holds the pod's affinity rules (pointer — 8 bytes).
	Affinity *Affinity `json:"affinity,omitempty"`
	// Containers are the pod's containers with resource requirements.
	Containers []ContainerSnapshot `json:"containers"`
	// Tolerations are the pod's tolerations.
	Tolerations []Toleration `json:"tolerations"`
	// TopologySpreadConstraints are the pod's topology spread constraints.
	TopologySpreadConstraints []TopologySpreadConstraint `json:"topologySpreadConstraints"`
	// PVCNames are the names of PVCs mounted by this pod.
	PVCNames []string `json:"pvcNames"`
	// Labels are the pod labels.
	Labels map[string]string `json:"labels"`
	// OwnerRef is the owning controller (e.g., Deployment, StatefulSet).
	OwnerRef OwnerReference `json:"ownerRef"`
	// Name is the pod name.
	Name string `json:"name"`
	// Namespace is the pod namespace.
	Namespace string `json:"namespace"`
	// NodeName is the node the pod is assigned to (empty if unscheduled).
	NodeName string `json:"nodeName"`
	// Phase is the pod phase (Running, Pending, etc.).
	Phase string `json:"phase"`
	// IsSpot indicates whether this pod is tagged for spot scheduling (set by scenarios).
	IsSpot bool `json:"isSpot"`
}

// ContainerSnapshot represents a single container within a pod.
type ContainerSnapshot struct {
	// Name is the container name.
	Name string `json:"name"`
	// Image is the container image.
	Image string `json:"image"`
	// Requests are the resource requests (CPU in millicores, memory in bytes).
	Requests ResourcePair `json:"requests"`
	// Limits are the resource limits (CPU in millicores, memory in bytes).
	Limits ResourcePair `json:"limits"`
}

// ResourcePair holds CPU and memory resource values.
type ResourcePair struct {
	// CPU in millicores.
	CPU int64 `json:"cpu"`
	// Memory in bytes.
	Memory int64 `json:"memory"`
}

// OwnerReference identifies the controller that owns a pod.
type OwnerReference struct {
	// Kind is the controller kind (Deployment, StatefulSet, DaemonSet, ReplicaSet, Job).
	Kind string `json:"kind"`
	// Name is the controller name.
	Name string `json:"name"`
}

// ControllerSnapshot represents a workload controller.
type ControllerSnapshot struct {
	// Kind is the controller kind (Deployment, StatefulSet, DaemonSet, ReplicaSet).
	Kind string `json:"kind"`
	// Name is the controller name.
	Name string `json:"name"`
	// Namespace is the controller namespace.
	Namespace string `json:"namespace"`
	// Replicas is the current number of ready replicas.
	Replicas int32 `json:"replicas"`
	// DesiredReplicas is the desired replica count.
	DesiredReplicas int32 `json:"desiredReplicas"`
	// UpdateStrategy is the update strategy (RollingUpdate, Recreate, OnDelete).
	UpdateStrategy string `json:"updateStrategy"`
	// PDBName is the name of the PDB associated with this controller, if any.
	PDBName string `json:"pdbName,omitempty"`
}

// HPASnapshot represents a horizontal pod autoscaler.
type HPASnapshot struct {
	// Name is the HPA name.
	Name string `json:"name"`
	// Namespace is the HPA namespace.
	Namespace string `json:"namespace"`
	// TargetRef identifies the scale target.
	TargetRef OwnerReference `json:"targetRef"`
	// MinReplicas is the minimum replica count.
	MinReplicas int32 `json:"minReplicas"`
	// MaxReplicas is the maximum replica count.
	MaxReplicas int32 `json:"maxReplicas"`
	// CurrentReplicas is the current number of replicas.
	CurrentReplicas int32 `json:"currentReplicas"`
	// MetricTargets are the metric targets that drive scaling.
	MetricTargets []MetricTarget `json:"metricTargets"`
}

// MetricTarget describes a single HPA metric target.
type MetricTarget struct {
	// Type is the metric type (Resource, Pods, Object, External).
	Type string `json:"type"`
	// Name is the metric name (e.g., "cpu", "memory", or a custom metric name).
	Name string `json:"name"`
	// TargetValue is the target value (percentage for Resource type, absolute for others).
	TargetValue int64 `json:"targetValue"`
}

// PDBSnapshot represents a pod disruption budget.
type PDBSnapshot struct {
	// Name is the PDB name.
	Name string `json:"name"`
	// Namespace is the PDB namespace.
	Namespace string `json:"namespace"`
	// SelectorLabels are the label selector for pods covered by this PDB.
	SelectorLabels map[string]string `json:"selectorLabels"`
	// MinAvailable is the minimum number of pods that must remain available.
	// Zero means the field is not set.
	MinAvailable int32 `json:"minAvailable"`
	// MaxUnavailable is the maximum number of pods that can be unavailable.
	// Zero means the field is not set.
	MaxUnavailable int32 `json:"maxUnavailable"`
}

// PVCSnapshot represents a persistent volume claim.
type PVCSnapshot struct {
	// Name is the PVC name.
	Name string `json:"name"`
	// Namespace is the PVC namespace.
	Namespace string `json:"namespace"`
	// StorageClass is the storage class name.
	StorageClass string `json:"storageClass"`
	// CapacityBytes is the storage capacity in bytes.
	CapacityBytes int64 `json:"capacityBytes"`
	// AccessModes are the access modes (ReadWriteOnce, ReadOnlyMany, ReadWriteMany).
	AccessModes []string `json:"accessModes"`
	// BoundNode is the node the PV is bound to (for topology-aware storage).
	BoundNode string `json:"boundNode,omitempty"`
}

// PricingData holds pricing information for instance types in the cluster.
type PricingData struct {
	// Provider is the cloud provider name (aws, gcp, azure).
	Provider string `json:"provider"`
	// Region is the cloud region.
	Region string `json:"region"`
	// InstancePricing maps instance type to pricing info.
	InstancePricing map[string]InstancePricing `json:"instancePricing"`
}

// InstancePricing holds the cost data for a single instance type.
type InstancePricing struct {
	// OnDemandHourly is the on-demand hourly cost in USD.
	OnDemandHourly float64 `json:"onDemandHourly"`
	// SpotHourly is the spot/preemptible hourly cost in USD.
	SpotHourly float64 `json:"spotHourly"`
}

// ContainerUsageProfile holds current and historical resource usage for a container.
type ContainerUsageProfile struct {
	// CurrentCPU is the current CPU usage in millicores.
	CurrentCPU int64 `json:"currentCPU"`
	// CurrentMemory is the current memory usage in bytes.
	CurrentMemory int64 `json:"currentMemory"`
	// P50CPU is the 50th percentile CPU usage in millicores.
	P50CPU int64 `json:"p50CPU"`
	// P90CPU is the 90th percentile CPU usage in millicores.
	P90CPU int64 `json:"p90CPU"`
	// P95CPU is the 95th percentile CPU usage in millicores.
	P95CPU int64 `json:"p95CPU"`
	// P99CPU is the 99th percentile CPU usage in millicores.
	P99CPU int64 `json:"p99CPU"`
	// P50Memory is the 50th percentile memory usage in bytes.
	P50Memory int64 `json:"p50Memory"`
	// P90Memory is the 90th percentile memory usage in bytes.
	P90Memory int64 `json:"p90Memory"`
	// P95Memory is the 95th percentile memory usage in bytes.
	P95Memory int64 `json:"p95Memory"`
	// P99Memory is the 99th percentile memory usage in bytes.
	P99Memory int64 `json:"p99Memory"`
	// DataPoints is the number of samples used to compute percentiles.
	DataPoints int `json:"dataPoints"`
}

// Taint represents a node taint.
type Taint struct {
	// Key is the taint key.
	Key string `json:"key"`
	// Value is the taint value.
	Value string `json:"value"`
	// Effect is the taint effect (NoSchedule, PreferNoSchedule, NoExecute).
	Effect string `json:"effect"`
}

// Toleration represents a pod toleration.
type Toleration struct {
	// Key is the taint key to match.
	Key string `json:"key"`
	// Operator is the match operator (Equal, Exists).
	Operator string `json:"operator"`
	// Value is the taint value to match (used with Equal operator).
	Value string `json:"value"`
	// Effect is the taint effect to match (empty matches all effects).
	Effect string `json:"effect"`
}

// NodeCondition represents a node condition.
type NodeCondition struct {
	// Type is the condition type (Ready, MemoryPressure, DiskPressure, PIDPressure).
	Type string `json:"type"`
	// Status is the condition status (True, False, Unknown).
	Status string `json:"status"`
}

// Affinity holds node affinity rules for a pod.
type Affinity struct {
	// NodeAffinity holds the node affinity scheduling rules.
	NodeAffinity *NodeAffinity `json:"nodeAffinity,omitempty"`
}

// NodeAffinity holds required and preferred node scheduling terms.
type NodeAffinity struct {
	// RequiredDuringSchedulingIgnoredDuringExecution is the hard constraint.
	RequiredDuringSchedulingIgnoredDuringExecution *NodeSelector `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

// NodeSelector holds a list of node selector terms (OR-ed).
type NodeSelector struct {
	// NodeSelectorTerms is a list of terms; a pod matches if any term matches.
	NodeSelectorTerms []NodeSelectorTerm `json:"nodeSelectorTerms"`
}

// NodeSelectorTerm holds a list of match expressions (AND-ed).
type NodeSelectorTerm struct {
	// MatchExpressions is a list of label match expressions; all must match.
	MatchExpressions []NodeSelectorRequirement `json:"matchExpressions"`
}

// NodeSelectorRequirement is a label match expression.
type NodeSelectorRequirement struct {
	// Key is the label key.
	Key string `json:"key"`
	// Operator is the match operator (In, NotIn, Exists, DoesNotExist).
	Operator string `json:"operator"`
	// Values is the set of values for In/NotIn operators.
	Values []string `json:"values"`
}

// TopologySpreadConstraint controls how pods are spread across topology domains.
type TopologySpreadConstraint struct {
	// MaxSkew is the maximum difference in pod count between any two topology domains.
	MaxSkew int32 `json:"maxSkew"`
	// TopologyKey is the node label key used to define topology domains.
	TopologyKey string `json:"topologyKey"`
	// WhenUnsatisfiable is the action when the constraint cannot be satisfied
	// (DoNotSchedule, ScheduleAnyway).
	WhenUnsatisfiable string `json:"whenUnsatisfiable"`
	// LabelSelector selects which pods count toward the spread.
	LabelSelector map[string]string `json:"labelSelector"`
}

// ProfileKey returns the usage profile map key for a given container.
// Format: "namespace/pod/container".
// Uses string concatenation instead of fmt.Sprintf to avoid allocations on the hot path.
func ProfileKey(namespace, pod, container string) string {
	return namespace + "/" + pod + "/" + container
}
