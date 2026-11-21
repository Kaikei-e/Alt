package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	generatedDirMode  os.FileMode = 0o755
	generatedFileMode os.FileMode = 0o644
)

func writeArtifact(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), generatedDirMode); err != nil {
		return fmt.Errorf("create output directory for %q: %w", path, err)
	}
	if err := os.WriteFile(path, content, generatedFileMode); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}
