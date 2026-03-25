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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	SetCacheDir(tmpDir)
	defer SetCacheDir("")

	prices := map[string]float64{
		"m6i.xlarge":  0.192,
		"m6i.2xlarge": 0.384,
	}

	err := SetCached("aws", "us-east-1", prices)
	require.NoError(t, err)

	cached, err := GetCached("aws", "us-east-1")
	require.NoError(t, err)
	assert.InDelta(t, 0.192, cached["m6i.xlarge"], 1e-9)
	assert.InDelta(t, 0.384, cached["m6i.2xlarge"], 1e-9)
}

func TestCacheMiss(t *testing.T) {
	tmpDir := t.TempDir()
	SetCacheDir(tmpDir)
	defer SetCacheDir("")

	_, err := GetCached("aws", "us-west-2")
	require.Error(t, err)
}

func TestCacheTTLExpiry(t *testing.T) {
	tmpDir := t.TempDir()
	SetCacheDir(tmpDir)
	defer SetCacheDir("")

	prices := map[string]float64{"m6i.xlarge": 0.192}
	err := SetCached("aws", "us-east-1", prices)
	require.NoError(t, err)

	// Manually set the file's modification time to 25 hours ago
	path := cacheFilePath("aws", "us-east-1")
	oldTime := time.Now().Add(-25 * time.Hour)
	require.NoError(t, os.Chtimes(path, oldTime, oldTime))

	_, err = GetCached("aws", "us-east-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestCacheCorruption(t *testing.T) {
	tmpDir := t.TempDir()
	SetCacheDir(tmpDir)
	defer SetCacheDir("")

	// Write corrupted data
	dir := filepath.Join(tmpDir)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := cacheFilePath("aws", "us-east-1")
	require.NoError(t, os.WriteFile(path, []byte("not json{{{"), 0o600))

	_, err := GetCached("aws", "us-east-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing cache file")

	// Corrupted file should be removed
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestCacheCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "deep", "nested")
	SetCacheDir(nestedDir)
	defer SetCacheDir("")

	prices := map[string]float64{"m6i.xlarge": 0.192}
	err := SetCached("aws", "us-east-1", prices)
	require.NoError(t, err)

	// Verify directory was created
	_, err = os.Stat(nestedDir)
	require.NoError(t, err)
}

func TestCacheDifferentRegions(t *testing.T) {
	tmpDir := t.TempDir()
	SetCacheDir(tmpDir)
	defer SetCacheDir("")

	require.NoError(t, SetCached("aws", "us-east-1", map[string]float64{"m6i.xlarge": 0.192}))
	require.NoError(t, SetCached("aws", "eu-west-1", map[string]float64{"m6i.xlarge": 0.210}))

	east, err := GetCached("aws", "us-east-1")
	require.NoError(t, err)
	assert.InDelta(t, 0.192, east["m6i.xlarge"], 1e-9)

	west, err := GetCached("aws", "eu-west-1")
	require.NoError(t, err)
	assert.InDelta(t, 0.210, west["m6i.xlarge"], 1e-9)
}
