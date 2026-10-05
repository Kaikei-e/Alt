package pki

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Observer receives lifecycle events. Tests record; production logs/metrics.
type Observer interface {
	OnClassified(state State, remaining time.Duration)
	OnReissued(reason string)
	OnRenewed(success bool)
	OnRetry(attempt int, err error)
}

type nopObserver struct{}

func (nopObserver) OnClassified(State, time.Duration) {}
func (nopObserver) OnReissued(string)                 {}
func (nopObserver) OnRenewed(bool)                    {}
func (nopObserver) OnRetry(int, error)                {}

// Manager is the cert lifecycle state machine. It has no step-ca dependency:
// tests inject a fake Issuer, a CertFile on a temp dir, and a fake clock.
type Manager struct {
	Cfg      Config
	Issuer   Issuer
	Files    *CertFile
	Log      *slog.Logger
	Observer Observer
	Now      func() time.Time
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) observer() Observer {
	if m.Observer != nil {
		return m.Observer
	}
	return nopObserver{}
}

func (m *Manager) logger() *slog.Logger {
	if m.Log != nil {
		return m.Log
	}
	return slog.Default()
}

// Enroll runs Tick until the leaf is fresh or the retry budget / context is
// exhausted. Failures are logged; they are never treated as success.
func (m *Manager) Enroll(ctx context.Context) error {
	attempts := m.Cfg.RetryAttempts
	if attempts < 1 {
		attempts = 1
	}
	backoff := m.Cfg.RetryBackoff
	if backoff <= 0 {
		backoff = defaultBackoff
	}
	var last error
	for i := 1; i <= attempts; i++ {
		state, err := m.Tick(ctx)
		if err == nil && (state == StateFresh || state == StateNearExpiry) {
			return nil
		}
		if err == nil {
			err = fmt.Errorf("pki: enroll left state %s", state)
		}
		last = err
		m.observer().OnRetry(i, err)
		m.logger().ErrorContext(ctx, "pki_enrollment_retry",
			"subject", m.Cfg.Subject, "attempt", i, "attempts", attempts, "error_type", LogSafeError(err))
		if i == attempts {
			break
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("pki: enroll canceled: %w", ctx.Err())
		case <-timer.C:
		}
	}
	return fmt.Errorf("pki: enroll failed after %d attempts: %w", attempts, last)
}

// Tick inspects the on-disk cert and re-issues when missing, near expiry, or
// expired. Expired certs are re-enrolled (new key), never renewed in place.
func (m *Manager) Tick(ctx context.Context) (State, error) {
	if err := ctx.Err(); err != nil {
		return StateMissing, err
	}
	cert, err := m.Files.Load(ctx)
	if err != nil {
		if errors.Is(err, ErrCertNotFound) {
			return m.issue(ctx, "missing")
		}
		if errors.Is(err, ErrCertParseFailed) || errors.Is(err, ErrCertPairMismatch) {
			return m.issue(ctx, "corrupt")
		}
		return StateCorrupt, fmt.Errorf("pki: load cert: %w", err)
	}
	now := m.now()
	state := ClassifyRemaining(cert.NotBefore, cert.NotAfter, now, m.Cfg.RenewAtFraction)
	m.observer().OnClassified(state, cert.NotAfter.Sub(now))
	switch state {
	case StateFresh:
		return state, nil
	case StateNearExpiry:
		if r, ok := m.Issuer.(RekeyIssuer); ok {
			return m.rekey(ctx, r)
		}
		return m.issue(ctx, "near_expiry")
	case StateExpired:
		return m.issue(ctx, "expired")
	default:
		return state, nil
	}
}

func (m *Manager) issue(ctx context.Context, reason string) (State, error) {
	m.observer().OnReissued(reason)
	m.logger().InfoContext(ctx, "pki_enrollment_reissue",
		"subject", m.Cfg.Subject, "reason", reason, "provisioner", m.Cfg.Provisioner)
	certPEM, keyPEM, err := m.Issuer.Issue(ctx, m.Cfg.Subject, m.Cfg.SANs)
	if err != nil {
		m.observer().OnRenewed(false)
		m.logger().ErrorContext(ctx, "pki_enrollment_failed",
			"subject", m.Cfg.Subject, "reason", reason, "error_type", LogSafeError(err))
		return StateExpired, fmt.Errorf("pki: issue cert: %w", err)
	}
	if err := m.Files.Write(ctx, certPEM, keyPEM); err != nil {
		m.observer().OnRenewed(false)
		m.logger().ErrorContext(ctx, "pki_enrollment_failed",
			"subject", m.Cfg.Subject, "reason", reason, "error_type", LogSafeError(err))
		return StateExpired, fmt.Errorf("pki: write cert: %w", err)
	}
	m.observer().OnRenewed(true)
	cert, lerr := m.Files.Load(ctx)
	if lerr != nil {
		return StateCorrupt, fmt.Errorf("pki: load after write: %w", lerr)
	}
	state := ClassifyRemaining(cert.NotBefore, cert.NotAfter, m.now(), m.Cfg.RenewAtFraction)
	m.observer().OnClassified(state, cert.NotAfter.Sub(m.now()))
	return state, nil
}

func (m *Manager) rekey(ctx context.Context, r RekeyIssuer) (State, error) {
	m.observer().OnReissued("near_expiry")
	m.logger().InfoContext(ctx, "pki_enrollment_rekey",
		"subject", m.Cfg.Subject, "reason", "near_expiry", "provisioner", m.Cfg.Provisioner)
	certPEM, err := readRegularNoFollow(m.Files.CertPath, maxRootPEMBytes)
	if err != nil {
		m.observer().OnRenewed(false)
		return StateExpired, fmt.Errorf("pki: read cert for rekey: %w", err)
	}
	keyPEM, err := readRegularNoFollow(m.Files.KeyPath, maxRootPEMBytes)
	if err != nil {
		m.observer().OnRenewed(false)
		return StateExpired, fmt.Errorf("pki: read key for rekey: %w", err)
	}
	newCert, newKey, err := r.Rekey(ctx, certPEM, keyPEM, m.Cfg.Subject, m.Cfg.SANs)
	if err != nil {
		m.observer().OnRenewed(false)
		m.logger().ErrorContext(ctx, "pki_enrollment_failed",
			"subject", m.Cfg.Subject, "reason", "near_expiry", "error_type", LogSafeError(err))
		return StateExpired, fmt.Errorf("pki: rekey cert: %w", err)
	}
	if err := m.Files.Write(ctx, newCert, newKey); err != nil {
		m.observer().OnRenewed(false)
		m.logger().ErrorContext(ctx, "pki_enrollment_failed",
			"subject", m.Cfg.Subject, "reason", "near_expiry", "error_type", LogSafeError(err))
		return StateExpired, fmt.Errorf("pki: write rekeyed cert: %w", err)
	}
	m.observer().OnRenewed(true)
	cert, lerr := m.Files.Load(ctx)
	if lerr != nil {
		return StateCorrupt, fmt.Errorf("pki: load after rekey: %w", lerr)
	}
	state := ClassifyRemaining(cert.NotBefore, cert.NotAfter, m.now(), m.Cfg.RenewAtFraction)
	m.observer().OnClassified(state, cert.NotAfter.Sub(m.now()))
	return state, nil
}

// Run ticks until ctx is cancelled. Tick errors are logged and retried; they
// never look like a successful continue.
func (m *Manager) Run(ctx context.Context) error {
	interval := m.Cfg.TickInterval
	if interval <= 0 {
		interval = defaultTick
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			m.logger().InfoContext(ctx, "pki_enrollment_stopped", "subject", m.Cfg.Subject, "error", ctx.Err())
			return ctx.Err()
		case <-ticker.C:
			if _, err := m.Tick(ctx); err != nil {
				m.logger().ErrorContext(ctx, "pki_enrollment_tick_failed",
					"subject", m.Cfg.Subject, "error_type", LogSafeError(err))
			}
		}
	}
}
