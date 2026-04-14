package summarization

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"alt/utils/logger"

	"github.com/stretchr/testify/require"
)

func init() {
	logger.InitLogger()
}

// Each helper must forward the shared service secret as X-Service-Token so
// pre-processor's RequireServiceAuth middleware admits the request.

func TestCallPreProcessorSummarize_SendsServiceToken(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Service-Token")
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"summary":"s","article_id":"a"}`))
	}))
	defer srv.Close()

	_, err := callPreProcessorSummarize(context.Background(), "c", "a", "t", srv.URL, "the-secret")
	require.NoError(t, err)
	require.Equal(t, "the-secret", got, "X-Service-Token header must match configured service secret")
}

func TestStreamPreProcessorSummarize_SendsServiceToken(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Service-Token")
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"token":"x"}`))
	}))
	defer srv.Close()

	body, err := streamPreProcessorSummarize(context.Background(), "c", "a", "t", srv.URL, "the-secret")
	require.NoError(t, err)
	defer body.Close()
	require.Equal(t, "the-secret", got)
}

func TestCallPreProcessorSummarizeQueue_SendsServiceToken(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Service-Token")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "job_id": "j1"})
	}))
	defer srv.Close()

	jobID, err := callPreProcessorSummarizeQueue(context.Background(), "a", "t", srv.URL, "the-secret")
	require.NoError(t, err)
	require.Equal(t, "j1", jobID)
	require.Equal(t, "the-secret", got)
}

func TestCallPreProcessorSummarizeStatus_SendsServiceToken(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Service-Token")
		_ = json.NewEncoder(w).Encode(SummarizeStatus{JobID: "j1", Status: "completed"})
	}))
	defer srv.Close()

	st, err := callPreProcessorSummarizeStatus(context.Background(), "j1", srv.URL, "the-secret")
	require.NoError(t, err)
	require.NotNil(t, st)
	require.Equal(t, "the-secret", got)
}

func TestCallPreProcessorSummarize_EmptySecretOmitsHeader(t *testing.T) {
	// Empty secret must NOT send a blank X-Service-Token header — doing so would
	// trip pre-processor's "empty token" path and produce misleading 401 logs.
	// We allow the call to proceed header-less so tests and legacy paths keep
	// working; production misses are caught by config validation.
	var present bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header[http.CanonicalHeaderKey("X-Service-Token")]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"summary":"s","article_id":"a"}`))
	}))
	defer srv.Close()

	_, err := callPreProcessorSummarize(context.Background(), "c", "a", "t", srv.URL, "")
	require.NoError(t, err)
	require.False(t, present, "empty secret must not attach an X-Service-Token header")
	_ = strings.TrimSpace
}
