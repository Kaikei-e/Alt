package preprocessor_connect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A fake pre-processor that captures the X-Service-Token header of each call.
// It answers both unary (POST) and server-streaming endpoints with the
// minimum JSON/framed payload Connect expects.
func newFakePreProcessor(t *testing.T, capture *atomic.Value) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	unary := func(w http.ResponseWriter, r *http.Request) {
		capture.Store(r.Header.Get("X-Service-Token"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}
	for _, p := range []string{
		"/services.preprocessor.v2.PreProcessorService/Summarize",
		"/services.preprocessor.v2.PreProcessorService/QueueSummarize",
		"/services.preprocessor.v2.PreProcessorService/GetSummarizeStatus",
	} {
		mux.HandleFunc(p, unary)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		capture.Store(r.Header.Get("X-Service-Token"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})

	return httptest.NewServer(mux)
}

func TestConnectPreProcessorClient_SummarizeSendsServiceToken(t *testing.T) {
	var got atomic.Value
	srv := newFakePreProcessor(t, &got)
	defer srv.Close()

	c := NewConnectPreProcessorClient(srv.URL, "unit-test-secret")
	_, _ = c.Summarize(context.Background(), "c", "a", "t")

	if got.Load() != "unit-test-secret" {
		t.Fatalf("X-Service-Token missing or wrong, got %v", got.Load())
	}
}

func TestConnectPreProcessorClient_QueueSendsServiceToken(t *testing.T) {
	var got atomic.Value
	srv := newFakePreProcessor(t, &got)
	defer srv.Close()

	c := NewConnectPreProcessorClient(srv.URL, "unit-test-secret")
	_, _ = c.QueueSummarize(context.Background(), "a", "t")

	if got.Load() != "unit-test-secret" {
		t.Fatalf("X-Service-Token missing or wrong, got %v", got.Load())
	}
}

func TestConnectPreProcessorClient_GetStatusSendsServiceToken(t *testing.T) {
	var got atomic.Value
	srv := newFakePreProcessor(t, &got)
	defer srv.Close()

	c := NewConnectPreProcessorClient(srv.URL, "unit-test-secret")
	_, _ = c.GetSummarizeStatus(context.Background(), "j1")

	if got.Load() != "unit-test-secret" {
		t.Fatalf("X-Service-Token missing or wrong, got %v", got.Load())
	}
}

func TestConnectPreProcessorClient_EmptySecretOmitsHeader(t *testing.T) {
	var got atomic.Value
	srv := newFakePreProcessor(t, &got)
	defer srv.Close()

	c := NewConnectPreProcessorClient(srv.URL, "")
	_, _ = c.Summarize(context.Background(), "c", "a", "t")

	if v := got.Load(); v != nil && v.(string) != "" {
		t.Fatalf("empty secret must not attach X-Service-Token, got %q", v)
	}
}
