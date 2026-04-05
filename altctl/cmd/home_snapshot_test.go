package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSnapshotListCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/admin/snapshots/list" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"snapshots": []map[string]interface{}{
				{"snapshot_id": "snap-1", "status": "valid", "items_row_count": 122297},
			},
		})
	}))
	defer server.Close()

	cmd := rootCmd
	cmd.SetArgs([]string{"home", "snapshot", "list", "--sovereign-url", server.URL})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("snapshot list failed: %v", err)
	}
}

func TestSnapshotLatestCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/snapshots/latest" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"snapshot_id": "snap-latest",
			"status":      "valid",
		})
	}))
	defer server.Close()

	cmd := rootCmd
	cmd.SetArgs([]string{"home", "snapshot", "latest", "--sovereign-url", server.URL})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("snapshot latest failed: %v", err)
	}
}

func TestSnapshotCreateCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/admin/snapshots/create" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"snapshot_id":    "snap-new",
			"status":         "valid",
			"items_row_count": 122300,
		})
	}))
	defer server.Close()

	cmd := rootCmd
	cmd.SetArgs([]string{"home", "snapshot", "create", "--sovereign-url", server.URL})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("snapshot create failed: %v", err)
	}
}
