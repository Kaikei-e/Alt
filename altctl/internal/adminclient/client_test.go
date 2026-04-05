package adminclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:9001", "test-token")
	if c.BaseURL != "http://localhost:9001" {
		t.Errorf("expected BaseURL http://localhost:9001, got %s", c.BaseURL)
	}
	if c.ServiceToken != "test-token" {
		t.Errorf("expected ServiceToken test-token, got %s", c.ServiceToken)
	}
	if c.HTTPClient == nil {
		t.Fatal("expected HTTPClient to be non-nil")
	}
}

func TestCall_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method and content type
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		if st := r.Header.Get("X-Service-Token"); st != "my-secret" {
			t.Errorf("expected X-Service-Token my-secret, got %s", st)
		}
		if r.URL.Path != "/alt.knowledge_home.v1.KnowledgeHomeAdminService/StartReproject" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Decode request body
		var reqBody map[string]string
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if reqBody["mode"] != "dry_run" {
			t.Errorf("expected mode dry_run, got %s", reqBody["mode"])
		}

		// Write response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"runId":  "abc-123",
			"status": "started",
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "my-secret")

	req := map[string]string{"mode": "dry_run"}
	var resp map[string]string
	err := c.Call(context.Background(), "StartReproject", req, &resp)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if resp["runId"] != "abc-123" {
		t.Errorf("expected runId abc-123, got %s", resp["runId"])
	}
	if resp["status"] != "started" {
		t.Errorf("expected status started, got %s", resp["status"])
	}
}

func TestCall_ServiceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "invalid_argument",
			"message": "missing required field: mode",
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "token")

	req := map[string]string{}
	var resp map[string]string
	err := c.Call(context.Background(), "StartReproject", req, &resp)
	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
	if _, ok := err.(*APIError); !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	apiErr := err.(*APIError)
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
}

func TestCall_ConnectionError(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "token")
	c.HTTPClient.Timeout = 1 * time.Second

	req := map[string]string{}
	var resp map[string]string
	err := c.Call(context.Background(), "StartReproject", req, &resp)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

func TestCall_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	c := NewClient(server.URL, "token")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req := map[string]string{}
	var resp map[string]string
	err := c.Call(ctx, "StartReproject", req, &resp)
	if err == nil {
		t.Fatal("expected context canceled error, got nil")
	}
}

func TestCall_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if len(reqBody) != 0 {
			t.Errorf("expected empty body, got %v", reqBody)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	c := NewClient(server.URL, "token")

	req := map[string]interface{}{}
	var resp map[string]string
	err := c.Call(context.Background(), "GetSLOStatus", req, &resp)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
}
