package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRetentionRunDryRun(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/admin/retention/run" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["dry_run"] != true {
			t.Errorf("expected dry_run true, got %v", body["dry_run"])
		}
		w.Header().Set("Content-Type", "application/json")
		// Field names mirror the real sovereign retentionRunResponse
		// contract (dry_run/actions/error) documented in home_retention.go —
		// a bare {status, partitions_read, rows_exported} fixture would
		// leave the actions table permanently untested.
		json.NewEncoder(w).Encode(map[string]interface{}{
			"dry_run": true,
			"actions": []map[string]interface{}{
				{"action": "export", "table": "knowledge_events", "partition": "y2025m11", "rows": 50000, "status": "would_export"},
			},
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"home", "retention", "run", "--sovereign-url", server.URL})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("retention run failed: %v", err)
		}
	})

	if !strings.Contains(out, "Retention dry-run completed") {
		t.Errorf("expected dry-run completion message, got:\n%s", out)
	}
	assertRowContains(t, out, "export", "knowledge_events")
	assertRowContains(t, out, "y2025m11", "50000")
}

// TestRetentionRunReportsError covers the edge path documented in
// home_retention.go: the sovereign server always answers 200 OK and signals
// failure via the "error" field, not the HTTP status, so altctl must read
// it explicitly and fail the command instead of reporting success.
func TestRetentionRunReportsError(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"dry_run": false,
			"actions": []map[string]interface{}{},
			"error":   "partition y2025m11 is locked by concurrent snapshot",
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"home", "retention", "run", "--live", "--sovereign-url", server.URL})

	var out string
	var err error
	out = captureStdout(t, func() {
		err = rootCmd.Execute()
	})

	if err == nil {
		t.Fatal("expected retention run to fail when the response carries an error field, got nil")
	}
	if !strings.Contains(err.Error(), "partition y2025m11 is locked") {
		t.Errorf("expected returned error to include server error detail, got: %v", err)
	}
	if !strings.Contains(out, "Retention failed") {
		t.Errorf("expected rendered output to report the failure, got:\n%s", out)
	}
}

func TestRetentionStatusCommand(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/retention/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"logs": []map[string]interface{}{
				{"action": "export", "target_table": "knowledge_events", "target_partition": "y2025m10", "rows_affected": 40000, "dry_run": false, "status": "completed", "run_at": "2026-07-01T00:00:00Z"},
			},
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"home", "retention", "status", "--sovereign-url", server.URL})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("retention status failed: %v", err)
		}
	})

	assertRowContains(t, out, "knowledge_events", "y2025m10")
	assertRowContains(t, out, "y2025m10", "40000")
	assertRowContains(t, out, "40000", "completed")
}

func TestRetentionStatusCommand_Empty(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"logs": []map[string]interface{}{}})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"home", "retention", "status", "--sovereign-url", server.URL})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("retention status failed: %v", err)
		}
	})

	if !strings.Contains(out, "No retention runs found") {
		t.Errorf("expected empty-log message, got:\n%s", out)
	}
}

func TestRetentionEligibleCommand(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/retention/eligible" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"partitions": []map[string]interface{}{
				{"table_name": "knowledge_events", "partition_name": "y2025m11", "row_count": 50000, "size_bytes": 5 * 1024 * 1024},
			},
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"home", "retention", "eligible", "--sovereign-url", server.URL})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("retention eligible failed: %v", err)
		}
	})

	assertRowContains(t, out, "y2025m11", "50000")
	// size_bytes above the 1 MiB threshold must render in MB, not raw bytes.
	assertRowContains(t, out, "y2025m11", "5.0 MB")
}
