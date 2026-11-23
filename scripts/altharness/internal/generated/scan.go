package generated

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	ruleDoNotEdit        = "codegen.do-not-edit-modified"
	ruleMissingToolMod   = "codegen.missing-tool-mod"
	ruleInvalidToolMod   = "codegen.invalid-tool-mod"
	ruleUnpinnedBuf      = "codegen.unpinned-buf-plugin"
	ruleUnpinnedLatest   = "codegen.unpinned-latest"
	ruleDriftGoCanonical = "codegen.drift.go-canonical"
	ruleDriftPythonPKI   = "codegen.drift.python-pki"
	ruleDriftOpenAPI     = "codegen.drift.openapi"
	ruleDriftProtovis    = "codegen.drift.protovis"
	ruleRegenToolMissing = "codegen.regen-tool-missing"
	ruleInvalidGitRev    = "codegen.invalid-git-rev"
	ruleGitUnavailable   = "codegen.git-unavailable"
	ruleGitDiffError     = "codegen.git-diff-error"
)

var excludedPrefixes = []string{"stopped-using-k8s/", "scripts/harness/corpus/"}

var excludedDirNames = map[string]struct{}{
	".git":         {},
	".venv":        {},
	"__pycache__":  {},
	"build":        {},
	"dist":         {},
	"node_modules": {},
	"target":       {},
	"vendor":       {},
}

var generatedHeaderRE = regexp.MustCompile(`(?i)^\s*(?://+|#+|/\*|\*|--|<!--|;)\s*Code generated .* DO NOT EDIT\.?\s*(?:\*/|-->)?\s*$`)

func isExcludedRel(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, prefix := range excludedPrefixes {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	for _, part := range strings.Split(rel, "/") {
		if _, ok := excludedDirNames[part]; ok {
			return true
		}
	}
	return false
}

func skipDirRel(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, prefix := range excludedPrefixes {
		base := strings.TrimSuffix(prefix, "/")
		if rel == base || strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

func (c *checker) listRepoFiles(ctx context.Context, repoRoot string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list repo files: %w", err)
	}
	gitMeta := filepath.Join(repoRoot, ".git")
	if _, err := os.Lstat(gitMeta); err == nil {
		if _, lookErr := c.which("git"); lookErr == nil {
			stdout, _, code, runErr := c.exec(ctx, []string{"git", "-C", repoRoot, "ls-files"}, "")
			if ctx.Err() != nil {
				return nil, fmt.Errorf("list repo files: %w", ctx.Err())
			}
			if runErr == nil && code == 0 {
				var files []string
				for _, line := range splitLines(stdout) {
					line = strings.TrimSpace(line)
					if line == "" || isExcludedRel(line) {
						continue
					}
					files = append(files, filepath.ToSlash(line))
				}
				sort.Strings(files)
				return files, nil
			}
		}
	}
	return walkRepoFiles(repoRoot), nil
}

func walkRepoFiles(repoRoot string) []string {
	var found []string
	_ = filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == repoRoot {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if _, skip := excludedDirNames[d.Name()]; skip || skipDirRel(rel) {
				return filepath.SkipDir
			}
			if d.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		if isExcludedRel(rel) {
			return nil
		}
		found = append(found, rel)
		return nil
	})
	sort.Strings(found)
	return found
}

func extractGeneratedHeader(text string) (string, bool) {
	lines := splitLines(text)
	if len(lines) > headerScanLines {
		lines = lines[:headerScanLines]
	}
	for _, line := range lines {
		if generatedHeaderRE.MatchString(line) {
			return strings.TrimSpace(line), true
		}
	}
	return "", false
}

func fileHasGeneratedHeader(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for i := 0; i < headerScanLines && sc.Scan(); i++ {
		if generatedHeaderRE.MatchString(sc.Text()) {
			return true
		}
	}
	return false
}

func (c *checker) discoverGeneratedFiles(ctx context.Context, repoRoot string) ([]string, error) {
	root := prepareRoot(repoRoot)
	files, err := c.listRepoFiles(ctx, root)
	if err != nil {
		return nil, err
	}
	var generated []string
	for _, rel := range files {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("discover generated files: %w", err)
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		if isSafeWorktreeFile(root, full) && fileHasGeneratedHeader(full) {
			generated = append(generated, rel)
		}
	}
	sort.Strings(generated)
	return generated, nil
}

func isSafeWorktreeFile(root, full string) bool {
	fi, err := os.Lstat(full)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return false
	}
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return false
	}
	return withinRoot(root, resolved)
}

func pathEscapes(root, full string) bool {
	fi, err := os.Lstat(full)
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return true
	}
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return true
	}
	return !withinRoot(root, resolved)
}

func prepareRoot(repoRoot string) string {
	resolved, err := resolvePath(repoRoot)
	if err != nil {
		return repoRoot
	}
	return resolved
}
