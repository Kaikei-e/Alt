package config

import (
	"testing"
	"time"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CERT_SUBJECT", "test-subject")
}

func TestLoad_ProxyResponseHeaderTimeoutDefault(t *testing.T) {
	setRequiredEnv(t)

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.ProxyResponseHeaderTimeout != 15*time.Second {
		t.Fatalf("ProxyResponseHeaderTimeout=%v want 15s", c.ProxyResponseHeaderTimeout)
	}
}

// Services whose upstream legitimately thinks for minutes (LLM inference)
// need the wait raised past the default without loosening it fleet-wide.
func TestLoad_ProxyResponseHeaderTimeoutOverride(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PROXY_RESPONSE_HEADER_TIMEOUT", "960s")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.ProxyResponseHeaderTimeout != 960*time.Second {
		t.Fatalf("ProxyResponseHeaderTimeout=%v want 960s", c.ProxyResponseHeaderTimeout)
	}
}

func TestLoad_ProxyResponseHeaderTimeoutRejectsGarbage(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PROXY_RESPONSE_HEADER_TIMEOUT", "soon")

	if _, err := Load(); err == nil {
		t.Fatal("expected an unparseable PROXY_RESPONSE_HEADER_TIMEOUT to fail startup, got nil error")
	}
}
