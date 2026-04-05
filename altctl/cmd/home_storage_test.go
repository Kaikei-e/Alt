package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStorageCommand(t *testing.T) {
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
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("storage failed: %v", err)
	}
}
