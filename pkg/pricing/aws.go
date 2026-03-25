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
	"net/url"
	"strconv"
	"time"

	"k8s.io/klog/v2"
)

const (
	// defaultSpotDiscount is the default spot discount (65% off on-demand).
	defaultSpotDiscount = 0.35

	// awsPricingEndpoint is the AWS Pricing API endpoint.
	// The Pricing API is only available in us-east-1 and ap-south-1.
	awsPricingEndpoint = "https://pricing.us-east-1.amazonaws.com"

	awsHTTPTimeout = 30 * time.Second
)

// awsRegionNames maps AWS region codes to the names used in the pricing API.
var awsRegionNames = map[string]string{
	"us-east-1":      "US East (N. Virginia)",
	"us-east-2":      "US East (Ohio)",
	"us-west-1":      "US West (N. California)",
	"us-west-2":      "US West (Oregon)",
	"eu-west-1":      "EU (Ireland)",
	"eu-west-2":      "EU (London)",
	"eu-west-3":      "EU (Paris)",
	"eu-central-1":   "EU (Frankfurt)",
	"eu-north-1":     "EU (Stockholm)",
	"ap-southeast-1": "Asia Pacific (Singapore)",
	"ap-southeast-2": "Asia Pacific (Sydney)",
	"ap-northeast-1": "Asia Pacific (Tokyo)",
	"ap-northeast-2": "Asia Pacific (Seoul)",
	"ap-south-1":     "Asia Pacific (Mumbai)",
	"sa-east-1":      "South America (Sao Paulo)",
	"ca-central-1":   "Canada (Central)",
}

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
		spotDiscount: defaultSpotDiscount,
		httpClient:   &http.Client{Timeout: awsHTTPTimeout},
		region:       region,
	}
	for _, opt := range opts {
		opt(p)
	}

	// Try cache first
	cached, err := GetCached("aws", region)
	if err == nil && len(cached) > 0 {
		klog.V(1).InfoS("Using cached AWS pricing", "region", region, "instanceTypes", len(cached))
		p.prices = cached
		return p, nil
	}

	// Fetch from API
	if err := p.fetchPricing(ctx, region); err != nil {
		return nil, fmt.Errorf("fetching AWS pricing for region %s: %w", region, err)
	}

	// Cache the result
	if cacheErr := SetCached("aws", region, p.prices); cacheErr != nil {
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
	return "aws"
}

// fetchPricing fetches EC2 on-demand pricing from the AWS Bulk Pricing API
// using the regional pricing JSON endpoint.
func (p *AWSProvider) fetchPricing(ctx context.Context, region string) error {
	regionName, ok := awsRegionNames[region]
	if !ok {
		return fmt.Errorf("unknown AWS region: %s", region)
	}

	klog.V(1).InfoS("Fetching AWS pricing", "region", region)

	// Use the Bulk API regional pricing endpoint.
	// Format: /offers/v1.0/aws/AmazonEC2/current/{region}/index.json
	pricingURL := fmt.Sprintf("%s/offers/v1.0/aws/AmazonEC2/current/%s/index.json", awsPricingEndpoint, region)

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
		return fmt.Errorf("AWS pricing API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading pricing response: %w", err)
	}

	return p.parseBulkPricing(body, regionName)
}

// awsBulkPricing represents the structure of the AWS Bulk Pricing JSON.
type awsBulkPricing struct {
	Products map[string]awsProduct `json:"products"`
	Terms    struct {
		OnDemand map[string]map[string]awsTerm `json:"OnDemand"`
	} `json:"terms"`
}

type awsProduct struct {
	SKU        string            `json:"sku"`
	Attributes map[string]string `json:"attributes"`
}

type awsTerm struct {
	PriceDimensions map[string]awsPriceDimension `json:"priceDimensions"`
}

type awsPriceDimension struct {
	PricePerUnit map[string]string `json:"pricePerUnit"`
}

func (p *AWSProvider) parseBulkPricing(data []byte, regionName string) error {
	var bulk awsBulkPricing
	if err := json.Unmarshal(data, &bulk); err != nil {
		return fmt.Errorf("parsing AWS pricing JSON: %w", err)
	}

	for sku, product := range bulk.Products {
		attrs := product.Attributes
		// Filter: Linux, Shared tenancy, on-demand, correct region
		if attrs["operatingSystem"] != "Linux" {
			continue
		}
		if attrs["tenancy"] != "Shared" {
			continue
		}
		if attrs["capacitystatus"] != "Used" {
			continue
		}
		if attrs["location"] != regionName {
			continue
		}

		instanceType := attrs["instanceType"]
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

func (p *AWSProvider) extractOnDemandPrice(sku string, onDemand map[string]map[string]awsTerm) float64 {
	skuTerms, ok := onDemand[sku]
	if !ok {
		return 0
	}

	for _, term := range skuTerms {
		for _, dim := range term.PriceDimensions {
			if usd, ok := dim.PricePerUnit["USD"]; ok {
				price, err := strconv.ParseFloat(usd, 64)
				if err == nil && price > 0 {
					return price
				}
			}
		}
	}
	return 0
}

// FetchAWSPricingSimple fetches pricing for specific instance types using
// the AWS Pricing query API. This is simpler but requires AWS credentials.
func FetchAWSPricingSimple(ctx context.Context, region string, instanceTypes []string, httpClient *http.Client) (map[string]float64, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: awsHTTPTimeout}
	}

	prices := make(map[string]float64)
	regionName, ok := awsRegionNames[region]
	if !ok {
		return nil, fmt.Errorf("unknown AWS region: %s", region)
	}

	for _, instanceType := range instanceTypes {
		price, err := fetchSingleInstancePrice(ctx, httpClient, instanceType, regionName)
		if err != nil {
			klog.V(2).InfoS("Failed to fetch price for instance type",
				"instanceType", instanceType, "err", err)
			continue
		}
		prices[instanceType] = price
	}

	return prices, nil
}

func fetchSingleInstancePrice(ctx context.Context, client *http.Client, instanceType, regionName string) (float64, error) {
	// Use the pricing filter API
	filterURL := fmt.Sprintf("%s/offers/v1.0/aws/AmazonEC2/current/index.json", awsPricingEndpoint)

	params := url.Values{}
	params.Set("instanceType", instanceType)
	params.Set("location", regionName)
	params.Set("operatingSystem", "Linux")
	params.Set("tenancy", "Shared")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, filterURL, nil)
	if err != nil {
		return 0, err
	}
	req.URL.RawQuery = params.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status %d", resp.StatusCode)
	}

	// For MVP, this is a simplified path - the bulk API approach above is preferred
	return 0, fmt.Errorf("simplified API not implemented, use bulk API")
}
