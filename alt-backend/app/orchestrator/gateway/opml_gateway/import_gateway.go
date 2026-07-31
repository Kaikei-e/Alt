package opml_gateway

import (
	"alt/domain"
	"alt/utils"
	"alt/utils/logger"
	"alt/utils/url_validator"
	"context"
	"net/url"
	"strings"
)

// FeedLinkBulkStore is the one capability OPML import needs.
//
// Its predecessor looped here, issuing an exists-check and an insert per
// outline. In-process that was two queries; with alt-data-hub owning the table
// it would be two round trips per outline, so a 500-entry file would cost a
// thousand. The loop moved to the provider — but only the loop.
type FeedLinkBulkStore interface {
	RegisterFeedLinkBulk(ctx context.Context, urls []string) (*domain.OPMLImportResult, error)
}

// ImportGateway implements opml_port.ImportOPMLPort.
type ImportGateway struct {
	store FeedLinkBulkStore
}

func NewImportGateway(store FeedLinkBulkStore) *ImportGateway {
	if store == nil {
		panic("opml_gateway: FeedLinkBulkStore is required (see .claude/rules/di-wiring.md)")
	}
	return &ImportGateway{store: store}
}

// RegisterFeedLinkBulk validates and sanitises here, then registers in one call.
//
// The SSRF check, the tracking-parameter strip and the batch-level dedupe are
// pure functions over strings and stay on this side (ADR-000954 D4). They also
// produce part of the answer the user is shown — how many outlines were
// rejected before the data plane ever saw them — which is why the counts are
// merged rather than taken wholesale from the response.
func (g *ImportGateway) RegisterFeedLinkBulk(ctx context.Context, urls []string) (*domain.OPMLImportResult, error) {
	result := &domain.OPMLImportResult{
		Total: len(urls),
	}

	seen := make(map[string]struct{})
	accepted := make([]string, 0, len(urls))

	for _, rawURL := range urls {
		trimmed := strings.TrimSpace(rawURL)
		if trimmed == "" {
			result.Failed++
			result.FailedURLs = append(result.FailedURLs, rawURL)
			continue
		}

		// SSRF protection
		parsedURL, err := url.Parse(trimmed)
		if err != nil {
			result.Failed++
			result.FailedURLs = append(result.FailedURLs, trimmed)
			continue
		}
		if err := url_validator.IsAllowedURL(parsedURL); err != nil {
			logger.Logger.WarnContext(ctx, "OPML import: URL not allowed", "url", trimmed, "reason", err.Error())
			result.Failed++
			result.FailedURLs = append(result.FailedURLs, trimmed)
			continue
		}

		// Strip tracking parameters
		sanitized, sanitizeErr := utils.StripTrackingParams(trimmed)
		if sanitizeErr != nil {
			sanitized = trimmed
		}

		// Batch-level deduplication (after sanitization, UTM-only differences collapse)
		if _, exists := seen[sanitized]; exists {
			result.Skipped++
			continue
		}
		seen[sanitized] = struct{}{}
		accepted = append(accepted, sanitized)
	}

	if len(accepted) == 0 {
		return result, nil
	}

	registered, err := g.store.RegisterFeedLinkBulk(ctx, accepted)
	if err != nil {
		logger.Logger.WarnContext(ctx, "OPML import: bulk registration failed", "url_count", len(accepted), "error", err)
		return nil, err
	}

	result.Imported = registered.Imported
	result.Skipped += registered.Skipped
	result.Failed += registered.Failed
	result.FailedURLs = append(result.FailedURLs, registered.FailedURLs...)
	return result, nil
}
