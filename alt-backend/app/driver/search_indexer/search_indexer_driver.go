package search_indexer

import (
	"alt/domain"
	"alt/port/search_indexer_port"
	"context"
)

type HTTPSearchIndexerDriver struct{}

func NewHTTPSearchIndexerDriver() search_indexer_port.SearchIndexerPort {
	return &HTTPSearchIndexerDriver{}
}

func (d *HTTPSearchIndexerDriver) SearchArticles(ctx context.Context, query string, userID string) ([]domain.SearchIndexerArticleHit, error) {
	hits, err := SearchArticlesWithUserID(query, userID)
	if err != nil {
		return nil, err
	}

	domainHits := make([]domain.SearchIndexerArticleHit, len(hits))
	for i, hit := range hits {
		domainHits[i] = domain.SearchIndexerArticleHit{
			ID:      hit.ID,
			Title:   hit.Title,
			Content: hit.Content,
			Tags:    hit.Tags,
		}
	}

	return domainHits, nil
}
