// Copyright 2026 KubeWise Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"k8s.io/klog/v2"
)

const (
	azureRetailPricesURL = "https://prices.azure.com/api/retail/prices"

	// azureUnitHour is the unit-of-measure value for hourly pricing in the Azure Retail Prices API.
	azureUnitHour = "1 Hour"

	// azurePriceTypeConsumption is the price type for pay-as-you-go (on-demand) pricing.
	azurePriceTypeConsumption = "Consumption"

	// azureServiceVMs is the Azure service name for Virtual Machines.
	azureServiceVMs = "Virtual Machines"

	// azureVMPriceFilter is the OData filter template used to query Linux on-demand VM prices.
	azureVMPriceFilter = "armRegionName eq '%s' and serviceName eq '" + azureServiceVMs + "' and priceType eq '" + azurePriceTypeConsumption + "'"
)

// AzureProvider implements PricingProvider for Azure Virtual Machines.
type AzureProvider struct {
	// prices maps instance type (armSkuName) to on-demand hourly cost.
	prices map[string]float64
	// spotDiscount is the spot discount multiplier.
	spotDiscount float64
	// region is the Azure region.
	region string
	// httpClient is used for API calls.
	httpClient *http.Client
	// baseURL can be overridden for testing.
	baseURL string
}

// AzureOption is a functional option for configuring the Azure pricing provider.
type AzureOption func(*AzureProvider)

// WithAzureHTTPClient sets a custom HTTP client.
func WithAzureHTTPClient(client *http.Client) AzureOption {
	return func(p *AzureProvider) {
		p.httpClient = client
	}
}

// WithAzureSpotDiscount sets the spot discount multiplier.
func WithAzureSpotDiscount(discount float64) AzureOption {
	return func(p *AzureProvider) {
		p.spotDiscount = discount
	}
}

// WithAzureBaseURL sets a custom base URL (for testing).
func WithAzureBaseURL(baseURL string) AzureOption {
	return func(p *AzureProvider) {
		p.baseURL = baseURL
	}
}

// NewAzureProvider creates a new Azure pricing provider.
func NewAzureProvider(ctx context.Context, region string, opts ...AzureOption) (*AzureProvider, error) {
	p := &AzureProvider{
		prices:       make(map[string]float64),
		spotDiscount: DefaultSpotDiscount,
		region:       region,
		httpClient:   &http.Client{Timeout: DefaultHTTPTimeout},
		baseURL:      azureRetailPricesURL,
	}
	for _, opt := range opts {
		opt(p)
	}

	// Try cache first
	cached, err := GetCached(ProviderAzure, region)
	if err == nil && len(cached) > 0 {
		klog.V(1).InfoS("Using cached Azure pricing", "region", region, "instanceTypes", len(cached))
		p.prices = cached
		return p, nil
	}

	// Fetch from API with retry
	err = Retry(ctx, DefaultRetryConfig(), func() error {
		p.prices = make(map[string]float64) // reset on retry
		return p.fetchPricing(ctx, region)
	})
	if err != nil {
		return nil, AzureSetupError(region, err)
	}

	// Cache the result
	if cacheErr := SetCached(ProviderAzure, region, p.prices); cacheErr != nil {
		klog.V(1).InfoS("Failed to cache Azure pricing", "err", cacheErr)
	}

	return p, nil
}

// NewAzureProviderFromPrices creates an Azure provider with pre-loaded pricing data.
func NewAzureProviderFromPrices(prices map[string]float64, spotDiscount float64) *AzureProvider {
	return &AzureProvider{
		prices:       prices,
		spotDiscount: spotDiscount,
	}
}

func (p *AzureProvider) HourlyCost(instanceType string, _ string, spot bool) (float64, error) {
	price, ok := p.prices[instanceType]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrNoPricing, instanceType)
	}
	if spot {
		return price * p.spotDiscount, nil
	}
	return price, nil
}

func (p *AzureProvider) Provider() string {
	return ProviderAzure
}

// azureRetailResponse represents the paginated response from the Azure Retail Prices API.
type azureRetailResponse struct {
	Items        []azurePriceItem `json:"Items"`
	NextPageLink string           `json:"NextPageLink"`
}

// azurePriceItem represents a single pricing line item returned by the Azure Retail Prices API.
type azurePriceItem struct {
	ArmRegionName string  `json:"armRegionName"`
	ArmSkuName    string  `json:"armSkuName"`
	ServiceName   string  `json:"serviceName"`
	UnitPrice     float64 `json:"unitPrice"`
	UnitOfMeasure string  `json:"unitOfMeasure"`
	Type          string  `json:"type"`
	ProductName   string  `json:"productName"`
	SkuName       string  `json:"skuName"`
}

func (p *AzureProvider) fetchPricing(ctx context.Context, region string) error {
	klog.V(1).InfoS("Fetching Azure pricing", "region", region)

	// Build OData filter for Linux on-demand VMs in the specified region
	filter := fmt.Sprintf(azureVMPriceFilter, region)

	pageURL := fmt.Sprintf("%s?$filter=%s", p.baseURL, url.QueryEscape(filter))

	for pageURL != "" {
		nextPage, err := p.fetchPage(ctx, pageURL)
		if err != nil {
			return err
		}
		pageURL = nextPage
	}

	klog.V(1).InfoS("Azure pricing fetched", "region", region, "instanceTypes", len(p.prices))
	return nil
}

// fetchPage fetches a single page of Azure pricing results and stores qualifying VM prices.
func (p *AzureProvider) fetchPage(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching Azure prices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", classifyHTTPError(resp.StatusCode, "Azure pricing API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var azureResp azureRetailResponse
	if err := json.Unmarshal(body, &azureResp); err != nil {
		return "", fmt.Errorf("parsing Azure pricing response: %w", err)
	}

	for _, item := range azureResp.Items {
		// Filter for Linux VMs with hourly pricing
		if item.UnitOfMeasure != azureUnitHour {
			continue
		}
		if item.Type != azurePriceTypeConsumption {
			continue
		}
		if item.ArmSkuName == "" {
			continue
		}
		if item.UnitPrice <= 0 {
			continue
		}
		// Store the first (lowest) price per SKU
		if _, exists := p.prices[item.ArmSkuName]; !exists {
			p.prices[item.ArmSkuName] = item.UnitPrice
		}
	}

	return azureResp.NextPageLink, nil
}
