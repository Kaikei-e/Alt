package tlsutil

import (
	"crypto/tls"
	"strings"
	"testing"
)

// ClientAuthLabel must produce an operator-readable name for every
// ClientAuthType so the startup log can state the effective client-auth mode.
// The insecure default (NoClientCert) must be labelled distinctly so it is
// obvious in the log that the mTLS listener is NOT verifying client certs.
func TestClientAuthLabel(t *testing.T) {
	cases := map[tls.ClientAuthType]string{
		tls.NoClientCert:               "no_client_cert",
		tls.RequestClientCert:          "request_client_cert",
		tls.RequireAnyClientCert:       "require_any_client_cert",
		tls.VerifyClientCertIfGiven:    "verify_client_cert_if_given",
		tls.RequireAndVerifyClientCert: "require_and_verify_client_cert",
	}
	for mode, want := range cases {
		if got := ClientAuthLabel(mode); got != want {
			t.Errorf("ClientAuthLabel(%d) = %q, want %q", mode, got, want)
		}
	}
}

// A mode requiring verification must report ClientAuthEnforced()==true; the
// permissive defaults must report false, which the startup log uses to decide
// between an info line and a loud warning.
func TestClientAuthEnforced(t *testing.T) {
	if ClientAuthEnforced(tls.NoClientCert) {
		t.Error("NoClientCert must not be reported as enforced")
	}
	if ClientAuthEnforced(tls.VerifyClientCertIfGiven) {
		t.Error("VerifyClientCertIfGiven must not be reported as enforced (cert optional)")
	}
	if !ClientAuthEnforced(tls.RequireAndVerifyClientCert) {
		t.Error("RequireAndVerifyClientCert must be reported as enforced")
	}
	// Unknown value must still yield a non-empty label (no panic).
	if got := ClientAuthLabel(tls.ClientAuthType(99)); !strings.HasPrefix(got, "unknown") {
		t.Errorf("unknown mode label = %q, want unknown-prefixed", got)
	}
}
