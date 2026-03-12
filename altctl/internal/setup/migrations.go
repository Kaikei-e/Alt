package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// MigrationDir defines an Atlas migration directory to hash
type MigrationDir struct {
	Name string // human-readable name
	Path string // relative path from project root to migrations dir
}

// DefaultMigrationDirs returns the Atlas migration directories
func DefaultMigrationDirs() []MigrationDir {
	return []MigrationDir{
		{Name: "main DB", Path: "migrations-atlas/migrations"},
		{Name: "recap DB", Path: "recap-migration-atlas/migrations"},
		{Name: "RAG DB", Path: "rag-migration-atlas/migrations"},
	}
}

// RegenerateAtlasHash runs atlas migrate hash for the given migration directory.
// Uses Docker to run the Atlas CLI to avoid requiring a local Atlas install.
func RegenerateAtlasHash(projectRoot string, dir MigrationDir) error {
	absPath := filepath.Join(projectRoot, dir.Path)
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("migration directory not found: %s", dir.Path)
	}

	cmd := exec.Command("docker", "run", "--rm",
		"-v", absPath+":/migrations:rw",
		"--user", "0:0",
		"--entrypoint", "atlas",
		"arigaio/atlas:latest-alpine",
		"migrate", "hash",
		"--dir", "file:///migrations",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("atlas migrate hash failed for %s: %w", dir.Name, err)
	}
	return nil
}
