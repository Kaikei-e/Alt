package migrate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

// VolumeBackuper handles tar-based volume backup and restore operations
type VolumeBackuper struct {
	projectName string // Docker Compose project name prefix
	logger      *slog.Logger
	dryRun      bool
}

// NewVolumeBackuper creates a new volume backuper
func NewVolumeBackuper(projectName string, logger *slog.Logger, dryRun bool) *VolumeBackuper {
	return &VolumeBackuper{
		projectName: projectName,
		logger:      logger,
		dryRun:      dryRun,
	}
}

// Backup creates a tar.gz backup of a Docker volume
func (v *VolumeBackuper) Backup(ctx context.Context, spec VolumeSpec, outputPath string) error {
	if spec.BackupType != BackupTypeTar {
		return fmt.Errorf("volume %s is not a tar-based volume", spec.Name)
	}

	volumeName := v.fullVolumeName(spec.Name)

	v.logger.Info("backing up volume",
		"volume", spec.Name,
		"docker_volume", volumeName,
		"output", outputPath,
	)

	if v.dryRun {
		v.logger.Info("[dry-run] would backup volume",
			"volume", volumeName,
		)
		return nil
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Check if volume exists
	if err := v.checkVolumeExists(ctx, volumeName); err != nil {
		return err
	}

	// Get absolute path for output
	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("getting absolute path: %w", err)
	}

	outputFilename := filepath.Base(outputPath)

	// Use busybox to create tar archive
	// docker run --rm -v <volume>:/data -v <output_dir>:/backup busybox tar czvf /backup/<filename> -C /data .
	args := []string{
		"run", "--rm",
		"-v", volumeName + ":/data:ro",
		"-v", absOutputDir + ":/backup",
		"busybox",
		"tar", "czvf", "/backup/" + outputFilename,
		"-C", "/data", ".",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stderr = v.logWriter("tar stderr")

	if err := cmd.Run(); err != nil {
		// Clean up failed backup file
		os.Remove(outputPath)
		return fmt.Errorf("tar backup failed: %w", err)
	}

	// Verify file was created
	info, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("verifying backup file: %w", err)
	}

	v.logger.Info("volume backup complete",
		"volume", spec.Name,
		"size", info.Size(),
	)

	return nil
}

// Restore restores a tar.gz backup to a Docker volume
func (v *VolumeBackuper) Restore(ctx context.Context, spec VolumeSpec, inputPath string) error {
	if spec.BackupType != BackupTypeTar {
		return fmt.Errorf("volume %s is not a tar-based volume", spec.Name)
	}

	volumeName := v.fullVolumeName(spec.Name)

	v.logger.Info("restoring volume",
		"volume", spec.Name,
		"docker_volume", volumeName,
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

	if v.dryRun {
		v.logger.Info("[dry-run] would restore volume",
			"volume", volumeName,
		)
		return nil
	}

	// Get absolute path for input file
	absInputPath, err := filepath.Abs(inputPath)
	if err != nil {
		return fmt.Errorf("getting absolute path: %w", err)
	}

	inputDir := filepath.Dir(absInputPath)
	inputFilename := filepath.Base(absInputPath)

	// Ensure volume exists (create if needed)
	if err := v.ensureVolumeExists(ctx, volumeName); err != nil {
		return err
	}

	// Use busybox to extract tar archive
	// docker run --rm -v <volume>:/data -v <input_dir>:/backup busybox tar xzvf /backup/<filename> -C /data
	args := []string{
		"run", "--rm",
		"-v", volumeName + ":/data",
		"-v", inputDir + ":/backup:ro",
		"busybox",
		"tar", "xzvf", "/backup/" + inputFilename,
		"-C", "/data",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stderr = v.logWriter("tar stderr")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tar restore failed: %w", err)
	}

	v.logger.Info("volume restore complete",
		"volume", spec.Name,
	)

	return nil
}

// fullVolumeName returns the full Docker volume name with project prefix
func (v *VolumeBackuper) fullVolumeName(volumeName string) string {
	if v.projectName == "" {
		return volumeName
	}
	return v.projectName + "_" + volumeName
}

// checkVolumeExists checks if a Docker volume exists
func (v *VolumeBackuper) checkVolumeExists(ctx context.Context, volumeName string) error {
	cmd := exec.CommandContext(ctx, "docker", "volume", "inspect", volumeName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("volume %s does not exist", volumeName)
	}
	return nil
}

// ensureVolumeExists creates a Docker volume if it doesn't exist
func (v *VolumeBackuper) ensureVolumeExists(ctx context.Context, volumeName string) error {
	// Check if exists
	if err := v.checkVolumeExists(ctx, volumeName); err == nil {
		return nil
	}

	// Create volume
	cmd := exec.CommandContext(ctx, "docker", "volume", "create", volumeName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("creating volume %s: %w", volumeName, err)
	}

	v.logger.Info("created volume", "volume", volumeName)
	return nil
}

// GetVolumeSize returns the size of a Docker volume in bytes
func (v *VolumeBackuper) GetVolumeSize(ctx context.Context, volumeName string) (int64, error) {
	fullName := v.fullVolumeName(volumeName)

	// Use du command inside busybox to get size
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", fullName+":/data:ro",
		"busybox",
		"du", "-sb", "/data",
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("getting volume size: %w", err)
	}

	var size int64
	if _, err := fmt.Sscanf(string(output), "%d", &size); err != nil {
		return 0, fmt.Errorf("parsing volume size: %w", err)
	}

	return size, nil
}

// logWriter is a writer that logs to slog
type logWriter struct {
	logger *slog.Logger
	prefix string
}

func (w *logWriter) Write(data []byte) (int, error) {
	if len(data) > 0 {
		w.logger.Debug(w.prefix, "output", string(data))
	}
	return len(data), nil
}

// logWriter returns a writer that logs to the logger
func (v *VolumeBackuper) logWriter(prefix string) *logWriter {
	return &logWriter{logger: v.logger, prefix: prefix}
}

// ClearVolume removes all data from a Docker volume
func (v *VolumeBackuper) ClearVolume(ctx context.Context, volumeName string) error {
	fullName := v.fullVolumeName(volumeName)

	v.logger.Warn("clearing volume", "volume", fullName)

	if v.dryRun {
		v.logger.Info("[dry-run] would clear volume", "volume", fullName)
		return nil
	}

	// Use busybox to remove all files
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", fullName+":/data",
		"busybox",
		"sh", "-c", "rm -rf /data/* /data/.[!.]* 2>/dev/null || true",
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clearing volume: %w", err)
	}

	return nil
}
