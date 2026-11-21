package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeArtifact(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create output directory for %q: %w", path, err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}
