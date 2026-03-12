package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateEnvFile_CreatesFromExample(t *testing.T) {
	dir := t.TempDir()

	// Create .env.example
	example := "POSTGRES_DB=alt\nPOSTGRES_USER=alt_user\n"
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte(example), 0644); err != nil {
		t.Fatal(err)
	}

	created, err := CreateEnvFile(dir, false)
	if err != nil {
		t.Fatalf("CreateEnvFile failed: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}

	content, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != example {
		t.Errorf("expected .env to match .env.example, got %q", content)
	}
}

func TestCreateEnvFile_SkipsExisting(t *testing.T) {
	dir := t.TempDir()

	// Create .env.example and existing .env
	os.WriteFile(filepath.Join(dir, ".env.example"), []byte("NEW=value"), 0644)
	os.WriteFile(filepath.Join(dir, ".env"), []byte("OLD=value"), 0644)

	created, err := CreateEnvFile(dir, false)
	if err != nil {
		t.Fatalf("CreateEnvFile failed: %v", err)
	}
	if created {
		t.Error("expected created=false when .env exists")
	}

	// Should not overwrite
	content, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if string(content) != "OLD=value" {
		t.Error("should not overwrite existing .env")
	}
}

func TestCreateEnvFile_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, ".env.example"), []byte("NEW=value"), 0644)
	os.WriteFile(filepath.Join(dir, ".env"), []byte("OLD=value"), 0644)

	created, err := CreateEnvFile(dir, true)
	if err != nil {
		t.Fatalf("CreateEnvFile failed: %v", err)
	}
	if !created {
		t.Error("expected created=true with force")
	}

	content, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if string(content) != "NEW=value" {
		t.Error("force should overwrite .env")
	}
}

func TestCreateEnvFile_NoExample(t *testing.T) {
	dir := t.TempDir()

	_, err := CreateEnvFile(dir, false)
	if err == nil {
		t.Error("expected error when .env.example missing")
	}
}
