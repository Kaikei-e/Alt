package migrate

import (
	"context"
	"log/slog"
	"testing"
)

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
