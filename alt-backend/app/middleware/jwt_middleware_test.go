package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestJWTAuthMiddleware_MissingExpirationRejected(t *testing.T) {
	e := echo.New()
	userID := uuid.New().String()
	claims := BackendClaims{
		Email:    "user@example.com",
		Role:     "user",
		Sid:      "session-1",
		TenantID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "auth-hub",
			Audience: jwt.ClaimStrings{"alt-backend"},
			Subject:  userID,
			IssuedAt: jwt.NewNumericDate(time.Now()),
			// ExpiresAt deliberately omitted
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testBackendTokenSecret))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	req.Header.Set(backendTokenHeader, signed)
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	mw := NewJWTAuthMiddleware(nil, testAuthConfig())
	_, err = mw.validateJWT(c)
	require.Error(t, err, "token without exp claim must be rejected")
}

func TestJWTAuthMiddleware_UnexpectedAlgorithmRejected(t *testing.T) {
	e := echo.New()
	userID := uuid.New().String()
	now := time.Now()
	claims := BackendClaims{
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
	// Sign with HS384 instead of HS256
	token := jwt.NewWithClaims(jwt.SigningMethodHS384, claims)
	signed, err := token.SignedString([]byte(testBackendTokenSecret))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	req.Header.Set(backendTokenHeader, signed)
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	mw := NewJWTAuthMiddleware(nil, testAuthConfig())
	_, err = mw.validateJWT(c)
	require.Error(t, err, "token signed with HS384 instead of HS256 must be rejected")
}
