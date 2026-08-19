package pki

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var (
	// ErrCertNotFound: cert file absent from disk.
	ErrCertNotFound = errors.New("pki: cert not found")
	// ErrCertParseFailed: file present but not a valid X.509 PEM or cert/key pair.
	ErrCertParseFailed = errors.New("pki: cert parse failed")
)

const (
	certDirPerm     os.FileMode = 0o750
	pairJournalName             = ".pki-pair-journal"
)

// renameFile is os.Rename; tests inject failures between key and cert install.
var renameFile = os.Rename

// CertFile reads and atomically replaces a PEM leaf + key pair.
type CertFile struct {
	CertPath string
	KeyPath  string
}

func (c *CertFile) journalPath() string {
	return filepath.Join(filepath.Dir(c.KeyPath), pairJournalName)
}

// Load reads the cert/key pair, restores a previous key from the install
// journal if a crash left a mismatch, and rejects an unmatched pair.
func (c *CertFile) Load(_ context.Context) (*x509.Certificate, error) {
	if err := c.recoverInstall(); err != nil {
		return nil, err
	}
	return c.loadPair()
}

func (c *CertFile) loadPair() (*x509.Certificate, error) {
	raw, err := readRegularNoFollow(c.CertPath, maxRootPEMBytes)
	if err != nil {
		if isNotExist(err) {
			return nil, ErrCertNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrCertParseFailed, err)
	}
	keyPEM, err := readRegularNoFollow(c.KeyPath, maxRootPEMBytes)
	if err != nil {
		if isNotExist(err) {
			return nil, fmt.Errorf("%w: key missing", ErrCertParseFailed)
		}
		return nil, fmt.Errorf("%w: %v", ErrCertParseFailed, err)
	}
	if _, err := tls.X509KeyPair(raw, keyPEM); err != nil {
		return nil, fmt.Errorf("%w: cert/key pair mismatch", ErrCertParseFailed)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("%w: no PEM block", ErrCertParseFailed)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCertParseFailed, err)
	}
	return cert, nil
}

func (c *CertFile) recoverInstall() error {
	journalPEM, err := readRegularNoFollow(c.journalPath(), maxRootPEMBytes)
	if err != nil {
		if isNotExist(err) {
			return nil
		}
		return fmt.Errorf("%w: read pair journal: %v", ErrCertParseFailed, err)
	}
	certPEM, certErr := readRegularNoFollow(c.CertPath, maxRootPEMBytes)
	keyPEM, keyErr := readRegularNoFollow(c.KeyPath, maxRootPEMBytes)
	if certErr == nil && keyErr == nil {
		if _, pairErr := tls.X509KeyPair(certPEM, keyPEM); pairErr == nil {
			_ = os.Remove(c.journalPath())
			return nil
		}
	}
	if err := c.installKeyPEM(journalPEM); err != nil {
		return fmt.Errorf("%w: restore pair journal: %v", ErrCertParseFailed, err)
	}
	_ = os.Remove(c.journalPath())
	return nil
}

func (c *CertFile) installKeyPEM(keyPEM []byte) error {
	if err := assertTrustedDest(c.KeyPath); err != nil {
		return err
	}
	tmp, err := writeTemp(c.KeyPath, keyPEM, 0o400)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }()
	return renameFile(tmp, c.KeyPath)
}

func isNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOENT)
}

// Write stages both PEMs as sibling temps, journals the previous key, then
// renames key then cert. A cert-rename failure restores the journaled key.
func (c *CertFile) Write(_ context.Context, certPEM, keyPEM []byte) error {
	if err := os.MkdirAll(filepath.Dir(c.CertPath), certDirPerm); err != nil {
		return fmt.Errorf("pki: mkdir cert dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(c.KeyPath), certDirPerm); err != nil {
		return fmt.Errorf("pki: mkdir key dir: %w", err)
	}
	if err := assertTrustedDest(c.CertPath); err != nil {
		return fmt.Errorf("pki: cert dest: %w", err)
	}
	if err := assertTrustedDest(c.KeyPath); err != nil {
		return fmt.Errorf("pki: key dest: %w", err)
	}
	if err := assertTrustedDest(c.journalPath()); err != nil {
		return fmt.Errorf("pki: pair journal dest: %w", err)
	}
	certTmp, err := writeTemp(c.CertPath, certPEM, 0o444)
	if err != nil {
		return fmt.Errorf("pki: write cert tmp: %w", err)
	}
	defer func() { _ = os.Remove(certTmp) }()
	keyTmp, err := writeTemp(c.KeyPath, keyPEM, 0o400)
	if err != nil {
		return fmt.Errorf("pki: write key tmp: %w", err)
	}
	defer func() { _ = os.Remove(keyTmp) }()
	if err := c.journalExistingKey(); err != nil {
		return err
	}
	if err := renameFile(keyTmp, c.KeyPath); err != nil {
		return fmt.Errorf("pki: rename key: %w", err)
	}
	if err := renameFile(certTmp, c.CertPath); err != nil {
		if rerr := c.restoreJournaledKey(); rerr != nil {
			return fmt.Errorf("pki: rename cert: %w (rollback: %v)", err, rerr)
		}
		return fmt.Errorf("pki: rename cert: %w", err)
	}
	certOnDisk, err := readRegularNoFollow(c.CertPath, maxRootPEMBytes)
	if err != nil {
		_ = c.restoreJournaledKey()
		return fmt.Errorf("pki: read installed cert: %w", err)
	}
	keyOnDisk, err := readRegularNoFollow(c.KeyPath, maxRootPEMBytes)
	if err != nil {
		_ = c.restoreJournaledKey()
		return fmt.Errorf("pki: read installed key: %w", err)
	}
	if _, err := tls.X509KeyPair(certOnDisk, keyOnDisk); err != nil {
		if rerr := c.restoreJournaledKey(); rerr != nil {
			return fmt.Errorf("pki: installed cert/key pair mismatch: %w (rollback: %v)", err, rerr)
		}
		return fmt.Errorf("pki: installed cert/key pair mismatch: %w", err)
	}
	_ = os.Remove(c.journalPath())
	return nil
}

func (c *CertFile) journalExistingKey() error {
	prev, err := readRegularNoFollow(c.KeyPath, maxRootPEMBytes)
	if err != nil {
		if isNotExist(err) {
			return nil
		}
		return fmt.Errorf("pki: read current key for journal: %w", err)
	}
	tmp, err := writeTemp(c.journalPath(), prev, 0o400)
	if err != nil {
		return fmt.Errorf("pki: write pair journal: %w", err)
	}
	if err := renameFile(tmp, c.journalPath()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("pki: rename pair journal: %w", err)
	}
	return nil
}

func (c *CertFile) restoreJournaledKey() error {
	prev, err := readRegularNoFollow(c.journalPath(), maxRootPEMBytes)
	if err != nil {
		return err
	}
	return c.installKeyPEM(prev)
}

func writeTemp(finalPath string, data []byte, mode os.FileMode) (string, error) {
	dir := filepath.Dir(finalPath)
	tmp, err := os.CreateTemp(dir, ".pki-enroll-*")
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
