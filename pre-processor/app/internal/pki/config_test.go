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
	c, err := LoadConfig("pre-processor")
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeDisabled {
		t.Fatalf("mode=%q want disabled", c.Mode)
	}
	if c.Subject != "pre-processor" {
		t.Fatalf("subject=%q", c.Subject)
	}
}

func TestLoadConfig_GarbageModeFails(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", "maybe")
	if _, err := LoadConfig("pre-processor"); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfig_EnabledRejectsSharedProvisioner(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "pre-processor")
	t.Setenv("STEP_CA_PROVISIONER", "pki-agent")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/pki-agent-pre-processor-jwk")
	_, err := LoadConfig("pre-processor")
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
	t.Setenv("CERT_SUBJECT", "pre-processor")
	c, err := LoadConfig("pre-processor")
	if err != nil {
		t.Fatal(err)
	}
	if c.Provisioner != "pki-agent-pre-processor" {
		t.Fatalf("provisioner=%q", c.Provisioner)
	}
	if got := c.PasswordFile; got != "/run/secrets/pki-agent-pre-processor-jwk" {
		t.Fatalf("password file=%q", got)
	}
}

func TestLoadConfig_DistinctSubjectsDoNotShareIdentity(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "pre-processor")
	a, err := LoadConfig("pre-processor")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CERT_SUBJECT", "rag-orchestrator")
	b, err := LoadConfig("rag-orchestrator")
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
