// Package usecase orchestrates the cert lifecycle state machine. Depends
// only on domain (entities + port interfaces). No HTTP, disk, or crypto
// imports.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"pki-agent/internal/domain"
)

// Rotator is the single public entry point. Caller invokes Tick on a
// ticker; the usecase is not internally concurrent.
type Rotator struct {
	Subject         string
	SANs            []string
	RenewAtFraction float64
	Loader          domain.CertLoader
	Issuer          domain.CAIssuer
	Writer          domain.CertWriter
	Observer        domain.Observer
}

// Tick inspects the on-disk cert and triggers reissue as needed.
func (r *Rotator) Tick(ctx context.Context, now time.Time) (domain.CertState, error) {
	cert, err := r.Loader.Load(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrCertNotFound) {
			return r.issue(ctx, "missing")
		}
		if errors.Is(err, domain.ErrCertParseFailed) {
			return r.issue(ctx, "corrupt")
		}
		return domain.StateCorrupt, fmt.Errorf("load cert: %w", err)
	}
	state := domain.ClassifyRemaining(cert.NotBefore, cert.NotAfter, now, r.RenewAtFraction)
	r.Observer.OnClassified(state, cert.NotAfter.Sub(now))
	switch state {
	case domain.StateFresh:
		return state, nil
	case domain.StateNearExpiry, domain.StateExpired:
		reason := "near_expiry"
		if state == domain.StateExpired {
			reason = "expired"
		}
		return r.issue(ctx, reason)
	default:
		return state, nil
	}
}

func (r *Rotator) issue(ctx context.Context, reason string) (domain.CertState, error) {
	r.Observer.OnReissued(reason)
	certPEM, keyPEM, err := r.Issuer.Issue(ctx, r.Subject, r.SANs)
	if err != nil {
		r.Observer.OnRenewed(false)
		return domain.StateExpired, fmt.Errorf("issue cert: %w", err)
	}
	if err := r.Writer.Write(ctx, certPEM, keyPEM); err != nil {
		r.Observer.OnRenewed(false)
		return domain.StateExpired, fmt.Errorf("write cert: %w", err)
	}
	if err := r.Writer.MarkRotated(ctx, time.Now()); err != nil {
		r.Observer.OnRenewed(false)
		return domain.StateFresh, nil
	}
	r.Observer.OnRenewed(true)
	// Re-load the cert we just wrote so the observer's classified state
	// reflects the new StateFresh immediately. Without this, /healthz
	// (which reads observer.State()) stays at 503 until the next tick
	// classifies an existing cert — up to TICK_INTERVAL after the first
	// successful issue on a cold-started sidecar.
	if cert, lerr := r.Loader.Load(ctx); lerr == nil {
		now := time.Now()
		state := domain.ClassifyRemaining(cert.NotBefore, cert.NotAfter, now, r.RenewAtFraction)
		r.Observer.OnClassified(state, cert.NotAfter.Sub(now))
	}
	return domain.StateFresh, nil
}
