<h2 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.png">
    <source media="(prefers-color-scheme: light)" srcset="assets/logo.png">
    <img alt="KubeWise" src="assets/logo.png" width="420">
  </picture>
</h2>

<p align="center">
  <strong>Kubernetes cost and performance "what-if" simulator.</strong><br>
  Snapshot your cluster, simulate changes, and see the cost impact before making them.
</p>

<p align="center">
  <a href="https://github.com/tochemey/kubewise/actions/workflows/ci.yml"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/tochemey/kubewise/ci.yml?style=flat-square" ></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue?style=flat-square" alt="License"></a>
  <a href="https://goreportcard.com/report/github.com/tochemey/kubewise"><img src="https://goreportcard.com/badge/github.com/tochemey/kubewise" alt="Go Report Card"></a>
</p>

---

## 😵 The Problem

Kubernetes billing is a black box. Teams overprovision because the cost of an outage outweighs the cost of waste — and nobody has time to model the tradeoffs. Cloud cost tools tell you *what you're spending*, but they don't answer **"what would happen if I changed X?"** You're flying blind every time you touch resource requests, node pools, or scheduling policies.

## 💡 The Solution

KubeWise is a Kubernetes cost × performance "what-if" simulator. It snapshots your live cluster, lets you define hypothetical changes, simulates the Kubernetes scheduler against the modified state, and reports cost savings alongside reliability risk — all without touching your cluster.

**The core loop: snapshot → mutate → simulate → compare.**

1. **📸 Snapshot the cluster.** Read everything that matters: pod resource usage, node capacities, autoscaler configs, PDB rules, affinity constraints, and current cloud pricing. This becomes the baseline with a real dollar cost attached.

2. **✏️ Define a scenario.** Describe a hypothetical change:
   - *"Right-size every pod's requests to p95 actual usage + 20% buffer"*
   - *"Move all stateless services to spot instances"*
   - *"Consolidate from 3 node pools to 2 using larger instance types"*
   - *"What if traffic doubles next month?"*

3. **⚙️ Simulate.** Replay the Kubernetes scheduler's bin-packing algorithm against the modified scenario. Run autoscaler logic. Check affinity rules, PDBs, topology constraints. Model spot interruption probability.

4. **📊 Compare and score.** Side-by-side: baseline vs. scenario. Cost delta, reliability delta, and confidence intervals. Not a single number — a distribution.

## 🎯 Differentiator

Existing tools either show you the past (Kubecost) or act on the future without you (CAST AI). Nobody lets you **explore the future yourself**. KubeWise sits in the gap — **high insight AND high user agency**.

|                            | Observability tools | Autopilots | **KubeWise** |
|----------------------------|---------------------|------------|--------------|
| Shows cost data            | ✅                   | ✅          | ✅            |
| Recommends changes         | ❌                   | ✅          | ✅            |
| Simulates before acting    | ❌                   | ❌          | ✅            |
| User controls the decision | ✅                   | ❌          | ✅            |
| Runs client-side (no SaaS) | ❌                   | ❌          | ✅            |

## ✨ Features

- **📉 Right-size workloads** based on actual P50/P90/P95/P99 usage with configurable safety buffers
- **🔗 Simulate node consolidation** to find the minimum number of nodes needed
- **💰 Estimate spot savings** with eviction risk assessment per workload
- **🤖 CI/CD integration** via GitHub Action that posts cost impact as PR comments
- **🔒 Runs entirely client-side** — no SaaS dependency, your cluster data never leaves your machine

## 🚀 Quick Start

### Install

```bash
# Via krew
kubectl krew install whatif

# Via Go
go install github.com/tochemey/kubewise/cmd/kubectl-whatif@latest
```

### Global Flags

These flags apply to all commands:

| Flag | Default | Description |
|------|---------|-------------|
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig file |
| `--context` | current context | Kubernetes context to use |
| `--namespace` | all | Limit to specific namespace |
| `--prometheus-url` | auto-detect | Prometheus endpoint for historical metrics |
| `--output`, `-o` | `table` | Output format: `table`, `json`, `markdown` |
| `--verbose` | `false` | Show detailed per-workload breakdown |
| `--no-color` | `false` | Disable terminal colors |

## 📸 Snapshot — See Current Cost Breakdown

Snapshot captures the live cluster state and displays the current cost breakdown per namespace using a stacked-panel layout.

```bash
# Basic snapshot
kubectl whatif snapshot

# Save snapshot to file for later use
kubectl whatif snapshot --save=cluster-snapshot.json

# JSON output for scripting
kubectl whatif snapshot --output=json

# Limit to a single namespace
kubectl whatif snapshot --namespace=api
```

Example output (each namespace is displayed as a separate panel):

```
KubeWise: Snapshot of current cluster cost breakdown

  Total monthly cost:    $14230
  Cluster OOM risk:      0.8%  low
  Overall risk:          low
  Namespaces:            3

--------------------------------------------
  api

  Monthly cost:    $1200
  CPU requested:   4 cores
  Mem requested:   8 Gi
  Risk:            low

  Workloads:
    api-gateway    $800.00     low
    api-auth       $400.00     low

--------------------------------------------
  data-pipeline

  Monthly cost:    $980
  CPU requested:   2.5 cores
  Mem requested:   12 Gi
  Risk:            moderate

  Workloads:
    etl            $980.00     moderate

--------------------------------------------
  default

  Monthly cost:    $640
  CPU requested:   1 cores
  Mem requested:   2 Gi
  Risk:            low

  Workloads:
    web            $640.00     low
```

## 📉 Right-Size — Simulate Resource Optimization

Right-sizes pod resource requests based on actual usage percentiles with a configurable safety buffer.

```bash
# Default: p95 percentile + 20% buffer
kubectl whatif rightsize

# Conservative: p99 percentile + 30% buffer
kubectl whatif rightsize --percentile=p99 --buffer=30

# Aggressive: p90 percentile + 10% buffer
kubectl whatif rightsize --percentile=p90 --buffer=10

# Scope to specific namespaces
kubectl whatif rightsize --scope-namespaces=api,data-pipeline

# Exclude system namespaces
kubectl whatif rightsize --exclude-namespaces=kube-system,monitoring

# Show per-workload details
kubectl whatif rightsize --verbose
```

| Flag | Default | Description |
|------|---------|-------------|
| `--percentile` | `p95` | Usage percentile: `p50`, `p90`, `p95`, `p99` |
| `--buffer` | `20` | Buffer percentage above the percentile |
| `--scope-namespaces` | all | Comma-separated namespaces to include |
| `--exclude-namespaces` | none | Comma-separated namespaces to exclude |
| `--limit-strategy` | none | How to set limits: `ratio`, `fixed`, or empty |

Example output:

```
KubeWise: Right-size simulation (p95 + 20% buffer)

  Current monthly cost:    $14230
  Projected monthly cost:  $9840
  Savings:                 $4390/mo (30.8%)  low
  Cluster OOM risk:        0.8%              low

  Top savings by namespace:
    api-gateway          $1200/mo saved    risk: low
    data-pipeline        $980/mo saved     risk: moderate
    ml-inference         $870/mo saved     risk: high
    web-frontend         $640/mo saved     risk: low
    auth-service         $700/mo saved     risk: low
```

## 🔗 Consolidate — Simulate Node Pool Consolidation

Simulates consolidating workloads onto fewer or different node types using bin-packing.

```bash
# Consolidate to a specific instance type
kubectl whatif consolidate --node-type=m6i.xlarge

# Cap the number of nodes
kubectl whatif consolidate --node-type=m6i.2xlarge --max-nodes=10

# Keep an existing node pool
kubectl whatif consolidate --node-type=m6i.xlarge --keep-pool=critical-pool
```

| Flag | Default | Description |
|------|---------|-------------|
| `--node-type` | required | Target instance type for consolidation |
| `--max-nodes` | unlimited | Maximum number of nodes in the new pool |
| `--keep-pool` | none | Existing node pool to preserve |
| `--target-cpu` | `0.8` | Target CPU utilization ratio |
| `--target-memory` | `0.8` | Target memory utilization ratio |

## 💰 Spot — Simulate Spot Instance Migration

Estimates savings from migrating eligible workloads to spot instances, with per-workload eviction risk scoring.

```bash
# Default: workloads with >=2 replicas, 65% discount
kubectl whatif spot

# More aggressive: include single-replica workloads
kubectl whatif spot --min-replicas=1

# Custom discount rate
kubectl whatif spot --discount=0.70

# Exclude specific namespaces
kubectl whatif spot --exclude-namespaces=kube-system,databases

# With detailed risk breakdown
kubectl whatif spot --verbose
```

| Flag | Default | Description |
|------|---------|-------------|
| `--min-replicas` | `2` | Minimum replicas for spot eligibility |
| `--discount` | `0.65` | Spot discount fraction (0.0 - 1.0) |
| `--exclude-namespaces` | none | Comma-separated namespaces to exclude |
| `--controller-types` | `Deployment,ReplicaSet` | Controller types eligible for spot |

## 📝 Scenario Files — Define Reusable Scenarios

Define scenarios as YAML files for repeatability and version control:

```yaml
apiVersion: kubewise.io/v1alpha1
kind: RightSize
metadata:
  name: conservative
  description: "Conservative right-sizing"
spec:
  percentile: p95
  buffer: 30
  scope:
    namespaces: ["*"]
    exclude:
      - namespace: kube-system
  limits:
    strategy: ratio
```

```bash
# Apply a single scenario
kubectl whatif apply -f scenario.yaml

# Compare multiple scenarios side by side
kubectl whatif compare -f aggressive.yaml -f conservative.yaml
```

See [docs/scenarios.md](docs/scenarios.md) for all scenario types and options.

## 📁 Output Formats

All commands support three output formats:

```bash
kubectl whatif rightsize                    # Terminal table (default)
kubectl whatif rightsize --output=json      # JSON for scripting
kubectl whatif rightsize --output=markdown  # Markdown for PR comments
```

The `snapshot` command uses a stacked-panel layout in table mode, showing each namespace as a bordered panel with cost, resource, and workload details. JSON and markdown outputs include the same data in their respective formats.

## 🤖 CI/CD Integration

### GitHub Action

Add KubeWise to your GitHub workflow to automatically post cost impact analysis on pull requests:

```yaml
# .github/workflows/cost-check.yml
name: Cost Check
on:
  pull_request:

jobs:
  cost-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: tochemey/kubewise/action@v1
        with:
          kubeconfig: ${{ secrets.KUBECONFIG_B64 }}
          scenario: rightsize
          percentile: p95
          buffer: '20'
          comment: 'true'
```

### Action Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `kubeconfig` | yes | — | Base64-encoded kubeconfig |
| `scenario` | yes | `rightsize` | Scenario type: `rightsize`, `consolidate`, `spot`, `snapshot` |
| `scenario-file` | no | — | Path to scenario YAML (overrides scenario type) |
| `percentile` | no | `p95` | Usage percentile (rightsize only) |
| `buffer` | no | `20` | Buffer percentage (rightsize only) |
| `node-type` | no | — | Target instance type (consolidate only, required) |
| `min-replicas` | no | `2` | Minimum replicas for spot eligibility (spot only) |
| `discount` | no | `0.65` | Spot discount fraction (spot only) |
| `save` | no | — | Save snapshot to JSON file (snapshot only) |
| `comment` | no | `true` | Post result as PR comment |
| `fail-on-risk` | no | `false` | Fail the check if risk is red |

### Action Outputs

| Output | Description |
|--------|-------------|
| `savings` | Projected monthly savings |
| `savings-percent` | Projected savings percentage |
| `risk-level` | Overall risk level (`green`, `amber`, `red`) |
| `markdown` | Full markdown report |

### Examples

**Right-size on every PR:**

```yaml
- uses: tochemey/kubewise/action@v1
  with:
    kubeconfig: ${{ secrets.KUBECONFIG_B64 }}
    scenario: rightsize
    comment: 'true'
```

**Spot migration analysis with risk gate:**

```yaml
- uses: tochemey/kubewise/action@v1
  with:
    kubeconfig: ${{ secrets.KUBECONFIG_B64 }}
    scenario: spot
    min-replicas: '2'
    discount: '0.65'
    fail-on-risk: 'true'
```

**Cluster cost snapshot:**

```yaml
- uses: tochemey/kubewise/action@v1
  with:
    kubeconfig: ${{ secrets.KUBECONFIG_B64 }}
    scenario: snapshot
    comment: 'true'
```

**Custom scenario file:**

```yaml
- uses: tochemey/kubewise/action@v1
  with:
    kubeconfig: ${{ secrets.KUBECONFIG_B64 }}
    scenario-file: scenarios/production-rightsize.yaml
    comment: 'true'
    fail-on-risk: 'true'
```

**Use outputs in downstream steps:**

```yaml
- uses: tochemey/kubewise/action@v1
  id: kubewise
  with:
    kubeconfig: ${{ secrets.KUBECONFIG_B64 }}
    scenario: rightsize

- run: echo "Projected savings: ${{ steps.kubewise.outputs.savings }}"
```

## 📚 Documentation

- [Quick Start](docs/quickstart.md)
- [Scenarios](docs/scenarios.md)
- [Architecture](docs/architecture.md)

## 🤝 Contributing

```bash
make lint        # Run linters
make test-unit   # Run unit tests
make test-all    # Lint + test
make build       # Build binary
```

## 📄 License

Apache License 2.0. See [LICENSE](LICENSE) for details.
