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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tochemey/kubewise/pkg/collector"
)

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		name         string
		nodes        []collector.NodeSnapshot
		wantProvider string
		wantRegion   string
	}{
		{
			name: "AWS via EKS label",
			nodes: []collector.NodeSnapshot{
				{
					Labels: map[string]string{
						"eks.amazonaws.com/nodegroup":   "default-pool",
						"topology.kubernetes.io/region": "us-east-1",
					},
				},
			},
			wantProvider: "aws",
			wantRegion:   "us-east-1",
		},
		{
			name: "GCP via GKE label",
			nodes: []collector.NodeSnapshot{
				{
					Labels: map[string]string{
						"cloud.google.com/gke-nodepool": "default-pool",
						"topology.kubernetes.io/region": "us-central1",
					},
				},
			},
			wantProvider: "gcp",
			wantRegion:   "us-central1",
		},
		{
			name: "Azure via kubernetes.azure.com label",
			nodes: []collector.NodeSnapshot{
				{
					Labels: map[string]string{
						"kubernetes.azure.com/cluster":  "my-cluster",
						"topology.kubernetes.io/region": "eastus",
					},
				},
			},
			wantProvider: "azure",
			wantRegion:   "eastus",
		},
		{
			name:         "empty nodes",
			nodes:        []collector.NodeSnapshot{},
			wantProvider: "",
			wantRegion:   "",
		},
		{
			name: "unknown provider",
			nodes: []collector.NodeSnapshot{
				{
					Labels: map[string]string{
						"topology.kubernetes.io/region": "somewhere",
					},
				},
			},
			wantProvider: "",
			wantRegion:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, region := DetectProvider(tt.nodes)
			assert.Equal(t, tt.wantProvider, provider)
			assert.Equal(t, tt.wantRegion, region)
		})
	}
}
