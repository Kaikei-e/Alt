package domain

import (
	"fmt"
	"time"
)

// SearchIndexerConfig represents the configuration for the search indexer service
type SearchIndexerConfig struct {
	databaseURL       string
	meilisearchHost   string
	meilisearchAPIKey string
	indexInterval     time.Duration
	batchSize         int
	retryInterval     time.Duration
	httpAddr          string
}

// NewSearchIndexerConfig creates a new SearchIndexerConfig
func NewSearchIndexerConfig(databaseURL, meilisearchHost, meilisearchAPIKey string) (*SearchIndexerConfig, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("database URL cannot be empty")
	}

	if meilisearchHost == "" {
		return nil, fmt.Errorf("meilisearch host cannot be empty")
	}

	return &SearchIndexerConfig{
		databaseURL:       databaseURL,
		meilisearchHost:   meilisearchHost,
		meilisearchAPIKey: meilisearchAPIKey,
		indexInterval:     1 * time.Minute, // default values
		batchSize:         200,
		retryInterval:     1 * time.Minute,
		httpAddr:          ":9300",
	}, nil
}

// DatabaseURL returns the database connection URL
func (c *SearchIndexerConfig) DatabaseURL() string {
	return c.databaseURL
}

// MeilisearchHost returns the Meilisearch host URL
func (c *SearchIndexerConfig) MeilisearchHost() string {
	return c.meilisearchHost
}

// MeilisearchAPIKey returns the Meilisearch API key
func (c *SearchIndexerConfig) MeilisearchAPIKey() string {
	return c.meilisearchAPIKey
}

// IndexInterval returns the interval between indexing runs
func (c *SearchIndexerConfig) IndexInterval() time.Duration {
	return c.indexInterval
}

// BatchSize returns the batch size for indexing operations
func (c *SearchIndexerConfig) BatchSize() int {
	return c.batchSize
}

// RetryInterval returns the interval between retry attempts
func (c *SearchIndexerConfig) RetryInterval() time.Duration {
	return c.retryInterval
}

// HTTPAddr returns the HTTP server address
func (c *SearchIndexerConfig) HTTPAddr() string {
	return c.httpAddr
}
