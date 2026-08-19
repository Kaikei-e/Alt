package handler

import (
	"bytes"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xssPayload is a classic reflected-XSS probe. Handler-authored error
// bodies must not be text/html (or otherwise HTML-capable) that echo it raw.
const xssPayload = `<script>alert("xss")</script>`

func assertNotHTMLCapableError(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	ct := strings.ToLower(rec.Header().Get("Content-Type"))
	assert.NotContains(t, ct, "text/html")
	assert.NotContains(t, rec.Body.String(), xssPayload)
	assert.NotContains(t, rec.Body.String(), "<script>")
}

func TestHandleError_HTMLEscapesMessage(t *testing.T) {
	h := createTestBFFHandler(t, BFFConfig{EnableErrorNormalization: false})
	rec := httptest.NewRecorder()

	h.handleError(rec, http.StatusBadRequest, xssPayload, "req-1")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, strings.ToLower(rec.Header().Get("Content-Type")), "text/plain")
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assertNotHTMLCapableError(t, rec)
	assert.Contains(t, rec.Body.String(), html.EscapeString(xssPayload))
}

func TestHandleError_NormalizedPathDoesNotEchoMessage(t *testing.T) {
	h := createTestBFFHandler(t, BFFConfig{EnableErrorNormalization: true})
	rec := httptest.NewRecorder()

	h.handleError(rec, http.StatusBadRequest, xssPayload, "req-1")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assertNotHTMLCapableError(t, rec)
}

func TestBFFHandler_AuthErrorDoesNotWriteHTMLWithRequestBytes(t *testing.T) {
	h := createTestBFFHandler(t, BFFConfig{})

	req := httptest.NewRequest(http.MethodPost, "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(xssPayload)))
	req.Header.Set("X-Alt-Backend-Token", xssPayload)
	req.URL.RawQuery = "q=" + xssPayload
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assertNotHTMLCapableError(t, rec)
}

func TestAggregationHandler_ValidationErrorDoesNotReflectRequestBody(t *testing.T) {
	h := NewAggregationHandler(nil, testAggSecret, "auth-hub", "alt-backend", nil)

	queries := make([]string, MaxQueriesPerRequest+1)
	for i := range queries {
		queries[i] = xssPayload
	}
	reqBody, err := json.Marshal(AggregationRequest{Queries: queries})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/aggregate", bytes.NewReader(reqBody))
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, testAggSecret))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, strings.ToLower(rec.Header().Get("Content-Type")), "text/plain")
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assertNotHTMLCapableError(t, rec)
	assert.Contains(t, rec.Body.String(), "too many queries")
}

func TestAggregationHandler_UnknownQueryJSONIsNotHTML(t *testing.T) {
	h := NewAggregationHandler(
		func(string, string, []byte) (*AggregatedResult, error) {
			t.Fatal("unknown query must not be forwarded")
			return nil, nil
		},
		testAggSecret, "auth-hub", "alt-backend", nil,
	)

	reqBody, err := json.Marshal(AggregationRequest{Queries: []string{xssPayload}})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/aggregate", bytes.NewReader(reqBody))
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, testAggSecret))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.NotContains(t, rec.Body.String(), "<script>")

	var resp AggregationResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	result, ok := resp.Results[xssPayload]
	require.True(t, ok)
	assert.NotEmpty(t, result.Error)
}
