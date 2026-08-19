package pki

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"alt-butterfly-facade/internal/tlsutil"
	"go.step.sm/crypto/jose"
)

func TestLoadConfig_EmptyEnrollmentFails(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", "")
	if _, err := LoadConfig("alt-butterfly-facade"); err == nil {
		t.Fatal("empty PKI_ENROLLMENT must fail, not default disabled")
	}
}

func TestLoadConfig_ExactProvisionerAndPasswordBasename(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "alt-butterfly-facade")
	t.Setenv("STEP_CA_PROVISIONER", "pki-agent-alt-butterfly-facade-extra")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/pki-agent-alt-butterfly-facade-extra-jwk")
	_, err := LoadConfig("alt-butterfly-facade")
	if err == nil {
		t.Fatal("substring-scoped provisioner/password must be rejected")
	}
	if errors.Is(err, ErrSharedProvisioner) {
		t.Fatalf("wrong sentinel: %v", err)
	}
}

func TestLoadConfig_RejectsWrongPasswordBasename(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "alt-butterfly-facade")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", filepath.Join(t.TempDir(), "wrong-name"))
	if _, err := LoadConfig("alt-butterfly-facade"); err == nil {
		t.Fatal("password basename must be exactly pki-agent-<subject>-jwk")
	}
}

func TestLoadConfig_TempDirAllowedWhenBasenameMatches(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "alt-butterfly-facade")
	path := filepath.Join(t.TempDir(), "pki-agent-alt-butterfly-facade-jwk")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", path)
	c, err := LoadConfig("alt-butterfly-facade")
	if err != nil {
		t.Fatal(err)
	}
	if c.PasswordFile != path {
		t.Fatalf("password file=%q", c.PasswordFile)
	}
}

func TestInspectJWEHeader_RejectsWrongP2CZipUnknown(t *testing.T) {
	header := func(h map[string]any) string {
		t.Helper()
		raw, err := json.Marshal(h)
		if err != nil {
			t.Fatal(err)
		}
		b64 := base64.RawURLEncoding.EncodeToString(raw)
		return b64 + ".aaaa.bbbb.cccc.dddd"
	}
	if err := inspectJWEHeader(header(map[string]any{
		"alg": stepCAPBES2Alg, "enc": stepCAPBES2Enc, "p2c": 1000,
	})); err == nil || !strings.Contains(err.Error(), "p2c") {
		t.Fatalf("wrong p2c: %v", err)
	}
	if err := inspectJWEHeader(header(map[string]any{
		"alg": stepCAPBES2Alg, "enc": stepCAPBES2Enc, "p2c": stepCAPBES2P2C, "zip": "DEF",
	})); err == nil {
		t.Fatal("zip must be rejected")
	}
	if err := inspectJWEHeader(header(map[string]any{
		"alg": stepCAPBES2Alg, "enc": stepCAPBES2Enc, "p2c": stepCAPBES2P2C, "evil": true,
	})); err == nil {
		t.Fatal("unknown header must be rejected")
	}
	tooBig := strings.Repeat("a", maxCompactJWEBytes+1)
	if err := inspectJWEHeader(tooBig); err == nil || !strings.Contains(err.Error(), "size cap") {
		t.Fatalf("compact cap: %v", err)
	}
	if err := inspectJWEHeader(header(map[string]any{
		"alg": stepCAPBES2Alg, "enc": stepCAPBES2Enc, "p2c": stepCAPBES2P2C, "p2s": "YWFh",
	})); err != nil {
		t.Fatalf("step-ca default header must pass: %v", err)
	}
}

func TestNativeStepCAIssuer_RejectsWrongP2CBeforeDecrypt(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-butterfly-facade", []byte("pw-p2c"))
	iss := newNativeIssuer(t, ca)
	bad, err := jose.NewEncrypter(jose.DefaultEncAlgorithm, jose.Recipient{
		Algorithm:  jose.PBES2_HS256_A128KW,
		Key:        ca.password,
		PBES2Count: 1000,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(ca.jwk)
	if err != nil {
		t.Fatal(err)
	}
	jwe, err := bad.Encrypt(raw)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := jwe.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decryptProvisionerJWK(compact, ca.password); err == nil {
		t.Fatal("p2c=1000 must fail before PBKDF2")
	}
	if _, _, err := iss.Issue(context.Background(), "alt-butterfly-facade", []string{"alt-butterfly-facade"}); err != nil {
		t.Fatal(err)
	}
}

func TestNativeStepCAIssuer_RequiresDualEKU(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-butterfly-facade", []byte("pw-eku"))
	ca.mutateLeaf = func(c *x509.Certificate) {
		c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "alt-butterfly-facade", []string{"alt-butterfly-facade"}); err == nil {
		t.Fatal("serverAuth-only leaf must be rejected")
	}
}

func TestHandle_StopWaitsForInFlightRun(t *testing.T) {
	dir := t.TempDir()
	inner := &fakeIssuer{notBefore: time.Now().Add(-time.Minute), lifetime: 24 * time.Hour}
	sticky := &stickyIssuer{inner: inner}
	cfg := &Config{
		Mode:            ModeEnabled,
		Subject:         "alt-butterfly-facade",
		SANs:            []string{"alt-butterfly-facade"},
		CertPath:        filepath.Join(dir, "svc-cert.pem"),
		KeyPath:         filepath.Join(dir, "svc-key.pem"),
		Provisioner:     "pki-agent-alt-butterfly-facade",
		PasswordFile:    "/run/secrets/pki-agent-alt-butterfly-facade-jwk",
		RenewAtFraction: 0.66,
		TickInterval:    15 * time.Millisecond,
		RetryAttempts:   1,
		RetryBackoff:    time.Millisecond,
	}
	ctx := withT(context.Background(), t)
	h, err := StartWith(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg, sticky)
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

func TestHandle_StopClosesIdleConnections(t *testing.T) {
	dir := t.TempDir()
	inner := &fakeIssuer{notBefore: time.Now().Add(-time.Minute), lifetime: 24 * time.Hour}
	iss := &closeTrackingIssuer{inner: inner}
	cfg := &Config{
		Mode:            ModeEnabled,
		Subject:         "alt-butterfly-facade",
		SANs:            []string{"alt-butterfly-facade"},
		CertPath:        filepath.Join(dir, "svc-cert.pem"),
		KeyPath:         filepath.Join(dir, "svc-key.pem"),
		Provisioner:     "pki-agent-alt-butterfly-facade",
		PasswordFile:    "/run/secrets/pki-agent-alt-butterfly-facade-jwk",
		RenewAtFraction: 0.66,
		TickInterval:    time.Hour,
		RetryAttempts:   1,
		RetryBackoff:    time.Millisecond,
	}
	ctx := withT(context.Background(), t)
	h, err := StartWith(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg, iss)
	if err != nil {
		t.Fatal(err)
	}
	h.Stop()
	if iss.closed.Load() == 0 {
		t.Fatal("Stop must CloseIdleConnections")
	}
}

func TestCertFile_WriteRespectsCanceledContext(t *testing.T) {
	dir := t.TempDir()
	cf := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: filepath.Join(dir, "svc-key.pem")}
	nb := time.Now().Add(-time.Minute)
	cert, key := newSelfSignedPEM(t, "alt-butterfly-facade", nb, nb.Add(time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cf.Write(ctx, cert, key); err == nil {
		t.Fatal("Write after cancel must fail")
	}
	if _, err := os.Stat(cf.CertPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canceled Write must not install files")
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

type closeTrackingIssuer struct {
	inner  *fakeIssuer
	closed atomic.Int32
}

func (c *closeTrackingIssuer) Issue(ctx context.Context, subject string, sans []string) ([]byte, []byte, error) {
	return c.inner.Issue(ctx, subject, sans)
}

func (c *closeTrackingIssuer) CloseIdleConnections() {
	c.closed.Add(1)
}

func TestNativeStepCAIssuer_CARootSymlinkRejected(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-butterfly-facade", []byte("pw-rootlink"))
	iss := newNativeIssuer(t, ca)
	dir := t.TempDir()
	link := filepath.Join(dir, "root.pem")
	if err := os.Symlink(iss.RootFile, link); err != nil {
		t.Fatal(err)
	}
	iss.RootFile = link
	iss.cred = nil
	if _, _, err := iss.Issue(context.Background(), "alt-butterfly-facade", []string{"alt-butterfly-facade"}); err == nil {
		t.Fatal("symlink CA root must be rejected")
	}
}

func TestCertFile_CertRenameFailureRollsBackKey(t *testing.T) {
	dir := t.TempDir()
	cf := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: filepath.Join(dir, "svc-key.pem")}
	nb := time.Now().Add(-time.Minute)
	oldCert, oldKey := newSelfSignedPEM(t, "alt-butterfly-facade", nb, nb.Add(time.Hour))
	if err := cf.Write(context.Background(), oldCert, oldKey); err != nil {
		t.Fatal(err)
	}
	newCert, newKey := newSelfSignedPEM(t, "alt-butterfly-facade", nb, nb.Add(2*time.Hour))
	cf.renameFile = func(oldpath, newpath string) error {
		if newpath == cf.CertPath {
			return errors.New("injected cert rename failure")
		}
		return os.Rename(oldpath, newpath)
	}
	if err := cf.Write(context.Background(), newCert, newKey); err == nil {
		t.Fatal("expected cert rename failure")
	}
	cf.renameFile = nil
	if _, err := tls.X509KeyPair(mustRead(t, cf.CertPath), mustRead(t, cf.KeyPath)); err != nil {
		t.Fatalf("rollback left a mismatched pair: %v", err)
	}
	got, err := cf.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.NotAfter.Sub(got.NotBefore) != time.Hour {
		t.Fatalf("expected old leaf lifetime, got %s", got.NotAfter.Sub(got.NotBefore))
	}
}

func TestCertFile_CrashBetweenRenames_LoadCorruptAndTickReissues(t *testing.T) {
	nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	iss := &fakeIssuer{notBefore: nb, lifetime: 24 * time.Hour}
	m, files, _ := newTestManager(t, iss, nb)
	ctx := withT(context.Background(), t)
	if _, err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	callsAfterEnroll := iss.callCount()
	oldCert := mustRead(t, files.CertPath)
	newCert, newKey := newSelfSignedPEM(t, "alt-butterfly-facade", nb, nb.Add(48*time.Hour))
	files.afterKeyRename = func() error {
		return errSimulatedCrash
	}
	if err := files.Write(ctx, newCert, newKey); !errors.Is(err, errSimulatedCrash) {
		t.Fatalf("got %v", err)
	}
	files.afterKeyRename = nil

	_, err := files.Load(ctx)
	if !errors.Is(err, ErrCertPairMismatch) {
		t.Fatalf("crashed install must not classify as Fresh: %v", err)
	}
	if _, err := tls.X509KeyPair(mustRead(t, files.CertPath), mustRead(t, files.KeyPath)); err != nil {
		t.Fatalf("hot-reloader last-good disk pair was left mismatched: %v", err)
	}
	if string(mustRead(t, files.CertPath)) != string(oldCert) {
		t.Fatal("crash recovery must restore last-good cert pairing")
	}

	state, err := m.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state != StateFresh && state != StateNearExpiry {
		t.Fatalf("state=%s", state)
	}
	if iss.callCount() <= callsAfterEnroll {
		t.Fatal("Tick must reissue immediately after crashed install")
	}
	if _, err := files.Load(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTick_CrashKeepsTLSUtilLastGood(t *testing.T) {
	nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	iss := &fakeIssuer{notBefore: nb, lifetime: time.Hour}
	m, files, _ := newTestManager(t, iss, nb)
	ctx := withT(context.Background(), t)
	if _, err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	cfg, err := tlsutil.LoadServerConfig(files.CertPath, files.KeyPath, files.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	first, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "alt-butterfly-facade"})
	if err != nil {
		t.Fatal(err)
	}
	newCert, newKey := newSelfSignedPEM(t, "alt-butterfly-facade", nb, nb.Add(2*time.Hour))
	files.afterKeyRename = func() error { return errSimulatedCrash }
	_ = files.Write(ctx, newCert, newKey)
	files.afterKeyRename = nil
	second, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "alt-butterfly-facade"})
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Certificate[0]) != string(second.Certificate[0]) {
		t.Fatal("GetCertificate must keep last-good across crashed install")
	}
}
