package main

import (
	"alt/config"
	"alt/di"
	"alt/middleware"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
	"github.com/stretchr/testify/require"
)

type recordedLog struct {
	level   slog.Level
	message string
	attrs   map[string]string
}

type captureHandler struct {
	mu      sync.Mutex
	records []recordedLog
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]string)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, recordedLog{
		level:   r.Level,
		message: r.Message,
		attrs:   attrs,
	})
	h.mu.Unlock()
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &captureSubHandler{parent: h, prefix: attrs}
}

func (h *captureHandler) WithGroup(string) slog.Handler { return h }

type captureSubHandler struct {
	parent *captureHandler
	prefix []slog.Attr
}

func (s *captureSubHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return s.parent.Enabled(ctx, lvl)
}

func (s *captureSubHandler) Handle(ctx context.Context, r slog.Record) error {
	clone := r.Clone()
	clone.AddAttrs(s.prefix...)
	return s.parent.Handle(ctx, clone)
}

func (s *captureSubHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(s.prefix)+len(attrs))
	merged = append(merged, s.prefix...)
	merged = append(merged, attrs...)
	return &captureSubHandler{parent: s.parent, prefix: merged}
}

func (s *captureSubHandler) WithGroup(string) slog.Handler { return s }

func (h *captureHandler) byLevel(lvl slog.Level) []recordedLog {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []recordedLog
	for _, rec := range h.records {
		if rec.level == lvl {
			out = append(out, rec)
		}
	}
	return out
}

type stubCSRFUsecase struct{}

func (stubCSRFUsecase) GenerateToken(context.Context) (string, error)       { return "tok", nil }
func (stubCSRFUsecase) ValidateToken(context.Context, string) (bool, error) { return true, nil }

type customStatusCoderError struct {
	code int
	msg  string
}

func (e customStatusCoderError) StatusCode() int { return e.code }
func (e customStatusCoderError) Error() string   { return e.msg }

func TestNewRESTEcho_HTTPErrorHandlerResponses(t *testing.T) {
	e := newRESTEcho(&di.ApplicationComponents{}, &config.Config{}, false, slog.New(slog.DiscardHandler))
	e.POST("/v1/feeds/read", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	}, middleware.CSRFMiddleware(stubCSRFUsecase{}))
	e.POST("/v1/feeds/search", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	}, middleware.ValidationMiddleware())
	e.GET("/v1/boom", func(c *echo.Context) error {
		return errors.New("db down")
	})

	t.Run("CSRF missing token preserves v4 JSON body", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/feeds/read", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusForbidden, rec.Code)
		require.JSONEq(t, `{"error":"Forbidden","detail":{"error":"csrf_token_missing","message":"CSRF token is required"}}`, rec.Body.String())
		t.Logf("CSRF response body: %s", strings.TrimSpace(rec.Body.String()))
	})

	t.Run("Validation missing query preserves v4 JSON body", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/feeds/search", strings.NewReader(`{"other":"x"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.JSONEq(t, `{"error":"Bad Request","detail":{"error":"validation_failed","message":"Validation failed","details":[{"field":"query","message":"query field is required"}]}}`, rec.Body.String())
		t.Logf("Validation response body: %s", strings.TrimSpace(rec.Body.String()))
	})

	t.Run("echo.ErrNotFound returns 404 not 500", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/does-not-exist", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"error":"Not Found","detail":"Not Found"}`, rec.Body.String())
	})

	t.Run("echo.ErrMethodNotAllowed returns 405 not 500", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/feeds/read", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		require.JSONEq(t, `{"error":"Method Not Allowed","detail":"Method Not Allowed"}`, rec.Body.String())
	})

	t.Run("echo.ErrStatusRequestEntityTooLarge returns 413 with byte-identical body", func(t *testing.T) {
		e.POST("/v1/too-large", func(c *echo.Context) error {
			return echo.ErrStatusRequestEntityTooLarge
		})
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/too-large", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		require.JSONEq(t, `{"error":"Request Entity Too Large","detail":"Request Entity Too Large"}`, rec.Body.String())
	})

	t.Run("unclassified error returns 500 internal_error", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/boom", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		require.JSONEq(t, `{"error":"internal_error"}`, rec.Body.String())
	})
}

func TestNewRESTEcho_PanicLoggedOnceAndReturns500(t *testing.T) {
	capture := &captureHandler{}
	testLog := slog.New(capture)
	e := newRESTEcho(&di.ApplicationComponents{}, &config.Config{}, false, testLog)
	e.Use(middleware.RequestIDMiddleware())
	e.Use(echomiddleware.Recover())
	// LoggingMiddleware sits inside Recover (matching orchestrator/rest/routes.go),
	// so panics unwind past it and must be logged by HTTPErrorHandler.
	e.Use(middleware.LoggingMiddleware(testLog))
	e.POST("/v1/panic", func(c *echo.Context) error {
		panic("register_favorite_feed_usecase: home-open hook is not wired")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/panic", strings.NewReader(`{"secret":"sensitive-body-value"}`))
	req.Header.Set("Authorization", "Bearer secret-bearer-token")
	req.Header.Set("Cookie", "ory_kratos_session=secret-cookie-value")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"internal_error"}`, rec.Body.String())

	errLogs := capture.byLevel(slog.LevelError)
	require.Len(t, errLogs, 1, "recovered panic must produce exactly one error log record")
	entry := errLogs[0]
	require.Equal(t, http.MethodPost, entry.attrs["method"])
	require.Equal(t, "/v1/panic", entry.attrs["path"])
	joined := entry.message
	for k, v := range entry.attrs {
		joined += " " + k + "=" + v
	}
	require.Contains(t, joined, "register_favorite_feed_usecase: home-open hook is not wired")
	require.NotEmpty(t, entry.attrs["stack"])
	require.Contains(t, entry.attrs["stack"], "goroutine ")
	require.NotContains(t, joined, "secret-bearer-token")
	require.NotContains(t, joined, "secret-cookie-value")
	require.NotContains(t, joined, "sensitive-body-value")
}

func TestNewRESTEcho_StatusCoderErrorRedactedAndLogged(t *testing.T) {
	capture := &captureHandler{}
	testLog := slog.New(capture)
	e := newRESTEcho(&di.ApplicationComponents{}, &config.Config{}, false, testLog)
	e.GET("/v1/status-err", func(c *echo.Context) error {
		return customStatusCoderError{
			code: http.StatusBadRequest,
			msg:  "strconv.ParseInt: internal detail must not reach client",
		}
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/status-err", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"Bad Request","detail":"Bad Request"}`, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "internal detail must not reach client")

	warnLogs := capture.byLevel(slog.LevelWarn)
	require.Len(t, warnLogs, 1, "client error should be logged once at warn level")
	require.Contains(t, warnLogs[0].attrs["error"], "internal detail must not reach client")
}
