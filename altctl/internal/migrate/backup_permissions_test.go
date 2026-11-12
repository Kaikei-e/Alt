package migrate

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestManifest_Save_IsOwnerOnlyUnderPermissiveUmask(t *testing.T) {
	old := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(old) })

	path := filepath.Join(t.TempDir(), ManifestFilename)
	if err := NewManifest("test").Save(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("manifest mode = %#o, want 0600", got)
	}
}

// A full db-profile backup under umask 0: everything the command creates must
// be owner-only, and no child process may receive a credential in its argv.
func TestMigrator_Backup_CreatesOnlyOwnerOnlyPathsAndNoArgvSecrets(t *testing.T) {
	old := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(old) })

	binDir := t.TempDir()
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	script := "#!/usr/bin/env sh\necho \"$@\" >> " + argvLog + "\n" +
		"if [ \"$1\" = \"exec\" ]; then printf 'PGDMP-fake-content'; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	outRoot := filepath.Join(t.TempDir(), "backups")
	m := NewMigrator("", "alt", slog.Default(), false)
	result, err := m.Backup(context.Background(), BackupOptions{
		OutputDir: outRoot,
		Force:     true,
		Profile:   ProfileDB,
		Compress:  true,
	})
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	paths := []string{outRoot, result.ArchivePath}
	_ = filepath.WalkDir(outRoot, func(p string, d fs.DirEntry, err error) error {
		if err == nil && p != outRoot {
			paths = append(paths, p)
		}
		return nil
	})
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o600)
		if info.IsDir() {
			want = 0o700
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %#o, want %#o", p, got, want)
		}
	}

	argv, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(argv)), "password") || strings.Contains(string(argv), "PGPASS") {
		t.Errorf("a child process received a credential on its command line:\n%s", argv)
	}
}
