package migrate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PostgresBackuper handles pg_dump/pg_restore based backup operations
type PostgresBackuper struct {
	projectName string
	composeDir  string
	logger      *slog.Logger
	dryRun      bool
}

// NewPostgresBackuper creates a new PostgreSQL backuper. composeDir locates
// the compose files used to resolve the live container name for a service
// via "docker compose ps -q"; pass "" to always use the static fallback
// naming (e.g. in unit tests with no compose project running).
func NewPostgresBackuper(projectName, composeDir string, logger *slog.Logger, dryRun bool) *PostgresBackuper {
	return &PostgresBackuper{
		projectName: projectName,
		composeDir:  composeDir,
		logger:      logger,
		dryRun:      dryRun,
	}
}

// Backup creates a pg_dump backup of a PostgreSQL database
func (p *PostgresBackuper) Backup(ctx context.Context, spec VolumeSpec, outputPath string) error {
	if spec.BackupType != BackupTypePostgreSQL {
		return fmt.Errorf("volume %s is not a PostgreSQL volume", spec.Name)
	}

	container := p.resolveContainer(ctx, spec)

	p.logger.Info("backing up PostgreSQL database",
		"volume", spec.Name,
		"container", container,
		"database", spec.DBName,
		"output", outputPath,
	)

	if p.dryRun {
		p.logger.Info("[dry-run] would run pg_dump",
			"container", container,
			"database", spec.DBName,
		)
		return nil
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Run pg_dump inside the container, capture output to file
	// pg_dump -U <user> --format=custom --compress=6 <dbname>
	args := []string{
		"exec", container,
		"pg_dump",
		"-U", spec.DBUser,
		"--format=custom",
		"--compress=6",
		spec.DBName,
	}

	cmd := exec.CommandContext(ctx, "docker", args...)

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer outFile.Close()

	cmd.Stdout = outFile
	cmd.Stderr = &logWriter{logger: p.logger, prefix: "pg_dump stderr"}

	if err := cmd.Run(); err != nil {
		os.Remove(outputPath)
		return fmt.Errorf("pg_dump failed for %s: %w", spec.Name, err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("verifying dump file: %w", err)
	}

	if info.Size() == 0 {
		os.Remove(outputPath)
		return fmt.Errorf("pg_dump produced empty file for %s", spec.Name)
	}

	p.logger.Info("PostgreSQL backup complete",
		"volume", spec.Name,
		"size", info.Size(),
	)

	return nil
}

// Restore restores a pg_dump backup to a PostgreSQL database
func (p *PostgresBackuper) Restore(ctx context.Context, spec VolumeSpec, inputPath string) error {
	if spec.BackupType != BackupTypePostgreSQL {
		return fmt.Errorf("volume %s is not a PostgreSQL volume", spec.Name)
	}

	container := p.resolveContainer(ctx, spec)

	p.logger.Info("restoring PostgreSQL database",
		"volume", spec.Name,
		"container", container,
		"database", spec.DBName,
		"input", inputPath,
	)

	// Verify input file exists
	info, err := os.Stat(inputPath)
	if err != nil {
		return fmt.Errorf("input file not found: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("input file is empty")
	}

	if p.dryRun {
		p.logger.Info("[dry-run] would run pg_restore",
			"container", container,
			"database", spec.DBName,
		)
		return nil
	}

	// Run pg_restore inside the container
	// pg_restore -U <user> -d <dbname> --clean --if-exists --single-transaction
	// --single-transaction wraps the whole restore in one transaction so a
	// mid-restore failure rolls back cleanly instead of leaving the database
	// half-dropped/half-restored.
	args := []string{
		"exec", "-i", container,
		"pg_restore",
		"-U", spec.DBUser,
		"-d", spec.DBName,
		"--clean",
		"--if-exists",
		"--single-transaction",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)

	inFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("opening input file: %w", err)
	}
	defer inFile.Close()

	cmd.Stdin = inFile
	cmd.Stderr = &logWriter{logger: p.logger, prefix: "pg_restore stderr"}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_restore failed for %s: %w", spec.Name, err)
	}

	p.logger.Info("PostgreSQL restore complete",
		"volume", spec.Name,
	)

	return nil
}

// resolveContainer resolves the live container ID for spec.Service via
// "docker compose ps -q", so a custom project name or a scaled/renamed
// service doesn't silently target the wrong (or a nonexistent) container.
// Falls back to the static name map when compose files aren't configured,
// docker is unreachable, or nothing is running for that service — e.g. in
// unit tests, or when Backup/Restore is used outside the compose project.
func (p *PostgresBackuper) resolveContainer(ctx context.Context, spec VolumeSpec) string {
	files := composeFileList(p.composeDir)
	if len(files) == 0 {
		return p.containerName(spec)
	}

	args := []string{"compose"}
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			args = append(args, "-f", f)
		}
	}
	args = append(args, "ps", "-q", spec.Service)

	out, err := exec.CommandContext(ctx, "docker", args...).Output()
	id := strings.TrimSpace(string(out))
	if err != nil || id == "" {
		return p.containerName(spec)
	}
	return id
}

// containerName is the static fallback container name for a PostgreSQL
// service, used when the live container ID cannot be resolved.
func (p *PostgresBackuper) containerName(spec VolumeSpec) string {
	// Known container name mappings
	switch spec.Service {
	case "db":
		return "alt-db"
	case "kratos-db":
		return "alt-kratos-db-1"
	case "recap-db":
		return "recap-db"
	case "rag-db":
		return "rag-db"
	case "knowledge-sovereign-db":
		return "alt-knowledge-sovereign-db-1"
	case "pre-processor-db":
		return "pre-processor-db"
	default:
		return p.projectName + "-" + spec.Service
	}
}

// dumpFilename returns the dump filename for a volume
func (p *PostgresBackuper) dumpFilename(spec VolumeSpec) string {
	return spec.Name + ".dump"
}
