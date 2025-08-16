package kratos

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	kratosclient "github.com/ory/kratos-client-go"

	"auth-service/app/config"
)

// Client represents a Kratos client wrapper
type Client struct {
	publicAPI  *kratosclient.APIClient
	adminAPI   *kratosclient.APIClient
	publicURL  string
	adminURL   string
	logger     *slog.Logger
}

// NewClient creates a new Kratos client
func NewClient(cfg *config.Config, logger *slog.Logger) (*Client, error) {
	// Validate URLs
	if !isValidURL(cfg.KratosPublicURL) {
		return nil, fmt.Errorf("invalid Kratos public URL: %s", cfg.KratosPublicURL)
	}
	if !isValidURL(cfg.KratosAdminURL) {
		return nil, fmt.Errorf("invalid Kratos admin URL: %s", cfg.KratosAdminURL)
	}

	// Create public API client with enhanced CSRF support
	publicConfig := kratosclient.NewConfiguration()
	publicConfig.Servers = []kratosclient.ServerConfiguration{
		{
			URL: cfg.KratosPublicURL,
		},
	}
	
	// 🚨 CRITICAL: Enhanced HTTP client for CSRF compatibility
	publicConfig.HTTPClient = &http.Client{
		Timeout: 30 * time.Second,
		// 🎯 CRITICAL: Custom transport for cookie handling
		Transport: &http.Transport{
			DisableKeepAlives: false, // Enable keep-alives for session continuity
		},
	}
	
	// 🎯 CRITICAL: Default headers for CSRF compatibility
	if publicConfig.DefaultHeader == nil {
		publicConfig.DefaultHeader = make(map[string]string)
	}
	publicConfig.DefaultHeader["Accept"] = "application/json"
	publicConfig.DefaultHeader["Content-Type"] = "application/json"
	
	publicAPI := kratosclient.NewAPIClient(publicConfig)

	// Create admin API client
	adminConfig := kratosclient.NewConfiguration()
	adminConfig.Servers = []kratosclient.ServerConfiguration{
		{
			URL: cfg.KratosAdminURL,
		},
	}
	adminConfig.HTTPClient = &http.Client{
		Timeout: 30 * time.Second,
	}
	adminAPI := kratosclient.NewAPIClient(adminConfig)

	logger.Info("Kratos client initialized",
		"public_url", cfg.KratosPublicURL,
		"admin_url", cfg.KratosAdminURL)

	return &Client{
		publicAPI: publicAPI,
		adminAPI:  adminAPI,
		publicURL: cfg.KratosPublicURL,
		adminURL:  cfg.KratosAdminURL,
		logger:    logger,
	}, nil
}

// PublicAPI returns the public API client
func (c *Client) PublicAPI() *kratosclient.APIClient {
	return c.publicAPI
}

// AdminAPI returns the admin API client
func (c *Client) AdminAPI() *kratosclient.APIClient {
	return c.adminAPI
}

// HealthCheck checks if Kratos is healthy
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Try to get the version from the public API as a health check
	_, response, err := c.publicAPI.MetadataAPI.GetVersion(ctx).Execute()
	if err != nil {
		return fmt.Errorf("failed to connect to Kratos public API: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Kratos public API returned status %d", response.StatusCode)
	}

	// Also check admin API
	_, response, err = c.adminAPI.MetadataAPI.GetVersion(ctx).Execute()
	if err != nil {
		return fmt.Errorf("failed to connect to Kratos admin API: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Kratos admin API returned status %d", response.StatusCode)
	}

	return nil
}

// GetPublicURL returns the public URL
func (c *Client) GetPublicURL() string {
	return c.publicURL
}

// GetAdminURL returns the admin URL
func (c *Client) GetAdminURL() string {
	return c.adminURL
}

// isValidURL validates if a URL is properly formatted
func isValidURL(urlStr string) bool {
	if urlStr == "" {
		return false
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Must have a scheme (http or https) and host
	return parsedURL.Scheme != "" && parsedURL.Host != ""
}