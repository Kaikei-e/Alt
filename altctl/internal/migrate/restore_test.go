package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createTestBackup creates a test backup directory with a valid manifest
func createTestBackup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	backupDir := filepath.Join(dir, "test_backup")
	volumesDir := filepath.Join(backupDir, "volumes")
	if err := os.MkdirAll(volumesDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifest := &Manifest{
		Version:       ManifestVersion,
		CreatedAt:     time.Now().UTC(),
		AltctlVersion: "test",
		Volumes:       []VolumeBackup{},
	}

	registry := NewVolumeRegistry()
	for _, spec := range registry.All() {
		var filename string
		if spec.BackupType == BackupTypePostgreSQL {
			filename = spec.Name + ".dump"
		} else {
			filename = spec.Name + ".tar.gz"
		}
		filePath := filepath.Join(volumesDir, filename)
		if err := os.WriteFile(filePath, []byte("test-data-"+spec.Name), 0644); err != nil {
			t.Fatal(err)
		}

		checksum, _ := FileChecksum(filePath)
		info, _ := os.Stat(filePath)

		manifest.AddVolume(VolumeBackup{
			Name:       spec.Name,
			Type:       spec.BackupType,
			Filename:   filepath.Join("volumes", filename),
			Size:       info.Size(),
			Checksum:   checksum,
			Service:    spec.Service,
			BackedUpAt: manifest.CreatedAt,
		})
	}
	manifest.Finalize()

	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(backupDir, ManifestFilename), data, 0644); err != nil {
		t.Fatal(err)
	}

	return backupDir
}

func TestMigrator_Restore_DryRun_AllVolumes(t *testing.T) {
	backupDir := createTestBackup(t)
	migrator := NewMigrator("/tmp/compose", "alt", slog.Default(), true)

	err := migrator.Restore(context.Background(), RestoreOptions{
		BackupDir: backupDir,
		Force:     true,
		Verify:    false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigrator_Restore_DryRun_ProfileDB(t *testing.T) {
	backupDir := createTestBackup(t)
	migrator := NewMigrator("/tmp/compose", "alt", slog.Default(), true)

	err := migrator.Restore(context.Background(), RestoreOptions{
		BackupDir: backupDir,
		Force:     true,
		Verify:    false,
		Profile:   ProfileDB,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigrator_Restore_DryRun_SpecificVolumes(t *testing.T) {
	backupDir := createTestBackup(t)
	migrator := NewMigrator("/tmp/compose", "alt", slog.Default(), true)

	err := migrator.Restore(context.Background(), RestoreOptions{
		BackupDir: backupDir,
		Force:     true,
		Verify:    false,
		Volumes:   []string{"db_data_17"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigrator_Restore_DryRun_UnknownVolume(t *testing.T) {
	backupDir := createTestBackup(t)
	migrator := NewMigrator("/tmp/compose", "alt", slog.Default(), true)

	err := migrator.Restore(context.Background(), RestoreOptions{
		BackupDir: backupDir,
		Force:     true,
		Verify:    false,
		Volumes:   []string{"nonexistent_volume"},
	})
	if err == nil {
		t.Error("expected error for unknown volume name")
	}
}

// TestMigrator_Restore_ReturnsErrorWhenAllVolumesFail guards against the
// "restore complete, exit 0" false-success bug: individual volume restore
// failures were only logged and continued past, so Restore() always
// returned nil even when every volume failed to restore.
func TestMigrator_Restore_ReturnsErrorWhenAllVolumesFail(t *testing.T) {
	backupDir := createTestBackup(t)

	migrator := &Migrator{
		registry:     NewVolumeRegistry(),
		volumeBackup: &fakeBackupEngine{err: errors.New("simulated tar restore failure")},
		pgBackup:     &fakeBackupEngine{err: errors.New("simulated pg_restore failure")},
		composeDir:   "/tmp/compose",
		projectName:  "alt",
		logger:       slog.Default(),
		dryRun:       false,
	}

	err := migrator.Restore(context.Background(), RestoreOptions{
		BackupDir: backupDir,
		Force:     true,
		Verify:    false,
	})
	if err == nil {
		t.Fatal("expected Restore() to return an error when every volume restore fails")
	}
}

// TestMigrator_Restore_ReturnsNilWhenVolumesSucceed is the GREEN-path
// sibling: the new aggregate-error logic must not regress the
// fully-successful restore case.
func TestMigrator_Restore_ReturnsNilWhenVolumesSucceed(t *testing.T) {
	backupDir := createTestBackup(t)

	migrator := &Migrator{
		registry:     NewVolumeRegistry(),
		volumeBackup: &fakeBackupEngine{},
		pgBackup:     &fakeBackupEngine{},
		composeDir:   "/tmp/compose",
		projectName:  "alt",
		logger:       slog.Default(),
		dryRun:       true,
	}

	err := migrator.Restore(context.Background(), RestoreOptions{
		BackupDir: backupDir,
		Force:     true,
		Verify:    false,
	})
	if err != nil {
		t.Fatalf("unexpected error when all volumes succeed: %v", err)
	}
}

// TestMigrator_Restore_AbortsOnBrokenRegistry is the regression test for C1:
// a broken .altctl.yaml (a stack declared with no matching compose file --
// stack.NewRegistry's Critical-Rule-9 hard fail-fast error) must ABORT
// restore, not silently degrade into "nothing running" and skip the
// stop-containers guard. Before the fix, composeFileList swallowed the
// registry load error into an empty file list, getRunningContainers reported
// zero running containers, and restore proceeded straight to overwriting
// volumes -- even if the real containers for that stack were live.
//
// This asserts both that Restore() returns an error AND that neither backup
// engine was ever invoked, i.e. the abort happens before any volume write is
// attempted.
func TestMigrator_Restore_AbortsOnBrokenRegistry(t *testing.T) {
	backupDir := createTestBackup(t)

	// project root layout: <root>/compose/*.yaml + <root>/.altctl.yaml,
	// mirroring composeFileList's convention (configPath is a sibling of
	// composeDir's parent). db.yaml exists and is a valid stack, but
	// .altctl.yaml declares a "sovereign" stack with no matching
	// sovereign.yaml -- exactly the fail-fast case stack.NewRegistry
	// documents (altctl/CLAUDE.md Critical Rule 9 / registry.go
	// composeFileServicesIfExists).
	root := t.TempDir()
	composeDir := filepath.Join(root, "compose")
	if err := os.MkdirAll(composeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(composeDir, "db.yaml"), []byte("services:\n  db:\n    image: postgres\n"), 0644); err != nil {
		t.Fatal(err)
	}
	brokenConfig := "stacks:\n  sovereign:\n    optional: false\n"
	if err := os.WriteFile(filepath.Join(root, ".altctl.yaml"), []byte(brokenConfig), 0644); err != nil {
		t.Fatal(err)
	}

	tarEngine := &fakeBackupEngine{}
	pgEngine := &fakeBackupEngine{}
	migrator := &Migrator{
		registry:     NewVolumeRegistry(),
		volumeBackup: tarEngine,
		pgBackup:     pgEngine,
		composeDir:   composeDir,
		projectName:  "alt",
		logger:       slog.Default(),
		dryRun:       false,
	}

	err := migrator.Restore(context.Background(), RestoreOptions{
		BackupDir: backupDir,
		Force:     true,
		Verify:    false,
	})
	if err == nil {
		t.Fatal("expected Restore() to abort when .altctl.yaml declares a stack with no matching compose file, got nil error")
	}
	if tarEngine.calls > 0 || pgEngine.calls > 0 {
		t.Fatalf("expected restore to abort before touching any volume, got tar calls=%d pg calls=%d", tarEngine.calls, pgEngine.calls)
	}
}
