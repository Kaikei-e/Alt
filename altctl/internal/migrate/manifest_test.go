package migrate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alt-project/altctl/internal/fsperm"
)

func TestNewManifest(t *testing.T) {
	m := NewManifest("1.0.0")

	if m.Version != ManifestVersion {
		t.Errorf("Expected version %s, got %s", ManifestVersion, m.Version)
	}
	if m.AltctlVersion != "1.0.0" {
		t.Errorf("Expected altctl version 1.0.0, got %s", m.AltctlVersion)
	}
	if m.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if len(m.Volumes) != 0 {
		t.Error("Volumes should be empty initially")
	}
}

func TestManifest_AddVolume(t *testing.T) {
	m := NewManifest("1.0.0")

	vb := VolumeBackup{
		Name:     "test_volume",
		Type:     BackupTypePostgreSQL,
		Filename: "postgres/test_volume.dump",
		Size:     1024,
		Checksum: "sha256:abc123",
		Service:  "db",
	}

	m.AddVolume(vb)

	if len(m.Volumes) != 1 {
		t.Errorf("Expected 1 volume, got %d", len(m.Volumes))
	}

	added := m.Volumes[0]
	if added.TypeString != "postgresql" {
		t.Errorf("Expected TypeString 'postgresql', got '%s'", added.TypeString)
	}
	if added.BackedUpAt.IsZero() {
		t.Error("BackedUpAt should be set")
	}
}

func TestManifest_ComputeChecksum(t *testing.T) {
	m := NewManifest("1.0.0")
	m.AddVolume(VolumeBackup{
		Name:     "vol1",
		Checksum: "sha256:abc",
	})
	m.AddVolume(VolumeBackup{
		Name:     "vol2",
		Checksum: "sha256:def",
	})

	checksum := m.ComputeChecksum()
	if checksum == "" {
		t.Error("Checksum should not be empty")
	}
	if checksum[:7] != "sha256:" {
		t.Error("Checksum should start with 'sha256:'")
	}

	// Same volumes should produce same checksum
	m2 := NewManifest("1.0.0")
	m2.AddVolume(VolumeBackup{
		Name:     "vol1",
		Checksum: "sha256:abc",
	})
	m2.AddVolume(VolumeBackup{
		Name:     "vol2",
		Checksum: "sha256:def",
	})

	if m.ComputeChecksum() != m2.ComputeChecksum() {
		t.Error("Same content should produce same checksum")
	}
}

func TestManifest_Finalize(t *testing.T) {
	m := NewManifest("1.0.0")
	m.AddVolume(VolumeBackup{
		Name:     "test",
		Checksum: "sha256:test",
	})

	if m.Checksum != "" {
		t.Error("Checksum should be empty before finalize")
	}

	m.Finalize()

	if m.Checksum == "" {
		t.Error("Checksum should be set after finalize")
	}
}

func TestManifest_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, ManifestFilename)

	// Create and save manifest
	m := NewManifest("1.0.0")
	m.AddVolume(VolumeBackup{
		Name:     "db_data_17",
		Type:     BackupTypePostgreSQL,
		Filename: "postgres/db_data_17.dump",
		Size:     1024,
		Checksum: "sha256:abc123",
		Service:  "db",
	})
	m.Finalize()

	if err := m.Save(manifestPath); err != nil {
		t.Fatalf("Failed to save manifest: %v", err)
	}

	// Load and verify
	loaded, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("Failed to load manifest: %v", err)
	}

	if loaded.Version != m.Version {
		t.Errorf("Version mismatch: got %s, want %s", loaded.Version, m.Version)
	}
	if loaded.AltctlVersion != m.AltctlVersion {
		t.Errorf("AltctlVersion mismatch: got %s, want %s", loaded.AltctlVersion, m.AltctlVersion)
	}
	if len(loaded.Volumes) != len(m.Volumes) {
		t.Errorf("Volumes count mismatch: got %d, want %d", len(loaded.Volumes), len(m.Volumes))
	}
	if loaded.Checksum != m.Checksum {
		t.Errorf("Checksum mismatch: got %s, want %s", loaded.Checksum, m.Checksum)
	}
}

func TestManifest_GetVolume(t *testing.T) {
	m := NewManifest("1.0.0")
	m.AddVolume(VolumeBackup{Name: "vol1", Checksum: "sha256:1"})
	m.AddVolume(VolumeBackup{Name: "vol2", Checksum: "sha256:2"})

	// Found
	v, ok := m.GetVolume("vol1")
	if !ok {
		t.Error("Should find vol1")
	}
	if v.Name != "vol1" {
		t.Errorf("Expected vol1, got %s", v.Name)
	}

	// Not found
	_, ok = m.GetVolume("nonexistent")
	if ok {
		t.Error("Should not find nonexistent")
	}
}

func TestFileChecksum(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Create test file
	content := []byte("hello world")
	if err := os.WriteFile(testFile, content, fsperm.SecretFile); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	checksum, err := FileChecksum(testFile)
	if err != nil {
		t.Fatalf("FileChecksum failed: %v", err)
	}

	if checksum[:7] != "sha256:" {
		t.Error("Checksum should start with 'sha256:'")
	}

	// Same content should produce same checksum
	testFile2 := filepath.Join(tmpDir, "test2.txt")
	if err := os.WriteFile(testFile2, content, fsperm.SecretFile); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	checksum2, err := FileChecksum(testFile2)
	if err != nil {
		t.Fatalf("FileChecksum failed: %v", err)
	}

	if checksum != checksum2 {
		t.Error("Same content should produce same checksum")
	}
}

func TestManifest_Verify(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	postgresDir := filepath.Join(tmpDir, "postgres")
	if err := os.MkdirAll(postgresDir, fsperm.PrivateDir); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	testFile := filepath.Join(postgresDir, "test.dump")
	content := []byte("test backup data")
	if err := os.WriteFile(testFile, content, fsperm.SecretFile); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Get checksum
	checksum, err := FileChecksum(testFile)
	if err != nil {
		t.Fatalf("Failed to get checksum: %v", err)
	}

	// Create manifest
	m := NewManifest("1.0.0")
	m.AddVolume(VolumeBackup{
		Name:     "test",
		Type:     BackupTypePostgreSQL,
		Filename: "postgres/test.dump",
		Size:     int64(len(content)),
		Checksum: checksum,
	})
	m.Finalize()

	// Verify should pass
	if err := m.Verify(tmpDir); err != nil {
		t.Errorf("Verify failed: %v", err)
	}

	// Modify file - verify should fail
	if err := os.WriteFile(testFile, []byte("modified"), fsperm.SecretFile); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	if err := m.Verify(tmpDir); err == nil {
		t.Error("Verify should fail after file modification")
	}
}

func TestManifest_Verify_RejectsEscapingFilename(t *testing.T) {
	for _, badName := range []string{"../outside.dump", "volumes/../../outside.dump", "/etc/passwd", "", "."} {
		t.Run(badName, func(t *testing.T) {
			tmpDir := t.TempDir()
			m := NewManifest("1.0.0")
			m.AddVolume(VolumeBackup{
				Name:     "test",
				Type:     BackupTypePostgreSQL,
				Filename: badName,
				Size:     1,
			})
			if err := m.Verify(tmpDir); err == nil {
				t.Errorf("expected Verify to reject escaping/invalid filename %q", badName)
			}
		})
	}
}

func TestManifest_Verify_RejectsSymlinkLeafOutsideBackupDir(t *testing.T) {
	outsideDir := t.TempDir()
	secretFile := filepath.Join(outsideDir, "secret.dump")
	content := []byte("outside-secret-payload")
	if err := os.WriteFile(secretFile, content, fsperm.SecretFile); err != nil {
		t.Fatalf("writing outside file: %v", err)
	}
	checksum, err := FileChecksum(secretFile)
	if err != nil {
		t.Fatalf("computing checksum: %v", err)
	}

	backupDir := t.TempDir()
	volumesDir := filepath.Join(backupDir, "volumes")
	if err := os.MkdirAll(volumesDir, fsperm.SecretDir); err != nil {
		t.Fatalf("creating volumes dir: %v", err)
	}
	if err := os.Symlink(secretFile, filepath.Join(volumesDir, "db.dump")); err != nil {
		t.Fatalf("creating leaf symlink: %v", err)
	}

	m := NewManifest("1.0.0")
	m.AddVolume(VolumeBackup{
		Name:     "db_data_17",
		Type:     BackupTypePostgreSQL,
		Filename: "volumes/db.dump",
		Size:     int64(len(content)),
		Checksum: checksum,
	})
	m.Finalize()

	if err := m.Verify(backupDir); err == nil {
		t.Fatal("expected Verify to reject leaf symlink escaping backupDir")
	}
}

func TestManifest_Verify_RejectsSymlinkDirectoryOutsideBackupDir(t *testing.T) {
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "db.dump")
	content := []byte("outside-dir-payload")
	if err := os.WriteFile(outsideFile, content, fsperm.SecretFile); err != nil {
		t.Fatalf("writing outside file: %v", err)
	}
	checksum, err := FileChecksum(outsideFile)
	if err != nil {
		t.Fatalf("computing checksum: %v", err)
	}

	backupDir := t.TempDir()
	if err := os.Symlink(outsideDir, filepath.Join(backupDir, "volumes")); err != nil {
		t.Fatalf("creating directory symlink: %v", err)
	}

	m := NewManifest("1.0.0")
	m.AddVolume(VolumeBackup{
		Name:     "db_data_17",
		Type:     BackupTypePostgreSQL,
		Filename: "volumes/db.dump",
		Size:     int64(len(content)),
		Checksum: checksum,
	})
	m.Finalize()

	if err := m.Verify(backupDir); err == nil {
		t.Fatal("expected Verify to reject directory symlink escaping backupDir")
	}
}

func TestManifest_Verify_RejectsInternalSymlinkAndNonRegularFile(t *testing.T) {
	t.Run("internal leaf symlink", func(t *testing.T) {
		backupDir := t.TempDir()
		volumesDir := filepath.Join(backupDir, "volumes")
		if err := os.MkdirAll(volumesDir, fsperm.SecretDir); err != nil {
			t.Fatalf("creating volumes dir: %v", err)
		}
		realFile := filepath.Join(volumesDir, "real.dump")
		content := []byte("internal-payload")
		if err := os.WriteFile(realFile, content, fsperm.SecretFile); err != nil {
			t.Fatalf("writing real file: %v", err)
		}
		checksum, err := FileChecksum(realFile)
		if err != nil {
			t.Fatalf("computing checksum: %v", err)
		}
		if err := os.Symlink("real.dump", filepath.Join(volumesDir, "link.dump")); err != nil {
			t.Fatalf("creating internal symlink: %v", err)
		}

		m := NewManifest("1.0.0")
		m.AddVolume(VolumeBackup{
			Name:     "db_data_17",
			Type:     BackupTypePostgreSQL,
			Filename: "volumes/link.dump",
			Size:     int64(len(content)),
			Checksum: checksum,
		})
		m.Finalize()

		if err := m.Verify(backupDir); err == nil {
			t.Fatal("expected Verify to reject internal leaf symlink")
		}
	})

	t.Run("internal directory symlink", func(t *testing.T) {
		backupDir := t.TempDir()
		realDir := filepath.Join(backupDir, "real_volumes")
		if err := os.MkdirAll(realDir, fsperm.SecretDir); err != nil {
			t.Fatalf("creating real dir: %v", err)
		}
		realFile := filepath.Join(realDir, "db.dump")
		content := []byte("internal-dir-payload")
		if err := os.WriteFile(realFile, content, fsperm.SecretFile); err != nil {
			t.Fatalf("writing real file: %v", err)
		}
		checksum, err := FileChecksum(realFile)
		if err != nil {
			t.Fatalf("computing checksum: %v", err)
		}
		if err := os.Symlink("real_volumes", filepath.Join(backupDir, "volumes")); err != nil {
			t.Fatalf("creating internal dir symlink: %v", err)
		}

		m := NewManifest("1.0.0")
		m.AddVolume(VolumeBackup{
			Name:     "db_data_17",
			Type:     BackupTypePostgreSQL,
			Filename: "volumes/db.dump",
			Size:     int64(len(content)),
			Checksum: checksum,
		})
		m.Finalize()

		if err := m.Verify(backupDir); err == nil {
			t.Fatal("expected Verify to reject internal directory symlink")
		}
	})

	t.Run("directory instead of regular file", func(t *testing.T) {
		backupDir := t.TempDir()
		notAFile := filepath.Join(backupDir, "volumes", "db.dump")
		if err := os.MkdirAll(notAFile, fsperm.SecretDir); err != nil {
			t.Fatalf("creating directory at file path: %v", err)
		}
		info, err := os.Stat(notAFile)
		if err != nil {
			t.Fatalf("stat directory: %v", err)
		}

		m := NewManifest("1.0.0")
		m.AddVolume(VolumeBackup{
			Name:     "db_data_17",
			Type:     BackupTypePostgreSQL,
			Filename: "volumes/db.dump",
			Size:     info.Size(),
			Checksum: "sha256:ignored",
		})
		m.Finalize()

		if err := m.Verify(backupDir); err == nil {
			t.Fatal("expected Verify to reject non-regular file (directory)")
		}
	})
}

func TestResolveUnderBase_SymlinkContainmentAndBaseDirAlias(t *testing.T) {
	realBase := filepath.Join(t.TempDir(), "real_backup")
	volumesDir := filepath.Join(realBase, "volumes")
	if err := os.MkdirAll(volumesDir, fsperm.SecretDir); err != nil {
		t.Fatalf("creating volumes dir: %v", err)
	}
	goodFile := filepath.Join(volumesDir, "db.dump")
	if err := os.WriteFile(goodFile, []byte("ok"), fsperm.SecretFile); err != nil {
		t.Fatalf("writing good file: %v", err)
	}

	// Simulate a symlinked host parent directory (e.g. /var -> /private/var).
	aliasBase := filepath.Join(t.TempDir(), "alias_backup")
	if err := os.Symlink(realBase, aliasBase); err != nil {
		t.Fatalf("creating base dir symlink: %v", err)
	}

	got, err := resolveUnderBase(aliasBase, "volumes/db.dump")
	if err != nil {
		t.Fatalf("expected valid regular file under symlinked baseDir to pass, got: %v", err)
	}
	want, err := filepath.EvalSymlinks(goodFile)
	if err != nil {
		t.Fatalf("EvalSymlinks(goodFile): %v", err)
	}
	if got != want {
		t.Errorf("resolveUnderBase() = %q, want %q", got, want)
	}

	outsideFile := filepath.Join(t.TempDir(), "outside.dump")
	if err := os.WriteFile(outsideFile, []byte("secret"), fsperm.SecretFile); err != nil {
		t.Fatalf("writing outside file: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(volumesDir, "escape.dump")); err != nil {
		t.Fatalf("creating escape symlink: %v", err)
	}
	if _, err := resolveUnderBase(aliasBase, "volumes/escape.dump"); err == nil {
		t.Fatal("expected resolveUnderBase to reject symlink escaping baseDir")
	}

	if _, err := resolveUnderBase(aliasBase, "volumes"); err == nil {
		t.Fatal("expected resolveUnderBase to reject directory target")
	}
}

func TestBackupDir(t *testing.T) {
	baseDir := "/backups"
	dir := BackupDir(baseDir)

	if filepath.Dir(dir) != baseDir {
		t.Errorf("Expected base dir %s, got %s", baseDir, filepath.Dir(dir))
	}

	// Check timestamp format (YYYYMMDD_HHMMSS)
	timestamp := filepath.Base(dir)
	if len(timestamp) != 15 { // 20060102_150405
		t.Errorf("Unexpected timestamp format: %s (len=%d)", timestamp, len(timestamp))
	}

	// Parse timestamp - should be valid
	_, err := time.Parse("20060102_150405", timestamp)
	if err != nil {
		t.Errorf("Failed to parse timestamp: %v", err)
	}
}
