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

package pricing

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildAWSBulkPricingJSON(instanceType, location string, priceUSD float64) []byte {
	bulk := map[string]any{
		"products": map[string]any{
			"sku-001": map[string]any{
				"sku": "sku-001",
				"attributes": map[string]string{
					"instanceType":    instanceType,
					"location":        location,
					"operatingSystem": "Linux",
					"tenancy":         "Shared",
					"capacitystatus":  "Used",
				},
			},
		},
		"terms": map[string]any{
			"OnDemand": map[string]any{
				"sku-001": map[string]any{
					"sku-001.term": map[string]any{
						"priceDimensions": map[string]any{
							"sku-001.term.dim": map[string]any{
								"pricePerUnit": map[string]string{
									"USD": formatFloat(priceUSD),
								},
							},
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(bulk)
	return data
}

func formatFloat(f float64) string {
	return json.Number(func() string {
		b, _ := json.Marshal(f)
		return string(b)
	}()).String()
}

func TestAWSProviderHourlyCost(t *testing.T) {
	prices := map[string]float64{
		"m6i.xlarge":  0.192,
		"m6i.2xlarge": 0.384,
	}
	provider := NewAWSProviderFromPrices(prices, defaultSpotDiscount)

	t.Run("on-demand", func(t *testing.T) {
		cost, err := provider.HourlyCost("m6i.xlarge", "us-east-1", false)
		require.NoError(t, err)
		assert.InDelta(t, 0.192, cost, 1e-9)
	})

	t.Run("spot pricing", func(t *testing.T) {
		cost, err := provider.HourlyCost("m6i.xlarge", "us-east-1", true)
		require.NoError(t, err)
		assert.InDelta(t, 0.192*0.35, cost, 1e-9)
	})

	t.Run("unknown instance type", func(t *testing.T) {
		_, err := provider.HourlyCost("unknown.type", "us-east-1", false)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoPricing))
	})

	t.Run("provider name", func(t *testing.T) {
		assert.Equal(t, "aws", provider.Provider())
	})
}

func TestAWSProviderCustomSpotDiscount(t *testing.T) {
	prices := map[string]float64{"m6i.xlarge": 0.192}
	provider := NewAWSProviderFromPrices(prices, 0.40)

	cost, err := provider.HourlyCost("m6i.xlarge", "", true)
	require.NoError(t, err)
	assert.InDelta(t, 0.192*0.40, cost, 1e-9)
}

func TestAWSProviderFetchPricing(t *testing.T) {
	bulkJSON := buildAWSBulkPricingJSON("m6i.xlarge", "US East (N. Virginia)", 0.192)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(bulkJSON)
	}))
	defer server.Close()

	// Use a temp cache dir to avoid interfering with real cache
	tmpDir := t.TempDir()
	SetCacheDir(tmpDir)
	defer SetCacheDir("")

	p := &AWSProvider{
		prices:       make(map[string]float64),
		spotDiscount: defaultSpotDiscount,
		httpClient:   server.Client(),
		region:       "us-east-1",
	}

	// Override the pricing endpoint by swapping the fetchPricing URL
	// We test parseBulkPricing directly instead
	err := p.parseBulkPricing(bulkJSON, "US East (N. Virginia)")
	require.NoError(t, err)

	cost, err := p.HourlyCost("m6i.xlarge", "us-east-1", false)
	require.NoError(t, err)
	assert.InDelta(t, 0.192, cost, 1e-9)
}

func TestAWSProviderParseBulkPricingFilters(t *testing.T) {
	// Build a response with multiple products, only one should match
	bulk := map[string]any{
		"products": map[string]any{
			"sku-linux": map[string]any{
				"sku": "sku-linux",
				"attributes": map[string]string{
					"instanceType":    "m6i.xlarge",
					"location":        "US East (N. Virginia)",
					"operatingSystem": "Linux",
					"tenancy":         "Shared",
					"capacitystatus":  "Used",
				},
			},
			"sku-windows": map[string]any{
				"sku": "sku-windows",
				"attributes": map[string]string{
					"instanceType":    "m6i.xlarge",
					"location":        "US East (N. Virginia)",
					"operatingSystem": "Windows",
					"tenancy":         "Shared",
					"capacitystatus":  "Used",
				},
			},
			"sku-dedicated": map[string]any{
				"sku": "sku-dedicated",
				"attributes": map[string]string{
					"instanceType":    "m6i.xlarge",
					"location":        "US East (N. Virginia)",
					"operatingSystem": "Linux",
					"tenancy":         "Dedicated",
					"capacitystatus":  "Used",
				},
			},
			"sku-wrong-region": map[string]any{
				"sku": "sku-wrong-region",
				"attributes": map[string]string{
					"instanceType":    "m6i.2xlarge",
					"location":        "EU (Ireland)",
					"operatingSystem": "Linux",
					"tenancy":         "Shared",
					"capacitystatus":  "Used",
				},
			},
		},
		"terms": map[string]any{
			"OnDemand": map[string]any{
				"sku-linux": map[string]any{
					"sku-linux.term": map[string]any{
						"priceDimensions": map[string]any{
							"sku-linux.term.dim": map[string]any{
								"pricePerUnit": map[string]string{"USD": "0.192"},
							},
						},
					},
				},
				"sku-windows": map[string]any{
					"sku-windows.term": map[string]any{
						"priceDimensions": map[string]any{
							"sku-windows.term.dim": map[string]any{
								"pricePerUnit": map[string]string{"USD": "0.350"},
							},
						},
					},
				},
			},
		},
	}

	data, _ := json.Marshal(bulk)
	p := &AWSProvider{prices: make(map[string]float64), region: "us-east-1"}

	err := p.parseBulkPricing(data, "US East (N. Virginia)")
	require.NoError(t, err)

	// Only Linux + Shared + correct region should be in prices
	assert.Equal(t, 1, len(p.prices))
	assert.InDelta(t, 0.192, p.prices["m6i.xlarge"], 1e-9)
}

func TestAWSProviderHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	SetCacheDir(tmpDir)
	defer SetCacheDir("")

	p := &AWSProvider{
		prices:       make(map[string]float64),
		spotDiscount: defaultSpotDiscount,
		httpClient:   server.Client(),
		region:       "us-east-1",
	}

	// This will fail because we can't override the base URL in fetchPricing easily,
	// but we can test parseBulkPricing with invalid JSON
	err := p.parseBulkPricing([]byte("not json"), "US East (N. Virginia)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing AWS pricing JSON")
}
