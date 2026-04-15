package domain

import (
	"context"
	"crypto/x509"
	"time"
)

// Port interfaces (domain-level contracts). Implementations live in
// adapter/gateway/ (business-level wrappers) and infrastructure/ (raw
// drivers). Usecase layer depends only on these.

// CertLoader reads the leaf cert from a shared volume.
type CertLoader interface {
	Load(ctx context.Context) (*x509.Certificate, error)
}

// CAIssuer requests a freshly-signed leaf from the configured CA.
// Implementations MUST generate a new private key on every call.
type CAIssuer interface {
	Issue(ctx context.Context, subject string, sans []string) (certPEM, keyPEM []byte, err error)
}

// CertWriter persists cert / key pair atomically and records rotation events.
type CertWriter interface {
	Write(ctx context.Context, certPEM, keyPEM []byte) error
	MarkRotated(ctx context.Context, at time.Time) error
}

// Observer receives lifecycle events for metrics / logging. Non-blocking.
type Observer interface {
	OnClassified(state CertState, remaining time.Duration)
	OnReissued(reason string)
	OnRenewed(success bool)
}
