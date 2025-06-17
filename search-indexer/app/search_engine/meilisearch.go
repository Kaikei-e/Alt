package search_engine

import (
	"github.com/meilisearch/meilisearch-go"
)

func NewMeilisearchClient(host string, apiKey string) meilisearch.ServiceManager {
	return meilisearch.New(host, meilisearch.WithAPIKey(apiKey))
}

