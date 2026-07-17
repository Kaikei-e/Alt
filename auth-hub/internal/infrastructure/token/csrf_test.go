package token

import (
	"errors"
	"strings"
	"testing"
	"time"

	"auth-hub/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCSRFSecret = "this-is-a-valid-csrf-secret-that-is-at-least-32-chars"

func TestHMACCSRFGenerator_Generate(t *testing.T) {
	gen := NewHMACCSRFGenerator(testCSRFSecret)

	token, err := gen.Generate("session-123")
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Contains(t, token, ".")
}

func TestHMACCSRFGenerator_RotatesWithTimestamp(t *testing.T) {
	gen := NewHMACCSRFGenerator(testCSRFSecret)
	gen.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	token1, err := gen.Generate("session-123")
	require.NoError(t, err)

	gen.now = func() time.Time { return time.Unix(1_700_000_001, 0) }
	token2, err := gen.Generate("session-123")
	require.NoError(t, err)

	assert.NotEqual(t, token1, token2)
	assert.True(t, strings.HasPrefix(token1, "1700000000."))
	assert.True(t, strings.HasPrefix(token2, "1700000001."))
}

func TestHMACCSRFGenerator_DifferentSessions(t *testing.T) {
	gen := NewHMACCSRFGenerator(testCSRFSecret)
	gen.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

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

func TestHMACCSRFGenerator_Validate_OK(t *testing.T) {
	gen := NewHMACCSRFGenerator(testCSRFSecret)
	fixed := time.Unix(1_700_000_000, 0)
	gen.now = func() time.Time { return fixed }

	token, err := gen.Generate("session-123")
	require.NoError(t, err)
	assert.NoError(t, gen.Validate("session-123", token))
}

func TestHMACCSRFGenerator_Validate_Expired(t *testing.T) {
	gen := NewHMACCSRFGenerator(testCSRFSecret)
	gen.ttl = 10 * time.Second
	gen.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	token, err := gen.Generate("session-123")
	require.NoError(t, err)

	gen.now = func() time.Time { return time.Unix(1_700_000_020, 0) }
	err = gen.Validate("session-123", token)
	assert.True(t, errors.Is(err, domain.ErrCSRFTokenExpired))
}

func TestHMACCSRFGenerator_Validate_WrongSession(t *testing.T) {
	gen := NewHMACCSRFGenerator(testCSRFSecret)
	gen.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	token, err := gen.Generate("session-123")
	require.NoError(t, err)
	err = gen.Validate("other-session", token)
	assert.True(t, errors.Is(err, domain.ErrCSRFTokenInvalid))
}

func TestHMACCSRFGenerator_Validate_Malformed(t *testing.T) {
	gen := NewHMACCSRFGenerator(testCSRFSecret)
	err := gen.Validate("session-123", "not-a-token")
	assert.True(t, errors.Is(err, domain.ErrCSRFTokenInvalid))
}
