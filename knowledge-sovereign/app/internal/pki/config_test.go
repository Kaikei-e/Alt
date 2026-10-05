package pki

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func unsetEnrollmentEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PKI_ENROLLMENT", "PKI_ENROLLMENT_FILE"} {
		t.Setenv(key, "placeholder-for-cleanup")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

// knowledge-sovereign presents the enrolled leaf to its mTLS peers, so an
// unset mode is a forgotten setting, never an implicit "disabled".
func TestLoadConfig_UnsetEnrollmentFails(t *testing.T) {
	unsetEnrollmentEnv(t)
	_, err := LoadConfig("knowledge-sovereign")
	if err == nil {
		t.Fatal("unset PKI_ENROLLMENT must be a startup error")
	}
	if !strings.Contains(err.Error(), "PKI_ENROLLMENT") {
		t.Fatalf("error must name PKI_ENROLLMENT: %v", err)
	}
}

func TestLoadConfig_ExplicitDisabled(t *testing.T) {
	unsetEnrollmentEnv(t)
	t.Setenv("PKI_ENROLLMENT", ModeDisabled)
	c, err := LoadConfig("knowledge-sovereign")
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeDisabled {
		t.Fatalf("mode=%q want disabled", c.Mode)
	}
	if c.Subject != "knowledge-sovereign" {
		t.Fatalf("subject=%q", c.Subject)
	}
}

func TestLoadConfig_GarbageModeFails(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", "maybe")
	if _, err := LoadConfig("knowledge-sovereign"); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfig_EnabledRejectsSharedProvisioner(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "knowledge-sovereign")
	t.Setenv("STEP_CA_PROVISIONER", "pki-agent")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/pki-agent-knowledge-sovereign-jwk")
	_, err := LoadConfig("knowledge-sovereign")
	if !errors.Is(err, ErrSharedProvisioner) {
		t.Fatalf("got %v", err)
	}
}

func TestLoadConfig_EnabledRejectsSharedRootSecret(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "tag-generator")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password")
	_, err := LoadConfig("tag-generator")
	if !errors.Is(err, ErrSharedRootSecret) {
		t.Fatalf("got %v", err)
	}
	assertNoPasswordFileInError(t, err, "/run/secrets/", "/run/secrets/step_ca_root_password")
}

func TestLoadConfig_EnabledSubjectScopedDefaults(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "knowledge-sovereign")
	c, err := LoadConfig("knowledge-sovereign")
	if err != nil {
		t.Fatal(err)
	}
	if c.Provisioner != "pki-agent-knowledge-sovereign" {
		t.Fatalf("provisioner=%q", c.Provisioner)
	}
	if got := c.PasswordFile; got != "/run/secrets/pki-agent-knowledge-sovereign-jwk" {
		t.Fatalf("password file=%q", got)
	}
}

func TestLoadConfig_DistinctSubjectsDoNotShareIdentity(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "knowledge-sovereign")
	a, err := LoadConfig("knowledge-sovereign")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CERT_SUBJECT", "auth-hub")
	b, err := LoadConfig("auth-hub")
	if err != nil {
		t.Fatal(err)
	}
	if a.Provisioner == b.Provisioner || a.PasswordFile == b.PasswordFile {
		t.Fatalf("subjects share identity a=%q/%q b=%q/%q", a.Provisioner, a.PasswordFile, b.Provisioner, b.PasswordFile)
	}
	if filepath.Base(a.PasswordFile) == filepath.Base(b.PasswordFile) {
		t.Fatal("password file basenames collided")
	}
}

func TestLoadConfig_EnrollmentFileMissingErrors(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT_FILE", filepath.Join(t.TempDir(), "missing-enrollment"))
	if _, err := LoadConfig("knowledge-sovereign"); err == nil {
		t.Fatal("explicit *_FILE must error when missing")
	}
}

func TestLoadConfig_EnrollmentFileEmptyErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty-enrollment")
	if err := os.WriteFile(p, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PKI_ENROLLMENT_FILE", p)
	if _, err := LoadConfig("knowledge-sovereign"); err == nil {
		t.Fatal("explicit *_FILE must error when empty")
	}
}

func TestLoadConfig_EnabledRejectsHTTP(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "knowledge-sovereign")
	t.Setenv("STEP_CA_URL", "http://step-ca:9000")
	_, err := LoadConfig("knowledge-sovereign")
	if !errors.Is(err, ErrNotHTTPS) {
		t.Fatalf("got %v", err)
	}
}
