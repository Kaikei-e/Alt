package pki

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func unsetenv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "placeholder-for-cleanup")
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfig_UnsetEnrollmentDefaultsDisabled(t *testing.T) {
	unsetenv(t, "PKI_ENROLLMENT")
	unsetenv(t, "PKI_ENROLLMENT_FILE")
	c, err := LoadConfig("alt-backend")
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeDisabled {
		t.Fatalf("unset PKI_ENROLLMENT must stay disabled for image-first rollout, got %q", c.Mode)
	}
}

func TestLoadConfig_EmptyEnrollmentFails(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", "")
	if _, err := LoadConfig("alt-backend"); err == nil {
		t.Fatal("empty PKI_ENROLLMENT must fail, not default disabled")
	}
}

func TestLoadConfig_FileMissingFails(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT_FILE", filepath.Join(t.TempDir(), "missing"))
	if _, err := LoadConfig("alt-backend"); err == nil {
		t.Fatal("missing PKI_ENROLLMENT_FILE must fail")
	}
}

func TestLoadConfig_FileEmptyFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(path, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PKI_ENROLLMENT_FILE", path)
	if _, err := LoadConfig("alt-backend"); err == nil {
		t.Fatal("empty PKI_ENROLLMENT_FILE must fail")
	}
}

func TestLoadConfig_FileEmptyPathFails(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT_FILE", "")
	if _, err := LoadConfig("alt-backend"); err == nil {
		t.Fatal("empty PKI_ENROLLMENT_FILE path must fail")
	}
}

func TestLoadConfig_RejectsHTTPAndSchemelessCAURL(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "alt-backend")
	for _, raw := range []string{"http://step-ca:9000", "step-ca:9000", "https://"} {
		t.Setenv("STEP_CA_URL", raw)
		_, err := LoadConfig("alt-backend")
		if !errors.Is(err, ErrInsecureCAURL) {
			t.Errorf("STEP_CA_URL=%q: got %v, want ErrInsecureCAURL", raw, err)
		}
	}
}

func TestLoadConfig_ExactProvisionerAndPasswordBasename(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "rag")
	t.Setenv("STEP_CA_PROVISIONER", "pki-agent-rag-orchestrator")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/pki-agent-rag-orchestrator-jwk")
	_, err := LoadConfig("rag")
	if err == nil {
		t.Fatal("substring-scoped provisioner/password must be rejected")
	}
	if errors.Is(err, ErrSharedProvisioner) {
		t.Fatalf("wrong sentinel: %v", err)
	}
}

func TestLoadConfig_RejectsWrongPasswordBasename(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "alt-backend")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", filepath.Join(t.TempDir(), "wrong-name"))
	if _, err := LoadConfig("alt-backend"); err == nil {
		t.Fatal("password basename must be exactly pki-agent-<subject>-jwk")
	}
}

func TestLoadConfig_TempDirAllowedWhenBasenameMatches(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "alt-backend")
	path := filepath.Join(t.TempDir(), "pki-agent-alt-backend-jwk")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", path)
	c, err := LoadConfig("alt-backend")
	if err != nil {
		t.Fatal(err)
	}
	if c.PasswordFile != path {
		t.Fatalf("password file=%q", c.PasswordFile)
	}
}

func TestNativeStepCAIssuer_RejectsAllRedirects(t *testing.T) {
	password := []byte("redirect-pw")
	for _, code := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			ca := newFakeStepCA(t, "pki-agent-alt-backend", password)
			ca.redirectSign = code
			iss := newNativeIssuer(t, ca)
			_, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"})
			if !errors.Is(err, ErrRedirect) {
				t.Fatalf("got %v, want ErrRedirect", err)
			}
		})
	}
}

func TestNativeStepCAIssuer_HTTPClientRequiresHTTPS(t *testing.T) {
	iss := &NativeStepCAIssuer{
		CAURL:        "http://127.0.0.1:1",
		RootFile:     filepath.Join(t.TempDir(), "missing.pem"),
		Provisioner:  "pki-agent-alt-backend",
		PasswordFile: filepath.Join(t.TempDir(), "pki-agent-alt-backend-jwk"),
	}
	_, err := iss.httpClient(context.Background(), nil)
	if !errors.Is(err, ErrInsecureCAURL) {
		t.Fatalf("got %v, want ErrInsecureCAURL", err)
	}
}

func TestNativeStepCAIssuer_RejectsTLS12OnlyCA(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("tls12-pw"))
	srv := httptest.NewUnstartedServer(http.HandlerFunc(ca.serveHTTP))
	srv.TLS = &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{ca.caCert.Raw},
			PrivateKey:  ca.caKey,
		}},
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	rootFile := filepath.Join(t.TempDir(), "root.pem")
	if err := os.WriteFile(rootFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.caCert.Raw}), 0o444); err != nil {
		t.Fatal(err)
	}
	iss := &NativeStepCAIssuer{
		CAURL:        srv.URL,
		RootFile:     rootFile,
		Provisioner:  ca.provisioner,
		PasswordFile: writePasswordFile(t, t.TempDir(), "pki-agent-alt-backend-jwk", string(ca.password), 0o400),
		Timeout:      2 * time.Second,
	}
	_, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"})
	if err == nil {
		t.Fatal("TLS 1.2-only CA must be rejected under repo TLS 1.3 policy")
	}
}

func TestNativeStepCAIssuer_CAErrorIsSentinelWithoutBody(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("sentinel-pw"))
	secret := "super-secret-jwk-password-and-ott"
	ca.signStatus = http.StatusUnauthorized
	ca.signBody = []byte(`{"status":401,"message":"` + secret + `"}`)
	iss := newNativeIssuer(t, ca)
	_, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"})
	if !errors.Is(err, ErrCARejected) {
		t.Fatalf("got %v, want ErrCARejected", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("CA body leaked: %v", err)
	}
}

func TestNativeStepCAIssuer_ProvisionerPageCap(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("pages-pw"))
	ca.endlessProvisioners = true
	iss := newNativeIssuer(t, ca)
	_, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"})
	if !errors.Is(err, ErrProvisionerPageLimit) {
		t.Fatalf("got %v, want ErrProvisionerPageLimit", err)
	}
}

func TestNativeStepCAIssuer_ResponseSizeCap(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("huge-pw"))
	ca.signStatus = http.StatusCreated
	ca.signBody = bytesRepeat('A', maxResponseBytes+8)
	iss := newNativeIssuer(t, ca)
	_, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("got %v, want ErrResponseTooLarge", err)
	}
}

func TestNativeStepCAIssuer_PasswordSymlinkRejected(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("symlink-pw"))
	url, root := ca.start(t)
	dir := t.TempDir()
	real := writePasswordFile(t, dir, "pki-agent-alt-backend-jwk", string(ca.password), 0o400)
	link := filepath.Join(dir, "link-jwk")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	iss := &NativeStepCAIssuer{
		CAURL: url, RootFile: root,
		Provisioner:  "pki-agent-alt-backend",
		PasswordFile: link,
		Timeout:      time.Second,
	}
	if _, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"}); err == nil {
		t.Fatal("symlink password file must be rejected")
	}
}

func TestNativeStepCAIssuer_PasswordSizeCap(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("cap-pw"))
	url, root := ca.start(t)
	path := writePasswordFile(t, t.TempDir(), "pki-agent-alt-backend-jwk", strings.Repeat("x", maxPasswordBytes+1), 0o400)
	iss := &NativeStepCAIssuer{
		CAURL: url, RootFile: root,
		Provisioner:  "pki-agent-alt-backend",
		PasswordFile: path,
		Timeout:      time.Second,
	}
	_, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"})
	if !errors.Is(err, ErrPasswordTooLarge) {
		t.Fatalf("got %v, want ErrPasswordTooLarge", err)
	}
}

func TestNativeStepCAIssuer_RejectsExtraDNS(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("extra-dns"))
	ca.mutateLeaf = func(c *x509.Certificate) {
		c.DNSNames = append(c.DNSNames, "evil.example")
	}
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"}); err == nil {
		t.Fatal("extra DNS SAN must be rejected")
	}
}

func TestNativeStepCAIssuer_ExactIPAndURISANs(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("typed-san"))
	iss := newNativeIssuer(t, ca)
	sans := []string{"127.0.0.1", "spiffe://alt.local/alt-backend"}
	certPEM, keyPEM, err := iss.Issue(context.Background(), "alt-backend", sans)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}
	leaf, err := parseCertPEM(string(certPEM))
	if err != nil {
		t.Fatal(err)
	}
	if len(leaf.DNSNames) != 0 {
		t.Fatalf("unexpected DNS SANs %q", leaf.DNSNames)
	}
	if len(leaf.IPAddresses) != 1 || !leaf.IPAddresses[0].Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("IP SANs = %v", leaf.IPAddresses)
	}
	if len(leaf.URIs) != 1 || leaf.URIs[0].String() != "spiffe://alt.local/alt-backend" {
		t.Fatalf("URI SANs = %v", leaf.URIs)
	}
}

func TestNativeStepCAIssuer_RejectsMismatchedLeafKey(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("key-mismatch"))
	iss := newNativeIssuer(t, ca)
	certPEM, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = iss.validateAndEncode("alt-backend", []string{"alt-backend"}, &signResponseJSON{Crt: string(certPEM)}, other)
	if err == nil {
		t.Fatal("leaf public key must equal CSR/private key")
	}
}

func TestNativeStepCAIssuer_RejectsServerAuthOnlyEKU(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("eku-or"))
	ca.mutateLeaf = func(c *x509.Certificate) {
		c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"}); err == nil {
		t.Fatal("leaf with only serverAuth EKU must be rejected")
	}
}

func TestCertFile_RejectsSymlinkDestAndParent(t *testing.T) {
	nb := time.Now()
	cert, key := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(time.Hour))

	t.Run("dest_symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target.pem")
		if err := os.WriteFile(target, []byte("nope"), 0o644); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(dir, "svc-cert.pem")
		if err := os.Symlink(target, dest); err != nil {
			t.Fatal(err)
		}
		cf := &CertFile{CertPath: dest, KeyPath: filepath.Join(dir, "svc-key.pem")}
		if err := cf.Write(context.Background(), cert, key); err == nil {
			t.Fatal("symlink dest must be rejected")
		}
	})

	t.Run("parent_symlink", func(t *testing.T) {
		root := t.TempDir()
		realDir := filepath.Join(root, "real")
		if err := os.Mkdir(realDir, 0o750); err != nil {
			t.Fatal(err)
		}
		linkDir := filepath.Join(root, "link")
		if err := os.Symlink(realDir, linkDir); err != nil {
			t.Fatal(err)
		}
		cf := &CertFile{
			CertPath: filepath.Join(linkDir, "svc-cert.pem"),
			KeyPath:  filepath.Join(linkDir, "svc-key.pem"),
		}
		if err := cf.Write(context.Background(), cert, key); err == nil {
			t.Fatal("symlink parent dir must be rejected")
		}
	})
}

func TestHandle_StopWaitsForInFlightRun(t *testing.T) {
	dir := t.TempDir()
	inner := &fakeIssuer{notBefore: time.Now().Add(-time.Minute), lifetime: 24 * time.Hour}
	sticky := &stickyIssuer{inner: inner}
	cfg := &Config{
		Mode:            ModeEnabled,
		Subject:         "alt-backend",
		SANs:            []string{"alt-backend"},
		CertPath:        filepath.Join(dir, "svc-cert.pem"),
		KeyPath:         filepath.Join(dir, "svc-key.pem"),
		Provisioner:     "pki-agent-alt-backend",
		PasswordFile:    "/run/secrets/pki-agent-alt-backend-jwk",
		RenewAtFraction: 0.66,
		TickInterval:    15 * time.Millisecond,
		RetryAttempts:   1,
		RetryBackoff:    time.Millisecond,
	}
	ctx := withT(context.Background(), t)
	h, err := StartWith(ctx, slogDiscard(), cfg, sticky)
	if err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	sticky.gate.Store(&gate)
	if err := os.Remove(cfg.CertPath); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for sticky.entered.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sticky.entered.Load() == 0 {
		t.Fatal("Run did not enter in-flight Issue")
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(80 * time.Millisecond)
		close(gate)
		close(released)
	}()
	start := time.Now()
	h.Stop()
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("Stop returned before in-flight Issue finished")
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("release helper stuck")
	}
}

func TestStartWithRegisterer_DoesNotUseDefaultRegisterer(t *testing.T) {
	dir := t.TempDir()
	nb := time.Now().Add(-time.Minute)
	cfg := &Config{
		Mode:            ModeEnabled,
		Subject:         "alt-backend-private-metrics",
		SANs:            []string{"alt-backend-private-metrics"},
		CertPath:        filepath.Join(dir, "svc-cert.pem"),
		KeyPath:         filepath.Join(dir, "svc-key.pem"),
		Provisioner:     "pki-agent-alt-backend-private-metrics",
		PasswordFile:    "/run/secrets/pki-agent-alt-backend-private-metrics-jwk",
		RenewAtFraction: 0.66,
		TickInterval:    time.Hour,
		RetryAttempts:   1,
		RetryBackoff:    time.Millisecond,
	}
	reg := prometheus.NewPedanticRegistry()
	ctx := withT(context.Background(), t)
	h, err := StartWithObserver(ctx, slogDiscard(), cfg, &fakeIssuer{notBefore: nb, lifetime: 24 * time.Hour}, NewPromObserver(cfg.Subject, reg))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Stop)
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if strings.Contains(mf.GetName(), "pki_enrollment") && strings.Contains(mf.String(), cfg.Subject) {
			t.Fatalf("PKI collector leaked onto DefaultRegisterer: %s", mf.GetName())
		}
	}
	priv, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(priv) == 0 {
		t.Fatal("private registry has no PKI series")
	}
}

type stickyIssuer struct {
	inner   *fakeIssuer
	gate    atomic.Pointer[chan struct{}]
	entered atomic.Int32
}

func (s *stickyIssuer) Issue(ctx context.Context, subject string, sans []string) ([]byte, []byte, error) {
	if gate := s.gate.Load(); gate != nil {
		s.entered.Add(1)
		<-*gate
	}
	return s.inner.Issue(ctx, subject, sans)
}

func slogDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(ioDiscard(), nil))
}

func ioDiscard() *discardWriter { return &discardWriter{} }

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestCertMatchesSANs_RejectsExtras(t *testing.T) {
	cert := &x509.Certificate{
		DNSNames:       []string{"alt-backend", "extra"},
		IPAddresses:    []net.IP{net.ParseIP("127.0.0.1")},
		URIs:           []*url.URL{{Scheme: "spiffe", Host: "alt.local", Path: "/alt-backend"}},
		EmailAddresses: []string{"ops@alt.local"},
	}
	if err := certMatchesSANs(cert, []string{"alt-backend"}); err == nil {
		t.Fatal("extra DNS must fail")
	}
	if err := certMatchesSANs(cert, []string{"alt-backend", "extra", "127.0.0.1", "spiffe://alt.local/alt-backend", "ops@alt.local"}); err != nil {
		t.Fatal(err)
	}
}
