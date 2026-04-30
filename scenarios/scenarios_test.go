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

func TestSmallClusterFixtureExists(t *testing.T) {
	path := filepath.Join("..", "testdata", "snapshots", "small-cluster.json")
	_, err := os.Stat(path)
	require.NoError(t, err, "small-cluster.json fixture should exist")
}
