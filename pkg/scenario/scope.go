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
	"slices"

	"github.com/tochemey/kubewise/pkg/collector"
)

// Scope defines which pods are in scope for a scenario mutation.
type Scope struct {
	// Namespaces is the list of namespaces to include. ["*"] means all namespaces.
	Namespaces []string `yaml:"namespaces"`
	// ExcludeNamespaces is a list of namespaces to exclude (takes priority over includes).
	ExcludeNamespaces []string `yaml:"exclude_namespaces"`
	// ExcludeLabels is a map of label key-value pairs to exclude.
	ExcludeLabels map[string]string `yaml:"exclude_labels"`
}

// Includes returns true if the given pod is in scope.
// Excludes take priority over includes.
func (s Scope) Includes(pod collector.PodSnapshot) bool {
	// Check excludes first (they take priority)
	if slices.Contains(s.ExcludeNamespaces, pod.Namespace) {
		return false
	}
	for k, v := range s.ExcludeLabels {
		if pod.Labels[k] == v {
			return false
		}
	}

	// Check includes
	if len(s.Namespaces) == 0 {
		return true
	}
	if len(s.Namespaces) == 1 && s.Namespaces[0] == "*" {
		return true
	}
	return slices.Contains(s.Namespaces, pod.Namespace)
}

// DefaultScope returns a scope that includes all pods.
func DefaultScope() Scope {
	return Scope{Namespaces: []string{"*"}}
}
