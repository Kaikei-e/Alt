package config

import (
	"strings"
	"testing"
)

// INTERNAL_AUTH_SECRET belongs on cmd/datahub's required list for the same
// reason BACKEND_TOKEN_SECRET does, and the failure it prevents is worse:
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
