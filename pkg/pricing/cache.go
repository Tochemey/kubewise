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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/klog/v2"
)

// Cache configuration constants.
const (
	// cacheTTL is the maximum age of a cached pricing file before it is considered stale.
	cacheTTL = 24 * time.Hour
	// cacheSubDir is the directory path under the user's home for pricing cache files.
	cacheSubDir = ".kubewise/pricing"
	// cacheFileFormat is the filename format for cached pricing data: provider_region.json.
	cacheFileFormat = "%s_%s.json"
	// cacheDirPerms is the filesystem permission for the cache directory.
	cacheDirPerms = 0o755
	// cacheFilePerms is the filesystem permission for cache files (owner-only read/write).
	cacheFilePerms = 0o600
)

// cacheDir can be overridden for testing via SetCacheDir.
var cacheDir string

// SetCacheDir overrides the cache directory path. Pass an empty string to
// restore the default (~/.kubewise/pricing). Intended for testing.
func SetCacheDir(dir string) {
	cacheDir = dir
}

// getCacheDir returns the resolved cache directory path.
func getCacheDir() string {
	if cacheDir != "" {
		return cacheDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, cacheSubDir)
}

// cacheFilePath returns the full path for a provider/region cache file.
func cacheFilePath(provider, region string) string {
	return filepath.Join(getCacheDir(), fmt.Sprintf(cacheFileFormat, provider, region))
}

// GetCached returns cached pricing data if it exists and is within the TTL.
// Returns an error if the cache is missing, expired, or corrupted.
func GetCached(provider, region string) (map[string]float64, error) {
	dir := getCacheDir()
	if dir == "" {
		return nil, fmt.Errorf("cache directory not available")
	}

	path := cacheFilePath(provider, region)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cache file not found: %w", err)
	}

	// Check TTL
	if time.Since(info.ModTime()) > cacheTTL {
		klog.V(2).InfoS("Cache expired", "path", path, "age", time.Since(info.ModTime()))
		return nil, fmt.Errorf("cache expired")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cache file: %w", err)
	}

	var prices map[string]float64
	if err := json.Unmarshal(data, &prices); err != nil {
		// Corrupted cache — remove it
		klog.V(1).InfoS("Removing corrupted cache file", "path", path)
		_ = os.Remove(path)
		return nil, fmt.Errorf("parsing cache file: %w", err)
	}

	klog.V(2).InfoS("Cache hit", "provider", provider, "region", region, "instanceTypes", len(prices))
	return prices, nil
}

// SetCached writes pricing data to the cache. The file is created with
// owner-only permissions (0600) under ~/.kubewise/pricing/.
func SetCached(provider, region string, prices map[string]float64) error {
	dir := getCacheDir()
	if dir == "" {
		return fmt.Errorf("cache directory not available")
	}

	if err := os.MkdirAll(dir, cacheDirPerms); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	data, err := json.MarshalIndent(prices, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cache data: %w", err)
	}

	path := cacheFilePath(provider, region)
	if err := os.WriteFile(path, data, cacheFilePerms); err != nil {
		return fmt.Errorf("writing cache file: %w", err)
	}

	klog.V(2).InfoS("Cache written", "path", path, "instanceTypes", len(prices))
	return nil
}

// ClearCache removes all cached pricing data.
func ClearCache() error {
	dir := getCacheDir()
	if dir == "" {
		return nil
	}
	return os.RemoveAll(dir)
}
