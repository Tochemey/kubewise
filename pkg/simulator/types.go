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

package simulator

import (
	"github.com/tochemey/kubewise/pkg/collector"
)

// SchedulingState tracks node resource usage and pod placements during simulation.
type SchedulingState struct {
	// Nodes maps node name to its scheduling state.
	Nodes map[string]*NodeState
	// Placements records all pod-to-node assignments.
	Placements []Placement
}

// NodeState tracks a single node's resources and placed pods during simulation.
type NodeState struct {
	// Name is the node name.
	Name string
	// Allocatable is the node's allocatable resources.
	Allocatable collector.ResourcePair
	// UsedCPU is the total CPU millicores consumed by placed pods.
	UsedCPU int64
	// UsedMemory is the total memory bytes consumed by placed pods.
	UsedMemory int64
	// Labels are the node labels.
	Labels map[string]string
	// Taints are the node taints.
	Taints []collector.Taint
	// Pods is the list of pods placed on this node.
	Pods []collector.PodSnapshot
}

// Placement records a single pod-to-node assignment.
type Placement struct {
	PodNamespace string
	PodName      string
	NodeName     string
}

// NodeUtilization captures the resource utilization of a node.
// Fields ordered largest-to-smallest alignment.
type NodeUtilization struct {
	CPUUsed     int64
	CPUTotal    int64
	MemoryUsed  int64
	MemoryTotal int64
	NodeName    string
	PodCount    int
}

// SimulationResult is the output of a scheduling simulation.
type SimulationResult struct {
	// Placements records all successful pod-to-node assignments.
	Placements []Placement
	// UnschedulablePods are pods that could not be placed on any node.
	UnschedulablePods []collector.PodSnapshot
	// NodeUtilization maps node name to its utilization stats.
	NodeUtilization map[string]NodeUtilization
	// TotalNodes is the total number of nodes in the simulation.
	TotalNodes int
	// ActiveNodes is the number of nodes with at least one pod.
	ActiveNodes int
}

// NewSchedulingState creates a SchedulingState from a list of node snapshots.
func NewSchedulingState(nodes []collector.NodeSnapshot) *SchedulingState {
	state := &SchedulingState{
		Nodes:      make(map[string]*NodeState, len(nodes)),
		Placements: make([]Placement, 0, len(nodes)*10), // estimate ~10 pods per node
	}
	for _, node := range nodes {
		state.Nodes[node.Name] = &NodeState{
			Name:        node.Name,
			Allocatable: node.Allocatable,
			Labels:      node.Labels,
			Taints:      node.Taints,
			Pods:        make([]collector.PodSnapshot, 0, 8), // pre-allocate per-node pod list
		}
	}
	return state
}

// Place assigns a pod to a node, updating resource usage.
func (s *SchedulingState) Place(pod collector.PodSnapshot, nodeName string) {
	s.PlaceWithResources(pod, nodeName, totalPodCPURequest(pod), totalPodMemoryRequest(pod))
}

// PlaceWithResources assigns a pod to a node using pre-computed resource totals.
// This avoids recomputing pod resource sums when they're already known.
func (s *SchedulingState) PlaceWithResources(pod collector.PodSnapshot, nodeName string, cpuReq, memReq int64) {
	node, ok := s.Nodes[nodeName]
	if !ok {
		return
	}

	node.UsedCPU += cpuReq
	node.UsedMemory += memReq
	node.Pods = append(node.Pods, pod)

	s.Placements = append(s.Placements, Placement{
		PodNamespace: pod.Namespace,
		PodName:      pod.Name,
		NodeName:     nodeName,
	})
}

// ActiveNodes returns all nodes that have at least one pod placed on them.
func (s *SchedulingState) ActiveNodes() []*NodeState {
	var active []*NodeState
	for _, node := range s.Nodes {
		if len(node.Pods) > 0 {
			active = append(active, node)
		}
	}
	return active
}

// Utilization computes per-node utilization statistics.
func (s *SchedulingState) Utilization() map[string]NodeUtilization {
	result := make(map[string]NodeUtilization, len(s.Nodes))
	for name, node := range s.Nodes {
		result[name] = NodeUtilization{
			NodeName:    name,
			CPUUsed:     node.UsedCPU,
			CPUTotal:    node.Allocatable.CPU,
			MemoryUsed:  node.UsedMemory,
			MemoryTotal: node.Allocatable.Memory,
			PodCount:    len(node.Pods),
		}
	}
	return result
}

// totalPodCPURequest sums CPU requests across all containers in a pod.
func totalPodCPURequest(pod collector.PodSnapshot) int64 {
	var total int64
	for _, c := range pod.Containers {
		total += c.Requests.CPU
	}
	return total
}

// totalPodMemoryRequest sums memory requests across all containers in a pod.
func totalPodMemoryRequest(pod collector.PodSnapshot) int64 {
	var total int64
	for _, c := range pod.Containers {
		total += c.Requests.Memory
	}
	return total
}
