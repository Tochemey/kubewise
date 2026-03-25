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
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// pricingFile represents the YAML pricing file format.
type pricingFile struct {
	Pricing map[string]pricingEntry `yaml:"pricing"`
}

type pricingEntry struct {
	OnDemandHourly float64 `yaml:"on_demand_hourly"`
	SpotHourly     float64 `yaml:"spot_hourly"`
}

// FilePricingProvider implements PricingProvider using a YAML pricing file.
type FilePricingProvider struct {
	prices map[string]pricingEntry
}

// LoadPricingFromFile reads a YAML pricing file and returns a PricingProvider.
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

func (p *FilePricingProvider) HourlyCost(instanceType string, _ string, spot bool) (float64, error) {
	entry, ok := p.prices[instanceType]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrNoPricing, instanceType)
	}
	if spot {
		if entry.SpotHourly > 0 {
			return entry.SpotHourly, nil
		}
		// Fall back to on-demand if spot price not specified
		return entry.OnDemandHourly * defaultSpotDiscount, nil
	}
	return entry.OnDemandHourly, nil
}

func (p *FilePricingProvider) Provider() string {
	return "file"
}
