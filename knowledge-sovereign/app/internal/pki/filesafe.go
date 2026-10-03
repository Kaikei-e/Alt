package pki

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	maxPasswordBytes = 4 << 10
	maxEnvFileBytes  = 4 << 10
	maxRootPEMBytes  = 1 << 20
)

func assertTrustedParent(path string) error {
	if path == "" {
		return errors.New("pki: empty path")
	}
	dir := filepath.Dir(path)
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("pki: lstat parent of %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("pki: parent directory of %q is a symlink", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("pki: parent of %q is not a directory", path)
	}
	return nil
}

func assertTrustedDest(path string) error {
	if err := assertTrustedParent(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("pki: lstat %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("pki: %q is a symlink", path)
	}
	return nil
}

func readRegularNoFollow(path string, maxBytes int64) ([]byte, error) {
	if err := assertTrustedParent(path); err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if base == "." || base == ".." || strings.ContainsRune(base, filepath.Separator) {
		return nil, fmt.Errorf("pki: invalid basename for %q", path)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("pki: open parent of %q: %w", path, err)
	}
	defer func() { _ = root.Close() }()
	f, err := root.OpenFile(base, os.O_RDONLY|syscall.O_NOFOLLOW, 0) // #nosec G304 -- path from typed Config
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, fmt.Errorf("pki: %q is a symlink", path)
		}
		return nil, fmt.Errorf("pki: open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("pki: fstat %q: %w", path, err)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("pki: %q is not a regular file", path)
	}
	if st.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("pki: %q has insecure permission bits", path)
	}
	if st.Size() > maxBytes {
		return nil, fmt.Errorf("pki: %q exceeds %d-byte cap", path, maxBytes)
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("pki: read %q: %w", path, err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("pki: %q exceeds %d-byte cap", path, maxBytes)
	}
	return raw, nil
}

func isNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOENT)
}
