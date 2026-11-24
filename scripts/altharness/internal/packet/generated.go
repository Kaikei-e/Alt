package packet

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var generatedHeaderRe = regexp.MustCompile(`(?i)^\s*(?://+|#+|/\*|\*|--|<!--|;)\s*Code generated\b[^\n]*DO NOT EDIT\.?`)

func hasGeneratedHeaderText(text string) bool {
	lines := splitLines(text)
	limit := generatedHeaderScanLines
	if len(lines) < limit {
		limit = len(lines)
	}
	for _, line := range lines[:limit] {
		if generatedHeaderRe.MatchString(line) {
			return true
		}
	}
	return false
}

func fileHasGeneratedHeader(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	raw := make([]byte, 8192)
	n, _ := f.Read(raw)
	raw = raw[:n]
	probe := raw
	if len(probe) > 4096 {
		probe = probe[:4096]
	}
	if bytes.IndexByte(probe, 0) >= 0 {
		return false
	}
	return hasGeneratedHeaderText(validUTF8(raw))
}

func gitRevHasGeneratedHeader(ctx context.Context, repo, rev, norm string) bool {
	stdout, _, code, err := runGit(ctx, repo, "show", "--end-of-options", rev+":"+norm)
	if err != nil || code != 0 {
		return false
	}
	probe := stdout
	if len(probe) > 4096 {
		probe = probe[:4096]
	}
	if bytes.IndexByte(probe, 0) >= 0 {
		return false
	}
	chunk := stdout
	if len(chunk) > 8192 {
		chunk = chunk[:8192]
	}
	return hasGeneratedHeaderText(validUTF8(chunk))
}

func repoRelIsGenerated(ctx context.Context, repo, rel, baseCommit string) bool {
	norm := normalizeRepoRel(rel)
	if norm == "" {
		return false
	}
	disk := filepath.Join(repo, filepath.FromSlash(norm))
	if isRegularNonSymlink(disk) && withinDir(disk, repo) && fileHasGeneratedHeader(disk) {
		return true
	}
	if _, err := lookGit(); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		return false
	}
	if gitRevHasGeneratedHeader(ctx, repo, "HEAD", norm) {
		return true
	}
	if baseCommit != "" && baseCommit != "HEAD" && !strings.HasPrefix(baseCommit, "-") {
		return gitRevHasGeneratedHeader(ctx, repo, baseCommit, norm)
	}
	return false
}

func isRegularNonSymlink(path string) bool {
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return true
}

func listGeneratedForbiddenPaths(ctx context.Context, repo string) ([]string, error) {
	var candidates []string
	if _, err := lookGit(); err == nil {
		if _, statErr := os.Stat(filepath.Join(repo, ".git")); statErr == nil {
			stdout, _, code, runErr := runGit(ctx, repo, "ls-files", "-z")
			if runErr != nil {
				return nil, runErr
			}
			if code == 0 {
				candidates = splitNull(stdout)
			}
		}
	}
	if len(candidates) == 0 {
		err := filepath.WalkDir(repo, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			rel, relErr := filepath.Rel(repo, path)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if rel == "." {
				return nil
			}
			parts := strings.Split(rel, "/")
			for _, part := range parts {
				if part == ".git" {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
			if d.Type()&os.ModeSymlink != 0 {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			candidates = append(candidates, rel)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	uniq := map[string]struct{}{}
	for _, rel := range candidates {
		uniq[rel] = struct{}{}
	}
	var keys []string
	for rel := range uniq {
		keys = append(keys, rel)
	}
	sort.Strings(keys)
	var generated []string
	for _, rel := range keys {
		skip := false
		for _, prefix := range excludedGeneratedPrefixes {
			if strings.HasPrefix(rel, prefix) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		full := filepath.Join(repo, filepath.FromSlash(rel))
		if isRegularNonSymlink(full) && withinDir(full, repo) && fileHasGeneratedHeader(full) {
			generated = append(generated, rel)
		}
	}
	if generated == nil {
		generated = []string{}
	}
	return generated, nil
}

func validateGeneratedCodeEdits(ctx context.Context, handoff map[string]any, repo string, allow bool, issues *[]Issue, baseCommit string) error {
	if repo == "" || allow {
		return nil
	}
	modified, ok := asSlice(handoff["modified_files"])
	if !ok {
		return nil
	}
	for idx, entry := range modified {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		item, ok := asMap(entry)
		if !ok {
			continue
		}
		rel, ok := asString(item["path"])
		if !ok {
			continue
		}
		if repoRelIsGenerated(ctx, repo, rel, baseCommit) {
			*issues = append(*issues, Issue{
				Code:    "GENERATED_CODE_MODIFIED",
				Path:    fmt.Sprintf("$.modified_files[%d].path", idx),
				Message: fmt.Sprintf("modified file '%s' carries a generated-code DO NOT EDIT header; set scope.allow_generated_code_edits=true only when explicitly regenerating", rel),
			})
		}
	}
	return nil
}
