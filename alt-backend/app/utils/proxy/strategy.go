package proxy

import (
	"context"
	"os"

	"alt/utils/logger"
)

// GetStrategy determines the appropriate proxy strategy based on environment configuration.
// Priority order: SIDECAR > ENVOY > NGINX > DISABLED
//
// Environment variables:
//   - SIDECAR_PROXY_ENABLED: "true" to enable sidecar mode
//   - SIDECAR_PROXY_URL: override default sidecar URL
//   - ENVOY_PROXY_ENABLED: "true" to enable envoy mode
//   - ENVOY_PROXY_URL: override default envoy URL
//   - NGINX_PROXY_ENABLED: "true" to enable nginx mode (RSS-only)
//   - NGINX_PROXY_URL: override default nginx URL
//
// Note: This function is typically called once at startup. The context.Background()
// usage is intentional for startup-time logging.
func GetStrategy() *Strategy {
	// Priority order: SIDECAR > ENVOY > NGINX > DISABLED
	if os.Getenv("SIDECAR_PROXY_ENABLED") == "true" {
		baseURL := os.Getenv("SIDECAR_PROXY_URL")
		if baseURL == "" {
			baseURL = DefaultSidecarProxyURL
		}
		logger.SafeInfoContext(context.Background(), "Proxy strategy: SIDECAR mode selected",
			"base_url", baseURL,
			"path_template", "/proxy/{scheme}://{host}{path}")
		return &Strategy{
			Mode:         ModeSidecar,
			BaseURL:      baseURL,
			PathTemplate: "/proxy/{scheme}://{host}{path}",
			Enabled:      true,
		}
	}

	if os.Getenv("ENVOY_PROXY_ENABLED") == "true" {
		baseURL := os.Getenv("ENVOY_PROXY_URL")
		if baseURL == "" {
			baseURL = DefaultEnvoyProxyURL
		}
		logger.SafeInfoContext(context.Background(), "Proxy strategy: ENVOY mode selected",
			"base_url", baseURL,
			"path_template", "/proxy/{scheme}://{host}{path}")
		return &Strategy{
			Mode:         ModeEnvoy,
			BaseURL:      baseURL,
			PathTemplate: "/proxy/{scheme}://{host}{path}",
			Enabled:      true,
		}
	}

	if os.Getenv("NGINX_PROXY_ENABLED") == "true" {
		baseURL := os.Getenv("NGINX_PROXY_URL")
		if baseURL == "" {
			baseURL = DefaultNginxProxyURL
		}
		logger.SafeInfoContext(context.Background(), "Proxy strategy: NGINX mode selected",
			"base_url", baseURL,
			"path_template", "/rss-proxy/{scheme}://{host}{path}")
		return &Strategy{
			Mode:         ModeNginx,
			BaseURL:      baseURL,
			PathTemplate: "/rss-proxy/{scheme}://{host}{path}",
			Enabled:      true,
		}
	}

	logger.SafeInfoContext(context.Background(), "Proxy strategy: DISABLED mode - direct connection will be used")
	return &Strategy{
		Mode:         ModeDisabled,
		BaseURL:      "",
		PathTemplate: "",
		Enabled:      false,
	}
}

// GetStrategyWithContext is like GetStrategy but uses the provided context for logging.
// Use this when proxy strategy needs to be determined during request handling
// rather than at application startup.
func GetStrategyWithContext(ctx context.Context) *Strategy {
	// Priority order: SIDECAR > ENVOY > NGINX > DISABLED
	if os.Getenv("SIDECAR_PROXY_ENABLED") == "true" {
		baseURL := os.Getenv("SIDECAR_PROXY_URL")
		if baseURL == "" {
			baseURL = DefaultSidecarProxyURL
		}
		logger.SafeInfoContext(ctx, "Proxy strategy: SIDECAR mode selected",
			"base_url", baseURL,
			"path_template", "/proxy/{scheme}://{host}{path}")
		return &Strategy{
			Mode:         ModeSidecar,
			BaseURL:      baseURL,
			PathTemplate: "/proxy/{scheme}://{host}{path}",
			Enabled:      true,
		}
	}

	if os.Getenv("ENVOY_PROXY_ENABLED") == "true" {
		baseURL := os.Getenv("ENVOY_PROXY_URL")
		if baseURL == "" {
			baseURL = DefaultEnvoyProxyURL
		}
		logger.SafeInfoContext(ctx, "Proxy strategy: ENVOY mode selected",
			"base_url", baseURL,
			"path_template", "/proxy/{scheme}://{host}{path}")
		return &Strategy{
			Mode:         ModeEnvoy,
			BaseURL:      baseURL,
			PathTemplate: "/proxy/{scheme}://{host}{path}",
			Enabled:      true,
		}
	}

	if os.Getenv("NGINX_PROXY_ENABLED") == "true" {
		baseURL := os.Getenv("NGINX_PROXY_URL")
		if baseURL == "" {
			baseURL = DefaultNginxProxyURL
		}
		logger.SafeInfoContext(ctx, "Proxy strategy: NGINX mode selected",
			"base_url", baseURL,
			"path_template", "/rss-proxy/{scheme}://{host}{path}")
		return &Strategy{
			Mode:         ModeNginx,
			BaseURL:      baseURL,
			PathTemplate: "/rss-proxy/{scheme}://{host}{path}",
			Enabled:      true,
		}
	}

	logger.SafeInfoContext(ctx, "Proxy strategy: DISABLED mode - direct connection will be used")
	return &Strategy{
		Mode:         ModeDisabled,
		BaseURL:      "",
		PathTemplate: "",
		Enabled:      false,
	}
}
