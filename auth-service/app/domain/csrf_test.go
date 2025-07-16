package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewCSRFToken(t *testing.T) {
	tests := []struct {
		name        string
		sessionID   string
		tokenLength int
		duration    time.Duration
		expectErr   bool
	}{
		{
			name:        "valid CSRF token creation",
			sessionID:   "session-123",
			tokenLength: 32,
			duration:    1 * time.Hour,
			expectErr:   false,
		},
		{
			name:        "valid with default token length",
			sessionID:   "session-123",
			tokenLength: 0, // should default to 32
			duration:    1 * time.Hour,
			expectErr:   false,
		},
		{
			name:        "empty session ID",
			sessionID:   "",
			tokenLength: 32,
			duration:    1 * time.Hour,
			expectErr:   true,
		},
		{
			name:        "zero duration",
			sessionID:   "session-123",
			tokenLength: 32,
			duration:    0,
			expectErr:   true,
		},
		{
			name:        "negative duration",
			sessionID:   "session-123",
			tokenLength: 32,
			duration:    -1 * time.Hour,
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			csrfToken, err := NewCSRFToken(tt.sessionID, tt.tokenLength, tt.duration)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, csrfToken)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, csrfToken)
				assert.NotEmpty(t, csrfToken.Token)
				assert.Equal(t, tt.sessionID, csrfToken.SessionID)
				assert.False(t, csrfToken.IsExpired())

				// Verify token is hex encoded
				expectedLength := tt.tokenLength
				if expectedLength <= 0 {
					expectedLength = 32
				}
				assert.Equal(t, expectedLength*2, len(csrfToken.Token)) // hex encoding doubles length
				assert.True(t, isHexString(csrfToken.Token))
			}
		})
	}
}

func TestCSRFToken_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		expected  bool
	}{
		{
			name:      "not expired",
			expiresAt: time.Now().Add(1 * time.Hour),
			expected:  false,
		},
		{
			name:      "expired",
			expiresAt: time.Now().Add(-1 * time.Hour),
			expected:  true,
		},
		{
			name:      "expires now",
			expiresAt: time.Now(),
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &CSRFToken{
				Token:     "test-token",
				SessionID: "session-123",
				CreatedAt: time.Now(),
				ExpiresAt: tt.expiresAt,
			}

			assert.Equal(t, tt.expected, token.IsExpired())
		})
	}
}

func TestCSRFToken_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		expected  bool
	}{
		{
			name:      "valid token",
			expiresAt: time.Now().Add(1 * time.Hour),
			expected:  true,
		},
		{
			name:      "expired token",
			expiresAt: time.Now().Add(-1 * time.Hour),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &CSRFToken{
				Token:     "test-token",
				SessionID: "session-123",
				CreatedAt: time.Now(),
				ExpiresAt: tt.expiresAt,
			}

			assert.Equal(t, tt.expected, token.IsValid())
		})
	}
}

func TestCSRFToken_Validate(t *testing.T) {
	validToken := "abc123def456"

	tests := []struct {
		name          string
		storedToken   string
		providedToken string
		expiresAt     time.Time
		expectErr     bool
	}{
		{
			name:          "valid token",
			storedToken:   validToken,
			providedToken: validToken,
			expiresAt:     time.Now().Add(1 * time.Hour),
			expectErr:     false,
		},
		{
			name:          "empty provided token",
			storedToken:   validToken,
			providedToken: "",
			expiresAt:     time.Now().Add(1 * time.Hour),
			expectErr:     true,
		},
		{
			name:          "token mismatch",
			storedToken:   validToken,
			providedToken: "different-token",
			expiresAt:     time.Now().Add(1 * time.Hour),
			expectErr:     true,
		},
		{
			name:          "expired token",
			storedToken:   validToken,
			providedToken: validToken,
			expiresAt:     time.Now().Add(-1 * time.Hour),
			expectErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			csrfToken := &CSRFToken{
				Token:     tt.storedToken,
				SessionID: "session-123",
				CreatedAt: time.Now(),
				ExpiresAt: tt.expiresAt,
			}

			err := csrfToken.Validate(tt.providedToken)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper function to check if a string is valid hex
func isHexString(s string) bool {
	for _, char := range s {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}
