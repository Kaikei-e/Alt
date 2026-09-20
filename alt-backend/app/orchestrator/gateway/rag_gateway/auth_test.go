package rag_gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuthRequestEditor(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		wantHeader string
	}{
		{
			name:       "non-empty token sets Authorization header",
			token:      "my-secret-token",
			wantHeader: "Bearer my-secret-token",
		},
		{
			name:       "empty token sets no Authorization header",
			token:      "",
			wantHeader: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := BearerAuthRequestEditor(tt.token)
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
			if err != nil {
				t.Fatalf("unexpected error creating request: %v", err)
			}

			if err := fn(context.Background(), req); err != nil {
				t.Fatalf("unexpected error from request editor: %v", err)
			}

			got := req.Header.Get("Authorization")
			if got != tt.wantHeader {
				t.Errorf("Authorization header = %q, want %q", got, tt.wantHeader)
			}
		})
	}
}

func TestWithBearerToken(t *testing.T) {
	opt := WithBearerToken("test-token-12345678901234")
	client := &Client{}
	if err := opt(client); err != nil {
		t.Fatalf("unexpected error applying WithBearerToken: %v", err)
	}
	if len(client.RequestEditors) != 1 {
		t.Fatalf("expected 1 RequestEditor, got %d", len(client.RequestEditors))
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error creating request: %v", err)
	}
	if err := client.RequestEditors[0](context.Background(), req); err != nil {
		t.Fatalf("unexpected error executing RequestEditor: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-token-12345678901234" {
		t.Errorf("Authorization header = %q, want %q", got, "Bearer test-token-12345678901234")
	}
}

func TestClient_BearerTokenRoundTrip(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"indexed"}`))
	}))
	defer srv.Close()

	client, err := NewClientWithResponses(srv.URL, WithBearerToken("test-bearer-token-val"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	resp, err := client.UpsertIndexWithResponse(context.Background(), UpsertIndexJSONRequestBody{
		ArticleId: "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01",
		UserId:    "11111111-2222-3333-4444-555555555555",
	})
	if err != nil {
		t.Fatalf("unexpected error from UpsertIndex: %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode())
	}
	if receivedAuth != "Bearer test-bearer-token-val" {
		t.Errorf("received auth = %q, want %q", receivedAuth, "Bearer test-bearer-token-val")
	}
}
