package opml_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExportGateway implements opml_port.ExportOPMLPort.
type ExportGateway struct {
	altDB *alt_db.AltDBRepository
}

func NewExportGateway(pool *pgxpool.Pool) *ExportGateway {
	return &ExportGateway{altDB: alt_db.NewAltDBRepositoryWithPool(pool)}
}

func (g *ExportGateway) FetchFeedLinksForExport(ctx context.Context) ([]*domain.FeedLinkForExport, error) {
	if g.altDB == nil {
		return nil, errors.New("database connection not available")
	}

	query := `
		SELECT fl.url,
		       COALESCE(sub.title, '') AS title
		FROM feed_links fl
		LEFT JOIN LATERAL (
			SELECT DISTINCT ON (feed_link_id) title
			FROM feeds
			WHERE feed_link_id = fl.id
			ORDER BY feed_link_id, created_at DESC
		) sub ON true
		ORDER BY fl.url ASC
	`

	rows, err := g.altDB.GetPool().Query(ctx, query)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching feed links for export", "error", err)
		return nil, errors.New("error fetching feed links for export")
	}
	defer rows.Close()

	links := make([]*domain.FeedLinkForExport, 0)
	for rows.Next() {
		var feedURL, title string
		if err := rows.Scan(&feedURL, &title); err != nil {
			logger.SafeErrorContext(ctx, "Error scanning feed link for export", "error", err)
			return nil, errors.New("error scanning feed links for export")
		}

		// Fallback: use domain name if title is empty
		if title == "" {
			if parsed, parseErr := url.Parse(feedURL); parseErr == nil && parsed.Host != "" {
				title = parsed.Host
			}
		}

		links = append(links, &domain.FeedLinkForExport{
			URL:   feedURL,
			Title: title,
		})
	}

	if err := rows.Err(); err != nil {
		logger.SafeErrorContext(ctx, "Row iteration error", "error", err)
		return nil, errors.New("error iterating feed links for export")
	}

	return links, nil
}
