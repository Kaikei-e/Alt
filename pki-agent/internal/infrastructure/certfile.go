// Package infrastructure contains side-effecting implementations: disk I/O,
// HTTP clients, OTT signing. Keep all OS / crypto / net calls here so the
// usecase layer stays pure.
package infrastructure

import (
	"context"
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
	return cert, nil
}

// Write atomically replaces both cert and key. The cert is written as
// 0444, the key as 0400. Both are chowned to OwnerUID:OwnerGID *before*
// rename; post-rename fchmod/fchown would race a reader.
func (c *CertFile) Write(_ context.Context, certPEM, keyPEM []byte) error {
	if err := c.atomicWrite(c.CertPath, certPEM, 0o444); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}
	if err := c.atomicWrite(c.KeyPath, keyPEM, 0o400); err != nil {
		return fmt.Errorf("write key: %w", err)
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
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".pkiagent-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	// Only chown if explicitly configured; otherwise inherit the runtime
	// uid (lets the consumer's uid drive the perms via the sidecar container).
	if c.OwnerUID > 0 && c.OwnerGID > 0 {
		if err := os.Chown(tmpName, c.OwnerUID, c.OwnerGID); err != nil {
			_ = tmp.Close()
			cleanup()
			return err
		}
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
