package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// ResolveRedisPassword resolves the Redis authentication password from the file
// referenced by REDIS_PASSWORD_FILE.
// If REDIS_AUTH=disabled is set (case-insensitive), it logs redis_auth_disabled once and returns an empty string.
// If REDIS_PASSWORD_FILE is set, it fails fast on missing, unreadable, or empty file contents (rules 8 and 9).
// If neither REDIS_PASSWORD_FILE nor REDIS_AUTH=disabled is set, it returns a startup error naming both variables.
func ResolveRedisPassword(logger *slog.Logger) (string, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if strings.EqualFold(strings.TrimSpace(os.Getenv("REDIS_AUTH")), "disabled") {
		logger.Warn("redis_auth_disabled", "reason", "REDIS_AUTH=disabled was set explicitly")
		return "", nil
	}

	envKey := "REDIS_PASSWORD_FILE"
	filePath, set := os.LookupEnv("REDIS_STREAMS_PASSWORD_FILE")
	if set && strings.TrimSpace(filePath) != "" {
		envKey = "REDIS_STREAMS_PASSWORD_FILE"
	} else {
		filePath, set = os.LookupEnv("REDIS_PASSWORD_FILE")
		envKey = "REDIS_PASSWORD_FILE"
	}
	if !set {
		return "", fmt.Errorf("redis authentication requires REDIS_PASSWORD_FILE or REDIS_AUTH=disabled")
	}

	trimmedPath := strings.TrimSpace(filePath)
	if trimmedPath == "" {
		logger.Error("redis_password_file_empty", "env", envKey)
		return "", fmt.Errorf("%s is set but empty; set REDIS_AUTH=disabled to run with redis auth disabled explicitly", envKey)
	}

	content, err := os.ReadFile(trimmedPath)
	if err != nil {
		logger.Error("redis_password_file_read_failed", "file", trimmedPath, "error", err)
		return "", fmt.Errorf("read %s %s: %w", envKey, trimmedPath, err)
	}

	password := strings.TrimSpace(string(content))
	if password == "" {
		logger.Error("redis_password_file_content_empty", "file", trimmedPath)
		return "", fmt.Errorf("%s %s resolved to an empty password", envKey, trimmedPath)
	}

	return password, nil
}
