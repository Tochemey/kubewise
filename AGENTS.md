# agents.md — KubeWise

> Context file for AI coding agents (Claude Code, Cursor, Copilot, etc.) working on the KubeWise codebase. Read this before writing any code.

## What is this project?

KubeWise is a Kubernetes right-sizing tool that pulls real CPU/memory usage percentiles from Prometheus and recommends new resource requests with cost impact and OOM-risk classification. It is a CLI distributed as a kubectl plugin (`kubectl whatif`) and a GitHub Action that posts the cost delta as a PR comment.

Pipeline:

1. Snapshot the cluster (Kubernetes API + Prometheus percentiles)
2. Apply a scenario (`RightSize` or `Composite`) to a deep copy of the snapshot
3. Score OOM risk per workload from the percentile distribution
4. Compute cost from cloud pricing APIs (AWS/Azure/GCP) with offline fallback
5. Render to terminal table, JSON, or markdown (PR comment)

Written in Go, runs entirely client-side (no SaaS, no in-cluster agent), ships as a single binary.

## Repository layout

```
kubewise/
├── cmd/
│   └── kubectl-whatif/
│       └── main.go                 # CLI entry point — cobra root command
├── pkg/
│   ├── collector/                  # Reads cluster state + metrics
│   │   ├── snapshot.go             # Orchestrates full snapshot collection
│   │   ├── prometheus.go           # PromQL queries for usage percentiles
│   │   ├── metrics.go              # metrics-server fallback
│   │   └── types.go                # All snapshot struct definitions
│   ├── pricing/                    # Cloud provider cost data
│   │   ├── aws.go
│   │   ├── gcp.go
│   │   ├── azure.go
│   │   ├── cache.go                # Local file cache with TTL
│   │   └── fallback.go             # YAML-based manual pricing input
│   ├── scenario/                   # Mutations applied to snapshot copies
│   │   ├── engine.go               # Deep-copy + apply logic; Composite
│   │   ├── rightsize.go            # Adjusts requests/limits from percentiles
│   │   ├── scope.go                # Namespace/label scope filtering
│   │   └── parser.go               # YAML scenario file parser
│   ├── risk/                       # Risk scoring
│   │   ├── oom.go                  # OOM probability from usage percentiles
│   │   ├── scheduling.go           # Unschedulable pod detection
│   │   └── aggregate.go            # Cluster-wide rollup + classification
│   └── output/                     # Rendering
│       ├── table.go                # Terminal tables (lipgloss)
│       ├── json.go
│       └── markdown.go             # CI/CD PR comment format
├── scenarios/                      # Example scenario YAML files
├── action/                         # GitHub Action wrapper
├── docs/
├── internal/                       # Shared internal utilities
│   ├── units/                      # CPU/memory unit conversion helpers
│   └── kube/                       # Kubeconfig + client-go helpers
├── testdata/                       # Fixture snapshots for tests
├── go.mod
├── go.sum
├── .golangci.yml               # Linter config (goheader, etc.)
├── Makefile
└── README.md
```

## Architecture principles

### Pure functions over side effects

The pipeline is a chain of pure transformations:

```
Collect(cluster) → Snapshot
DeepCopy(snapshot) → mutable copy
Apply(scenario, copy) → mutated snapshot
Score(mutated) → RiskReport
```

Only the collector, pricing, and output layers have side effects (API calls, file I/O). Everything in between operates on in-memory structs.

### Snapshot is the single source of truth

Every module operates on `ClusterSnapshot` or its sub-structs. Never call the Kubernetes API or Prometheus mid-pipeline. All data must be captured upfront during the collect phase. If you find yourself wanting to make an API call inside `pkg/scenario/` or `pkg/risk/`, you're doing it wrong — add the data to the snapshot instead.

### Scenarios are pure mutations

A scenario takes a snapshot copy and returns a mutated version. It must never:
- Read from the Kubernetes API
- Write to disk
- Modify the original snapshot (always deep-copy first)
- Call other scenarios implicitly (composition is handled by the engine)

## Go conventions

### Module boundaries

Each package under `pkg/` has a single public entry point function that orchestrates the module's work. Internal helpers are unexported. Cross-package dependencies flow downward only:

```
cmd/ → pkg/collector, pkg/scenario, pkg/pricing, pkg/risk, pkg/output
pkg/scenario → pkg/collector/types (snapshot structs only, not collection functions)
pkg/risk → pkg/collector/types
pkg/output → pkg/risk (report structs)
```

Never import upward. Never import between sibling packages at the same level.

### Error handling

Use `fmt.Errorf` with `%w` for wrapping. Every public function returns `error` as the last return value. Use sentinel errors for conditions callers need to check:

```go
var (
    ErrNoMetricsServer  = errors.New("metrics-server not available")
    ErrNoPricing        = errors.New("pricing data unavailable for instance type")
    ErrUnschedulable    = errors.New("pod cannot be scheduled on any node")
)
```

Never `log.Fatal` or `os.Exit` inside `pkg/`. Only `cmd/` may exit. Packages return errors; the CLI decides how to present them.

### Logging

We use `k8s.io/klog/v2` — the standard structured logger in the Kubernetes ecosystem. This is a hard requirement; do not use `log`, `log/slog`, `fmt.Println`, `logr`, `zap`, or any other logging library.

**Initialization:** klog is initialized once in `cmd/kubectl-whatif/main.go` via cobra's `PersistentPreRun`:

```go
import "k8s.io/klog/v2"

func init() {
    // Bind klog flags to cobra (--v, --vmodule, --logtostderr, etc.)
    klog.InitFlags(nil)
}

// In the root command's PersistentPreRunE:
func setupLogging(cmd *cobra.Command, args []string) error {
    // Flush buffered logs on exit
    defer klog.Flush()
    return nil
}
```

**Verbosity levels:** Use klog's V-level system consistently across the codebase:

| Level                      | Use for                         | Example                                                   |
|----------------------------|---------------------------------|-----------------------------------------------------------|
| `klog.V(0)` / `klog.InfoS` | Always-visible operational info | "Snapshot collected", "Simulation complete"               |
| `klog.V(1)`                | High-level progress             | "Collecting nodes", "Querying Prometheus"                 |
| `klog.V(2)`                | Per-resource detail             | "Processing pod api-gateway/web-7b4f9"                    |
| `klog.V(3)`                | Internal decisions              | "Pod scored 82 on node-pool-a-3"                          |
| `klog.V(4)`                | Trace-level debug               | "Predicate FitsResources: 200m available, 150m requested" |

**Structured logging only.** Always use the `S`-suffixed functions (`InfoS`, `ErrorS`, `V(n).InfoS`) with key-value pairs. Never use the unstructured variants (`Info`, `Infof`, `Error`, `Errorf`):

```go
// CORRECT — structured with key-value pairs
klog.InfoS("Snapshot collected", "nodes", len(snap.Nodes), "pods", len(snap.Pods))
klog.V(2).InfoS("Processing pod", "namespace", pod.Namespace, "name", pod.Name)
klog.ErrorS(err, "Failed to query Prometheus", "endpoint", promURL)

// WRONG — unstructured
klog.Infof("Collected %d nodes and %d pods", len(snap.Nodes), len(snap.Pods))
klog.Error("something failed: ", err)
```

**Key naming conventions:**
- Use camelCase for keys: `"podName"`, `"nodeCount"`, `"costDelta"`
- Use `"err"` only via `ErrorS` (it handles the error argument automatically)
- Namespace and name should be separate keys, not combined: `"namespace", "default", "name", "web-7b4f9"` — not `"pod", "default/web-7b4f9"`

**No logging in pure functions.** The scenario engine (`pkg/scenario/`) and risk scorer (`pkg/risk/`) should not log. They are pure transformations. If you need observability into these modules, return structured metadata in the result type. Logging belongs in the orchestration layer (`cmd/`) and the I/O layer (`pkg/collector/`, `pkg/pricing/`, `pkg/output/`).

**Testing:** In tests, call `klog.SetOutput(io.Discard)` or use `ktesting.NewLogger` from `k8s.io/klog/v2/ktesting` to capture and assert log output.

### License headers

**Every `.go` file must begin with the project license header.** This is enforced by the `goheader` linter in `.golangci.yml`. Files without the header will fail CI.

The header template is defined in `.golangci.yml`:

```yaml
# .golangci.yml (relevant section)
linters:
  enable:
    - goheader

linters-settings:
  goheader:
    template: |-
      Copyright 2026 Arsene Tochemey Gandote

      Licensed under the Apache License, Version 2.0 (the "License");
      you may not use this file except in compliance with the License.
      You may obtain a copy of the License at

          http://www.apache.org/licenses/LICENSE-2.0

      Unless required by applicable law or agreed to in writing, software
      distributed under the License is distributed on an "AS IS" BASIS,
      WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
      See the License for the specific language governing permissions and
      limitations under the License.
```

**What this means for every `.go` file you create or modify:**

```go
// Copyright 2026 KubeWise Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
    // ...
)
```

**Rules:**
- The header must be the very first lines of the file, before the `package` declaration
- Use the current year at time of file creation. Do not update the year on subsequent edits to the same file.
- The `YEAR` field in the golangci template is matched dynamically — golangci-lint's goheader linter accepts any valid year, so `2025`, `2026`, etc. all pass
- Use exactly `//` comment style (not `/* */` block comments)
- One blank line between the header and the `package` declaration
- Test files (`*_test.go`) also require the header
- Generated files (e.g., from `deepcopy-gen`) should include the header in the generator's boilerplate config

**If you forget:** `golangci-lint run` will report `goheader` violations. The CI pipeline runs this on every PR. Fix it before pushing.

**Adding the header to an existing file that's missing it:**

```go
// Insert these two lines at the very top of the file:
// Copyright 2026 KubeWise Authors
// SPDX-License-Identifier: Apache-2.0
```

### Naming

- Snapshot structs: `NodeSnapshot`, `PodSnapshot`, `ContainerUsageProfile`
- Scenario types: `RightSizeScenario`, `CompositeScenario`
- Result types: `RiskReport`, `output.Report`
- Functions: verb-first (`CollectSnapshot`, `ApplyScenario`, `ScoreOOMRisk`)
- Test files: `*_test.go` in the same package
- Fixture files: `testdata/` at repo root, loaded via `os.ReadFile` in tests

### Resource units

Kubernetes uses millicores for CPU and bytes for memory. All internal representations use these raw units. Conversion to human-readable formats (e.g., "500m", "2Gi") happens only in `pkg/output/`. Use the helpers in `internal/units/`:

```go
// internal/units/cpu.go
func MillicoresToCores(m int64) float64
func ParseCPU(s string) (int64, error)       // "500m" → 500, "2" → 2000

// internal/units/memory.go  
func BytesToGiB(b int64) float64
func ParseMemory(s string) (int64, error)     // "512Mi" → 536870912, "2Gi" → 2147483648
```

Never pass floats for CPU/memory between packages. Floats are for display only.

## Module implementation guide

### pkg/collector — Cluster snapshot collection

**Key dependency:** `k8s.io/client-go`. Use the discovery client to detect API server capabilities (e.g., whether metrics-server is installed).

**Snapshot collection order:**
1. Nodes (allocatable resources, labels, taints)
2. Pods (specs, owner references, scheduling constraints)
3. Controllers (replica counts, update strategies)
4. HPAs and PDBs
5. PVCs (for topology-aware storage constraints)
6. Actual usage from metrics-server (`/apis/metrics.k8s.io/v1beta1`)
7. Historical usage from Prometheus (optional, graceful fallback)

**Prometheus queries for percentiles:**

```promql
# CPU percentile over 14 days, per container
quantile_over_time(0.95,
  rate(container_cpu_usage_seconds_total{
    namespace="$ns", pod="$pod", container="$container"
  }[5m])
[14d:5m])

# Memory percentile over 14 days, per container
quantile_over_time(0.95,
  container_memory_working_set_bytes{
    namespace="$ns", pod="$pod", container="$container"
  }
[14d:5m])
```

**Critical implementation details:**
- Use `informers` for bulk list operations (faster than sequential `Get` calls)
- Prometheus endpoint is auto-discovered via the `monitoring.coreos.com` service or configurable via `--prometheus-url`
- If Prometheus returns partial data (some containers missing), log warnings but don't fail — merge what's available
- Set a timeout of 30s on Prometheus queries (large clusters have slow range queries)
- Snapshot serialization: use `encoding/json` with struct tags. Support `--save` flag to dump snapshot to file for debugging

### pkg/pricing — Cloud pricing data

**Detection order:**
1. Check node labels for cloud provider (`node.kubernetes.io/instance-type`, `topology.kubernetes.io/zone`)
2. If AWS: use EC2 Pricing API via `aws-sdk-go-v2`
3. If GCP: use Cloud Billing Catalog via REST
4. If Azure: use Retail Prices REST API
5. If undetected or API fails: check `~/.kubewise/pricing/` cache
6. Last resort: prompt for YAML pricing file

**Caching:** Store pricing data in `~/.kubewise/pricing/{provider}_{region}.json` with a 24-hour TTL. Check file mtime before fetching. This avoids hitting rate limits and makes offline usage possible after the first run.

**Important: GCP prices CPU and memory separately.** A node's hourly cost on GCP is `(vCPUs × predefined_cpu_hourly) + (GB_memory × predefined_ram_hourly)`. AWS and Azure price by instance type (single hourly rate). The `PricingData` interface must abstract this difference.

```go
type PricingProvider interface {
    HourlyCost(instanceType string, region string, spot bool) (float64, error)
}
```

### pkg/scenario — Scenario mutations

**Deep copy is critical.** Use a custom deep-copy function, not `encoding/json` round-trip (too slow for large snapshots). Generate deep-copy methods with `deepcopy-gen` or write them manually for each snapshot struct. Every scenario MUST operate on a copy, never the original.

**Right-sizing mutation walkthrough:**

```go
func (s *RightSizeScenario) Apply(snap *ClusterSnapshot) (*ClusterSnapshot, error) {
    mutated := DeepCopy(snap)
    
    for i, pod := range mutated.Pods {
        if !s.InScope(pod) {
            continue
        }
        for j, container := range pod.Containers {
            profile, ok := mutated.UsageProfile[profileKey(pod, container)]
            if !ok {
                continue // no usage data, skip (don't guess)
            }
            
            targetCPU := percentileValue(profile, s.Percentile, "cpu")
            targetMem := percentileValue(profile, s.Percentile, "memory")
            
            // Apply buffer
            newRequestCPU := int64(float64(targetCPU) * (1.0 + float64(s.Buffer)/100.0))
            newRequestMem := int64(float64(targetMem) * (1.0 + float64(s.Buffer)/100.0))
            
            // Enforce minimums
            newRequestCPU = max(newRequestCPU, 10)      // 10m floor
            newRequestMem = max(newRequestMem, 33554432) // 32Mi floor
            
            // Apply limits based on strategy
            mutated.Pods[i].Containers[j].Requests.CPU = newRequestCPU
            mutated.Pods[i].Containers[j].Requests.Memory = newRequestMem
            mutated.Pods[i].Containers[j].Limits = computeLimits(
                container, newRequestCPU, newRequestMem, s.LimitStrategy,
            )
        }
    }
    return mutated, nil
}
```

**Scope filtering pattern (reused across all scenarios):**

```go
type Scope struct {
    Namespaces        []string          // ["*"] means all
    ExcludeNamespaces []string
    ExcludeLabels     map[string]string // e.g., {"kubewise.io/skip": "true"}
}

func (s Scope) Includes(pod PodSnapshot) bool {
    // Check excludes first (they take priority)
    for _, ns := range s.ExcludeNamespaces {
        if pod.Namespace == ns { return false }
    }
    for k, v := range s.ExcludeLabels {
        if pod.Labels[k] == v { return false }
    }
    // Check includes
    if len(s.Namespaces) == 1 && s.Namespaces[0] == "*" {
        return true
    }
    return slices.Contains(s.Namespaces, pod.Namespace)
}
```

### pkg/risk — Risk scoring

**OOM risk calculation:**

```go
func OOMRisk(pod PodSnapshot, profile ContainerUsageProfile, newLimit int64) float64 {
    if profile.DataPoints < 100 {
        return -1 // insufficient data, return sentinel
    }
    // Use the histogram: what fraction of samples exceed the new limit?
    // If we only have percentiles (not full histogram), interpolate:
    // If p99 < newLimit → risk ≈ <1%
    // If p95 < newLimit < p99 → risk ≈ 1-5%
    // If p90 < newLimit < p95 → risk ≈ 5-10%
    // etc.
    // For MVP, this interpolation is sufficient.
    // Phase 2: query actual histogram buckets from Prometheus.
}
```

**Risk classification thresholds are defined as constants, not magic numbers:**

```go
const (
    OOMRiskLowThreshold      = 0.01  // 1%
    OOMRiskModerateThreshold  = 0.05  // 5%
    EvictionRiskLowThreshold  = 0.001 // 0.1%
    EvictionRiskModThreshold  = 0.01  // 1%
    SchedulingRiskLowThreshold = 0.0
    SchedulingRiskModThreshold = 0.01 // 1%
)
```

### pkg/output — CLI rendering

**Terminal output uses lipgloss for styling.** Keep styles defined centrally:

```go
var (
    headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
    greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
    amberStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
    redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)
```

**JSON output must be stable.** Sort keys alphabetically. Use `json.MarshalIndent` with 2-space indent. This ensures `diff` works for comparing consecutive runs.

**Markdown output is designed for GitHub PR comments.** Use collapsible `<details>` sections for per-namespace breakdowns so the comment stays compact.

## Testing expectations

### Every module must have tests

- `pkg/collector`: Mock the Kubernetes API with `fake.NewSimpleClientset`. Test that snapshot structs are populated correctly from API responses.
- `pkg/pricing`: Use HTTP test servers (`httptest.NewServer`) to mock cloud pricing APIs. Test cache TTL logic. Test YAML fallback parsing.
- `pkg/scenario`: Create fixture snapshots in `testdata/`, apply scenarios, assert mutations. Test scope filtering edge cases (empty namespace list, wildcard, exclude overrides include).
- `pkg/risk`: Test boundary conditions around thresholds. Test with insufficient data (should return unknown, not zero). Test aggregate rollup math.

### Test fixture format

Store fixtures as JSON files in `testdata/`:

```
testdata/
├── snapshots/
│   ├── small-cluster.json      # 3 nodes, 10 pods — fast unit tests
│   └── medium-cluster.json     # 20 nodes, 200 pods — realistic
├── scenarios/
│   └── rightsize-p95.yaml
└── expected/
    └── rightsize-p95-result.json
```

Load fixtures with a helper:

```go
func loadFixture[T any](t *testing.T, path string) T {
    t.Helper()
    data, err := os.ReadFile(filepath.Join("testdata", path))
    require.NoError(t, err)
    var v T
    require.NoError(t, json.Unmarshal(data, &v))
    return v
}
```

### Integration tests

Use `kind` (Kubernetes in Docker) for integration tests. The `Makefile` should have:

```makefile
test-unit:
	go test ./pkg/... -short -count=1

test-integration:
	kind create cluster --name kubewise-test
	kubectl apply -f testdata/manifests/
	go test ./pkg/... -run Integration -count=1
	kind delete cluster --name kubewise-test

lint:
	golangci-lint run ./...

test-all: lint test-unit test-integration
```

## Common pitfalls

### Don't confuse requests with limits

Kubernetes scheduling uses *requests* (guaranteed resources), not *limits* (max burst). The bin-packer must pack based on requests. Limits are only relevant for OOM risk scoring.

### Don't forget system pods

`kube-system` pods (kube-proxy, CoreDNS, CNI agents, etc.) consume node resources. The snapshot must include them, and the bin-packer must account for them even if they're excluded from scenario mutations.

### DaemonSets are not normal pods

DaemonSets run on every matching node automatically. When simulating node pool changes, you must recalculate which DaemonSets would run on the new nodes and reserve their resources *before* bin-packing other pods.

### Allocatable != Capacity

Node capacity is the total hardware resource. Allocatable is capacity minus system reserved (kubelet, OS, eviction thresholds). Always use allocatable for scheduling math.

### Metrics-server gives point-in-time, not percentiles

Metrics-server returns *current* CPU/memory usage. It has no history. If Prometheus is unavailable, right-sizing must use wider safety buffers (e.g., 50% instead of 20%) because you're basing decisions on a single data point. Flag this in the output: "Confidence: low (no historical data)".

### GCP pricing is not per-instance

AWS and Azure price by instance type (single hourly rate). GCP prices CPU and memory separately (predefined or custom). The pricing abstraction must handle this difference behind a common interface.

### Deep copy is not optional

Go slices and maps are reference types. A naive struct copy shares underlying arrays. If a scenario modifies `mutated.Pods[0].Containers[0].Requests.CPU`, it will also modify the original snapshot unless you deep-copy all nested slices and maps. This is the #1 source of simulation bugs. Test it explicitly: modify a copy and assert the original is unchanged.

### Don't use the wrong logger

The codebase uses `k8s.io/klog/v2` exclusively. Do not introduce `log`, `log/slog`, `fmt.Println`, `logr`, or `zap`. client-go itself uses klog internally — using a different logger causes split log streams, inconsistent formatting, and broken `--v` verbosity flags. If you see a file using `log.Printf`, that's a bug — replace it with `klog.V(n).InfoS`.

### Don't forget the license header

Every `.go` file must start with the copyright header before the `package` line. The `goheader` linter in `.golangci.yml` enforces this and CI will reject PRs with missing headers. This includes test files, generated files, and one-off scripts under `cmd/`. The most common mistake is creating a new file and jumping straight to `package foo` — always add the header first.

## Scenario YAML schema

Two scenario kinds are supported:

```yaml
apiVersion: kubewise.io/v1alpha1
kind: RightSize | Composite
metadata:
  name: my-scenario
  description: "Human-readable description shown in output"
spec:
  # kind-specific fields (see scenarios/ for examples)
```

For `Composite` scenarios (chaining multiple `RightSize` passes):

```yaml
apiVersion: kubewise.io/v1alpha1
kind: Composite
metadata:
  name: layered-rightsize
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
        scope:
          namespaces: ["payments"]
```

Composite scenarios apply mutations sequentially — each step receives the output of the previous step.

## CLI flags reference

Global flags (inherited by all subcommands):

```
--kubeconfig string     Path to kubeconfig (default: $KUBECONFIG or ~/.kube/config)
--context string        Kubernetes context to use
--namespace string      Limit to a specific namespace (default: all)
--prometheus-url string Prometheus endpoint (auto-discovered if not set)
--output string         Output format: table, json, markdown (default: table)
--verbose               Show detailed per-workload breakdown
--no-color              Disable terminal colors
```

## Dependencies (go.mod)

```
k8s.io/client-go           # Kubernetes API client
k8s.io/apimachinery         # K8s type definitions
k8s.io/metrics              # metrics-server API types
k8s.io/klog/v2              # Structured logging (K8s ecosystem standard)
github.com/spf13/cobra      # CLI framework
github.com/charmbracelet/lipgloss  # Terminal styling
github.com/prometheus/client_golang # Prometheus HTTP client
github.com/aws/aws-sdk-go-v2       # AWS pricing (only for AWS clusters)
github.com/stretchr/testify        # Test assertions
```

Keep dependencies minimal. Do not add a dependency for something achievable in <50 lines of Go. In particular:
- No ORM or database — we read from K8s API and Prometheus, not a DB
- No web framework — this is a CLI tool, not a server (yet)
- No configuration library beyond cobra — flags + YAML scenario files are sufficient
- No alternative logging library — use `k8s.io/klog/v2` exclusively (see Logging section below)

## Code review checklist

Before submitting any PR, verify:

- [ ] Every `.go` file (including tests) starts with the license header: `// Copyright <YEAR> KubeWise Authors` + `// SPDX-License-Identifier: Apache-2.0`
- [ ] All logging uses `klog.InfoS` / `klog.ErrorS` / `klog.V(n).InfoS` — no `fmt.Println`, `log.*`, or unstructured `klog.Infof`
- [ ] No logging inside `pkg/scenario/` or `pkg/risk/` (pure functions — return metadata, don't log)
- [ ] No API calls inside `pkg/scenario/` or `pkg/risk/`
- [ ] All resource calculations use `int64` millicores/bytes, not floats
- [ ] Scenario mutations operate on a deep copy, not the original
- [ ] New predicates have individual unit tests with edge cases
- [ ] Fixture snapshots in `testdata/` cover the new code path
- [ ] `--output=json` format is stable (sorted keys, no non-deterministic fields)
- [ ] Error messages include the namespace/pod/container name for debuggability
- [ ] No `log.Fatal`, `klog.Fatal`, or `os.Exit` outside of `cmd/`
- [ ] Functions are < 80 lines; extract helpers if longer
- [ ] `golangci-lint run` passes clean (includes goheader check)