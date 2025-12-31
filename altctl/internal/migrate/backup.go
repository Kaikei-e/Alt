package migrate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BackupOptions configures the backup operation
type BackupOptions struct {
	OutputDir     string // Base output directory
	Force         bool   // Force backup even if containers are running
	AltctlVersion string // Version string for manifest
}

// Migrator orchestrates backup and restore operations
type Migrator struct {
	registry     *VolumeRegistry
	volumeBackup *VolumeBackuper
	composeDir   string
	projectName  string
	logger       *slog.Logger
	dryRun       bool
}

// NewMigrator creates a new migrator instance
func NewMigrator(composeDir, projectName string, logger *slog.Logger, dryRun bool) *Migrator {
	return &Migrator{
		registry:     NewVolumeRegistry(),
		volumeBackup: NewVolumeBackuper(projectName, logger, dryRun),
		composeDir:   composeDir,
		projectName:  projectName,
		logger:       logger,
		dryRun:       dryRun,
	}
}

// Backup performs a full backup of all registered volumes
func (m *Migrator) Backup(ctx context.Context, opts BackupOptions) (*Manifest, error) {
	// Check if any containers are running (for data consistency)
	running, err := m.getRunningContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("checking running containers: %w", err)
	}

	if len(running) > 0 && !opts.Force {
		return nil, fmt.Errorf("containers are running: %v. Use --force to backup anyway (may cause inconsistent data)", running)
	}

	if len(running) > 0 {
		m.logger.Warn("backing up with running containers - data may be inconsistent",
			"running", running,
		)
	}

	// Create backup directory with timestamp
	backupDir := BackupDir(opts.OutputDir)
	if !m.dryRun {
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return nil, fmt.Errorf("creating backup directory: %w", err)
		}
	}

	m.logger.Info("starting backup",
		"output_dir", backupDir,
		"volumes", len(m.registry.All()),
	)

	// Create manifest
	manifest := NewManifest(opts.AltctlVersion)

	// Create volumes subdirectory
	volumesDir := filepath.Join(backupDir, "volumes")

	if !m.dryRun {
		if err := os.MkdirAll(volumesDir, 0755); err != nil {
			return nil, fmt.Errorf("creating volumes directory: %w", err)
		}
	}

	// Backup all volumes using tar (requires containers to be stopped)
	for _, spec := range m.registry.All() {
		if err := m.backupVolume(ctx, spec, volumesDir, manifest); err != nil {
			m.logger.Error("volume backup failed",
				"volume", spec.Name,
				"error", err,
			)
			// Continue with other volumes
			continue
		}
	}

	// Finalize and save manifest
	manifest.Finalize()

	if !m.dryRun {
		manifestPath := filepath.Join(backupDir, ManifestFilename)
		if err := manifest.Save(manifestPath); err != nil {
			return nil, fmt.Errorf("saving manifest: %w", err)
		}
	}

	m.logger.Info("backup complete",
		"output_dir", backupDir,
		"volumes_backed_up", len(manifest.Volumes),
	)

	return manifest, nil
}

// backupVolume backs up a single volume using tar
func (m *Migrator) backupVolume(ctx context.Context, spec VolumeSpec, outputDir string, manifest *Manifest) error {
	filename := spec.Name + ".tar.gz"
	outputPath := filepath.Join(outputDir, filename)

	startTime := time.Now()
	if err := m.volumeBackup.Backup(ctx, spec, outputPath); err != nil {
		return err
	}

	if m.dryRun {
		manifest.AddVolume(VolumeBackup{
			Name:       spec.Name,
			Type:       BackupTypeTar,
			Filename:   filepath.Join("volumes", filename),
			Size:       0,
			Checksum:   "sha256:dry-run",
			Service:    spec.Service,
			BackedUpAt: startTime,
		})
		return nil
	}

	// Get file info for manifest
	info, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("getting file info: %w", err)
	}

	checksum, err := FileChecksum(outputPath)
	if err != nil {
		return fmt.Errorf("calculating checksum: %w", err)
	}

	manifest.AddVolume(VolumeBackup{
		Name:       spec.Name,
		Type:       BackupTypeTar,
		Filename:   filepath.Join("volumes", filename),
		Size:       info.Size(),
		Checksum:   checksum,
		Service:    spec.Service,
		BackedUpAt: startTime,
	})

	return nil
}

// getRunningContainers returns a list of running containers for this project
func (m *Migrator) getRunningContainers(ctx context.Context) ([]string, error) {
	// Build compose file arguments
	args := []string{"compose"}
	composeFiles := []string{
		filepath.Join(m.composeDir, "base.yaml"),
		filepath.Join(m.composeDir, "db.yaml"),
		filepath.Join(m.composeDir, "auth.yaml"),
		filepath.Join(m.composeDir, "core.yaml"),
		filepath.Join(m.composeDir, "workers.yaml"),
		filepath.Join(m.composeDir, "recap.yaml"),
		filepath.Join(m.composeDir, "rag.yaml"),
		filepath.Join(m.composeDir, "logging.yaml"),
		filepath.Join(m.composeDir, "ai.yaml"),
	}

	for _, f := range composeFiles {
		if _, err := os.Stat(f); err == nil {
			args = append(args, "-f", f)
		}
	}

	args = append(args, "ps", "-q")

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.Output()
	if err != nil {
		// No containers running is not an error
		return nil, nil
	}

	var running []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line != "" {
			running = append(running, line)
		}
	}

	return running, nil
}

// ListBackups returns a list of available backups in the given directory
func ListBackups(baseDir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(baseDir, entry.Name(), ManifestFilename)
		manifest, err := LoadManifest(manifestPath)
		if err != nil {
			// Not a valid backup directory
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		var totalSize int64
		for _, v := range manifest.Volumes {
			totalSize += v.Size
		}

		backups = append(backups, BackupInfo{
			Name:        entry.Name(),
			Path:        filepath.Join(baseDir, entry.Name()),
			CreatedAt:   manifest.CreatedAt,
			ModTime:     info.ModTime(),
			VolumeCount: len(manifest.Volumes),
			TotalSize:   totalSize,
			Manifest:    manifest,
		})
	}

	return backups, nil
}

// BackupInfo contains information about a backup
type BackupInfo struct {
	Name        string
	Path        string
	CreatedAt   time.Time
	ModTime     time.Time
	VolumeCount int
	TotalSize   int64
	Manifest    *Manifest
}

// FormatSize formats bytes as human-readable string
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
