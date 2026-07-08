package migrate

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBackupEngine is a backupEngine stub that simulates volume backup/restore
// failures without invoking a real Docker daemon.
type fakeBackupEngine struct {
	err error
}

func (f *fakeBackupEngine) Backup(ctx context.Context, spec VolumeSpec, outputPath string) error {
	return f.err
}

func (f *fakeBackupEngine) Restore(ctx context.Context, spec VolumeSpec, inputPath string) error {
	return f.err
}

func TestMigrator_Backup_DryRun_ProfileDB(t *testing.T) {
	migrator := NewMigrator("/tmp/compose", "alt", slog.Default(), true)

	result, err := migrator.Backup(context.Background(), BackupOptions{
		OutputDir:     t.TempDir(),
		Force:         true,
		AltctlVersion: "test",
		Profile:       ProfileDB,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil BackupResult")
	}

	// ProfileDB should only include PG volumes (6)
	if len(result.Manifest.Volumes) != 6 {
		t.Errorf("Expected 6 volumes for ProfileDB, got %d", len(result.Manifest.Volumes))
	}

	for _, v := range result.Manifest.Volumes {
		if v.Type != BackupTypePostgreSQL {
			t.Errorf("ProfileDB should only contain PostgreSQL volumes, got %s (%s)", v.Name, v.TypeString)
		}
	}
}

func TestMigrator_Backup_DryRun_ProfileEssential(t *testing.T) {
	migrator := NewMigrator("/tmp/compose", "alt", slog.Default(), true)

	result, err := migrator.Backup(context.Background(), BackupOptions{
		OutputDir:     t.TempDir(),
		Force:         true,
		AltctlVersion: "test",
		Profile:       ProfileEssential,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ProfileEssential: critical(6) + data(3) + search(1) = 10
	if len(result.Manifest.Volumes) != 10 {
		t.Errorf("Expected 10 volumes for ProfileEssential, got %d", len(result.Manifest.Volumes))
	}

	// Should not include metrics or models
	for _, v := range result.Manifest.Volumes {
		if v.Name == "clickhouse_data" || v.Name == "prometheus_data" || v.Name == "grafana_data" || v.Name == "news_creator_models" {
			t.Errorf("ProfileEssential should not include %s", v.Name)
		}
	}
}

func TestMigrator_Backup_DryRun_ProfileAll(t *testing.T) {
	migrator := NewMigrator("/tmp/compose", "alt", slog.Default(), true)

	result, err := migrator.Backup(context.Background(), BackupOptions{
		OutputDir:     t.TempDir(),
		Force:         true,
		AltctlVersion: "test",
		Profile:       ProfileAll,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Manifest.Volumes) != 14 {
		t.Errorf("Expected 14 volumes for ProfileAll, got %d", len(result.Manifest.Volumes))
	}
}

func TestMigrator_Backup_DryRun_WithExclude(t *testing.T) {
	migrator := NewMigrator("/tmp/compose", "alt", slog.Default(), true)

	result, err := migrator.Backup(context.Background(), BackupOptions{
		OutputDir:     t.TempDir(),
		Force:         true,
		AltctlVersion: "test",
		Profile:       ProfileAll,
		Exclude:       []string{"clickhouse_data", "prometheus_data", "grafana_data"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Manifest.Volumes) != 11 {
		t.Errorf("Expected 11 volumes after excluding 3, got %d", len(result.Manifest.Volumes))
	}
}

func TestMigrator_Backup_DryRun_DefaultProfile(t *testing.T) {
	migrator := NewMigrator("/tmp/compose", "alt", slog.Default(), true)

	// Empty profile should default to ProfileAll for backward compatibility
	result, err := migrator.Backup(context.Background(), BackupOptions{
		OutputDir:     t.TempDir(),
		Force:         true,
		AltctlVersion: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Manifest.Volumes) != 14 {
		t.Errorf("Empty profile should default to all (14 volumes), got %d", len(result.Manifest.Volumes))
	}
}

func TestBackupResult_HasTimings(t *testing.T) {
	migrator := NewMigrator("/tmp/compose", "alt", slog.Default(), true)

	result, err := migrator.Backup(context.Background(), BackupOptions{
		OutputDir:     t.TempDir(),
		Force:         true,
		AltctlVersion: "test",
		Profile:       ProfileDB,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.VolumeTimings) != 6 {
		t.Errorf("Expected 6 volume timings, got %d", len(result.VolumeTimings))
	}

	for _, timing := range result.VolumeTimings {
		if timing.Name == "" {
			t.Error("Volume timing should have a name")
		}
	}

	if result.Elapsed <= 0 {
		t.Error("Total elapsed time should be positive")
	}
}

func TestBackupResult_ConcurrencyDefault(t *testing.T) {
	migrator := NewMigrator("/tmp/compose", "alt", slog.Default(), true)

	// Concurrency 0 should use default (not panic)
	result, err := migrator.Backup(context.Background(), BackupOptions{
		OutputDir:     t.TempDir(),
		Force:         true,
		AltctlVersion: "test",
		Profile:       ProfileDB,
		Concurrency:   0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result with default concurrency")
	}
}

func TestCompressBackupDir(t *testing.T) {
	// Create a fake backup directory with some files
	baseDir := t.TempDir()
	backupDir := filepath.Join(baseDir, "20260409_120000")
	volumesDir := filepath.Join(backupDir, "volumes")
	if err := os.MkdirAll(volumesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write some test files
	if err := os.WriteFile(filepath.Join(backupDir, "manifest.json"), []byte(`{"version":"1.0"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(volumesDir, "db_data_17.dump"), []byte("fake-pg-dump-data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Compress
	archivePath, err := CompressBackupDir(context.Background(), backupDir)
	if err != nil {
		t.Fatalf("CompressBackupDir failed: %v", err)
	}

	// Verify archive was created
	if !strings.HasSuffix(archivePath, ".tar.gz") {
		t.Errorf("Expected .tar.gz suffix, got %s", archivePath)
	}

	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("Archive file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Archive file is empty")
	}

	// Verify original directory was removed
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Error("Original backup directory should be removed after compression")
	}
}

func TestCompressBackupDir_NonExistent(t *testing.T) {
	_, err := CompressBackupDir(context.Background(), "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for non-existent directory")
	}
}

func TestMigrator_Backup_WithCompress(t *testing.T) {
	migrator := NewMigrator("/tmp/compose", "alt", slog.Default(), true)

	result, err := migrator.Backup(context.Background(), BackupOptions{
		OutputDir:     t.TempDir(),
		Force:         true,
		AltctlVersion: "test",
		Profile:       ProfileDB,
		Compress:      true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// In dry-run, ArchivePath should still be set (predicted path)
	if result.ArchivePath == "" {
		t.Error("Expected ArchivePath to be set when Compress=true")
	}
	if !strings.HasSuffix(result.ArchivePath, ".tar.gz") {
		t.Errorf("Expected .tar.gz suffix, got %s", result.ArchivePath)
	}
}

// TestMigrator_Backup_ReturnsErrorWhenAllVolumesFail guards against the
// "backup complete, exit 0" false-success bug: when every volume backup
// fails, Backup() must surface an aggregate error instead of silently
// returning nil (the per-volume errors were only visible in VolumeTiming,
// which callers like runMigrateBackup never inspected for exit status).
func TestMigrator_Backup_ReturnsErrorWhenAllVolumesFail(t *testing.T) {
	migrator := &Migrator{
		registry:     NewVolumeRegistry(),
		volumeBackup: &fakeBackupEngine{err: errors.New("simulated tar backup failure")},
		pgBackup:     &fakeBackupEngine{err: errors.New("simulated pg_dump failure")},
		composeDir:   "/tmp/compose",
		projectName:  "alt",
		logger:       slog.Default(),
		dryRun:       false,
	}

	result, err := migrator.Backup(context.Background(), BackupOptions{
		OutputDir:     t.TempDir(),
		Force:         true,
		AltctlVersion: "test",
		Profile:       ProfileDB,
	})

	if err == nil {
		t.Fatal("expected Backup() to return an error when every volume backup fails")
	}
	if result == nil {
		t.Fatal("expected a non-nil result carrying per-volume timings even on failure")
	}
	for _, timing := range result.VolumeTimings {
		if timing.Error == nil {
			t.Errorf("expected volume %s to have a recorded backup error", timing.Name)
		}
	}
}

// TestMigrator_Backup_ReturnsNilWhenVolumesSucceed is the GREEN-path sibling:
// the new aggregate-error logic must not regress the fully-successful case.
func TestMigrator_Backup_ReturnsNilWhenVolumesSucceed(t *testing.T) {
	migrator := &Migrator{
		registry:     NewVolumeRegistry(),
		volumeBackup: &fakeBackupEngine{},
		pgBackup:     &fakeBackupEngine{},
		composeDir:   "/tmp/compose",
		projectName:  "alt",
		logger:       slog.Default(),
		dryRun:       true,
	}

	result, err := migrator.Backup(context.Background(), BackupOptions{
		OutputDir:     t.TempDir(),
		Force:         true,
		AltctlVersion: "test",
		Profile:       ProfileDB,
	})
	if err != nil {
		t.Fatalf("unexpected error when all volumes succeed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestComposeFileList_IncludesSovereign guards against configuration drift:
// the compose file list backup/restore use to detect and stop running
// containers must be derived from the stack registry (the single source of
// truth also used by `altctl up`/`down`), not a hand-maintained list that can
// forget a stack such as sovereign.yaml.
func TestComposeFileList_IncludesSovereign(t *testing.T) {
	files := composeFileList("/compose")

	want := filepath.Join("/compose", "sovereign.yaml")
	found := false
	for _, f := range files {
		if f == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected composeFileList to include %q, got %v", want, files)
	}
}

// TestMigrator_BuildComposeArgs_IncludesSovereignWhenPresent verifies that
// restore's pre-restore "down" (buildComposeArgs, restore.go) draws from the
// same composeFileList as backup's getRunningContainers, so a stack like
// sovereign can't be stopped by one code path and missed by the other.
func TestMigrator_BuildComposeArgs_IncludesSovereignWhenPresent(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"base.yaml", "db.yaml", "sovereign.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("services: {}\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	m := &Migrator{composeDir: dir}
	args := m.buildComposeArgs("down")

	wantFlag := filepath.Join(dir, "sovereign.yaml")
	found := false
	for _, a := range args {
		if a == wantFlag {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected buildComposeArgs to reference %q, got %v", wantFlag, args)
	}
}
