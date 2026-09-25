package register_feed_gateway

import (
	"math"
	"strings"
	"time"
)

// calculateBackoff computes exponential backoff delay capped at maxDelay.
func calculateBackoff(attempt int, initialDelay, maxDelay time.Duration) time.Duration {
	delay := time.Duration(float64(initialDelay) * math.Pow(2, float64(attempt-1)))
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504")
}
