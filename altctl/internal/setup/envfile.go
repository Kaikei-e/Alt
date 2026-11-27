package setup

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/alt-project/altctl/internal/fsperm"
)

const (
	envExampleFilename = ".env.example"
	envFilename        = ".env"
)

// CreateEnvFile copies .env.example to .env in the given project root.
// Returns true if the file was created or overwritten. Existing regular .env
// files are skipped unless force is true. Symlinks and non-regular .env targets
// are always rejected, and written files are locked down to fsperm.SecretFile.
func CreateEnvFile(projectRoot string, force bool) (bool, error) {
	root, err := os.OpenRoot(projectRoot)
	if err != nil {
		return false, fmt.Errorf("opening project root %s: %w", projectRoot, err)
	}
	defer func() { _ = root.Close() }()

	content, err := readEnvExample(root, projectRoot)
	if err != nil {
		return false, err
	}

	info, err := root.Lstat(envFilename)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("refusing to write %s: symlink is not allowed", envFilename)
		}
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("refusing to write %s: existing path is not a regular file", envFilename)
		}
		if !force {
			return false, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspecting %s: %w", envFilename, err)
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	f, err := root.OpenFile(envFilename, flags, fsperm.SecretFile)
	if err != nil {
		return false, fmt.Errorf("opening %s: %w", envFilename, err)
	}
	openInfo, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return false, fmt.Errorf("stat %s: %w", envFilename, err)
	}
	if !openInfo.Mode().IsRegular() {
		_ = f.Close()
		return false, fmt.Errorf("refusing to write %s: opened path is not a regular file", envFilename)
	}
	if err := f.Chmod(fsperm.SecretFile); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("setting %s permissions: %w", envFilename, err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("writing %s: %w", envFilename, err)
	}
	if err := f.Close(); err != nil {
		return false, fmt.Errorf("closing %s: %w", envFilename, err)
	}

	return true, nil
}

func readEnvExample(root *os.Root, projectRoot string) ([]byte, error) {
	f, err := root.Open(envExampleFilename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s not found in %s: %w", envExampleFilename, projectRoot, err)
		}
		return nil, fmt.Errorf("reading %s: %w", envExampleFilename, err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", envExampleFilename, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", envExampleFilename)
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", envExampleFilename, err)
	}

	return content, nil
}
