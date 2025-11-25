package robots_txt_gateway

import (
	"alt/port/robots_txt_port"
	"alt/utils/security"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TDD RED PHASE: Write failing tests first

func TestRobotsTxtGateway_FetchRobotsTxt_Success(t *testing.T) {
	// Use RoundTripper to mock HTTP responses and avoid SSRF validation issues
	robotsTxtContent := `User-agent: *
Disallow: /admin/
Disallow: /private/
Crawl-delay: 5

User-agent: Alt-RSS-Reader
Disallow: /api/
Crawl-delay: 10`

	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(robotsTxtContent)),
			Header:     make(http.Header),
		}
		resp.Header.Set("Content-Type", "text/plain")
		return resp, nil
	})

	httpClient := &http.Client{
		Timeout:   5 * time.Second,
		Transport: rt,
	}
	// Use test-friendly constructor that preserves Transport
	ssrfValidator := security.NewSSRFValidator()
	gateway := NewRobotsTxtGatewayWithDeps(httpClient, ssrfValidator)

	// Execute with a valid public domain (SSRF validation will pass for example.com)
	ctx := context.Background()
	result, err := gateway.FetchRobotsTxt(ctx, "example.com", "https")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Contains(t, result.Content, "User-agent")
	assert.Contains(t, result.DisallowPaths, "/admin/")
	assert.Contains(t, result.DisallowPaths, "/private/")
	assert.Contains(t, result.DisallowPaths, "/api/")
	assert.Equal(t, 10, result.CrawlDelay) // Should use the higher crawl delay
	assert.False(t, result.FetchedAt.IsZero())
}

func TestRobotsTxtGateway_FetchRobotsTxt_NotFound(t *testing.T) {
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}
		return resp, nil
	})

	httpClient := &http.Client{Timeout: 5 * time.Second, Transport: rt}
	gateway := NewRobotsTxtGateway(httpClient)

	ctx := context.Background()
	result, err := gateway.FetchRobotsTxt(ctx, "example.com", "https")

	// Should not error, but return robots.txt with 404 status
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusNotFound, result.StatusCode)
	assert.Empty(t, result.DisallowPaths)
	assert.Equal(t, 0, result.CrawlDelay)
}

func TestRobotsTxtGateway_FetchRobotsTxt_EmptyRobotsTxt(t *testing.T) {
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}
		return resp, nil
	})

	httpClient := &http.Client{Timeout: 5 * time.Second, Transport: rt}
	gateway := NewRobotsTxtGateway(httpClient)

	ctx := context.Background()
	result, err := gateway.FetchRobotsTxt(ctx, "example.com", "https")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Empty(t, result.Content)
	assert.Empty(t, result.DisallowPaths)
	assert.Equal(t, 0, result.CrawlDelay)
}

func TestRobotsTxtGateway_FetchRobotsTxt_InvalidDomain(t *testing.T) {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	gateway := NewRobotsTxtGateway(httpClient)

	ctx := context.Background()
	// Invalid domain that should fail SSRF validation
	result, err := gateway.FetchRobotsTxt(ctx, "192.168.1.1", "http")

	// Should error due to SSRF protection
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestRobotsTxtGateway_ImplementsPort(t *testing.T) {
	// Verify that RobotsTxtGateway implements RobotsTxtPort interface
	var _ robots_txt_port.RobotsTxtPort = (*RobotsTxtGateway)(nil)
}

// roundTripperFunc is a helper to stub http.RoundTripper
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
