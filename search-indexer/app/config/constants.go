package config

import (
	"os"
	"strconv"
	"time"
)

// Service constants with env var override support.
var (
	IndexInterval      = durationEnv("INDEX_INTERVAL", 5*time.Minute)
	IndexBatchSize     = intEnv("INDEX_BATCH_SIZE", 200)
	IndexRetryInterval = durationEnv("INDEX_RETRY_INTERVAL", 1*time.Minute)
	HTTPAddr           = stringEnv("HTTP_ADDR", ":9300")
	ConnectAddr        = stringEnv("CONNECT_ADDR", ":9301")
	DBTimeout          = durationEnv("DB_TIMEOUT", 10*time.Second)
	MeiliTimeout       = durationEnv("MEILI_TIMEOUT", 15*time.Second)
)

func stringEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func intEnv(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func durationEnv(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}
