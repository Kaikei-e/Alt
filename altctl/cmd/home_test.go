package cmd

import "testing"

// TestAddAdminFlags_DefaultBackendURL guards against the admin API default
// port drifting from the real alt-backend Connect-RPC admin port. compose/core.yaml
// publishes "9101:9101" and sets CONNECT_PORT=9101 for alt-backend; a stale
// default here makes every `altctl home` subcommand fail to connect out of
// the box (it silently hits nothing on 9001).
func TestAddAdminFlags_DefaultBackendURL(t *testing.T) {
	cmd := homeFlagsCmd
	flag := cmd.Flags().Lookup("backend-url")
	if flag == nil {
		t.Fatal("backend-url flag not registered")
	}

	const want = "http://localhost:9101"
	if flag.DefValue != want {
		t.Errorf("backend-url default = %q, want %q (alt-backend Connect-RPC admin port per compose/core.yaml)", flag.DefValue, want)
	}
}
