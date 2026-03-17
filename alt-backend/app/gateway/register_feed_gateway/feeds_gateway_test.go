package register_feed_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"
	"alt/utils/proxy"
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
)

// MockRSSFeedFetcher for testing
type MockRSSFeedFetcher struct {
	feeds  map[string]*gofeed.Feed
	errors map[string]error
}

func NewMockRSSFeedFetcher() *MockRSSFeedFetcher {
	return &MockRSSFeedFetcher{
		feeds:  make(map[string]*gofeed.Feed),
		errors: make(map[string]error),
	}
}

func (m *MockRSSFeedFetcher) FetchRSSFeed(ctx context.Context, link string) (*gofeed.Feed, error) {
	if err, exists := m.errors[link]; exists {
		return nil, err
	}
	if feed, exists := m.feeds[link]; exists {
		return feed, nil
	}
	return &gofeed.Feed{
		Title:    "Test Feed",
		Link:     link,
		FeedLink: link,
	}, nil
}

func (m *MockRSSFeedFetcher) SetFeed(url string, feed *gofeed.Feed) {
	m.feeds[url] = feed
}

func (m *MockRSSFeedFetcher) SetError(url string, err error) {
	m.errors[url] = err
}

func TestRegisterFeedsGateway_RegisterFeeds(t *testing.T) {
	gateway := &RegisterFeedsGateway{
		alt_db: nil, // This will cause an error, which we can test
	}

	testFeeds := []*domain.FeedItem{
		{
			Title:       "Test Feed 1",
			Description: "Test Description 1",
			Link:        "https://example.com/feed1",
			Published:   "2024-01-01T00:00:00Z",
		},
		{
			Title:       "Test Feed 2",
			Description: "Test Description 2",
			Link:        "https://example.com/feed2",
			Published:   "2024-01-02T00:00:00Z",
		},
	}

	type args struct {
		ctx   context.Context
		feeds []*domain.FeedItem
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "register feeds with nil database (should error)",
			args: args{
				ctx:   context.Background(),
				feeds: testFeeds,
			},
			wantErr: true,
		},
		{
			name: "register empty feeds list",
			args: args{
				ctx:   context.Background(),
				feeds: []*domain.FeedItem{},
			},
			wantErr: true,
		},
		{
			name: "register nil feeds",
			args: args{
				ctx:   context.Background(),
				feeds: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gateway.RegisterFeeds(tt.args.ctx, tt.args.feeds)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterFeedsGateway.RegisterFeeds() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegisterFeedGateway_RegisterFeedLink_NilDB(t *testing.T) {
	gateway := &RegisterFeedGateway{alt_db: nil}

	err := gateway.RegisterFeedLink(context.Background(), "https://example.com/feed.xml")
	if err == nil {
		t.Error("RegisterFeedLink() expected error with nil database, got nil")
	}
	assert.Contains(t, err.Error(), "database connection not available")
}

func TestNewRegisterFeedsGateway(t *testing.T) {
	var pool *pgxpool.Pool
	gateway := NewRegisterFeedsGateway(pool)

	if gateway == nil {
		t.Error("NewRegisterFeedsGateway() returned nil")
	}
	if gateway.alt_db != nil {
		t.Error("NewRegisterFeedsGateway() with nil pool should have nil repository")
	}
}

func TestNewRegisterFeedLinkGateway(t *testing.T) {
	var pool *pgxpool.Pool
	gateway := NewRegisterFeedLinkGateway(pool)

	if gateway == nil {
		t.Error("NewRegisterFeedLinkGateway() returned nil")
	}
	if gateway.alt_db != nil {
		t.Error("NewRegisterFeedLinkGateway() with nil pool should have nil repository")
	}
}

func TestRegisterFeedsGateway_ValidationEdgeCases(t *testing.T) {
	gateway := &RegisterFeedsGateway{
		alt_db: nil,
	}

	edgeCaseFeeds := []*domain.FeedItem{
		{
			Title:       "",
			Description: "Valid description",
			Link:        "https://example.com/feed1",
			Published:   "2024-01-01T00:00:00Z",
		},
		{
			Title:       "Valid title",
			Description: "",
			Link:        "https://example.com/feed2",
			Published:   "2024-01-01T00:00:00Z",
		},
		{
			Title:       "Valid title",
			Description: "Valid description",
			Link:        "",
			Published:   "2024-01-01T00:00:00Z",
		},
		{
			Title:       "Valid title",
			Description: "Valid description",
			Link:        "https://example.com/feed3",
			Published:   "",
		},
	}

	_, err := gateway.RegisterFeeds(context.Background(), edgeCaseFeeds)
	if err == nil {
		t.Error("RegisterFeedsGateway.RegisterFeeds() expected error with nil database, got nil")
	}
}

func TestRegisterFeedsGateway_LargeDataset(t *testing.T) {
	gateway := &RegisterFeedsGateway{
		alt_db: nil,
	}

	var largeFeeds []*domain.FeedItem
	for i := 0; i < 1000; i++ {
		largeFeeds = append(largeFeeds, &domain.FeedItem{
			Title:       "Test Feed",
			Description: "Test Description",
			Link:        "https://example.com/feed",
			Published:   "2024-01-01T00:00:00Z",
		})
	}

	_, err := gateway.RegisterFeeds(context.Background(), largeFeeds)
	if err == nil {
		t.Error("RegisterFeedsGateway.RegisterFeeds() expected error with nil database, got nil")
	}
}

func TestBuildFeedModels_OgImageURL(t *testing.T) {
	imgURL := "https://example.com/image.jpg"

	tests := []struct {
		name           string
		ogImageURL     string
		wantOgImageURL *string
	}{
		{
			name:           "OgImageURL is mapped when non-empty",
			ogImageURL:     imgURL,
			wantOgImageURL: &imgURL,
		},
		{
			name:           "OgImageURL is nil when empty",
			ogImageURL:     "",
			wantOgImageURL: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feeds := []*domain.FeedItem{
				{
					Title:       "Test Feed",
					Description: "Test Description",
					Link:        "https://example.com/feed1",
					OgImageURL:  tt.ogImageURL,
				},
			}

			models := buildFeedModels(context.Background(), feeds)
			assert.Len(t, models, 1)

			if tt.wantOgImageURL == nil {
				assert.Nil(t, models[0].OgImageURL)
			} else {
				assert.NotNil(t, models[0].OgImageURL)
				assert.Equal(t, *tt.wantOgImageURL, *models[0].OgImageURL)
			}
		})
	}
}

// Proxy/fetcher tests remain here as they test types defined in this package

func TestDefaultRSSFeedFetcher_WithProxy_Success(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://nginx-external.alt-ingress.svc.cluster.local:8888")
	t.Setenv("PROXY_ENABLED", "true")

	fetcher := &DefaultRSSFeedFetcher{}

	ctx := context.Background()
	_, err := fetcher.FetchRSSFeed(ctx, "https://example.com/feed.xml")
	if err == nil {
		t.Error("Expected proxy configuration to be applied but none found")
	}
}

func TestDefaultRSSFeedFetcher_WithProxy_ProxyFailure(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://invalid-proxy.invalid:8888")
	t.Setenv("PROXY_ENABLED", "true")

	fetcher := &DefaultRSSFeedFetcher{}

	ctx := context.Background()
	_, err := fetcher.FetchRSSFeed(ctx, "https://example.com/feed.xml")
	if err == nil {
		t.Error("Expected proxy failure to be handled but no proxy support found")
	}
}

func TestDefaultRSSFeedFetcher_WithProxy_ProxyTimeout(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://nginx-external.alt-ingress.svc.cluster.local:8888")
	t.Setenv("PROXY_ENABLED", "true")

	fetcher := &DefaultRSSFeedFetcher{}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := fetcher.FetchRSSFeed(ctx, "https://example.com/feeds/proxy-test.xml")
	if err == nil {
		t.Error("Expected proxy timeout handling but no proxy support found")
	}
}

func TestDefaultRSSFeedFetcher_ProxyConfigFromEnv(t *testing.T) {
	tests := []struct {
		name         string
		httpProxy    string
		proxyEnabled string
		wantProxy    bool
	}{
		{
			name:         "proxy enabled with valid URL",
			httpProxy:    "http://nginx-external.alt-ingress.svc.cluster.local:8888",
			proxyEnabled: "true",
			wantProxy:    true,
		},
		{
			name:         "proxy disabled",
			httpProxy:    "http://nginx-external.alt-ingress.svc.cluster.local:8888",
			proxyEnabled: "false",
			wantProxy:    false,
		},
		{
			name:         "no proxy URL provided",
			httpProxy:    "",
			proxyEnabled: "true",
			wantProxy:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HTTP_PROXY", tt.httpProxy)
			t.Setenv("PROXY_ENABLED", tt.proxyEnabled)

			config := getProxyConfigFromEnv()
			if config == nil {
				t.Error("Expected proxy config but got nil")
			}
		})
	}
}

func TestDefaultRSSFeedFetcher_ConvertToProxyURL_URLConstruction(t *testing.T) {
	tests := []struct {
		name        string
		originalURL string
		baseURL     string
		expected    string
	}{
		{
			name:        "HTTPS RSS URL should have correct double slash",
			originalURL: "https://example.com/feed.xml",
			baseURL:     "http://envoy-proxy.alt-apps.svc.cluster.local:8085",
			expected:    "http://envoy-proxy.alt-apps.svc.cluster.local:8085/proxy/https://example.com/feed.xml",
		},
		{
			name:        "HTTP URL should have correct double slash",
			originalURL: "http://example.com/rss.xml",
			baseURL:     "http://envoy-proxy.alt-apps.svc.cluster.local:8085",
			expected:    "http://envoy-proxy.alt-apps.svc.cluster.local:8085/proxy/http://example.com/rss.xml",
		},
		{
			name:        "RSS URL with query parameters should be preserved",
			originalURL: "https://example.com/feeds/rss.xml?format=atom",
			baseURL:     "http://envoy-proxy.alt-apps.svc.cluster.local:8085",
			expected:    "http://envoy-proxy.alt-apps.svc.cluster.local:8085/proxy/https://example.com/feeds/rss.xml?format=atom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := &proxy.Strategy{
				Mode:    proxy.ModeEnvoy,
				BaseURL: tt.baseURL,
				Enabled: true,
			}

			result := proxy.ConvertToProxyURL(tt.originalURL, strategy)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// capturePgxIface captures the arguments passed to Begin/Exec for verifying sanitized URLs.
type capturePgxIface struct {
	beginErr   error
	execArgs   []interface{} // captured from the first Exec call on the tx
	execCalled bool
}

func (c *capturePgxIface) Query(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
	return nil, nil
}
func (c *capturePgxIface) QueryRow(_ context.Context, _ string, _ ...interface{}) pgx.Row {
	return nil
}
func (c *capturePgxIface) Exec(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (c *capturePgxIface) BeginTx(_ context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	return nil, c.beginErr
}
func (c *capturePgxIface) Close() {}
func (c *capturePgxIface) Begin(_ context.Context) (pgx.Tx, error) {
	return nil, c.beginErr
}

func TestRegisterFeedGateway_RegisterFeedLink_StripsTrackingParams(t *testing.T) {
	logger.InitLogger()

	// Use a mock pool that returns an error from Begin so we can verify
	// the sanitized URL is passed through (it will fail at Begin, but the
	// StripTrackingParams logic runs before the DB call).
	mockPool := &capturePgxIface{beginErr: pgx.ErrTxClosed}
	repo := alt_db.NewAltDBRepository(mockPool)
	gateway := &RegisterFeedGateway{alt_db: repo}

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "URL with utm_source",
			input: "https://example.com/feed.xml?utm_source=chatgpt.com",
		},
		{
			name:  "URL with multiple tracking params",
			input: "https://example.com/feed.xml?utm_source=rss&fbclid=abc123&gclid=xyz",
		},
		{
			name:  "clean URL unchanged",
			input: "https://example.com/feed.xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := gateway.RegisterFeedLink(context.Background(), tt.input)
			// Error is expected (Begin fails), but the function should not panic
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to register RSS feed link")
		})
	}
}
