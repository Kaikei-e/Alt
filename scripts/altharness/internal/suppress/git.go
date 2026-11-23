package suppress

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"alt/scripts/altharness/internal/execx"
)

func gitCmd(ctx context.Context, repo string, args ...string) (string, string, int, error) {
	if err := ctx.Err(); err != nil {
		return "", "", 1, fmt.Errorf("git: %w", err)
	}
	stdout, stderr, code, err := execx.Combined(ctx, execx.Cmd{
		Args: append([]string{"git", "-C", repo}, args...),
	})
	if err != nil {
		return stdout, stderr, code, fmt.Errorf("git: %w", err)
	}
	return stdout, stderr, code, nil
}

func gitMissing(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "command not found") || strings.Contains(err.Error(), "executable file not found"))
}

func calledProcessError(repo string, args []string, code int) error {
	cmd := append([]string{"git", "-C", repo}, args...)
	parts := make([]string, len(cmd))
	for i, a := range cmd {
		parts[i] = pyQuote(a)
	}
	return fmt.Errorf("Command '%s' returned non-zero exit status %d.", "["+strings.Join(parts, ", ")+"]", code)
}

func gitChecked(ctx context.Context, repo string, args ...string) (string, error) {
	stdout, stderr, code, err := gitCmd(ctx, repo, args...)
	if err != nil {
		if gitMissing(err) {
			return "", fmt.Errorf("git")
		}
		return "", err
	}
	if code != 0 {
		if strings.TrimSpace(stderr) != "" {
			return stdout, calledProcessError(repo, args, code)
		}
		return stdout, calledProcessError(repo, args, code)
	}
	return stdout, nil
}

func validateGitRev(ctx context.Context, repo, rev string) (string, string, error) {
	if rev == "" || strings.TrimSpace(rev) == "" || strings.HasPrefix(rev, "-") {
		return "", fmt.Sprintf("invalid --base git revision: %s", pyQuote(rev)), nil
	}
	args := []string{"rev-parse", "--verify", "--end-of-options", rev + "^{commit}"}
	stdout, stderr, code, err := gitCmd(ctx, repo, args...)
	if err != nil {
		if gitMissing(err) {
			return "", "", fmt.Errorf("git")
		}
		return "", "", err
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = strings.TrimSpace(stdout)
		}
		return "", fmt.Sprintf("invalid --base git revision %s: %s", pyQuote(rev), msg), nil
	}
	return strings.TrimSpace(stdout), "", nil
}

func validateDiffRev(ctx context.Context, repo, rev string) (string, error) {
	cleaned := strings.TrimSpace(rev)
	if cleaned == "" || strings.HasPrefix(cleaned, "-") {
		return "", fmt.Errorf("invalid git revision %s: must not be empty or start with '-'", pyQuote(rev))
	}
	args := []string{"rev-parse", "--verify", "--end-of-options", cleaned + "^{commit}"}
	stdout, _, code, err := gitCmd(ctx, repo, args...)
	if err != nil {
		if gitMissing(err) {
			return "", fmt.Errorf("git")
		}
		return "", err
	}
	if code != 0 {
		return "", calledProcessError(repo, args, code)
	}
	resolved := strings.TrimSpace(stdout)
	if resolved == "" {
		return "", fmt.Errorf("git revision did not resolve to a commit: %s", pyQuote(rev))
	}
	return resolved, nil
}

func resolveAutoBase(ctx context.Context, repo string) (string, error) {
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		return "", nil
	}
	headOut, _, code, err := gitCmd(ctx, repo, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		if gitMissing(err) {
			return "", fmt.Errorf("git")
		}
		return "", err
	}
	if code != 0 {
		return "", nil
	}
	head := strings.TrimSpace(headOut)
	var cands []string
	if env := strings.TrimSpace(os.Getenv("LINT_BASE")); env != "" && !allZerosRE.MatchString(env) && !strings.HasPrefix(env, "-") {
		cands = append(cands, env)
	}
	cands = append(cands, "origin/main")
	for _, cand := range cands {
		out, _, code, err := gitCmd(ctx, repo, "rev-parse", "--verify", "--quiet", "--end-of-options", cand+"^{commit}")
		if err != nil {
			return "", err
		}
		if code != 0 {
			continue
		}
		target := strings.TrimSpace(out)
		mb, _, code, err := gitCmd(ctx, repo, "merge-base", target, head)
		if err != nil {
			return "", err
		}
		if code != 0 || strings.TrimSpace(mb) == "" {
			continue
		}
		mergeBase := strings.TrimSpace(mb)
		if mergeBase == head {
			prev, _, code, err := gitCmd(ctx, repo, "rev-parse", "--verify", "--quiet", "HEAD~1^{commit}")
			if err != nil {
				return "", err
			}
			if code == 0 && strings.TrimSpace(prev) != "" {
				return strings.TrimSpace(prev), nil
			}
		}
		return mergeBase, nil
	}
	return head, nil
}

func gitLsFiles(ctx context.Context, repo string) ([]string, error) {
	out, err := gitChecked(ctx, repo, "ls-files", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	var files []string
	for _, line := range nonEmptyLines(out) {
		if _, ok := set[line]; ok {
			continue
		}
		set[line] = struct{}{}
		files = append(files, line)
	}
	sortStrings(files)
	return files, nil
}

func gitShow(ctx context.Context, repo, rev, rel string) (string, bool, error) {
	args := []string{"show", "--end-of-options", rev + ":" + rel}
	stdout, _, code, err := gitCmd(ctx, repo, args...)
	if err != nil {
		if gitMissing(err) {
			return "", false, fmt.Errorf("git")
		}
		return "", false, err
	}
	if code != 0 {
		return "", false, nil
	}
	return stdout, true, nil
}

func safeRepoFile(repo, rel string) (string, bool) {
	if rel == "" || strings.Contains(rel, "\x00") {
		return "", false
	}
	full := filepath.Join(repo, filepath.FromSlash(rel))
	st, err := os.Lstat(full)
	if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", false
	}
	repoResolved := repo
	if r, err := filepath.EvalSymlinks(repo); err == nil {
		repoResolved = r
	}
	relOut, err := filepath.Rel(repoResolved, resolved)
	if err != nil || relOut == ".." || strings.HasPrefix(relOut, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}

func safeWorktreeFile(repo, rel string) (string, bool) {
	if rel == "" || strings.Contains(rel, "\x00") || strings.HasPrefix(rel, "/") || filepath.IsAbs(rel) {
		return "", false
	}
	cur := repo
	var st os.FileInfo
	var err error
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
		cur = filepath.Join(cur, part)
		st, err = os.Lstat(cur)
		if err != nil || st.Mode()&os.ModeSymlink != 0 {
			return "", false
		}
	}
	if st == nil || !st.Mode().IsRegular() {
		return "", false
	}
	return cur, true
}

func defaultRepo(ctx context.Context) string {
	out, _, code, err := gitCmd(ctx, ".", "rev-parse", "--show-toplevel")
	if err == nil && code == 0 {
		if s := strings.TrimSpace(out); s != "" {
			return s
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func cleanDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", pathErr(err))
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return abs, nil
}

func pathErr(err error) error { return err }

func gitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func gitList(ctx context.Context, repo string, args ...string) ([]string, error) {
	out, err := gitChecked(ctx, repo, args...)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, line := range gitLines(out) {
		seen[line] = struct{}{}
	}
	files := make([]string, 0, len(seen))
	for line := range seen {
		files = append(files, line)
	}
	sort.Strings(files)
	return files, nil
}
