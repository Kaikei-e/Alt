package usecase

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pki-agent/internal/domain"
	"pki-agent/internal/infrastructure"
)

var (
	_ domain.CertLoader = (*fakeLoader)(nil)
	_ domain.CAIssuer   = (*fakeIssuer)(nil)
	_ domain.CertWriter = (*fakeWriter)(nil)
	_ domain.Observer   = (*fakeObs)(nil)
)

type fakeLoader struct {
	cert *x509.Certificate
	err  error
	// postIssueCert, when non-nil, is returned from Load starting with the
	// second call — simulates the on-disk cert appearing after the issuer
	// wrote it. Preserves the existing "always return cert/err" behaviour
	// when left nil.
	postIssueCert *x509.Certificate
	calls         int
}

func (f *fakeLoader) Load(_ context.Context) (*x509.Certificate, error) {
	f.calls++
	if f.postIssueCert != nil && f.calls > 1 {
		return f.postIssueCert, nil
	}
	return f.cert, f.err
}

type fakeIssuer struct {
	called int
	err    error
}

func (f *fakeIssuer) Issue(_ context.Context, _ string, _ []string) ([]byte, []byte, error) {
	f.called++
	return []byte("CERT"), []byte("KEY"), f.err
}

type fakeWriter struct {
	wrote, marked int
	writeErr      error
}

func (f *fakeWriter) Write(_ context.Context, _, _ []byte) error {
	f.wrote++
	return f.writeErr
}

func (f *fakeWriter) MarkRotated(_ context.Context, _ time.Time) error {
	f.marked++
	return nil
}

type fakeObs struct {
	classified []domain.CertState
	reissued   []string
	renewed    []bool
}

func (f *fakeObs) OnClassified(s domain.CertState, _ time.Duration) {
	f.classified = append(f.classified, s)
}
func (f *fakeObs) OnReissued(r string) { f.reissued = append(f.reissued, r) }
func (f *fakeObs) OnRenewed(ok bool)   { f.renewed = append(f.renewed, ok) }

func newRotator(l domain.CertLoader, i domain.CAIssuer, w domain.CertWriter, o domain.Observer) *Rotator {
	return &Rotator{
		Subject: "alt-backend", SANs: []string{"alt-backend"},
		RenewAtFraction: 0.66,
		Loader:          l, Issuer: i, Writer: w, Observer: o,
	}
}

func TestTick_Missing_TriggersIssue(t *testing.T) {
	loader := &fakeLoader{err: domain.ErrCertNotFound}
	issuer := &fakeIssuer{}
	writer := &fakeWriter{}
	obs := &fakeObs{}
	r := newRotator(loader, issuer, writer, obs)

	state, err := r.Tick(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if state != domain.StateFresh {
		t.Fatalf("state=%s", state)
	}
	if issuer.called != 1 || writer.wrote != 1 || writer.marked != 1 {
		t.Fatalf("issue chain not called: %+v %+v", issuer, writer)
	}
	if len(obs.reissued) != 1 || obs.reissued[0] != "missing" {
		t.Fatalf("observer: %+v", obs)
	}
}

func TestTick_Expired_ReissuesNotRenews(t *testing.T) {
	nb := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	cert := &x509.Certificate{NotBefore: nb, NotAfter: nb.Add(24 * time.Hour)}
	loader := &fakeLoader{cert: cert}
	issuer := &fakeIssuer{}
	writer := &fakeWriter{}
	obs := &fakeObs{}
	r := newRotator(loader, issuer, writer, obs)

	// now is 25h after not_before -> expired.
	state, err := r.Tick(context.Background(), nb.Add(25*time.Hour))
	if err != nil || state != domain.StateFresh {
		t.Fatalf("state=%s err=%v", state, err)
	}
	if obs.reissued[0] != "expired" {
		t.Fatalf("expected expired reason, got %v", obs.reissued)
	}
}

func TestTick_Fresh_Noop(t *testing.T) {
	nb := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	cert := &x509.Certificate{NotBefore: nb, NotAfter: nb.Add(24 * time.Hour)}
	loader := &fakeLoader{cert: cert}
	issuer := &fakeIssuer{}
	writer := &fakeWriter{}
	obs := &fakeObs{}
	r := newRotator(loader, issuer, writer, obs)

	_, err := r.Tick(context.Background(), nb.Add(1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if issuer.called != 0 || writer.wrote != 0 {
		t.Fatalf("fresh cert should not issue: %+v %+v", issuer, writer)
	}
	if obs.classified[0] != domain.StateFresh {
		t.Fatalf("classified=%v", obs.classified)
	}
}

func TestTick_NearExpiry_Reissues(t *testing.T) {
	nb := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	cert := &x509.Certificate{NotBefore: nb, NotAfter: nb.Add(24 * time.Hour)}
	loader := &fakeLoader{cert: cert}
	issuer := &fakeIssuer{}
	writer := &fakeWriter{}
	obs := &fakeObs{}
	r := newRotator(loader, issuer, writer, obs)

	_, err := r.Tick(context.Background(), nb.Add(20*time.Hour)) // 83% elapsed
	if err != nil {
		t.Fatal(err)
	}
	if issuer.called != 1 {
		t.Fatalf("near_expiry should issue; issuer=%+v", issuer)
	}
	if obs.reissued[0] != "near_expiry" {
		t.Fatalf("reason=%v", obs.reissued)
	}
}

func TestTick_IssuerFails_Propagates(t *testing.T) {
	loader := &fakeLoader{err: domain.ErrCertNotFound}
	issuer := &fakeIssuer{err: errors.New("CA down")}
	r := newRotator(loader, issuer, &fakeWriter{}, &fakeObs{})
	if _, err := r.Tick(context.Background(), time.Now()); err == nil {
		t.Fatal("expected error")
	}
}

// TestTick_Missing_IssueUpdatesObserverClassified: after a successful
// issue-on-missing, the observer must see OnClassified(StateFresh) so that
// /healthz (which reads observer state) reports 200 immediately instead of
// staying at 503 until the next cert-rotation tick. Cold-started sidecars
// previously sat unhealthy for up to TICK_INTERVAL (default 5 min) after
// their very first cert issue; kubelet / docker healthcheck saw the
// misaligned state as if the service were broken.
func TestTick_Missing_IssueUpdatesObserverClassified(t *testing.T) {
	nb := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	postCert := &x509.Certificate{NotBefore: nb, NotAfter: nb.Add(24 * time.Hour)}
	loader := &fakeLoader{err: domain.ErrCertNotFound, postIssueCert: postCert}
	issuer := &fakeIssuer{}
	writer := &fakeWriter{}
	obs := &fakeObs{}
	r := newRotator(loader, issuer, writer, obs)

	state, err := r.Tick(context.Background(), nb)
	if err != nil || state != domain.StateFresh {
		t.Fatalf("state=%s err=%v", state, err)
	}
	var sawFresh bool
	for _, s := range obs.classified {
		if s == domain.StateFresh {
			sawFresh = true
			break
		}
	}
	if !sawFresh {
		t.Fatalf("want OnClassified(StateFresh) after issue-on-missing; got classified=%v", obs.classified)
	}
}

func newSelfSignedPEMAt(t *testing.T, notBefore, notAfter time.Time) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "alt-backend"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

func TestTick_MismatchedPairOnDisk_TriggersReissue(t *testing.T) {
	dir := t.TempDir()
	nb := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	cert1, _ := newSelfSignedPEMAt(t, nb, nb.Add(24*time.Hour))
	_, key2 := newSelfSignedPEMAt(t, nb, nb.Add(24*time.Hour))

	certPath := filepath.Join(dir, "svc-cert.pem")
	keyPath := filepath.Join(dir, "svc-key.pem")
	if err := os.WriteFile(certPath, cert1, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, key2, 0o400); err != nil {
		t.Fatal(err)
	}

	loader := &infrastructure.CertFile{CertPath: certPath, KeyPath: keyPath}
	issuer := &fakeIssuer{}
	writer := &fakeWriter{}
	obs := &fakeObs{}
	r := newRotator(loader, issuer, writer, obs)

	state, err := r.Tick(context.Background(), nb.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != domain.StateFresh {
		t.Fatalf("want StateFresh after reissue, got %s", state)
	}
	if issuer.called != 1 || writer.wrote != 1 {
		t.Fatalf("mismatched cert/key pair on disk must trigger reissue: issuer.called=%d writer.wrote=%d", issuer.called, writer.wrote)
	}
}
