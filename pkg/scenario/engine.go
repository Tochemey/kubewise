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

	"github.com/tochemey/kubewise/pkg/collector"
)

// Scenario defines a mutation that can be applied to a cluster snapshot.
type Scenario interface {
	// Kind returns the scenario type name (e.g., "RightSize", "Consolidate", "SpotMigrate").
	Kind() string
	// Apply mutates a snapshot copy and returns the result.
	// The snapshot passed to Apply is already a deep copy — implementations
	// may modify it directly.
	Apply(snap *collector.ClusterSnapshot) (*collector.ClusterSnapshot, error)
}

// ScenarioMetadata holds common metadata for all scenario types.
type ScenarioMetadata struct {
	// Name is the scenario name.
	Name string
	// Description is a human-readable description shown in output.
	Description string
}

// ApplyScenario deep-copies the snapshot, then applies the scenario to the copy.
// This is the ONLY place DeepCopy should be called for scenario application.
func ApplyScenario(scenario Scenario, snap *collector.ClusterSnapshot) (*collector.ClusterSnapshot, error) {
	if snap == nil {
		return nil, fmt.Errorf("snapshot is nil")
	}
	copied := snap.DeepCopy()
	result, err := scenario.Apply(copied)
	if err != nil {
		return nil, fmt.Errorf("applying %s scenario: %w", scenario.Kind(), err)
	}
	return result, nil
}

// ApplyComposite applies multiple scenarios sequentially.
// Each scenario receives the output of the previous one.
// The initial snapshot is deep-copied once at the start.
func ApplyComposite(scenarios []Scenario, snap *collector.ClusterSnapshot) (*collector.ClusterSnapshot, error) {
	if snap == nil {
		return nil, fmt.Errorf("snapshot is nil")
	}
	if len(scenarios) == 0 {
		return nil, fmt.Errorf("no scenarios to apply")
	}

	current := snap.DeepCopy()
	for i, s := range scenarios {
		result, err := s.Apply(current)
		if err != nil {
			return nil, fmt.Errorf("applying step %d (%s): %w", i+1, s.Kind(), err)
		}
		current = result
	}
	return current, nil
}

// CompositeScenario chains multiple scenarios sequentially.
type CompositeScenario struct {
	Meta  ScenarioMetadata
	Steps []Scenario
}

func (c *CompositeScenario) Kind() string { return "Composite" }

// Apply applies each step in order. The snapshot is already a deep copy.
func (c *CompositeScenario) Apply(snap *collector.ClusterSnapshot) (*collector.ClusterSnapshot, error) {
	current := snap
	for i, step := range c.Steps {
		result, err := step.Apply(current)
		if err != nil {
			return nil, fmt.Errorf("composite step %d (%s): %w", i+1, step.Kind(), err)
		}
		current = result
	}
	return current, nil
}
