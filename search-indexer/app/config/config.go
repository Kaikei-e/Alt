package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	Database    DatabaseConfig
	Meilisearch MeilisearchConfig
	Indexer     IndexerConfig
	HTTP        HTTPConfig
}

type DatabaseConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	Timeout  time.Duration
}

type MeilisearchConfig struct {
	Host    string
	APIKey  string
	Timeout time.Duration
}

type IndexerConfig struct {
	Interval     time.Duration
	BatchSize    int
	RetryDelay   time.Duration
	MaxRetries   int
	RetryTimeout time.Duration
}

type HTTPConfig struct {
	Addr              string
	ReadHeaderTimeout time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		Database: DatabaseConfig{
			Host:     getEnvRequired("DB_HOST"),
			Port:     getEnvRequired("DB_PORT"),
			Name:     getEnvRequired("DB_NAME"),
			User:     getEnvRequired("SEARCH_INDEXER_DB_USER"),
			Password: getEnvRequired("SEARCH_INDEXER_DB_PASSWORD"),
			Timeout:  10 * time.Second,
		},
		Meilisearch: MeilisearchConfig{
			Host:    getEnvRequired("MEILISEARCH_HOST"),
			APIKey:  os.Getenv("MEILISEARCH_API_KEY"),
			Timeout: 15 * time.Second,
		},
		Indexer: IndexerConfig{
			Interval:     1 * time.Minute,
			BatchSize:    200,
			RetryDelay:   1 * time.Minute,
			MaxRetries:   5,
			RetryTimeout: 1 * time.Minute,
		},
		HTTP: HTTPConfig{
			Addr:              ":9300",
			ReadHeaderTimeout: 5 * time.Second,
		},
	}

	return cfg, nil
}

func (c *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		c.Host, c.Port, c.User, c.Password, c.Name)
}

func getEnvRequired(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("required environment variable %s is not set", key))
	}
	return value
}