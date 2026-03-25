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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// extractQuery gets the PromQL query from either GET params or POST form body.
func extractQuery(r *http.Request) string {
	query := r.URL.Query().Get("query")
	if query == "" {
		_ = r.ParseForm()
		query = r.FormValue("query")
	}
	return query
}

// promResponse builds a Prometheus API JSON response with a single vector result.
func promResponse(value float64) string {
	return fmt.Sprintf(`{
		"status": "success",
		"data": {
			"resultType": "vector",
			"result": [
				{
					"metric": {},
					"value": [1700000000, "%.6f"]
				}
			]
		}
	}`, value)
}

// promEmptyResponse returns a Prometheus API response with no results.
func promEmptyResponse() string {
	return `{
		"status": "success",
		"data": {
			"resultType": "vector",
			"result": []
		}
	}`
}

func TestCollectPrometheusUsageSuccess(t *testing.T) {
	// Track which queries are made
	var queries []string

	// CPU values (in cores): p50=0.1, p90=0.15, p95=0.18, p99=0.22
	// Memory values (in bytes): p50=100MB, p90=140MB, p95=160MB, p99=200MB
	// Count: 4032
	responses := map[string]float64{
		"quantile_over_time(0.50": 0.1,    // p50 CPU
		"quantile_over_time(0.90": 0.15,   // p90 CPU
		"quantile_over_time(0.95": 0.18,   // p95 CPU
		"quantile_over_time(0.99": 0.22,   // p99 CPU
		"count_over_time":         4032.0, // data points
	}
	memResponses := map[string]float64{
		"quantile_over_time(0.50, container_memory": 100_000_000,
		"quantile_over_time(0.90, container_memory": 140_000_000,
		"quantile_over_time(0.95, container_memory": 160_000_000,
		"quantile_over_time(0.99, container_memory": 200_000_000,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := extractQuery(r)
		queries = append(queries, query)

		// Match memory queries first (more specific)
		for prefix, val := range memResponses {
			if strings.Contains(query, prefix) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, promResponse(val))
				return
			}
		}
		// Then CPU/count queries
		for prefix, val := range responses {
			if strings.Contains(query, prefix) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, promResponse(val))
				return
			}
		}

		// Unknown query
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, promEmptyResponse())
	}))
	defer server.Close()

	pods := []PodSnapshot{
		{
			Name:      "web-abc123",
			Namespace: "default",
			Containers: []ContainerSnapshot{
				{Name: "nginx"},
			},
		},
	}

	profiles, err := CollectPrometheusUsage(context.Background(), server.URL, pods, 14*24*time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	key := "default/web-abc123/nginx"
	profile, ok := profiles[key]
	require.True(t, ok)

	// CPU values converted to millicores (ceil)
	assert.Equal(t, int64(100), profile.P50CPU)
	assert.Equal(t, int64(150), profile.P90CPU)
	assert.Equal(t, int64(180), profile.P95CPU)
	assert.Equal(t, int64(220), profile.P99CPU)

	// Memory in bytes
	assert.Equal(t, int64(100_000_000), profile.P50Memory)
	assert.Equal(t, int64(140_000_000), profile.P90Memory)
	assert.Equal(t, int64(160_000_000), profile.P95Memory)
	assert.Equal(t, int64(200_000_000), profile.P99Memory)

	// Data points
	assert.Equal(t, 4032, profile.DataPoints)

	// Should have made 9 queries: 4 CPU percentiles + 4 memory percentiles + 1 count
	assert.Equal(t, 9, len(queries))
}

func TestCollectPrometheusUsagePartialFailure(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		query := extractQuery(r)
		// First container (nginx) succeeds, second (sidecar) returns empty results
		if strings.Contains(query, `container="sidecar"`) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, promEmptyResponse())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(query, "container_memory") {
			fmt.Fprint(w, promResponse(100_000_000))
		} else if strings.Contains(query, "count_over_time") {
			fmt.Fprint(w, promResponse(1000))
		} else {
			fmt.Fprint(w, promResponse(0.1))
		}
	}))
	defer server.Close()

	pods := []PodSnapshot{
		{
			Name:      "web-abc123",
			Namespace: "default",
			Containers: []ContainerSnapshot{
				{Name: "nginx"},
				{Name: "sidecar"},
			},
		},
	}

	profiles, err := CollectPrometheusUsage(context.Background(), server.URL, pods, 14*24*time.Hour)
	require.NoError(t, err)

	// Only nginx should have data; sidecar was skipped
	assert.Equal(t, 1, len(profiles))
	_, hasNginx := profiles["default/web-abc123/nginx"]
	assert.True(t, hasNginx)
	_, hasSidecar := profiles["default/web-abc123/sidecar"]
	assert.False(t, hasSidecar)
}

func TestCollectPrometheusUsageUnreachable(t *testing.T) {
	// Use a URL that won't connect
	profiles, err := CollectPrometheusUsage(context.Background(), "http://127.0.0.1:1", []PodSnapshot{
		{Name: "web", Namespace: "default", Containers: []ContainerSnapshot{{Name: "app"}}},
	}, 14*24*time.Hour)

	// Should return empty profiles (container skipped) but no top-level error
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

func TestCollectPrometheusUsageEmptyURL(t *testing.T) {
	_, err := CollectPrometheusUsage(context.Background(), "", nil, 14*24*time.Hour)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestBuildCPUQuery(t *testing.T) {
	query := buildCPUQuery("default", "web-abc123", "nginx", 0.95, "14d")
	assert.Contains(t, query, "quantile_over_time(0.95")
	assert.Contains(t, query, `namespace="default"`)
	assert.Contains(t, query, `pod="web-abc123"`)
	assert.Contains(t, query, `container="nginx"`)
	assert.Contains(t, query, "container_cpu_usage_seconds_total")
	assert.Contains(t, query, "[14d:5m]")
}

func TestBuildMemoryQuery(t *testing.T) {
	query := buildMemoryQuery("api", "api-pod", "api-container", 0.99, "7d")
	assert.Contains(t, query, "quantile_over_time(0.99")
	assert.Contains(t, query, `namespace="api"`)
	assert.Contains(t, query, "container_memory_working_set_bytes")
	assert.Contains(t, query, "[7d:5m]")
}

func TestBuildCountQuery(t *testing.T) {
	query := buildCountQuery("default", "web", "nginx", "14d")
	assert.Contains(t, query, "count_over_time")
	assert.Contains(t, query, `namespace="default"`)
	assert.Contains(t, query, "[14d]")
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{14 * 24 * time.Hour, "14d"},
		{7 * 24 * time.Hour, "7d"},
		{30 * 24 * time.Hour, "30d"},
		{6 * time.Hour, "6h"},
		{30 * time.Minute, "30m"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, formatDuration(tt.input))
		})
	}
}

func TestEstimateDataPoints(t *testing.T) {
	// 14 days = 14 * 24 * 60 / 5 = 4032
	assert.Equal(t, 4032, estimateDataPoints(14*24*time.Hour))
	// 7 days = 2016
	assert.Equal(t, 2016, estimateDataPoints(7*24*time.Hour))
}

func TestDiscoverPrometheusURL(t *testing.T) {
	t.Run("found in monitoring namespace", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus", Namespace: "monitoring"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 9090}},
			},
		})
		url := DiscoverPrometheusURL(context.Background(), clientset)
		assert.Equal(t, "http://prometheus.monitoring.svc:9090", url)
	})

	t.Run("found with custom port", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-server", Namespace: "monitoring"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 8080}},
			},
		})
		url := DiscoverPrometheusURL(context.Background(), clientset)
		assert.Equal(t, "http://prometheus-server.monitoring.svc:8080", url)
	})

	t.Run("not found", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		url := DiscoverPrometheusURL(context.Background(), clientset)
		assert.Equal(t, "", url)
	})
}

func TestCollectPrometheusUsageQueryContent(t *testing.T) {
	// Verify that all percentile queries are properly constructed
	var capturedQueries []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := extractQuery(r)
		capturedQueries = append(capturedQueries, query)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(query, "container_memory") && !strings.Contains(query, "count") {
			fmt.Fprint(w, promResponse(100_000_000))
		} else if strings.Contains(query, "count_over_time") {
			fmt.Fprint(w, promResponse(2016))
		} else {
			fmt.Fprint(w, promResponse(0.5))
		}
	}))
	defer server.Close()

	pods := []PodSnapshot{
		{Name: "p1", Namespace: "ns1", Containers: []ContainerSnapshot{{Name: "c1"}}},
	}

	_, err := CollectPrometheusUsage(context.Background(), server.URL, pods, 7*24*time.Hour)
	require.NoError(t, err)

	// Serialize queries for inspection
	data, _ := json.Marshal(capturedQueries)
	queryStr := string(data)

	// Should contain all quantiles for CPU
	assert.Contains(t, queryStr, "quantile_over_time(0.50, rate(container_cpu")
	assert.Contains(t, queryStr, "quantile_over_time(0.90, rate(container_cpu")
	assert.Contains(t, queryStr, "quantile_over_time(0.95, rate(container_cpu")
	assert.Contains(t, queryStr, "quantile_over_time(0.99, rate(container_cpu")

	// Should contain all quantiles for memory
	assert.Contains(t, queryStr, "quantile_over_time(0.50, container_memory")
	assert.Contains(t, queryStr, "quantile_over_time(0.90, container_memory")
	assert.Contains(t, queryStr, "quantile_over_time(0.95, container_memory")
	assert.Contains(t, queryStr, "quantile_over_time(0.99, container_memory")

	// Should contain count query
	assert.Contains(t, queryStr, "count_over_time")

	// All queries should use 7d window
	for _, q := range capturedQueries {
		assert.Contains(t, q, "7d")
		assert.Contains(t, q, `namespace="ns1"`)
		assert.Contains(t, q, `container="c1"`)
	}
}
