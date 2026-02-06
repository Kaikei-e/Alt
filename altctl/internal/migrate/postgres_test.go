package migrate

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestPostgresBackuper_BackupRejectsTarType(t *testing.T) {
	pg := NewPostgresBackuper("alt", slog.Default(), true)
	spec := VolumeSpec{
		Name:       "meili_data",
		BackupType: BackupTypeTar,
	}

	err := pg.Backup(context.Background(), spec, "/tmp/test.dump")
	if err == nil {
		t.Error("expected error for non-PostgreSQL volume")
	}
}

func TestPostgresBackuper_RestoreRejectsTarType(t *testing.T) {
	pg := NewPostgresBackuper("alt", slog.Default(), true)
	spec := VolumeSpec{
		Name:       "meili_data",
		BackupType: BackupTypeTar,
	}

	err := pg.Restore(context.Background(), spec, "/tmp/test.dump")
	if err == nil {
		t.Error("expected error for non-PostgreSQL volume")
	}
}

func TestPostgresBackuper_BackupDryRun(t *testing.T) {
	pg := NewPostgresBackuper("alt", slog.Default(), true)
	spec := VolumeSpec{
		Name:       "db_data_17",
		Service:     "db",
		BackupType: BackupTypePostgreSQL,
		DBName:     "alt",
		DBUser:     "alt_db_user",
		DBPort:     5432,
	}

	outputPath := filepath.Join(t.TempDir(), "test.dump")
	err := pg.Backup(context.Background(), spec, outputPath)
	if err != nil {
		t.Errorf("dry-run backup should not fail: %v", err)
	}

	// File should not be created in dry-run
	if _, err := os.Stat(outputPath); err == nil {
		t.Error("file should not be created in dry-run mode")
	}
}

func TestPostgresBackuper_RestoreDryRun(t *testing.T) {
	pg := NewPostgresBackuper("alt", slog.Default(), true)
	spec := VolumeSpec{
		Name:       "db_data_17",
		Service:     "db",
		BackupType: BackupTypePostgreSQL,
		DBName:     "alt",
		DBUser:     "alt_db_user",
		DBPort:     5432,
	}

	// Create a dummy file to restore from
	inputPath := filepath.Join(t.TempDir(), "test.dump")
	if err := os.WriteFile(inputPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	err := pg.Restore(context.Background(), spec, inputPath)
	if err != nil {
		t.Errorf("dry-run restore should not fail: %v", err)
	}
}

func TestPostgresBackuper_RestoreFileNotFound(t *testing.T) {
	pg := NewPostgresBackuper("alt", slog.Default(), false)
	spec := VolumeSpec{
		Name:       "db_data_17",
		Service:     "db",
		BackupType: BackupTypePostgreSQL,
		DBName:     "alt",
		DBUser:     "alt_db_user",
		DBPort:     5432,
	}

	err := pg.Restore(context.Background(), spec, "/nonexistent/test.dump")
	if err == nil {
		t.Error("expected error for missing input file")
	}
}

func TestPostgresBackuper_ContainerName(t *testing.T) {
	pg := NewPostgresBackuper("alt", slog.Default(), true)

	tests := []struct {
		spec VolumeSpec
		want string
	}{
		{
			VolumeSpec{Name: "db_data_17", Service: "db"},
			"alt-db",
		},
		{
			VolumeSpec{Name: "kratos_db_data", Service: "kratos-db"},
			"alt-kratos-db-1",
		},
		{
			VolumeSpec{Name: "recap_db_data", Service: "recap-db"},
			"recap-db",
		},
		{
			VolumeSpec{Name: "rag_db_data", Service: "rag-db"},
			"rag-db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.spec.Name, func(t *testing.T) {
			got := pg.containerName(tt.spec)
			if got != tt.want {
				t.Errorf("containerName(%s) = %q, want %q", tt.spec.Name, got, tt.want)
			}
		})
	}
}

func TestPostgresBackuper_DumpFilename(t *testing.T) {
	pg := NewPostgresBackuper("alt", slog.Default(), true)
	spec := VolumeSpec{Name: "db_data_17", DBName: "alt"}

	got := pg.dumpFilename(spec)
	if got != "db_data_17.dump" {
		t.Errorf("dumpFilename() = %q, want %q", got, "db_data_17.dump")
	}
}

func TestRegistryPostgreSQLVolumes(t *testing.T) {
	r := NewVolumeRegistry()
	pgVolumes := r.PostgreSQL()

	if len(pgVolumes) != 4 {
		t.Errorf("Expected 4 PostgreSQL volumes, got %d", len(pgVolumes))
	}

	expectedPG := map[string]bool{
		"db_data_17":     true,
		"kratos_db_data": true,
		"recap_db_data":  true,
		"rag_db_data":    true,
	}

	for _, v := range pgVolumes {
		if v.BackupType != BackupTypePostgreSQL {
			t.Errorf("Volume %s should be PostgreSQL type, got %s", v.Name, v.BackupType)
		}
		if !expectedPG[v.Name] {
			t.Errorf("Unexpected PostgreSQL volume: %s", v.Name)
		}
		if v.DBName == "" {
			t.Errorf("PostgreSQL volume %s should have DBName set", v.Name)
		}
		if v.DBUser == "" {
			t.Errorf("PostgreSQL volume %s should have DBUser set", v.Name)
		}
	}
}

func TestRegistryTarVolumesAfterPGChange(t *testing.T) {
	r := NewVolumeRegistry()
	tarVolumes := r.Tar()

	// 12 total - 4 PG = 8 tar
	if len(tarVolumes) != 8 {
		t.Errorf("Expected 8 tar volumes, got %d", len(tarVolumes))
	}

	// PG volumes should NOT be in tar list
	for _, v := range tarVolumes {
		if v.BackupType != BackupTypeTar {
			t.Errorf("Tar list contains non-tar volume: %s (%s)", v.Name, v.BackupType)
		}
	}
}
