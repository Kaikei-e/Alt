package rest

import (
	"alt/config"
	"alt/di"
	"alt/orchestrator/usecase/csrf_token_usecase"
	"alt/utils/logger"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

type stubCSRFGateway struct{}

func (stubCSRFGateway) GenerateToken(context.Context) (string, error)       { return "test-token", nil }
func (stubCSRFGateway) ValidateToken(context.Context, string) (bool, error) { return true, nil }
func (stubCSRFGateway) InvalidateToken(context.Context, string) error       { return nil }

func buildRegisteredEcho(t *testing.T, mutate func(*config.Config), trustedProxies ...netip.Prefix) *echo.Echo {
	t.Helper()
	logger.InitLogger()
	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		if he, ok := err.(*echo.HTTPError); ok {
			_ = c.JSON(he.Code, map[string]any{"error": http.StatusText(he.Code)})
			return
		}
		_ = c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
	cfg := &config.Config{}
	cfg.Server.ReadTimeout = 5 * time.Second
	if mutate != nil {
		mutate(cfg)
	}
	RegisterRoutes(context.Background(), e, &di.ApplicationComponents{
		CSRFTokenUsecase: csrf_token_usecase.NewCSRFTokenUsecase(stubCSRFGateway{}),
	}, cfg, trustedProxies...)
	return e
}

func TestBodyLimit_EnforcesTwoMiBByteBoundary(t *testing.T) {
	const twoMiB = config.MaxRequestBodyBytes

	e := buildRegisteredEcho(t, nil)
	e.POST("/v1/test-body-boundary", func(c echo.Context) error {
		_, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.NoContent(http.StatusNoContent)
	})

	t.Run("2097152 bytes accepted", func(t *testing.T) {
		body := bytes.Repeat([]byte("a"), twoMiB)
		req := httptest.NewRequest(http.MethodPost, "/v1/test-body-boundary", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set("X-CSRF-Token", "test-token")
		res := httptest.NewRecorder()
		e.ServeHTTP(res, req)
		require.Equal(t, http.StatusNoContent, res.Code)
	})

	t.Run("2097153 bytes rejected with 413", func(t *testing.T) {
		body := bytes.Repeat([]byte("a"), twoMiB+1)
		req := httptest.NewRequest(http.MethodPost, "/v1/test-body-boundary", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set("X-CSRF-Token", "test-token")
		res := httptest.NewRecorder()
		e.ServeHTTP(res, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, res.Code)
	})

	t.Run("chunked 2097153 bytes rejected with 413", func(t *testing.T) {
		chunked := io.MultiReader(bytes.NewReader(bytes.Repeat([]byte("a"), twoMiB+1)))
		req := httptest.NewRequest(http.MethodPost, "/v1/test-body-boundary", chunked)
		req.ContentLength = -1
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set("X-CSRF-Token", "test-token")
		res := httptest.NewRecorder()
		e.ServeHTTP(res, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, res.Code)
	})
}

func TestBodyLimit_RejectsOversizedBodyOnStreamingRoutes(t *testing.T) {
	e := buildRegisteredEcho(t, nil)
	e.POST("/sse/test-chunked", func(c echo.Context) error {
		_, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.NoContent(http.StatusOK)
	})

	for _, path := range []string{"/v1/feeds/summarize/stream", "/sse/v1/rag/answer"} {
		t.Run(path, func(t *testing.T) {
			body := bytes.Repeat([]byte("x"), (2<<20)+1)
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			res := httptest.NewRecorder()
			e.ServeHTTP(res, req)
			require.Equal(t, http.StatusRequestEntityTooLarge, res.Code)
		})
	}

	t.Run("chunked oversized body on streaming route", func(t *testing.T) {
		chunked := io.MultiReader(bytes.NewReader(bytes.Repeat([]byte("x"), (2<<20)+1)))
		req := httptest.NewRequest(http.MethodPost, "/sse/test-chunked", chunked)
		req.ContentLength = -1
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set("X-CSRF-Token", "test-token")
		res := httptest.NewRecorder()
		e.ServeHTTP(res, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, res.Code)
	})
}

func TestRegisterRoutes_TrustedProxyCIDRs(t *testing.T) {
	trusted, err := config.ParseTrustedProxyCIDRs("10.0.0.0/8, 172.16.0.0/12")
	require.NoError(t, err)
	e := buildRegisteredEcho(t, func(cfg *config.Config) {
		cfg.RateLimit.DOSProtection = config.DOSProtectionConfig{
			Enabled:       true,
			RateLimit:     2,
			BurstLimit:    2,
			WindowSize:    time.Minute,
			BlockDuration: time.Minute,
		}
	}, trusted...)
	e.GET("/v1/echo-ip", func(c echo.Context) error {
		return c.String(http.StatusOK, c.RealIP())
	})

	t.Run("spoofed leftmost XFF entry is ignored when peer is trusted", func(t *testing.T) {
		spoofedLeft := []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"}
		wantStatus := []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests}
		for i, spoof := range spoofedLeft {
			req := httptest.NewRequest(http.MethodGet, "/v1/echo-ip", nil)
			req.RemoteAddr = "10.0.0.2:4321"
			req.Header.Set("X-Forwarded-For", spoof+", 203.0.113.42, 172.16.0.9")
			req.Header.Set("X-Real-IP", spoof)
			res := httptest.NewRecorder()
			e.ServeHTTP(res, req)
			require.Equal(t, wantStatus[i], res.Code)
			if res.Code == http.StatusOK {
				require.Equal(t, "203.0.113.42", res.Body.String())
			}
		}
	})

	t.Run("direct peer outside trusted CIDRs has forwarded headers ignored", func(t *testing.T) {
		spoofed := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}
		wantStatus := []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests}
		for i, spoof := range spoofed {
			req := httptest.NewRequest(http.MethodGet, "/v1/echo-ip", nil)
			req.RemoteAddr = "198.51.100.77:5555"
			req.Header.Set("X-Forwarded-For", spoof)
			req.Header.Set("X-Real-IP", spoof)
			res := httptest.NewRecorder()
			e.ServeHTTP(res, req)
			require.Equal(t, wantStatus[i], res.Code)
			if res.Code == http.StatusOK {
				require.Equal(t, "198.51.100.77", res.Body.String())
			}
		}
	})
}
