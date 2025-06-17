package search_engine

import (
	"fmt"
	"search-indexer/logger"

	"github.com/meilisearch/meilisearch-go"
)

func NewMeilisearchClient(host string, apiKey string) meilisearch.ServiceManager {
	return meilisearch.New(host, meilisearch.WithAPIKey(apiKey))
}

func SearchArticles(idxArticles meilisearch.IndexManager, query string) (*meilisearch.SearchResponse, error) {

	const LIMIT = 20

	results, err := idxArticles.Search(query, &meilisearch.SearchRequest{
		Limit: LIMIT,
	})
	if err != nil {
		logger.Logger.Error("Failed to search articles", "error", err)
		return nil, err
	}

	return results, nil
}

// maybe use this later
func makeSearchFilter(tags []string) string {
	filter := ""
	for _, tag := range tags {
		filter += fmt.Sprintf("tags = %s", tag)
	}

	return filter
}
