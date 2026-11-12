package middleware

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
)

const (
	testIssuer   = "auth-hub"
	testAudience = "alt-backend"
)

var testVerifyPublicKey, testSigningPrivateKey = func() (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return pub, priv
}()

func newTestInterceptor() *AuthInterceptor {
	return NewAuthInterceptor(nil, &config.Config{
		Auth: config.AuthConfig{
			BackendTokenPublicKey: testVerifyPublicKey,
			BackendTokenIssuer:    testIssuer,
			BackendTokenAudience:  testAudience,
		},
	})
}

func signTestToken(t *testing.T, claims *BackendClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	signed, err := token.SignedString(testSigningPrivateKey)
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

func TestValidateToken_MissingExpClaim_Fails(t *testing.T) {
	userID := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	tenantID := uuid.MustParse("88888888-8888-8888-8888-888888888888")

	claims := baseClaims(userID)
	claims.TenantID = tenantID.String()
	claims.ExpiresAt = nil
	tokenStr := signTestToken(t, claims)

	a := newTestInterceptor()
	_, err := a.validateToken(tokenStr)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidToken) || errors.Is(err, errInvalidClaims),
		"missing exp claim must be rejected; got %v", err)
}

func TestAuthInterceptor_EdDSAContract(t *testing.T) {
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	userID := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	tenantID := uuid.MustParse("88888888-8888-8888-8888-888888888888")

	t.Run("valid EdDSA token accepted", func(t *testing.T) {
		claims := baseClaims(userID)
		claims.TenantID = tenantID.String()
		tokenStr := signTestToken(t, claims)

		a := newTestInterceptor()
		called := false
		unary := a.Interceptor().WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			called = true
			u, err := GetUserContext(ctx)
			require.NoError(t, err)
			assert.Equal(t, userID, u.UserID)
			assert.Equal(t, tenantID, u.TenantID)
			return connect.NewResponse(&struct{}{}), nil
		})
		req := connect.NewRequest(&struct{}{})
		req.Header().Set(backendTokenHeader, tokenStr)
		_, unaryErr := unary(context.Background(), req)
		require.NoError(t, unaryErr)
		require.True(t, called)
	})

	rejectCases := []struct {
		name      string
		makeToken func(t *testing.T) string
	}{
		{
			name: "HS256 signed with public-key bytes as HMAC secret rejected",
			makeToken: func(t *testing.T) string {
				claims := baseClaims(userID)
				claims.TenantID = tenantID.String()
				tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				s, err := tok.SignedString([]byte(testVerifyPublicKey))
				require.NoError(t, err)
				return s
			},
		},
		{
			name: "alg none rejected",
			makeToken: func(t *testing.T) string {
				claims := baseClaims(userID)
				claims.TenantID = tenantID.String()
				tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
				s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
				require.NoError(t, err)
				return s
			},
		},
		{
			name: "token signed by a different Ed25519 key rejected",
			makeToken: func(t *testing.T) string {
				claims := baseClaims(userID)
				claims.TenantID = tenantID.String()
				tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
				s, err := tok.SignedString(otherPriv)
				require.NoError(t, err)
				return s
			},
		},
		{
			name: "wrong issuer rejected",
			makeToken: func(t *testing.T) string {
				claims := baseClaims(userID)
				claims.TenantID = tenantID.String()
				claims.Issuer = "other-issuer"
				return signTestToken(t, claims)
			},
		},
		{
			name: "wrong audience rejected",
			makeToken: func(t *testing.T) string {
				claims := baseClaims(userID)
				claims.TenantID = tenantID.String()
				claims.Audience = jwt.ClaimStrings{"other-audience"}
				return signTestToken(t, claims)
			},
		},
		{
			name: "expired token rejected",
			makeToken: func(t *testing.T) string {
				claims := baseClaims(userID)
				claims.TenantID = tenantID.String()
				claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Minute))
				return signTestToken(t, claims)
			},
		},
	}

	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			tokenStr := tc.makeToken(t)
			a := newTestInterceptor()
			_, err := a.validateToken(tokenStr)
			require.Error(t, err)

			unary := a.Interceptor().WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				return connect.NewResponse(&struct{}{}), nil
			})
			req := connect.NewRequest(&struct{}{})
			req.Header().Set(backendTokenHeader, tokenStr)
			_, unaryErr := unary(context.Background(), req)
			require.Error(t, unaryErr)
			assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(unaryErr))
		})
	}
}

func TestAuthInterceptor_RejectsNilUUIDClaims(t *testing.T) {
	validUserID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	validTenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name     string
		userID   uuid.UUID
		tenantID uuid.UUID
	}{
		{
			name:     "nil sub",
			userID:   uuid.Nil,
			tenantID: validTenantID,
		},
		{
			name:     "nil tenant_id",
			userID:   validUserID,
			tenantID: uuid.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := baseClaims(tt.userID)
			claims.TenantID = tt.tenantID.String()
			tokenStr := signTestToken(t, claims)

			a := newTestInterceptor()
			_, err := a.validateToken(tokenStr)
			require.Error(t, err)
			assert.Equal(t, connect.CodeUnauthenticated, a.toConnectError(err).Code())

			unary := a.Interceptor().WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				return connect.NewResponse(&struct{}{}), nil
			})
			req := connect.NewRequest(&struct{}{})
			req.Header().Set(backendTokenHeader, tokenStr)
			_, unaryErr := unary(context.Background(), req)
			require.Error(t, unaryErr)
			assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(unaryErr))
		})
	}
}
