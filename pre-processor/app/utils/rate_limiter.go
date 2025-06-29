// ABOUTME: This file implements enhanced rate limiting with exponential backoff and jitter
// ABOUTME: Provides rate-limited HTTP client with circuit breaker integration
package utils

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiterMetrics holds metrics for the rate limiter
type RateLimiterMetrics struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	LastRequestTime    time.Time
	AverageInterval    time.Duration
}

// RateLimitedHTTPClient provides rate limiting with exponential backoff and circuit breaker
type RateLimitedHTTPClient struct {
	client         *http.Client
	rateLimiter    *RateLimiter
	circuitBreaker *CircuitBreaker
	logger         *slog.Logger
	userAgent      string
	maxRetries     int
	metrics        RateLimiterMetrics
	mu             sync.RWMutex
}

// RateLimiter enforces minimum intervals between requests with jitter
type RateLimiter struct {
	lastRequest time.Time
	interval    time.Duration
	mu          sync.Mutex
}

// NewRateLimitedHTTPClient creates a new rate-limited HTTP client
func NewRateLimitedHTTPClient(interval time.Duration, maxRetries int, timeout time.Duration) *RateLimitedHTTPClient {
	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   2,
		},
	}

	return &RateLimitedHTTPClient{
		client:      httpClient,
		rateLimiter: NewRateLimiter(interval),
		logger:      slog.Default(),
		userAgent:   "pre-processor/1.0 (+https://alt.example.com/bot)",
		maxRetries:  maxRetries,
	}
}

// NewRateLimitedHTTPClientWithCircuitBreaker creates a client with circuit breaker
func NewRateLimitedHTTPClientWithCircuitBreaker(
	interval time.Duration,
	maxRetries int,
	timeout time.Duration,
	circuitThreshold int,
	circuitTimeout time.Duration,
) *RateLimitedHTTPClient {
	client := NewRateLimitedHTTPClient(interval, maxRetries, timeout)
	client.circuitBreaker = NewCircuitBreaker(circuitThreshold, circuitTimeout)
	return client
}

// NewRateLimiter creates a new rate limiter with specified interval
func NewRateLimiter(interval time.Duration) *RateLimiter {
	return &RateLimiter{
		interval: interval,
	}
}

// randomFraction returns a random float64 in the range [0, max). It uses
// crypto/rand to avoid gosec G404 warnings. If randomness fails, 0 is returned.
func randomFraction(max float64) float64 {
	const precision = 1_000_000
	n, err := crand.Int(crand.Reader, big.NewInt(precision))
	if err != nil {
		return 0
	}
	return (float64(n.Int64()) / precision) * max
}

// Wait waits for the rate limit interval with jitter
func (r *RateLimiter) Wait() {
	r.mu.Lock()
	defer r.mu.Unlock()

	elapsed := time.Since(r.lastRequest)

	// Add jitter up to +20% of the interval to reduce thundering herd
	// Jitter should never shorten the wait below the base interval
	jitter := time.Duration(randomFraction(0.2) * float64(r.interval))
	waitTime := r.interval + jitter

	if elapsed < waitTime {
		time.Sleep(waitTime - elapsed)
	}
	r.lastRequest = time.Now()
}

// Get performs a GET request with rate limiting
func (c *RateLimitedHTTPClient) Get(url string) (*http.Response, error) {
	return c.GetWithContext(context.Background(), url)
}

// GetWithContext performs a GET request with context and rate limiting
func (c *RateLimitedHTTPClient) GetWithContext(ctx context.Context, url string) (*http.Response, error) {
	c.rateLimiter.Wait()

	c.logger.Info("making external request",
		"url", url,
		"timestamp", time.Now(),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		c.updateMetrics(false)
		return nil, fmt.Errorf("request creation failed: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)

	var resp *http.Response
	if c.circuitBreaker != nil {
		err = c.circuitBreaker.Call(func() error {
			resp, err = c.client.Do(req)
			if err != nil {
				return err
			}
			// Treat 5xx status codes as failures for circuit breaker
			if resp.StatusCode >= 500 {
				return fmt.Errorf("server error: %d", resp.StatusCode)
			}
			return nil
		})
	} else {
		resp, err = c.client.Do(req)
	}

	if err != nil {
		c.updateMetrics(false)
		return nil, err
	}

	c.updateMetrics(true)
	return resp, nil
}

// GetWithRetry performs a GET request with exponential backoff retry
func (c *RateLimitedHTTPClient) GetWithRetry(ctx context.Context, url string) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			jitter := time.Duration(randomFraction(0.1) * float64(backoff))

			select {
			case <-time.After(backoff + jitter):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		resp, err = c.GetWithContext(ctx, url)
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}

		if resp != nil {
			if cerr := resp.Body.Close(); cerr != nil {
				c.logger.Error("response body close failed", "error", cerr)
			}
		}

		c.logger.Warn("request failed, retrying",
			"url", url,
			"attempt", attempt+1,
			"max_retries", c.maxRetries,
			"error", err,
		)
	}

	return nil, fmt.Errorf("max retries (%d) exceeded: %w", c.maxRetries, err)
}

// Wait waits for the rate limiter interval
func (c *RateLimitedHTTPClient) Wait() {
	c.rateLimiter.Wait()
}

// Metrics returns current metrics for the client
func (c *RateLimitedHTTPClient) Metrics() RateLimiterMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.metrics
}

// updateMetrics updates the client metrics
func (c *RateLimitedHTTPClient) updateMetrics(success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics.TotalRequests++
	c.metrics.LastRequestTime = time.Now()

	if success {
		c.metrics.SuccessfulRequests++
	} else {
		c.metrics.FailedRequests++
	}

	// Calculate average interval (simplified)
	if c.metrics.TotalRequests > 1 {
		c.metrics.AverageInterval = time.Since(c.metrics.LastRequestTime) / time.Duration(c.metrics.TotalRequests)
	}
}
