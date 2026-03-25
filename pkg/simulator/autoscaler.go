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
	"fmt"

	"github.com/tochemey/kubewise/pkg/collector"
)

// SimulateWithAutoscaler runs the bin-packing simulation and adds virtual nodes
// until all pods are scheduled or the maxNodes cap is reached.
func SimulateWithAutoscaler(snap *collector.ClusterSnapshot, targetNodeType string, targetAllocatable collector.ResourcePair, maxNodes int) (*SimulationResult, error) {
	// Run initial simulation
	result, err := Simulate(snap)
	if err != nil {
		return nil, fmt.Errorf("initial simulation: %w", err)
	}

	nodeCount := len(snap.Nodes)
	iteration := 0

	// Keep adding nodes until all pods are scheduled or cap is hit
	for len(result.UnschedulablePods) > 0 {
		if maxNodes > 0 && nodeCount >= maxNodes {
			break
		}

		iteration++
		// Add a new virtual node
		virtualNode := collector.NodeSnapshot{
			Name:         fmt.Sprintf("virtual-%s-%d", targetNodeType, nodeCount+1),
			InstanceType: targetNodeType,
			Allocatable:  targetAllocatable,
			Capacity:     targetAllocatable,
			Labels: map[string]string{
				"node.kubernetes.io/instance-type": targetNodeType,
				"kubewise.io/virtual":              "true",
			},
		}
		snap.Nodes = append(snap.Nodes, virtualNode)
		nodeCount++

		// Re-simulate with the expanded node set
		result, err = Simulate(snap)
		if err != nil {
			return nil, fmt.Errorf("simulation iteration %d: %w", iteration, err)
		}
	}

	return result, nil
}
