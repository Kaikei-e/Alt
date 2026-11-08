// Package infrastructure contains side-effecting implementations: disk I/O,
// HTTP clients, OTT signing. Keep all OS / crypto / net calls here so the
// usecase layer stays pure.
package infrastructure

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pki-agent/internal/domain"
)

// CertFile reads/writes a PEM leaf cert + key pair on a shared volume.
// Writes are atomic: content lands in a sibling tmpfile (created with the
// target perms/ownership) and is renamed into place. Readers never see a
// partial file.
type CertFile struct {
	CertPath string
	KeyPath  string
	OwnerUID int
	OwnerGID int
	// ChownRequested is the explicit signal that OwnerUID/OwnerGID were
	// actually specified by the composition root and should be applied.
	// It must NOT be derived from `OwnerUID > 0` — UID 0 (root) is a valid,
	// intentional chown target (e.g. CERT_OWNER_UID=0 for the
	// tag-generator sidecar in compose/pki.yaml) and must not collide with
	// the zero-value "don't chown, inherit the runtime uid" case that bare
	// struct literals (tests, ad-hoc callers) rely on.
	ChownRequested bool

	// chown is the seam used to change ownership; defaults to os.Chown.
	// Tests inject a stub here so the chown *decision* is verifiable
	// without requiring the CAP_CHOWN privilege the real syscall needs.
	chown func(name string, uid, gid int) error
}

// Load reads the cert file and returns its parsed X.509. Returns
// domain.ErrCertNotFound / domain.ErrCertParseFailed per contract.
func (c *CertFile) Load(_ context.Context) (*x509.Certificate, error) {
	raw, err := os.ReadFile(c.CertPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, domain.ErrCertNotFound
		}
		return nil, fmt.Errorf("%w: %v", domain.ErrCertParseFailed, err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("%w: no PEM block", domain.ErrCertParseFailed)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrCertParseFailed, err)
	}
	keyRaw, err := os.ReadFile(c.KeyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, domain.ErrCertNotFound
		}
		return nil, fmt.Errorf("%w: %v", domain.ErrCertParseFailed, err)
	}
	if _, err := tls.X509KeyPair(raw, keyRaw); err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrCertKeyMismatch, err)
	}
	return cert, nil
}

// Write atomically replaces both cert and key. The cert is written as
// 0444, the key as 0400. Both are chowned to OwnerUID:OwnerGID *before*
// rename; post-rename fchmod/fchown would race a reader.
func (c *CertFile) Write(_ context.Context, certPEM, keyPEM []byte) error {
	certTmp, err := c.stageTempFile(c.CertPath, certPEM, 0o444)
	if err != nil {
		return fmt.Errorf("write cert: %w", err)
	}
	keyTmp, err := c.stageTempFile(c.KeyPath, keyPEM, 0o400)
	if err != nil {
		_ = os.Remove(certTmp)
		return fmt.Errorf("write key: %w", err)
	}
	prevKey, prevKeyErr := os.ReadFile(c.KeyPath)
	if err := os.Rename(keyTmp, c.KeyPath); err != nil {
		_ = os.Remove(keyTmp)
		_ = os.Remove(certTmp)
		return fmt.Errorf("write key: %w", err)
	}
	if err := os.Rename(certTmp, c.CertPath); err != nil {
		_ = os.Remove(certTmp)
		if prevKeyErr == nil {
			_ = c.atomicWrite(c.KeyPath, prevKey, 0o400)
		} else if errors.Is(prevKeyErr, os.ErrNotExist) {
			_ = os.Remove(c.KeyPath)
		}
		return fmt.Errorf("write cert: %w", err)
	}
	return nil
}

// MarkRotated writes a small marker file next to the cert so that external
// reload watchers (e.g. nginx sidecars with inotifywait) can key off the
// final atomic rename rather than the intermediate tmpfile activity.
func (c *CertFile) MarkRotated(_ context.Context, at time.Time) error {
	dir := filepath.Dir(c.CertPath)
	marker := filepath.Join(dir, "rotated.marker")
	content := []byte(at.UTC().Format(time.RFC3339Nano) + "\n")
	return c.atomicWrite(marker, content, 0o444)
}

func (c *CertFile) atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmpName, err := c.stageTempFile(path, data, mode)
	if err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil { // #nosec G703 -- tmpName comes from os.CreateTemp in path's directory; path is the configured cert/key path, not request input
		_ = os.Remove(tmpName) // #nosec G703 -- tmpName comes from os.CreateTemp in the cert directory
		return err
	}
	return nil
}

func (c *CertFile) stageTempFile(path string, data []byte, mode os.FileMode) (string, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".pkiagent-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", err
	}
	// Only chown if explicitly requested; otherwise inherit the runtime
	// uid (lets the consumer's uid drive the perms via the sidecar
	// container). The decision is ChownRequested, not OwnerUID/OwnerGID's
	// value — 0 is a legitimate, explicitly configured owner (root).
	if c.ChownRequested {
		chown := c.chown
		if chown == nil {
			chown = os.Chown
		}
		if err := chown(tmpName, c.OwnerUID, c.OwnerGID); err != nil {
			_ = tmp.Close()
			cleanup()
			return "", err
		}
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", err
	}
	return tmpName, nil
}
