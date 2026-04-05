package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHomeHealthCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"activeVersion": 2,
			"checkpointSeq": 1139408,
			"lastUpdated":   "2026-03-25T10:00:00Z",
			"backfillJobs": []map[string]interface{}{
				{"jobId": "bf-1", "status": "completed", "projectionVersion": 2, "totalEvents": 786000, "processedEvents": 786000},
			},
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{
		"home", "health",
		"--backend-url", server.URL,
		"--service-token", "test-token",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("home health failed: %v", err)
	}
}
