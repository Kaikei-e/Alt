package migrate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

// PostgresBackuper handles pg_dump/pg_restore based backup operations
type PostgresBackuper struct {
	projectName string
	logger      *slog.Logger
	dryRun      bool
}

// NewPostgresBackuper creates a new PostgreSQL backuper
func NewPostgresBackuper(projectName string, logger *slog.Logger, dryRun bool) *PostgresBackuper {
	return &PostgresBackuper{
		projectName: projectName,
		logger:      logger,
		dryRun:      dryRun,
	}
}

// Backup creates a pg_dump backup of a PostgreSQL database
func (p *PostgresBackuper) Backup(ctx context.Context, spec VolumeSpec, outputPath string) error {
	if spec.BackupType != BackupTypePostgreSQL {
		return fmt.Errorf("volume %s is not a PostgreSQL volume", spec.Name)
	}

	container := p.containerName(spec)

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

	container := p.containerName(spec)

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
	// pg_restore -U <user> -d <dbname> --clean --if-exists
	args := []string{
		"exec", "-i", container,
		"pg_restore",
		"-U", spec.DBUser,
		"-d", spec.DBName,
		"--clean",
		"--if-exists",
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

// containerName resolves the Docker container name for a PostgreSQL service
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
	default:
		return p.projectName + "-" + spec.Service
	}
}

// dumpFilename returns the dump filename for a volume
func (p *PostgresBackuper) dumpFilename(spec VolumeSpec) string {
	return spec.Name + ".dump"
}
