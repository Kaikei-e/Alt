package middleware

import (
	"context"
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
	secret := []byte("test-secret")

	interceptor := NewAuthInterceptor(logger, secret, "auth-hub", "alt-backend")

	assert.NotNil(t, interceptor)
}

func TestAuthInterceptor_ValidateToken_Valid(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	secret := []byte("test-secret")
	interceptor := NewAuthInterceptor(logger, secret, "auth-hub", "alt-backend")

	// Create a valid token
	userID := uuid.New()
	token := createTestToken(t, secret, userID, "test@example.com", "user", "auth-hub", "alt-backend", time.Now().Add(time.Hour))

	userCtx, err := interceptor.ValidateToken(token)

	require.NoError(t, err)
	assert.Equal(t, userID, userCtx.UserID)
	assert.Equal(t, "test@example.com", userCtx.Email)
	assert.Equal(t, "user", userCtx.Role)
}

func TestAuthInterceptor_ValidateToken_MissingToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	secret := []byte("test-secret")
	interceptor := NewAuthInterceptor(logger, secret, "auth-hub", "alt-backend")

	_, err := interceptor.ValidateToken("")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestAuthInterceptor_ValidateToken_InvalidToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	secret := []byte("test-secret")
	interceptor := NewAuthInterceptor(logger, secret, "auth-hub", "alt-backend")

	_, err := interceptor.ValidateToken("invalid-token")

	assert.Error(t, err)
}

func TestAuthInterceptor_ValidateToken_ExpiredToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	secret := []byte("test-secret")
	interceptor := NewAuthInterceptor(logger, secret, "auth-hub", "alt-backend")

	userID := uuid.New()
	token := createTestToken(t, secret, userID, "test@example.com", "user", "auth-hub", "alt-backend", time.Now().Add(-time.Hour))

	_, err := interceptor.ValidateToken(token)

	assert.Error(t, err)
}

func TestAuthInterceptor_ValidateToken_WrongIssuer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	secret := []byte("test-secret")
	interceptor := NewAuthInterceptor(logger, secret, "auth-hub", "alt-backend")

	userID := uuid.New()
	token := createTestToken(t, secret, userID, "test@example.com", "user", "wrong-issuer", "alt-backend", time.Now().Add(time.Hour))

	_, err := interceptor.ValidateToken(token)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issuer")
}

func TestAuthInterceptor_ValidateToken_WrongAudience(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	secret := []byte("test-secret")
	interceptor := NewAuthInterceptor(logger, secret, "auth-hub", "alt-backend")

	userID := uuid.New()
	token := createTestToken(t, secret, userID, "test@example.com", "user", "auth-hub", "wrong-audience", time.Now().Add(time.Hour))

	_, err := interceptor.ValidateToken(token)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "audience")
}

func TestAuthInterceptor_ValidateToken_WrongSecret(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	secret := []byte("test-secret")
	wrongSecret := []byte("wrong-secret")
	interceptor := NewAuthInterceptor(logger, secret, "auth-hub", "alt-backend")

	userID := uuid.New()
	token := createTestToken(t, wrongSecret, userID, "test@example.com", "user", "auth-hub", "alt-backend", time.Now().Add(time.Hour))

	_, err := interceptor.ValidateToken(token)

	assert.Error(t, err)
}

func TestAuthInterceptor_GetRawToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	secret := []byte("test-secret")
	interceptor := NewAuthInterceptor(logger, secret, "auth-hub", "alt-backend")

	userID := uuid.New()
	token := createTestToken(t, secret, userID, "test@example.com", "user", "auth-hub", "alt-backend", time.Now().Add(time.Hour))

	// Create a mock request header
	header := make(map[string][]string)
	header["X-Alt-Backend-Token"] = []string{token}

	rawToken := interceptor.GetRawToken(header)

	assert.Equal(t, token, rawToken)
}

func TestAuthInterceptor_Interceptor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	secret := []byte("test-secret")
	interceptor := NewAuthInterceptor(logger, secret, "auth-hub", "alt-backend")

	i := interceptor.Interceptor()

	assert.NotNil(t, i)
	// Verify it implements connect.Interceptor interface
	var _ connect.Interceptor = i
}

// Helper function to create test JWT tokens
func createTestToken(t *testing.T, secret []byte, userID uuid.UUID, email, role, issuer, audience string, expiresAt time.Time) string {
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

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(secret)
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
