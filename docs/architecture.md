# Architecture

## Data flow

```
                         ┌──────────────┐
                         │  Kubernetes  │
                         │   API Server │
                         └──────┬───────┘
                                │
                         ┌──────▼───────┐
                         │  Collector   │  pkg/collector
                         │  (snapshot)  │
                         └──────┬───────┘
                                │
                    ┌───────────▼───────────┐
                    │   ClusterSnapshot     │  pkg/collector/types
                    │   (in-memory struct)  │
                    └───────────┬───────────┘
                                │
                    ┌───────────▼───────────┐
                    │   Scenario Engine     │  pkg/scenario
                    │   (deep copy + mutate)│
                    └───────────┬───────────┘
                                │
                    ┌───────────▼───────────┐
                    │   Risk Scorer         │  pkg/risk
                    │   (OOM)               │
                    └───────────┬───────────┘
                                │
                    ┌───────────▼───────────┐
                    │   Pricing             │  pkg/pricing
                    │   (cloud rate cards)  │
                    └───────────┬───────────┘
                                │
                    ┌───────────▼───────────┐
                    │   Output Renderer     │  pkg/output
                    │   (table/JSON/MD)     │
                    └───────────────────────┘
```

## Module responsibilities

### pkg/collector

Reads the full schedulable state of the cluster. Produces a typed `ClusterSnapshot` struct that every other module operates on.

- **snapshot.go**: Orchestrates collection from Kubernetes API (nodes, pods, controllers, HPAs, PDBs, PVCs)
- **prometheus.go**: Queries Prometheus for historical usage percentiles (P50/P90/P95/P99)
- **types.go**: All snapshot struct definitions (the data contract)
- **deepcopy.go**: Explicit deep copy methods for all types

### pkg/pricing

Maps instance types to hourly costs. Supports AWS, GCP, and Azure.

- Auto-detects cloud provider from node labels
- Caches pricing in `~/.kubewise/pricing/` with 24-hour TTL
- Falls back to user-provided YAML pricing file
- GCP prices CPU and memory separately; AWS and Azure price by instance type

### pkg/scenario

Pure mutations applied to snapshot copies.

- **engine.go**: Deep copies the snapshot, applies the scenario, returns the mutated copy
- **rightsize.go**: Adjusts requests/limits based on usage percentiles
- **parser.go**: Parses scenario YAML files (RightSize, Composite)

### pkg/risk

Risk scoring for mutated snapshots.

- **oom.go**: OOM probability from usage percentiles vs new memory limits
- **scheduling.go**: Fraction of unschedulable pods
- **aggregate.go**: Cluster-wide rollup with green/amber/red classification

### pkg/output

Renders reports to terminal, JSON, or Markdown.

- **table.go**: Rich terminal output with lipgloss styling
- **json.go**: Stable, indented JSON with sorted keys
- **markdown.go**: GitHub PR comment format with collapsible sections

## Risk scoring

### OOM risk

Estimated from usage percentiles vs new memory limits using linear interpolation:

| Condition             | Estimated risk |
|-----------------------|----------------|
| newLimit > P99        | < 1% (green)   |
| P95 < newLimit <= P99 | 1-5% (amber)   |
| P90 < newLimit <= P95 | 5-10% (red)    |
| P50 < newLimit <= P90 | 10-50% (red)   |
| newLimit <= P50       | 50%+ (red)     |

Per-workload risk: `1 - product(1 - container_risk)` across all containers.

### Scheduling risk

- Fraction of pods that cannot be placed: `unschedulable / total`
- Green: 0%, Amber: < 1%, Red: >= 1%

## Pricing data

| Provider | Source                 | Pricing model         |
|----------|------------------------|-----------------------|
| AWS      | EC2 Pricing API (Bulk) | Per instance type     |
| GCP      | Cloud Billing Catalog  | Per vCPU + per GB RAM |
| Azure    | Retail Prices API      | Per instance type     |

Pricing is cached locally in `~/.kubewise/pricing/{provider}_{region}.json` with a 24-hour TTL. If cloud APIs are unreachable, users can provide a YAML pricing file with `--pricing-file`.
