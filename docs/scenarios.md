# Scenarios

KubeWise scenarios are hypothetical mutations applied to a snapshot of your cluster. Each scenario produces a cost estimate and risk assessment without making any actual changes.

## Scenario file format

All scenario files follow this structure:

```yaml
apiVersion: kubewise.io/v1alpha1
kind: RightSize | Composite
metadata:
  name: my-scenario
  description: "Human-readable description"
spec:
  # kind-specific fields
```

## RightSize

Adjusts resource requests and limits based on actual usage percentiles.

### YAML schema

```yaml
apiVersion: kubewise.io/v1alpha1
kind: RightSize
metadata:
  name: rightsize-conservative
  description: "Conservative right-sizing"
spec:
  percentile: p95          # p50, p90, p95, p99
  buffer: 20               # percentage headroom above the percentile
  scope:
    namespaces: ["*"]      # ["*"] = all, or specific list
    exclude:
      - namespace: kube-system
      - label: "kubewise.io/skip=true"
  limits:
    strategy: ratio        # ratio, fixed, unbounded
```

### Fields

| Field              | Type     | Default | Description                                    |
|--------------------|----------|---------|------------------------------------------------|
| `percentile`       | string   | p95     | Which usage percentile to base new requests on |
| `buffer`           | int      | 20      | Percentage headroom above the percentile       |
| `scope.namespaces` | string[] | ["*"]   | Namespaces to include                          |
| `scope.exclude`    | object[] | []      | Namespace or label exclusions                  |
| `limits.strategy`  | string   | ratio   | How to compute new limits                      |

### Limit strategies

- **ratio**: Maintains the original request-to-limit ratio. If the original request was 100m with a 200m limit (2x ratio), the new limit will be `newRequest * 2.0`.
- **fixed**: Sets the limit equal to the request (no burst allowed).
- **unbounded**: Removes limits entirely (set to 0).

### CLI equivalent

```bash
kubectl whatif rightsize --percentile=p95 --buffer=20 --exclude-namespace=kube-system --limit-strategy=ratio
```

## Composite

Chains multiple scenarios sequentially. Each step receives the output of the previous step.

### YAML schema

```yaml
apiVersion: kubewise.io/v1alpha1
kind: Composite
metadata:
  name: layered-rightsize
  description: "Right-size in two passes"
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
          namespaces: ["payments", "auth"]
```

### Common use cases

**Conservative right-sizing**: wide buffers, exclude system namespaces.

```bash
kubectl whatif apply -f scenarios/rightsize-conservative.yaml
```

**Compare approaches**: See the tradeoff between conservative and aggressive right-sizing.

```bash
kubectl whatif compare -f scenarios/rightsize-conservative.yaml -f scenarios/rightsize-aggressive.yaml
```
