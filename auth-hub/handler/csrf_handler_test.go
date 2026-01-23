package handler

import (
	"auth-hub/config"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateCSRFToken_Success(t *testing.T) {
	cfg := &config.Config{
		CSRFSecret: "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
	}
	handler := &CSRFHandler{config: cfg}

	token, err := handler.generateCSRFToken("test-session-id")
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestGenerateCSRFToken_EmptySecret(t *testing.T) {
	cfg := &config.Config{
		CSRFSecret: "",
	}
	handler := &CSRFHandler{config: cfg}

	token, err := handler.generateCSRFToken("test-session-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CSRF_SECRET is not configured")
	assert.Empty(t, token)
}

func TestGenerateCSRFToken_DeterministicOutput(t *testing.T) {
	cfg := &config.Config{
		CSRFSecret: "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
	}
	handler := &CSRFHandler{config: cfg}

	// Same session ID should produce same token
	token1, err1 := handler.generateCSRFToken("test-session-id")
	token2, err2 := handler.generateCSRFToken("test-session-id")

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, token1, token2)
}

func TestGenerateCSRFToken_DifferentSessionsProduceDifferentTokens(t *testing.T) {
	cfg := &config.Config{
		CSRFSecret: "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
	}
	handler := &CSRFHandler{config: cfg}

	token1, err1 := handler.generateCSRFToken("session-1")
	token2, err2 := handler.generateCSRFToken("session-2")

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotEqual(t, token1, token2)
}

func TestExtractSessionID(t *testing.T) {
	tests := []struct {
		name     string
		cookie   string
		expected string
	}{
		{
			name:     "valid session cookie",
			cookie:   "ory_kratos_session=abc123",
			expected: "abc123",
		},
		{
			name:     "session cookie with other cookies",
			cookie:   "other=value; ory_kratos_session=abc123; another=test",
			expected: "abc123",
		},
		{
			name:     "session cookie at end",
			cookie:   "other=value; ory_kratos_session=abc123",
			expected: "abc123",
		},
		{
			name:     "no session cookie",
			cookie:   "other=value; another=test",
			expected: "",
		},
		{
			name:     "empty cookie string",
			cookie:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSessionID(tt.cookie)
			assert.Equal(t, tt.expected, result)
		})
	}
}
