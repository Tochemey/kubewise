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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureProviderFromPrices(t *testing.T) {
	prices := map[string]float64{
		"Standard_D4s_v3": 0.192,
		"Standard_D8s_v3": 0.384,
	}
	provider := NewAzureProviderFromPrices(prices, defaultSpotDiscount)

	t.Run("on-demand", func(t *testing.T) {
		cost, err := provider.HourlyCost("Standard_D4s_v3", "eastus", false)
		require.NoError(t, err)
		assert.InDelta(t, 0.192, cost, 1e-9)
	})

	t.Run("spot pricing", func(t *testing.T) {
		cost, err := provider.HourlyCost("Standard_D4s_v3", "eastus", true)
		require.NoError(t, err)
		assert.InDelta(t, 0.192*defaultSpotDiscount, cost, 1e-9)
	})

	t.Run("unknown instance type", func(t *testing.T) {
		_, err := provider.HourlyCost("Standard_Unknown", "eastus", false)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoPricing))
	})

	t.Run("provider name", func(t *testing.T) {
		assert.Equal(t, "azure", provider.Provider())
	})
}

func TestAzureProviderFetchPricing(t *testing.T) {
	azureResp := azureRetailResponse{
		Items: []azurePriceItem{
			{
				ArmRegionName: "eastus",
				ArmSkuName:    "Standard_D4s_v3",
				ServiceName:   "Virtual Machines",
				UnitPrice:     0.192,
				UnitOfMeasure: "1 Hour",
				Type:          "Consumption",
				ProductName:   "Virtual Machines Dsv3 Series",
				SkuName:       "D4s v3",
			},
			{
				ArmRegionName: "eastus",
				ArmSkuName:    "Standard_D8s_v3",
				ServiceName:   "Virtual Machines",
				UnitPrice:     0.384,
				UnitOfMeasure: "1 Hour",
				Type:          "Consumption",
				ProductName:   "Virtual Machines Dsv3 Series",
				SkuName:       "D8s v3",
			},
			// Should be filtered out: wrong unit
			{
				ArmRegionName: "eastus",
				ArmSkuName:    "Standard_D4s_v3",
				ServiceName:   "Virtual Machines",
				UnitPrice:     140.0,
				UnitOfMeasure: "1 Month",
				Type:          "Consumption",
			},
			// Should be filtered out: zero price
			{
				ArmRegionName: "eastus",
				ArmSkuName:    "Standard_Free",
				ServiceName:   "Virtual Machines",
				UnitPrice:     0,
				UnitOfMeasure: "1 Hour",
				Type:          "Consumption",
			},
		},
		NextPageLink: "",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data, _ := json.Marshal(azureResp)
		w.Write(data)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	SetCacheDir(tmpDir)
	defer SetCacheDir("")

	provider, err := NewAzureProvider(context.Background(), "eastus",
		WithAzureHTTPClient(server.Client()),
		WithAzureBaseURL(server.URL),
	)
	require.NoError(t, err)

	cost, err := provider.HourlyCost("Standard_D4s_v3", "eastus", false)
	require.NoError(t, err)
	assert.InDelta(t, 0.192, cost, 1e-9)

	cost, err = provider.HourlyCost("Standard_D8s_v3", "eastus", false)
	require.NoError(t, err)
	assert.InDelta(t, 0.384, cost, 1e-9)

	// Filtered items should not be present
	_, err = provider.HourlyCost("Standard_Free", "eastus", false)
	require.Error(t, err)
}

func TestAzureProviderPagination(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		if callCount == 1 {
			// First page with a NextPageLink
			resp := azureRetailResponse{
				Items: []azurePriceItem{
					{
						ArmSkuName: "Standard_D4s_v3", UnitPrice: 0.192,
						UnitOfMeasure: "1 Hour", Type: "Consumption",
					},
				},
				NextPageLink: fmt.Sprintf("%s/page2", r.Host),
			}
			// Construct proper next page URL
			resp.NextPageLink = fmt.Sprintf("http://%s/page2", r.Host)
			data, _ := json.Marshal(resp)
			w.Write(data)
		} else {
			// Second page, no more pages
			resp := azureRetailResponse{
				Items: []azurePriceItem{
					{
						ArmSkuName: "Standard_D8s_v3", UnitPrice: 0.384,
						UnitOfMeasure: "1 Hour", Type: "Consumption",
					},
				},
			}
			data, _ := json.Marshal(resp)
			w.Write(data)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	SetCacheDir(tmpDir)
	defer SetCacheDir("")

	provider, err := NewAzureProvider(context.Background(), "eastus",
		WithAzureHTTPClient(server.Client()),
		WithAzureBaseURL(server.URL),
	)
	require.NoError(t, err)

	// Both pages should be fetched
	assert.Equal(t, 2, callCount)

	cost, err := provider.HourlyCost("Standard_D4s_v3", "", false)
	require.NoError(t, err)
	assert.InDelta(t, 0.192, cost, 1e-9)

	cost, err = provider.HourlyCost("Standard_D8s_v3", "", false)
	require.NoError(t, err)
	assert.InDelta(t, 0.384, cost, 1e-9)
}

func TestAzureProviderHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	SetCacheDir(tmpDir)
	defer SetCacheDir("")

	_, err := NewAzureProvider(context.Background(), "eastus",
		WithAzureHTTPClient(server.Client()),
		WithAzureBaseURL(server.URL),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestAzureProviderCustomSpotDiscount(t *testing.T) {
	prices := map[string]float64{"Standard_D4s_v3": 0.192}
	provider := NewAzureProviderFromPrices(prices, 0.40)

	cost, err := provider.HourlyCost("Standard_D4s_v3", "", true)
	require.NoError(t, err)
	assert.InDelta(t, 0.192*0.40, cost, 1e-9)
}
