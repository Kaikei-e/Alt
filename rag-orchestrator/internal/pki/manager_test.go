package pki

import (
	"context"
	"crypto/tls"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"rag-orchestrator/internal/infra/tlsutil"
)

type fakeIssuer struct {
	mu        sync.Mutex
	calls     int
	err       error
	lifetime  time.Duration
	notBefore time.Time
	block     chan struct{}
}

func (f *fakeIssuer) Issue(ctx context.Context, subject string, _ []string) ([]byte, []byte, error) {
	f.mu.Lock()
	f.calls++
	err := f.err
	nb := f.notBefore
	life := f.lifetime
	block := f.block
	f.mu.Unlock()
	if block != nil {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-block:
		}
	}
	if err != nil {
		return nil, nil, err
	}
	if life == 0 {
		life = 24 * time.Hour
	}
	if nb.IsZero() {
		nb = time.Now().Add(-time.Minute)
	}
	cert, key := newSelfSignedPEM(tFromCtx(ctx), subject, nb, nb.Add(life))
	return cert, key, nil
}

func (f *fakeIssuer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type testKey struct{}

func withT(ctx context.Context, t *testing.T) context.Context {
	return context.WithValue(ctx, testKey{}, t)
}

func tFromCtx(ctx context.Context) *testing.T {
	if t, ok := ctx.Value(testKey{}).(*testing.T); ok {
		return t
	}
	panic("fakeIssuer.Issue requires withT")
}

type recObs struct {
	mu         sync.Mutex
	classified []State
	reissued   []string
	renewed    []bool
	retries    int
}

func (o *recObs) OnClassified(s State, _ time.Duration) {
	o.mu.Lock()
	o.classified = append(o.classified, s)
	o.mu.Unlock()
}
func (o *recObs) OnReissued(r string) {
	o.mu.Lock()
	o.reissued = append(o.reissued, r)
	o.mu.Unlock()
}
func (o *recObs) OnRenewed(ok bool) {
	o.mu.Lock()
	o.renewed = append(o.renewed, ok)
	o.mu.Unlock()
}
func (o *recObs) OnRetry(int, error) {
	o.mu.Lock()
	o.retries++
	o.mu.Unlock()
}

func newTestManager(t *testing.T, issuer Issuer, now time.Time) (*Manager, *CertFile, *recObs) {
	t.Helper()
	dir := t.TempDir()
	files := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: filepath.Join(dir, "svc-key.pem")}
	obs := &recObs{}
	m := &Manager{
		Cfg: Config{
			Subject:         "alt-backend",
			SANs:            []string{"alt-backend"},
			Provisioner:     "pki-agent-alt-backend",
			RenewAtFraction: 0.66,
			RetryAttempts:   3,
			RetryBackoff:    time.Millisecond,
		},
		Issuer:   issuer,
		Files:    files,
		Observer: obs,
		Now:      func() time.Time { return now },
	}
	return m, files, obs
}

func TestTick_Missing_TriggersIssue(t *testing.T) {
	nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	iss := &fakeIssuer{notBefore: nb, lifetime: 24 * time.Hour}
	m, _, obs := newTestManager(t, iss, nb)
	ctx := withT(context.Background(), t)
	state, err := m.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state != StateFresh {
		t.Fatalf("state=%s", state)
	}
	if iss.callCount() != 1 {
		t.Fatalf("issue calls=%d", iss.callCount())
	}
	if len(obs.reissued) != 1 || obs.reissued[0] != "missing" {
		t.Fatalf("reissued=%v", obs.reissued)
	}
}

func TestTick_MismatchedPair_ReissuesImmediately(t *testing.T) {
	nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	iss := &fakeIssuer{notBefore: nb, lifetime: 24 * time.Hour}
	m, files, obs := newTestManager(t, iss, nb.Add(time.Hour))
	ctx := withT(context.Background(), t)
	cert, key := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(24*time.Hour))
	if err := files.Write(ctx, cert, key); err != nil {
		t.Fatal(err)
	}
	_, otherKey := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(24*time.Hour))
	if err := os.Remove(files.KeyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.KeyPath, otherKey, 0o400); err != nil {
		t.Fatal(err)
	}
	state, err := m.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state != StateFresh {
		t.Fatalf("reissue should restore fresh, state=%s", state)
	}
	if iss.callCount() != 1 {
		t.Fatalf("mismatch must reissue immediately, calls=%d", iss.callCount())
	}
	if len(obs.reissued) != 1 || obs.reissued[0] != "corrupt" {
		t.Fatalf("reissued=%v", obs.reissued)
	}
	onDiskCert, err := os.ReadFile(files.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	onDiskKey, err := os.ReadFile(files.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.X509KeyPair(onDiskCert, onDiskKey); err != nil {
		t.Fatalf("reissued pair must match: %v", err)
	}
}

func TestTick_JournalRestore_DoesNotReportFreshOnNewKey(t *testing.T) {
	nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	iss := &fakeIssuer{notBefore: nb.Add(time.Hour), lifetime: 24 * time.Hour}
	m, files, _ := newTestManager(t, iss, nb.Add(time.Hour))
	ctx := withT(context.Background(), t)
	certA, keyA := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(24*time.Hour))
	if err := files.Write(ctx, certA, keyA); err != nil {
		t.Fatal(err)
	}
	_, keyB := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(24*time.Hour))
	if err := os.WriteFile(files.journalPath(), keyA, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(files.KeyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.KeyPath, keyB, 0o400); err != nil {
		t.Fatal(err)
	}
	state, err := m.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state != StateFresh {
		t.Fatalf("restored previous pair should be fresh, state=%s", state)
	}
	if iss.callCount() != 0 {
		t.Fatalf("journal restore must not reissue, calls=%d", iss.callCount())
	}
	onDiskKey, err := os.ReadFile(files.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDiskKey) != string(keyA) {
		t.Fatal("Tick must restore the journaled previous key, not keep the unmatched new key")
	}
}

func TestTick_Fresh_Noop(t *testing.T) {
	nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	iss := &fakeIssuer{notBefore: nb, lifetime: 24 * time.Hour}
	m, files, _ := newTestManager(t, iss, nb.Add(time.Hour))
	ctx := withT(context.Background(), t)
	cert, key := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(24*time.Hour))
	if err := files.Write(ctx, cert, key); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if iss.callCount() != 0 {
		t.Fatalf("fresh cert should not issue, calls=%d", iss.callCount())
	}
}

func TestTick_NearExpiry_Reissues(t *testing.T) {
	nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	iss := &fakeIssuer{notBefore: nb.Add(16 * time.Hour), lifetime: 24 * time.Hour}
	m, files, obs := newTestManager(t, iss, nb.Add(16*time.Hour))
	ctx := withT(context.Background(), t)
	cert, key := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(24*time.Hour))
	if err := files.Write(ctx, cert, key); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if iss.callCount() != 1 {
		t.Fatalf("near_expiry should issue, calls=%d", iss.callCount())
	}
	if obs.reissued[0] != "near_expiry" {
		t.Fatalf("reason=%v", obs.reissued)
	}
}

func TestTick_Expired_ReenrollsNotRenews(t *testing.T) {
	nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	iss := &fakeIssuer{notBefore: nb.Add(25 * time.Hour), lifetime: 24 * time.Hour}
	m, files, obs := newTestManager(t, iss, nb.Add(25*time.Hour))
	ctx := withT(context.Background(), t)
	cert, key := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(24*time.Hour))
	if err := files.Write(ctx, cert, key); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if iss.callCount() != 1 {
		t.Fatalf("expired should re-enroll, calls=%d", iss.callCount())
	}
	if obs.reissued[0] != "expired" {
		t.Fatalf("reason=%v", obs.reissued)
	}
}

func TestTick_IssuerFails_Propagates(t *testing.T) {
	iss := &fakeIssuer{err: errors.New("CA down")}
	m, _, obs := newTestManager(t, iss, time.Now())
	ctx := withT(context.Background(), t)
	if _, err := m.Tick(ctx); err == nil {
		t.Fatal("expected error")
	}
	if len(obs.renewed) != 1 || obs.renewed[0] {
		t.Fatalf("renewed=%v", obs.renewed)
	}
}

func TestEnroll_RetriesThenFails(t *testing.T) {
	iss := &fakeIssuer{err: errors.New("CA down")}
	m, _, obs := newTestManager(t, iss, time.Now())
	ctx := withT(context.Background(), t)
	if err := m.Enroll(ctx); err == nil {
		t.Fatal("expected enroll failure")
	}
	if iss.callCount() != 3 {
		t.Fatalf("attempts=%d", iss.callCount())
	}
	if obs.retries == 0 {
		t.Fatal("expected loud retries")
	}
}

func TestEnroll_Canceled(t *testing.T) {
	iss := &fakeIssuer{block: make(chan struct{})}
	m, _, _ := newTestManager(t, iss, time.Now())
	ctx, cancel := context.WithCancel(withT(context.Background(), t))
	done := make(chan error, 1)
	go func() { done <- m.Enroll(ctx) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("enroll did not observe cancel")
	}
}

func TestRun_StopsOnCancel(t *testing.T) {
	nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	iss := &fakeIssuer{notBefore: nb, lifetime: 24 * time.Hour}
	m, files, _ := newTestManager(t, iss, nb)
	m.Cfg.TickInterval = 30 * time.Millisecond
	ctx := withT(context.Background(), t)
	cert, key := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(24*time.Hour))
	if err := files.Write(ctx, cert, key); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- m.Run(runCtx) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestTick_AtomicWriteVisibleToTLSUtil(t *testing.T) {
	nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	iss := &fakeIssuer{notBefore: nb, lifetime: time.Hour}
	m, files, _ := newTestManager(t, iss, nb)
	ctx := withT(context.Background(), t)
	if _, err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	ca := files.CertPath
	cfg, err := tlsutil.LoadServerConfig(files.CertPath, files.KeyPath, ca)
	if err != nil {
		t.Fatal(err)
	}
	first, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "alt-backend"})
	if err != nil {
		t.Fatal(err)
	}
	iss.mu.Lock()
	iss.notBefore = nb.Add(time.Minute)
	iss.mu.Unlock()
	m.Now = func() time.Time { return nb.Add(50 * time.Minute) }
	oldCert, key := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(time.Hour))
	if err := files.Write(ctx, oldCert, key); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "alt-backend"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Certificate[0] == nil || second.Certificate[0] == nil {
		t.Fatal("missing leaf")
	}
	if string(first.Certificate[0]) == string(second.Certificate[0]) {
		t.Fatal("GetCertificate did not observe atomic rotation")
	}
}

type recordingRekeyIssuer struct {
	fakeIssuer
	rekeyCalls int
}

func (f *recordingRekeyIssuer) Rekey(ctx context.Context, _, _ []byte, subject string, sans []string) ([]byte, []byte, error) {
	f.mu.Lock()
	f.rekeyCalls++
	f.mu.Unlock()
	return f.Issue(ctx, subject, sans)
}

func TestTick_NearExpiry_UsesRekeyWhenAvailable(t *testing.T) {
	nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	iss := &recordingRekeyIssuer{fakeIssuer: fakeIssuer{notBefore: nb.Add(16 * time.Hour), lifetime: 24 * time.Hour}}
	m, files, obs := newTestManager(t, iss, nb.Add(16*time.Hour))
	ctx := withT(context.Background(), t)
	cert, key := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(24*time.Hour))
	if err := files.Write(ctx, cert, key); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if iss.rekeyCalls != 1 {
		t.Fatalf("rekey calls=%d", iss.rekeyCalls)
	}
	if iss.callCount() != 1 {
		t.Fatalf("issue via rekey=%d", iss.callCount())
	}
	if obs.reissued[0] != "near_expiry" {
		t.Fatalf("reason=%v", obs.reissued)
	}
}

func TestTick_Expired_DoesNotRekey(t *testing.T) {
	nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	iss := &recordingRekeyIssuer{fakeIssuer: fakeIssuer{notBefore: nb.Add(25 * time.Hour), lifetime: 24 * time.Hour}}
	m, files, obs := newTestManager(t, iss, nb.Add(25*time.Hour))
	ctx := withT(context.Background(), t)
	cert, key := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(24*time.Hour))
	if err := files.Write(ctx, cert, key); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if iss.rekeyCalls != 0 {
		t.Fatalf("expired must not rekey, calls=%d", iss.rekeyCalls)
	}
	if iss.callCount() != 1 {
		t.Fatalf("expired should re-enroll, calls=%d", iss.callCount())
	}
	if obs.reissued[0] != "expired" {
		t.Fatalf("reason=%v", obs.reissued)
	}
}
