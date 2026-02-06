package service

import (
	"context"
	"testing"
	"time"

	"log/slog"

	"pre-processor/config"
)

// TestNewArticleFetcherServiceWithFactory tests factory-based constructor
func TestNewArticleFetcherServiceWithFactory(t *testing.T) {
	tests := map[string]struct {
		config      *config.Config
		expectEnvoy bool
		description string
	}{
		"factory_envoy_enabled": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  "http://test-envoy:8080",
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   30 * time.Second,
					UserAgent:      "test-factory-envoy",
				},
			},
			expectEnvoy: true,
			description: "Factory should create Envoy-enabled fetcher",
		},
		"factory_direct_http": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "test-factory-direct",
				},
			},
			expectEnvoy: false,
			description: "Factory should create direct HTTP fetcher",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			service := NewArticleFetcherServiceWithFactory(tc.config, logger)

			if service == nil {
				t.Errorf("%s: expected service but got nil", tc.description)
				return
			}

			fetcherService, ok := service.(*articleFetcherService)
			if !ok {
				t.Errorf("%s: expected *articleFetcherService but got different type", tc.description)
				return
			}

			if fetcherService.httpClient == nil {
				t.Errorf("%s: expected httpClient to be set but got nil", tc.description)
				return
			}

			clientType := getClientTypeName(fetcherService.httpClient)
			if tc.expectEnvoy && clientType != "EnvoyHTTPClient" {
				t.Errorf("%s: expected EnvoyHTTPClient but got %s", tc.description, clientType)
			}
			if !tc.expectEnvoy && clientType == "EnvoyHTTPClient" {
				t.Errorf("%s: expected non-Envoy client but got EnvoyHTTPClient", tc.description)
			}

			t.Logf("%s: created fetcher with client type: %s", tc.description, clientType)
		})
	}
}

// TestArticleFetcherFactory_Integration tests end-to-end factory integration
func TestArticleFetcherFactory_Integration(t *testing.T) {
	tests := map[string]struct {
		config      *config.Config
		targetURL   string
		description string
	}{
		"private_network_blocked": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "test-integration",
				},
			},
			targetURL:   "http://example.com",
			description: "Article fetching is disabled for ethical compliance",
		},
		"envoy_config_error": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  "",
					EnvoyProxyPath: "/proxy/https://",
				},
			},
			targetURL:   "https://example.com",
			description: "Article fetching is disabled for ethical compliance",
		},
		"invalid_url": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "test-integration",
				},
			},
			targetURL:   "invalid-url-format",
			description: "Article fetching is disabled for ethical compliance",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			service := NewArticleFetcherServiceWithFactory(tc.config, logger)

			ctx := context.Background()
			article, err := service.FetchArticle(ctx, tc.targetURL)

			if err != nil {
				t.Errorf("%s: unexpected error (article fetching disabled): %v", tc.description, err)
				return
			}

			if article != nil {
				t.Errorf("%s: expected nil article (fetching disabled) but got: %+v", tc.description, article)
				return
			}

			t.Logf("%s: article fetching disabled, returned nil as expected", tc.description)
		})
	}
}
