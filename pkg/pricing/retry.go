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
	"math/rand/v2"
	"time"

	"k8s.io/klog/v2"
)

// Retry configuration defaults for CLI use.
const (
	// defaultMaxAttempts is the total number of attempts before giving up.
	defaultMaxAttempts = 5
	// defaultInitialBackoff is the delay before the first retry.
	defaultInitialBackoff = 200 * time.Millisecond
	// defaultMaxBackoff caps the delay between retries.
	defaultMaxBackoff = 3 * time.Second
	// defaultMultiplier is the exponential backoff scaling factor.
	defaultMultiplier = 2.0
	// jitterBase is the lower bound of the jitter multiplier (±25% jitter = 0.75–1.25).
	jitterBase = 0.75
	// jitterRange is the range of the jitter multiplier added to jitterBase.
	jitterRange = 0.5
)

// RetryConfig holds parameters for the retry loop.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts (1 = no retry).
	MaxAttempts int
	// InitialBackoff is the delay before the first retry.
	InitialBackoff time.Duration
	// MaxBackoff caps the delay between retries.
	MaxBackoff time.Duration
	// Multiplier scales the backoff after each failure.
	Multiplier float64
}

// DefaultRetryConfig returns the standard retry configuration for CLI use:
// 5 attempts, 200ms initial backoff, 2x multiplier, 3s max.
// Worst-case total wait: ~6.2 seconds.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:    defaultMaxAttempts,
		InitialBackoff: defaultInitialBackoff,
		MaxBackoff:     defaultMaxBackoff,
		Multiplier:     defaultMultiplier,
	}
}

// NonRetryableError wraps an error that should not be retried.
// Typically used for authentication or authorization failures (HTTP 401/403)
// where retrying would not help without user intervention.
type NonRetryableError struct {
	Err error
}

// Error returns the underlying error message.
func (e *NonRetryableError) Error() string { return e.Err.Error() }

// Unwrap returns the underlying error for errors.Is/As compatibility.
func (e *NonRetryableError) Unwrap() error { return e.Err }

// Retry executes fn up to cfg.MaxAttempts times with exponential backoff and jitter.
// It stops immediately if fn returns a *NonRetryableError or the context is cancelled.
// Returns the last error annotated with the attempt count.
func Retry(ctx context.Context, cfg RetryConfig, fn func() error) error {
	backoff := cfg.InitialBackoff

	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Non-retryable errors fail immediately.
		if nre, ok := lastErr.(*NonRetryableError); ok {
			return nre.Err
		}

		if attempt == cfg.MaxAttempts {
			break
		}

		klog.V(2).InfoS("Retrying after transient error",
			"attempt", attempt, "maxAttempts", cfg.MaxAttempts,
			"backoff", backoff, "err", lastErr)

		// Wait with jitter (±25%).
		jitter := time.Duration(float64(backoff) * (jitterBase + rand.Float64()*jitterRange))
		timer := time.NewTimer(jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("retry cancelled after %d/%d attempts: %w", attempt, cfg.MaxAttempts, ctx.Err())
		case <-timer.C:
		}

		// Scale backoff for next iteration.
		backoff = time.Duration(float64(backoff) * cfg.Multiplier)
		backoff = min(backoff, cfg.MaxBackoff)
	}

	return fmt.Errorf("failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
}
