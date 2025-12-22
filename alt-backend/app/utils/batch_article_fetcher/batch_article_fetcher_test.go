package batch_article_fetcher

import (
	"alt/gateway/fetch_article_gateway"
	"alt/utils/rate_limiter"
	"alt/utils/security"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBatchArticleFetcher_GroupByDomain(t *testing.T) {
	fetcher := NewBatchArticleFetcher(nil, nil)

	urls := []string{
		"https://example.com/article1",
		"https://example.com/article2",
		"https://example.com/article3",
		"https://test.com/article1",
		"https://test.com/article2",
		"https://other.com/article1",
	}

	groups := fetcher.groupByDomain(urls)

	// Check that URLs are grouped correctly
	if len(groups["example.com"]) != 3 {
		t.Errorf("Expected 3 URLs for example.com, got %d", len(groups["example.com"]))
	}
	if len(groups["test.com"]) != 2 {
		t.Errorf("Expected 2 URLs for test.com, got %d", len(groups["test.com"]))
	}
	if len(groups["other.com"]) != 1 {
		t.Errorf("Expected 1 URL for other.com, got %d", len(groups["other.com"]))
	}
}

func TestBatchArticleFetcher_FetchMultiple_10Domains13URLs(t *testing.T) {
	// Use public IPs to avoid DNS dependency in tests
	domains := []string{
		"93.184.216.34",
		"93.184.216.35",
		"93.184.216.36",
		"93.184.216.37",
		"93.184.216.38",
		"93.184.216.39",
		"93.184.216.40",
		"93.184.216.41",
		"93.184.216.42",
		"93.184.216.43",
	}

	// Track request counts per domain
	requestCounts := make(map[string]int)
	var mu sync.Mutex

	// Create stub transport that counts requests per host and returns HTML content
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		host := req.URL.Hostname()
		mu.Lock()
		requestCounts[host]++
		mu.Unlock()
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("<html><body>Test content for " + host + "</body></html>")),
			Header:     make(http.Header),
		}
		resp.Header.Set("Content-Type", "text/html")
		return resp, nil
	})

	// Create 13 URLs with some duplicates across domains
	// Domain distribution:
	// example1.com: 2 URLs
	// example2.com: 2 URLs
	// example3.com: 1 URL
	// example4.com: 1 URL
	// example5.com: 1 URL
	// example6.com: 1 URL
	// example7.com: 1 URL
	// example8.com: 1 URL
	// example9.com: 1 URL
	// example10.com: 2 URLs
	urls := []string{
		"http://" + domains[0] + "/article1",
		"http://" + domains[0] + "/article2",
		"http://" + domains[1] + "/article1",
		"http://" + domains[1] + "/article2",
		"http://" + domains[2] + "/article1",
		"http://" + domains[3] + "/article1",
		"http://" + domains[4] + "/article1",
		"http://" + domains[5] + "/article1",
		"http://" + domains[6] + "/article1",
		"http://" + domains[7] + "/article1",
		"http://" + domains[8] + "/article1",
		"http://" + domains[9] + "/article1",
		"http://" + domains[9] + "/article2",
	}

	// Create rate limiter with 5 second interval
	rateLimiter := rate_limiter.NewHostRateLimiter(5 * time.Second)

	// Create HTTP client
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: rt,
	}

	// Create fetcher with test SSRF validator that allows localhost
	testSSRFValidator := security.NewSSRFValidator()
	testSSRFValidator.SetTestingMode(true)

	gatewayFactory := func() *fetch_article_gateway.FetchArticleGateway {
		return fetch_article_gateway.NewFetchArticleGatewayWithDeps(rateLimiter, httpClient, testSSRFValidator)
	}

	fetcher := NewBatchArticleFetcherWithFactory(rateLimiter, httpClient, gatewayFactory)

	// Measure time
	start := time.Now()
	results := fetcher.FetchMultiple(context.Background(), urls)
	duration := time.Since(start)

	// Verify results
	if len(results) != 13 {
		t.Errorf("Expected 13 results, got %d", len(results))
	}

	// Verify all URLs have results
	for _, url := range urls {
		result, ok := results[url]
		if !ok {
			t.Errorf("Missing result for URL: %s", url)
			continue
		}
		if result.Error != nil {
			t.Errorf("Error fetching %s: %v", url, result.Error)
		}
		if result.Content == "" {
			t.Errorf("Empty content for URL: %s", url)
		}
	}

	// Verify that we only made 10 requests (one per domain)
	// Note: Due to rate limiting, requests to the same domain are sequential
	// So we should have exactly 10 unique domain requests
	totalRequests := 0
	for _, count := range requestCounts {
		totalRequests += count
	}

	// We expect 13 requests total (one per URL), but they should be grouped by domain
	if totalRequests != 13 {
		t.Errorf("Expected 13 total requests, got %d", totalRequests)
	}

	// Verify domain grouping: each domain should have the expected number of requests
	expectedCounts := map[string]int{
		domains[0]: 2,
		domains[1]: 2,
		domains[2]: 1,
		domains[3]: 1,
		domains[4]: 1,
		domains[5]: 1,
		domains[6]: 1,
		domains[7]: 1,
		domains[8]: 1,
		domains[9]: 2,
	}

	for domain, expected := range expectedCounts {
		actual := requestCounts[domain]
		if actual != expected {
			t.Errorf("Domain %s: expected %d requests, got %d", domain, expected, actual)
		}
	}

	// Verify that parallel processing occurred (should be faster than sequential)
	// With 10 domains processed in parallel, and 5 second rate limit per domain,
	// the slowest domain would take: 2 requests * 5 seconds = 10 seconds (first is immediate)
	// But since domains are parallel, total time should be around 10 seconds, not 65 seconds (13 * 5)
	// Actually, first request per domain is immediate, subsequent ones wait 5 seconds
	// So max time should be: max(2*5, 2*5, 1*0, ...) = 10 seconds for domains with 2 URLs
	// Sequential would be: 13 * 5 = 65 seconds
	if duration > 20*time.Second {
		t.Errorf("Fetch took too long (%v), suggesting sequential processing instead of parallel", duration)
	}

	t.Logf("Fetch completed in %v with %d domains processed in parallel", duration, len(domains))
}

func TestBatchArticleFetcher_FetchMultiple_EmptyURLs(t *testing.T) {
	rateLimiter := rate_limiter.NewHostRateLimiter(5 * time.Second)
	httpClient := &http.Client{Timeout: 30 * time.Second}
	fetcher := NewBatchArticleFetcher(rateLimiter, httpClient)

	results := fetcher.FetchMultiple(context.Background(), []string{})

	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty URLs, got %d", len(results))
	}
}

func TestBatchArticleFetcher_FetchMultiple_InvalidURLs(t *testing.T) {
	rateLimiter := rate_limiter.NewHostRateLimiter(5 * time.Second)
	httpClient := &http.Client{Timeout: 30 * time.Second}
	fetcher := NewBatchArticleFetcher(rateLimiter, httpClient)

	urls := []string{
		"not-a-valid-url",
		"://invalid",
		"",
	}

	results := fetcher.FetchMultiple(context.Background(), urls)

	// Invalid URLs should be skipped in grouping, so results may be empty or partial
	// The exact behavior depends on URL parsing
	if len(results) > len(urls) {
		t.Errorf("Results count (%d) should not exceed input URLs count (%d)", len(results), len(urls))
	}
}

func TestBatchArticleFetcher_GenerateArticleID(t *testing.T) {
	fetcher := NewBatchArticleFetcher(nil, nil)

	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "simple URL",
			url:      "https://example.com/article",
			expected: "article_https:__example.com_article",
		},
		{
			name:     "URL with path",
			url:      "https://example.com/path/to/article",
			expected: "article_https:__example.com_path_to_article",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fetcher.generateArticleID(tt.url)
			if result != tt.expected {
				t.Errorf("generateArticleID(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

// roundTripperFunc is a helper to stub http.RoundTripper in tests.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
