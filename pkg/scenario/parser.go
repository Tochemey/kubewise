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
	"os"

	"gopkg.in/yaml.v3"
)

// scenarioFile represents the top-level YAML structure of a scenario file.
type scenarioFile struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   scenarioMetadata `yaml:"metadata"`
	Spec       yaml.Node        `yaml:"spec"`
}

type scenarioMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// compositeSpec is the spec for Composite scenarios.
type compositeSpec struct {
	Steps []compositeStep `yaml:"steps"`
}

type compositeStep struct {
	Kind string    `yaml:"kind"`
	Spec yaml.Node `yaml:"spec"`
}

// rightSizeSpec is the spec for RightSize scenarios (parsed from YAML).
type rightSizeSpec struct {
	Percentile string      `yaml:"percentile"`
	Buffer     int         `yaml:"buffer"`
	Scope      *scopeSpec  `yaml:"scope"`
	Limits     *limitsSpec `yaml:"limits"`
}

type scopeSpec struct {
	Namespaces []string      `yaml:"namespaces"`
	Exclude    []excludeSpec `yaml:"exclude"`
}

type excludeSpec struct {
	Namespace string `yaml:"namespace"`
	Label     string `yaml:"label"`
}

type limitsSpec struct {
	Strategy string `yaml:"strategy"`
}

// consolidateSpec is the spec for Consolidate scenarios.
type consolidateSpec struct {
	TargetNodeType string   `yaml:"target_node_type"`
	MaxNodes       int      `yaml:"max_nodes"`
	KeepNodePools  []string `yaml:"keep_node_pools"`
}

// spotMigrateSpec is the spec for SpotMigrate scenarios.
type spotMigrateSpec struct {
	Eligibility  spotEligibility `yaml:"eligibility"`
	SpotDiscount float64         `yaml:"spot_discount"`
}

type spotEligibility struct {
	MinReplicas       int      `yaml:"min_replicas"`
	ControllerTypes   []string `yaml:"controller_types"`
	ExcludeNamespaces []string `yaml:"exclude_namespaces"`
}

// ParseScenarioFile reads a YAML scenario file and returns a Scenario.
func ParseScenarioFile(path string) (Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading scenario file %s: %w", path, err)
	}
	return ParseScenarioBytes(data)
}

// ParseScenarioBytes parses scenario YAML bytes and returns a Scenario.
func ParseScenarioBytes(data []byte) (Scenario, error) {
	var sf scenarioFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parsing scenario YAML: %w", err)
	}

	if sf.Kind == "" {
		return nil, fmt.Errorf("scenario YAML missing 'kind' field")
	}

	meta := ScenarioMetadata{
		Name:        sf.Metadata.Name,
		Description: sf.Metadata.Description,
	}

	return parseScenarioByKind(sf.Kind, meta, &sf.Spec)
}

func parseScenarioByKind(kind string, meta ScenarioMetadata, specNode *yaml.Node) (Scenario, error) {
	switch kind {
	case KindRightSize:
		return parseRightSizeScenario(meta, specNode)
	case KindConsolidate:
		return parseConsolidateScenario(meta, specNode)
	case KindSpotMigrate:
		return parseSpotMigrateScenario(meta, specNode)
	case KindComposite:
		return parseCompositeScenario(meta, specNode)
	default:
		return nil, fmt.Errorf("unknown scenario kind: %s", kind)
	}
}

func parseRightSizeScenario(meta ScenarioMetadata, specNode *yaml.Node) (Scenario, error) {
	var spec rightSizeSpec
	if err := specNode.Decode(&spec); err != nil {
		return nil, fmt.Errorf("parsing RightSize spec: %w", err)
	}

	scope := DefaultScope()
	if spec.Scope != nil {
		scope.Namespaces = spec.Scope.Namespaces
		for _, exc := range spec.Scope.Exclude {
			if exc.Namespace != "" {
				scope.ExcludeNamespaces = append(scope.ExcludeNamespaces, exc.Namespace)
			}
			if exc.Label != "" {
				if scope.ExcludeLabels == nil {
					scope.ExcludeLabels = make(map[string]string)
				}
				// Parse "key=value" format
				key, value := parseLabel(exc.Label)
				scope.ExcludeLabels[key] = value
			}
		}
	}

	limitStrategy := LimitStrategyRatio
	if spec.Limits != nil && spec.Limits.Strategy != "" {
		limitStrategy = spec.Limits.Strategy
	}

	return &RightSizeScenario{
		Meta:          meta,
		Percentile:    spec.Percentile,
		Buffer:        spec.Buffer,
		Scope:         scope,
		LimitStrategy: limitStrategy,
	}, nil
}

func parseConsolidateScenario(meta ScenarioMetadata, specNode *yaml.Node) (Scenario, error) {
	var spec consolidateSpec
	if err := specNode.Decode(&spec); err != nil {
		return nil, fmt.Errorf("parsing Consolidate spec: %w", err)
	}

	return &ConsolidateScenario{
		Meta:           meta,
		TargetNodeType: spec.TargetNodeType,
		MaxNodes:       spec.MaxNodes,
		KeepNodePools:  spec.KeepNodePools,
	}, nil
}

func parseSpotMigrateScenario(meta ScenarioMetadata, specNode *yaml.Node) (Scenario, error) {
	var spec spotMigrateSpec
	if err := specNode.Decode(&spec); err != nil {
		return nil, fmt.Errorf("parsing SpotMigrate spec: %w", err)
	}

	return &SpotMigrateScenario{
		Meta:              meta,
		MinReplicas:       spec.Eligibility.MinReplicas,
		ControllerTypes:   spec.Eligibility.ControllerTypes,
		ExcludeNamespaces: spec.Eligibility.ExcludeNamespaces,
		SpotDiscount:      spec.SpotDiscount,
	}, nil
}

func parseCompositeScenario(meta ScenarioMetadata, specNode *yaml.Node) (Scenario, error) {
	var spec compositeSpec
	if err := specNode.Decode(&spec); err != nil {
		return nil, fmt.Errorf("parsing Composite spec: %w", err)
	}

	if len(spec.Steps) == 0 {
		return nil, fmt.Errorf("composite scenario has no steps")
	}

	var steps []Scenario
	for i, step := range spec.Steps {
		s, err := parseScenarioByKind(step.Kind, ScenarioMetadata{
			Name: fmt.Sprintf("%s-step-%d", meta.Name, i+1),
		}, &step.Spec)
		if err != nil {
			return nil, fmt.Errorf("parsing composite step %d (%s): %w", i+1, step.Kind, err)
		}
		steps = append(steps, s)
	}

	return &CompositeScenario{Meta: meta, Steps: steps}, nil
}

// parseLabel splits "key=value" into key and value.
func parseLabel(s string) (string, string) {
	for i, c := range s {
		if c == '=' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}
