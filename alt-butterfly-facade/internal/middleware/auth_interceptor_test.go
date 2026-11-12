package middleware

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"log/slog"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt-butterfly-facade/internal/domain"
)

func TestNewAuthInterceptor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	interceptor := NewAuthInterceptor(logger, testPublicKey, "auth-hub", "alt-backend")

	assert.NotNil(t, interceptor)
}

func TestNewAuthInterceptor_EmptyKey_FailsClosed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	assert.Panics(t, func() {
		NewAuthInterceptor(logger, ed25519.PublicKey{}, "auth-hub", "alt-backend")
	}, "NewAuthInterceptor must fail closed (panic) when the verify key is empty")
}

func TestNewAuthInterceptor_NilKey_FailsClosed(t *testing.T) {
	assert.Panics(t, func() {
		NewAuthInterceptor(nil, nil, "auth-hub", "alt-backend")
	}, "NewAuthInterceptor must fail closed (panic) when the verify key is nil, even without a logger")
}

func TestNewAuthInterceptor_WrongSizeKey_FailsClosed(t *testing.T) {
	assert.Panics(t, func() {
		NewAuthInterceptor(nil, ed25519.PublicKey(make([]byte, 16)), "auth-hub", "alt-backend")
	})
}

func TestAuthInterceptor_ValidateToken_Valid(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interceptor := NewAuthInterceptor(logger, testPublicKey, "auth-hub", "alt-backend")

	// Create a valid token
	userID := uuid.New()
	token := createTestToken(t, testPrivateKey, userID, "test@example.com", "user", "auth-hub", "alt-backend", time.Now().Add(time.Hour))

	userCtx, err := interceptor.ValidateToken(token)

	require.NoError(t, err)
	assert.Equal(t, userID, userCtx.UserID)
	assert.Equal(t, "test@example.com", userCtx.Email)
	assert.Equal(t, "user", userCtx.Role)
}

func TestAuthInterceptor_ValidateToken_MissingToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interceptor := NewAuthInterceptor(logger, testPublicKey, "auth-hub", "alt-backend")

	_, err := interceptor.ValidateToken("")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestAuthInterceptor_ValidateToken_InvalidToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interceptor := NewAuthInterceptor(logger, testPublicKey, "auth-hub", "alt-backend")

	_, err := interceptor.ValidateToken("invalid-token")

	assert.Error(t, err)
}

func TestAuthInterceptor_ValidateToken_ExpiredToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interceptor := NewAuthInterceptor(logger, testPublicKey, "auth-hub", "alt-backend")

	userID := uuid.New()
	token := createTestToken(t, testPrivateKey, userID, "test@example.com", "user", "auth-hub", "alt-backend", time.Now().Add(-time.Hour))

	_, err := interceptor.ValidateToken(token)

	assert.Error(t, err)
}

func TestAuthInterceptor_ValidateToken_WrongIssuer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interceptor := NewAuthInterceptor(logger, testPublicKey, "auth-hub", "alt-backend")

	userID := uuid.New()
	token := createTestToken(t, testPrivateKey, userID, "test@example.com", "user", "wrong-issuer", "alt-backend", time.Now().Add(time.Hour))

	_, err := interceptor.ValidateToken(token)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issuer")
}

func TestAuthInterceptor_ValidateToken_WrongAudience(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interceptor := NewAuthInterceptor(logger, testPublicKey, "auth-hub", "alt-backend")

	userID := uuid.New()
	token := createTestToken(t, testPrivateKey, userID, "test@example.com", "user", "auth-hub", "wrong-audience", time.Now().Add(time.Hour))

	_, err := interceptor.ValidateToken(token)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "audience")
}

func TestAuthInterceptor_ValidateToken_OtherEd25519Key(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interceptor := NewAuthInterceptor(logger, testPublicKey, "auth-hub", "alt-backend")

	_, otherKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	token := createTestToken(t, otherKey, uuid.New(), "test@example.com", "user", "auth-hub", "alt-backend", time.Now().Add(time.Hour))

	_, err = interceptor.ValidateToken(token)

	assert.Error(t, err)
}

func TestAuthInterceptor_ValidateToken_HS256WithPublicKeyBytesRejected(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interceptor := NewAuthInterceptor(logger, testPublicKey, "auth-hub", "alt-backend")

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims(time.Now().Add(time.Hour)))
	signed, err := token.SignedString([]byte(testPublicKey))
	require.NoError(t, err)

	_, err = interceptor.ValidateToken(signed)

	assert.Error(t, err)
}

func TestAuthInterceptor_ValidateToken_AlgNoneRejected(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interceptor := NewAuthInterceptor(logger, testPublicKey, "auth-hub", "alt-backend")

	token := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims(time.Now().Add(time.Hour)))
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = interceptor.ValidateToken(signed)

	assert.Error(t, err)
}

func validClaims(exp time.Time) jwt.MapClaims {
	return jwt.MapClaims{
		"sub":   uuid.New().String(),
		"email": "test@example.com",
		"role":  "user",
		"sid":   "session-123",
		"iss":   "auth-hub",
		"aud":   []string{"alt-backend"},
		"exp":   exp.Unix(),
		"iat":   time.Now().Unix(),
	}
}

func TestAuthInterceptor_GetRawToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interceptor := NewAuthInterceptor(logger, testPublicKey, "auth-hub", "alt-backend")

	userID := uuid.New()
	token := createTestToken(t, testPrivateKey, userID, "test@example.com", "user", "auth-hub", "alt-backend", time.Now().Add(time.Hour))

	// Create a mock request header
	header := make(map[string][]string)
	header["X-Alt-Backend-Token"] = []string{token}

	rawToken := interceptor.GetRawToken(header)

	assert.Equal(t, token, rawToken)
}

func TestAuthInterceptor_Interceptor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interceptor := NewAuthInterceptor(logger, testPublicKey, "auth-hub", "alt-backend")

	i := interceptor.Interceptor()

	assert.NotNil(t, i)
	// Verify it implements connect.Interceptor interface
	var _ connect.Interceptor = i
}

// Helper function to create test JWT tokens
func createTestToken(t *testing.T, key ed25519.PrivateKey, userID uuid.UUID, email, role, issuer, audience string, expiresAt time.Time) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub":   userID.String(),
		"email": email,
		"role":  role,
		"sid":   "session-123",
		"iss":   issuer,
		"aud":   []string{audience},
		"exp":   expiresAt.Unix(),
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tokenStr, err := token.SignedString(key)
	require.NoError(t, err)

	return tokenStr
}

func TestGetUserContext(t *testing.T) {
	uc := &domain.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
	}

	ctx := domain.SetUserContext(context.Background(), uc)

	retrieved, err := domain.GetUserContext(ctx)

	require.NoError(t, err)
	assert.Equal(t, uc.UserID, retrieved.UserID)
	assert.Equal(t, uc.Email, retrieved.Email)
}

func TestGetUserContext_NoContext(t *testing.T) {
	_, err := domain.GetUserContext(context.Background())

	assert.Error(t, err)
	assert.Equal(t, domain.ErrNoUserContext, err)
}
