package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alt-project/altctl/internal/fsperm"
	"github.com/stretchr/testify/require"
)

func readFileIn(t *testing.T, dir, name string) []byte {
	t.Helper()
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	b, err := root.ReadFile(name)
	require.NoError(t, err)
	return b
}

func readOptionalFileIn(t *testing.T, dir, name string) []byte {
	t.Helper()
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	b, err := root.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	require.NoError(t, err)
	return b
}

func (f *fakeBackupEngine) resolveContainer(_ context.Context, spec VolumeSpec) string {
	return "alt-" + spec.Service
}

type noResolverBackupEngine struct{}

func (noResolverBackupEngine) Backup(context.Context, VolumeSpec, string) error  { return nil }
func (noResolverBackupEngine) Restore(context.Context, VolumeSpec, string) error { return nil }

// createTestBackup creates a test backup directory with a valid manifest
func createTestBackup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	backupDir := filepath.Join(dir, "test_backup")
	volumesDir := filepath.Join(backupDir, "volumes")
	if err := os.MkdirAll(volumesDir, fsperm.PrivateDir); err != nil {
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
		if err := os.WriteFile(filePath, []byte("test-data-"+spec.Name), fsperm.SecretFile); err != nil {
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
	if err := os.WriteFile(filepath.Join(backupDir, ManifestFilename), data, fsperm.SecretFile); err != nil {
		t.Fatal(err)
	}

	return backupDir
}

func writeFakeDocker(t *testing.T, script string) {
	t.Helper()
	binDir := t.TempDir()
	fakeDocker := filepath.Join(binDir, "docker")
	// #nosec G306 -- executable fake docker script for test PATH
	if err := os.WriteFile(fakeDocker, []byte(script), fsperm.OwnerExecFile); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
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
	t.Run("reaches all volume engines and aggregates their failures", func(t *testing.T) {
		backupDir := createTestBackup(t)

		composeDir := t.TempDir()
		composeFile := filepath.Join(composeDir, "compose.yaml")
		if err := os.WriteFile(composeFile, []byte("name: alt\ninclude:\n  - db.yaml\n"), fsperm.SecretFile); err != nil {
			t.Fatal(err)
		}

		script := `#!/usr/bin/env sh
set -e
case "$*" in
  *"compose -f "*)
    exit 0
    ;;
  "exec "*" pg_isready -h 127.0.0.1 -p 5432 "*)
    exit 0
    ;;
esac
exit 1
`
		writeFakeDocker(t, script)

		tarEngine := &fakeBackupEngine{err: errors.New("simulated tar restore failure")}
		pgEngine := &fakeBackupEngine{err: errors.New("simulated pg_restore failure")}
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
			t.Fatal("expected Restore() to return an error when every volume restore fails")
		}
		if pgEngine.calls.Load() != 7 || tarEngine.calls.Load() != 8 {
			t.Fatalf("expected all 7 pg and 8 tar volumes to be attempted, got pg=%d tar=%d (err=%v)", pgEngine.calls.Load(), tarEngine.calls.Load(), err)
		}
	})

	t.Run("fails fast without invoking docker compose up without -f when no compose files exist", func(t *testing.T) {
		backupDir := createTestBackup(t)

		logFile := filepath.Join(t.TempDir(), "docker.log")
		script := `#!/usr/bin/env sh
printf '%s\n' "$*" >> "` + logFile + `"
exit 1
`
		writeFakeDocker(t, script)

		tarEngine := &fakeBackupEngine{}
		pgEngine := &fakeBackupEngine{}
		migrator := &Migrator{
			registry:     NewVolumeRegistry(),
			volumeBackup: tarEngine,
			pgBackup:     pgEngine,
			composeDir:   filepath.Join(t.TempDir(), "nonexistent-compose-dir"),
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
			t.Fatal("expected Restore() to fail when no compose files exist")
		}

		raw := readOptionalFileIn(t, filepath.Dir(logFile), filepath.Base(logFile))
		for _, call := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			if strings.Contains(call, "up -d") {
				t.Fatalf("expected startDatabaseContainers to fail fast without invoking docker compose up without -f, got docker calls: %q", strings.TrimSpace(string(raw)))
			}
		}
	})
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

// TestMigrator_Restore_AbortsOnMissingAggregate is the regression test for
// C1 under the aggregate-file strategy: a compose dir that exists but lacks
// the aggregate compose.yaml must ABORT restore, not silently degrade into
// "nothing running" and skip the stop-containers guard. Before the fix,
// composeFileList degraded such a layout into whatever per-stack files it
// could find (or none), getRunningContainers reported zero running
// containers, and restore proceeded straight to overwriting volumes -- even
// if the real containers for that stack were live.
//
// This asserts both that Restore() returns an error AND that neither backup
// engine was ever invoked, i.e. the abort happens before any volume write is
// attempted.
func TestMigrator_Restore_AbortsOnMissingAggregate(t *testing.T) {
	backupDir := createTestBackup(t)

	// composeDir exists and has a per-stack file, but no aggregate
	// compose.yaml -- the layout composeFileList must refuse loudly.
	root := t.TempDir()
	composeDir := filepath.Join(root, "compose")
	if err := os.MkdirAll(composeDir, fsperm.PrivateDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(composeDir, "db.yaml"), []byte("services:\n  db:\n    image: postgres\n"), fsperm.SecretFile); err != nil {
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
		t.Fatal("expected Restore() to abort when the aggregate compose.yaml is missing, got nil error")
	}
	if tarEngine.calls.Load() > 0 || pgEngine.calls.Load() > 0 {
		t.Fatalf("expected restore to abort before touching any volume, got tar calls=%d pg calls=%d", tarEngine.calls.Load(), pgEngine.calls.Load())
	}
}

func TestMigrator_Restore_StartsDatabaseServiceBeforePGRestore(t *testing.T) {
	t.Run("starts database service and waits for readiness before pg_restore", func(t *testing.T) {
		backupDir := createTestBackup(t)

		composeDir := t.TempDir()
		composeFile := filepath.Join(composeDir, "compose.yaml")
		if err := os.WriteFile(composeFile, []byte("name: alt\ninclude:\n  - db.yaml\n"), fsperm.SecretFile); err != nil {
			t.Fatal(err)
		}

		logFile := filepath.Join(t.TempDir(), "docker.log")
		script := `#!/usr/bin/env sh
set -e
printf '%s\n' "$*" >> "` + logFile + `"
case "$*" in
  *"compose"*"ps -q db")
    printf 'alt-db\n'
    exit 0
    ;;
  *"compose"*"ps -q")
    exit 0
    ;;
  *"compose"*"up -d db")
    exit 0
    ;;
  "exec alt-db pg_isready -h 127.0.0.1 -p 5432 -U alt_db_user -d alt")
    exit 0
    ;;
  "exec -i alt-db pg_restore"*)
    cat >/dev/null
    exit 0
    ;;
esac
exit 1
`
		writeFakeDocker(t, script)

		migrator := NewMigrator(composeDir, "alt", slog.Default(), false)
		err := migrator.Restore(context.Background(), RestoreOptions{
			BackupDir: backupDir,
			Force:     true,
			Verify:    false,
			Volumes:   []string{"db_data_17"},
		})
		if err != nil {
			t.Fatalf("Restore() failed: %v", err)
		}

		raw := readFileIn(t, filepath.Dir(logFile), filepath.Base(logFile))
		calls := strings.Split(strings.TrimSpace(string(raw)), "\n")

		upIdx := -1
		readyIdx := -1
		restoreIdx := -1
		wantUp := "compose -f " + composeFile + " up -d db"
		wantReady := "exec alt-db pg_isready -h 127.0.0.1 -p 5432 -U alt_db_user -d alt"
		wantRestorePrefix := "exec -i alt-db pg_restore -U alt_db_user -d alt"

		for i, c := range calls {
			switch {
			case c == wantUp && upIdx == -1:
				upIdx = i
			case c == wantReady && readyIdx == -1:
				readyIdx = i
			case strings.HasPrefix(c, wantRestorePrefix) && restoreIdx == -1:
				restoreIdx = i
			}
		}

		if upIdx == -1 {
			t.Fatalf("expected %q before pg_restore, got calls: %v", wantUp, calls)
		}
		if readyIdx == -1 {
			t.Fatalf("expected %q before pg_restore, got calls: %v", wantReady, calls)
		}
		if restoreIdx == -1 {
			t.Fatalf("expected %q call, got calls: %v", wantRestorePrefix, calls)
		}
		if upIdx >= readyIdx || readyIdx >= restoreIdx {
			t.Fatalf("expected call order up (%d) < pg_isready (%d) < pg_restore (%d), got calls: %v", upIdx, readyIdx, restoreIdx, calls)
		}
	})

	t.Run("aborts before restoring any volume when starting database service fails", func(t *testing.T) {
		backupDir := createTestBackup(t)

		composeDir := t.TempDir()
		composeFile := filepath.Join(composeDir, "compose.yaml")
		if err := os.WriteFile(composeFile, []byte("name: alt\ninclude:\n  - db.yaml\n"), fsperm.SecretFile); err != nil {
			t.Fatal(err)
		}

		logFile := filepath.Join(t.TempDir(), "docker.log")
		script := `#!/usr/bin/env sh
set -e
printf '%s\n' "$*" >> "` + logFile + `"
case "$*" in
  *"compose"*"ps -q")
    exit 0
    ;;
  *"compose"*"up -d"*)
    echo "simulated compose up failure" >&2
    exit 1
    ;;
esac
exit 0
`
		writeFakeDocker(t, script)

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
			t.Fatal("expected Restore() to fail when starting database container fails, got nil")
		}
		if tarEngine.calls.Load() > 0 || pgEngine.calls.Load() > 0 {
			t.Fatalf("expected restore to abort before touching any volume, got tar calls=%d pg calls=%d", tarEngine.calls.Load(), pgEngine.calls.Load())
		}
	})

	t.Run("returns error when injected pgBackup engine lacks container resolution", func(t *testing.T) {
		backupDir := createTestBackup(t)

		composeDir := t.TempDir()
		composeFile := filepath.Join(composeDir, "compose.yaml")
		if err := os.WriteFile(composeFile, []byte("name: alt\ninclude:\n  - db.yaml\n"), fsperm.SecretFile); err != nil {
			t.Fatal(err)
		}

		script := `#!/usr/bin/env sh
exit 0
`
		writeFakeDocker(t, script)

		migrator := &Migrator{
			registry:     NewVolumeRegistry(),
			volumeBackup: &fakeBackupEngine{},
			pgBackup:     noResolverBackupEngine{},
			composeDir:   composeDir,
			projectName:  "alt",
			logger:       slog.Default(),
			dryRun:       false,
		}

		err := migrator.Restore(context.Background(), RestoreOptions{
			BackupDir: backupDir,
			Force:     true,
			Verify:    false,
			Volumes:   []string{"db_data_17"},
		})
		if err == nil {
			t.Fatal("expected Restore() to return an error when pgBackup lacks resolveContainer")
		}
	})

	t.Run("returns error before any docker call when PostgreSQL volume spec has DBPort 0", func(t *testing.T) {
		backupDir := createTestBackup(t)

		composeDir := t.TempDir()
		composeFile := filepath.Join(composeDir, "compose.yaml")
		if err := os.WriteFile(composeFile, []byte("name: alt\ninclude:\n  - db.yaml\n"), fsperm.SecretFile); err != nil {
			t.Fatal(err)
		}

		logFile := filepath.Join(t.TempDir(), "docker.log")
		script := `#!/usr/bin/env sh
printf '%s\n' "$*" >> "` + logFile + `"
exit 0
`
		writeFakeDocker(t, script)

		registry := &VolumeRegistry{
			volumes: []VolumeSpec{
				{
					Name:       "db_data_17",
					Service:    "db",
					BackupType: BackupTypePostgreSQL,
					Category:   CategoryCritical,
					DBName:     "alt",
					DBUser:     "alt_db_user",
					DBPort:     0,
				},
			},
		}

		migrator := &Migrator{
			registry:     registry,
			volumeBackup: &fakeBackupEngine{},
			pgBackup:     &fakeBackupEngine{},
			composeDir:   composeDir,
			projectName:  "alt",
			logger:       slog.Default(),
			dryRun:       false,
		}

		err := migrator.Restore(context.Background(), RestoreOptions{
			BackupDir: backupDir,
			Force:     true,
			Verify:    false,
			Volumes:   []string{"db_data_17"},
		})
		if err == nil {
			t.Fatal("expected Restore() to return an error when PostgreSQL volume spec has DBPort 0")
		}
		if raw := readOptionalFileIn(t, filepath.Dir(logFile), filepath.Base(logFile)); strings.TrimSpace(string(raw)) != "" {
			t.Fatalf("expected no docker calls before failing on DBPort 0, got: %q", strings.TrimSpace(string(raw)))
		}
	})
}

func TestMigrator_Restore_RejectsEscapingFilename(t *testing.T) {
	for _, badName := range []string{"../escape.tar.gz", "/etc/passwd", "", "."} {
		t.Run(badName, func(t *testing.T) {
			backupDir := t.TempDir()
			if !filepath.IsAbs(backupDir) {
				t.Fatalf("expected t.TempDir() to be absolute, got %q", backupDir)
			}
			m := NewManifest("test")
			m.AddVolume(VolumeBackup{
				Name:       "meili_data",
				Type:       BackupTypeTar,
				TypeString: "tar",
				Filename:   badName,
				Size:       1,
			})
			m.Finalize()
			require.NoError(t, m.Save(filepath.Join(backupDir, ManifestFilename)))

			tarEngine := &fakeBackupEngine{}
			pgEngine := &fakeBackupEngine{}
			migrator := &Migrator{
				registry:     NewVolumeRegistry(),
				volumeBackup: tarEngine,
				pgBackup:     pgEngine,
				composeDir:   filepath.Join(t.TempDir(), "compose"),
				projectName:  "alt",
				logger:       slog.Default(),
				dryRun:       true,
			}

			err := migrator.Restore(context.Background(), RestoreOptions{
				BackupDir: backupDir,
				Force:     true,
				Verify:    false,
				Volumes:   []string{"meili_data"},
			})
			require.Error(t, err)
			if tarEngine.calls.Load() != 0 || pgEngine.calls.Load() != 0 {
				t.Fatalf("expected restore engine not to be called on escaping filename, got tar=%d pg=%d", tarEngine.calls.Load(), pgEngine.calls.Load())
			}
		})
	}
}

func TestMigrator_Restore_RejectsSymlinkedBackupFiles(t *testing.T) {
	type restoreCase struct {
		name      string
		volName   string
		volType   BackupType
		typeStr   string
		filename  string
		dirLink   bool
		verifyOpt bool
	}

	cases := []restoreCase{
		{
			name:      "postgresql leaf symlink verify=false",
			volName:   "db_data_17",
			volType:   BackupTypePostgreSQL,
			typeStr:   "postgresql",
			filename:  "volumes/db_data_17.dump",
			dirLink:   false,
			verifyOpt: false,
		},
		{
			name:      "postgresql leaf symlink verify=true",
			volName:   "db_data_17",
			volType:   BackupTypePostgreSQL,
			typeStr:   "postgresql",
			filename:  "volumes/db_data_17.dump",
			dirLink:   false,
			verifyOpt: true,
		},
		{
			name:      "tar leaf symlink verify=false",
			volName:   "meili_data",
			volType:   BackupTypeTar,
			typeStr:   "tar",
			filename:  "volumes/meili_data.tar.gz",
			dirLink:   false,
			verifyOpt: false,
		},
		{
			name:      "tar leaf symlink verify=true",
			volName:   "meili_data",
			volType:   BackupTypeTar,
			typeStr:   "tar",
			filename:  "volumes/meili_data.tar.gz",
			dirLink:   false,
			verifyOpt: true,
		},
		{
			name:      "postgresql directory symlink verify=false",
			volName:   "db_data_17",
			volType:   BackupTypePostgreSQL,
			typeStr:   "postgresql",
			filename:  "volumes/db_data_17.dump",
			dirLink:   true,
			verifyOpt: false,
		},
		{
			name:      "tar directory symlink verify=false",
			volName:   "meili_data",
			volType:   BackupTypeTar,
			typeStr:   "tar",
			filename:  "volumes/meili_data.tar.gz",
			dirLink:   true,
			verifyOpt: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outsideDir := t.TempDir()
			leafName := filepath.Base(tc.filename)
			outsideFile := filepath.Join(outsideDir, leafName)
			content := []byte("outside-host-file")
			require.NoError(t, os.WriteFile(outsideFile, content, fsperm.SecretFile))
			checksum, err := FileChecksum(outsideFile)
			require.NoError(t, err)

			backupDir := t.TempDir()
			if tc.dirLink {
				require.NoError(t, os.Symlink(outsideDir, filepath.Join(backupDir, "volumes")))
			} else {
				volumesDir := filepath.Join(backupDir, "volumes")
				require.NoError(t, os.MkdirAll(volumesDir, fsperm.SecretDir))
				require.NoError(t, os.Symlink(outsideFile, filepath.Join(volumesDir, leafName)))
			}

			m := NewManifest("test")
			m.AddVolume(VolumeBackup{
				Name:       tc.volName,
				Type:       tc.volType,
				TypeString: tc.typeStr,
				Filename:   tc.filename,
				Size:       int64(len(content)),
				Checksum:   checksum,
			})
			m.Finalize()
			require.NoError(t, m.Save(filepath.Join(backupDir, ManifestFilename)))

			tarEngine := &fakeBackupEngine{}
			pgEngine := &fakeBackupEngine{}
			migrator := &Migrator{
				registry:     NewVolumeRegistry(),
				volumeBackup: tarEngine,
				pgBackup:     pgEngine,
				composeDir:   filepath.Join(t.TempDir(), "compose"),
				projectName:  "alt",
				logger:       slog.Default(),
				dryRun:       true,
			}

			err = migrator.Restore(context.Background(), RestoreOptions{
				BackupDir: backupDir,
				Force:     true,
				Verify:    tc.verifyOpt,
				Volumes:   []string{tc.volName},
			})
			require.Error(t, err)
			require.Zero(t, tarEngine.calls.Load(), "tar restore engine must not be invoked on symlinked backup file")
			require.Zero(t, pgEngine.calls.Load(), "pg restore engine must not be invoked on symlinked backup file")
		})
	}
}

func TestBackupDirManifestLoading_RejectsSymlinkedManifest(t *testing.T) {
	outsideDir := t.TempDir()
	outsideManifestPath := filepath.Join(outsideDir, ManifestFilename)
	m := NewManifest("test")
	m.Finalize()
	require.NoError(t, m.Save(outsideManifestPath))

	parentDir := t.TempDir()
	backupDir := filepath.Join(parentDir, "20261128_000000")
	require.NoError(t, os.MkdirAll(backupDir, fsperm.SecretDir))
	require.NoError(t, os.Symlink(outsideManifestPath, filepath.Join(backupDir, ManifestFilename)))

	t.Run("VerifyBackup rejects symlinked manifest.json", func(t *testing.T) {
		_, err := VerifyBackup(backupDir)
		require.Error(t, err)
	})

	t.Run("GetBackupSummary rejects symlinked manifest.json", func(t *testing.T) {
		_, err := GetBackupSummary(backupDir)
		require.Error(t, err)
	})

	t.Run("Migrator.Restore rejects symlinked manifest.json", func(t *testing.T) {
		tarEngine := &fakeBackupEngine{}
		pgEngine := &fakeBackupEngine{}
		migrator := &Migrator{
			registry:     NewVolumeRegistry(),
			volumeBackup: tarEngine,
			pgBackup:     pgEngine,
			composeDir:   filepath.Join(t.TempDir(), "compose"),
			projectName:  "alt",
			logger:       slog.Default(),
			dryRun:       true,
		}
		err := migrator.Restore(context.Background(), RestoreOptions{
			BackupDir: backupDir,
			Force:     true,
			Verify:    false,
		})
		require.Error(t, err)
		require.Zero(t, tarEngine.calls.Load())
		require.Zero(t, pgEngine.calls.Load())
	})

	t.Run("ListBackups skips backup with symlinked manifest.json", func(t *testing.T) {
		backups, err := ListBackups(parentDir)
		require.NoError(t, err)
		require.Empty(t, backups)
	})
}
