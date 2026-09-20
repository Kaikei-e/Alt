package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

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

// newTestAdminCmd returns a bare *cobra.Command carrying the admin flags,
// for loadOperatorToken tests that need to control flag values without
// going through Execute().
func newTestAdminCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	addAdminFlags(cmd)
	return cmd
}

func writeTokenFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	return path
}

func TestLoadOperatorToken_FromFlag(t *testing.T) {
	setupRootTest(t)
	tokenFile := writeTokenFile(t, t.TempDir(), "token.txt", "flag-token\n")

	cmd := newTestAdminCmd(t)
	if err := cmd.Flags().Set("operator-token-file", tokenFile); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	token, err := loadOperatorToken(cmd)
	if err != nil {
		t.Fatalf("loadOperatorToken() unexpected error: %v", err)
	}
	if token != "flag-token" {
		t.Errorf("token = %q, want %q", token, "flag-token")
	}
}

func TestLoadOperatorToken_FromEnv(t *testing.T) {
	setupRootTest(t)
	tokenFile := writeTokenFile(t, t.TempDir(), "token.txt", "env-token")
	t.Setenv(operatorTokenFileEnv, tokenFile)

	cmd := newTestAdminCmd(t)

	token, err := loadOperatorToken(cmd)
	if err != nil {
		t.Fatalf("loadOperatorToken() unexpected error: %v", err)
	}
	if token != "env-token" {
		t.Errorf("token = %q, want %q", token, "env-token")
	}
}

func TestLoadOperatorToken_FlagTakesPriorityOverEnv(t *testing.T) {
	setupRootTest(t)
	dir := t.TempDir()
	flagFile := writeTokenFile(t, dir, "flag.txt", "flag-token")
	envFile := writeTokenFile(t, dir, "env.txt", "env-token")
	t.Setenv(operatorTokenFileEnv, envFile)

	cmd := newTestAdminCmd(t)
	if err := cmd.Flags().Set("operator-token-file", flagFile); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	token, err := loadOperatorToken(cmd)
	if err != nil {
		t.Fatalf("loadOperatorToken() unexpected error: %v", err)
	}
	if token != "flag-token" {
		t.Errorf("token = %q, want %q (flag must win over env)", token, "flag-token")
	}
}

// TestLoadOperatorToken_DefaultMissing_ReturnsEmptyWithoutError covers the
// dev/staging shape: no flag, no env, and the project-root default file is
// not mounted because the target alt-backend runs OPERATOR_AUTH=disabled.
// altctl cannot tell that apart from "operator forgot altctl init", so it
// must not fail here — only an *explicit* path may fail.
func TestLoadOperatorToken_DefaultMissing_ReturnsEmptyWithoutError(t *testing.T) {
	setupRootTest(t) // cfg.Project.Root is a fresh, empty t.TempDir()

	cmd := newTestAdminCmd(t)

	token, err := loadOperatorToken(cmd)
	if err != nil {
		t.Fatalf("loadOperatorToken() unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty when the default file is not mounted", token)
	}
}

func TestLoadOperatorToken_DefaultPresent_ReadsProjectRootSecret(t *testing.T) {
	setupRootTest(t)
	secretsDir := filepath.Join(cfg.Project.Root, "secrets")
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	writeTokenFile(t, secretsDir, "backend_operator_token.txt", "default-token")

	cmd := newTestAdminCmd(t)

	token, err := loadOperatorToken(cmd)
	if err != nil {
		t.Fatalf("loadOperatorToken() unexpected error: %v", err)
	}
	if token != "default-token" {
		t.Errorf("token = %q, want %q", token, "default-token")
	}
}

func TestLoadOperatorToken_ExplicitFlagMissing_IsAnError(t *testing.T) {
	setupRootTest(t)

	cmd := newTestAdminCmd(t)
	missing := filepath.Join(t.TempDir(), "never-mounted.txt")
	if err := cmd.Flags().Set("operator-token-file", missing); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	if _, err := loadOperatorToken(cmd); err == nil {
		t.Fatal("loadOperatorToken() expected error for an explicit but missing token file")
	}
}
