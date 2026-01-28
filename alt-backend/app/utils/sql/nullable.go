// Package sql provides utilities for working with SQL types and database operations.
package sql

import (
	"database/sql"
	"strings"
	"time"
)

// NullStringPtr converts sql.NullString to *string.
// Returns nil if the value is not valid.
func NullStringPtr(value sql.NullString) *string {
	if value.Valid {
		result := value.String
		return &result
	}
	return nil
}

// NullTimePtr converts sql.NullTime to *time.Time.
// Returns nil if the value is not valid.
func NullTimePtr(value sql.NullTime) *time.Time {
	if value.Valid {
		t := value.Time
		return &t
	}
	return nil
}

// NullLowerTrimStringPtr converts sql.NullString to *string with lowercase and trim.
// Returns nil if the value is not valid or if the trimmed string is empty.
func NullLowerTrimStringPtr(value sql.NullString) *string {
	if value.Valid {
		trimmed := strings.TrimSpace(value.String)
		if trimmed == "" {
			return nil
		}
		lowered := strings.ToLower(trimmed)
		return &lowered
	}
	return nil
}

// ClampText truncates a string to the specified maximum byte length.
// If the string is shorter than maxBytes, it returns the original string unchanged.
func ClampText(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes]
}
