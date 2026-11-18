package reading_queue_usecase

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"alt/domain"
	"alt/orchestrator/port/reading_queue_port"
)

var ErrInvalidInput = domain.ErrInvalidReadingEvent

func hydrateQueueEntries(
	ctx context.Context,
	articleBatchPort reading_queue_port.FetchArticlesByIDsPort,
	ogImagePort reading_queue_port.GetOgImageURLsPort,
	entries []domain.ReadingQueueEntry,
) ([]domain.HydratedReadingQueueItem, error) {
	if len(entries) == 0 {
		return []domain.HydratedReadingQueueItem{}, nil
	}

	articleIDs := make([]uuid.UUID, 0, len(entries))
	articleIDStrings := make([]string, 0, len(entries))
	for _, entry := range entries {
		articleIDs = append(articleIDs, entry.ArticleID)
		articleIDStrings = append(articleIDStrings, entry.ArticleID.String())
	}

	articles, err := articleBatchPort.FetchArticlesByIDs(ctx, articleIDs)
	if err != nil {
		return nil, fmt.Errorf("hydrate queue articles: %w", err)
	}
	articleByID := make(map[uuid.UUID]*domain.Article, len(articles))
	for _, art := range articles {
		if art != nil {
			articleByID[art.ID] = art
		}
	}

	ogURLs, ogErr := ogImagePort.GetOgImageURLsByArticleIDs(ctx, articleIDStrings)
	if ogErr != nil {
		slog.WarnContext(ctx, "reading_queue: failed to fetch og image URLs, continuing without images",
			"error", ogErr, "count", len(articleIDStrings))
		ogURLs = map[string]string{}
	}

	items := make([]domain.HydratedReadingQueueItem, 0, len(entries))
	for _, entry := range entries {
		item := domain.HydratedReadingQueueItem{
			ArticleID: entry.ArticleID,
			ImageURL:  ogURLs[entry.ArticleID.String()],
			QueuedAt:  entry.QueuedAt,
			ExpiresAt: entry.ExpiresAt,
		}
		if art, ok := articleByID[entry.ArticleID]; ok {
			item.Title = art.Title
			item.URL = art.URL
			item.SourceName = extractSourceName(art.URL)
		}
		items = append(items, item)
	}
	return items, nil
}

func extractSourceName(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	return strings.TrimPrefix(host, "www.")
}
