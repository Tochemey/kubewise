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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tochemey/kubewise/pkg/collector"
)

func TestScopeWildcard(t *testing.T) {
	scope := Scope{Namespaces: []string{"*"}}
	assert.True(t, scope.Includes(collector.PodSnapshot{Namespace: "default"}))
	assert.True(t, scope.Includes(collector.PodSnapshot{Namespace: "kube-system"}))
	assert.True(t, scope.Includes(collector.PodSnapshot{Namespace: "anything"}))
}

func TestScopeEmptyNamespacesIncludesAll(t *testing.T) {
	scope := Scope{}
	assert.True(t, scope.Includes(collector.PodSnapshot{Namespace: "default"}))
	assert.True(t, scope.Includes(collector.PodSnapshot{Namespace: "kube-system"}))
}

func TestScopeSpecificNamespaces(t *testing.T) {
	scope := Scope{Namespaces: []string{"default", "api"}}
	assert.True(t, scope.Includes(collector.PodSnapshot{Namespace: "default"}))
	assert.True(t, scope.Includes(collector.PodSnapshot{Namespace: "api"}))
	assert.False(t, scope.Includes(collector.PodSnapshot{Namespace: "kube-system"}))
	assert.False(t, scope.Includes(collector.PodSnapshot{Namespace: "other"}))
}

func TestScopeExcludeNamespaceOverridesInclude(t *testing.T) {
	scope := Scope{
		Namespaces:        []string{"*"},
		ExcludeNamespaces: []string{"kube-system", "payments"},
	}
	assert.True(t, scope.Includes(collector.PodSnapshot{Namespace: "default"}))
	assert.False(t, scope.Includes(collector.PodSnapshot{Namespace: "kube-system"}))
	assert.False(t, scope.Includes(collector.PodSnapshot{Namespace: "payments"}))
}

func TestScopeExcludeNamespaceWithSpecificIncludes(t *testing.T) {
	scope := Scope{
		Namespaces:        []string{"default", "api", "kube-system"},
		ExcludeNamespaces: []string{"kube-system"},
	}
	assert.True(t, scope.Includes(collector.PodSnapshot{Namespace: "default"}))
	assert.True(t, scope.Includes(collector.PodSnapshot{Namespace: "api"}))
	assert.False(t, scope.Includes(collector.PodSnapshot{Namespace: "kube-system"}))
}

func TestScopeExcludeLabels(t *testing.T) {
	scope := Scope{
		Namespaces:    []string{"*"},
		ExcludeLabels: map[string]string{"kubewise.io/skip": "true"},
	}

	assert.True(t, scope.Includes(collector.PodSnapshot{
		Namespace: "default",
		Labels:    map[string]string{"app": "web"},
	}))

	assert.False(t, scope.Includes(collector.PodSnapshot{
		Namespace: "default",
		Labels:    map[string]string{"kubewise.io/skip": "true"},
	}))

	// Different value for the same key — should NOT be excluded
	assert.True(t, scope.Includes(collector.PodSnapshot{
		Namespace: "default",
		Labels:    map[string]string{"kubewise.io/skip": "false"},
	}))
}

func TestScopeExcludeLabelsWithNilLabels(t *testing.T) {
	scope := Scope{
		Namespaces:    []string{"*"},
		ExcludeLabels: map[string]string{"kubewise.io/skip": "true"},
	}

	// Pod with nil labels should not be excluded
	assert.True(t, scope.Includes(collector.PodSnapshot{Namespace: "default"}))
}

func TestScopeMultipleExcludeLabels(t *testing.T) {
	scope := Scope{
		Namespaces: []string{"*"},
		ExcludeLabels: map[string]string{
			"kubewise.io/skip": "true",
			"env":              "staging",
		},
	}

	// Excluded by first label
	assert.False(t, scope.Includes(collector.PodSnapshot{
		Namespace: "default",
		Labels:    map[string]string{"kubewise.io/skip": "true"},
	}))

	// Excluded by second label
	assert.False(t, scope.Includes(collector.PodSnapshot{
		Namespace: "default",
		Labels:    map[string]string{"env": "staging"},
	}))

	// Not excluded
	assert.True(t, scope.Includes(collector.PodSnapshot{
		Namespace: "default",
		Labels:    map[string]string{"env": "production"},
	}))
}

func TestDefaultScope(t *testing.T) {
	scope := DefaultScope()
	assert.Equal(t, []string{"*"}, scope.Namespaces)
	assert.True(t, scope.Includes(collector.PodSnapshot{Namespace: "anything"}))
}
