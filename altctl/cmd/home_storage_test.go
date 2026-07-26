package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStorageCommand(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/storage/stats" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tables": []map[string]interface{}{
				{"name": "knowledge_events", "total_size": "1.1 GB", "table_size": "760 MB", "index_size": "340 MB", "row_count": 786652},
				{"name": "knowledge_home_items", "total_size": "50 MB", "table_size": "40 MB", "index_size": "10 MB", "row_count": 122297},
			},
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"home", "storage", "--sovereign-url", server.URL})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("storage failed: %v", err)
		}
	})

	// Every table row from the response must show up with its own size
	// figures and row count, not just the first row or a stale placeholder.
	assertRowContains(t, out, "knowledge_events", "1.1 GB")
	assertRowContains(t, out, "knowledge_events", "786652")
	assertRowContains(t, out, "knowledge_home_items", "50 MB")
	assertRowContains(t, out, "knowledge_home_items", "122297")
}

func TestStorageCommand_NoTables(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"tables": []map[string]interface{}{}})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"home", "storage", "--sovereign-url", server.URL})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("storage failed: %v", err)
		}
	})

	if !strings.Contains(out, "No tables found") {
		t.Errorf("expected empty-tables message, got:\n%s", out)
	}
}
