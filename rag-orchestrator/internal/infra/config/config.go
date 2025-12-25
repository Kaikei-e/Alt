package config

import (
	"os"
)

type Config struct {
	Env            string
	Port           string
	DBHost         string
	DBPort         string
	DBUser         string
	DBPassword     string
	DBName         string
	OllamaURL      string
	EmbeddingModel string
}

func Load() *Config {
	return &Config{
		Env:            getEnv("ENV", "development"),
		Port:           getEnv("PORT", "9010"),
		DBHost:         getEnv("DB_HOST", "rag-db"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "rag_user"),
		DBPassword:     getEnv("DB_PASSWORD", "rag_password"), // Should be loaded from file in prod usually, but simplified for now adapting to provided secrets in compose
		DBName:         getEnv("DB_NAME", "rag_db"),
		OllamaURL:      getEnv("AUGUR_EXTERNAL", "http://augur-external:11434"),
		EmbeddingModel: getEnv("EMBEDDING_MODEL", "embeddinggemma"), // Default to gemma3:4b if not specified, assuming it supports embedding
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
