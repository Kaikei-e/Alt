package guards

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var generatedHeaderRE = regexp.MustCompile(`(?i)^\s*(?://+|#+|/\*|\*|--|<!--|;)\s*Code generated\b[^\n]*DO NOT EDIT\.?`)

func repoRelIsGenerated(repoRoot, rel string) bool {
	norm := normalizeRepoRel(rel)
	if norm == "" {
		return false
	}
	repoResolved := resolveExisting(repoRoot)
	if repoResolved == "" {
		repoResolved = repoRoot
	}
	disk := filepath.Join(repoResolved, filepath.FromSlash(norm))
	if info, err := os.Lstat(disk); err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		if resolved, err := filepath.EvalSymlinks(disk); err == nil && pathInside(resolved, repoResolved) && fileHasGeneratedHeader(disk) {
			return true
		}
	}
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		return false
	}
	if gitRevHasGeneratedHeader(repoRoot, "HEAD", norm) {
		return true
	}
	return false
}

func normalizeRepoRel(path string) string {
	var parts []string
	for _, seg := range strings.Split(strings.TrimSpace(path), "/") {
		if seg != "" && seg != "." {
			parts = append(parts, seg)
		}
	}
	return strings.Join(parts, "/")
}

func fileHasGeneratedHeader(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	limit := 4096
	if len(raw) < limit {
		limit = len(raw)
	}
	if strings.Contains(string(raw[:limit]), "\x00") {
		return false
	}
	n := 8192
	if len(raw) < n {
		n = len(raw)
	}
	return hasGeneratedHeader(string(raw[:n]))
}

func hasGeneratedHeader(text string) bool {
	lines := strings.Split(text, "\n")
	if len(lines) > 40 {
		lines = lines[:40]
	}
	for _, line := range lines {
		if generatedHeaderRE.MatchString(line) {
			return true
		}
	}
	return false
}

func gitRevHasGeneratedHeader(repoRoot, rev, norm string) bool {
	git, err := exec.LookPath("git")
	if err != nil {
		return false
	}
	cmd := exec.Command(git, "-C", repoRoot, "show", "--end-of-options", rev+":"+norm)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	limit := 4096
	if len(out) < limit {
		limit = len(out)
	}
	if strings.Contains(string(out[:limit]), "\x00") {
		return false
	}
	n := 8192
	if len(out) < n {
		n = len(out)
	}
	return hasGeneratedHeader(string(out[:n]))
}
