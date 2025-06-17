package main

import (
	"os"
	"strconv"
	"time"
)

// ProcessingConfig holds all processing configuration
type ProcessingConfig struct {
	BatchSize              int
	SummarizeInterval      time.Duration
	FeedProcessingInterval time.Duration
	QualityCheckInterval   time.Duration
	ModelID                string

	// Sleep intervals between processing individual items
	FeedFetchSleep     time.Duration
	SummarizationSleep time.Duration
	QualityCheckSleep  time.Duration

	// Concurrency settings
	MaxConcurrentSummarizations int
	MaxConcurrentQualityChecks  int
}

// GetConfig returns the configuration with environment variable overrides
func GetConfig() *ProcessingConfig {
	return &ProcessingConfig{
		BatchSize:              getEnvInt("BATCH_SIZE", 40),
		SummarizeInterval:      getEnvDuration("SUMMARIZE_INTERVAL", 5*time.Second),
		FeedProcessingInterval: getEnvDuration("FEED_PROCESSING_INTERVAL", 3*time.Minute),
		QualityCheckInterval:   getEnvDuration("QUALITY_CHECK_INTERVAL", 10*time.Minute),
		ModelID:                getEnvString("MODEL_ID", "phi4-mini:3.8b"),

		FeedFetchSleep:     getEnvDuration("FEED_FETCH_SLEEP", 2*time.Second),
		SummarizationSleep: getEnvDuration("SUMMARIZATION_SLEEP", 10*time.Second),
		QualityCheckSleep:  getEnvDuration("QUALITY_CHECK_SLEEP", 30*time.Second),

		MaxConcurrentSummarizations: getEnvInt("MAX_CONCURRENT_SUMMARIZATIONS", 1),
		MaxConcurrentQualityChecks:  getEnvInt("MAX_CONCURRENT_QUALITY_CHECKS", 1),
	}
}

func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
