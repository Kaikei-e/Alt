package cmd

import "testing"

// TestAddAdminFlags_DefaultBackendURL guards against the admin API default
// port drifting from the real alt-backend admin listener. The admin
// Connect-RPC services moved off the browser-facing :9101 onto the internal
// listener; compose/core.yaml sets INTERNAL_PORT=9102 and publishes it on
// 127.0.0.1 only. A stale default here makes every `altctl home` subcommand
// fail to connect out of the box.
func TestAddAdminFlags_DefaultBackendURL(t *testing.T) {
	cmd := homeFlagsCmd
	flag := cmd.Flags().Lookup("backend-url")
	if flag == nil {
		t.Fatal("backend-url flag not registered")
	}

	const want = "http://localhost:9102"
	if flag.DefValue != want {
		t.Errorf("backend-url default = %q, want %q (alt-backend internal listener per compose/core.yaml INTERNAL_PORT)", flag.DefValue, want)
	}
}
