package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHomeFlagsCommand(t *testing.T) {
	setupHomeTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetFeatureFlags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enableHomePage":      true,
			"enableTracking":      true,
			"enableProjectionV2":  false,
			"rolloutPercentage":   37,
			"enableRecallRail":    true,
			"enableLens":          true,
			"enableStreamUpdates": true,
			"enableSupersedeUx":   false,
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{
		"home", "flags",
		"--backend-url", server.URL,
	})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("home flags failed: %v", err)
		}
	})

	// Mix true/false and a non-default (37%) rollout so a mismapped field
	// (e.g. supersede_ux echoing tracking's value, or a stale hardcoded
	// 100%) would be caught rather than masked by all-true/round-number
	// fixture data.
	assertRowContains(t, out, "enable_home_page", "true")
	assertRowContains(t, out, "enable_projection_v2", "false")
	assertRowContains(t, out, "rollout_percentage", "37%")
	assertRowContains(t, out, "enable_supersede_ux", "false")
}
