<h2 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.png">
    <source media="(prefers-color-scheme: light)" srcset="assets/logo.png">
    <img alt="KubeWise" src="assets/logo.png" width="420">
  </picture>
</h2>

<p align="center">
  <strong>Right-size Kubernetes workloads from real Prometheus usage data.</strong><br>
  A kubectl plugin and GitHub Action that recommends pod request changes
  with cost impact and OOM-risk classification — so the savings number lands in your PR review, not three weeks after the migration.
</p>

<p align="center">
  <a href="https://github.com/tochemey/kubewise/actions/workflows/ci.yml"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/tochemey/kubewise/ci.yml?style=flat-square" ></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue?style=flat-square" alt="License"></a>
  <a href="https://goreportcard.com/report/github.com/tochemey/kubewise"><img src="https://goreportcard.com/badge/github.com/tochemey/kubewise" alt="Go Report Card"></a>
</p>

---

## The Problem

Teams overprovision Kubernetes workloads because nobody knows what the right number is. The data exists — Prometheus has weeks of CPU and memory history per pod — but reading it, applying a percentile, and translating "12 fewer cores" into "$1,400/month" is the kind of work that always gets pushed to next sprint. So requests stay padded and the cluster bill keeps climbing.

## What KubeWise Does

KubeWise pulls real usage percentiles (P50/P90/P95/P99) from Prometheus, applies a configurable safety buffer, and reports the resulting CPU/memory request changes with:

- a **dollar amount per namespace** computed from live cloud pricing (AWS/Azure/GCP)
- an **OOM-risk classification** per workload, so you don't tighten the screws on a workload that already runs hot
- a **PR-ready markdown report** posted as a comment by the included GitHub Action

The whole flow runs against your own kubeconfig + Prometheus. No SaaS, no agent installed in the cluster, no data leaving your machine.

## How It Compares

| Need                                                          | Tool                                                  |
|---------------------------------------------------------------|-------------------------------------------------------|
| "Show me what we spent last month"                            | Kubecost / OpenCost                                   |
| "Auto-tune my cluster, I trust you"                           | CAST AI / Karpenter                                   |
| "Recommend new requests, post the cost delta on every PR"     | **KubeWise**                                          |
| "Recommend new requests, no CI, runs in-cluster"              | Goldilocks, Robusta KRR, VPA recommender              |

The wedge is the GitHub Action: right-sizing recommendations land in code review with a real dollar number, instead of being a quarterly chore nobody owns.

## Features

- **Right-size workloads** from P50/P90/P95/P99 Prometheus usage with a configurable safety buffer
- **Snapshot the cluster** as a stacked-panel cost breakdown per namespace
- **Live cloud pricing** for AWS, Azure, and GCP, with offline fallback
- **GitHub Action** that posts a markdown cost-impact report as a PR comment, with optional risk-gated check
- **OOM-risk classification** per workload, derived from the percentile distribution
- **Scenario YAML files** for repeatable, version-controlled right-sizing policies
- **Runs client-side** — no SaaS, no in-cluster agent, no telemetry

## 🚀 Quick Start

### Install

```bash
# Homebrew (macOS / Linux)
brew install tochemey/tap/kubewise

# Scoop (Windows)
scoop bucket add kubewise https://github.com/tochemey/scoop-bucket.git
scoop install kubewise

# krew (kubectl plugin)
kubectl krew install whatif

# Go install
go install github.com/tochemey/kubewise/cmd/kubectl-whatif@latest
```

Pre-built binaries for macOS, Linux, and Windows are available on the [Releases](https://github.com/tochemey/kubewise/releases) page.

**From source:**

```bash
git clone https://github.com/tochemey/kubewise.git
cd kubewise
make install
```

### Global Flags

These flags apply to all commands:

| Flag               | Default          | Description                                |
|--------------------|------------------|--------------------------------------------|
| `--kubeconfig`     | `~/.kube/config` | Path to kubeconfig file                    |
| `--context`        | current context  | Kubernetes context to use                  |
| `--namespace`      | all              | Limit to specific namespace                |
| `--prometheus-url` | auto-detect      | Prometheus endpoint for historical metrics |
| `--output`, `-o`   | `table`          | Output format: `table`, `json`, `markdown` |
| `--verbose`        | `false`          | Show detailed per-workload breakdown       |
| `--no-color`       | `false`          | Disable terminal colors                    |

## Snapshot — See Current Cost Breakdown

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

## Right-Size — Recommend Resource Requests

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

| Flag                   | Default | Description                                   |
|------------------------|---------|-----------------------------------------------|
| `--percentile`         | `p95`   | Usage percentile: `p50`, `p90`, `p95`, `p99`  |
| `--buffer`             | `20`    | Buffer percentage above the percentile        |
| `--scope-namespaces`   | all     | Comma-separated namespaces to include         |
| `--exclude-namespaces` | none    | Comma-separated namespaces to exclude         |
| `--limit-strategy`     | none    | How to set limits: `ratio`, `fixed`, or empty |

Example output:

```
KubeWise: Right-size recommendations (p95 + 20% buffer)

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

## Scenario Files — Define Reusable Scenarios

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

## Output Formats

All commands support three output formats:

```bash
kubectl whatif rightsize                    # Terminal table (default)
kubectl whatif rightsize --output=json      # JSON for scripting
kubectl whatif rightsize --output=markdown  # Markdown for PR comments
```

The `snapshot` command uses a stacked-panel layout in table mode, showing each namespace as a bordered panel with cost, resource, and workload details. JSON and markdown outputs include the same data in their respective formats.

## CI/CD Integration

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

| Input           | Required | Default     | Description                                     |
|-----------------|----------|-------------|-------------------------------------------------|
| `kubeconfig`    | yes      | —           | Base64-encoded kubeconfig                       |
| `scenario`      | yes      | `rightsize` | Scenario type: `rightsize` or `snapshot`        |
| `scenario-file` | no       | —           | Path to scenario YAML (overrides scenario type) |
| `percentile`    | no       | `p95`       | Usage percentile (rightsize only)               |
| `buffer`        | no       | `20`        | Buffer percentage (rightsize only)              |
| `save`          | no       | —           | Save snapshot to JSON file (snapshot only)      |
| `comment`       | no       | `true`      | Post result as PR comment                       |
| `fail-on-risk`  | no       | `false`     | Fail the check if risk is red                   |

### Action Outputs

| Output     | Description          |
|------------|----------------------|
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

**Use the markdown output in downstream steps:**

```yaml
- uses: tochemey/kubewise/action@v1
  id: kubewise
  with:
    kubeconfig: ${{ secrets.KUBECONFIG_B64 }}
    scenario: rightsize

- run: echo "${{ steps.kubewise.outputs.markdown }}" >> $GITHUB_STEP_SUMMARY
```

## Documentation

- [Quick Start](docs/quickstart.md)
- [Scenarios](docs/scenarios.md)
- [Architecture](docs/architecture.md)

## Contributing

```bash
make lint        # Run linters
make test-unit   # Run unit tests
make test-all    # Lint + test
make build       # Build binary
```

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
