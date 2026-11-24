package guards

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var envAllowedSuffixes = []string{".example", ".template", ".sample"}
var keySuffixes = []string{".key", ".pem"}
var protectedDirs = map[string]struct{}{"secrets": {}, ".kisage": {}, ".git": {}}

// RunProtected blocks edits to env files, keys, secrets, and tracked migrations.
func RunProtected(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	_ = ctx
	_ = args
	_ = stdout
	if stderr == nil {
		stderr = io.Discard
	}
	payload, code := readEditPayload(stdin, stderr)
	if code >= 0 {
		return code
	}
	var violations []string
	for _, path := range payload.Paths {
		if msg := CheckProtectedPath(path); msg != "" {
			violations = append(violations, msg)
		}
	}
	if len(violations) > 0 {
		fmt.Fprintln(stderr, strings.Join(violations, "\n"))
		return 2
	}
	return 0
}

// CheckProtectedPath returns a block message when rawPath is protected.
func CheckProtectedPath(rawPath string) string {
	norm := filepath.Clean(rawPath)
	name := filepath.Base(norm)
	dir := filepath.Dir(norm)
	var parts []string
	if dir != "." && dir != string(filepath.Separator) {
		parts = strings.Split(filepath.ToSlash(dir), "/")
	}
	if isProtectedEnvName(name) {
		return fmt.Sprintf("Blocked edit to %s: environment files matching .env are protected unless they end in .example, .template, or .sample. Change host env through deploy/desired-env.lkal and the kisage tools, or edit a tracked template file instead.", rawPath)
	}
	for _, suf := range keySuffixes {
		if strings.HasSuffix(name, suf) {
			return fmt.Sprintf("Blocked edit to %s: private key and certificate files (*.key, *.pem) must not be modified directly. Keys and certificates are issued by tooling, not written by agents; ask the user if one has to change.", rawPath)
		}
	}
	for _, seg := range parts {
		if _, ok := protectedDirs[seg]; !ok {
			continue
		}
		next := "Use standard git CLI commands instead of editing .git internals directly."
		switch seg {
		case ".kisage":
			next = "Change host env through deploy/desired-env.lkal and the kisage tools."
		case "secrets":
			next = "Secret files are written by the host operator; ask the user to change them."
		}
		return fmt.Sprintf("Blocked edit to %s: files inside a '%s' directory are protected. %s", rawPath, seg, next)
	}
	for _, seg := range parts {
		if seg != "migrations" {
			continue
		}
		blocked, gitErr := trackedInGit(rawPath)
		if !blocked {
			return ""
		}
		if gitErr != "" {
			return fmt.Sprintf("Blocked edit to %s: migration tracking status could not be verified by git (%s). Ensure you are inside the git repository, add a new migration file, and regenerate atlas.sum with atlas migrate hash.", rawPath, gitErr)
		}
		return fmt.Sprintf("Blocked edit to %s: tracked files under a 'migrations' directory are immutable once applied. Add a new migration file and regenerate atlas.sum with atlas migrate hash.", rawPath)
	}
	return ""
}

func isProtectedEnvName(name string) bool {
	isEnv := name == ".env" || strings.HasPrefix(name, ".env.") || strings.HasSuffix(name, ".env")
	if !isEnv {
		return false
	}
	for _, suf := range envAllowedSuffixes {
		if strings.HasSuffix(name, suf) {
			return false
		}
	}
	return true
}

func trackedInGit(rawPath string) (bool, string) {
	abs, err := filepath.Abs(rawPath)
	if err != nil {
		return true, err.Error()
	}
	probe := filepath.Dir(abs)
	for {
		if st, err := os.Stat(probe); err == nil && st.IsDir() {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return true, "no existing parent directory was found to verify git tracking"
		}
		probe = parent
	}
	tail, err := filepath.Rel(probe, abs)
	if err != nil {
		return true, err.Error()
	}
	probeReal, err := filepath.EvalSymlinks(probe)
	if err != nil {
		probeReal = probe
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return true, "git could not be executed (git executable not found on PATH)"
	}
	cmd := exec.Command(git, "-C", probeReal, "ls-files", "--error-unmatch", "--", tail)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, ""
	}
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
		return false, ""
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	return true, detail
}
