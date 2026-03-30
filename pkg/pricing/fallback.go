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
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// pricingFile represents the top-level structure of the YAML pricing file.
type pricingFile struct {
	Pricing map[string]pricingEntry `yaml:"pricing"`
}

// pricingEntry holds the on-demand and spot hourly costs for a single instance type.
type pricingEntry struct {
	OnDemandHourly float64 `yaml:"on_demand_hourly"`
	SpotHourly     float64 `yaml:"spot_hourly"`
}

// FilePricingProvider implements PricingProvider using a user-supplied YAML pricing file.
// This is the manual fallback when cloud API pricing is unavailable.
type FilePricingProvider struct {
	prices map[string]pricingEntry
}

// LoadPricingFromFile reads a YAML pricing file and returns a PricingProvider.
// The file must contain a top-level "pricing" key mapping instance type names
// to their on_demand_hourly and (optionally) spot_hourly costs.
func LoadPricingFromFile(path string) (PricingProvider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading pricing file %s: %w", path, err)
	}

	var pf pricingFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parsing pricing file %s: %w", path, err)
	}

	if len(pf.Pricing) == 0 {
		return nil, fmt.Errorf("pricing file %s contains no pricing entries", path)
	}

	return &FilePricingProvider{prices: pf.Pricing}, nil
}

// HourlyCost returns the hourly cost for the given instance type from the pricing file.
// If spot is true and no explicit spot price is provided, the on-demand price
// is discounted by DefaultSpotDiscount.
func (p *FilePricingProvider) HourlyCost(instanceType string, _ string, spot bool) (float64, error) {
	entry, ok := p.prices[instanceType]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrNoPricing, instanceType)
	}
	if spot {
		if entry.SpotHourly > 0 {
			return entry.SpotHourly, nil
		}
		return entry.OnDemandHourly * DefaultSpotDiscount, nil
	}
	return entry.OnDemandHourly, nil
}

// Provider returns the provider name for file-based pricing.
func (p *FilePricingProvider) Provider() string {
	return ProviderFile
}
