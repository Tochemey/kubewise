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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRightSizeScenario(t *testing.T) {
	yaml := `
apiVersion: kubewise.io/v1alpha1
kind: RightSize
metadata:
  name: conservative
  description: "Conservative right-sizing"
spec:
  percentile: p95
  buffer: 20
  scope:
    namespaces: ["*"]
    exclude:
      - namespace: kube-system
      - label: "kubewise.io/skip=true"
  limits:
    strategy: ratio
`
	s, err := ParseScenarioBytes([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, "RightSize", s.Kind())

	rs, ok := s.(*RightSizeScenario)
	require.True(t, ok)
	assert.Equal(t, "conservative", rs.Meta.Name)
	assert.Equal(t, "Conservative right-sizing", rs.Meta.Description)
	assert.Equal(t, "p95", rs.Percentile)
	assert.Equal(t, 20, rs.Buffer)
	assert.Equal(t, "ratio", rs.LimitStrategy)
	assert.Equal(t, []string{"*"}, rs.Scope.Namespaces)
	assert.Equal(t, []string{"kube-system"}, rs.Scope.ExcludeNamespaces)
	assert.Equal(t, "true", rs.Scope.ExcludeLabels["kubewise.io/skip"])
}

func TestParseRightSizeScenarioDefaults(t *testing.T) {
	yaml := `
apiVersion: kubewise.io/v1alpha1
kind: RightSize
metadata:
  name: minimal
spec:
  percentile: p90
  buffer: 10
`
	s, err := ParseScenarioBytes([]byte(yaml))
	require.NoError(t, err)

	rs, ok := s.(*RightSizeScenario)
	require.True(t, ok)
	assert.Equal(t, "ratio", rs.LimitStrategy)          // default
	assert.Equal(t, []string{"*"}, rs.Scope.Namespaces) // default scope
}

func TestParseCompositeScenario(t *testing.T) {
	yaml := `
apiVersion: kubewise.io/v1alpha1
kind: Composite
metadata:
  name: layered-rightsize
  description: "Aggressive then conservative right-size"
spec:
  steps:
    - kind: RightSize
      spec:
        percentile: p90
        buffer: 15
    - kind: RightSize
      spec:
        percentile: p99
        buffer: 30
`
	s, err := ParseScenarioBytes([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, "Composite", s.Kind())

	cs, ok := s.(*CompositeScenario)
	require.True(t, ok)
	assert.Equal(t, "layered-rightsize", cs.Meta.Name)
	assert.Equal(t, 2, len(cs.Steps))
	assert.Equal(t, "RightSize", cs.Steps[0].Kind())
	assert.Equal(t, "RightSize", cs.Steps[1].Kind())

	rs, ok := cs.Steps[0].(*RightSizeScenario)
	require.True(t, ok)
	assert.Equal(t, "p90", rs.Percentile)
	assert.Equal(t, 15, rs.Buffer)
}

func TestParseUnknownKind(t *testing.T) {
	yaml := `
apiVersion: kubewise.io/v1alpha1
kind: DoSomethingWeird
metadata:
  name: test
spec:
  foo: bar
`
	_, err := ParseScenarioBytes([]byte(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown scenario kind")
}

func TestParseMissingKind(t *testing.T) {
	yaml := `
apiVersion: kubewise.io/v1alpha1
metadata:
  name: test
spec:
  foo: bar
`
	_, err := ParseScenarioBytes([]byte(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'kind'")
}

func TestParseInvalidYAML(t *testing.T) {
	_, err := ParseScenarioBytes([]byte("{{not yaml"))
	require.Error(t, err)
}

func TestParseCompositeNoSteps(t *testing.T) {
	yaml := `
apiVersion: kubewise.io/v1alpha1
kind: Composite
metadata:
  name: empty
spec:
  steps: []
`
	_, err := ParseScenarioBytes([]byte(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no steps")
}

func TestParseScenarioFile(t *testing.T) {
	content := `
apiVersion: kubewise.io/v1alpha1
kind: RightSize
metadata:
  name: from-file
spec:
  percentile: p95
  buffer: 25
`
	path := filepath.Join(t.TempDir(), "scenario.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	s, err := ParseScenarioFile(path)
	require.NoError(t, err)
	assert.Equal(t, "RightSize", s.Kind())

	rs, ok := s.(*RightSizeScenario)
	require.True(t, ok)
	assert.Equal(t, "from-file", rs.Meta.Name)
}

func TestParseScenarioFileNotFound(t *testing.T) {
	_, err := ParseScenarioFile("/nonexistent/path.yaml")
	require.Error(t, err)
}

func TestParseLabel(t *testing.T) {
	tests := []struct {
		input     string
		wantKey   string
		wantValue string
	}{
		{"kubewise.io/skip=true", "kubewise.io/skip", "true"},
		{"env=production", "env", "production"},
		{"novalue", "novalue", ""},
		{"key=", "key", ""},
		{"=value", "", "value"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			k, v := parseLabel(tt.input)
			assert.Equal(t, tt.wantKey, k)
			assert.Equal(t, tt.wantValue, v)
		})
	}
}
