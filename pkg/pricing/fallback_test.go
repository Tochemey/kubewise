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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPricingFromFileValid(t *testing.T) {
	content := `
pricing:
  m6i.xlarge:
    on_demand_hourly: 0.192
    spot_hourly: 0.058
  m6i.2xlarge:
    on_demand_hourly: 0.384
    spot_hourly: 0.115
`
	path := filepath.Join(t.TempDir(), "pricing.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	provider, err := LoadPricingFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "file", provider.Provider())

	// On-demand
	cost, err := provider.HourlyCost("m6i.xlarge", "", false)
	require.NoError(t, err)
	assert.InDelta(t, 0.192, cost, 1e-9)

	// Spot with explicit price
	cost, err = provider.HourlyCost("m6i.xlarge", "", true)
	require.NoError(t, err)
	assert.InDelta(t, 0.058, cost, 1e-9)

	// Larger instance
	cost, err = provider.HourlyCost("m6i.2xlarge", "", false)
	require.NoError(t, err)
	assert.InDelta(t, 0.384, cost, 1e-9)
}

func TestLoadPricingFromFileSpotFallback(t *testing.T) {
	content := `
pricing:
  m6i.xlarge:
    on_demand_hourly: 0.192
`
	path := filepath.Join(t.TempDir(), "pricing.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	provider, err := LoadPricingFromFile(path)
	require.NoError(t, err)

	// Spot without explicit spot price falls back to discount
	cost, err := provider.HourlyCost("m6i.xlarge", "", true)
	require.NoError(t, err)
	assert.InDelta(t, 0.192*DefaultSpotDiscount, cost, 1e-9)
}

func TestLoadPricingFromFileUnknownInstance(t *testing.T) {
	content := `
pricing:
  m6i.xlarge:
    on_demand_hourly: 0.192
`
	path := filepath.Join(t.TempDir(), "pricing.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	provider, err := LoadPricingFromFile(path)
	require.NoError(t, err)

	_, err = provider.HourlyCost("unknown.type", "", false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoPricing))
}

func TestLoadPricingFromFileNotFound(t *testing.T) {
	_, err := LoadPricingFromFile("/nonexistent/pricing.yaml")
	require.Error(t, err)
}

func TestLoadPricingFromFileInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("{{{{not yaml"), 0o600))

	_, err := LoadPricingFromFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing pricing file")
}

func TestLoadPricingFromFileEmptyPricing(t *testing.T) {
	content := `
pricing:
`
	path := filepath.Join(t.TempDir(), "empty.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	_, err := LoadPricingFromFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no pricing entries")
}
