package middleware

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
)

const (
	testSecret   = "test-secret-please-change-in-production"
	testIssuer   = "auth-hub"
	testAudience = "alt-backend"
)

func newTestInterceptor() *AuthInterceptor {
	return NewAuthInterceptor(nil, &config.Config{
		Auth: config.AuthConfig{
			BackendTokenSecret:   testSecret,
			BackendTokenIssuer:   testIssuer,
			BackendTokenAudience: testAudience,
		},
	})
}

func signTestToken(t *testing.T, claims *BackendClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testSecret))
	require.NoError(t, err)
	return signed
}

func baseClaims(userID uuid.UUID) *BackendClaims {
	return &BackendClaims{
		Email: "user@example.com",
		Role:  "user",
		Sid:   "session-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    testIssuer,
			Audience:  jwt.ClaimStrings{testAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
}

func TestValidateToken_TenantIDClaimPropagated(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	claims := baseClaims(userID)
	claims.TenantID = tenantID.String()
	tokenStr := signTestToken(t, claims)

	a := newTestInterceptor()
	userCtx, err := a.validateToken(tokenStr)

	require.NoError(t, err)
	require.NotNil(t, userCtx)
	assert.Equal(t, userID, userCtx.UserID)
	assert.Equal(t, tenantID, userCtx.TenantID,
		"TenantID must come from the tenant_id claim, not be collapsed to UserID")
}

func TestValidateToken_TenantIDDistinctFromUserID(t *testing.T) {
	userID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	tenantID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	claims := baseClaims(userID)
	claims.TenantID = tenantID.String()
	tokenStr := signTestToken(t, claims)

	a := newTestInterceptor()
	userCtx, err := a.validateToken(tokenStr)

	require.NoError(t, err)
	assert.NotEqual(t, userCtx.UserID, userCtx.TenantID,
		"a JWT carrying distinct user_id and tenant_id must not collapse them")
}

func TestValidateToken_MissingTenantIDClaim_Fails(t *testing.T) {
	userID := uuid.MustParse("55555555-5555-5555-5555-555555555555")

	claims := baseClaims(userID)
	// Deliberately leave TenantID empty.
	tokenStr := signTestToken(t, claims)

	a := newTestInterceptor()
	_, err := a.validateToken(tokenStr)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidClaims),
		"missing tenant_id claim must be rejected (fail-closed); got %v", err)
}

func TestValidateToken_InvalidTenantIDFormat_Fails(t *testing.T) {
	userID := uuid.MustParse("66666666-6666-6666-6666-666666666666")

	claims := baseClaims(userID)
	claims.TenantID = "not-a-uuid"
	tokenStr := signTestToken(t, claims)

	a := newTestInterceptor()
	_, err := a.validateToken(tokenStr)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidClaims),
		"malformed tenant_id claim must be rejected; got %v", err)
}
