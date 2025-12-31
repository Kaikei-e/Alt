package migrate

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// RestoreOptions configures the restore operation
type RestoreOptions struct {
	BackupDir string // Path to backup directory
	Force     bool   // Force restore without confirmation
	Verify    bool   // Verify backup integrity before restore
}

// Restore performs a full restore from a backup
func (m *Migrator) Restore(ctx context.Context, opts RestoreOptions) error {
	// Load and validate manifest
	manifestPath := filepath.Join(opts.BackupDir, ManifestFilename)
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	m.logger.Info("starting restore",
		"backup_dir", opts.BackupDir,
		"backup_date", manifest.CreatedAt,
		"volumes", len(manifest.Volumes),
	)

	// Verify backup integrity if requested
	if opts.Verify {
		m.logger.Info("verifying backup integrity...")
		if err := manifest.Verify(opts.BackupDir); err != nil {
			return fmt.Errorf("backup verification failed: %w", err)
		}
		m.logger.Info("backup integrity verified")
	}

	// Check if any containers are running
	running, err := m.getRunningContainers(ctx)
	if err != nil {
		return fmt.Errorf("checking running containers: %w", err)
	}

	if len(running) > 0 && !opts.Force {
		return fmt.Errorf("containers are running: %v. Stop them first or use --force", running)
	}

	if len(running) > 0 && opts.Force {
		m.logger.Warn("stopping running containers for restore")
		if err := m.stopContainers(ctx); err != nil {
			return fmt.Errorf("stopping containers: %w", err)
		}
	}

	// Restore all volumes using tar
	for _, vb := range manifest.Volumes {
		spec, ok := m.registry.Get(vb.Name)
		if !ok {
			m.logger.Warn("unknown volume in backup, skipping",
				"volume", vb.Name,
			)
			continue
		}

		if err := m.restoreVolume(ctx, spec, opts.BackupDir, vb); err != nil {
			m.logger.Error("volume restore failed",
				"volume", vb.Name,
				"error", err,
			)
			// Continue with other volumes
			continue
		}
	}

	m.logger.Info("restore complete")
	return nil
}

// restoreVolume restores a single volume using tar
func (m *Migrator) restoreVolume(ctx context.Context, spec VolumeSpec, backupDir string, vb VolumeBackup) error {
	inputPath := filepath.Join(backupDir, vb.Filename)

	m.logger.Info("restoring volume",
		"volume", spec.Name,
	)

	return m.volumeBackup.Restore(ctx, spec, inputPath)
}

// stopContainers stops all project containers
func (m *Migrator) stopContainers(ctx context.Context) error {
	if m.dryRun {
		m.logger.Info("[dry-run] would stop all containers")
		return nil
	}

	args := m.buildComposeArgs("down")
	cmd := exec.CommandContext(ctx, "docker", args...)
	return cmd.Run()
}

// buildComposeArgs builds docker compose command arguments
func (m *Migrator) buildComposeArgs(args ...string) []string {
	composeFiles := []string{
		filepath.Join(m.composeDir, "base.yaml"),
		filepath.Join(m.composeDir, "db.yaml"),
		filepath.Join(m.composeDir, "auth.yaml"),
		filepath.Join(m.composeDir, "core.yaml"),
		filepath.Join(m.composeDir, "workers.yaml"),
		filepath.Join(m.composeDir, "recap.yaml"),
		filepath.Join(m.composeDir, "rag.yaml"),
	}

	result := []string{"compose"}
	for _, f := range composeFiles {
		result = append(result, "-f", f)
	}
	result = append(result, args...)
	return result
}

// VerifyBackup verifies the integrity of a backup
func VerifyBackup(backupDir string) (*Manifest, error) {
	manifestPath := filepath.Join(backupDir, ManifestFilename)
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}

	if err := manifest.Verify(backupDir); err != nil {
		return nil, err
	}

	return manifest, nil
}

// GetBackupSummary returns a summary of a backup
func GetBackupSummary(backupDir string) (string, error) {
	manifest, err := LoadManifest(filepath.Join(backupDir, ManifestFilename))
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Backup: %s\n", filepath.Base(backupDir)))
	sb.WriteString(fmt.Sprintf("Created: %s\n", manifest.CreatedAt.Format("2006-01-02 15:04:05 MST")))
	sb.WriteString(fmt.Sprintf("Altctl Version: %s\n", manifest.AltctlVersion))
	sb.WriteString(fmt.Sprintf("Volumes: %d\n\n", len(manifest.Volumes)))

	var totalSize int64
	for _, v := range manifest.Volumes {
		totalSize += v.Size
		sb.WriteString(fmt.Sprintf("  %-30s %10s  %s\n",
			v.Name,
			FormatSize(v.Size),
			v.TypeString,
		))
	}

	sb.WriteString(fmt.Sprintf("\nTotal Size: %s\n", FormatSize(totalSize)))

	return sb.String(), nil
}
