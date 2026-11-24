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
		if msg := CheckProtectedPathInCwd(path, payload.Cwd); msg != "" {
			violations = append(violations, msg)
		}
	}
	if len(violations) > 0 {
		fmt.Fprintln(stderr, strings.Join(violations, "\n"))
		return 2
	}
	return 0
}

// CheckProtectedPath returns a block message when rawPath is protected,
// resolving relative paths against the current working directory.
func CheckProtectedPath(rawPath string) string {
	return CheckProtectedPathInCwd(rawPath, "")
}

// CheckProtectedPathInCwd returns a block message when rawPath is protected.
// Relative paths are resolved against cwd (falling back to os.Getwd()), and
// both the lexical and symlink-resolved paths are checked so in-repo symlinks
// (e.g. benign.txt -> .env) and relative subdir cwds cannot bypass the guard.
func CheckProtectedPathInCwd(rawPath, cwd string) string {
	candidates, migTargets := expandProtectedCandidates(rawPath, cwd)
	for _, cand := range candidates {
		if msg := checkStaticProtected(rawPath, cand); msg != "" {
			return msg
		}
	}
	for _, target := range migTargets {
		blocked, gitErr := trackedInGit(target)
		if !blocked {
			continue
		}
		if gitErr != "" {
			return fmt.Sprintf("Blocked edit to %s: migration tracking status could not be verified by git (%s). Ensure you are inside the git repository, add a new migration file, and regenerate atlas.sum with atlas migrate hash.", rawPath, gitErr)
		}
		return fmt.Sprintf("Blocked edit to %s: tracked files under a 'migrations' directory are immutable once applied. Add a new migration file and regenerate atlas.sum with atlas migrate hash.", rawPath)
	}
	return ""
}

func expandProtectedCandidates(rawPath, cwd string) (candidates []string, migTargets []string) {
	seen := map[string]struct{}{}
	addCand := func(p string) {
		if p == "" {
			return
		}
		clean := filepath.Clean(p)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		candidates = append(candidates, clean)
	}

	rawClean := filepath.Clean(rawPath)
	addCand(rawClean)

	base := strings.TrimSpace(cwd)
	if base == "" {
		if wd, err := os.Getwd(); err == nil {
			base = wd
		}
	}
	var joined string
	if filepath.IsAbs(rawPath) {
		joined = rawClean
	} else if base != "" {
		joined = filepath.Clean(filepath.Join(base, rawPath))
	} else {
		joined = rawClean
	}
	addCand(joined)

	resolved := ""
	if r, err := resolveNonStrict(joined); err == nil && r != "" {
		resolved = r
		addCand(resolved)
	}

	// Collect concrete target paths to verify via git ls-files when any
	// lexical or symlink-resolved representation lives under a 'migrations' dir.
	seenMig := map[string]struct{}{}
	addMig := func(p string) {
		if p == "" {
			return
		}
		if _, ok := seenMig[p]; ok {
			return
		}
		seenMig[p] = struct{}{}
		migTargets = append(migTargets, p)
	}
	if hasPathSegment(resolved, "migrations") {
		addMig(resolved)
	}
	if hasPathSegment(joined, "migrations") {
		if resolved != "" {
			addMig(resolved)
		} else {
			addMig(joined)
		}
	}
	if hasPathSegment(rawClean, "migrations") {
		if resolved != "" {
			addMig(resolved)
		} else {
			addMig(joined)
		}
	}
	return candidates, migTargets
}

func dirSegments(norm string) []string {
	dir := filepath.Dir(norm)
	if dir == "." || dir == string(filepath.Separator) {
		return nil
	}
	return strings.Split(filepath.ToSlash(dir), "/")
}

func hasPathSegment(norm, target string) bool {
	if norm == "" {
		return false
	}
	for _, seg := range dirSegments(norm) {
		if seg == target {
			return true
		}
	}
	return false
}

func checkStaticProtected(rawPath, norm string) string {
	name := filepath.Base(norm)
	parts := dirSegments(norm)
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
