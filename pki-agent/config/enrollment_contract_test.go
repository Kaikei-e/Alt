package config

import (
	"strings"
	"testing"
)

// Wave 4 enrollment contract: each CERT_SUBJECT mints via its own JWK
// provisioner and password file. Today's default (STEP_CA_PROVISIONER=pki-agent
// + the shared step_ca_root_password secret) is the F-001 blast radius from
// ADR-000747 — a stolen sidecar can mint a leaf for any allowlisted CN.

func TestLoad_ProvisionerNameIsSubjectScoped(t *testing.T) {
	t.Setenv("CERT_SUBJECT", "alt-backend")
	// Do not set STEP_CA_PROVISIONER: the default is what production compose
	// currently ships (pki-agent for every sidecar).

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	want := "pki-agent-alt-backend"
	if c.Provisioner != want {
		t.Fatalf("STEP_CA_PROVISIONER=%q, Wave 4 subject-scoped enrollment requires %q (shared JWK provisioner is F-001 blast radius)", c.Provisioner, want)
	}
	if !strings.Contains(c.Provisioner, "alt-backend") {
		t.Fatalf("provisioner %q does not include CERT_SUBJECT alt-backend", c.Provisioner)
	}
}

func TestLoad_ProvisionerPasswordIsNotSharedRootSecret(t *testing.T) {
	t.Setenv("CERT_SUBJECT", "tag-generator")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(c.PasswordFile, "step_ca_root_password") {
		t.Fatalf("provisioner password file %q is the shared root secret; Wave 4 requires a per-subject JWK password file containing the subject (e.g. pki-agent-tag-generator-jwk)", c.PasswordFile)
	}
	if !strings.Contains(c.PasswordFile, "tag-generator") {
		t.Fatalf("provisioner password file %q is not scoped to CERT_SUBJECT tag-generator", c.PasswordFile)
	}
}

func TestLoad_DistinctSubjectsDoNotShareProvisionerIdentity(t *testing.T) {
	t.Setenv("CERT_SUBJECT", "news-creator")
	a, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("CERT_SUBJECT", "recap-subworker")
	b, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if a.Provisioner == b.Provisioner {
		t.Fatalf("distinct subjects share provisioner %q; Wave 4 requires one JWK per CERT_SUBJECT", a.Provisioner)
	}
	if a.PasswordFile == b.PasswordFile {
		t.Fatalf("distinct subjects share provisioner password file %q; Wave 4 requires per-subject secrets", a.PasswordFile)
	}
}
