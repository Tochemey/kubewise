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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cloudbilling "google.golang.org/api/cloudbilling/v1beta"
)

// testMachineSpecs provides machine specs for unit tests.
var testMachineSpecs = map[string]gcpMachineSpec{
	"n1-standard-1":  {1, 3.75},
	"n2-standard-4":  {4, 16},
	"n2-highmem-8":   {8, 64},
	"n2-highcpu-8":   {8, 8},
	"e2-standard-8":  {8, 32},
	"n2d-standard-4": {4, 16},
}

func TestGCPProviderFromRates(t *testing.T) {
	cpuHourly := 0.031611
	memHourly := 0.004237
	provider := NewGCPProviderFromRates(cpuHourly, memHourly, DefaultSpotDiscount, testMachineSpecs)

	t.Run("on-demand n2-standard-4", func(t *testing.T) {
		cost, err := provider.HourlyCost("n2-standard-4", "us-central1", false)
		require.NoError(t, err)
		expected := 4*cpuHourly + 16*memHourly
		assert.InDelta(t, expected, cost, 1e-6)
	})

	t.Run("spot pricing", func(t *testing.T) {
		cost, err := provider.HourlyCost("n2-standard-4", "us-central1", true)
		require.NoError(t, err)
		expected := (4*cpuHourly + 16*memHourly) * DefaultSpotDiscount
		assert.InDelta(t, expected, cost, 1e-6)
	})

	t.Run("n1-standard-1", func(t *testing.T) {
		cost, err := provider.HourlyCost("n1-standard-1", "us-central1", false)
		require.NoError(t, err)
		expected := 1*cpuHourly + 3.75*memHourly
		assert.InDelta(t, expected, cost, 1e-6)
	})

	t.Run("e2-standard-8", func(t *testing.T) {
		cost, err := provider.HourlyCost("e2-standard-8", "", false)
		require.NoError(t, err)
		expected := 8*cpuHourly + 32*memHourly
		assert.InDelta(t, expected, cost, 1e-6)
	})

	t.Run("unknown instance type", func(t *testing.T) {
		_, err := provider.HourlyCost("custom-machine-42", "", false)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoPricing))
	})

	t.Run("provider name", func(t *testing.T) {
		assert.Equal(t, "gcp", provider.Provider())
	})
}

func TestGCPProviderCPUMemorySeparatePricing(t *testing.T) {
	cpuHourly := 0.05
	memHourly := 0.01
	provider := NewGCPProviderFromRates(cpuHourly, memHourly, 0.30, testMachineSpecs)

	// n2-highmem-8: 8 vCPUs, 64 GB RAM
	cost, err := provider.HourlyCost("n2-highmem-8", "", false)
	require.NoError(t, err)
	assert.InDelta(t, 8*0.05+64*0.01, cost, 1e-9)

	// n2-highcpu-8: 8 vCPUs, 8 GB RAM
	cost, err = provider.HourlyCost("n2-highcpu-8", "", false)
	require.NoError(t, err)
	assert.InDelta(t, 8*0.05+8*0.01, cost, 1e-9)

	// Spot version
	cost, err = provider.HourlyCost("n2-highmem-8", "", true)
	require.NoError(t, err)
	assert.InDelta(t, (8*0.05+64*0.01)*0.30, cost, 1e-9)
}

func TestSharedCoreMachineTypesComplete(t *testing.T) {
	// Shared-core types are skipped by the Compute API (IsSharedCpu=true)
	// so they must be in gcpSharedCoreMachineTypes with correct fractional vCPUs.
	expected := map[string]gcpMachineSpec{
		"e2-micro":  {0.25, 1},
		"e2-small":  {0.5, 2},
		"e2-medium": {1, 4},
	}
	for name, want := range expected {
		spec, ok := gcpSharedCoreMachineTypes[name]
		assert.True(t, ok, "shared-core type %s missing", name)
		if ok {
			assert.Equal(t, want.VCPUs, spec.VCPUs, "wrong vCPUs for %s", name)
			assert.Equal(t, want.MemoryGB, spec.MemoryGB, "wrong memory for %s", name)
		}
	}
}

func TestIsComputeOnDemandGroup(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		expected    bool
	}{
		{"N2 on demand", "N2 Standard VMs (On Demand)", true},
		{"E2 on demand", "E2 VMs (On Demand)", true},
		{"N1 on demand", "N1 Standard VMs (On Demand)", true},
		{"N2D on demand", "N2D Standard VMs (On Demand)", true},
		{"CUD group", "N2 Standard VMs (1 Year CUD)", false},
		{"A2 group", "A2 VMs (On Demand)", false},
		{"unrelated", "Cloud Storage", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isComputeOnDemandGroup(tt.displayName))
		})
	}
}

func TestExtractFamilyFromSKU(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		expected    string
	}{
		{"N2 core", "N2 Instance Core running in Americas", "n2"},
		{"N2 ram", "N2 Instance Ram running in EMEA", "n2"},
		{"N2D core", "N2D Instance Core running in Americas", "n2d"},
		{"E2 core", "E2 Instance Core running in Americas", "e2"},
		{"N1 core", "N1 Predefined Instance Core running in Americas", "n1"},
		{"unknown", "A2 Instance Core running in Americas", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractFamilyFromSKU(tt.displayName))
		})
	}
}

func TestClassifyResource(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		expected    string
	}{
		{"core keyword", "N2 Instance Core running in Americas", "cpu"},
		{"cpu keyword", "N2 CPU running in Americas", "cpu"},
		{"vcpu keyword", "N2 vCPU running in Americas", "cpu"},
		{"ram keyword", "N2 Instance Ram running in EMEA", "ram"},
		{"memory keyword", "N2 Instance Memory running in EMEA", "ram"},
		{"unknown", "N2 Instance GPU running in Americas", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sku := &cloudbilling.GoogleCloudBillingSkugroupskusV1betaSkuGroupSku{
				DisplayName: tt.displayName,
			}
			assert.Equal(t, tt.expected, classifyResource(sku))
		})
	}
}

func TestClassifyResourceFromTaxonomy(t *testing.T) {
	sku := &cloudbilling.GoogleCloudBillingSkugroupskusV1betaSkuGroupSku{
		DisplayName: "Some Unknown SKU",
		ProductTaxonomy: &cloudbilling.GoogleCloudBillingSkugroupskusV1betaProductTaxonomy{
			TaxonomyCategories: []*cloudbilling.GoogleCloudBillingSkugroupskusV1betaTaxonomyCategory{
				{Category: "GCP"},
				{Category: "Compute"},
				{Category: "Cores"},
			},
		},
	}
	assert.Equal(t, "cpu", classifyResource(sku))

	sku.ProductTaxonomy.TaxonomyCategories[2].Category = "RAM"
	assert.Equal(t, "ram", classifyResource(sku))
}

func TestMachineTypeFamily(t *testing.T) {
	tests := []struct {
		machineType string
		expected    string
	}{
		{"n2-standard-4", "n2"},
		{"n2d-standard-8", "n2d"},
		{"n1-highmem-16", "n1"},
		{"e2-medium", "e2"},
		{"e2-micro", "e2"},
		{"custom-4-16384", ""},
	}
	for _, tt := range tests {
		t.Run(tt.machineType, func(t *testing.T) {
			assert.Equal(t, tt.expected, machineTypeFamily(tt.machineType))
		})
	}
}

func TestSKUMatchesRegion(t *testing.T) {
	t.Run("regional match", func(t *testing.T) {
		sku := &cloudbilling.GoogleCloudBillingSkugroupskusV1betaSkuGroupSku{
			GeoTaxonomy: &cloudbilling.GoogleCloudBillingSkugroupskusV1betaGeoTaxonomy{
				Type: "TYPE_REGIONAL",
				RegionalMetadata: &cloudbilling.GoogleCloudBillingSkugroupskusV1betaGeoTaxonomyRegional{
					Region: &cloudbilling.GoogleCloudBillingSkugroupskusV1betaGeoTaxonomyRegion{
						Region: "us-central1",
					},
				},
			},
		}
		assert.True(t, skuMatchesRegion(sku, "us-central1"))
		assert.False(t, skuMatchesRegion(sku, "us-east1"))
	})

	t.Run("case insensitive", func(t *testing.T) {
		sku := &cloudbilling.GoogleCloudBillingSkugroupskusV1betaSkuGroupSku{
			GeoTaxonomy: &cloudbilling.GoogleCloudBillingSkugroupskusV1betaGeoTaxonomy{
				Type: "TYPE_REGIONAL",
				RegionalMetadata: &cloudbilling.GoogleCloudBillingSkugroupskusV1betaGeoTaxonomyRegional{
					Region: &cloudbilling.GoogleCloudBillingSkugroupskusV1betaGeoTaxonomyRegion{
						Region: "US-CENTRAL1",
					},
				},
			},
		}
		assert.True(t, skuMatchesRegion(sku, "us-central1"))
	})

	t.Run("multi-regional match", func(t *testing.T) {
		sku := &cloudbilling.GoogleCloudBillingSkugroupskusV1betaSkuGroupSku{
			GeoTaxonomy: &cloudbilling.GoogleCloudBillingSkugroupskusV1betaGeoTaxonomy{
				Type: "TYPE_MULTI_REGIONAL",
				MultiRegionalMetadata: &cloudbilling.GoogleCloudBillingSkugroupskusV1betaGeoTaxonomyMultiRegional{
					Regions: []*cloudbilling.GoogleCloudBillingSkugroupskusV1betaGeoTaxonomyRegion{
						{Region: "us-central1"},
						{Region: "us-east1"},
					},
				},
			},
		}
		assert.True(t, skuMatchesRegion(sku, "us-central1"))
		assert.True(t, skuMatchesRegion(sku, "us-east1"))
		assert.False(t, skuMatchesRegion(sku, "eu-west1"))
	})

	t.Run("nil geo taxonomy", func(t *testing.T) {
		sku := &cloudbilling.GoogleCloudBillingSkugroupskusV1betaSkuGroupSku{}
		assert.False(t, skuMatchesRegion(sku, "us-central1"))
	})

	t.Run("global type no match", func(t *testing.T) {
		sku := &cloudbilling.GoogleCloudBillingSkugroupskusV1betaSkuGroupSku{
			GeoTaxonomy: &cloudbilling.GoogleCloudBillingSkugroupskusV1betaGeoTaxonomy{
				Type: "TYPE_GLOBAL",
			},
		}
		assert.False(t, skuMatchesRegion(sku, "us-central1"))
	})
}
