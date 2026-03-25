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
	"fmt"
	"slices"

	"github.com/tochemey/kubewise/pkg/collector"
)

// ConsolidateScenario replaces current node types with a target type and re-packs.
type ConsolidateScenario struct {
	Meta              ScenarioMetadata
	TargetNodeType    string
	MaxNodes          int
	KeepNodePools     []string
	TargetAllocatable collector.ResourcePair
}

func (c *ConsolidateScenario) Kind() string { return "Consolidate" }

// Apply mutates the snapshot by removing replaceable nodes and adding
// a single initial virtual node of the target type. The actual bin-packing
// and node scaling is done by SimulateWithAutoscaler.
func (c *ConsolidateScenario) Apply(snap *collector.ClusterSnapshot) (*collector.ClusterSnapshot, error) {
	if c.TargetNodeType == "" {
		return nil, fmt.Errorf("target node type is required")
	}
	if c.TargetAllocatable.CPU == 0 && c.TargetAllocatable.Memory == 0 {
		return nil, fmt.Errorf("target allocatable resources must be specified")
	}

	keepNodes, removedNodeNames := c.separateNodes(snap.Nodes)

	// Collect pods: pods on kept nodes stay, pods on removed nodes become unscheduled
	var pods []collector.PodSnapshot
	for _, pod := range snap.Pods {
		p := pod
		if slices.Contains(removedNodeNames, pod.NodeName) {
			p.NodeName = "" // mark as unscheduled
		}
		pods = append(pods, p)
	}

	// Create one initial virtual node of the target type
	virtualNode := c.createVirtualNode(1)

	// Build new node list: kept nodes + initial virtual node
	var nodes []collector.NodeSnapshot
	nodes = append(nodes, keepNodes...)
	nodes = append(nodes, virtualNode)

	snap.Nodes = nodes
	snap.Pods = pods
	return snap, nil
}

// separateNodes splits nodes into keep and replace groups based on KeepNodePools.
func (c *ConsolidateScenario) separateNodes(nodes []collector.NodeSnapshot) (keep []collector.NodeSnapshot, removedNames []string) {
	for _, node := range nodes {
		if c.shouldKeep(node) {
			keep = append(keep, node)
		} else {
			removedNames = append(removedNames, node.Name)
		}
	}
	return
}

// shouldKeep returns true if the node's pool is in KeepNodePools.
func (c *ConsolidateScenario) shouldKeep(node collector.NodeSnapshot) bool {
	if len(c.KeepNodePools) == 0 {
		return false
	}
	pool := node.NodePool
	if pool == "" {
		pool = detectNodePoolFromLabels(node.Labels)
	}
	return slices.Contains(c.KeepNodePools, pool)
}

// CreateVirtualNode creates a virtual node of the target type with the given index.
func (c *ConsolidateScenario) createVirtualNode(index int) collector.NodeSnapshot {
	return collector.NodeSnapshot{
		Name:         fmt.Sprintf("virtual-%s-%d", c.TargetNodeType, index),
		InstanceType: c.TargetNodeType,
		Allocatable:  c.TargetAllocatable,
		Capacity:     c.TargetAllocatable,
		Labels: map[string]string{
			"node.kubernetes.io/instance-type": c.TargetNodeType,
			"kubewise.io/virtual":              "true",
		},
	}
}

// CreateVirtualNodes creates N virtual nodes of the target type.
func (c *ConsolidateScenario) CreateVirtualNodes(count int) []collector.NodeSnapshot {
	nodes := make([]collector.NodeSnapshot, count)
	for i := range count {
		nodes[i] = c.createVirtualNode(i + 1)
	}
	return nodes
}

// detectNodePoolFromLabels extracts node pool name from common cloud provider labels.
func detectNodePoolFromLabels(labels map[string]string) string {
	for _, key := range []string{
		"cloud.google.com/gke-nodepool",
		"eks.amazonaws.com/nodegroup",
		"agentpool",
	} {
		if v, ok := labels[key]; ok {
			return v
		}
	}
	return ""
}
