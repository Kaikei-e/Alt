package resterr

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"alt/utils/errors"
	"alt/utils/logger"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	logger.InitLogger()
}

func newTestEchoContext(method, path string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-Request-ID", "test-req-123")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

func TestHandleFeedError_WrappedAppContextError(t *testing.T) {
	c, rec := newTestEchoContext(http.MethodGet, "/v1/feeds")

	baseErr := errors.NewAppContextError(
		"RATE_LIMIT_ERROR",
		"rate limit exceeded",
		"gateway",
		"FeedGateway",
		"fetch",
		nil,
		nil,
	)
	wrappedErr := fmt.Errorf("usecase failed: %w", baseErr)

	enriched := BuildAppContextError(c, wrappedErr, "rest_feeds", "fetch_feeds")
	assert.Equal(t, "rest_feeds", enriched.Layer)
	assert.Equal(t, "RATE_LIMIT_ERROR", enriched.Code)
	assert.Equal(t, "fetch_feeds", enriched.Operation)
	assert.Equal(t, "RESTHandler", enriched.Component)

	err := HandleFeedError(c, wrappedErr, "fetch_feeds")
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	var resp errors.SecureHTTPResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "RATE_LIMIT_ERROR", resp.Error.Code)
}

func TestHandleFeedError_WrappedAppError(t *testing.T) {
	c, rec := newTestEchoContext(http.MethodPost, "/v1/feeds")

	baseErr := errors.ValidationError("invalid feed URL", map[string]interface{}{"field": "url"})
	wrappedErr := fmt.Errorf("feed processing error: %w", baseErr)

	enriched := BuildAppContextError(c, wrappedErr, "rest_feeds", "register_feed")
	assert.Equal(t, "rest_feeds", enriched.Layer)
	assert.Equal(t, string(errors.ErrCodeValidation), enriched.Code)
	assert.Equal(t, "register_feed", enriched.Operation)

	err := HandleFeedError(c, wrappedErr, "register_feed")
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp errors.SecureHTTPResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, string(errors.ErrCodeValidation), resp.Error.Code)
}

func TestHandleError_RestLayerLabel(t *testing.T) {
	c, rec := newTestEchoContext(http.MethodGet, "/v1/articles")

	baseErr := errors.NewAppContextError(
		"FEED_NOT_FOUND",
		"feed not found",
		"usecase",
		"FeedUsecase",
		"fetch",
		nil,
		nil,
	)
	wrappedErr := fmt.Errorf("feed usecase failed: %w", baseErr)

	enriched := BuildAppContextError(c, wrappedErr, "rest", "fetch_article")
	assert.Equal(t, "rest", enriched.Layer)
	assert.Equal(t, "FEED_NOT_FOUND", enriched.Code)

	err := HandleError(c, wrappedErr, "fetch_article")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	var resp errors.SecureHTTPResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "FEED_NOT_FOUND", resp.Error.Code)
}

func TestHandleError_UnknownError(t *testing.T) {
	c, rec := newTestEchoContext(http.MethodGet, "/v1/dashboard")

	rawErr := fmt.Errorf("unexpected database connection drop")
	enriched := BuildAppContextError(c, rawErr, "rest", "get_metrics")
	assert.Equal(t, "rest", enriched.Layer)
	assert.Equal(t, "UNKNOWN_ERROR", enriched.Code)

	err := HandleError(c, rawErr, "get_metrics")
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleValidationError(t *testing.T) {
	c, rec := newTestEchoContext(http.MethodPost, "/v1/images/fetch")

	err := HandleValidationError(c, "URL is required", "url", "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "URL is required", resp["error"])
	assert.Equal(t, "url", resp["field"])
	assert.Equal(t, "VALIDATION_ERROR", resp["code"])
}
