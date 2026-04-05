package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHomeFlagsCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetFeatureFlags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enableHomePage":     true,
			"enableTracking":     true,
			"enableProjectionV2": false,
			"rolloutPercentage":  100,
			"enableRecallRail":   true,
			"enableLens":         true,
			"enableStreamUpdates": true,
			"enableSupersedeUx":  false,
		})
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{
		"home", "flags",
		"--backend-url", server.URL,
		"--service-token", "test-token",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("home flags failed: %v", err)
	}
}
