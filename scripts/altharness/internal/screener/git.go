package screener

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type gitEntry struct {
	status string
	oldRel string
	newRel string
}

func screenGitDiff(ctx context.Context, repoRoot, base string, head *string) ([]Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolvedBase, err := validateGitRev(ctx, repoRoot, base, "--base")
	if err != nil {
		return nil, err
	}
	var resolvedHead string
	if head != nil {
		resolvedHead, err = validateGitRev(ctx, repoRoot, *head, "--head")
		if err != nil {
			return nil, err
		}
	}
	diffArgs := []string{"diff", "--name-status", "-z", resolvedBase}
	if head != nil {
		diffArgs = append(diffArgs, resolvedHead)
	}
	diffArgs = append(diffArgs, "--")
	stdout, stderr, code, err := runGit(ctx, repoRoot, diffArgs...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = fmt.Sprintf("exit %d", code)
		}
		return nil, fmt.Errorf("git diff failed: %s", msg)
	}
	var cands []Candidate
	for _, entry := range parseGitDiffEntries(stdout) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.status == "D" || entry.newRel == "" {
			part, err := screenDeletedGit(ctx, repoRoot, resolvedBase, entry.oldRel)
			if err != nil {
				return nil, err
			}
			cands = append(cands, part...)
			continue
		}
		lang := inferLang(entry.newRel, "", false)
		if lang == "" {
			continue
		}
		pair, err := loadGitPair(ctx, repoRoot, resolvedBase, head, resolvedHead, entry.oldRel, entry.newRel)
		if err != nil {
			return nil, err
		}
		if pair == nil {
			continue
		}
		part, err := screenSources(pair[0], pair[1], entry.newRel, entry.oldRel, lang, true)
		if err != nil {
			return nil, err
		}
		cands = append(cands, part...)
	}
	if cands == nil {
		cands = []Candidate{}
	}
	return cands, nil
}

func validateGitRev(ctx context.Context, repo, rev, label string) (string, error) {
	if strings.TrimSpace(rev) == "" || strings.HasPrefix(rev, "-") {
		return "", fmt.Errorf("invalid git revision for %s: %q", label, rev)
	}
	stdout, stderr, code, err := runGit(ctx, repo, "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("invalid git revision for %s (%q): %s", label, rev, err.Error())
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = fmt.Sprintf("exit %d", code)
		}
		return "", fmt.Errorf("invalid git revision for %s (%q): %s", label, rev, msg)
	}
	return strings.TrimSpace(stdout), nil
}

func screenDeletedGit(ctx context.Context, repo, base, oldRel string) ([]Candidate, error) {
	text, ok, err := gitShow(ctx, repo, base, oldRel)
	if err != nil || !ok {
		return nil, err
	}
	lang := inferLang(oldRel, "", false)
	norm := strings.ReplaceAll(oldRel, "\\", "/")
	isTest := (lang != "" && isTestFile(oldRel, lang, text)) || testPathRE.MatchString(norm)
	if !isTest {
		return nil, nil
	}
	return []Candidate{makeCandidate(
		oldRel,
		"<file>",
		"test.file_deleted",
		fmt.Sprintf("test file present (%d lines)", splitLines(text)),
		"<deleted>",
	)}, nil
}

func loadGitPair(ctx context.Context, repo, base string, head *string, resolvedHead, oldRel, newRel string) (*[2]string, error) {
	before, ok, err := gitShow(ctx, repo, base, oldRel)
	if err != nil || !ok {
		return nil, err
	}
	if head != nil {
		after, ok, err := gitShow(ctx, repo, resolvedHead, newRel)
		if err != nil || !ok {
			return nil, err
		}
		return &[2]string{before, after}, nil
	}
	afterPath := filepath.Join(repo, filepath.FromSlash(newRel))
	info, err := os.Lstat(afterPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to read symlink in worktree: %s", newRel)
	}
	if !info.Mode().IsRegular() {
		return nil, nil
	}
	rootAbs, err := filepath.Abs(repo)
	if err != nil {
		return nil, err
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		rootReal = rootAbs
	}
	abs, err := filepath.Abs(afterPath)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(rootReal, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("worktree path escapes repo root: %s", newRel)
	}
	text, err := readText(afterPath)
	if err != nil {
		return nil, err
	}
	return &[2]string{before, text}, nil
}

func gitShow(ctx context.Context, repo, rev, rel string) (string, bool, error) {
	stdout, _, code, err := runGit(ctx, repo, "show", "--end-of-options", rev+":"+rel)
	if err != nil {
		return "", false, err
	}
	if code != 0 {
		return "", false, nil
	}
	return stdout, true, nil
}

func runGit(ctx context.Context, repo string, args ...string) (string, string, int, error) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return "", "", 0, fmt.Errorf("git executable not found in PATH")
	}
	if err := ctx.Err(); err != nil {
		return "", "", 0, err
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, gitBin, append([]string{"-C", repo}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if errors.Is(cctx.Err(), context.DeadlineExceeded) {
		return "", stderr.String(), 0, fmt.Errorf("git command timed out after 60s")
	}
	if err != nil {
		if cctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled) {
			return "", stderr.String(), 0, ctx.Err()
		}
		if cmd.ProcessState == nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return stdout.String(), stderr.String(), 1, fmt.Errorf("%s", msg)
		}
	}
	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	return stdout.String(), stderr.String(), code, nil
}

func parseGitDiffEntries(stdout string) []gitEntry {
	tokens := strings.Split(stdout, "\x00")
	var entries []gitEntry
	for idx := 0; idx < len(tokens); {
		status := tokens[idx]
		if status == "" {
			idx++
			continue
		}
		switch {
		case strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C"):
			if idx+2 < len(tokens) {
				entries = append(entries, gitEntry{status: status[:1], oldRel: tokens[idx+1], newRel: tokens[idx+2]})
			}
			idx += 3
		case status == "M" || status == "T":
			if idx+1 < len(tokens) {
				rel := tokens[idx+1]
				entries = append(entries, gitEntry{status: status, oldRel: rel, newRel: rel})
			}
			idx += 2
		case status == "D":
			if idx+1 < len(tokens) {
				entries = append(entries, gitEntry{status: "D", oldRel: tokens[idx+1]})
			}
			idx += 2
		default:
			idx += 2
		}
	}
	return entries
}

func splitLines(s string) int {
	if s == "" {
		return 0
	}
	n := 0
	for i := 0; i < len(s); {
		n++
		j := i
		for j < len(s) && s[j] != '\n' && s[j] != '\r' {
			j++
		}
		if j >= len(s) {
			break
		}
		if s[j] == '\r' && j+1 < len(s) && s[j+1] == '\n' {
			i = j + 2
		} else {
			i = j + 1
		}
		if i == len(s) {
			break
		}
	}
	return n
}
