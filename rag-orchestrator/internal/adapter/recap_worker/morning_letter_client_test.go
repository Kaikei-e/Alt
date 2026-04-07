package recap_worker_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"rag-orchestrator/internal/adapter/recap_worker"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchLatest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/morning/letters/latest", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "letter-001",
			"target_date": "2026-04-07",
			"body": {
				"lead": "Today's key developments...",
				"sections": [
					{"key": "top3", "title": "Top Stories", "bullets": ["Story A", "Story B"]},
					{"key": "what_changed", "title": "What Changed", "bullets": ["Change X"]}
				]
			}
		}`))
	}))
	defer server.Close()

	client := recap_worker.NewClient(server.URL, server.Client())
	doc, err := client.FetchLatest(context.Background())

	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "Today's key developments...", doc.Lead)
	assert.Len(t, doc.Sections, 2)
	assert.Equal(t, "top3", doc.Sections[0].Key)
	assert.Equal(t, "Top Stories", doc.Sections[0].Title)
	assert.Equal(t, []string{"Story A", "Story B"}, doc.Sections[0].Bullets)
}

func TestFetchLatest_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := recap_worker.NewClient(server.URL, server.Client())
	doc, err := client.FetchLatest(context.Background())

	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestFetchLatest_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := recap_worker.NewClient(server.URL, server.Client())
	_, err := client.FetchLatest(context.Background())

	require.Error(t, err)
}

func TestFetchByDate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/morning/letters/2026-04-06", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "letter-002",
			"target_date": "2026-04-06",
			"body": {
				"lead": "Yesterday's briefing",
				"sections": [{"key": "top3", "title": "Top Stories", "bullets": ["X"]}]
			}
		}`))
	}))
	defer server.Close()

	client := recap_worker.NewClient(server.URL, server.Client())
	doc, err := client.FetchByDate(context.Background(), "2026-04-06")

	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "Yesterday's briefing", doc.Lead)
}

func TestFetchByDate_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := recap_worker.NewClient(server.URL, server.Client())
	doc, err := client.FetchByDate(context.Background(), "2020-01-01")

	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestFetchLatest_NetworkError(t *testing.T) {
	client := recap_worker.NewClient("http://localhost:1", http.DefaultClient)
	_, err := client.FetchLatest(context.Background())

	require.Error(t, err)
}
