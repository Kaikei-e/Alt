package preprocessor_client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"alt/shared/domain/authcontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Summarize_PropagatesJWTHeader(t *testing.T) {
	var receivedToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("X-Alt-Backend-Token")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":    true,
			"summary":    "test summary",
			"article_id": "article-1",
		})
	}))
	defer server.Close()

	client := NewClientWithTransport(server.URL, server.Client().Transport)
	ctx := authcontext.WithJWT(context.Background(), "my-user-jwt")
	summary, err := client.Summarize(ctx, "", "article-1", "Title")
	require.NoError(t, err)
	assert.Equal(t, "test summary", summary)
	assert.Equal(t, "my-user-jwt", receivedToken)
}

func TestClient_Summarize_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/summarize", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "article-1", body["article_id"])

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":    true,
			"summary":    "a short summary",
			"article_id": "article-1",
		})
	}))
	defer server.Close()

	client := NewClientWithTransport(server.URL, server.Client().Transport)
	summary, err := client.Summarize(context.Background(), "", "article-1", "Title")

	require.NoError(t, err)
	assert.Equal(t, "a short summary", summary)
}

func TestClient_Summarize_MissingArticleID(t *testing.T) {
	client := NewClientWithTransport("https://unused", http.DefaultTransport)
	_, err := client.Summarize(context.Background(), "content", "", "title")
	require.Error(t, err)
}

func TestClient_NewClient_RequiresHTTPS(t *testing.T) {
	_, err := NewClient("http://pre-processor:9443")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must use https:// scheme")
}

func TestClient_NewClient_FailsMissingCertEnv(t *testing.T) {
	t.Setenv("MTLS_CERT_FILE", "")
	t.Setenv("MTLS_KEY_FILE", "")
	t.Setenv("MTLS_CA_FILE", "")

	_, err := NewClient("https://pre-processor:9443")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing MTLS cert/key/ca")
}

func TestClient_Summarize_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	client := NewClientWithTransport(server.URL, server.Client().Transport)
	_, err := client.Summarize(context.Background(), "", "article-1", "title")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClient_Summarize_UnsuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false})
	}))
	defer server.Close()

	client := NewClientWithTransport(server.URL, server.Client().Transport)
	_, err := client.Summarize(context.Background(), "", "article-1", "title")

	require.Error(t, err)
}

func TestClient_StreamSummarize_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/summarize/stream", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: chunk\n\n"))
	}))
	defer server.Close()

	client := NewClientWithTransport(server.URL, server.Client().Transport)
	stream, err := client.StreamSummarize(context.Background(), "content", "article-1", "title")
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	buf := make([]byte, 64)
	n, _ := stream.Read(buf)
	assert.Contains(t, string(buf[:n]), "chunk")
}

func TestClient_StreamSummarize_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream down"))
	}))
	defer server.Close()

	client := NewClientWithTransport(server.URL, server.Client().Transport)
	_, err := client.StreamSummarize(context.Background(), "content", "article-1", "title")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")
}

func TestClient_QueueSummarize_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/summarize/queue", r.URL.Path)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"job_id": "job-123",
			"status": "pending",
		})
	}))
	defer server.Close()

	client := NewClientWithTransport(server.URL, server.Client().Transport)
	jobID, err := client.QueueSummarize(context.Background(), "article-1", "title")

	require.NoError(t, err)
	assert.Equal(t, "job-123", jobID)
}

func TestClient_GetSummarizeStatus_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/summarize/status/job-123", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"job_id":     "job-123",
			"status":     "completed",
			"summary":    "done",
			"article_id": "article-1",
		})
	}))
	defer server.Close()

	client := NewClientWithTransport(server.URL, server.Client().Transport)
	status, err := client.GetSummarizeStatus(context.Background(), "job-123")

	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, "completed", status.Status)
	assert.Equal(t, "done", status.Summary)
}

func TestClient_GetSummarizeStatus_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClientWithTransport(server.URL, server.Client().Transport)
	status, err := client.GetSummarizeStatus(context.Background(), "missing-job")

	require.NoError(t, err)
	assert.Nil(t, status)
}
