package gateway

import (
	"os"
	"search-indexer/domain"
	"search-indexer/port"
)

// ConfigDriver defines the interface for configuration data source
type ConfigDriver interface {
	GetEnv(key string) string
	ValidateConnection() error
}

// ConfigGateway implements the configuration repository port
type ConfigGateway struct {
	driver ConfigDriver
}

// NewConfigGateway creates a new ConfigGateway
func NewConfigGateway(driver ConfigDriver) *ConfigGateway {
	return &ConfigGateway{
		driver: driver,
	}
}

// LoadSearchIndexerConfig loads the search indexer configuration
func (g *ConfigGateway) LoadSearchIndexerConfig() (*domain.SearchIndexerConfig, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	meilisearchHost := os.Getenv("MEILISEARCH_HOST")
	meilisearchAPIKey := os.Getenv("MEILISEARCH_API_KEY")

	config, err := g.convertToDomain(databaseURL, meilisearchHost, meilisearchAPIKey)
	if err != nil {
		return nil, &port.RepositoryError{
			Op:  "LoadSearchIndexerConfig",
			Err: err.Error(),
		}
	}

	return config, nil
}

// convertToDomain converts raw configuration data to domain object
func (g *ConfigGateway) convertToDomain(databaseURL, meilisearchHost, meilisearchAPIKey string) (*domain.SearchIndexerConfig, error) {
	return domain.NewSearchIndexerConfig(databaseURL, meilisearchHost, meilisearchAPIKey)
}
