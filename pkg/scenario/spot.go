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
	"slices"

	"github.com/tochemey/kubewise/pkg/collector"
)

// SpotMigrateScenario moves eligible workloads to spot instances.
type SpotMigrateScenario struct {
	Meta              ScenarioMetadata
	MinReplicas       int
	ControllerTypes   []string
	ExcludeNamespaces []string
	SpotDiscount      float64
}

func (s *SpotMigrateScenario) Kind() string { return KindSpotMigrate }

// Apply tags eligible pods as spot-scheduled based on controller type,
// replica count, and namespace exclusions. The snapshot is already a deep copy.
func (s *SpotMigrateScenario) Apply(snap *collector.ClusterSnapshot) (*collector.ClusterSnapshot, error) {
	// Build controller lookup: "namespace/name" → ControllerSnapshot
	controllers := buildControllerMap(snap.Controllers)

	for i := range snap.Pods {
		pod := &snap.Pods[i]
		if s.isEligible(pod, controllers) {
			pod.IsSpot = true
		}
	}

	return snap, nil
}

// isEligible checks if a pod is eligible for spot migration.
func (s *SpotMigrateScenario) isEligible(pod *collector.PodSnapshot, controllers map[string]collector.ControllerSnapshot) bool {
	// Check namespace exclusion
	if slices.Contains(s.ExcludeNamespaces, pod.Namespace) {
		return false
	}

	// Check controller type
	if len(s.ControllerTypes) > 0 && !slices.Contains(s.ControllerTypes, pod.OwnerRef.Kind) {
		return false
	}

	// Check replica count
	if s.MinReplicas > 0 {
		key := controllerKey(pod.Namespace, pod.OwnerRef.Name)
		ctrl, ok := controllers[key]
		if !ok {
			return false
		}
		if int(ctrl.DesiredReplicas) < s.MinReplicas {
			return false
		}
	}

	return true
}

// buildControllerMap creates a lookup map keyed by "namespace/name".
func buildControllerMap(controllers []collector.ControllerSnapshot) map[string]collector.ControllerSnapshot {
	m := make(map[string]collector.ControllerSnapshot, len(controllers))
	for _, ctrl := range controllers {
		m[controllerKey(ctrl.Namespace, ctrl.Name)] = ctrl
	}
	return m
}

func controllerKey(namespace, name string) string {
	return namespace + "/" + name
}
