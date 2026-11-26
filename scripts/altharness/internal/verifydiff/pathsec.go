package verifydiff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	validLabelRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	unsafeCharRE = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	multiDotRE   = regexp.MustCompile(`\.{2,}`)
	multiDashRE  = regexp.MustCompile(`-{2,}`)
)

// SanitizeLabel collapses an arbitrary path or name into a single safe filename stem
// without path separators, parent traversal, or leading/trailing punctuation.
func SanitizeLabel(raw string) string {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.ReplaceAll(cleaned, "/", "__")
	cleaned = strings.ReplaceAll(cleaned, "\\", "__")
	cleaned = unsafeCharRE.ReplaceAllString(cleaned, "-")
	cleaned = multiDotRE.ReplaceAllString(cleaned, "-")
	cleaned = multiDashRE.ReplaceAllString(cleaned, "-")
	cleaned = strings.Trim(cleaned, ".-_")
	if cleaned == "" {
		return "verify-diff"
	}
	if len(cleaned) > 128 {
		cleaned = strings.TrimRight(cleaned[:128], ".-_")
		if cleaned == "" {
			return "verify-diff"
		}
	}
	return cleaned
}

func validateExplicitLabel(label string) (string, error) {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return "", errors.New("label must not be empty")
	}
	if strings.Contains(trimmed, "..") || strings.ContainsAny(trimmed, "/\\\x00") {
		return "", fmt.Errorf("invalid label %q: path separators and '..' are forbidden", label)
	}
	if !validLabelRE.MatchString(trimmed) {
		return "", fmt.Errorf("invalid label %q: must match %s", label, validLabelRE.String())
	}
	return trimmed, nil
}

func resolveExistingDir(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", errors.New("empty path")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", p)
	}
	return resolved, nil
}

func pathWithin(candidate, root string) bool {
	if candidate == root {
		return true
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// validateRepoFilePath enforces CWE-22/CWE-59 invariants for --file:
// repo-relative POSIX path, no NUL/absolute/"..", and on-disk realpath inside repoRoot.
func validateRepoFilePath(repoRoot, rawPath string) (relSlash string, absResolved string, err error) {
	if strings.Contains(rawPath, "\x00") {
		return "", "", errors.New("--file contains NUL byte")
	}
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", "", errors.New("--file must not be empty")
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "\\") {
		return "", "", fmt.Errorf("--file must be repository-relative, got absolute path %q", rawPath)
	}
	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	for _, seg := range strings.Split(normalized, "/") {
		if seg == ".." {
			return "", "", fmt.Errorf("--file must not contain '..' traversal: %q", rawPath)
		}
	}
	cleaned := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(normalized)), "./")
	if cleaned == "" || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", "", fmt.Errorf("invalid --file path %q", rawPath)
	}

	repoResolved, err := resolveExistingDir(repoRoot)
	if err != nil {
		return "", "", fmt.Errorf("invalid repo root %q: %w", repoRoot, err)
	}

	joined := filepath.Join(repoResolved, filepath.FromSlash(cleaned))
	if _, err := os.Lstat(joined); err != nil {
		return "", "", fmt.Errorf("target file does not exist in worktree: %s (%w)", cleaned, err)
	}
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve target file %s: %w", cleaned, err)
	}
	if !pathWithin(resolved, repoResolved) {
		return "", "", fmt.Errorf("target file %q resolves outside repository root (%s)", rawPath, resolved)
	}
	st, err := os.Stat(resolved)
	if err != nil || !st.Mode().IsRegular() {
		return "", "", fmt.Errorf("target file %q is not a regular file", rawPath)
	}
	relReal, err := filepath.Rel(repoResolved, resolved)
	if err != nil {
		return "", "", err
	}
	return filepath.ToSlash(relReal), resolved, nil
}

// resolveSafeOutputPath resolves a repo-scoped output path (for evidence dir or decisions file),
// rejecting symlink escapes and relative traversal outside repoRoot.
func resolveSafeOutputPath(repoResolved, rawPath string, defaultPath string) (string, error) {
	if strings.TrimSpace(rawPath) == "" {
		return defaultPath, nil
	}
	if strings.Contains(rawPath, "\x00") {
		return "", errors.New("output path contains NUL byte")
	}
	var candidate string
	if filepath.IsAbs(rawPath) {
		candidate = filepath.Clean(rawPath)
	} else {
		normalized := strings.ReplaceAll(strings.TrimSpace(rawPath), "\\", "/")
		for _, seg := range strings.Split(normalized, "/") {
			if seg == ".." {
				return "", fmt.Errorf("output path %q must not contain '..'", rawPath)
			}
		}
		candidate = filepath.Join(repoResolved, filepath.FromSlash(normalized))
	}
	// Walk existing ancestor to verify no symlink escapes if candidate is under repoRoot.
	cur := candidate
	var tail []string
	for {
		if st, err := os.Lstat(cur); err == nil {
			if st.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("output path must not traverse or target a symlink: %s", cur)
			}
			resolved, rerr := filepath.EvalSymlinks(cur)
			if rerr != nil {
				return "", rerr
			}
			final := filepath.Join(append([]string{resolved}, tail...)...)
			if !filepath.IsAbs(rawPath) && !pathWithin(final, repoResolved) {
				return "", fmt.Errorf("output path %q escapes repository root", rawPath)
			}
			return final, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
	return candidate, nil
}

func validateBaseCommitInput(rev string) (string, error) {
	trimmed := strings.TrimSpace(rev)
	if trimmed == "" {
		return "", errors.New("--base-commit must not be empty")
	}
	if strings.HasPrefix(trimmed, "-") {
		return "", fmt.Errorf("invalid --base-commit %q: must not start with '-'", rev)
	}
	if strings.ContainsAny(trimmed, "\x00\n\r\t ") {
		return "", fmt.Errorf("invalid --base-commit %q: whitespace or control characters are forbidden", rev)
	}
	return trimmed, nil
}

func splitShellWords(input string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inToken := false
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(input); i++ {
		ch := input[i]
		if escaped {
			cur.WriteByte(ch)
			inToken = true
			escaped = false
			continue
		}
		if inSingle {
			if ch == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(ch)
			}
			continue
		}
		if inDouble {
			switch ch {
			case '"':
				inDouble = false
			case '\\':
				escaped = true
			default:
				cur.WriteByte(ch)
			}
			continue
		}
		switch ch {
		case ' ', '\t', '\n', '\r':
			if inToken {
				words = append(words, cur.String())
				cur.Reset()
				inToken = false
			}
		case '\'':
			inSingle = true
			inToken = true
		case '"':
			inDouble = true
			inToken = true
		case '\\':
			escaped = true
			inToken = true
		default:
			cur.WriteByte(ch)
			inToken = true
		}
	}
	if escaped || inSingle || inDouble {
		return nil, errors.New("unclosed quote or trailing escape in --runner-cmd")
	}
	if inToken {
		words = append(words, cur.String())
	}
	if len(words) == 0 {
		return nil, errors.New("--runner-cmd must not be empty")
	}
	return words, nil
}
