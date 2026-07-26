package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSnapshotListCommand(t *testing.T) {
	setupHomeTest(t)

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
				{"snapshot_id": "snap-1", "status": "valid", "projection_version": 2, "event_seq_boundary": 1139408, "items_row_count": 122297, "snapshot_at": "2026-07-20T00:00:00Z"},
			},
		})
	}))
	defer server.Close()

	cmd := rootCmd
	cmd.SetArgs([]string{"home", "snapshot", "list", "--sovereign-url", server.URL})

	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("snapshot list failed: %v", err)
		}
	})

	assertRowContains(t, out, "snap-1", "valid")
	assertRowContains(t, out, "snap-1", "122297")
	assertRowContains(t, out, "snap-1", "2026-07-20T00:00:00Z")
}

func TestSnapshotListCommand_Empty(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"snapshots": []map[string]interface{}{}})
	}))
	defer server.Close()

	cmd := rootCmd
	cmd.SetArgs([]string{"home", "snapshot", "list", "--sovereign-url", server.URL})

	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("snapshot list failed: %v", err)
		}
	})

	if !strings.Contains(out, "No snapshots found") {
		t.Errorf("expected empty-snapshots message, got:\n%s", out)
	}
}

func TestSnapshotLatestCommand(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/snapshots/latest" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"snapshot_id":        "snap-latest",
			"status":             "valid",
			"projection_version": 2,
			"items_row_count":    122297,
			"digest_row_count":   3400,
			"recall_row_count":   980,
		})
	}))
	defer server.Close()

	cmd := rootCmd
	cmd.SetArgs([]string{"home", "snapshot", "latest", "--sovereign-url", server.URL})

	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("snapshot latest failed: %v", err)
		}
	})

	assertRowContains(t, out, "Snapshot ID", "snap-latest")
	assertRowContains(t, out, "Items Rows", "122297")
	assertRowContains(t, out, "Digest Rows", "3400")
	assertRowContains(t, out, "Recall Rows", "980")
}

func TestSnapshotCreateCommand(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/admin/snapshots/create" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"snapshot_id":     "snap-new",
			"status":          "valid",
			"items_row_count": 122300,
		})
	}))
	defer server.Close()

	cmd := rootCmd
	cmd.SetArgs([]string{"home", "snapshot", "create", "--sovereign-url", server.URL})

	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("snapshot create failed: %v", err)
		}
	})

	if !strings.Contains(out, "Snapshot created") {
		t.Errorf("expected create success message, got:\n%s", out)
	}
	assertRowContains(t, out, "Snapshot ID", "snap-new")
	assertRowContains(t, out, "Items Rows", "122300")
}
