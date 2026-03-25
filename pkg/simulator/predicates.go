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
	"slices"

	"github.com/tochemey/kubewise/pkg/collector"
)

// FitsResources checks if the node has enough allocatable CPU and memory
// after accounting for all already-placed pods.
func FitsResources(pod collector.PodSnapshot, node *NodeState) bool {
	podCPU := totalPodCPURequest(pod)
	podMem := totalPodMemoryRequest(pod)
	availCPU := node.Allocatable.CPU - node.UsedCPU
	availMem := node.Allocatable.Memory - node.UsedMemory
	return podCPU <= availCPU && podMem <= availMem
}

// MatchesAffinity checks requiredDuringScheduling node affinity only.
// PreferredDuringScheduling is skipped (handled in scoring, Phase 2).
func MatchesAffinity(pod collector.PodSnapshot, node *NodeState) bool {
	if pod.Affinity == nil || pod.Affinity.NodeAffinity == nil {
		return true
	}
	required := pod.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if required == nil {
		return true
	}
	// Pod must match at least one NodeSelectorTerm (OR logic)
	for _, term := range required.NodeSelectorTerms {
		if matchesNodeSelectorTerm(term, node.Labels) {
			return true
		}
	}
	return false
}

// matchesNodeSelectorTerm checks if a node's labels satisfy all match expressions
// in the term (AND logic across expressions).
func matchesNodeSelectorTerm(term collector.NodeSelectorTerm, labels map[string]string) bool {
	for _, expr := range term.MatchExpressions {
		if !matchesExpression(expr, labels) {
			return false
		}
	}
	return true
}

// matchesExpression evaluates a single NodeSelectorRequirement against node labels.
func matchesExpression(expr collector.NodeSelectorRequirement, labels map[string]string) bool {
	value, exists := labels[expr.Key]

	switch expr.Operator {
	case "In":
		return exists && slices.Contains(expr.Values, value)
	case "NotIn":
		return !exists || !slices.Contains(expr.Values, value)
	case "Exists":
		return exists
	case "DoesNotExist":
		return !exists
	default:
		return false
	}
}

// ToleratesTaints checks that the pod tolerates all NoSchedule taints on the node.
func ToleratesTaints(pod collector.PodSnapshot, node *NodeState) bool {
	for _, taint := range node.Taints {
		if taint.Effect != "NoSchedule" {
			continue
		}
		if !podToleratesTaint(pod.Tolerations, taint) {
			return false
		}
	}
	return true
}

// podToleratesTaint checks if any of the pod's tolerations match the given taint.
func podToleratesTaint(tolerations []collector.Toleration, taint collector.Taint) bool {
	for _, t := range tolerations {
		if tolerationMatchesTaint(t, taint) {
			return true
		}
	}
	return false
}

// tolerationMatchesTaint checks if a single toleration matches a taint.
func tolerationMatchesTaint(toleration collector.Toleration, taint collector.Taint) bool {
	// Empty key with Exists operator tolerates everything
	if toleration.Key == "" && toleration.Operator == "Exists" {
		return true
	}

	// Key must match
	if toleration.Key != taint.Key {
		return false
	}

	// Effect must match (empty toleration effect matches all effects)
	if toleration.Effect != "" && toleration.Effect != taint.Effect {
		return false
	}

	// Operator check
	switch toleration.Operator {
	case "Exists":
		return true
	case "Equal", "":
		return toleration.Value == taint.Value
	default:
		return false
	}
}

// SatisfiesTopologySpread checks if placing the pod on the node would satisfy
// all of the pod's topology spread constraints.
func SatisfiesTopologySpread(pod collector.PodSnapshot, node *NodeState, state *SchedulingState) bool {
	for _, constraint := range pod.TopologySpreadConstraints {
		if constraint.WhenUnsatisfiable == "ScheduleAnyway" {
			continue
		}
		if !satisfiesSingleConstraint(pod, node, state, constraint) {
			return false
		}
	}
	return true
}

// satisfiesSingleConstraint checks a single topology spread constraint.
func satisfiesSingleConstraint(_ collector.PodSnapshot, node *NodeState, state *SchedulingState, constraint collector.TopologySpreadConstraint) bool {
	topologyKey := constraint.TopologyKey
	targetDomain, ok := node.Labels[topologyKey]
	if !ok {
		// Node doesn't have the topology key — cannot place here
		return false
	}

	// Count matching pods per domain
	domainCounts := countPodsPerDomain(state, constraint.LabelSelector, topologyKey)

	// Simulate placing this pod on the target domain
	domainCounts[targetDomain]++

	// Find min and max counts
	minCount := int32(0)
	maxCount := int32(0)
	first := true
	for _, count := range domainCounts {
		if first {
			minCount = count
			maxCount = count
			first = false
		} else {
			if count < minCount {
				minCount = count
			}
			if count > maxCount {
				maxCount = count
			}
		}
	}

	// Also consider domains that exist in the cluster but have zero matching pods
	for _, n := range state.Nodes {
		if domain, hasKey := n.Labels[topologyKey]; hasKey {
			if _, counted := domainCounts[domain]; !counted {
				domainCounts[domain] = 0
				minCount = 0
			}
		}
	}

	skew := maxCount - minCount
	return skew <= constraint.MaxSkew
}

// countPodsPerDomain counts pods matching the label selector per topology domain.
func countPodsPerDomain(state *SchedulingState, selector map[string]string, topologyKey string) map[string]int32 {
	counts := make(map[string]int32)
	for _, node := range state.Nodes {
		domain, hasDomain := node.Labels[topologyKey]
		if !hasDomain {
			continue
		}
		for _, pod := range node.Pods {
			if matchesSelector(pod.Labels, selector) {
				counts[domain]++
			}
		}
	}
	return counts
}

// matchesSelector checks if all selector labels are present in the pod labels.
func matchesSelector(podLabels, selector map[string]string) bool {
	for k, v := range selector {
		if podLabels[k] != v {
			return false
		}
	}
	return true
}
