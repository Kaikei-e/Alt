package httpclient

import (
	"os"
	"testing"
	"time"
)

func TestMTLSEnforced_FalseByDefault(t *testing.T) {
	t.Setenv("MTLS_ENFORCE", "")
	if MTLSEnforced() {
		t.Fatal("MTLSEnforced should be false when env unset")
	}
}

func TestMTLSEnforced_TrueWhenEnvTrue(t *testing.T) {
	t.Setenv("MTLS_ENFORCE", "true")
	if !MTLSEnforced() {
		t.Fatal("MTLSEnforced should be true when env is true")
	}
}

func TestNewMTLSClient_NonMTLSReturnsPlain(t *testing.T) {
	t.Setenv("MTLS_ENFORCE", "")
	c, err := NewMTLSClient(5 * time.Second)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c.Transport != nil {
		t.Fatal("non-mTLS path should not set Transport")
	}
	if c.Timeout != 5*time.Second {
		t.Fatalf("timeout mismatch: got %v want 5s", c.Timeout)
	}
}

func TestNewMTLSClient_MissingEnvFailsClosed(t *testing.T) {
	t.Setenv("MTLS_ENFORCE", "true")
	t.Setenv("MTLS_CERT_FILE", "")
	t.Setenv("MTLS_KEY_FILE", "")
	t.Setenv("MTLS_CA_FILE", "")
	if _, err := NewMTLSClient(time.Second); err == nil {
		t.Fatal("expected error when MTLS_* paths not set")
	}
}

func TestNewMTLSClient_BadCertPathFailsClosed(t *testing.T) {
	tmp, err := os.CreateTemp("", "rag-mtls-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	t.Setenv("MTLS_ENFORCE", "true")
	t.Setenv("MTLS_CERT_FILE", "/nonexistent/cert.pem")
	t.Setenv("MTLS_KEY_FILE", tmp.Name())
	t.Setenv("MTLS_CA_FILE", tmp.Name())
	if _, err := NewMTLSClient(time.Second); err == nil {
		t.Fatal("expected error for missing cert file")
	}
}

func TestPreflightMTLS_NoOpWhenDisabled(t *testing.T) {
	t.Setenv("MTLS_ENFORCE", "")
	if err := PreflightMTLS(); err != nil {
		t.Fatalf("PreflightMTLS should be no-op when disabled: %v", err)
	}
}
