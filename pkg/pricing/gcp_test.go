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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCPProviderFromRates(t *testing.T) {
	// N2-standard-4: 4 vCPUs, 16 GB RAM
	// Cost = 4 * 0.031611 + 16 * 0.004237 = 0.126444 + 0.067792 = 0.194236
	cpuHourly := 0.031611
	memHourly := 0.004237
	provider := NewGCPProviderFromRates(cpuHourly, memHourly, defaultSpotDiscount)

	t.Run("on-demand n2-standard-4", func(t *testing.T) {
		cost, err := provider.HourlyCost("n2-standard-4", "us-central1", false)
		require.NoError(t, err)
		expected := 4*cpuHourly + 16*memHourly
		assert.InDelta(t, expected, cost, 1e-6)
	})

	t.Run("spot pricing", func(t *testing.T) {
		cost, err := provider.HourlyCost("n2-standard-4", "us-central1", true)
		require.NoError(t, err)
		expected := (4*cpuHourly + 16*memHourly) * defaultSpotDiscount
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
	// Verify that GCP pricing correctly computes cost as (vCPUs * cpu_rate) + (GB_RAM * mem_rate)
	cpuHourly := 0.05
	memHourly := 0.01
	provider := NewGCPProviderFromRates(cpuHourly, memHourly, 0.30)

	// n2-highmem-8: 8 vCPUs, 64 GB RAM
	cost, err := provider.HourlyCost("n2-highmem-8", "", false)
	require.NoError(t, err)
	assert.InDelta(t, 8*0.05+64*0.01, cost, 1e-9) // 0.4 + 0.64 = 1.04

	// n2-highcpu-8: 8 vCPUs, 8 GB RAM — much cheaper due to less memory
	cost, err = provider.HourlyCost("n2-highcpu-8", "", false)
	require.NoError(t, err)
	assert.InDelta(t, 8*0.05+8*0.01, cost, 1e-9) // 0.4 + 0.08 = 0.48

	// Spot version
	cost, err = provider.HourlyCost("n2-highmem-8", "", true)
	require.NoError(t, err)
	assert.InDelta(t, (8*0.05+64*0.01)*0.30, cost, 1e-9)
}

func TestGCPMachineTypeLookup(t *testing.T) {
	// Verify all expected machine type families exist
	families := []string{"n1-standard-", "n2-standard-", "e2-standard-", "n2d-standard-"}
	for _, family := range families {
		found := false
		for machineType := range gcpMachineTypes {
			if len(machineType) > len(family) && machineType[:len(family)] == family {
				found = true
				break
			}
		}
		assert.True(t, found, "missing machine type family: %s", family)
	}
}

func TestGCPMoneyToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		money    gcpMoney
		expected float64
	}{
		{"zero", gcpMoney{Units: "0", Nanos: 0}, 0.0},
		{"whole units", gcpMoney{Units: "1", Nanos: 0}, 1.0},
		{"nanos only", gcpMoney{Units: "0", Nanos: 31611000}, 0.031611},
		{"combined", gcpMoney{Units: "1", Nanos: 500000000}, 1.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.expected, tt.money.ToFloat64(), 1e-9)
		})
	}
}

func TestContainsRegion(t *testing.T) {
	assert.True(t, containsRegion([]string{"us-central1", "us-east1"}, "us-central1"))
	assert.True(t, containsRegion([]string{"US-CENTRAL1"}, "us-central1"))
	assert.False(t, containsRegion([]string{"us-east1"}, "us-central1"))
	assert.False(t, containsRegion(nil, "us-central1"))
}
