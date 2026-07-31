package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
)

// feedLinksForExportQuery pairs every subscription with the title of the most
// recently collected feed behind it.
//
// The LATERAL join is why this is not ListFeedLinks with an extra column: it
// picks one row per link out of a table that has many, and making the plain
// list carry that work would charge every feed-link read for a query only OPML
// export needs.
const feedLinksForExportQuery = `
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

// FetchFeedLinksForExport returns each subscription with its newest feed title.
//
// This SQL used to live in opml_gateway, issued through
// AltDBRepository.GetPool() — the last gateway in the module reaching past the
// driver layer for a raw pool (capability catalog §4-7). It moves here because
// ADR-000954 leaves alt-backend with no pool to reach for; the layering fix and
// the process split are the same edit rather than two.
//
// An empty title means no feed has ever been collected for the link. It stays
// empty here: substituting the hostname is what the OPML renderer wants, and a
// different caller might want something else.
func (r *FeedRepository) FetchFeedLinksForExport(ctx context.Context) ([]*domain.FeedLinkForExport, error) {
	rows, err := r.pool.Query(ctx, feedLinksForExportQuery)
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
		links = append(links, &domain.FeedLinkForExport{URL: feedURL, Title: title})
	}

	if err := rows.Err(); err != nil {
		logger.SafeErrorContext(ctx, "Row iteration error", "error", err)
		return nil, errors.New("error iterating feed links for export")
	}

	return links, nil
}
