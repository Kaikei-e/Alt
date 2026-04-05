package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBackfillTriggerCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alt.knowledge_home.v1.KnowledgeHomeAdminService/TriggerBackfill" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"job": map[string]interface{}{
				"jobId": "job-1", "status": "pending", "projectionVersion": 2,
			},
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{
		"home", "backfill", "trigger",
		"--backend-url", server.URL,
		"--service-token", "test-token",
		"--projection-version", "2",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("backfill trigger failed: %v", err)
	}
}

func TestBackfillStatusCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetBackfillStatus" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"job": map[string]interface{}{
				"jobId": "job-1", "status": "running", "totalEvents": 1000, "processedEvents": 500,
			},
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{
		"home", "backfill", "status",
		"--backend-url", server.URL,
		"--service-token", "test-token",
		"--job-id", "job-1",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("backfill status failed: %v", err)
	}
}
