package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetBackupStatus_NoBackups(t *testing.T) {
	dir := t.TempDir()

	status, err := GetBackupStatus(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.HasBackup {
		t.Error("expected HasBackup to be false")
	}
	if status.Health != HealthCritical {
		t.Errorf("expected HealthCritical, got %s", status.Health)
	}
	if status.ExpectedVolumes != len(defaultVolumes) {
		t.Errorf("expected %d expected volumes, got %d", len(defaultVolumes), status.ExpectedVolumes)
	}
}

func TestGetBackupStatus_Healthy(t *testing.T) {
	dir := t.TempDir()

	// Create a backup directory with valid manifest
	backupDir := filepath.Join(dir, "20260206_030000")
	volumesDir := filepath.Join(backupDir, "volumes")
	if err := os.MkdirAll(volumesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create dummy volume files and manifest
	manifest := &Manifest{
		Version:       ManifestVersion,
		CreatedAt:     time.Now().UTC().Add(-1 * time.Hour),
		AltctlVersion: "test",
		Volumes:       []VolumeBackup{},
	}

	registry := NewVolumeRegistry()
	for _, spec := range registry.All() {
		filename := spec.Name + ".tar.gz"
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
	manifestPath := filepath.Join(backupDir, ManifestFilename)
	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	status, err := GetBackupStatus(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !status.HasBackup {
		t.Error("expected HasBackup to be true")
	}
	if status.Health != HealthGood {
		t.Errorf("expected HealthGood, got %s", status.Health)
	}
	if status.ActualVolumes != len(defaultVolumes) {
		t.Errorf("expected %d actual volumes, got %d", len(defaultVolumes), status.ActualVolumes)
	}
	if status.ExpectedVolumes != len(defaultVolumes) {
		t.Errorf("expected %d expected volumes, got %d", len(defaultVolumes), status.ExpectedVolumes)
	}
	if status.ChecksumOK != true {
		t.Error("expected checksums to be valid")
	}
}

func TestGetBackupStatus_StaleBackup(t *testing.T) {
	dir := t.TempDir()

	// Create a backup directory with old timestamp
	backupDir := filepath.Join(dir, "20260101_030000")
	volumesDir := filepath.Join(backupDir, "volumes")
	if err := os.MkdirAll(volumesDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifest := &Manifest{
		Version:       ManifestVersion,
		CreatedAt:     time.Now().UTC().Add(-48 * time.Hour), // 2 days old
		AltctlVersion: "test",
		Volumes:       []VolumeBackup{},
	}

	// Add just one volume
	filename := "db_data_17.tar.gz"
	filePath := filepath.Join(volumesDir, filename)
	if err := os.WriteFile(filePath, []byte("test-data"), 0644); err != nil {
		t.Fatal(err)
	}
	checksum, _ := FileChecksum(filePath)
	info, _ := os.Stat(filePath)
	manifest.AddVolume(VolumeBackup{
		Name:       "db_data_17",
		Type:       BackupTypeTar,
		Filename:   filepath.Join("volumes", filename),
		Size:       info.Size(),
		Checksum:   checksum,
		Service:    "db",
		BackedUpAt: manifest.CreatedAt,
	})
	manifest.Finalize()

	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(backupDir, ManifestFilename), data, 0644); err != nil {
		t.Fatal(err)
	}

	status, err := GetBackupStatus(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !status.HasBackup {
		t.Error("expected HasBackup to be true")
	}
	// Stale backup (> 25h) should be warning
	if status.Health != HealthWarning {
		t.Errorf("expected HealthWarning for stale backup, got %s", status.Health)
	}
	// Missing volumes
	if status.ActualVolumes >= status.ExpectedVolumes {
		t.Error("expected missing volumes")
	}
}

func TestGetBackupStatus_MissingVolumes(t *testing.T) {
	dir := t.TempDir()

	backupDir := filepath.Join(dir, "20260206_020000")
	volumesDir := filepath.Join(backupDir, "volumes")
	if err := os.MkdirAll(volumesDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifest := &Manifest{
		Version:       ManifestVersion,
		CreatedAt:     time.Now().UTC().Add(-30 * time.Minute),
		AltctlVersion: "test",
		Volumes:       []VolumeBackup{},
	}

	// Only add 2 volumes
	for _, name := range []string{"db_data_17", "meili_data"} {
		filename := name + ".tar.gz"
		filePath := filepath.Join(volumesDir, filename)
		if err := os.WriteFile(filePath, []byte("test-data-"+name), 0644); err != nil {
			t.Fatal(err)
		}
		checksum, _ := FileChecksum(filePath)
		info, _ := os.Stat(filePath)
		manifest.AddVolume(VolumeBackup{
			Name:       name,
			Type:       BackupTypeTar,
			Filename:   filepath.Join("volumes", filename),
			Size:       info.Size(),
			Checksum:   checksum,
			Service:    "test",
			BackedUpAt: manifest.CreatedAt,
		})
	}
	manifest.Finalize()

	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(backupDir, ManifestFilename), data, 0644); err != nil {
		t.Fatal(err)
	}

	status, err := GetBackupStatus(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.ActualVolumes != 2 {
		t.Errorf("expected 2 actual volumes, got %d", status.ActualVolumes)
	}
	if len(status.MissingVolumes) == 0 {
		t.Error("expected missing volumes to be reported")
	}
	// Recent but missing volumes → warning
	if status.Health != HealthWarning {
		t.Errorf("expected HealthWarning for missing volumes, got %s", status.Health)
	}
}

func TestHealthString(t *testing.T) {
	tests := []struct {
		health HealthLevel
		want   string
	}{
		{HealthGood, "GOOD"},
		{HealthWarning, "WARNING"},
		{HealthCritical, "CRITICAL"},
	}
	for _, tt := range tests {
		if got := tt.health.String(); got != tt.want {
			t.Errorf("HealthLevel.String() = %v, want %v", got, tt.want)
		}
	}
}
