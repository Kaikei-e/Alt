package rest

import (
	"alt/utils/logger"
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleCSPReport_BoundsBodyAt16KiB(t *testing.T) {
	logger.InitLogger()
	e := echo.New()
	e.POST("/security/csp-report", handleCSPReport)

	oversized := bytes.Repeat([]byte("a"), 16*1024+1)
	req := httptest.NewRequest(http.MethodPost, "/security/csp-report", bytes.NewReader(oversized))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestHandleCSPReport_InvalidJSONDoesNotLogRawBody(t *testing.T) {
	var logBuf bytes.Buffer
	origLogger := logger.Logger
	logger.Logger = slog.New(slog.NewTextHandler(&logBuf, nil))
	defer func() {
		logger.Logger = origLogger
	}()

	e := echo.New()
	e.POST("/security/csp-report", handleCSPReport)

	secretMarker := "SENSITIVE_RAW_PAYLOAD_MARKER_DO_NOT_LOG"
	req := httptest.NewRequest(http.MethodPost, "/security/csp-report", strings.NewReader("{invalid-json:"+secretMarker))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.NotContains(t, logBuf.String(), secretMarker, "raw body must not be logged when JSON unmarshaling fails")
	assert.Contains(t, logBuf.String(), "body_len=")
}
