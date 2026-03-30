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
	"strconv"

	"k8s.io/klog/v2"
)

const (
	// awsPricingEndpoint is the AWS Pricing API endpoint.
	// The Pricing API is only available in us-east-1 and ap-south-1.
	awsPricingEndpoint = "https://pricing.us-east-1.amazonaws.com"

	// awsBulkPricingPathTemplate is the URL path template for the AWS Bulk Pricing
	// regional JSON endpoint. The placeholder is the AWS region code.
	awsBulkPricingPathTemplate = "/offers/v1.0/aws/AmazonEC2/current/%s/index.json"

	// awsFilterOS is the operating system filter value used when querying AWS pricing.
	awsFilterOS = "Linux"
	// awsFilterTenancy is the tenancy filter value used when querying AWS pricing.
	awsFilterTenancy = "Shared"
	// awsFilterCapacityStatus is the capacity status filter value used when querying AWS pricing.
	awsFilterCapacityStatus = "Used"

	// awsAttrOperatingSystem is the AWS pricing attribute key for operating system.
	awsAttrOperatingSystem = "operatingSystem"
	// awsAttrTenancy is the AWS pricing attribute key for tenancy.
	awsAttrTenancy = "tenancy"
	// awsAttrCapacityStatus is the AWS pricing attribute key for capacity status.
	awsAttrCapacityStatus = "capacitystatus"
	// awsAttrRegionCode is the AWS pricing attribute key for region code (e.g., "us-east-1").
	awsAttrRegionCode = "regionCode"
	// awsAttrInstanceType is the AWS pricing attribute key for instance type.
	awsAttrInstanceType = "instanceType"
)

// AWSProvider implements PricingProvider for AWS EC2 instances.
type AWSProvider struct {
	// prices maps instance type to on-demand hourly cost.
	prices map[string]float64
	// spotDiscount is the multiplier applied to on-demand for spot pricing.
	// e.g., 0.35 means spot = 35% of on-demand (65% off).
	spotDiscount float64
	// httpClient is used for API calls.
	httpClient *http.Client
	// region is the AWS region.
	region string
}

// AWSOption configures the AWS pricing provider.
type AWSOption func(*AWSProvider)

// WithSpotDiscount sets the spot discount multiplier.
func WithSpotDiscount(discount float64) AWSOption {
	return func(p *AWSProvider) {
		p.spotDiscount = discount
	}
}

// WithHTTPClient sets a custom HTTP client (useful for testing).
func WithHTTPClient(client *http.Client) AWSOption {
	return func(p *AWSProvider) {
		p.httpClient = client
	}
}

// NewAWSProvider creates a new AWS pricing provider.
// It fetches pricing data from the AWS Bulk Pricing API for the given region.
func NewAWSProvider(ctx context.Context, region string, opts ...AWSOption) (*AWSProvider, error) {
	p := &AWSProvider{
		prices:       make(map[string]float64),
		spotDiscount: DefaultSpotDiscount,
		httpClient:   &http.Client{Timeout: DefaultHTTPTimeout},
		region:       region,
	}
	for _, opt := range opts {
		opt(p)
	}

	// Try cache first
	cached, err := GetCached(ProviderAWS, region)
	if err == nil && len(cached) > 0 {
		klog.V(1).InfoS("Using cached AWS pricing", "region", region, "instanceTypes", len(cached))
		p.prices = cached
		return p, nil
	}

	// Fetch from API with retry
	err = Retry(ctx, DefaultRetryConfig(), func() error {
		p.prices = make(map[string]float64) // reset on retry
		return p.fetchPricing(ctx, region)
	})
	if err != nil {
		return nil, AWSSetupError(region, err)
	}

	// Cache the result
	if cacheErr := SetCached(ProviderAWS, region, p.prices); cacheErr != nil {
		klog.V(1).InfoS("Failed to cache AWS pricing", "err", cacheErr)
	}

	return p, nil
}

// NewAWSProviderFromPrices creates an AWS provider with pre-loaded pricing data.
// Useful for testing and when pricing is loaded from cache or fallback.
func NewAWSProviderFromPrices(prices map[string]float64, spotDiscount float64) *AWSProvider {
	return &AWSProvider{
		prices:       prices,
		spotDiscount: spotDiscount,
		region:       "",
	}
}

func (p *AWSProvider) HourlyCost(instanceType string, _ string, spot bool) (float64, error) {
	price, ok := p.prices[instanceType]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrNoPricing, instanceType)
	}
	if spot {
		return price * p.spotDiscount, nil
	}
	return price, nil
}

func (p *AWSProvider) Provider() string {
	return ProviderAWS
}

// fetchPricing fetches EC2 on-demand pricing from the AWS Bulk Pricing API
// using the regional pricing JSON endpoint.
func (p *AWSProvider) fetchPricing(ctx context.Context, region string) error {
	klog.V(1).InfoS("Fetching AWS pricing", "region", region)

	// Use the Bulk API regional pricing endpoint.
	// Format: /offers/v1.0/aws/AmazonEC2/current/{region}/index.json
	pricingURL := fmt.Sprintf("%s"+awsBulkPricingPathTemplate, awsPricingEndpoint, region)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pricingURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching pricing from %s: %w", pricingURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return classifyHTTPError(resp.StatusCode, "AWS pricing API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading pricing response: %w", err)
	}

	return p.parseBulkPricing(body, region)
}

// awsBulkPricing represents the top-level structure of the AWS Bulk Pricing JSON
// returned by the regional pricing endpoint.
type awsBulkPricing struct {
	Products map[string]awsProduct `json:"products"`
	Terms    struct {
		OnDemand map[string]map[string]awsTerm `json:"OnDemand"`
	} `json:"terms"`
}

// awsProduct represents an individual product entry in the AWS Bulk Pricing response.
type awsProduct struct {
	SKU        string            `json:"sku"`
	Attributes map[string]string `json:"attributes"`
}

// awsTerm represents an on-demand pricing term, containing one or more price dimensions.
type awsTerm struct {
	PriceDimensions map[string]awsPriceDimension `json:"priceDimensions"`
}

// awsPriceDimension represents a single price dimension within a pricing term.
type awsPriceDimension struct {
	PricePerUnit map[string]string `json:"pricePerUnit"`
}

// parseBulkPricing unmarshals the AWS Bulk Pricing JSON and extracts on-demand
// hourly prices for Linux/Shared instances in the specified region.
// It matches products by the regionCode attribute rather than the location
// display name, so it works for any AWS region without a static name mapping.
func (p *AWSProvider) parseBulkPricing(data []byte, region string) error {
	var bulk awsBulkPricing
	if err := json.Unmarshal(data, &bulk); err != nil {
		return fmt.Errorf("parsing AWS pricing JSON: %w", err)
	}

	for sku, product := range bulk.Products {
		attrs := product.Attributes
		// Filter: Linux, Shared tenancy, on-demand, correct region
		if attrs[awsAttrOperatingSystem] != awsFilterOS {
			continue
		}
		if attrs[awsAttrTenancy] != awsFilterTenancy {
			continue
		}
		if attrs[awsAttrCapacityStatus] != awsFilterCapacityStatus {
			continue
		}
		if attrs[awsAttrRegionCode] != region {
			continue
		}

		instanceType := attrs[awsAttrInstanceType]
		if instanceType == "" {
			continue
		}

		// Find the on-demand price for this SKU
		price := p.extractOnDemandPrice(sku, bulk.Terms.OnDemand)
		if price > 0 {
			p.prices[instanceType] = price
		}
	}

	klog.V(1).InfoS("AWS pricing parsed", "region", p.region, "instanceTypes", len(p.prices))
	return nil
}

// extractOnDemandPrice returns the on-demand hourly price in USD for the given
// SKU, or 0 if no valid price is found.
func (p *AWSProvider) extractOnDemandPrice(sku string, onDemand map[string]map[string]awsTerm) float64 {
	skuTerms, ok := onDemand[sku]
	if !ok {
		return 0
	}

	for _, term := range skuTerms {
		for _, dim := range term.PriceDimensions {
			if usd, ok := dim.PricePerUnit[CurrencyUSD]; ok {
				price, err := strconv.ParseFloat(usd, 64)
				if err == nil && price > 0 {
					return price
				}
			}
		}
	}
	return 0
}
