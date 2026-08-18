package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/alt-project/altctl/internal/stack"
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
	Compress      bool          // Compress final backup directory into .tar.gz
}

// BackupResult contains the outcome of a backup operation
type BackupResult struct {
	Manifest      *Manifest
	Elapsed       time.Duration
	VolumeTimings []VolumeTiming
	ArchivePath   string // Path to .tar.gz archive (set when Compress=true)
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

// backupEngine performs a single-volume backup/restore operation. Both
// *PostgresBackuper and *VolumeBackuper satisfy it; tests substitute fakes to
// simulate volume failures without invoking a real Docker daemon.
type backupEngine interface {
	Backup(ctx context.Context, spec VolumeSpec, outputPath string) error
	Restore(ctx context.Context, spec VolumeSpec, inputPath string) error
}

// Migrator orchestrates backup and restore operations
type Migrator struct {
	registry     *VolumeRegistry
	volumeBackup backupEngine
	pgBackup     backupEngine
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
		pgBackup:     NewPostgresBackuper(projectName, composeDir, logger, dryRun),
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
	var failedVolumes []string
	for _, timing := range allTimings {
		if timing.Error != nil {
			m.logger.Error("volume backup failed",
				"volume", timing.Name,
				"error", timing.Error,
			)
			failedVolumes = append(failedVolumes, timing.Name)
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
			if err != nil {
				// Fail at backup time rather than shipping a manifest entry
				// with an empty checksum — Verify would otherwise report a
				// misleading "checksum mismatch" for what was actually a
				// checksum computation failure.
				m.logger.Error("checksum computation failed",
					"volume", timing.Name,
					"error", err,
				)
				failedVolumes = append(failedVolumes, timing.Name)
				continue
			}
			vb.Checksum = checksum
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

	result := &BackupResult{
		Manifest:      manifest,
		Elapsed:       elapsed,
		VolumeTimings: allTimings,
	}

	// Compress backup directory into .tar.gz if requested
	if opts.Compress {
		if m.dryRun {
			result.ArchivePath = backupDir + ".tar.gz"
			m.logger.Info("[dry-run] would compress backup",
				"archive", result.ArchivePath,
			)
		} else {
			archivePath, err := CompressBackupDir(ctx, backupDir)
			if err != nil {
				return nil, fmt.Errorf("compressing backup: %w", err)
			}
			result.ArchivePath = archivePath
			m.logger.Info("backup compressed",
				"archive", archivePath,
			)
		}
	}

	if len(failedVolumes) > 0 {
		m.logger.Error("backup finished with failures",
			"output_dir", backupDir,
			"volumes_backed_up", len(manifest.Volumes),
			"volumes_failed", len(failedVolumes),
			"elapsed", elapsed,
		)
		return result, fmt.Errorf("backup failed for %d volume(s): %s", len(failedVolumes), strings.Join(failedVolumes, ", "))
	}

	m.logger.Info("backup complete",
		"output_dir", backupDir,
		"volumes_backed_up", len(manifest.Volumes),
		"elapsed", elapsed,
	)

	return result, nil
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

// composeFileList returns the -f file list for every docker compose
// invocation this package builds (running-container detection, pre-restore
// "down", live container-ID lookup): the aggregate compose/compose.yaml
// alone. Backup and restore both use this as their single source of truth
// so container detection / stop operations can't silently drift out of
// sync and miss a stack (e.g. sovereign.yaml, whose DB volume would
// otherwise be overwritten by a restore while its container is still up) --
// the aggregate reaches every stack of the real project through its
// include: graph (drift is guarded by internal/stack's aggregate tests).
//
// Concatenating every stack's own file (the pre-C3 strategy) is
// structurally broken here just as it was for up/down: multiple -f flags
// MERGE same-named services (include: does not), and the isolated
// local-dev overlays dev.yaml / frontend-dev.yaml redeclare
// alt-frontend-sv / alt-backend with resource limits that conflict with
// core.yaml's ("services.alt-frontend-sv: can't set distinct values on
// 'mem_limit' and 'deploy.resources.limits.memory': invalid compose
// project"), so docker compose rejects the merged project before ps/down
// can run at all. Those isolated stacks run under their own compose
// project names and never mount this project's volumes, so leaving them
// out loses nothing (see cmd/compose_target.go).
//
// Two distinct failure shapes, which must NOT be treated alike (C1):
//
//   - composeDir itself does not exist at all. This is genuinely "nothing
//     to back up or stop" -- e.g. a synthetic test path like "/tmp/compose"
//     that was never meant to be a real project, or `altctl` invoked
//     outside a repo checkout. Tolerated: returns (nil, nil).
//   - composeDir exists but the aggregate file is missing (or unreadable).
//     A real project layout problem that must ABORT loudly. Degrading it
//     into an empty file list would make getRunningContainers report
//     "nothing running" even when a live stack's containers were up,
//     letting restore skip the stop-running-containers guard and overwrite
//     their volumes out from under them.
func composeFileList(composeDir string) ([]string, error) {
	if _, err := os.Stat(composeDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading compose dir %s: %w", composeDir, err)
	}
	aggregate := filepath.Join(composeDir, stack.AggregateComposeFile)
	if _, err := os.Stat(aggregate); err != nil {
		return nil, fmt.Errorf("aggregate compose file %s: %w", aggregate, err)
	}
	return []string{aggregate}, nil
}

// getRunningContainers returns a list of running containers for this project
func (m *Migrator) getRunningContainers(ctx context.Context) ([]string, error) {
	// Build compose file arguments
	args := []string{"compose"}

	fileList, err := composeFileList(m.composeDir)
	if err != nil {
		// A broken registry (malformed .altctl.yaml, a declared stack with
		// no matching compose file, ...) must abort here rather than be
		// treated as "no compose files, nothing running" -- see
		// composeFileList's doc comment (C1).
		return nil, fmt.Errorf("resolving compose files: %w", err)
	}

	var composeFilesFound int
	for _, f := range fileList {
		if _, err := os.Stat(f); err == nil {
			args = append(args, "-f", f)
			composeFilesFound++
		}
	}

	args = append(args, "ps", "-q")

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.Output()
	if err != nil {
		if composeFilesFound == 0 {
			// No compose files exist at m.composeDir at all (e.g. it doesn't
			// point at a real project) — docker compose has nothing to
			// inspect either way, so this isn't a "check failed" condition
			// distinct from "nothing running".
			return nil, nil
		}
		// At least one compose file was found but the command still failed:
		// a non-zero exit here means the command itself failed (docker
		// daemon down, permission denied, ...) and must propagate, not be
		// treated as "no running containers" — silently swallowing it here
		// would bypass the --force safety gate above.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("docker compose ps: %w: %s", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("docker compose ps: %w", err)
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

// CompressBackupDir compresses a backup directory into a .tar.gz archive,
// then removes the original directory. Returns the archive path.
func CompressBackupDir(ctx context.Context, backupDir string) (string, error) {
	// Verify directory exists
	info, err := os.Stat(backupDir)
	if err != nil {
		return "", fmt.Errorf("backup directory not found: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", backupDir)
	}

	archivePath := backupDir + ".tar.gz"
	dirName := filepath.Base(backupDir)
	parentDir := filepath.Dir(backupDir)

	// Create tar.gz using OS tar command for efficiency
	cmd := exec.CommandContext(ctx,
		"tar", "czf", archivePath,
		"-C", parentDir,
		dirName,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("tar compression failed: %w\n%s", err, string(output))
	}

	// Verify archive was created
	archiveInfo, err := os.Stat(archivePath)
	if err != nil {
		return "", fmt.Errorf("archive not found after compression: %w", err)
	}
	if archiveInfo.Size() == 0 {
		os.Remove(archivePath)
		return "", fmt.Errorf("archive is empty")
	}

	// Remove original directory
	if err := os.RemoveAll(backupDir); err != nil {
		return archivePath, fmt.Errorf("archive created but failed to remove original: %w", err)
	}

	return archivePath, nil
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
