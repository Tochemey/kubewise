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
	"time"

	"github.com/tochemey/kubewise/pkg/collector"
)

// Cloud provider name constants used for detection and routing.
const (
	// ProviderAWS identifies Amazon Web Services.
	ProviderAWS = "aws"
	// ProviderGCP identifies Google Cloud Platform.
	ProviderGCP = "gcp"
	// ProviderAzure identifies Microsoft Azure.
	ProviderAzure = "azure"
	// ProviderFile identifies the file-based pricing provider.
	ProviderFile = "file"
)

// Kubernetes well-known node label keys used for cloud provider and topology detection.
const (
	// LabelTopologyRegion is the standard Kubernetes label for the cloud region.
	LabelTopologyRegion = "topology.kubernetes.io/region"
	// LabelTopologyZone is the standard Kubernetes label for the cloud zone.
	LabelTopologyZone = "topology.kubernetes.io/zone"
	// LabelEKSNodeGroup is the EKS-specific label indicating an AWS-managed node group.
	LabelEKSNodeGroup = "eks.amazonaws.com/nodegroup"
	// LabelGKENodePool is the GKE-specific label indicating a GCP-managed node pool.
	LabelGKENodePool = "cloud.google.com/gke-nodepool"
	// LabelAKSCluster is the AKS-specific label indicating an Azure-managed cluster.
	LabelAKSCluster = "kubernetes.azure.com/cluster"
)

// Shared pricing configuration constants.
const (
	// DefaultSpotDiscount is the default spot/preemptible discount multiplier.
	// 0.35 means spot costs 35% of on-demand (65% discount).
	DefaultSpotDiscount = 0.35

	// DefaultHTTPTimeout is the HTTP client timeout for pricing API requests.
	DefaultHTTPTimeout = 30 * time.Second

	// HoursPerMonth is the average number of hours in a month, used for cost projections.
	HoursPerMonth = 730

	// CurrencyUSD is the ISO-4217 currency code for US Dollars.
	CurrencyUSD = "USD"
)

// AWS zone suffix bounds for heuristic provider detection.
const (
	awsZoneSuffixMin = 'a'
	awsZoneSuffixMax = 'f'
)

var (
	// ErrNoPricing is returned when pricing data is unavailable for an instance type.
	ErrNoPricing = errors.New("pricing data unavailable for instance type")
	// ErrUnknownProvider is returned when the cloud provider cannot be detected.
	ErrUnknownProvider = errors.New("unknown cloud provider")
)

// PricingProvider abstracts cloud provider pricing lookups.
// Each implementation fetches and caches hourly instance costs from the
// respective cloud's pricing API.
type PricingProvider interface {
	// HourlyCost returns the hourly cost in USD for the given instance type and region.
	// If spot is true, returns the spot/preemptible price.
	HourlyCost(instanceType string, region string, spot bool) (float64, error)
	// Provider returns the provider name (ProviderAWS, ProviderGCP, ProviderAzure, or ProviderFile).
	Provider() string
}

// DetectProvider examines node labels to determine the cloud provider and region.
// It checks provider-specific labels first (EKS, GKE, AKS), then falls back
// to zone naming heuristics. Returns (provider, region); both are empty if
// detection fails.
func DetectProvider(nodes []collector.NodeSnapshot) (string, string) {
	for _, node := range nodes {
		provider, region := detectFromLabels(node.Labels)
		if provider != "" {
			return provider, region
		}
	}
	return "", ""
}

// detectFromLabels infers the cloud provider from Kubernetes node labels.
// It checks provider-specific labels first, then uses zone name patterns
// as a heuristic fallback.
func detectFromLabels(labels map[string]string) (string, string) {
	region := labels[LabelTopologyRegion]

	// AWS: has EKS-specific node group label.
	if _, ok := labels[LabelEKSNodeGroup]; ok {
		return ProviderAWS, region
	}

	// GKE: has GCP-specific node pool label.
	if _, ok := labels[LabelGKENodePool]; ok {
		return ProviderGCP, region
	}

	// AKS: has Azure-specific cluster label.
	if _, ok := labels[LabelAKSCluster]; ok {
		return ProviderAzure, region
	}

	// Heuristic fallback: infer from zone naming patterns.
	zone := labels[LabelTopologyZone]
	switch {
	case strings.HasPrefix(zone, "us-east-") || strings.HasPrefix(zone, "us-west-") ||
		strings.HasPrefix(zone, "eu-west-") || strings.HasPrefix(zone, "ap-"):
		// AWS zones look like us-east-1a (region + single letter suffix).
		if len(zone) > 0 && zone[len(zone)-1] >= awsZoneSuffixMin && zone[len(zone)-1] <= awsZoneSuffixMax {
			return ProviderAWS, region
		}
	case strings.Contains(zone, "-central1-") || strings.Contains(zone, "-east1-") ||
		strings.Contains(zone, "-west1-"):
		// GCP zones look like us-central1-a (region + dash + letter).
		return ProviderGCP, region
	}

	return "", region
}

// NewProvider creates a PricingProvider for the given cloud provider and region.
// It fetches pricing from the cloud API with retry and exponential backoff,
// falling back to the local cache if populated. Returns a descriptive error
// with setup instructions if all attempts fail.
func NewProvider(ctx context.Context, providerName string, region string) (PricingProvider, error) {
	switch providerName {
	case ProviderAWS:
		return NewAWSProvider(ctx, region)
	case ProviderGCP:
		return NewGCPProvider(ctx, region)
	case ProviderAzure:
		return NewAzureProvider(ctx, region)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, providerName)
	}
}
