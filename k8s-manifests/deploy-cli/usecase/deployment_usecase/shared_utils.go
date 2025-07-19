package deployment_usecase

import (
	"encoding/base64"
	"strings"
)

// containsInsensitive checks if a string contains a substring (case-insensitive)
func containsInsensitive(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(indexOfInsensitive(s, substr) >= 0))
}

// indexOfInsensitive performs case-insensitive substring search
func indexOfInsensitive(s, substr string) int {
	sLower := toLower(s)
	substrLower := toLower(substr)

	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return i
		}
	}
	return -1
}

// toLower converts string to lowercase
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + ('a' - 'A')
		} else {
			result[i] = c
		}
	}
	return string(result)
}

// isPEMFormat checks if the data is in PEM format
func isPEMFormat(data string) bool {
	return strings.Contains(data, "-----BEGIN") && strings.Contains(data, "-----END")
}

// isBase64Encoded checks if the data is base64 encoded
func isBase64Encoded(data string) bool {
	// Try to decode as base64
	_, err := base64.StdEncoding.DecodeString(data)
	return err == nil && !strings.Contains(data, "-----BEGIN")
}
