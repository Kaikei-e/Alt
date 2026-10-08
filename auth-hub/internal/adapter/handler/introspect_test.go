package handler

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"auth-hub/internal/domain"
	"auth-hub/internal/infrastructure/token"
	"auth-hub/internal/usecase"
	"auth-hub/middleware"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestHandleIntrospectToken(t *testing.T) {
	jwtCfg := token.JWTConfig{
		Secret:   "test-secret",
		Issuer:   "auth-hub-test",
		Audience: "alt-backend",
		TTL:      5 * time.Minute,
	}
	issuer := token.NewJWTIssuer(jwtCfg)
	introspectUC := usecase.NewIntrospectToken(issuer)
	h := NewInternalHandler(nil, nil, introspectUC)

	e := echo.New()

	// Dedicated introspection limiter (e.g. 1200/min = 20 req/s, burst 100)
	introspectRL := middleware.NewRateLimiter(rate.Limit(1200.0/60.0), 100)
	introspectGroup := e.Group("/internal", introspectRL.Middleware())
	introspectGroup.POST("/token/introspect", h.HandleIntrospectToken, middleware.RequireMTLSPeer([]string{"search-indexer", "knowledge-sovereign"}))

	// Separate cache invalidation limiter (10 req/min = 1/6 req/s, burst 3)
	internalRL := middleware.NewRateLimiter(10.0/60.0, 3)
	internalGroup := e.Group("/internal", internalRL.Middleware())
	internalGroup.POST("/session/invalidate", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]bool{"ok": true})
	})

	userUUID := uuid.New().String()
	tenantUUID := uuid.New().String()

	validToken, err := issuer.IssueBackendToken(&domain.Identity{
		UserID:   userUUID,
		TenantID: tenantUUID,
		Role:     "user",
	}, "session-1")
	require.NoError(t, err)

	setupRequest := func(cn string, token string, hasTLS bool) *httptest.ResponseRecorder {
		body := `{"token":"` + token + `"}`
		req := httptest.NewRequest(http.MethodPost, "/internal/token/introspect", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

		if hasTLS {
			req.TLS = &tls.ConnectionState{
				VerifiedChains: [][]*x509.Certificate{
					{
						{Subject: pkix.Name{CommonName: cn}},
					},
				},
			}
		}

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	t.Run("valid minted token yields claims for allowed peer", func(t *testing.T) {
		rec := setupRequest("search-indexer", validToken, true)
		assert.Equal(t, http.StatusOK, rec.Code)

		var res domain.IntrospectedToken
		err = json.Unmarshal(rec.Body.Bytes(), &res)
		require.NoError(t, err)
		assert.True(t, res.Active)
		assert.Equal(t, userUUID, res.Sub)
		assert.Equal(t, tenantUUID, res.TenantID)
	})

	t.Run("foreign CN fails", func(t *testing.T) {
		rec := setupRequest("unknown-service", validToken, true)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("exposed frontend/plain path denied (no TLS/no certs)", func(t *testing.T) {
		rec := setupRequest("", validToken, false)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("wrong alg/tenant missing fails", func(t *testing.T) {
		// Hand-craft a token with none alg
		claims := jwt.RegisteredClaims{
			Subject: userUUID,
		}
		badToken, _ := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)

		rec := setupRequest("knowledge-sovereign", badToken, true)
		assert.Equal(t, http.StatusOK, rec.Code)

		var res map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &res)
		require.NoError(t, err)
		assert.False(t, res["active"].(bool))
	})

	t.Run("burst 5+ positive introspection requests from service IP do not 429", func(t *testing.T) {
		serviceIP := "10.244.0.15:54321"
		for i := 0; i < 10; i++ {
			body := `{"token":"` + validToken + `"}`
			req := httptest.NewRequest(http.MethodPost, "/internal/token/introspect", strings.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.RemoteAddr = serviceIP
			req.TLS = &tls.ConnectionState{
				VerifiedChains: [][]*x509.Certificate{
					{
						{Subject: pkix.Name{CommonName: "search-indexer"}},
					},
				},
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code, "request %d should succeed without 429", i+1)
			var res domain.IntrospectedToken
			err := json.Unmarshal(rec.Body.Bytes(), &res)
			require.NoError(t, err)
			assert.True(t, res.Active)
			assert.Equal(t, userUUID, res.Sub)
			assert.Equal(t, tenantUUID, res.TenantID)
		}
	})

	t.Run("configured limit exhaustion still triggers 429", func(t *testing.T) {
		// Create a route with a tightly configured limiter (burst 5)
		tightLimiter := middleware.NewRateLimiter(1.0, 5)
		tightRouter := echo.New()
		tightGroup := tightRouter.Group("/internal", tightLimiter.Middleware())
		tightGroup.POST("/token/introspect", h.HandleIntrospectToken, middleware.RequireMTLSPeer([]string{"knowledge-sovereign"}))

		serviceIP := "10.244.1.20:41234"
		for i := 0; i < 5; i++ {
			body := `{"token":"` + validToken + `"}`
			req := httptest.NewRequest(http.MethodPost, "/internal/token/introspect", strings.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.RemoteAddr = serviceIP
			req.TLS = &tls.ConnectionState{
				VerifiedChains: [][]*x509.Certificate{
					{
						{Subject: pkix.Name{CommonName: "knowledge-sovereign"}},
					},
				},
			}
			rec := httptest.NewRecorder()
			tightRouter.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code, "request %d within burst should succeed", i+1)
		}

		// 6th request exhausts burst -> 429
		body := `{"token":"` + validToken + `"}`
		req := httptest.NewRequest(http.MethodPost, "/internal/token/introspect", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.RemoteAddr = serviceIP
		req.TLS = &tls.ConnectionState{
			VerifiedChains: [][]*x509.Certificate{
				{
					{Subject: pkix.Name{CommonName: "knowledge-sovereign"}},
				},
			},
		}
		rec := httptest.NewRecorder()
		tightRouter.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code, "request exceeding burst must be 429")
	})

	t.Run("separate cache invalidate limiter retains 10/min burst 3", func(t *testing.T) {
		serviceIP := "10.244.2.30:52345"
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest(http.MethodPost, "/internal/session/invalidate", strings.NewReader(`{"cookie":"c"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.RemoteAddr = serviceIP
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code, "invalidation request %d should succeed", i+1)
		}

		// 4th immediate request must be 429
		req4 := httptest.NewRequest(http.MethodPost, "/internal/session/invalidate", strings.NewReader(`{"cookie":"c"}`))
		req4.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req4.RemoteAddr = serviceIP
		rec4 := httptest.NewRecorder()
		e.ServeHTTP(rec4, req4)
		assert.Equal(t, http.StatusTooManyRequests, rec4.Code, "4th invalidation request must be rate limited (burst 3 exhausted)")
	})
}

type fakeTokenVerifier struct {
	result *domain.IntrospectedToken
	err    error
}

func (f *fakeTokenVerifier) IntrospectBackendToken(_ string) (*domain.IntrospectedToken, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestInternalHandler_HandleIntrospectToken(t *testing.T) {
	e := echo.New()

	t.Run("active token returns snake_case claims", func(t *testing.T) {
		subUUID := uuid.New().String()
		tenantUUID := uuid.New().String()
		exp := int64(1800000000)

		fakeVerifier := &fakeTokenVerifier{
			result: &domain.IntrospectedToken{
				Active:   true,
				Sub:      subUUID,
				TenantID: tenantUUID,
				Exp:      exp,
			},
		}
		introspectUC := usecase.NewIntrospectToken(fakeVerifier)
		h := NewInternalHandler(nil, nil, introspectUC)

		body := `{"token":"active-token"}`
		req := httptest.NewRequest(http.MethodPost, "/internal/token/introspect", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.HandleIntrospectToken(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
		dec.UseNumber()
		err = dec.Decode(&resp)
		require.NoError(t, err)

		keys := make([]string, 0, len(resp))
		for k := range resp {
			keys = append(keys, k)
		}
		assert.ElementsMatch(t, []string{"active", "sub", "tenant_id", "exp"}, keys)
		assert.Equal(t, true, resp["active"])
		assert.Equal(t, subUUID, resp["sub"])
		assert.Equal(t, tenantUUID, resp["tenant_id"])
		assert.Equal(t, json.Number(strconv.FormatInt(exp, 10)), resp["exp"])
	})

	t.Run("inactive token returns active false only", func(t *testing.T) {
		fakeVerifier := &fakeTokenVerifier{
			err: errors.New("token verification failed"),
		}
		introspectUC := usecase.NewIntrospectToken(fakeVerifier)
		h := NewInternalHandler(nil, nil, introspectUC)

		body := `{"token":"invalid-token"}`
		req := httptest.NewRequest(http.MethodPost, "/internal/token/introspect", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.HandleIntrospectToken(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
		dec.UseNumber()
		err = dec.Decode(&resp)
		require.NoError(t, err)

		keys := make([]string, 0, len(resp))
		for k := range resp {
			keys = append(keys, k)
		}
		assert.ElementsMatch(t, []string{"active"}, keys)
		assert.Equal(t, false, resp["active"])
		assert.Equal(t, map[string]any{"active": false}, resp)
	})
}
