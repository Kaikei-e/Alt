package repository

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"

	"pre-processor/config"
	"pre-processor/domain"
	"pre-processor/driver"
	datahubv1 "pre-processor/gen/proto/alt/datahub/v1"
	"pre-processor/gen/proto/alt/datahub/v1/datahubv1connect"
	"pre-processor/utils"
)

// maxInternalResponseBytes bounds response bodies read from internal services
// so a misbehaving peer cannot exhaust memory. Connect-RPC defaults to no
// limit, so the bound has to be passed explicitly — it used to be applied by
// wrapping the REST response body in an io.LimitReader.
const maxInternalResponseBytes = 10 * 1024 * 1024

// ExternalAPIRepository implementation.
type externalAPIRepository struct {
	logger  *slog.Logger
	client  *http.Client
	dataHub datahubv1connect.DataHubServiceClient
	config  *config.Config
}

// NewExternalAPIRepository creates a new external API repository. The HTTP
// client uses the hardened transport (DialContext, TLSHandshake and
// ResponseHeader timeouts) shared across the service.
//
// The alt-data-hub client is built over that same *http.Client, so the
// transport hardening — and the transport substitution the tests rely on —
// applies to the Connect-RPC calls too.
func NewExternalAPIRepository(cfg *config.Config, logger *slog.Logger) ExternalAPIRepository {
	httpClient := utils.NewHTTPClientManager().GetDefaultClient()
	return &externalAPIRepository{
		logger: logger,
		config: cfg,
		client: httpClient,
		dataHub: datahubv1connect.NewDataHubServiceClient(httpClient, cfg.AltService.Host,
			connect.WithReadMaxBytes(maxInternalResponseBytes)),
	}
}

// SummarizeArticle summarizes an article using external API.
func (r *externalAPIRepository) SummarizeArticle(ctx context.Context, article *domain.Article, priority string) (*domain.SummarizedContent, error) {
	// Input validation
	if article == nil {
		r.logger.ErrorContext(ctx, "article cannot be nil")
		return nil, fmt.Errorf("article cannot be nil")
	}

	if article.ID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return nil, fmt.Errorf("article ID cannot be empty")
	}

	if article.Content == "" {
		r.logger.ErrorContext(ctx, "article content cannot be empty", "article_id", article.ID)
		return nil, fmt.Errorf("article content cannot be empty")
	}

	r.logger.InfoContext(ctx, "summarizing article", "article_id", article.ID)

	// Use existing driver function
	driverSummary, err := driver.ArticleSummarizerAPIClient(ctx, article, r.config, r.logger, priority)
	if err != nil {
		// Handle content too short as a normal case, not an error
		if errors.Is(err, domain.ErrContentTooShort) {
			r.logger.InfoContext(ctx, "skipping summarization: content too short", "article_id", article.ID)
			return nil, domain.ErrContentTooShort
		}
		// Handle 429 (service overloaded) - propagate for backpressure
		if errors.Is(err, domain.ErrServiceOverloaded) {
			r.logger.WarnContext(ctx, "downstream service overloaded", "article_id", article.ID)
			return nil, domain.ErrServiceOverloaded
		}
		// Handle 422 (content not processable) - non-retryable, immediate dead_letter
		if errors.Is(err, domain.ErrContentNotProcessable) {
			r.logger.WarnContext(ctx, "content not processable by model", "article_id", article.ID)
			return nil, domain.ErrContentNotProcessable
		}
		r.logger.ErrorContext(ctx, "failed to summarize article", "error", err, "article_id", article.ID)
		return nil, fmt.Errorf("failed to summarize article: %w", err)
	}

	// Convert driver type to models type
	summarizedContent := &domain.SummarizedContent{
		ArticleID:       driverSummary.ArticleID,
		SummaryJapanese: driverSummary.SummaryJapanese,
	}

	r.logger.InfoContext(ctx, "article summarized successfully", "article_id", article.ID)

	return summarizedContent, nil
}

// StreamSummarizeArticle streams the summary for an article using external API.
func (r *externalAPIRepository) StreamSummarizeArticle(ctx context.Context, article *domain.Article, priority string) (io.ReadCloser, error) {
	// Input validation
	if article == nil {
		r.logger.ErrorContext(ctx, "article cannot be nil")
		return nil, fmt.Errorf("article cannot be nil")
	}

	if article.ID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return nil, fmt.Errorf("article ID cannot be empty")
	}

	if article.Content == "" {
		r.logger.ErrorContext(ctx, "article content cannot be empty", "article_id", article.ID)
		return nil, fmt.Errorf("article content cannot be empty")
	}

	r.logger.InfoContext(ctx, "streaming summary for article", "article_id", article.ID)

	// Use driver function for streaming
	streamBody, err := driver.StreamArticleSummarizerAPIClient(ctx, article, r.config, r.logger, priority)
	if err != nil {
		if errors.Is(err, domain.ErrContentTooShort) {
			r.logger.InfoContext(ctx, "skipping summarization: content too short", "article_id", article.ID)
			return nil, domain.ErrContentTooShort
		}
		r.logger.ErrorContext(ctx, "failed to start streaming summary", "error", err, "article_id", article.ID)
		return nil, fmt.Errorf("failed to start streaming summary: %w", err)
	}

	r.logger.InfoContext(ctx, "streaming started successfully", "article_id", article.ID)
	return streamBody, nil
}

// CheckHealth checks the health of an external service.
func (r *externalAPIRepository) CheckHealth(ctx context.Context, serviceURL string) error {
	// Input validation
	if serviceURL == "" {
		r.logger.ErrorContext(ctx, "service URL cannot be empty")
		return fmt.Errorf("service URL cannot be empty")
	}

	// Validate URL format
	if !strings.HasPrefix(serviceURL, "http://") && !strings.HasPrefix(serviceURL, "https://") {
		r.logger.ErrorContext(ctx, "invalid service URL", "url", serviceURL)
		return fmt.Errorf("invalid service URL: must start with http:// or https://")
	}

	// Parse URL to ensure it's valid
	parsedURL, err := url.Parse(serviceURL)
	if err != nil {
		r.logger.ErrorContext(ctx, "invalid service URL format", "url", serviceURL, "error", err)
		return fmt.Errorf("invalid service URL: %w", err)
	}

	r.logger.InfoContext(ctx, "checking service health", "url", serviceURL)

	// GREEN PHASE: Basic health check implementation
	healthEndpoint := parsedURL.String() + "/health"

	req, err := http.NewRequestWithContext(ctx, "GET", healthEndpoint, http.NoBody)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to create health check request", "error", err)
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		r.logger.ErrorContext(ctx, "health check request failed", "error", err)
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			r.logger.WarnContext(ctx, "failed to close health check response body", "error", cerr, "url", healthEndpoint)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		r.logger.ErrorContext(ctx, "service not healthy", "status", resp.StatusCode)
		return fmt.Errorf("service not healthy: status %d", resp.StatusCode)
	}

	r.logger.InfoContext(ctx, "service is healthy")

	return nil
}

// GetSystemUserID retrieves the system user ID from alt-data-hub with retry logic.
func (r *externalAPIRepository) GetSystemUserID(ctx context.Context) (string, error) {
	const maxRetries = 3
	baseDelay := 2 * time.Second

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		userID, err := r.getSystemUserIDOnce(ctx)
		if err == nil {
			return userID, nil
		}
		lastErr = err

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		// Log retry attempt (except for last attempt)
		if attempt < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<attempt) // 2s, 4s
			r.logger.WarnContext(ctx, "GetSystemUserID failed, retrying",
				"attempt", attempt+1,
				"max_attempts", maxRetries,
				"delay", delay,
				"error", err)

			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return "", fmt.Errorf("GetSystemUserID failed after %d attempts: %w", maxRetries, lastErr)
}

// getSystemUserIDOnce performs a single attempt to retrieve the system user ID.
//
// This used to be GET /v1/internal/system-user on alt-backend. ADR-000954 D6
// absorbed that REST route into alt-data-hub's Connect capability as the
// GetSystemUser RPC, and D7 put it in the alt.datahub.v1 namespace. The host
// is unchanged — only the protocol and the path moved.
func (r *externalAPIRepository) getSystemUserIDOnce(ctx context.Context) (string, error) {
	// Authentication is established at the TLS transport layer (mTLS).
	resp, err := r.dataHub.GetSystemUser(ctx, connect.NewRequest(&datahubv1.GetSystemUserRequest{}))
	if err != nil {
		return "", fmt.Errorf("GetSystemUser: %w", err)
	}

	if resp.Msg.UserId == "" {
		return "", fmt.Errorf("received empty user_id")
	}

	return resp.Msg.UserId, nil
}
