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
  <a href="https://goreportcard.com/report/github.com/tochemey/kubewise"><img src="https://goreportcard.com/badge/github.com/tochemey/kubewise?style=flat-square" alt="Go Report Card"></a>
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

### Run

```bash
# See current cost breakdown
kubectl whatif snapshot

# Simulate right-sizing
kubectl whatif rightsize --percentile=p95 --buffer=20

# Simulate node consolidation
kubectl whatif consolidate --node-type=m6i.xlarge

# Simulate spot migration
kubectl whatif spot --min-replicas=2 --discount=0.65
```

### Example output

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

## 📁 Output Formats

```bash
kubectl whatif rightsize                    # Terminal table (default)
kubectl whatif rightsize --output=json      # JSON for scripting
kubectl whatif rightsize --output=markdown  # Markdown for PR comments
```

## 📝 Scenario Files

Define reusable scenarios as YAML files:

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
kubectl whatif apply -f scenario.yaml
kubectl whatif compare -f scenario-a.yaml -f scenario-b.yaml
```

See [docs/scenarios.md](docs/scenarios.md) for all scenario types and options.

## 🤖 CI/CD Integration

Add to your GitHub workflow:

```yaml
- uses: kubewise/action@v1
  with:
    kubeconfig: ${{ secrets.KUBECONFIG }}
    scenario: rightsize
    percentile: p95
    buffer: 20
    comment: true
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
