package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuditCommand(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alt.knowledge_home.v1.KnowledgeHomeAdminService/RunProjectionAudit" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"audit": map[string]interface{}{
				"auditId":           "audit-1",
				"projectionName":    "knowledge_home_items",
				"projectionVersion": "1",
				"sampleSize":        100,
				"mismatchCount":     0,
			},
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{
		"home", "audit",
		"--backend-url", server.URL,
	})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("audit failed: %v", err)
		}
	})

	assertRowContains(t, out, "Audit ID", "audit-1")
	assertRowContains(t, out, "Sample Size", "100")
	assertRowContains(t, out, "Mismatches", "0")
	if !strings.Contains(out, "No mismatches detected") {
		t.Errorf("expected zero-mismatch audit to report success, got:\n%s", out)
	}
}

// TestAuditCommand_Mismatches covers the audit-failed edge path: when the
// server reports mismatches, altctl must both render the count and warn
// the operator, not silently report success like the zero-mismatch case.
func TestAuditCommand_Mismatches(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"audit": map[string]interface{}{
				"auditId":           "audit-2",
				"projectionName":    "knowledge_home_items",
				"projectionVersion": "1",
				"sampleSize":        100,
				"mismatchCount":     3,
			},
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{
		"home", "audit",
		"--backend-url", server.URL,
	})

	out := captureStdout(t, func() {
		// A nonzero mismatch count is a reportable audit outcome, not a
		// command failure: it must still exit 0 so it can be scripted.
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("audit failed: %v", err)
		}
	})

	assertRowContains(t, out, "Mismatches", "3")
	if !strings.Contains(out, "Found 3 mismatches") {
		t.Errorf("expected mismatch warning, got:\n%s", out)
	}
}
