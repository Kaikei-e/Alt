package token

import (
	"errors"
	"testing"

	"auth-hub/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestHMACCSRFGenerator_Generate(t *testing.T) {
	gen := NewHMACCSRFGenerator("this-is-a-valid-csrf-secret-that-is-at-least-32-chars")

	token, err := gen.Generate("session-123")
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestHMACCSRFGenerator_Deterministic(t *testing.T) {
	gen := NewHMACCSRFGenerator("this-is-a-valid-csrf-secret-that-is-at-least-32-chars")

	token1, _ := gen.Generate("session-123")
	token2, _ := gen.Generate("session-123")
	assert.Equal(t, token1, token2)
}

func TestHMACCSRFGenerator_DifferentSessions(t *testing.T) {
	gen := NewHMACCSRFGenerator("this-is-a-valid-csrf-secret-that-is-at-least-32-chars")

	token1, _ := gen.Generate("session-1")
	token2, _ := gen.Generate("session-2")
	assert.NotEqual(t, token1, token2)
}

func TestHMACCSRFGenerator_EmptySecret(t *testing.T) {
	gen := NewHMACCSRFGenerator("")

	token, err := gen.Generate("session-123")
	assert.Empty(t, token)
	assert.True(t, errors.Is(err, domain.ErrCSRFSecretMissing))
}
