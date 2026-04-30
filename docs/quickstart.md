# Quick Start

## Prerequisites

- **kubectl** installed and configured with access to a Kubernetes cluster
- **Go 1.26+** (for building from source) or **krew** (for plugin installation)
- A running Kubernetes cluster (EKS, GKE, AKS, or local kind/minikube)

## Installation

### Homebrew (macOS / Linux)

```bash
brew install tochemey/tap/kubewise
```

### Scoop (Windows)

```bash
scoop bucket add kubewise https://github.com/tochemey/scoop-bucket.git
scoop install kubewise
```

### krew (kubectl plugin)

```bash
kubectl krew install whatif
```

### Pre-built binaries

Download from the [Releases](https://github.com/tochemey/kubewise/releases) page. Binaries are available for macOS (amd64/arm64), Linux (amd64/arm64), and Windows (amd64).

### Go install

```bash
go install github.com/tochemey/kubewise/cmd/kubectl-whatif@latest
```

### From source

```bash
git clone https://github.com/tochemey/kubewise.git
cd kubewise
make install
```

## First run

### Snapshot your cluster

```bash
kubectl whatif snapshot
```

This collects your cluster's current state (nodes, pods, resource requests, usage metrics) and displays a cost breakdown by namespace.

Save the snapshot for debugging:

```bash
kubectl whatif snapshot --save snapshot.json
```

### Right-size your workloads

```bash
kubectl whatif rightsize --percentile=p95 --buffer=20
```

This simulates adjusting resource requests based on actual P95 usage with a 20% safety buffer. The output shows:

- **Current monthly cost** vs **projected monthly cost**
- **Savings** in dollars and percentage
- **OOM risk** per namespace (green/amber/red)
- **Per-namespace breakdown** of savings

Example output:

```
KubeWise: Right-size simulation (p95 + 20% buffer)

  Current monthly cost:    $14230
  Projected monthly cost:  $9840
  Savings:                 $4390/mo (30.8%)  low

  Top savings by namespace:
    api-gateway          $1200/mo saved    risk: low
    data-pipeline        $980/mo saved     risk: moderate
    web-frontend         $640/mo saved     risk: low
```

### Scenario files and comparisons

```bash
# Apply a scenario file
kubectl whatif apply -f scenarios/rightsize-conservative.yaml

# Compare scenarios side by side
kubectl whatif compare -f scenarios/rightsize-conservative.yaml -f scenarios/rightsize-aggressive.yaml
```

## Output formats

```bash
# Terminal table (default)
kubectl whatif rightsize

# JSON (for scripting)
kubectl whatif rightsize --output=json

# Markdown (for PR comments)
kubectl whatif rightsize --output=markdown
```

## Common flags

| Flag               | Description                          | Default                           |
|--------------------|--------------------------------------|-----------------------------------|
| `--kubeconfig`     | Path to kubeconfig                   | `$KUBECONFIG` or `~/.kube/config` |
| `--context`        | Kubernetes context to use            | Current context                   |
| `--namespace`      | Limit to a specific namespace        | All namespaces                    |
| `--prometheus-url` | Prometheus endpoint                  | Auto-discovered                   |
| `--output` / `-o`  | Output format: table, json, markdown | table                             |
| `--verbose`        | Show per-workload breakdown          | false                             |
| `--no-color`       | Disable terminal colors              | false                             |
