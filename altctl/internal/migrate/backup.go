package migrate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// BackupOptions configures the backup operation
type BackupOptions struct {
	OutputDir     string        // Base output directory
	Force         bool          // Force backup even if containers are running
	AltctlVersion string        // Version string for manifest
	Profile       BackupProfile // Backup profile (db, essential, all)
	Include       []string      // Only include these volume names
	Exclude       []string      // Exclude these volume names
	Concurrency   int           // Max parallel pg_dump operations (default: 4)
}

// BackupResult contains the outcome of a backup operation
type BackupResult struct {
	Manifest      *Manifest
	Elapsed       time.Duration
	VolumeTimings []VolumeTiming
}

// VolumeTiming records per-volume backup timing
type VolumeTiming struct {
	Name    string
	Elapsed time.Duration
	Size    int64
	Error   error
}

// defaultConcurrency is the default number of parallel pg_dump operations
const defaultConcurrency = 4

// Migrator orchestrates backup and restore operations
type Migrator struct {
	registry     *VolumeRegistry
	volumeBackup *VolumeBackuper
	pgBackup     *PostgresBackuper
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
		pgBackup:     NewPostgresBackuper(projectName, logger, dryRun),
		composeDir:   composeDir,
		projectName:  projectName,
		logger:       logger,
		dryRun:       dryRun,
	}
}

// Backup performs a backup of volumes filtered by profile and include/exclude options
func (m *Migrator) Backup(ctx context.Context, opts BackupOptions) (*BackupResult, error) {
	totalStart := time.Now()

	// Default to ProfileAll for backward compatibility when no profile specified
	profile := opts.Profile
	if profile == "" {
		profile = ProfileAll
	}

	// Resolve volumes based on profile and filters
	volumes, err := ResolveVolumes(m.registry, profile, opts.Include, opts.Exclude)
	if err != nil {
		return nil, fmt.Errorf("resolving volumes: %w", err)
	}

	if len(volumes) == 0 {
		return nil, fmt.Errorf("no volumes selected for backup (profile=%s)", profile)
	}

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
		"profile", string(profile),
		"volumes", len(volumes),
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

	// Separate PG and tar volumes
	var pgVolumes, tarVolumes []VolumeSpec
	for _, spec := range volumes {
		if spec.BackupType == BackupTypePostgreSQL {
			pgVolumes = append(pgVolumes, spec)
		} else {
			tarVolumes = append(tarVolumes, spec)
		}
	}

	var allTimings []VolumeTiming
	var timingsMu sync.Mutex

	// Back up PG volumes in parallel
	if len(pgVolumes) > 0 {
		concurrency := opts.Concurrency
		if concurrency <= 0 {
			concurrency = defaultConcurrency
		}

		m.logger.Info("backing up PostgreSQL databases",
			"count", len(pgVolumes),
			"concurrency", concurrency,
		)

		g, gCtx := errgroup.WithContext(ctx)
		g.SetLimit(concurrency)

		for _, spec := range pgVolumes {
			g.Go(func() error {
				timing := m.backupVolumeWithTiming(gCtx, spec, volumesDir)
				timingsMu.Lock()
				allTimings = append(allTimings, timing)
				timingsMu.Unlock()
				return nil // don't fail the group; individual errors are tracked in timing
			})
		}

		if err := g.Wait(); err != nil {
			return nil, fmt.Errorf("parallel pg_dump: %w", err)
		}
	}

	// Back up tar volumes sequentially
	if len(tarVolumes) > 0 {
		m.logger.Info("backing up tar volumes",
			"count", len(tarVolumes),
		)

		for _, spec := range tarVolumes {
			timing := m.backupVolumeWithTiming(ctx, spec, volumesDir)
			allTimings = append(allTimings, timing)
		}
	}

	// Build manifest from timings
	for _, timing := range allTimings {
		if timing.Error != nil {
			m.logger.Error("volume backup failed",
				"volume", timing.Name,
				"error", timing.Error,
			)
			continue
		}

		spec, _ := m.registry.Get(timing.Name)
		var filename string
		switch spec.BackupType {
		case BackupTypePostgreSQL:
			filename = spec.Name + ".dump"
		default:
			filename = spec.Name + ".tar.gz"
		}

		vb := VolumeBackup{
			Name:       timing.Name,
			Type:       spec.BackupType,
			Filename:   filepath.Join("volumes", filename),
			Size:       timing.Size,
			Service:    spec.Service,
			BackedUpAt: time.Now().UTC(),
		}

		if !m.dryRun {
			outputPath := filepath.Join(volumesDir, filename)
			checksum, err := FileChecksum(outputPath)
			if err == nil {
				vb.Checksum = checksum
			}
		} else {
			vb.Checksum = "sha256:dry-run"
		}

		manifest.AddVolume(vb)
	}

	// Finalize and save manifest
	manifest.Finalize()

	if !m.dryRun {
		manifestPath := filepath.Join(backupDir, ManifestFilename)
		if err := manifest.Save(manifestPath); err != nil {
			return nil, fmt.Errorf("saving manifest: %w", err)
		}
	}

	elapsed := time.Since(totalStart)

	m.logger.Info("backup complete",
		"output_dir", backupDir,
		"volumes_backed_up", len(manifest.Volumes),
		"elapsed", elapsed,
	)

	return &BackupResult{
		Manifest:      manifest,
		Elapsed:       elapsed,
		VolumeTimings: allTimings,
	}, nil
}

// backupVolumeWithTiming backs up a single volume and returns timing info
func (m *Migrator) backupVolumeWithTiming(ctx context.Context, spec VolumeSpec, outputDir string) VolumeTiming {
	start := time.Now()

	var filename string
	switch spec.BackupType {
	case BackupTypePostgreSQL:
		filename = spec.Name + ".dump"
	default:
		filename = spec.Name + ".tar.gz"
	}

	outputPath := filepath.Join(outputDir, filename)

	var backupErr error
	switch spec.BackupType {
	case BackupTypePostgreSQL:
		backupErr = m.pgBackup.Backup(ctx, spec, outputPath)
	default:
		backupErr = m.volumeBackup.Backup(ctx, spec, outputPath)
	}

	timing := VolumeTiming{
		Name:    spec.Name,
		Elapsed: time.Since(start),
		Error:   backupErr,
	}

	if backupErr == nil && !m.dryRun {
		if info, err := os.Stat(outputPath); err == nil {
			timing.Size = info.Size()
		}
	}

	return timing
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
