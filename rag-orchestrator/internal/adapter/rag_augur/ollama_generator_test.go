package rag_augur

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildOptions_GemmaModel(t *testing.T) {
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	gen := NewOllamaGenerator("http://localhost:11434", "gemma3-12b-rag", 100, testLogger)
	opts := gen.buildOptions(4096)

	if opts["temperature"] != 0.7 {
		t.Fatalf("expected temperature 0.7, got %v", opts["temperature"])
	}
	if opts["top_p"] != 0.85 {
		t.Fatalf("expected top_p 0.85, got %v", opts["top_p"])
	}
	if opts["top_k"] != 40 {
		t.Fatalf("expected top_k 40, got %v", opts["top_k"])
	}
	if opts["num_ctx"] != 8192 {
		t.Fatalf("expected num_ctx 8192, got %v", opts["num_ctx"])
	}
	if opts["repeat_penalty"] != 1.15 {
		t.Fatalf("expected repeat_penalty 1.15, got %v", opts["repeat_penalty"])
	}
	if opts["num_predict"] != 4096 {
		t.Fatalf("expected num_predict 4096, got %v", opts["num_predict"])
	}
}

func TestBuildOptions_SwallowModel(t *testing.T) {
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	gen := NewOllamaGenerator("http://localhost:11434", "swallow-8b-rag", 100, testLogger)
	opts := gen.buildOptions(4096)

	if opts["temperature"] != 0.6 {
		t.Fatalf("expected temperature 0.6, got %v", opts["temperature"])
	}
	if opts["num_ctx"] != 16384 {
		t.Fatalf("expected num_ctx 16384, got %v", opts["num_ctx"])
	}
}

func TestGetThinkParam_GemmaReturnsNil(t *testing.T) {
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	gen := NewOllamaGenerator("http://localhost:11434", "gemma3-12b-rag", 100, testLogger)
	result := gen.getThinkParam(4096)

	if result != nil {
		t.Fatalf("expected nil for gemma model, got %v", result)
	}
}

func TestGetThinkParam_SwallowReturnsNil(t *testing.T) {
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	gen := NewOllamaGenerator("http://localhost:11434", "swallow-8b-rag", 100, testLogger)
	result := gen.getThinkParam(4096)

	if result != nil {
		t.Fatalf("expected nil for swallow model, got %v", result)
	}
}

func TestGetThinkParam_Qwen3ReturnsFalse(t *testing.T) {
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	gen := NewOllamaGenerator("http://localhost:11434", "qwen3-8b", 100, testLogger)
	result := gen.getThinkParam(4096)

	if result != false {
		t.Fatalf("expected false for qwen3 model, got %v", result)
	}
}

func TestOllamaGeneratorGenerate_StreamAggregatesContent(t *testing.T) {
	var streamFlag bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		streamValue, ok := req["stream"].(bool)
		if !ok {
			t.Fatalf("expected stream flag in request")
		}
		streamFlag = streamValue

		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintln(w, `{"message":{"content":""},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"message":{"content":"{\"answer\":\"hi\""},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"message":{"content":"}"},"done":true}`)
	}))
	defer server.Close()

	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	gen := NewOllamaGenerator(server.URL, "test-model", 100, testLogger)
	resp, err := gen.Generate(context.Background(), "prompt", 1000)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !streamFlag {
		t.Fatalf("expected stream=true in request")
	}
	if resp == nil {
		t.Fatal("expected response")
	}
	if !resp.Done {
		t.Fatalf("expected done=true, got false")
	}
	if resp.Text != `{"answer":"hi"}` {
		t.Fatalf("unexpected response text: %q", resp.Text)
	}
}
