package opml_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils"
	"alt/utils/logger"
	"alt/utils/url_validator"
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ImportGateway implements opml_port.ImportOPMLPort.
type ImportGateway struct {
	altDB *alt_db.AltDBRepository
}

func NewImportGateway(pool *pgxpool.Pool) *ImportGateway {
	return &ImportGateway{altDB: alt_db.NewAltDBRepositoryWithPool(pool)}
}

func (g *ImportGateway) RegisterFeedLinkBulk(ctx context.Context, urls []string) (*domain.OPMLImportResult, error) {
	if g.altDB == nil {
		return nil, errors.New("database connection not available")
	}

	result := &domain.OPMLImportResult{
		Total: len(urls),
	}

	seen := make(map[string]struct{})

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

		// Check if already exists in DB
		existingID, _ := g.altDB.FetchFeedLinkIDByURL(ctx, sanitized)
		if existingID != nil {
			result.Skipped++
			continue
		}

		// Register new feed link
		err = g.altDB.RegisterRSSFeedLink(ctx, sanitized)
		if err != nil {
			logger.Logger.WarnContext(ctx, "OPML import: failed to register feed link", "url", sanitized, "error", err)
			result.Failed++
			result.FailedURLs = append(result.FailedURLs, sanitized)
			continue
		}

		result.Imported++
	}

	return result, nil
}
