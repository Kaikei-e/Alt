package middleware

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alt/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJWTAuthMiddleware_PanicsOnInvalidPublicKey(t *testing.T) {
	t.Run("nil key panics", func(t *testing.T) {
		require.Panics(t, func() {
			NewJWTAuthMiddleware(nil, config.AuthConfig{})
		})
	})

	t.Run("short key panics", func(t *testing.T) {
		require.Panics(t, func() {
			NewJWTAuthMiddleware(nil, config.AuthConfig{
				BackendTokenPublicKey: ed25519.PublicKey("short"),
			})
		})
	})
}

func validRESTBackendClaims(userID string, now time.Time) BackendClaims {
	return BackendClaims{
		Email:    "user@example.com",
		Role:     "user",
		Sid:      "session-1",
		TenantID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "auth-hub",
			Audience:  jwt.ClaimStrings{"alt-backend"},
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	}
}

func TestJWTAuthMiddleware_EdDSAContract(t *testing.T) {
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	userID := uuid.New().String()
	now := time.Now()

	t.Run("valid EdDSA token accepted", func(t *testing.T) {
		claims := validRESTBackendClaims(userID, now)
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		signed, err := token.SignedString(testBackendTokenPrivateKey)
		require.NoError(t, err)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
		req.Header.Set(backendTokenHeader, signed)
		res := httptest.NewRecorder()
		c := e.NewContext(req, res)

		mw := NewJWTAuthMiddleware(nil, testAuthConfig())
		called := false
		h := mw.RequireJWT()(func(c echo.Context) error {
			called = true
			userCtx, ok := GetUserContext(c)
			require.True(t, ok)
			assert.Equal(t, userID, userCtx.ID)
			assert.Equal(t, userID, userCtx.TenantID)
			assert.Equal(t, "user@example.com", userCtx.Email)
			return c.NoContent(http.StatusOK)
		})
		require.NoError(t, h(c))
		require.True(t, called)
	})

	rejectCases := []struct {
		name      string
		makeToken func(t *testing.T) string
	}{
		{
			name: "HS256 signed with public-key bytes as HMAC secret rejected",
			makeToken: func(t *testing.T) string {
				claims := validRESTBackendClaims(userID, now)
				tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				s, err := tok.SignedString([]byte(testBackendTokenPublicKey))
				require.NoError(t, err)
				return s
			},
		},
		{
			name: "alg none rejected",
			makeToken: func(t *testing.T) string {
				claims := validRESTBackendClaims(userID, now)
				tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
				s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
				require.NoError(t, err)
				return s
			},
		},
		{
			name: "token signed by a different Ed25519 key rejected",
			makeToken: func(t *testing.T) string {
				claims := validRESTBackendClaims(userID, now)
				tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
				s, err := tok.SignedString(otherPriv)
				require.NoError(t, err)
				return s
			},
		},
		{
			name: "wrong issuer rejected",
			makeToken: func(t *testing.T) string {
				claims := validRESTBackendClaims(userID, now)
				claims.Issuer = "other-issuer"
				tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
				s, err := tok.SignedString(testBackendTokenPrivateKey)
				require.NoError(t, err)
				return s
			},
		},
		{
			name: "wrong audience rejected",
			makeToken: func(t *testing.T) string {
				claims := validRESTBackendClaims(userID, now)
				claims.Audience = jwt.ClaimStrings{"other-audience"}
				tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
				s, err := tok.SignedString(testBackendTokenPrivateKey)
				require.NoError(t, err)
				return s
			},
		},
		{
			name: "expired token rejected",
			makeToken: func(t *testing.T) string {
				claims := validRESTBackendClaims(userID, now)
				claims.ExpiresAt = jwt.NewNumericDate(now.Add(-1 * time.Minute))
				tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
				s, err := tok.SignedString(testBackendTokenPrivateKey)
				require.NoError(t, err)
				return s
			},
		},
		{
			name: "missing exp claim rejected",
			makeToken: func(t *testing.T) string {
				claims := validRESTBackendClaims(userID, now)
				claims.ExpiresAt = nil
				tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
				s, err := tok.SignedString(testBackendTokenPrivateKey)
				require.NoError(t, err)
				return s
			},
		},
	}

	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			signed := tc.makeToken(t)
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
			req.Header.Set(backendTokenHeader, signed)
			res := httptest.NewRecorder()
			c := e.NewContext(req, res)

			mw := NewJWTAuthMiddleware(nil, testAuthConfig())
			called := false
			h := mw.RequireJWT()(func(c echo.Context) error {
				called = true
				return c.NoContent(http.StatusOK)
			})
			err := h(c)
			require.Error(t, err)
			httpErr, ok := err.(*echo.HTTPError)
			require.True(t, ok)
			assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
			assert.False(t, called)
		})
	}
}
