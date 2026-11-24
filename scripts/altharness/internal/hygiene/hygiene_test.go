package hygiene

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestZeroByteChecks(t *testing.T) {
	t.Parallel()

	t.Run("pre-existing 0-byte tracked files in HEAD pass", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		gitInit(t, root)
		writeBytes(t, root, "app/main.py", []byte("print('ok')\n"))
		writeBytes(t, root, "app/__init__.py", nil)
		writeBytes(t, root, "data/.gitkeep", nil)
		writeBytes(t, root, ".c2quay/locks/production.lock", nil)
		gitAdd(t, root, ".")
		gitCommit(t, root, "init")

		code, out, errb := runHygiene(root)
		if code != 0 {
			t.Fatalf("code=%d out=%s err=%s", code, out, errb)
		}
	})

	t.Run("tracked truncation unstaged and staged fails", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		gitInit(t, root)
		writeBytes(t, root, "recap-subworker/runs.py", []byte("def run():\n    return 1\n"))
		gitAdd(t, root, ".")
		gitCommit(t, root, "init")

		writeBytes(t, root, "recap-subworker/runs.py", nil)
		code, out, errb := runHygiene(root)
		if code != 1 || !strings.Contains(out+errb, "[hygiene.zero-byte-tracked]") {
			t.Fatalf("unstaged code=%d out=%s err=%s", code, out, errb)
		}

		gitAdd(t, root, "recap-subworker/runs.py")
		code, out, errb = runHygiene(root)
		if code != 1 || !strings.Contains(out+errb, "[hygiene.zero-byte-tracked]") {
			t.Fatalf("staged code=%d out=%s err=%s", code, out, errb)
		}
	})

	t.Run("untracked and staged allowlist behavior", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		gitInit(t, root)
		writeBytes(t, root, "README.md", []byte("# test\n"))
		gitAdd(t, root, ".")
		gitCommit(t, root, "init")

		writeBytes(t, root, "__init__.py", nil)
		writeBytes(t, root, "pkg/sub/__init__.py", nil)
		writeBytes(t, root, "empty_dir/.gitkeep", nil)
		code, out, errb := runHygiene(root)
		if code != 0 || !strings.Contains(out, "OK: worktree hygiene clean") {
			t.Fatalf("allowlisted untracked code=%d out=%s err=%s", code, out, errb)
		}

		writeBytes(t, root, "pkg/orphan.py", nil)
		code, out, errb = runHygiene(root)
		if code != 1 || !strings.Contains(out+errb, "[hygiene.zero-byte-untracked]") {
			t.Fatalf("non-allowlisted untracked code=%d out=%s err=%s", code, out, errb)
		}

		gitAdd(t, root, "pkg/orphan.py")
		code, out, errb = runHygiene(root)
		if code != 1 || !strings.Contains(out+errb, "[hygiene.zero-byte-staged]") {
			t.Fatalf("non-allowlisted staged code=%d out=%s err=%s", code, out, errb)
		}

		gitExec(t, root, "reset", "HEAD", "--", "pkg/orphan.py")
		if err := os.Remove(filepath.Join(root, "pkg", "orphan.py")); err != nil {
			t.Fatal(err)
		}
		gitAdd(t, root, "pkg/sub/__init__.py")
		code, out, errb = runHygiene(root)
		if code != 0 {
			t.Fatalf("allowlisted staged code=%d out=%s err=%s", code, out, errb)
		}
	})
}

func TestBinaryArtifacts(t *testing.T) {
	t.Parallel()

	t.Run("ELF untracked and staged", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		gitInit(t, root)
		writeBytes(t, root, "main.go", []byte("package main\n"))
		gitAdd(t, root, ".")
		gitCommit(t, root, "init")

		writeBytes(t, root, "alt-backend/app/alt-backend", append([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1}, bytes.Repeat([]byte{0}, 57)...))
		code, out, errb := runHygiene(root)
		if code != 1 || !strings.Contains(out, "[hygiene.binary-artifact] alt-backend/app/alt-backend: detected ELF binary artifact (untracked)") {
			t.Fatalf("elf untracked code=%d out=%s err=%s", code, out, errb)
		}
		gitAdd(t, root, "alt-backend/app/alt-backend")
		code, out, errb = runHygiene(root)
		if code != 1 || !strings.Contains(out, "[hygiene.binary-artifact]") {
			t.Fatalf("elf staged code=%d out=%s err=%s", code, out, errb)
		}
	})

	t.Run("Mach-O variants and Java class", func(t *testing.T) {
		t.Parallel()
		magics := []struct {
			name    string
			payload []byte
		}{
			{"macho-32-be", append([]byte{0xfe, 0xed, 0xfa, 0xce}, bytes.Repeat([]byte{0}, 28)...)},
			{"macho-32-le", append([]byte{0xce, 0xfa, 0xed, 0xfe}, bytes.Repeat([]byte{0}, 28)...)},
			{"macho-64-be", append([]byte{0xfe, 0xed, 0xfa, 0xcf}, bytes.Repeat([]byte{0}, 28)...)},
			{"macho-64-le", append([]byte{0xcf, 0xfa, 0xed, 0xfe}, bytes.Repeat([]byte{0}, 28)...)},
			{"macho-fat-2arch", makeMachOFat(2)},
		}
		for _, tc := range magics {
			root := t.TempDir()
			gitInit(t, root)
			writeBytes(t, root, "README.md", []byte("hi\n"))
			gitAdd(t, root, ".")
			gitCommit(t, root, "init")
			writeBytes(t, root, "bin/"+tc.name, tc.payload)
			code, out, errb := runHygiene(root)
			if code != 1 || !strings.Contains(out, "[hygiene.binary-artifact]") {
				t.Fatalf("%s code=%d out=%s err=%s", tc.name, code, out, errb)
			}
		}

		// Java .class (0xCAFEBABE with nfat_arch=52) must not false-positive.
		root := t.TempDir()
		gitInit(t, root)
		writeBytes(t, root, "README.md", []byte("hi\n"))
		gitAdd(t, root, ".")
		gitCommit(t, root, "init")
		writeBytes(t, root, "Sample.class", makeMachOFat(52))
		code, out, errb := runHygiene(root)
		if code != 0 {
			t.Fatalf("java class code=%d out=%s err=%s", code, out, errb)
		}
	})

	t.Run("PE binary vs MZ plain text", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		gitInit(t, root)
		writeBytes(t, root, "README.md", []byte("hi\n"))
		gitAdd(t, root, ".")
		gitCommit(t, root, "init")

		writeBytes(t, root, "short_mz.txt", []byte("MZ short text\n"))
		writeBytes(t, root, "long_mz.txt", append([]byte("MZ"), bytes.Repeat([]byte(" plain text line\n"), 10)...))
		code, out, errb := runHygiene(root)
		if code != 0 {
			t.Fatalf("mz text code=%d out=%s err=%s", code, out, errb)
		}

		writeBytes(t, root, "tool.exe", makePE(0x80))
		code, out, errb = runHygiene(root)
		if code != 1 || !strings.Contains(out, "[hygiene.binary-artifact] tool.exe: detected PE") {
			t.Fatalf("pe code=%d out=%s err=%s", code, out, errb)
		}
	})
}

func TestExclusionsCorpusAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("exclusions", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		gitInit(t, root)
		writeBytes(t, root, ".gitignore", []byte("ignored-bin\nignored-empty\n"))
		writeBytes(t, root, "stopped-using-k8s/old.yaml", []byte("kind: Pod\n"))
		writeBytes(t, root, "scripts/harness/corpus/C-99/before.py", []byte("x = 1\n"))
		gitAdd(t, root, ".")
		gitCommit(t, root, "init")

		elf := append([]byte{0x7f, 'E', 'L', 'F'}, bytes.Repeat([]byte{0}, 60)...)
		writeBytes(t, root, "stopped-using-k8s/old.yaml", nil)
		writeBytes(t, root, "stopped-using-k8s/legacy.elf", elf)
		writeBytes(t, root, "scripts/harness/corpus/C-99/before.py", nil)
		writeBytes(t, root, "scripts/harness/corpus/C-99/after.elf", elf)
		writeBytes(t, root, "ignored-bin", elf)
		writeBytes(t, root, "ignored-empty", nil)
		code, out, errb := runHygiene(root)
		if code != 0 {
			t.Fatalf("exclusions code=%d out=%s err=%s", code, out, errb)
		}
	})

	t.Run("not git and missing allowlist fail closed", func(t *testing.T) {
		t.Parallel()
		code, _, errb := runHygiene(t.TempDir())
		if code != 2 || !strings.Contains(errb, "not inside a git worktree") {
			t.Fatalf("non-git code=%d err=%s", code, errb)
		}

		root := t.TempDir()
		gitInit(t, root)
		writeBytes(t, root, "a.txt", []byte("ok\n"))
		gitAdd(t, root, ".")
		gitCommit(t, root, "init")
		code, _, errb = runHygiene(root, "--allowlist", filepath.Join(root, "missing.toml"))
		if code != 2 || !strings.Contains(errb, "allowlist file not found") {
			t.Fatalf("missing allowlist code=%d err=%s", code, errb)
		}
	})

	t.Run("real repository passes hygiene check", func(t *testing.T) {
		t.Parallel()
		root := repoRoot(t)
		code, out, errb := runHygiene(root)
		if code != 0 {
			t.Fatalf("real repo hygiene code=%d out=%s err=%s", code, out, errb)
		}
	})
}

func makeMachOFat(nfat uint32) []byte {
	buf := make([]byte, 40)
	copy(buf[:4], []byte{0xca, 0xfe, 0xba, 0xbe})
	binary.BigEndian.PutUint32(buf[4:8], nfat)
	return buf
}

func makePE(eLfanew uint32) []byte {
	buf := make([]byte, eLfanew+16)
	buf[0], buf[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(buf[0x3C:0x40], eLfanew)
	copy(buf[eLfanew:eLfanew+4], []byte{'P', 'E', 0, 0})
	return buf
}

func runHygiene(repo string, extra ...string) (int, string, string) {
	args := append([]string{"--repo-root", repo}, extra...)
	var out, errb bytes.Buffer
	code := Run(context.Background(), args, &out, &errb)
	return code, out.String(), errb.String()
}

func writeBytes(t *testing.T, root, rel string, content []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, content, fsperm.File); err != nil {
		t.Fatal(err)
	}
}

func gitExec(t *testing.T, root string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = root
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func gitInit(t *testing.T, root string) {
	t.Helper()
	gitExec(t, root, "init", "-q")
	gitExec(t, root, "config", "user.email", "t@example.com")
	gitExec(t, root, "config", "user.name", "t")
}

func gitAdd(t *testing.T, root, path string) {
	t.Helper()
	gitExec(t, root, "add", path)
}

func gitCommit(t *testing.T, root, msg string) {
	t.Helper()
	gitExec(t, root, "commit", "-q", "-m", msg)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
