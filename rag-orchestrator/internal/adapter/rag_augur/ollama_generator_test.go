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
		fmt.Fprintln(w, `{"message":{"content":""},"done":false}`)
		fmt.Fprintln(w, `{"message":{"content":"{\"answer\":\"hi\""},"done":false}`)
		fmt.Fprintln(w, `{"message":{"content":"}"},"done":true}`)
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
