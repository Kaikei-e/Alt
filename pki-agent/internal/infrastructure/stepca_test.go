package infrastructure

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
  echo "fake step error: $*" >&2
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

func TestStepCACLI_Issue_TokenFailure_DoesNotLogPasswordFile(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prev := slog.Default()
	slog.SetDefault(log)
	t.Cleanup(func() { slog.SetDefault(prev) })

	dir := t.TempDir()
	bin := fakeStepBin(t, dir)
	pw := filepath.Join(dir, "pki-agent-alt-backend-jwk")
	if err := os.WriteFile(pw, []byte("hunter2"), 0o400); err != nil {
		t.Fatal(err)
	}

	s := &StepCACLI{StepBinary: bin, PasswordFile: pw, CAURL: "u", RootFile: pw, Provisioner: "p"}
	t.Setenv("STEPFAIL", "1")
	_, _, err := s.Issue(context.Background(), "alt-backend", nil)
	if !errors.Is(err, domain.ErrTokenSign) {
		t.Fatalf("want ErrTokenSign, got %v", err)
	}
	logs := buf.String()
	if !strings.Contains(logs, "step_cli_failed") {
		t.Fatalf("expected step_cli_failed, got %s", logs)
	}
	for _, leakStr := range []string{pw, "--password-file", "hunter2", `"stderr"`} {
		if strings.Contains(logs, leakStr) {
			t.Fatalf("password file leaked in step-cli logs: %q in %s", leakStr, logs)
		}
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

func TestStepCACLI_Issue_RemovesKeyWhenTempDirSet(t *testing.T) {
	dir := t.TempDir()
	bin := fakeStepBin(t, dir)
	pw := filepath.Join(dir, "pw.txt")
	if err := os.WriteFile(pw, []byte("hunter2"), 0o400); err != nil {
		t.Fatal(err)
	}
	rootCA := filepath.Join(dir, "root.pem")
	if err := os.WriteFile(rootCA, []byte("-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	issueDir := filepath.Join(dir, "issue")
	if err := os.Mkdir(issueDir, 0o700); err != nil {
		t.Fatal(err)
	}

	s := &StepCACLI{
		CAURL: "https://step-ca:9000", RootFile: rootCA,
		Provisioner: "pki-agent", PasswordFile: pw,
		StepBinary: bin, TempDir: issueDir,
	}
	_, _, err := s.Issue(context.Background(), "alt-backend", []string{"alt-backend"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(issueDir, "svc-key.pem")); !os.IsNotExist(err) {
		t.Fatalf("private key file should be removed after Issue, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(issueDir, "svc-cert.pem")); !os.IsNotExist(err) {
		t.Fatalf("cert tmp file should be removed after Issue, stat err=%v", err)
	}
}

func TestStepCACLI_Issue_RejectsFlagInjectionInSubjectOrSAN(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "invoked")
	bin := filepath.Join(dir, "step")
	script := "#!/usr/bin/env sh\ntouch " + marker + "\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	s := &StepCACLI{
		CAURL: "https://step-ca:9000", RootFile: "/root.pem",
		Provisioner: "pki-agent", PasswordFile: "/pw.txt",
		StepBinary: bin,
	}

	cases := []struct {
		name    string
		subject string
		sans    []string
	}{
		{name: "subject_flag", subject: "--ott-file=/etc/shadow", sans: []string{"alt-backend"}},
		{name: "subject_short_flag", subject: "-f", sans: []string{"alt-backend"}},
		{name: "subject_empty", subject: "", sans: []string{"alt-backend"}},
		{name: "san_flag", subject: "alt-backend", sans: []string{"alt-backend", "--ca-url=https://evil"}},
		{name: "san_short_flag", subject: "alt-backend", sans: []string{"-x"}},
		{name: "san_empty", subject: "alt-backend", sans: []string{""}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := s.Issue(context.Background(), tc.subject, tc.sans)
			if err == nil {
				t.Fatalf("expected error for subject=%q sans=%v, got nil", tc.subject, tc.sans)
			}
			if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
				t.Fatalf("step binary must not be executed when flag injection is detected")
			}
		})
	}
}

func TestStepCACLI_Issue_RemovesTempFilesOnReadFailure(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "step")
	// Script writes cert_out, omits key_out (causing os.ReadFile(keyTmp) to fail).
	script := `#!/usr/bin/env sh
set -e
case "$2" in
  token)
    echo "FAKE.OTT.TOKEN"
    ;;
  certificate)
    cert_out="$4"
    printf -- "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n" > "$cert_out"
    ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	issueDir := filepath.Join(dir, "issue")
	if err := os.Mkdir(issueDir, 0o700); err != nil {
		t.Fatal(err)
	}

	s := &StepCACLI{
		CAURL: "https://step-ca:9000", RootFile: "/root.pem",
		Provisioner: "pki-agent", PasswordFile: "/pw.txt",
		StepBinary: bin, TempDir: issueDir,
	}
	_, _, err := s.Issue(context.Background(), "alt-backend", []string{"alt-backend"})
	if err == nil {
		t.Fatal("expected error when key file is missing")
	}
	if _, statErr := os.Stat(filepath.Join(issueDir, "svc-cert.pem")); !os.IsNotExist(statErr) {
		t.Fatalf("svc-cert.pem should be cleaned up via defer even when reading keyTmp fails, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(issueDir, "svc-key.pem")); !os.IsNotExist(statErr) {
		t.Fatalf("svc-key.pem should not remain in TempDir, stat err=%v", statErr)
	}
}
