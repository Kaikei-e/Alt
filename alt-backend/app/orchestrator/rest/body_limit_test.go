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
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

type stubCSRFGateway struct{}

func (stubCSRFGateway) GenerateToken(context.Context) (string, error)       { return "test-token", nil }
func (stubCSRFGateway) ValidateToken(context.Context, string) (bool, error) { return true, nil }
func (stubCSRFGateway) InvalidateToken(context.Context, string) error       { return nil }

func buildRegisteredEcho(t *testing.T, mutate func(*config.Config)) *echo.Echo {
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
	}, cfg)
	return e
}

func TestBodyLimit_RejectsBodyOverTwoMegabytes(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8")
	e := buildRegisteredEcho(t, nil)
	body := bytes.Repeat([]byte("x"), 3*1024*1024)
	req := httptest.NewRequest(http.MethodPost, "/v1/feeds/search", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, res.Code)
}

func TestBodyLimit_AllowsBodyUnderTwoMegabytes(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8")
	e := buildRegisteredEcho(t, nil)
	e.POST("/v1/test-body-ok", func(c echo.Context) error {
		_, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.NoContent(http.StatusNoContent)
	})
	body := bytes.Repeat([]byte("{"), 1)
	body = append(body, bytes.Repeat([]byte(" "), 512*1024)...)
	body = append(body, '}')
	req := httptest.NewRequest(http.MethodPost, "/v1/test-body-ok", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-CSRF-Token", "test-token")
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)
	require.Equal(t, http.StatusNoContent, res.Code)
}

func TestBodyLimit_RejectsOversizedBodyOnStreamingRoutes(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8")
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
			body := bytes.Repeat([]byte("x"), 3*1024*1024)
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			res := httptest.NewRecorder()
			e.ServeHTTP(res, req)
			require.Equal(t, http.StatusRequestEntityTooLarge, res.Code)
		})
	}

	t.Run("chunked oversized body on streaming route", func(t *testing.T) {
		chunked := io.MultiReader(bytes.NewReader(bytes.Repeat([]byte("x"), 3*1024*1024)))
		req := httptest.NewRequest(http.MethodPost, "/sse/test-chunked", chunked)
		req.ContentLength = -1
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set("X-CSRF-Token", "test-token")
		res := httptest.NewRecorder()
		e.ServeHTTP(res, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, res.Code)
	})
}

type slowReader struct {
	delay time.Duration
}

func (s slowReader) Read(p []byte) (int, error) {
	time.Sleep(s.delay)
	return 0, io.EOF
}

func TestStreamingRoute_BoundsBodyReadAndAllowsLongStreamResponse(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8")
	e := buildRegisteredEcho(t, func(cfg *config.Config) {
		cfg.Server.ReadTimeout = 40 * time.Millisecond
	})

	e.POST("/sse/test-body-timeout", func(c echo.Context) error {
		if _, err := io.ReadAll(c.Request().Body); err != nil {
			return echo.NewHTTPError(http.StatusRequestTimeout, "request body read timed out")
		}
		c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
		c.Response().WriteHeader(http.StatusOK)
		time.Sleep(90 * time.Millisecond)
		_, _ = c.Response().Write([]byte("data: ok\n\n"))
		c.Response().Flush()
		return nil
	})

	t.Run("slow request body on streaming route is cut off", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sse/test-body-timeout", slowReader{delay: 120 * time.Millisecond})
		req.Header.Set("X-CSRF-Token", "test-token")
		res := httptest.NewRecorder()
		e.ServeHTTP(res, req)
		require.Equal(t, http.StatusRequestTimeout, res.Code)
	})

	t.Run("fast request body with long streaming response succeeds", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sse/test-body-timeout", bytes.NewReader([]byte("{}")))
		req.Header.Set("X-CSRF-Token", "test-token")
		res := httptest.NewRecorder()
		e.ServeHTTP(res, req)
		require.Equal(t, http.StatusOK, res.Code)
		require.Contains(t, res.Body.String(), "data: ok")
	})
}

func TestRegisterRoutes_TrustedProxyCIDRs(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 172.16.0.0/12")
	e := buildRegisteredEcho(t, func(cfg *config.Config) {
		cfg.RateLimit.DOSProtection = config.DOSProtectionConfig{
			Enabled:       true,
			RateLimit:     2,
			BurstLimit:    2,
			WindowSize:    time.Minute,
			BlockDuration: time.Minute,
		}
	})
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
