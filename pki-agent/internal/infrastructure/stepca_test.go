package infrastructure

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"pki-agent/internal/domain"
)

// fakeStepBin writes a shell script to dir that mimics the minimal step CLI
// surface needed by StepCACLI. The first arg selects the subcommand:
//
//	step ca token <cn> ...     -> prints a fake OTT to stdout
//	step ca certificate <cn> <certOut> <keyOut> ... -> writes fixed cert/key bytes
//
// If the STEPFAIL env var is set, every call exits 2 with a canned error.
func fakeStepBin(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "step")
	script := `#!/usr/bin/env sh
set -e
if [ -n "$STEPFAIL" ]; then
  echo "fake step error" >&2
  exit 2
fi
case "$2" in
  token)
    echo "FAKE.OTT.TOKEN"
    ;;
  certificate)
    # args: step ca certificate <cn> <certOut> <keyOut> --ca-url ... --root ... --token ... --force
    cert_out="$4"
    key_out="$5"
    printf -- "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n" > "$cert_out"
    printf -- "-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----\n" > "$key_out"
    ;;
  *)
    echo "unknown: $*" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStepCACLI_Issue(t *testing.T) {
	dir := t.TempDir()
	bin := fakeStepBin(t, dir)
	// password file must exist.
	pw := filepath.Join(dir, "pw.txt")
	if err := os.WriteFile(pw, []byte("hunter2"), 0o400); err != nil {
		t.Fatal(err)
	}
	rootCA := filepath.Join(dir, "root.pem")
	if err := os.WriteFile(rootCA, []byte("-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----\n"), 0o444); err != nil {
		t.Fatal(err)
	}

	s := &StepCACLI{
		CAURL: "https://step-ca:9000", RootFile: rootCA,
		Provisioner: "pki-agent", PasswordFile: pw,
		StepBinary: bin,
	}
	cert, key, err := s.Issue(context.Background(), "alt-backend", []string{"alt-backend", "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cert) == 0 || len(key) == 0 {
		t.Fatalf("empty pem bytes")
	}
}

func TestStepCACLI_Issue_TokenFailure(t *testing.T) {
	dir := t.TempDir()
	bin := fakeStepBin(t, dir)
	pw := filepath.Join(dir, "pw.txt")
	_ = os.WriteFile(pw, []byte("x"), 0o400)

	s := &StepCACLI{StepBinary: bin, PasswordFile: pw, CAURL: "u", RootFile: pw, Provisioner: "p"}
	t.Setenv("STEPFAIL", "1")
	_, _, err := s.Issue(context.Background(), "alt-backend", nil)
	if !errors.Is(err, domain.ErrTokenSign) {
		t.Fatalf("want ErrTokenSign, got %v", err)
	}
}
