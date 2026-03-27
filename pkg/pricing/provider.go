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
	"errors"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/tochemey/kubewise/pkg/collector"
)

var (
	// ErrNoPricing is returned when pricing data is unavailable for an instance type.
	ErrNoPricing = errors.New("pricing data unavailable for instance type")
	// ErrUnknownProvider is returned when the cloud provider cannot be detected.
	ErrUnknownProvider = errors.New("unknown cloud provider")
)

// PricingProvider abstracts cloud provider pricing lookups.
type PricingProvider interface {
	// HourlyCost returns the hourly cost in USD for the given instance type and region.
	// If spot is true, returns the spot/preemptible price.
	HourlyCost(instanceType string, region string, spot bool) (float64, error)
	// Provider returns the provider name ("aws", "gcp", "azure", "file").
	Provider() string
}

// DetectProvider examines node labels to determine the cloud provider and region.
// Returns (provider, region). If detection fails, both are empty strings.
func DetectProvider(nodes []collector.NodeSnapshot) (string, string) {
	for _, node := range nodes {
		provider, region := detectFromLabels(node.Labels)
		if provider != "" {
			return provider, region
		}
	}
	return "", ""
}

func detectFromLabels(labels map[string]string) (string, string) {
	region := labels["topology.kubernetes.io/region"]

	// AWS: has eks.amazonaws.com labels or providerID prefix
	if _, ok := labels["eks.amazonaws.com/nodegroup"]; ok {
		return "aws", region
	}

	// GKE: has cloud.google.com labels
	if _, ok := labels["cloud.google.com/gke-nodepool"]; ok {
		return "gcp", region
	}

	// AKS: has agentpool label and kubernetes.azure.com labels
	if _, ok := labels["kubernetes.azure.com/cluster"]; ok {
		return "azure", region
	}

	// Heuristic fallback: infer from zone naming patterns
	zone := labels["topology.kubernetes.io/zone"]
	switch {
	case strings.HasPrefix(zone, "us-east-") || strings.HasPrefix(zone, "us-west-") ||
		strings.HasPrefix(zone, "eu-west-") || strings.HasPrefix(zone, "ap-"):
		// AWS zones look like us-east-1a
		if len(zone) > 0 && zone[len(zone)-1] >= 'a' && zone[len(zone)-1] <= 'f' {
			return "aws", region
		}
	case strings.Contains(zone, "-central1-") || strings.Contains(zone, "-east1-") ||
		strings.Contains(zone, "-west1-"):
		// GCP zones look like us-central1-a
		return "gcp", region
	}

	return "", region
}

// NewProvider creates a PricingProvider for the given cloud provider and region.
// It tries the cloud API first, then falls back to cache.
func NewProvider(ctx context.Context, providerName string, region string) (PricingProvider, error) {
	switch providerName {
	case "aws":
		p, err := NewAWSProvider(ctx, region)
		if err != nil {
			klog.V(1).InfoS("AWS pricing API failed, checking cache", "err", err)
			cached, cacheErr := GetCached("aws", region)
			if cacheErr == nil && len(cached) > 0 {
				return NewAWSProviderFromPrices(cached, defaultSpotDiscount), nil
			}
			return nil, fmt.Errorf("AWS pricing unavailable: %w (provide a YAML pricing file with --pricing-file)", err)
		}
		return p, nil
	case "gcp":
		p, err := NewGCPProvider(ctx, region)
		if err != nil {
			klog.V(1).InfoS("GCP pricing API failed, checking cache", "err", err)
			cached, cacheErr := GetCached("gcp", region)
			if cacheErr == nil && len(cached) > 0 {
				return &GCPProvider{precomputedPrices: cached, spotDiscount: defaultSpotDiscount}, nil
			}
			return nil, fmt.Errorf("GCP pricing unavailable: %w (provide a YAML pricing file with --pricing-file)", err)
		}
		return p, nil
	case "azure":
		p, err := NewAzureProvider(ctx, region)
		if err != nil {
			klog.V(1).InfoS("Azure pricing API failed, checking cache", "err", err)
			cached, cacheErr := GetCached("azure", region)
			if cacheErr == nil && len(cached) > 0 {
				return NewAzureProviderFromPrices(cached, defaultSpotDiscount), nil
			}
			return nil, fmt.Errorf("azure pricing unavailable: %w (provide a YAML pricing file with --pricing-file)", err)
		}
		return p, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, providerName)
	}
}
