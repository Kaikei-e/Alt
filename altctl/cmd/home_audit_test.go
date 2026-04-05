package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuditCommand(t *testing.T) {
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
		"--service-token", "test-token",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("audit failed: %v", err)
	}
}
