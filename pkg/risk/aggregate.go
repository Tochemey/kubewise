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

package risk

import (
	"fmt"

	"github.com/tochemey/kubewise/pkg/collector"
)

// Risk classification thresholds.
const (
	OOMRiskLowThreshold        = 0.01  // 1%
	OOMRiskModerateThreshold   = 0.05  // 5%
	EvictionRiskLowThreshold   = 0.001 // 0.1%
	EvictionRiskModThreshold   = 0.01  // 1%
	SchedulingRiskLowThreshold = 0.0
	SchedulingRiskModThreshold = 0.01 // 1%
)

// RiskLevel classifies risk severity.
type RiskLevel int

const (
	// RiskGreen indicates low risk.
	RiskGreen RiskLevel = iota
	// RiskAmber indicates moderate risk.
	RiskAmber
	// RiskRed indicates high risk.
	RiskRed
	// RiskUnknown indicates insufficient data to assess risk.
	RiskUnknown
)

// String returns the human-readable name of the risk level.
func (r RiskLevel) String() string {
	switch r {
	case RiskGreen:
		return "low"
	case RiskAmber:
		return "moderate"
	case RiskRed:
		return "high"
	case RiskUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// RiskReport holds the complete risk assessment for a simulation.
type RiskReport struct {
	// PerWorkload maps "namespace/name" to the workload's risk assessment.
	PerWorkload map[string]WorkloadRisk
	// ClusterOOM is the worst-case workload OOM risk across the cluster.
	ClusterOOM float64
	// ClusterEviction is the aggregate eviction risk (populated by spot scenarios).
	ClusterEviction float64
	// SchedulingRisk is the fraction of unschedulable pods (populated by simulator).
	SchedulingRisk float64
	// OverallLevel is the worst risk level across all dimensions.
	OverallLevel RiskLevel
}

// WorkloadRisk holds risk information for a single workload.
type WorkloadRisk struct {
	// Name is the workload name.
	Name string
	// Namespace is the workload namespace.
	Namespace string
	// OOMRisk is the probability of any pod in this workload being OOM-killed.
	OOMRisk float64
	// EvictionRisk is the probability of spot eviction (placeholder).
	EvictionRisk float64
	// Level is the overall risk level for this workload.
	Level RiskLevel
}

// ScoreOOMRisk computes the OOM risk report for right-sizing scenarios.
// It takes the mutated snapshot (with new limits) and computes per-workload
// and cluster-wide OOM risk.
func ScoreOOMRisk(snap *collector.ClusterSnapshot) *RiskReport {
	report := &RiskReport{
		PerWorkload: make(map[string]WorkloadRisk),
	}

	// Group containers by workload (owner reference)
	workloads := groupByWorkload(snap)

	for key, containers := range workloads {
		wr := WorkloadRisk{
			Name:      containers[0].ownerName,
			Namespace: containers[0].namespace,
		}

		hasData := false
		// Workload OOM risk = 1 - product(1 - container_oom_probability)
		survivalProb := 1.0
		for _, c := range containers {
			profile, ok := snap.UsageProfile[c.profileKey]
			if !ok {
				continue
			}
			oomRisk := OOMRisk(profile, c.limitMemory)
			if oomRisk < 0 {
				continue // insufficient data
			}
			hasData = true
			survivalProb *= (1 - oomRisk)
		}

		if !hasData {
			wr.OOMRisk = -1.0
			wr.Level = RiskUnknown
		} else {
			wr.OOMRisk = 1 - survivalProb
			wr.Level = classifyOOM(wr.OOMRisk)
		}

		report.PerWorkload[key] = wr

		// Track worst-case for cluster-wide metric
		if wr.OOMRisk > report.ClusterOOM {
			report.ClusterOOM = wr.OOMRisk
		}
	}

	report.OverallLevel = ClassifyRisk(report.ClusterOOM, report.ClusterEviction, report.SchedulingRisk)
	return report
}

// ClassifyRisk returns the worst risk level across all dimensions.
func ClassifyRisk(oom, eviction, scheduling float64) RiskLevel {
	worst := RiskGreen

	oomLevel := classifyOOM(oom)
	if oomLevel > worst {
		worst = oomLevel
	}

	evictionLevel := classifyEviction(eviction)
	if evictionLevel > worst {
		worst = evictionLevel
	}

	schedulingLevel := classifyScheduling(scheduling)
	if schedulingLevel > worst {
		worst = schedulingLevel
	}

	return worst
}

func classifyOOM(risk float64) RiskLevel {
	if risk < 0 {
		return RiskUnknown
	}
	switch {
	case risk < OOMRiskLowThreshold:
		return RiskGreen
	case risk < OOMRiskModerateThreshold:
		return RiskAmber
	default:
		return RiskRed
	}
}

func classifyEviction(risk float64) RiskLevel {
	if risk < 0 {
		return RiskUnknown
	}
	switch {
	case risk <= EvictionRiskLowThreshold:
		return RiskGreen
	case risk < EvictionRiskModThreshold:
		return RiskAmber
	default:
		return RiskRed
	}
}

func classifyScheduling(risk float64) RiskLevel {
	if risk < 0 {
		return RiskUnknown
	}
	switch {
	case risk <= SchedulingRiskLowThreshold:
		return RiskGreen
	case risk < SchedulingRiskModThreshold:
		return RiskAmber
	default:
		return RiskRed
	}
}

// containerInfo holds the info needed to compute risk for a single container.
type containerInfo struct {
	namespace   string
	ownerName   string
	profileKey  string
	limitMemory int64
}

// groupByWorkload groups containers by their owner workload.
func groupByWorkload(snap *collector.ClusterSnapshot) map[string][]containerInfo {
	workloads := make(map[string][]containerInfo)
	for _, pod := range snap.Pods {
		ownerName := pod.OwnerRef.Name
		if ownerName == "" {
			ownerName = pod.Name
		}
		key := fmt.Sprintf("%s/%s", pod.Namespace, ownerName)

		for _, container := range pod.Containers {
			workloads[key] = append(workloads[key], containerInfo{
				namespace:   pod.Namespace,
				ownerName:   ownerName,
				profileKey:  collector.ProfileKey(pod.Namespace, pod.Name, container.Name),
				limitMemory: container.Limits.Memory,
			})
		}
	}
	return workloads
}
