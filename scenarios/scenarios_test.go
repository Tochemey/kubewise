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

package scenarios_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tochemey/kubewise/pkg/scenario"
)

func TestAllExampleScenariosParse(t *testing.T) {
	files, err := filepath.Glob("*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, files, "should find scenario YAML files")

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			s, err := scenario.ParseScenarioFile(file)
			require.NoError(t, err, "failed to parse %s", file)
			assert.NotEmpty(t, s.Kind())
		})
	}
}

func TestRightsizeConservativeParsed(t *testing.T) {
	s, err := scenario.ParseScenarioFile("rightsize-conservative.yaml")
	require.NoError(t, err)
	assert.Equal(t, "RightSize", s.Kind())

	rs, ok := s.(*scenario.RightSizeScenario)
	require.True(t, ok)
	assert.Equal(t, "p95", rs.Percentile)
	assert.Equal(t, 30, rs.Buffer)
	assert.Equal(t, "ratio", rs.LimitStrategy)
	assert.Contains(t, rs.Scope.ExcludeNamespaces, "kube-system")
}

func TestRightsizeAggressiveParsed(t *testing.T) {
	s, err := scenario.ParseScenarioFile("rightsize-aggressive.yaml")
	require.NoError(t, err)
	assert.Equal(t, "RightSize", s.Kind())

	rs, ok := s.(*scenario.RightSizeScenario)
	require.True(t, ok)
	assert.Equal(t, "p90", rs.Percentile)
	assert.Equal(t, 10, rs.Buffer)
	assert.Equal(t, "fixed", rs.LimitStrategy)
}

func TestSpotStatelessParsed(t *testing.T) {
	s, err := scenario.ParseScenarioFile("spot-stateless.yaml")
	require.NoError(t, err)
	assert.Equal(t, "SpotMigrate", s.Kind())

	sm, ok := s.(*scenario.SpotMigrateScenario)
	require.True(t, ok)
	assert.Equal(t, 2, sm.MinReplicas)
	assert.Equal(t, []string{"Deployment", "ReplicaSet"}, sm.ControllerTypes)
	assert.Contains(t, sm.ExcludeNamespaces, "kube-system")
	assert.Contains(t, sm.ExcludeNamespaces, "payments")
	assert.InDelta(t, 0.65, sm.SpotDiscount, 1e-9)
}

func TestConsolidateM6iParsed(t *testing.T) {
	s, err := scenario.ParseScenarioFile("consolidate-m6i.yaml")
	require.NoError(t, err)
	assert.Equal(t, "Consolidate", s.Kind())

	cs, ok := s.(*scenario.ConsolidateScenario)
	require.True(t, ok)
	assert.Equal(t, "m6i.xlarge", cs.TargetNodeType)
	assert.Equal(t, 50, cs.MaxNodes)
	assert.Equal(t, []string{"gpu-pool"}, cs.KeepNodePools)
}

func TestCompositeSavingsParsed(t *testing.T) {
	s, err := scenario.ParseScenarioFile("composite-savings.yaml")
	require.NoError(t, err)
	assert.Equal(t, "Composite", s.Kind())

	comp, ok := s.(*scenario.CompositeScenario)
	require.True(t, ok)
	assert.Equal(t, 2, len(comp.Steps))
	assert.Equal(t, "RightSize", comp.Steps[0].Kind())
	assert.Equal(t, "SpotMigrate", comp.Steps[1].Kind())

	// Verify sub-step details
	rs, ok := comp.Steps[0].(*scenario.RightSizeScenario)
	require.True(t, ok)
	assert.Equal(t, "p95", rs.Percentile)
	assert.Equal(t, 20, rs.Buffer)

	sm, ok := comp.Steps[1].(*scenario.SpotMigrateScenario)
	require.True(t, ok)
	assert.Equal(t, 2, sm.MinReplicas)
	assert.InDelta(t, 0.65, sm.SpotDiscount, 1e-9)
}

func TestSmallClusterFixtureExists(t *testing.T) {
	path := filepath.Join("..", "testdata", "snapshots", "small-cluster.json")
	_, err := os.Stat(path)
	require.NoError(t, err, "small-cluster.json fixture should exist")
}
