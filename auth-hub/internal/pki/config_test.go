package pki

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_DefaultDisabled(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", "placeholder-for-cleanup")
	if err := os.Unsetenv("PKI_ENROLLMENT"); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("PKI_ENROLLMENT_FILE"); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig("auth-hub")
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeDisabled {
		t.Fatalf("mode=%q want disabled", c.Mode)
	}
	if c.Subject != "auth-hub" {
		t.Fatalf("subject=%q", c.Subject)
	}
}

func TestLoadConfig_GarbageModeFails(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", "maybe")
	if _, err := LoadConfig("auth-hub"); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfig_EnabledRejectsSharedProvisioner(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "auth-hub")
	t.Setenv("STEP_CA_PROVISIONER", "pki-agent")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/pki-agent-auth-hub-jwk")
	_, err := LoadConfig("auth-hub")
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
}

func TestLoadConfig_EnabledSubjectScopedDefaults(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "auth-hub")
	c, err := LoadConfig("auth-hub")
	if err != nil {
		t.Fatal(err)
	}
	if c.Provisioner != "pki-agent-auth-hub" {
		t.Fatalf("provisioner=%q", c.Provisioner)
	}
	if got := c.PasswordFile; got != "/run/secrets/pki-agent-auth-hub-jwk" {
		t.Fatalf("password file=%q", got)
	}
}

func TestLoadConfig_DistinctSubjectsDoNotShareIdentity(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "auth-hub")
	a, err := LoadConfig("auth-hub")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CERT_SUBJECT", "search-indexer")
	b, err := LoadConfig("search-indexer")
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

func TestLoadConfig_UnsetEnrollmentDisabledMigrationCompat(t *testing.T) {
	// Image-first mixed-mode: unset PKI_ENROLLMENT stays disabled until the
	// final compose cutover sets PKI_ENROLLMENT=enabled explicitly.
	t.Setenv("PKI_ENROLLMENT", "placeholder")
	if err := os.Unsetenv("PKI_ENROLLMENT"); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("PKI_ENROLLMENT_FILE"); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig("auth-hub")
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeDisabled {
		t.Fatalf("unset PKI_ENROLLMENT must stay disabled for migration compatibility, got %q", c.Mode)
	}
}

func TestLoadConfig_EnrollmentFileMissingErrors(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT_FILE", filepath.Join(t.TempDir(), "missing-enrollment"))
	if _, err := LoadConfig("auth-hub"); err == nil {
		t.Fatal("explicit *_FILE must error when missing")
	}
}

func TestLoadConfig_EnrollmentFileEmptyErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty-enrollment")
	if err := os.WriteFile(p, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PKI_ENROLLMENT_FILE", p)
	if _, err := LoadConfig("auth-hub"); err == nil {
		t.Fatal("explicit *_FILE must error when empty")
	}
}

func TestLoadConfig_EnabledRejectsHTTP(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "auth-hub")
	t.Setenv("STEP_CA_URL", "http://step-ca:9000")
	_, err := LoadConfig("auth-hub")
	if !errors.Is(err, ErrNotHTTPS) {
		t.Fatalf("got %v", err)
	}
}
