package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"
)

type SyncMode string

const (
	SyncEnabled  SyncMode = "enabled"
	SyncDisabled SyncMode = "disabled"
)

func ParseSyncMode(raw string) (SyncMode, error) {
	switch SyncMode(raw) {
	case SyncEnabled:
		return SyncEnabled, nil
	case SyncDisabled:
		return SyncDisabled, nil
	default:
		return "", fmt.Errorf("INOREADER_SYNC must be %q or %q, got %q", SyncEnabled, SyncDisabled, raw)
	}
}

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

// ResolveArticleSyncConfig validates INOREADER_SYNC ("enabled" or "disabled") and
// ARTICLE_SYNC_OWNER_USER_ID (required when enabled; optional when disabled, validated if non-empty).
func ResolveArticleSyncConfig(logger *slog.Logger) (SyncMode, string, error) {
	if logger == nil {
		logger = slog.Default()
	}

	rawMode, modeSet := os.LookupEnv("INOREADER_SYNC")
	if !modeSet {
		return "", "", fmt.Errorf("INOREADER_SYNC is required and must be %q or %q", SyncEnabled, SyncDisabled)
	}

	mode, err := ParseSyncMode(rawMode)
	if err != nil {
		return "", "", err
	}

	rawOwner, ownerSet := os.LookupEnv("ARTICLE_SYNC_OWNER_USER_ID")
	if mode == SyncDisabled {
		var ownerID string
		if ownerSet && rawOwner != "" {
			ownerID, err = parseArticleSyncOwnerUserID(rawOwner)
			if err != nil {
				return "", "", err
			}
		}
		logger.Info("article_sync_disabled",
			"reason", "INOREADER_SYNC=disabled",
			"how_to_enable", "set INOREADER_SYNC=enabled and ARTICLE_SYNC_OWNER_USER_ID=<uuid>",
		)
		return SyncDisabled, ownerID, nil
	}

	if !ownerSet {
		return "", "", fmt.Errorf("ARTICLE_SYNC_OWNER_USER_ID is required when INOREADER_SYNC=enabled")
	}
	ownerID, err := parseArticleSyncOwnerUserID(rawOwner)
	if err != nil {
		return "", "", err
	}

	logger.Info("article_sync_enabled", "owner_user_id", ownerID)
	return SyncEnabled, ownerID, nil
}
