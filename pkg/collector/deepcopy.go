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

package collector

import "maps"

// DeepCopy returns a fully independent copy of the ClusterSnapshot.
func (s *ClusterSnapshot) DeepCopy() *ClusterSnapshot {
	if s == nil {
		return nil
	}

	out := &ClusterSnapshot{
		Timestamp:           s.Timestamp,
		Pricing:             s.Pricing.DeepCopy(),
		MetricsAvailable:    s.MetricsAvailable,
		PrometheusAvailable: s.PrometheusAvailable,
	}

	out.Nodes = copySlice(s.Nodes, func(n NodeSnapshot) NodeSnapshot { return n.DeepCopy() })
	out.Pods = copySlice(s.Pods, func(p PodSnapshot) PodSnapshot { return p.DeepCopy() })
	out.Controllers = copySlice(s.Controllers, func(c ControllerSnapshot) ControllerSnapshot { return c.DeepCopy() })
	out.HPAs = copySlice(s.HPAs, func(h HPASnapshot) HPASnapshot { return h.DeepCopy() })
	out.PDBs = copySlice(s.PDBs, func(p PDBSnapshot) PDBSnapshot { return p.DeepCopy() })
	out.PVCs = copySlice(s.PVCs, func(p PVCSnapshot) PVCSnapshot { return p.DeepCopy() })
	out.UsageProfile = copyMap(s.UsageProfile)

	return out
}

// DeepCopy returns a fully independent copy of the NodeSnapshot.
func (n NodeSnapshot) DeepCopy() NodeSnapshot {
	out := n
	out.Labels = copyMap(n.Labels)
	out.Taints = copySliceSimple(n.Taints)
	out.Conditions = copySliceSimple(n.Conditions)
	return out
}

// DeepCopy returns a fully independent copy of the PodSnapshot.
func (p PodSnapshot) DeepCopy() PodSnapshot {
	out := p
	out.Containers = copySliceSimple(p.Containers)
	out.Labels = copyMap(p.Labels)
	out.Affinity = p.Affinity.DeepCopy()
	out.Tolerations = copySliceSimple(p.Tolerations)
	out.TopologySpreadConstraints = copySlice(p.TopologySpreadConstraints, func(t TopologySpreadConstraint) TopologySpreadConstraint {
		return t.DeepCopy()
	})
	out.PVCNames = copySliceSimple(p.PVCNames)
	return out
}

// DeepCopy returns a fully independent copy of the Affinity.
func (a *Affinity) DeepCopy() *Affinity {
	if a == nil {
		return nil
	}
	out := &Affinity{}
	out.NodeAffinity = a.NodeAffinity.DeepCopy()
	return out
}

// DeepCopy returns a fully independent copy of the NodeAffinity.
func (na *NodeAffinity) DeepCopy() *NodeAffinity {
	if na == nil {
		return nil
	}
	out := &NodeAffinity{}
	out.RequiredDuringSchedulingIgnoredDuringExecution = na.RequiredDuringSchedulingIgnoredDuringExecution.DeepCopy()
	return out
}

// DeepCopy returns a fully independent copy of the NodeSelector.
func (ns *NodeSelector) DeepCopy() *NodeSelector {
	if ns == nil {
		return nil
	}
	out := &NodeSelector{}
	out.NodeSelectorTerms = copySlice(ns.NodeSelectorTerms, func(t NodeSelectorTerm) NodeSelectorTerm {
		return t.DeepCopy()
	})
	return out
}

// DeepCopy returns a fully independent copy of the NodeSelectorTerm.
func (t NodeSelectorTerm) DeepCopy() NodeSelectorTerm {
	out := t
	out.MatchExpressions = copySlice(t.MatchExpressions, func(r NodeSelectorRequirement) NodeSelectorRequirement {
		return r.DeepCopy()
	})
	return out
}

// DeepCopy returns a fully independent copy of the NodeSelectorRequirement.
func (r NodeSelectorRequirement) DeepCopy() NodeSelectorRequirement {
	out := r
	out.Values = copySliceSimple(r.Values)
	return out
}

// DeepCopy returns a fully independent copy of the TopologySpreadConstraint.
func (t TopologySpreadConstraint) DeepCopy() TopologySpreadConstraint {
	out := t
	out.LabelSelector = copyMap(t.LabelSelector)
	return out
}

// DeepCopy returns a fully independent copy of the ControllerSnapshot.
func (c ControllerSnapshot) DeepCopy() ControllerSnapshot {
	return c // all value types, no slices or maps
}

// DeepCopy returns a fully independent copy of the HPASnapshot.
func (h HPASnapshot) DeepCopy() HPASnapshot {
	out := h
	out.MetricTargets = copySliceSimple(h.MetricTargets)
	return out
}

// DeepCopy returns a fully independent copy of the PDBSnapshot.
func (p PDBSnapshot) DeepCopy() PDBSnapshot {
	out := p
	out.SelectorLabels = copyMap(p.SelectorLabels)
	return out
}

// DeepCopy returns a fully independent copy of the PVCSnapshot.
func (p PVCSnapshot) DeepCopy() PVCSnapshot {
	out := p
	out.AccessModes = copySliceSimple(p.AccessModes)
	return out
}

// DeepCopy returns a fully independent copy of the PricingData.
func (p PricingData) DeepCopy() PricingData {
	out := p
	out.InstancePricing = copyMap(p.InstancePricing)
	return out
}

// copySlice copies a slice using a per-element copy function.
func copySlice[T any](src []T, copyFn func(T) T) []T {
	if src == nil {
		return nil
	}
	out := make([]T, len(src))
	for i, v := range src {
		out[i] = copyFn(v)
	}
	return out
}

// copySliceSimple copies a slice of value types (no nested references).
func copySliceSimple[T any](src []T) []T {
	if src == nil {
		return nil
	}
	out := make([]T, len(src))
	copy(out, src)
	return out
}

// copyMap copies a map with value types.
func copyMap[K comparable, V any](src map[K]V) map[K]V {
	if src == nil {
		return nil
	}
	out := make(map[K]V, len(src))
	maps.Copy(out, src)
	return out
}
