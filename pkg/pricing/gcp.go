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
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

const (
	gcpBillingCatalogURL = "https://cloudbilling.googleapis.com/v1/services/6F81-5844-456A/skus"
	gcpHTTPTimeout       = 30 * time.Second
)

// gcpMachineSpec defines vCPU and memory for a GCP machine type.
type gcpMachineSpec struct {
	VCPUs    float64
	MemoryGB float64
}

// gcpMachineTypes maps GCP machine type names to their vCPU and memory specs.
var gcpMachineTypes = map[string]gcpMachineSpec{
	// N1 series
	"n1-standard-1":  {1, 3.75},
	"n1-standard-2":  {2, 7.5},
	"n1-standard-4":  {4, 15},
	"n1-standard-8":  {8, 30},
	"n1-standard-16": {16, 60},
	"n1-standard-32": {32, 120},
	"n1-standard-64": {64, 240},
	"n1-highmem-2":   {2, 13},
	"n1-highmem-4":   {4, 26},
	"n1-highmem-8":   {8, 52},
	"n1-highmem-16":  {16, 104},
	"n1-highmem-32":  {32, 208},
	"n1-highcpu-2":   {2, 1.8},
	"n1-highcpu-4":   {4, 3.6},
	"n1-highcpu-8":   {8, 7.2},
	"n1-highcpu-16":  {16, 14.4},
	// N2 series
	"n2-standard-2":  {2, 8},
	"n2-standard-4":  {4, 16},
	"n2-standard-8":  {8, 32},
	"n2-standard-16": {16, 64},
	"n2-standard-32": {32, 128},
	"n2-standard-48": {48, 192},
	"n2-highmem-2":   {2, 16},
	"n2-highmem-4":   {4, 32},
	"n2-highmem-8":   {8, 64},
	"n2-highmem-16":  {16, 128},
	"n2-highcpu-2":   {2, 2},
	"n2-highcpu-4":   {4, 4},
	"n2-highcpu-8":   {8, 8},
	"n2-highcpu-16":  {16, 16},
	// E2 series
	"e2-standard-2":  {2, 8},
	"e2-standard-4":  {4, 16},
	"e2-standard-8":  {8, 32},
	"e2-standard-16": {16, 64},
	"e2-medium":      {1, 4},
	"e2-small":       {0.5, 2},
	"e2-micro":       {0.25, 1},
	// N2D series
	"n2d-standard-2":  {2, 8},
	"n2d-standard-4":  {4, 16},
	"n2d-standard-8":  {8, 32},
	"n2d-standard-16": {16, 64},
}

// GCPProvider implements PricingProvider for GCP Compute Engine instances.
// GCP prices CPU and memory separately; per-node cost = (vCPUs × cpuHourly) + (GB × memHourly).
type GCPProvider struct {
	// cpuHourlyPrice is the per-vCPU on-demand hourly cost.
	cpuHourlyPrice float64
	// memHourlyPrice is the per-GB on-demand hourly cost.
	memHourlyPrice float64
	// precomputedPrices caches per-instance-type costs for faster lookups.
	precomputedPrices map[string]float64
	// spotDiscount is the preemptible discount multiplier.
	spotDiscount float64
	// region is the GCP region.
	region string
	// httpClient is used for API calls.
	httpClient *http.Client
}

// GCPOption configures the GCP pricing provider.
type GCPOption func(*GCPProvider)

// WithGCPHTTPClient sets a custom HTTP client.
func WithGCPHTTPClient(client *http.Client) GCPOption {
	return func(p *GCPProvider) {
		p.httpClient = client
	}
}

// WithGCPSpotDiscount sets the preemptible discount multiplier.
func WithGCPSpotDiscount(discount float64) GCPOption {
	return func(p *GCPProvider) {
		p.spotDiscount = discount
	}
}

// NewGCPProvider creates a new GCP pricing provider.
func NewGCPProvider(ctx context.Context, region string, opts ...GCPOption) (*GCPProvider, error) {
	p := &GCPProvider{
		precomputedPrices: make(map[string]float64),
		spotDiscount:      defaultSpotDiscount,
		region:            region,
		httpClient:        &http.Client{Timeout: gcpHTTPTimeout},
	}
	for _, opt := range opts {
		opt(p)
	}

	// Try cache first
	cached, err := GetCached("gcp", region)
	if err == nil && len(cached) > 0 {
		klog.V(1).InfoS("Using cached GCP pricing", "region", region, "instanceTypes", len(cached))
		p.precomputedPrices = cached
		return p, nil
	}

	// Fetch from API
	if err := p.fetchPricing(ctx, region); err != nil {
		return nil, fmt.Errorf("fetching GCP pricing for region %s: %w", region, err)
	}

	// Cache the result
	if cacheErr := SetCached("gcp", region, p.precomputedPrices); cacheErr != nil {
		klog.V(1).InfoS("Failed to cache GCP pricing", "err", cacheErr)
	}

	return p, nil
}

// NewGCPProviderFromRates creates a GCP provider with known per-vCPU and per-GB rates.
func NewGCPProviderFromRates(cpuHourly, memHourly, spotDiscount float64) *GCPProvider {
	p := &GCPProvider{
		cpuHourlyPrice:    cpuHourly,
		memHourlyPrice:    memHourly,
		precomputedPrices: make(map[string]float64),
		spotDiscount:      spotDiscount,
	}
	// Precompute prices for all known machine types
	for machineType, spec := range gcpMachineTypes {
		p.precomputedPrices[machineType] = spec.VCPUs*cpuHourly + spec.MemoryGB*memHourly
	}
	return p
}

func (p *GCPProvider) HourlyCost(instanceType string, _ string, spot bool) (float64, error) {
	price, ok := p.precomputedPrices[instanceType]
	if !ok {
		// Try to compute from CPU/memory rates if available
		spec, specOK := gcpMachineTypes[instanceType]
		if !specOK || p.cpuHourlyPrice == 0 {
			return 0, fmt.Errorf("%w: %s", ErrNoPricing, instanceType)
		}
		price = spec.VCPUs*p.cpuHourlyPrice + spec.MemoryGB*p.memHourlyPrice
	}
	if spot {
		return price * p.spotDiscount, nil
	}
	return price, nil
}

func (p *GCPProvider) Provider() string {
	return "gcp"
}

// gcpBillingResponse represents the GCP Cloud Billing Catalog API response.
type gcpBillingResponse struct {
	SKUs          []gcpSKU `json:"skus"`
	NextPageToken string   `json:"nextPageToken"`
}

type gcpSKU struct {
	Description    string       `json:"description"`
	Category       gcpCategory  `json:"category"`
	ServiceRegions []string     `json:"serviceRegions"`
	PricingInfo    []gcpPricing `json:"pricingInfo"`
}

type gcpCategory struct {
	ServiceDisplayName string `json:"serviceDisplayName"`
	ResourceFamily     string `json:"resourceFamily"`
	ResourceGroup      string `json:"resourceGroup"`
	UsageType          string `json:"usageType"`
}

type gcpPricing struct {
	PricingExpression gcpPricingExpression `json:"pricingExpression"`
}

type gcpPricingExpression struct {
	TieredRates []gcpTieredRate `json:"tieredRates"`
}

type gcpTieredRate struct {
	UnitPrice gcpMoney `json:"unitPrice"`
}

type gcpMoney struct {
	CurrencyCode string `json:"currencyCode"`
	Units        string `json:"units"`
	Nanos        int64  `json:"nanos"`
}

func (m gcpMoney) ToFloat64() float64 {
	units := 0.0
	if m.Units != "" {
		fmt.Sscanf(m.Units, "%f", &units)
	}
	return units + float64(m.Nanos)/1e9
}

func (p *GCPProvider) fetchPricing(ctx context.Context, region string) error {
	klog.V(1).InfoS("Fetching GCP pricing", "region", region)

	gcpRegion := gcpRegionCode(region)

	cpuPrice, err := p.fetchSKUPrice(ctx, gcpRegion, "CPU", "Predefined")
	if err != nil {
		return fmt.Errorf("fetching GCP CPU pricing: %w", err)
	}
	memPrice, err := p.fetchSKUPrice(ctx, gcpRegion, "RAM", "Predefined")
	if err != nil {
		return fmt.Errorf("fetching GCP memory pricing: %w", err)
	}

	p.cpuHourlyPrice = cpuPrice
	p.memHourlyPrice = memPrice

	// Precompute per-instance-type prices
	for machineType, spec := range gcpMachineTypes {
		p.precomputedPrices[machineType] = spec.VCPUs*cpuPrice + spec.MemoryGB*memPrice
	}

	klog.V(1).InfoS("GCP pricing fetched", "region", region,
		"cpuHourly", cpuPrice, "memHourly", memPrice, "instanceTypes", len(p.precomputedPrices))
	return nil
}

func (p *GCPProvider) fetchSKUPrice(ctx context.Context, region, resourceGroup, usageType string) (float64, error) {
	reqURL := fmt.Sprintf("%s?key=&currencyCode=USD", gcpBillingCatalogURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetching SKU list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GCP billing API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("reading response: %w", err)
	}

	var billingResp gcpBillingResponse
	if err := json.Unmarshal(body, &billingResp); err != nil {
		return 0, fmt.Errorf("parsing GCP billing response: %w", err)
	}

	for _, sku := range billingResp.SKUs {
		if sku.Category.ResourceGroup != resourceGroup {
			continue
		}
		if !strings.Contains(sku.Category.UsageType, usageType) {
			continue
		}
		if !containsRegion(sku.ServiceRegions, region) {
			continue
		}
		if len(sku.PricingInfo) > 0 && len(sku.PricingInfo[0].PricingExpression.TieredRates) > 0 {
			return sku.PricingInfo[0].PricingExpression.TieredRates[0].UnitPrice.ToFloat64(), nil
		}
	}

	return 0, fmt.Errorf("no %s pricing found for region %s", resourceGroup, region)
}

// gcpRegionCode converts a GCP region to the code used in billing SKUs.
func gcpRegionCode(region string) string {
	// GCP billing uses lowercase region codes like "us-central1"
	return strings.ToLower(region)
}

func containsRegion(regions []string, target string) bool {
	for _, r := range regions {
		if strings.EqualFold(r, target) {
			return true
		}
	}
	return false
}
