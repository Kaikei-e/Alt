package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
)

func parseArticleSyncOwnerUserID(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("ARTICLE_SYNC_OWNER_USER_ID must not be empty")
	}

	parsed, err := uuid.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("ARTICLE_SYNC_OWNER_USER_ID must be a valid UUID: %w", err)
	}
	if parsed == uuid.Nil {
		return "", fmt.Errorf("ARTICLE_SYNC_OWNER_USER_ID must not be the nil UUID")
	}

	return parsed.String(), nil
}

// ResolveArticleSyncOwnerUserID validates ARTICLE_SYNC_OWNER_USER_ID as a required non-nil UUID.
func ResolveArticleSyncOwnerUserID() (string, error) {
	raw, set := os.LookupEnv("ARTICLE_SYNC_OWNER_USER_ID")
	if !set {
		return "", fmt.Errorf("ARTICLE_SYNC_OWNER_USER_ID is required")
	}

	return parseArticleSyncOwnerUserID(raw)
}
