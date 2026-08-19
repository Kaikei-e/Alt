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
	"strings"
)

var (
	// ErrCertNotFound: cert file absent from disk.
	ErrCertNotFound = errors.New("pki: cert not found")
	// ErrCertParseFailed: file present but not a valid X.509 PEM.
	ErrCertParseFailed = errors.New("pki: cert parse failed")
	// ErrCertPairMismatch: cert and key on disk do not form a usable pair.
	ErrCertPairMismatch = errors.New("pki: cert/key pair mismatch")
	errSimulatedCrash   = errors.New("pki: injected crash after key rename")
)

const (
	certDirPerm         os.FileMode = 0o750
	installJournalName              = ".pki-install.journal"
	journalInProgress               = "in-progress\n"
	journalNeedsReissue             = "needs-reissue\n"
)

// CertFile reads and transactionally replaces a PEM leaf + key pair.
type CertFile struct {
	CertPath string
	KeyPath  string

	afterKeyRename func() error
	renameFile     func(oldpath, newpath string) error
}

func (c *CertFile) journalPath() string {
	return filepath.Join(filepath.Dir(c.KeyPath), installJournalName)
}

func (c *CertFile) prevKeyPath() string {
	return c.KeyPath + ".prev"
}

func (c *CertFile) doRename(oldpath, newpath string) error {
	if c.renameFile != nil {
		return c.renameFile(oldpath, newpath)
	}
	return os.Rename(oldpath, newpath) // #nosec G703 -- atomic install of our temp
}

// Load recovers a crashed install if needed, then returns the parsed leaf.
// A leftover journal that is not a verified pair is never classified Fresh.
func (c *CertFile) Load(ctx context.Context) (*x509.Certificate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := c.recoverInstall(); err != nil {
		return nil, err
	}
	raw, err := readRegularNoFollow(c.CertPath, maxRootPEMBytes)
	if err != nil {
		if isNotExist(err) {
			return nil, ErrCertNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrCertParseFailed, err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("%w: no PEM block", ErrCertParseFailed)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCertParseFailed, err)
	}
	if err := c.requirePair(); err != nil {
		return nil, err
	}
	return cert, nil
}

func (c *CertFile) requirePair() error {
	certPEM, err := readRegularNoFollow(c.CertPath, maxRootPEMBytes)
	if err != nil {
		if isNotExist(err) {
			return ErrCertNotFound
		}
		return fmt.Errorf("%w: %v", ErrCertPairMismatch, err)
	}
	keyPEM, err := readRegularNoFollow(c.KeyPath, maxRootPEMBytes)
	if err != nil {
		if isNotExist(err) {
			return fmt.Errorf("%w: missing key", ErrCertPairMismatch)
		}
		return fmt.Errorf("%w: %v", ErrCertPairMismatch, err)
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return fmt.Errorf("%w: %v", ErrCertPairMismatch, err)
	}
	return nil
}

func (c *CertFile) recoverInstall() error {
	jp := c.journalPath()
	raw, err := os.ReadFile(jp) // #nosec G304 -- journal next to typed KeyPath
	if err != nil {
		if isNotExist(err) {
			return nil
		}
		return fmt.Errorf("pki: journal: %w", err)
	}
	if strings.TrimSpace(string(raw)) == strings.TrimSpace(journalNeedsReissue) {
		return ErrCertPairMismatch
	}
	if err := c.requirePair(); err == nil {
		_ = os.Remove(jp)
		_ = os.Remove(c.prevKeyPath())
		return nil
	}
	if _, err := os.Lstat(c.prevKeyPath()); err == nil {
		_ = c.doRename(c.prevKeyPath(), c.KeyPath)
	}
	if err := c.writeJournal(journalNeedsReissue); err != nil {
		return err
	}
	return ErrCertPairMismatch
}

func (c *CertFile) writeJournal(contents string) error {
	path := c.journalPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- journal next to typed KeyPath
	if err != nil {
		return fmt.Errorf("pki: journal: %w", err)
	}
	if _, err := f.WriteString(contents); err != nil {
		_ = f.Close()
		return fmt.Errorf("pki: journal: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("pki: journal: %w", err)
	}
	return f.Close()
}

func (c *CertFile) backupCurrentKey() error {
	if _, err := os.Lstat(c.KeyPath); err != nil {
		if isNotExist(err) {
			return nil
		}
		return err
	}
	raw, err := readRegularNoFollow(c.KeyPath, maxRootPEMBytes)
	if err != nil {
		return err
	}
	tmp, err := writeTemp(c.prevKeyPath(), raw, 0o400)
	if err != nil {
		return err
	}
	return c.doRename(tmp, c.prevKeyPath())
}

// Write stages both PEMs, journals the swap, then renames key then cert.
// A failed second rename restores the previous key. A crash between renames
// leaves a journal so Load cannot classify the mismatched pair as Fresh.
// tlsutil's GetCertificate keeps the last good pair when LoadX509KeyPair fails.
func (c *CertFile) Write(ctx context.Context, certPEM, keyPEM []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("pki: write: %w", err)
	}
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
	certTmp, err := writeTemp(c.CertPath, certPEM, 0o444)
	if err != nil {
		return fmt.Errorf("pki: write cert tmp: %w", err)
	}
	defer func() { _ = os.Remove(certTmp) }() // #nosec G703 -- temp created in CertPath dir
	keyTmp, err := writeTemp(c.KeyPath, keyPEM, 0o400)
	if err != nil {
		return fmt.Errorf("pki: write key tmp: %w", err)
	}
	defer func() { _ = os.Remove(keyTmp) }() // #nosec G703 -- temp created in KeyPath dir
	if err := c.backupCurrentKey(); err != nil {
		return fmt.Errorf("pki: backup key: %w", err)
	}
	if err := c.writeJournal(journalInProgress); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(c.journalPath())
		_ = os.Remove(c.prevKeyPath())
		return fmt.Errorf("pki: write: %w", err)
	}
	if err := c.doRename(keyTmp, c.KeyPath); err != nil {
		_ = os.Remove(c.journalPath())
		_ = os.Remove(c.prevKeyPath())
		return fmt.Errorf("pki: rename key: %w", err)
	}
	if c.afterKeyRename != nil {
		if err := c.afterKeyRename(); err != nil {
			if errors.Is(err, errSimulatedCrash) {
				// Process died between renames: leave journal + mismatched pair
				// so Load cannot classify Fresh. Reloader keeps last-good in memory.
				return err
			}
			_ = c.restorePrevKey()
			_ = os.Remove(c.journalPath())
			_ = os.Remove(c.prevKeyPath())
			return err
		}
	}
	if err := c.doRename(certTmp, c.CertPath); err != nil {
		_ = c.restorePrevKey()
		_ = os.Remove(c.journalPath())
		return fmt.Errorf("pki: rename cert: %w", err)
	}
	if err := c.requirePair(); err != nil {
		_ = c.restorePrevKey()
		_ = os.Remove(c.journalPath())
		return err
	}
	_ = os.Remove(c.journalPath())
	_ = os.Remove(c.prevKeyPath())
	return nil
}

func (c *CertFile) restorePrevKey() error {
	if _, err := os.Lstat(c.prevKeyPath()); err != nil {
		if isNotExist(err) {
			return nil
		}
		return err
	}
	return c.doRename(c.prevKeyPath(), c.KeyPath)
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
