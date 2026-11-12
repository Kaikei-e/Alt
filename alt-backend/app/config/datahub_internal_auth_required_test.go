package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// INTERNAL_AUTH_SECRET belongs on cmd/datahub's required list because
// di/datahub hands this value to kratos_client, which puts it verbatim into
// X-Internal-Auth. An unset variable is not a disabled feature — alt-data-hub
// reports healthy, sends an empty header, and auth-hub's
// middleware/internal_auth.go answers 401 to every GetSystemUser call
// (CLAUDE.md rule 9).
//
// NewConfig's own guard only covers INTERNAL_AUTH_SECRET_FILE resolving to
// empty, so with neither variable set nothing between startup and the first
// 401 says anything at all.
func TestValidateDataHubConfig_RequiresInternalAuthSecret(t *testing.T) {
	cfg := baseConfig()
	cfg.Auth.InternalAuthSecret = ""

	err := ValidateDataHubConfig(cfg)
	if err == nil {
		t.Fatal("expected an error naming INTERNAL_AUTH_SECRET")
	}
	if !strings.Contains(err.Error(), "INTERNAL_AUTH_SECRET") {
		t.Errorf("error = %v, want it to name INTERNAL_AUTH_SECRET", err)
	}
}

func TestValidateDataHubConfig_NeitherRequiresNorReadsBackendTokenKey(t *testing.T) {
	missingVerifyKey := filepath.Join(t.TempDir(), "unmounted-verify-key.pem")
	t.Setenv("IMAGE_PROXY_ENABLED", "false")
	t.Setenv("AUTH_HUB_URL", "https://auth-hub:9443")
	t.Setenv("INTERNAL_AUTH_SECRET", "distinct-internal-auth-secret-value")
	t.Setenv("SOVEREIGN_URL", "http://knowledge-sovereign:9500")
	t.Setenv("SOVEREIGN_EVENT_AUTH", "disabled")
	t.Setenv("MQHUB_ENABLED", "false")
	t.Setenv("MQHUB_CONNECT_URL", "http://mq-hub:9500")
	t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", missingVerifyKey)

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() read or failed on BACKEND_TOKEN_VERIFY_KEY_FILE: %v", err)
	}
	if err := ValidateDataHubConfig(cfg); err != nil {
		t.Fatalf("ValidateDataHubConfig() required or read backend token key: %v", err)
	}
	if len(cfg.Auth.BackendTokenPublicKey) != 0 {
		t.Fatalf("datahub populated BackendTokenPublicKey unexpectedly")
	}
}
