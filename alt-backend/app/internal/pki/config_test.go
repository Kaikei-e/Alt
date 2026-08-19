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
	c, err := LoadConfig("alt-backend")
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeDisabled {
		t.Fatalf("mode=%q want disabled", c.Mode)
	}
	if c.Subject != "alt-backend" {
		t.Fatalf("subject=%q", c.Subject)
	}
}

func TestLoadConfig_GarbageModeFails(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", "maybe")
	if _, err := LoadConfig("alt-backend"); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfig_EnabledRejectsSharedProvisioner(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "alt-backend")
	t.Setenv("STEP_CA_PROVISIONER", "pki-agent")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/pki-agent-alt-backend-jwk")
	_, err := LoadConfig("alt-backend")
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
	t.Setenv("CERT_SUBJECT", "alt-harvester")
	c, err := LoadConfig("alt-harvester")
	if err != nil {
		t.Fatal(err)
	}
	if c.Provisioner != "pki-agent-alt-harvester" {
		t.Fatalf("provisioner=%q", c.Provisioner)
	}
	if got := c.PasswordFile; got != "/run/secrets/pki-agent-alt-harvester-jwk" {
		t.Fatalf("password file=%q", got)
	}
}

func TestLoadConfig_DistinctSubjectsDoNotShareIdentity(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "alt-backend")
	a, err := LoadConfig("alt-backend")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CERT_SUBJECT", "alt-data-hub")
	b, err := LoadConfig("alt-data-hub")
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
