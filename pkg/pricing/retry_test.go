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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fastRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     5 * time.Millisecond,
		Multiplier:     2.0,
	}
}

func TestRetrySucceedsOnFirstAttempt(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), fastRetryConfig(), func() error {
		calls++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetrySucceedsAfterTransientFailures(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), fastRetryConfig(), func() error {
		calls++
		if calls < 3 {
			return fmt.Errorf("transient error %d", calls)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestRetryExhaustsAllAttempts(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), fastRetryConfig(), func() error {
		calls++
		return fmt.Errorf("persistent error")
	})
	require.Error(t, err)
	assert.Equal(t, 3, calls)
	assert.Contains(t, err.Error(), "failed after 3 attempts")
	assert.Contains(t, err.Error(), "persistent error")
}

func TestRetryStopsOnNonRetryableError(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), fastRetryConfig(), func() error {
		calls++
		return &NonRetryableError{Err: fmt.Errorf("auth failure: 403 forbidden")}
	})
	require.Error(t, err)
	assert.Equal(t, 1, calls, "should not retry non-retryable errors")
	assert.Contains(t, err.Error(), "403 forbidden")
}

func TestRetryRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	cfg := RetryConfig{
		MaxAttempts:    10,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := Retry(ctx, cfg, func() error {
		calls++
		return fmt.Errorf("keep failing")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retry cancelled")
	assert.Less(t, calls, 10, "should stop early due to context cancellation")
}

func TestRetryDefaultConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	assert.Equal(t, 5, cfg.MaxAttempts)
	assert.Equal(t, 200*time.Millisecond, cfg.InitialBackoff)
	assert.Equal(t, 3*time.Second, cfg.MaxBackoff)
	assert.Equal(t, 2.0, cfg.Multiplier)
}

func TestNonRetryableErrorUnwraps(t *testing.T) {
	inner := fmt.Errorf("inner error")
	nre := &NonRetryableError{Err: inner}
	assert.Equal(t, "inner error", nre.Error())
	assert.Equal(t, inner, nre.Unwrap())
}

func TestClassifyHTTPErrorNonRetryable(t *testing.T) {
	for _, code := range []int{401, 403, 404, 400} {
		err := classifyHTTPError(code, "status %d", code)
		nre, ok := err.(*NonRetryableError)
		assert.True(t, ok, "status %d should be non-retryable", code)
		assert.Contains(t, nre.Error(), fmt.Sprintf("status %d", code))
	}
}

func TestClassifyHTTPErrorRetryable(t *testing.T) {
	for _, code := range []int{429, 500, 502, 503, 504} {
		err := classifyHTTPError(code, "status %d", code)
		_, ok := err.(*NonRetryableError)
		assert.False(t, ok, "status %d should be retryable", code)
	}
}

func TestSetupErrorMessages(t *testing.T) {
	t.Run("AWS", func(t *testing.T) {
		err := AWSSetupError("us-east-1", fmt.Errorf("connection timeout"))
		msg := err.Error()
		assert.Contains(t, msg, "us-east-1")
		assert.Contains(t, msg, "--pricing-file")
		assert.Contains(t, msg, "connection timeout")
	})

	t.Run("GCP auth error", func(t *testing.T) {
		err := GCPSetupError("us-central1", fmt.Errorf("credentials not found"))
		msg := err.Error()
		assert.Contains(t, msg, "us-central1")
		assert.Contains(t, msg, "gcloud auth application-default login")
		assert.Contains(t, msg, "roles/billing.viewer")
		assert.Contains(t, msg, "roles/compute.viewer")
		assert.Contains(t, msg, "--pricing-file")
	})

	t.Run("GCP non-auth error", func(t *testing.T) {
		err := GCPSetupError("us-central1", fmt.Errorf("network timeout"))
		msg := err.Error()
		assert.Contains(t, msg, "us-central1")
		assert.Contains(t, msg, "--pricing-file")
		// Should still mention credentials setup as general guidance
		assert.Contains(t, msg, "gcloud auth")
	})

	t.Run("Azure", func(t *testing.T) {
		err := AzureSetupError("eastus", fmt.Errorf("connection refused"))
		msg := err.Error()
		assert.Contains(t, msg, "eastus")
		assert.Contains(t, msg, "prices.azure.com")
		assert.Contains(t, msg, "--pricing-file")
		assert.Contains(t, msg, "connection refused")
	})
}

func TestIsAuthError(t *testing.T) {
	assert.True(t, isAuthError(fmt.Errorf("credentials not found")))
	assert.True(t, isAuthError(fmt.Errorf("authentication failed")))
	assert.True(t, isAuthError(fmt.Errorf("permission denied")))
	assert.True(t, isAuthError(fmt.Errorf("HTTP 401 unauthorized")))
	assert.True(t, isAuthError(fmt.Errorf("HTTP 403 forbidden")))
	assert.False(t, isAuthError(fmt.Errorf("network timeout")))
	assert.False(t, isAuthError(fmt.Errorf("connection refused")))
	assert.False(t, isAuthError(nil))
}

func TestRetryWithClassifiedHTTPErrors(t *testing.T) {
	t.Run("retries 500 then succeeds", func(t *testing.T) {
		calls := 0
		err := Retry(context.Background(), fastRetryConfig(), func() error {
			calls++
			if calls == 1 {
				return classifyHTTPError(500, "server error")
			}
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 2, calls)
	})

	t.Run("stops on 403", func(t *testing.T) {
		calls := 0
		err := Retry(context.Background(), fastRetryConfig(), func() error {
			calls++
			return classifyHTTPError(403, "forbidden")
		})
		require.Error(t, err)
		assert.Equal(t, 1, calls)
		assert.True(t, strings.Contains(err.Error(), "forbidden"))
	})
}
