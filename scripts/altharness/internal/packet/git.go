package packet

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func resolveGitCommit(ctx context.Context, repoRoot, rev string) (string, error) {
	if strings.TrimSpace(rev) == "" || strings.HasPrefix(strings.TrimSpace(rev), "-") {
		return "", nil
	}
	stdout, _, code, err := runGit(ctx, repoRoot, "rev-parse", "--verify", "--end-of-options", strings.TrimSpace(rev)+"^{commit}")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", nil
	}
	return strings.TrimSpace(validUTF8(stdout)), nil
}

func currentHeadSHA(ctx context.Context, repoRoot string) (string, error) {
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		return "0000000", nil
	}
	resolved, err := resolveGitCommit(ctx, repoRoot, "HEAD")
	if err != nil {
		return "", err
	}
	if resolved == "" {
		return "0000000", nil
	}
	return resolved, nil
}

func readGitHeadText(repoRoot string) string {
	dot := filepath.Join(repoRoot, ".git")
	fi, err := os.Lstat(dot)
	if err != nil {
		return ""
	}
	headFile := filepath.Join(dot, "HEAD")
	if !fi.IsDir() {
		b, err := os.ReadFile(dot)
		if err != nil {
			return ""
		}
		line := ""
		if lines := splitLines(string(b)); len(lines) > 0 {
			line = lines[0]
		}
		if !strings.HasPrefix(line, "gitdir:") {
			return ""
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
		gitDir := raw
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(repoRoot, gitDir)
		}
		headFile = filepath.Join(gitDir, "HEAD")
	}
	b, err := os.ReadFile(headFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func resolveGitBranch(repoRoot string) string {
	if env := strings.TrimSpace(os.Getenv("ALT_HARNESS_BRANCH")); env != "" {
		return env
	}
	head := readGitHeadText(repoRoot)
	if head == "" {
		return "default"
	}
	if strings.HasPrefix(head, "ref: refs/heads/") {
		name := strings.TrimSpace(strings.TrimPrefix(head, "ref: refs/heads/"))
		if name == "" {
			return "default"
		}
		return name
	}
	if strings.HasPrefix(head, "ref: ") {
		name := strings.TrimSpace(strings.TrimPrefix(head, "ref: "))
		if name == "" {
			return "default"
		}
		return name
	}
	if len(head) > detachedSHALen {
		head = head[:detachedSHALen]
	}
	return "detached-" + head
}

var (
	branchUnsafeRE = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	branchDotsRE   = regexp.MustCompile(`\.{2,}`)
	branchDashRE   = regexp.MustCompile(`-{2,}`)
)

func sanitizeBranchName(branch string) string {
	cleaned := branchUnsafeRE.ReplaceAllString(strings.TrimSpace(branch), "-")
	cleaned = branchDotsRE.ReplaceAllString(cleaned, "-")
	cleaned = branchDashRE.ReplaceAllString(cleaned, "-")
	cleaned = strings.Trim(cleaned, ".-")
	if cleaned == "" {
		return "default"
	}
	return cleaned
}

func currentBranchSanitized(repoRoot string) string {
	return sanitizeBranchName(resolveGitBranch(repoRoot))
}

func gitPathsFromNUL(stdout []byte) []string {
	var paths []string
	for _, entry := range splitNull(stdout) {
		rel := normalizeRepoRel(entry)
		if rel == "" || rel == ".harness" || strings.HasPrefix(rel, harnessStatePrefix) {
			continue
		}
		paths = append(paths, rel)
	}
	return paths
}

func gitActualModifiedFiles(ctx context.Context, repoRoot, resolvedBase string) (map[string]struct{}, error) {
	diffOut, diffErr, code, err := runGit(ctx, repoRoot, "diff", "--name-only", "--no-renames", "-z", resolvedBase, "--")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, pErr("git diff failed against %s: %s", resolvedBase, strings.TrimSpace(validUTF8(diffErr)))
	}
	cachedOut, cachedErr, code, err := runGit(ctx, repoRoot, "diff", "--cached", "--name-only", "--no-renames", "-z", resolvedBase, "--")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, pErr("git diff --cached failed against %s: %s", resolvedBase, strings.TrimSpace(validUTF8(cachedErr)))
	}
	untracked, untrackedErr, code, err := runGit(ctx, repoRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, pErr("git ls-files failed: %s", strings.TrimSpace(validUTF8(untrackedErr)))
	}
	paths := map[string]struct{}{}
	for _, raw := range [][]byte{diffOut, cachedOut, untracked} {
		for _, rel := range gitPathsFromNUL(raw) {
			paths[rel] = struct{}{}
		}
	}
	return paths, nil
}

func gitFileTextAtRev(ctx context.Context, repoRoot, rev, rel string) (string, error) {
	stdout, _, code, err := runGit(ctx, repoRoot, "show", "--end-of-options", rev+":"+rel)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", nil
	}
	return validUTF8(stdout), nil
}

func readWorktreeText(repoRoot, rel string) string {
	full := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if isSymlink(full) || !withinDir(full, repoRoot) {
		return ""
	}
	st, err := os.Stat(full)
	if err != nil || !st.Mode().IsRegular() {
		return ""
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return ""
	}
	return validUTF8(b)
}

func gitDirtyFiles(ctx context.Context, repoRoot string) (map[string]struct{}, error) {
	dirty := map[string]struct{}{}
	groups := [][]string{
		{"diff", "--name-only", "--no-renames", "-z", "HEAD", "--"},
		{"diff", "--cached", "--name-only", "--no-renames", "-z", "--"},
		{"ls-files", "--others", "--exclude-standard", "-z"},
	}
	for _, args := range groups {
		stdout, _, code, err := runGit(ctx, repoRoot, args...)
		if err != nil {
			return nil, err
		}
		if code != 0 {
			continue
		}
		for _, rel := range gitPathsFromNUL(stdout) {
			dirty[rel] = struct{}{}
		}
	}
	return dirty, nil
}

func gitCommitFilesWithStatus(ctx context.Context, repoRoot, rev string) ([][2]string, error) {
	stdout, stderr, code, err := runGit(ctx, repoRoot, "diff-tree", "--no-commit-id", "--name-status", "--no-renames", "-r", "--root", "-z", rev)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, pErr("git diff-tree failed for %s: %s", rev, strings.TrimSpace(validUTF8(stderr)))
	}
	tokens := splitNullKeep(stdout)
	var entries [][2]string
	for idx := 0; idx+1 < len(tokens); {
		status := tokens[idx]
		if status == "" {
			idx++
			continue
		}
		rel := normalizeRepoRel(tokens[idx+1])
		if rel != "" {
			entries = append(entries, [2]string{status, rel})
		}
		idx += 2
	}
	return entries, nil
}

func gitBlobMode(ctx context.Context, repoRoot, rev, rel string) (string, error) {
	stdout, _, code, err := runGit(ctx, repoRoot, "ls-tree", rev, "--", rel)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", nil
	}
	line := strings.TrimSpace(validUTF8(stdout))
	if line == "" {
		return "", nil
	}
	mode, _, _ := strings.Cut(line, " ")
	return mode, nil
}

func gitCommitFiles(ctx context.Context, repoRoot, rev string) ([]string, error) {
	entries, err := gitCommitFilesWithStatus(ctx, repoRoot, rev)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, ent := range entries {
		out = append(out, ent[1])
	}
	return out, nil
}
