package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHomeHealthCommand(t *testing.T) {
	setupHomeTest(t)

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
	})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("home health failed: %v", err)
		}
	})

	// Assert the response is actually rendered, not just fetched without
	// error: active version, checkpoint sequence, last-updated timestamp,
	// and the backfill job's progress must all reach the terminal.
	assertRowContains(t, out, "Active Version", "2")
	assertRowContains(t, out, "Checkpoint Seq", "1139408")
	assertRowContains(t, out, "Last Updated", "2026-03-25T10:00:00Z")
	assertRowContains(t, out, "bf-1", "completed")
	if !strings.Contains(out, "786000/786000") {
		t.Errorf("expected health output to show backfill progress 786000/786000, got:\n%s", out)
	}
}
