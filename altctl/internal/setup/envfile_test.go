package setup

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

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

func TestCreateEnvFile_CreatesFromExample(t *testing.T) {
	dir := t.TempDir()

	// Create .env.example
	example := "POSTGRES_DB=alt\nPOSTGRES_USER=alt_user\n"
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte(example), fsperm.SecretFile); err != nil {
		t.Fatal(err)
	}

	created, err := CreateEnvFile(dir, false)
	if err != nil {
		t.Fatalf("CreateEnvFile failed: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}

	content := readFileIn(t, dir, ".env")
	if string(content) != example {
		t.Errorf("expected .env to match .env.example, got %q", content)
	}
}

func TestCreateEnvFile_SkipsExisting(t *testing.T) {
	dir := t.TempDir()

	// Create .env.example and existing .env
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.example"), []byte("NEW=value"), fsperm.SecretFile))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("OLD=value"), fsperm.SecretFile))

	created, err := CreateEnvFile(dir, false)
	if err != nil {
		t.Fatalf("CreateEnvFile failed: %v", err)
	}
	if created {
		t.Error("expected created=false when .env exists")
	}

	// Should not overwrite
	content := readFileIn(t, dir, ".env")
	if string(content) != "OLD=value" {
		t.Error("should not overwrite existing .env")
	}
}

func TestCreateEnvFile_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.example"), []byte("NEW=value"), fsperm.SecretFile))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("OLD=value"), fsperm.SecretFile))

	created, err := CreateEnvFile(dir, true)
	if err != nil {
		t.Fatalf("CreateEnvFile failed: %v", err)
	}
	if !created {
		t.Error("expected created=true with force")
	}

	content := readFileIn(t, dir, ".env")
	if string(content) != "NEW=value" {
		t.Error("force should overwrite .env")
	}
}

func TestCreateEnvFile_NoExample(t *testing.T) {
	dir := t.TempDir()

	created, err := CreateEnvFile(dir, false)
	require.ErrorContains(t, err, ".env.example")
	require.False(t, created)
}

func TestCreateEnvFile_NewFileHasMode0600(t *testing.T) {
	oldUmask := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(oldUmask) })

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.example"), []byte("A=1\n"), fsperm.GeneratedFile))

	created, err := CreateEnvFile(dir, false)
	require.NoError(t, err)
	require.True(t, created)

	info, err := os.Lstat(filepath.Join(dir, ".env"))
	require.NoError(t, err)
	require.Equal(t, fsperm.SecretFile, info.Mode().Perm())
}

func TestCreateEnvFile_ForceTightensExistingPermissionsTo0600(t *testing.T) {
	oldUmask := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(oldUmask) })

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.example"), []byte("NEW=value\n"), fsperm.GeneratedFile))
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("OLD=value\n"), fsperm.GeneratedFile))
	require.NoError(t, os.Chmod(envPath, fsperm.GeneratedFile))

	created, err := CreateEnvFile(dir, true)
	require.NoError(t, err)
	require.True(t, created)

	require.Equal(t, "NEW=value\n", string(readFileIn(t, dir, ".env")))
	info, err := os.Lstat(envPath)
	require.NoError(t, err)
	require.Equal(t, fsperm.SecretFile, info.Mode().Perm())
}

func TestCreateEnvFile_RejectsDanglingEnvSymlink(t *testing.T) {
	for _, force := range []bool{false, true} {
		name := "force=false"
		if force {
			name = "force=true"
		}
		t.Run(name, func(t *testing.T) {
			victim := filepath.Join(t.TempDir(), "victim")
			proj := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(proj, ".env.example"), []byte("A=1\n"), fsperm.SecretFile))
			require.NoError(t, os.Symlink(victim, filepath.Join(proj, ".env")))

			created, err := CreateEnvFile(proj, force)
			require.Error(t, err)
			require.False(t, created)

			_, statErr := os.Stat(victim)
			require.ErrorIs(t, statErr, os.ErrNotExist, "CreateEnvFile must not create victim through dangling symlink")
		})
	}
}

func TestCreateEnvFile_RejectsLiveEnvSymlinkOutsideProjectRoot(t *testing.T) {
	for _, force := range []bool{false, true} {
		name := "force=false"
		if force {
			name = "force=true"
		}
		t.Run(name, func(t *testing.T) {
			outsideDir := t.TempDir()
			victim := filepath.Join(outsideDir, "target.env")
			require.NoError(t, os.WriteFile(victim, []byte("ORIGINAL=secret\n"), fsperm.SecretFile))

			proj := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(proj, ".env.example"), []byte("OVERWRITTEN=1\n"), fsperm.SecretFile))
			require.NoError(t, os.Symlink(victim, filepath.Join(proj, ".env")))

			created, err := CreateEnvFile(proj, force)
			require.Error(t, err)
			require.False(t, created)

			require.Equal(t, "ORIGINAL=secret\n", string(readFileIn(t, outsideDir, "target.env")))
		})
	}
}

func TestCreateEnvFile_RejectsNonRegularEnv(t *testing.T) {
	for _, force := range []bool{false, true} {
		name := "force=false"
		if force {
			name = "force=true"
		}
		t.Run(name, func(t *testing.T) {
			proj := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(proj, ".env.example"), []byte("A=1\n"), fsperm.SecretFile))
			require.NoError(t, os.Mkdir(filepath.Join(proj, ".env"), fsperm.SecretDir))

			created, err := CreateEnvFile(proj, force)
			require.Error(t, err)
			require.False(t, created)
		})
	}
}

func TestCreateEnvFile_RejectsEscapingExampleSymlink(t *testing.T) {
	outsideDir := t.TempDir()
	outsideSecret := filepath.Join(outsideDir, "secret.env")
	require.NoError(t, os.WriteFile(outsideSecret, []byte("TOP_SECRET=1\n"), fsperm.SecretFile))

	proj := t.TempDir()
	require.NoError(t, os.Symlink(outsideSecret, filepath.Join(proj, ".env.example")))

	created, err := CreateEnvFile(proj, false)
	require.Error(t, err)
	require.False(t, created)

	_, statErr := os.Lstat(filepath.Join(proj, ".env"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}
