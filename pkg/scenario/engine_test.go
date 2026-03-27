// Copyright 2026 KubeWise Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scenario

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tochemey/kubewise/pkg/collector"
)

// mutatingScenario is a test scenario that modifies the snapshot.
type mutatingScenario struct {
	name  string
	mutFn func(snap *collector.ClusterSnapshot)
}

func (m *mutatingScenario) Kind() string { return m.name }
func (m *mutatingScenario) Apply(snap *collector.ClusterSnapshot) (*collector.ClusterSnapshot, error) {
	m.mutFn(snap)
	return snap, nil
}

// failingScenario always returns an error.
type failingScenario struct{}

func (f *failingScenario) Kind() string { return "Failing" }
func (f *failingScenario) Apply(_ *collector.ClusterSnapshot) (*collector.ClusterSnapshot, error) {
	return nil, fmt.Errorf("intentional failure")
}

func newMinimalSnapshot() *collector.ClusterSnapshot {
	return &collector.ClusterSnapshot{
		Timestamp: time.Now(),
		Pods: []collector.PodSnapshot{
			{
				Name:      "web-1",
				Namespace: "default",
				Containers: []collector.ContainerSnapshot{
					{Name: "nginx", Requests: collector.ResourcePair{CPU: 500, Memory: 256}},
				},
				Labels: map[string]string{"app": "web"},
			},
		},
		Nodes: []collector.NodeSnapshot{
			{Name: "node-1", Allocatable: collector.ResourcePair{CPU: 4000, Memory: 16000}},
		},
	}
}

func TestApplyScenarioDeepCopyIsolation(t *testing.T) {
	original := newMinimalSnapshot()
	originalCPU := original.Pods[0].Containers[0].Requests.CPU

	scenario := &mutatingScenario{
		name: "Mutator",
		mutFn: func(snap *collector.ClusterSnapshot) {
			snap.Pods[0].Containers[0].Requests.CPU = 9999
			snap.Pods[0].Labels["new"] = "label"
		},
	}

	result, err := ApplyScenario(scenario, original)
	require.NoError(t, err)

	// Result should have the mutation
	assert.Equal(t, int64(9999), result.Pods[0].Containers[0].Requests.CPU)
	assert.Equal(t, "label", result.Pods[0].Labels["new"])

	// Original should be unchanged
	assert.Equal(t, originalCPU, original.Pods[0].Containers[0].Requests.CPU)
	_, hasNew := original.Pods[0].Labels["new"]
	assert.False(t, hasNew)
}

func TestApplyScenarioNilSnapshot(t *testing.T) {
	scenario := &mutatingScenario{name: "Test", mutFn: func(_ *collector.ClusterSnapshot) {}}
	_, err := ApplyScenario(scenario, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestApplyScenarioError(t *testing.T) {
	snap := newMinimalSnapshot()
	_, err := ApplyScenario(&failingScenario{}, snap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Failing")
	assert.Contains(t, err.Error(), "intentional failure")
}

func TestApplyCompositeSequentialOrder(t *testing.T) {
	original := newMinimalSnapshot()
	var order []string

	step1 := &mutatingScenario{
		name: "Step1",
		mutFn: func(snap *collector.ClusterSnapshot) {
			order = append(order, "step1")
			// Double the CPU request
			snap.Pods[0].Containers[0].Requests.CPU *= 2
		},
	}
	step2 := &mutatingScenario{
		name: "Step2",
		mutFn: func(snap *collector.ClusterSnapshot) {
			order = append(order, "step2")
			// Add 100 to the (already doubled) CPU
			snap.Pods[0].Containers[0].Requests.CPU += 100
		},
	}

	result, err := ApplyComposite([]Scenario{step1, step2}, original)
	require.NoError(t, err)

	// Steps should execute in order
	assert.Equal(t, []string{"step1", "step2"}, order)

	// Result should reflect sequential application: (500 * 2) + 100 = 1100
	assert.Equal(t, int64(1100), result.Pods[0].Containers[0].Requests.CPU)

	// Original should be unchanged
	assert.Equal(t, int64(500), original.Pods[0].Containers[0].Requests.CPU)
}

func TestApplyCompositeNilSnapshot(t *testing.T) {
	_, err := ApplyComposite([]Scenario{&mutatingScenario{name: "T", mutFn: func(_ *collector.ClusterSnapshot) {}}}, nil)
	require.Error(t, err)
}

func TestApplyCompositeNoScenarios(t *testing.T) {
	_, err := ApplyComposite([]Scenario{}, newMinimalSnapshot())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no scenarios")
}

func TestApplyCompositeErrorInMiddle(t *testing.T) {
	step1 := &mutatingScenario{name: "OK", mutFn: func(_ *collector.ClusterSnapshot) {}}
	step2 := &failingScenario{}

	_, err := ApplyComposite([]Scenario{step1, step2}, newMinimalSnapshot())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step 2")
}

func TestCompositeScenarioKind(t *testing.T) {
	cs := &CompositeScenario{}
	assert.Equal(t, "Composite", cs.Kind())
}

func TestCompositeScenarioApply(t *testing.T) {
	snap := newMinimalSnapshot()

	cs := &CompositeScenario{
		Steps: []Scenario{
			&mutatingScenario{
				name: "Double",
				mutFn: func(s *collector.ClusterSnapshot) {
					s.Pods[0].Containers[0].Requests.CPU *= 2
				},
			},
		},
	}

	result, err := ApplyScenario(cs, snap)
	require.NoError(t, err)
	assert.Equal(t, int64(1000), result.Pods[0].Containers[0].Requests.CPU)
	assert.Equal(t, int64(500), snap.Pods[0].Containers[0].Requests.CPU)
}
