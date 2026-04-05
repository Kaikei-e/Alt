package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRetentionRunDryRun(t *testing.T) {
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
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "completed", "dry_run": true, "partitions_read": 2, "rows_exported": 0,
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"home", "retention", "run", "--sovereign-url", server.URL})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("retention run failed: %v", err)
	}
}

func TestRetentionStatusCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/retention/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"logs": []map[string]interface{}{
				{"action": "export", "target_table": "knowledge_events", "status": "completed"},
			},
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"home", "retention", "status", "--sovereign-url", server.URL})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("retention status failed: %v", err)
	}
}

func TestRetentionEligibleCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/retention/eligible" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"partitions": []map[string]interface{}{
				{"table_name": "knowledge_events", "partition_name": "y2025m11", "row_count": 50000},
			},
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"home", "retention", "eligible", "--sovereign-url", server.URL})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("retention eligible failed: %v", err)
	}
}
