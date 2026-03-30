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
	"net/http"
	"strings"
)

// HTTP status code threshold for server errors.
const httpStatusServerError = 500

// authKeywords are substrings in error messages that indicate an authentication
// or authorization failure. Used by isAuthError to classify errors.
var authKeywords = []string{
	"credentials",
	"authentication",
	"authorization",
	"permission",
	"401",
	"403",
}

// classifyHTTPError wraps an HTTP status code error as retryable or non-retryable.
// 401/403 are non-retryable (credential/permission issue).
// 429 (rate limit) and 5xx (server error) are retryable.
// All other status codes are non-retryable.
func classifyHTTPError(statusCode int, format string, args ...any) error {
	err := fmt.Errorf(format, args...)
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return &NonRetryableError{Err: err}
	case statusCode == http.StatusTooManyRequests || statusCode >= httpStatusServerError:
		return err // retryable
	default:
		return &NonRetryableError{Err: err}
	}
}

// AWSSetupError returns an actionable error message for AWS pricing failures.
// It guides the user through network requirements and manual alternatives.
func AWSSetupError(region string, underlying error) error {
	return fmt.Errorf(`AWS pricing API failed for region %q.

The AWS Bulk Pricing API does not require credentials for public pricing data.
If you are behind a corporate proxy or firewall, ensure HTTPS access to:
  pricing.us-east-1.amazonaws.com

Alternatively, provide pricing data manually:
  kubectl whatif snapshot --pricing-file pricing.yaml

Underlying error: %w`, region, underlying)
}

// GCPSetupError returns an actionable error message for GCP pricing failures.
// If the error looks like an auth issue, it provides detailed credential setup
// instructions including required IAM roles.
func GCPSetupError(region string, underlying error) error {
	msg := fmt.Sprintf("GCP pricing API failed for region %q.\n\n", region)

	if isAuthError(underlying) {
		msg += `To configure credentials:
  1. Install the gcloud CLI: https://cloud.google.com/sdk/docs/install
  2. Authenticate:  gcloud auth application-default login
  3. Set a project: gcloud config set project YOUR_PROJECT_ID

Required IAM roles:
  - roles/billing.viewer   (for pricing data)
  - roles/compute.viewer   (for machine type specs)
`
	} else {
		msg += `Ensure the following are configured:
  - GCP credentials: gcloud auth application-default login
  - Project:         gcloud config set project YOUR_PROJECT_ID
  - IAM roles:       roles/billing.viewer, roles/compute.viewer
`
	}

	msg += `
Alternatively, provide pricing data manually:
  kubectl whatif snapshot --pricing-file pricing.yaml`

	return fmt.Errorf("%s\nUnderlying error: %w", msg, underlying)
}

// AzureSetupError returns an actionable error message for Azure pricing failures.
// It guides the user through network requirements and manual alternatives.
func AzureSetupError(region string, underlying error) error {
	return fmt.Errorf(`Azure pricing API failed for region %q.

The Azure Retail Prices API is public and does not require credentials.
If you are behind a corporate proxy or firewall, ensure HTTPS access to:
  prices.azure.com

Alternatively, provide pricing data manually:
  kubectl whatif snapshot --pricing-file pricing.yaml

Underlying error: %w`, region, underlying)
}

// isAuthError reports whether err looks like an authentication or authorization
// failure by scanning the error message for known keywords.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, kw := range authKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
