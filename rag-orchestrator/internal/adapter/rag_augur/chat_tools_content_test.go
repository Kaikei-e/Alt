package rag_augur

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"rag-orchestrator/internal/domain"
)

// news-creator validates the chat payload with a schema that makes `content`
// required on every message. An assistant turn that only carries tool_calls has
// no text, and `omitempty` used to drop the field entirely — the request came
// back 422 (`body.messages.2.content Field required`) and the agentic loop
// degraded to non-agentic retrieval on every request.
func TestToChatMessages_AlwaysSerializesContentField(t *testing.T) {
	msgs := []domain.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "", ToolCalls: []domain.ToolCall{
			{Function: domain.ToolCallFunction{Name: "tag_search"}},
		}},
		{Role: "tool", Name: "tag_search", ToolCallID: "tag_search|", Content: "result"},
	}

	payload, err := json.Marshal(chatRequest{Model: "m", Messages: toChatMessages(msgs)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Messages) != len(msgs) {
		t.Fatalf("expected %d messages, got %d", len(msgs), len(decoded.Messages))
	}
	for i, m := range decoded.Messages {
		if _, ok := m["content"]; !ok {
			t.Fatalf("message %d omitted the content field: %v", i, m)
		}
	}
	if got := decoded.Messages[2]["content"]; got != "" {
		t.Fatalf("tool-call assistant message content = %v, want empty string", got)
	}
}

// The same guarantee has to hold on the wire, not just in a unit-level
// marshal: ChatWithTools is the call site news-creator rejected.
func TestChatWithTools_SendsEmptyContentForToolCallTurn(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"ok","tool_calls":[]},"done":true}`))
	}))
	defer srv.Close()

	gen := NewOllamaGenerator(srv.URL, "gemma4-e4b-rag", 10, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	_, err := gen.ChatWithTools(context.Background(), []domain.Message{
		{Role: "assistant", Content: "", ToolCalls: []domain.ToolCall{
			{Function: domain.ToolCallFunction{Name: "tag_search"}},
		}},
	}, []domain.ToolDefinition{{Type: "function"}}, 256)
	if err != nil {
		t.Fatalf("ChatWithTools: %v", err)
	}

	var decoded struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if _, ok := decoded.Messages[0]["content"]; !ok {
		t.Fatalf("assistant tool_calls message omitted content: %s", body)
	}
}
