package sovereignclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:9511")
	if c.BaseURL != "http://localhost:9511" {
		t.Errorf("expected BaseURL http://localhost:9511, got %s", c.BaseURL)
	}
	if c.HTTPClient == nil {
		t.Fatal("expected HTTPClient to be non-nil")
	}
}

func TestGet_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/admin/snapshots/list" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"snapshots": []map[string]string{
				{"snapshot_id": "snap-1", "status": "valid"},
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)

	var resp struct {
		Snapshots []struct {
			SnapshotID string `json:"snapshot_id"`
			Status     string `json:"status"`
		} `json:"snapshots"`
	}
	err := c.Get(context.Background(), "/admin/snapshots/list", &resp)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(resp.Snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(resp.Snapshots))
	}
	if resp.Snapshots[0].SnapshotID != "snap-1" {
		t.Errorf("expected snapshot_id snap-1, got %s", resp.Snapshots[0].SnapshotID)
	}
}

func TestPost_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		if r.URL.Path != "/admin/retention/run" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if reqBody["dry_run"] != true {
			t.Errorf("expected dry_run true, got %v", reqBody["dry_run"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "completed",
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)

	reqBody := map[string]interface{}{"dry_run": true}
	var resp map[string]string
	err := c.Post(context.Background(), "/admin/retention/run", reqBody, &resp)
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}
	if resp["status"] != "completed" {
		t.Errorf("expected status completed, got %s", resp["status"])
	}
}

func TestGet_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "database unavailable"}`))
	}))
	defer server.Close()

	c := NewClient(server.URL)

	var resp map[string]interface{}
	err := c.Get(context.Background(), "/admin/storage/stats", &resp)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", apiErr.StatusCode)
	}
}

func TestGet_ConnectionError(t *testing.T) {
	c := NewClient("http://127.0.0.1:1")
	c.HTTPClient.Timeout = 1 * time.Second

	var resp map[string]interface{}
	err := c.Get(context.Background(), "/admin/storage/stats", &resp)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}
