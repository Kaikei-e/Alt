package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBackfillTriggerCommand(t *testing.T) {
	setupHomeTest(t)

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
		"--projection-version", "2",
	})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("backfill trigger failed: %v", err)
		}
	})

	if !strings.Contains(out, "Backfill triggered") {
		t.Errorf("expected trigger success message, got:\n%s", out)
	}
	assertRowContains(t, out, "Job ID", "job-1")
	assertRowContains(t, out, "Status", "pending")
	assertRowContains(t, out, "Projection Version", "2")
}

func TestBackfillStatusCommand(t *testing.T) {
	setupHomeTest(t)

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
		"--job-id", "job-1",
	})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("backfill status failed: %v", err)
		}
	})

	assertRowContains(t, out, "Job ID", "job-1")
	assertRowContains(t, out, "Total Events", "1000")
	assertRowContains(t, out, "Processed Events", "500")
	assertRowContains(t, out, "Progress", "50.0%")
}

func TestBackfillStatusCommand_RequiresJobID(t *testing.T) {
	setupHomeTest(t)
	// backfillStatusCmd is a package-level *cobra.Command reused across
	// tests, so its FlagSet persists whatever a previous test set; without
	// this reset TestBackfillStatusCommand's --job-id=job-1 would leak in
	// and this test would exercise the network path instead of the
	// required-flag guard.
	backfillStatusCmd.Flags().Set("job-id", "")

	rootCmd.SetArgs([]string{"home", "backfill", "status"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --job-id is missing, got nil")
	}
	if !strings.Contains(err.Error(), "job-id") {
		t.Errorf("expected error to mention 'job-id', got: %v", err)
	}
}

// TestBackfillPauseCommand and TestBackfillResumeCommand cover the pause
// and resume subcommands, which previously had no test coverage at all
// despite performing the same admin RPC + required-flag pattern as
// trigger/status.
func TestBackfillPauseCommand(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alt.knowledge_home.v1.KnowledgeHomeAdminService/PauseBackfill" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if body["jobId"] != "job-1" {
			t.Errorf("expected jobId job-1, got %v", body["jobId"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{
		"home", "backfill", "pause",
		"--backend-url", server.URL,
		"--job-id", "job-1",
	})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("backfill pause failed: %v", err)
		}
	})

	if !strings.Contains(out, "Backfill paused: job-1") {
		t.Errorf("expected pause confirmation to name the job, got:\n%s", out)
	}
}

func TestBackfillPauseCommand_RequiresJobID(t *testing.T) {
	setupHomeTest(t)
	backfillPauseCmd.Flags().Set("job-id", "")

	rootCmd.SetArgs([]string{"home", "backfill", "pause"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --job-id is missing, got nil")
	}
	if !strings.Contains(err.Error(), "job-id") {
		t.Errorf("expected error to mention 'job-id', got: %v", err)
	}
}

func TestBackfillResumeCommand(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alt.knowledge_home.v1.KnowledgeHomeAdminService/ResumeBackfill" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if body["jobId"] != "job-1" {
			t.Errorf("expected jobId job-1, got %v", body["jobId"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{
		"home", "backfill", "resume",
		"--backend-url", server.URL,
		"--job-id", "job-1",
	})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("backfill resume failed: %v", err)
		}
	})

	if !strings.Contains(out, "Backfill resumed: job-1") {
		t.Errorf("expected resume confirmation to name the job, got:\n%s", out)
	}
}

func TestBackfillResumeCommand_RequiresJobID(t *testing.T) {
	setupHomeTest(t)
	backfillResumeCmd.Flags().Set("job-id", "")

	rootCmd.SetArgs([]string{"home", "backfill", "resume"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --job-id is missing, got nil")
	}
	if !strings.Contains(err.Error(), "job-id") {
		t.Errorf("expected error to mention 'job-id', got: %v", err)
	}
}
