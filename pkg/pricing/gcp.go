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
	"fmt"
	"strings"

	"golang.org/x/oauth2/google"
	cloudbilling "google.golang.org/api/cloudbilling/v1beta"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	"k8s.io/klog/v2"
)

// GCP-specific constants for API scopes, pagination, and resource classification.
const (
	// gcpScopeBillingReadOnly is the OAuth2 scope for read-only access to Cloud Billing.
	gcpScopeBillingReadOnly = "https://www.googleapis.com/auth/cloud-billing.readonly"
	// gcpScopeComputeReadOnly is the OAuth2 scope for read-only access to Compute Engine.
	gcpScopeComputeReadOnly = "https://www.googleapis.com/auth/compute.readonly"
	// gcpDefaultZoneSuffix is the zone letter appended to a region to pick an arbitrary zone.
	gcpDefaultZoneSuffix = "-a"
	// gcpOnDemandKeyword is the search string used to identify on-demand SKU groups.
	gcpOnDemandKeyword = "on demand"

	// gcpPageSizeSkuGroups is the page size when listing SKU groups.
	gcpPageSizeSkuGroups = 200
	// gcpPageSizeSkus is the page size when listing SKUs within a group.
	gcpPageSizeSkus = 500
	// gcpPageSizePrices is the page size when listing prices for a SKU.
	gcpPageSizePrices = 10
	// gcpPageSizeMachineTypes is the max results when listing Compute Engine machine types.
	gcpPageSizeMachineTypes = 500

	// mbPerGB is the number of megabytes in a gigabyte.
	mbPerGB = 1024
	// nanosPerUnit is the number of nanos in one unit, used for price conversion.
	nanosPerUnit = 1e9

	// gcpGeoTypeRegional is the GCP geo taxonomy type for regional resources.
	gcpGeoTypeRegional = "TYPE_REGIONAL"
	// gcpGeoTypeMultiRegional is the GCP geo taxonomy type for multi-regional resources.
	gcpGeoTypeMultiRegional = "TYPE_MULTI_REGIONAL"

	// gcpResourceCPU identifies a CPU pricing resource.
	gcpResourceCPU = "cpu"
	// gcpResourceRAM identifies a RAM pricing resource.
	gcpResourceRAM = "ram"
)

// SKU display name keywords used as search patterns to classify resources.
const (
	// gcpSKUPatternCore matches "core" in SKU display names (indicates CPU pricing).
	gcpSKUPatternCore = "core"
	// gcpSKUPatternCPU matches "cpu" in SKU display names (indicates CPU pricing).
	gcpSKUPatternCPU = "cpu"
	// gcpSKUPatternVCPU matches "vcpu" in SKU display names (indicates CPU pricing).
	gcpSKUPatternVCPU = "vcpu"
	// gcpSKUPatternRAM matches "ram" in SKU display names (indicates memory pricing).
	gcpSKUPatternRAM = "ram"
	// gcpSKUPatternMemory matches "memory" in SKU display names (indicates memory pricing).
	gcpSKUPatternMemory = "memory"
)

// gcpSupportedFamilies is the list of GCP machine families supported for pricing.
var gcpSupportedFamilies = []string{"n2d", "n1", "n2", "e2"}

// gcpMachineSpec defines vCPU and memory for a GCP machine type.
type gcpMachineSpec struct {
	VCPUs    float64
	MemoryGB float64
}

// gcpSharedCoreMachineTypes provides specs for shared-core machine types.
// These are needed because the Compute API reports GuestCpus=2 for all of them,
// while pricing uses the fractional CPU entitlement (0.25, 0.5, 1.0).
var gcpSharedCoreMachineTypes = map[string]gcpMachineSpec{
	"e2-micro":  {0.25, 1},
	"e2-small":  {0.5, 2},
	"e2-medium": {1, 4},
}

// GCPProvider implements PricingProvider for GCP Compute Engine instances.
// GCP prices CPU and memory separately; per-node cost = (vCPUs * cpuHourly) + (GB * memHourly).
type GCPProvider struct {
	// cpuHourlyPrice is the per-vCPU on-demand hourly cost (used by NewGCPProviderFromRates).
	cpuHourlyPrice float64
	// memHourlyPrice is the per-GB on-demand hourly cost (used by NewGCPProviderFromRates).
	memHourlyPrice float64
	// precomputedPrices caches per-instance-type costs for faster lookups.
	precomputedPrices map[string]float64
	// spotDiscount is the preemptible discount multiplier.
	spotDiscount float64
	// region is the GCP region.
	region string
	// clientOpts are options passed to the Cloud Billing service client.
	clientOpts []option.ClientOption
}

// GCPOption configures the GCP pricing provider.
type GCPOption func(*GCPProvider)

// WithGCPClientOption adds a google API client option (e.g., for custom HTTP client).
func WithGCPClientOption(opt option.ClientOption) GCPOption {
	return func(p *GCPProvider) {
		p.clientOpts = append(p.clientOpts, opt)
	}
}

// WithGCPSpotDiscount sets the preemptible discount multiplier.
func WithGCPSpotDiscount(discount float64) GCPOption {
	return func(p *GCPProvider) {
		p.spotDiscount = discount
	}
}

// NewGCPProvider creates a new GCP pricing provider.
// It uses the Cloud Billing Pricing API v1beta to fetch on-demand pricing.
// GCP Application Default Credentials (ADC) must be configured.
func NewGCPProvider(ctx context.Context, region string, opts ...GCPOption) (*GCPProvider, error) {
	p := &GCPProvider{
		precomputedPrices: make(map[string]float64),
		spotDiscount:      DefaultSpotDiscount,
		region:            region,
	}
	for _, opt := range opts {
		opt(p)
	}

	// Try cache first
	cached, err := GetCached(ProviderGCP, region)
	if err == nil && len(cached) > 0 {
		klog.V(1).InfoS("Using cached GCP pricing", "region", region, "instanceTypes", len(cached))
		p.precomputedPrices = cached
		return p, nil
	}

	// Fetch from API with retry
	err = Retry(ctx, DefaultRetryConfig(), func() error {
		p.precomputedPrices = make(map[string]float64) // reset on retry
		return p.fetchPricing(ctx, region)
	})
	if err != nil {
		return nil, GCPSetupError(region, err)
	}

	// Cache the result
	if cacheErr := SetCached(ProviderGCP, region, p.precomputedPrices); cacheErr != nil {
		klog.V(1).InfoS("Failed to cache GCP pricing", "err", cacheErr)
	}

	return p, nil
}

// NewGCPProviderFromRates creates a GCP provider with known per-vCPU and per-GB rates.
// machineSpecs maps instance type names to their vCPU and memory specs.
func NewGCPProviderFromRates(cpuHourly, memHourly, spotDiscount float64, machineSpecs map[string]gcpMachineSpec) *GCPProvider {
	p := &GCPProvider{
		cpuHourlyPrice:    cpuHourly,
		memHourlyPrice:    memHourly,
		precomputedPrices: make(map[string]float64),
		spotDiscount:      spotDiscount,
	}
	for machineType, spec := range machineSpecs {
		p.precomputedPrices[machineType] = spec.VCPUs*cpuHourly + spec.MemoryGB*memHourly
	}
	return p
}

// HourlyCost returns the hourly cost in USD for the given GCP instance type.
// If spot is true, the preemptible discount is applied.
func (p *GCPProvider) HourlyCost(instanceType string, _ string, spot bool) (float64, error) {
	price, ok := p.precomputedPrices[instanceType]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrNoPricing, instanceType)
	}
	if spot {
		return price * p.spotDiscount, nil
	}
	return price, nil
}

// Provider returns the provider name for GCP.
func (p *GCPProvider) Provider() string {
	return ProviderGCP
}

// gcpFamilyRate holds per-vCPU and per-GB hourly rates for a machine family.
type gcpFamilyRate struct {
	cpuHourly float64
	memHourly float64
}

// fetchPricing uses the Cloud Billing Pricing API v1beta to fetch on-demand
// compute pricing for the given region.
func (p *GCPProvider) fetchPricing(ctx context.Context, region string) error {
	klog.V(1).InfoS("Fetching GCP pricing via Pricing API v1beta", "region", region)

	opts := append([]option.ClientOption{
		option.WithScopes(gcpScopeBillingReadOnly),
	}, p.clientOpts...)

	svc, err := cloudbilling.NewService(ctx, opts...)
	if err != nil {
		return fmt.Errorf("creating Cloud Billing service (ensure GCP credentials are configured via 'gcloud auth application-default login'): %w", err)
	}

	gcpRegion := strings.ToLower(region)

	// Step 1: List SKU groups and find on-demand compute VM groups.
	var computeGroups []*cloudbilling.GoogleCloudBillingSkugroupsV1betaSkuGroup
	err = svc.SkuGroups.List().PageSize(gcpPageSizeSkuGroups).Pages(ctx, func(resp *cloudbilling.GoogleCloudBillingSkugroupsV1betaListSkuGroupsResponse) error {
		for _, g := range resp.SkuGroups {
			if isComputeOnDemandGroup(g.DisplayName) {
				computeGroups = append(computeGroups, g)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("listing SKU groups: %w", err)
	}
	klog.V(2).InfoS("Found compute on-demand SKU groups", "count", len(computeGroups))

	// Step 2: For each compute group, find CPU/RAM SKUs matching our region and fetch prices.
	familyRates := make(map[string]*gcpFamilyRate)

	for _, group := range computeGroups {
		if err := p.collectGroupPrices(ctx, svc, group.Name, gcpRegion, familyRates); err != nil {
			klog.V(1).InfoS("Failed to process SKU group", "group", group.DisplayName, "err", err)
			continue
		}
	}

	if len(familyRates) == 0 {
		return fmt.Errorf("no compute pricing found for region %s (found %d SKU groups)", gcpRegion, len(computeGroups))
	}

	// Step 3: Fetch machine type specs dynamically from the Compute API.
	machineSpecs, err := fetchMachineSpecs(ctx, gcpRegion, p.clientOpts)
	if err != nil {
		return fmt.Errorf("fetching machine type specs: %w", err)
	}

	// Merge shared-core types which have fractional CPU entitlements
	// that the Compute API doesn't expose (it reports GuestCpus=2 for all).
	for name, spec := range gcpSharedCoreMachineTypes {
		machineSpecs[name] = spec
	}

	// Step 4: Precompute per-instance-type prices using family-specific rates.
	for machineType, spec := range machineSpecs {
		family := machineTypeFamily(machineType)
		rates, ok := familyRates[family]
		if !ok {
			continue
		}
		p.precomputedPrices[machineType] = spec.VCPUs*rates.cpuHourly + spec.MemoryGB*rates.memHourly
	}

	klog.V(1).InfoS("GCP pricing fetched",
		"region", region, "families", len(familyRates), "instanceTypes", len(p.precomputedPrices))
	return nil
}

// fetchMachineSpecs dynamically fetches machine type specs from the Compute Engine API.
// It returns a map of machine type name to spec. Shared-core types (e2-micro, etc.)
// are skipped because their GuestCpus value doesn't reflect the fractional CPU entitlement.
func fetchMachineSpecs(ctx context.Context, region string, clientOpts []option.ClientOption) (map[string]gcpMachineSpec, error) {
	creds, err := google.FindDefaultCredentials(ctx, gcpScopeComputeReadOnly)
	if err != nil {
		return nil, fmt.Errorf("finding default credentials: %w", err)
	}
	if creds.ProjectID == "" {
		return nil, fmt.Errorf("no project ID in credentials (set GOOGLE_CLOUD_PROJECT or configure ADC with a project)")
	}

	opts := append([]option.ClientOption{
		option.WithScopes(gcpScopeComputeReadOnly),
	}, clientOpts...)

	computeSvc, err := compute.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating Compute service: %w", err)
	}

	// Machine types are identical across zones in a region; pick the first zone.
	zone := region + gcpDefaultZoneSuffix

	specs := make(map[string]gcpMachineSpec)
	err = computeSvc.MachineTypes.List(creds.ProjectID, zone).MaxResults(gcpPageSizeMachineTypes).Pages(ctx, func(resp *compute.MachineTypeList) error {
		for _, mt := range resp.Items {
			if mt.IsSharedCpu {
				continue
			}
			specs[mt.Name] = gcpMachineSpec{
				VCPUs:    float64(mt.GuestCpus),
				MemoryGB: float64(mt.MemoryMb) / mbPerGB,
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing machine types in %s: %w", zone, err)
	}

	klog.V(2).InfoS("Fetched machine type specs from Compute API", "zone", zone, "types", len(specs))
	return specs, nil
}

// collectGroupPrices iterates SKUs in a group and collects CPU/RAM rates per family for the region.
func (p *GCPProvider) collectGroupPrices(ctx context.Context, svc *cloudbilling.Service, groupName, region string, familyRates map[string]*gcpFamilyRate) error {
	return svc.SkuGroups.Skus.List(groupName).PageSize(gcpPageSizeSkus).Pages(ctx, func(resp *cloudbilling.GoogleCloudBillingSkugroupskusV1betaListSkuGroupSkusResponse) error {
		for _, sku := range resp.SkuGroupSkus {
			if !skuMatchesRegion(sku, region) {
				continue
			}

			family := extractFamilyFromSKU(sku.DisplayName)
			if family == "" {
				continue
			}

			resourceType := classifyResource(sku)
			if resourceType == "" {
				continue
			}

			price, err := fetchSKUListPrice(ctx, svc, sku.SkuId)
			if err != nil {
				klog.V(2).InfoS("Failed to get price for SKU", "sku", sku.DisplayName, "skuId", sku.SkuId, "err", err)
				continue
			}

			if _, ok := familyRates[family]; !ok {
				familyRates[family] = &gcpFamilyRate{}
			}
			switch resourceType {
			case gcpResourceCPU:
				familyRates[family].cpuHourly = price
			case gcpResourceRAM:
				familyRates[family].memHourly = price
			}
		}
		return nil
	})
}

// fetchSKUListPrice retrieves the hourly USD list price for a SKU.
func fetchSKUListPrice(ctx context.Context, svc *cloudbilling.Service, skuID string) (float64, error) {
	parent := fmt.Sprintf("skus/%s", skuID)
	resp, err := svc.Skus.Prices.List(parent).CurrencyCode(CurrencyUSD).PageSize(gcpPageSizePrices).Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("fetching price for SKU %s: %w", skuID, err)
	}
	for _, p := range resp.Prices {
		if p.Rate == nil || len(p.Rate.Tiers) == 0 {
			continue
		}
		tier := p.Rate.Tiers[0]
		if tier.ListPrice != nil {
			return float64(tier.ListPrice.Units) + float64(tier.ListPrice.Nanos)/nanosPerUnit, nil
		}
	}
	return 0, fmt.Errorf("no list price found for SKU %s", skuID)
}

// isComputeOnDemandGroup returns true if the SKU group display name indicates
// on-demand Compute Engine VMs for machine families we support.
func isComputeOnDemandGroup(displayName string) bool {
	lower := strings.ToLower(displayName)
	if !strings.Contains(lower, gcpOnDemandKeyword) {
		return false
	}
	for _, family := range gcpSupportedFamilies {
		if strings.Contains(lower, family+" ") || strings.HasPrefix(lower, family+" ") {
			return true
		}
	}
	return false
}

// skuMatchesRegion checks if a SKU applies to the target region.
func skuMatchesRegion(sku *cloudbilling.GoogleCloudBillingSkugroupskusV1betaSkuGroupSku, region string) bool {
	if sku.GeoTaxonomy == nil {
		return false
	}
	switch sku.GeoTaxonomy.Type {
	case gcpGeoTypeRegional:
		if sku.GeoTaxonomy.RegionalMetadata != nil && sku.GeoTaxonomy.RegionalMetadata.Region != nil {
			return strings.EqualFold(sku.GeoTaxonomy.RegionalMetadata.Region.Region, region)
		}
	case gcpGeoTypeMultiRegional:
		if sku.GeoTaxonomy.MultiRegionalMetadata != nil {
			for _, r := range sku.GeoTaxonomy.MultiRegionalMetadata.Regions {
				if strings.EqualFold(r.Region, region) {
					return true
				}
			}
		}
	}
	return false
}

// extractFamilyFromSKU extracts the machine family from a SKU display name.
// Examples: "N2 Instance Core running in Americas" -> "n2",
//
//	"N2D Instance Ram running in EMEA" -> "n2d"
func extractFamilyFromSKU(displayName string) string {
	lower := strings.ToLower(displayName)
	// Check longer prefixes first to avoid "n2" matching "n2d"
	for _, family := range gcpSupportedFamilies {
		if strings.HasPrefix(lower, family+" ") {
			return family
		}
	}
	return ""
}

// classifyResource determines if a SKU represents CPU or RAM pricing.
func classifyResource(sku *cloudbilling.GoogleCloudBillingSkugroupskusV1betaSkuGroupSku) string {
	lower := strings.ToLower(sku.DisplayName)
	if strings.Contains(lower, gcpSKUPatternCore) || strings.Contains(lower, gcpSKUPatternCPU) || strings.Contains(lower, gcpSKUPatternVCPU) {
		return gcpResourceCPU
	}
	if strings.Contains(lower, gcpSKUPatternRAM) || strings.Contains(lower, gcpSKUPatternMemory) {
		return gcpResourceRAM
	}
	// Fall back to product taxonomy categories
	if sku.ProductTaxonomy != nil {
		for _, cat := range sku.ProductTaxonomy.TaxonomyCategories {
			catLower := strings.ToLower(cat.Category)
			if strings.Contains(catLower, gcpSKUPatternCore) || strings.Contains(catLower, gcpSKUPatternCPU) {
				return gcpResourceCPU
			}
			if strings.Contains(catLower, gcpSKUPatternRAM) || strings.Contains(catLower, gcpSKUPatternMemory) {
				return gcpResourceRAM
			}
		}
	}
	return ""
}

// machineTypeFamily extracts the family prefix from a machine type name.
// Examples: "n2-standard-4" -> "n2", "n2d-standard-8" -> "n2d", "e2-medium" -> "e2"
func machineTypeFamily(machineType string) string {
	// Handle n2d before n2 by checking for known families
	for _, family := range gcpSupportedFamilies {
		if strings.HasPrefix(machineType, family+"-") {
			return family
		}
	}
	return ""
}
