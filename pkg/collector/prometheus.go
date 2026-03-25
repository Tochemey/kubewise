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

import (
	"context"
	"fmt"
	"math"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	queryTimeout = 30 * time.Second
)

// percentile definitions used for PromQL queries.
var percentiles = []struct {
	label    string
	quantile float64
}{
	{"p50", 0.50},
	{"p90", 0.90},
	{"p95", 0.95},
	{"p99", 0.99},
}

// CollectPrometheusUsage queries Prometheus for historical CPU/memory usage
// percentiles and returns a map keyed by "namespace/pod/container".
// If a query fails for a specific container, it is skipped with a warning.
func CollectPrometheusUsage(ctx context.Context, promURL string, pods []PodSnapshot, window time.Duration) (map[string]ContainerUsageProfile, error) {
	if promURL == "" {
		return nil, fmt.Errorf("prometheus URL is empty")
	}

	client, err := promapi.NewClient(promapi.Config{Address: promURL})
	if err != nil {
		return nil, fmt.Errorf("creating prometheus client: %w", err)
	}

	api := promv1.NewAPI(client)
	profiles := make(map[string]ContainerUsageProfile)
	windowStr := formatDuration(window)

	klog.V(1).InfoS("Collecting Prometheus usage data", "endpoint", promURL, "window", windowStr)

	for _, pod := range pods {
		for _, container := range pod.Containers {
			key := ProfileKey(pod.Namespace, pod.Name, container.Name)
			profile, err := queryContainerUsage(ctx, api, pod.Namespace, pod.Name, container.Name, windowStr, window)
			if err != nil {
				klog.V(2).InfoS("Skipping container, Prometheus query failed",
					"namespace", pod.Namespace,
					"pod", pod.Name,
					"container", container.Name,
					"err", err,
				)
				continue
			}
			profiles[key] = profile
		}
	}

	klog.V(1).InfoS("Prometheus usage collected", "containers", len(profiles))
	return profiles, nil
}

func queryContainerUsage(ctx context.Context, api promv1.API, namespace, pod, container, window string, windowDuration time.Duration) (ContainerUsageProfile, error) {
	var profile ContainerUsageProfile

	for _, p := range percentiles {
		cpuVal, err := queryScalar(ctx, api, buildCPUQuery(namespace, pod, container, p.quantile, window))
		if err != nil {
			return profile, fmt.Errorf("querying CPU %s: %w", p.label, err)
		}

		memVal, err := queryScalar(ctx, api, buildMemoryQuery(namespace, pod, container, p.quantile, window))
		if err != nil {
			return profile, fmt.Errorf("querying memory %s: %w", p.label, err)
		}

		// Convert CPU from cores to millicores
		cpuMillis := int64(math.Ceil(cpuVal * 1000))
		memBytes := int64(math.Ceil(memVal))

		switch p.label {
		case "p50":
			profile.P50CPU = cpuMillis
			profile.P50Memory = memBytes
		case "p90":
			profile.P90CPU = cpuMillis
			profile.P90Memory = memBytes
		case "p95":
			profile.P95CPU = cpuMillis
			profile.P95Memory = memBytes
		case "p99":
			profile.P99CPU = cpuMillis
			profile.P99Memory = memBytes
		}
	}

	// Query data point count (number of 5m samples in the window)
	countVal, err := queryScalar(ctx, api, buildCountQuery(namespace, pod, container, window))
	if err != nil {
		klog.V(3).InfoS("Could not determine data point count, using estimate",
			"namespace", namespace, "pod", pod, "container", container)
		// Estimate based on window (1 sample per 5 minutes)
		profile.DataPoints = estimateDataPoints(windowDuration)
	} else {
		profile.DataPoints = int(countVal)
	}

	return profile, nil
}

func queryScalar(ctx context.Context, api promv1.API, query string) (float64, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	klog.V(4).InfoS("Executing PromQL query", "query", query)

	result, warnings, err := api.Query(queryCtx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("prometheus query failed: %w", err)
	}

	for _, w := range warnings {
		klog.V(3).InfoS("Prometheus query warning", "warning", w)
	}

	switch v := result.(type) {
	case model.Vector:
		if len(v) == 0 {
			return 0, fmt.Errorf("empty result")
		}
		return float64(v[0].Value), nil
	case *model.Scalar:
		return float64(v.Value), nil
	default:
		return 0, fmt.Errorf("unexpected result type %T", result)
	}
}

// buildCPUQuery returns a PromQL query for CPU usage percentile.
// Uses quantile_over_time on the rate of container_cpu_usage_seconds_total.
func buildCPUQuery(namespace, pod, container string, quantile float64, window string) string {
	return fmt.Sprintf(
		`quantile_over_time(%.2f, rate(container_cpu_usage_seconds_total{namespace="%s", pod="%s", container="%s"}[5m])[%s:5m])`,
		quantile, namespace, pod, container, window,
	)
}

// buildMemoryQuery returns a PromQL query for memory usage percentile.
func buildMemoryQuery(namespace, pod, container string, quantile float64, window string) string {
	return fmt.Sprintf(
		`quantile_over_time(%.2f, container_memory_working_set_bytes{namespace="%s", pod="%s", container="%s"}[%s:5m])`,
		quantile, namespace, pod, container, window,
	)
}

// buildCountQuery returns a PromQL query for the number of data points.
func buildCountQuery(namespace, pod, container, window string) string {
	return fmt.Sprintf(
		`count_over_time(container_memory_working_set_bytes{namespace="%s", pod="%s", container="%s"}[%s])`,
		namespace, pod, container, window,
	)
}

// formatDuration converts a Go duration to a Prometheus-style duration string.
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

func estimateDataPoints(window time.Duration) int {
	// 1 sample per 5 minutes
	return int(window.Minutes() / 5)
}

// DiscoverPrometheusURL attempts to auto-discover the Prometheus endpoint
// by checking for known services in the cluster.
func DiscoverPrometheusURL(ctx context.Context, clientset kubernetes.Interface) string {
	// Check for Prometheus service in monitoring namespace
	for _, ns := range []string{"monitoring", "prometheus", "kube-monitoring"} {
		for _, svcName := range []string{"prometheus", "prometheus-server", "prometheus-k8s", "kube-prometheus-stack-prometheus"} {
			svc, err := clientset.CoreV1().Services(ns).Get(ctx, svcName, metav1.GetOptions{})
			if err != nil {
				continue
			}
			port := int32(9090)
			if len(svc.Spec.Ports) > 0 {
				port = svc.Spec.Ports[0].Port
			}
			url := fmt.Sprintf("http://%s.%s.svc:%d", svc.Name, svc.Namespace, port)
			klog.V(1).InfoS("Discovered Prometheus endpoint", "url", url)
			return url
		}
	}

	klog.V(1).InfoS("Prometheus endpoint not discovered")
	return ""
}
