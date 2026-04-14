package rest

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/require"
)

// H-005: The backend must enforce a global body size limit so that large
// POSTs cannot exhaust memory. 2 MiB is chosen to accommodate the 1 MB
// per-file OPML limit plus multipart boundaries.

// helper that mirrors the production body limit application.
func buildBodyLimitEcho() *echo.Echo {
	e := echo.New()
	e.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
		Skipper: func(c echo.Context) bool {
			return strings.Contains(c.Path(), "/sse/") || strings.Contains(c.Path(), "/stream")
		},
		Limit: "2M",
	}))
	e.POST("/v1/feeds/search", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.POST("/v1/feeds/summarize/stream", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	return e
}

func TestBodyLimit_RejectsBodyOverTwoMegabytes(t *testing.T) {
	e := buildBodyLimitEcho()
	body := bytes.Repeat([]byte("x"), 3*1024*1024) // 3 MiB
	req := httptest.NewRequest(http.MethodPost, "/v1/feeds/search", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, res.Code)
}

func TestBodyLimit_AllowsBodyUnderTwoMegabytes(t *testing.T) {
	e := buildBodyLimitEcho()
	body := bytes.Repeat([]byte("x"), 512*1024) // 512 KiB
	req := httptest.NewRequest(http.MethodPost, "/v1/feeds/search", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
}

func TestBodyLimit_SkipsStreamingPaths(t *testing.T) {
	e := buildBodyLimitEcho()
	body := bytes.Repeat([]byte("x"), 3*1024*1024) // 3 MiB
	req := httptest.NewRequest(http.MethodPost, "/v1/feeds/summarize/stream", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code,
		"streaming endpoints must not be affected by the global body limit")
}
