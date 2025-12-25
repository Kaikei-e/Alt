package config

import (
	"os"
	"strings"
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
		DBPassword:     getSecret("DB_PASSWORD", "DB_PASSWORD_FILE", "rag_password"),
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

func getSecret(envKey, fileEnvKey, fallback string) string {
	// 1. Try direct environment variable
	if value, ok := os.LookupEnv(envKey); ok {
		return value
	}

	// 2. Try reading from file specified by fileEnvKey
	if filePath, ok := os.LookupEnv(fileEnvKey); ok {
		content, err := os.ReadFile(filePath)
		if err == nil {
			return strings.TrimSpace(string(content))
		}
		// If file read fails, we could log but here we just fall through or return fallback?
		// For now, let's just proceed.
	}

	return fallback
}
