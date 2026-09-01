package rag_augur

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// embedServer replies to /api/embed with a fixed JSON body and HTTP 200.
func embedServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

// Under sustained load Ollama can answer /api/embed with 200, the right
// dimensions and normal usage stats while every component is zero
// (ollama/ollama#17878, open as of 2026-08-19). A zero vector has cosine 0 to
// everything, so a document indexed from one is permanently unretrievable and
// nothing in the logs says why. Accepting the response is the failure; the
// caller has to be able to fail the job instead.
func TestEncode_RejectsAllZeroVector(t *testing.T) {
	srv := embedServer(t, `{"embeddings":[[0.1,0.2,0.3],[0,0,0]]}`)
	defer srv.Close()

	var buf bytes.Buffer
	e := NewOllamaEmbedder(srv.URL, "bge-m3", 5,
		slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	got, err := e.Encode(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatalf("expected an error for an all-zero embedding, got %v", got)
	}
	if !errors.Is(err, ErrDegenerateEmbedding) {
		t.Fatalf("error must be identifiable as a degenerate embedding, got %v", err)
	}
	if got != nil {
		t.Fatalf("no vectors may be returned alongside the error, got %d", len(got))
	}

	logged := buf.String()
	if !strings.Contains(logged, "ollama_embed_degenerate_vector") {
		t.Fatalf("missing structured log: %s", logged)
	}
	if !strings.Contains(logged, `"index":1`) {
		t.Fatalf("log must name the offending index: %s", logged)
	}
	if !strings.Contains(logged, `"batch_size":2`) {
		t.Fatalf("log must name the batch size: %s", logged)
	}
}

// NaN / ±Inf components poison a pgvector column the same way. JSON cannot
// carry them literally — encoding/json rejects an out-of-range float before
// the guard sees it — so the classifier is exercised directly, which is also
// the surface any future non-JSON transport would hit.
func TestDegenerateReason_ClassifiesNonFinite(t *testing.T) {
	cases := map[string][]float32{
		"non_finite": {float32(math.Inf(1)), 0.2},
		"all_zero":   {0, 0, 0},
		"empty":      {},
	}
	for want, vec := range cases {
		if got := degenerateReason(vec); got != want {
			t.Fatalf("degenerateReason(%v) = %q, want %q", vec, got, want)
		}
	}
	if got := degenerateReason([]float32{float32(math.NaN()), 1}); got != "non_finite" {
		t.Fatalf("NaN component classified as %q", got)
	}
	if got := degenerateReason([]float32{0, 0, 0.0001}); got != "" {
		t.Fatalf("a near-zero but non-degenerate vector was rejected as %q", got)
	}
}

// An empty vector carries no information either and must not reach the store.
func TestEncode_RejectsEmptyVector(t *testing.T) {
	srv := embedServer(t, `{"embeddings":[[]]}`)
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "bge-m3", 5, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))

	if _, err := e.Encode(context.Background(), []string{"a"}); !errors.Is(err, ErrDegenerateEmbedding) {
		t.Fatalf("expected ErrDegenerateEmbedding for an empty vector, got %v", err)
	}
}

// A healthy batch must be untouched — the guard rejects poison, not signal.
func TestEncode_AcceptsHealthyVectors(t *testing.T) {
	srv := embedServer(t, `{"embeddings":[[0.1,-0.2,0.3],[0.4,0.5,-0.6]]}`)
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "bge-m3", 5, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))

	got, err := e.Encode(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(got) != 2 || len(got[0]) != 3 {
		t.Fatalf("unexpected shape: %v", got)
	}
	for _, v := range got {
		for _, c := range v {
			if math.IsNaN(float64(c)) {
				t.Fatalf("unexpected NaN in %v", v)
			}
		}
	}
}
