package migrate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
	if err := os.WriteFile(testFile, content, 0644); err != nil {
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
	if err := os.WriteFile(testFile2, content, 0644); err != nil {
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
	if err := os.MkdirAll(postgresDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	testFile := filepath.Join(postgresDir, "test.dump")
	content := []byte("test backup data")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
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
	if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	if err := m.Verify(tmpDir); err == nil {
		t.Error("Verify should fail after file modification")
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
