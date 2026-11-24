// Package generated verifies generated-code provenance, generator version pins,
// and DO NOT EDIT tamper resistance. Behavior matches
// scripts/lint/check_generated_code.py, including exit codes 0, 1, and 2.
package generated

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	exitClean      = 0
	exitViolations = 1
	exitUsage      = 2

	headerScanLines = 40
)

// Violation is one diagnostic. The text form is
//
//	path:line: [rule] message
type Violation struct {
	Path    string
	Line    int
	Rule    string
	Message string
}

// Run executes the generated-code checker. args are the CLI arguments
// (the program name is not included). A cancelled ctx stops subprocesses.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		fmt.Fprintln(stderr, "error: nil context")
		return exitUsage
	}
	parsed, perr := parseArgs(args)
	if perr != "" {
		fmt.Fprint(stderr, perr)
		return exitUsage
	}
	if parsed.help {
		fmt.Fprint(stdout, usageText)
		return exitClean
	}
	if msg := validateCLI(parsed); msg != "" {
		fmt.Fprint(stderr, msg)
		return exitUsage
	}
	c := &checker{}
	if parsed.before != nil && parsed.after != nil {
		return c.runPair(parsed, stderr)
	}
	root, err := resolveRepoRoot(parsed.repoRoot)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitViolations
	}
	return c.runRepo(ctx, parsed, root, stdout, stderr)
}

func (c *checker) runPair(args cliArgs, stderr io.Writer) int {
	before := *args.before
	after := *args.after
	if !isRegularFile(before) || !isRegularFile(after) {
		fmt.Fprint(stderr, "error: --before and --after paths must point to existing files\n")
		return exitUsage
	}
	violations, err := checkPair(before, after, args.logicalPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitViolations
	}
	if len(violations) > 0 {
		writeViolations(stderr, violations)
		return exitViolations
	}
	return exitClean
}

func (c *checker) runRepo(ctx context.Context, args cliArgs, repo string, stdout, stderr io.Writer) int {
	if args.listGenerated {
		files, err := c.discoverGeneratedFiles(ctx, repo)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitViolations
		}
		for _, rel := range files {
			fmt.Fprintln(stdout, rel)
		}
		return exitClean
	}

	pins, regen, diff := resolveCheckFlags(args)
	var violations []Violation
	if pins {
		found, err := c.checkPins(ctx, repo)
		if err != nil {
			return failErr(stderr, err)
		}
		violations = append(violations, found...)
	}
	if regen {
		found, err := c.checkRegeneration(ctx, repo)
		if err != nil {
			return failErr(stderr, err)
		}
		violations = append(violations, found...)
	}
	if diff && args.base != nil {
		found, err := c.checkGitDiff(ctx, repo, *args.base, args.head)
		if err != nil {
			return failErr(stderr, err)
		}
		violations = append(violations, found...)
	}
	if len(violations) > 0 {
		writeViolations(stderr, violations)
		fmt.Fprintf(stderr, "FAIL: %d generated-code violation(s) found.\n", len(violations))
		return exitViolations
	}
	if args.onlyDiff {
		fmt.Fprint(stdout, "OK: no tampered DO NOT EDIT generated files in git diff.\n")
	} else {
		fmt.Fprint(stdout, "OK: generated code pins and regeneration parity verified.\n")
	}
	return exitClean
}

func failErr(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "error: %v\n", err)
	return exitViolations
}

func writeViolations(stderr io.Writer, violations []Violation) {
	for _, v := range violations {
		fmt.Fprintf(stderr, "%s:%d: [%s] %s\n", v.Path, v.Line, v.Rule, v.Message)
	}
}

func resolveCheckFlags(args cliArgs) (pins, regen, diff bool) {
	if args.onlyDiff {
		return false, false, true
	}
	if args.base != nil {
		if args.checkPins || args.checkRegen {
			return args.checkPins, args.checkRegen, true
		}
		return true, true, true
	}
	return args.checkPins || !args.checkRegen, args.checkRegen || !args.checkPins, false
}

func validateCLI(args cliArgs) string {
	if (args.before == nil) != (args.after == nil) {
		return "error: --before and --after must be provided together\n"
	}
	if args.head != nil && args.base == nil {
		return "error: --head requires --base\n"
	}
	if args.onlyDiff && args.base == nil {
		return "error: --only-diff requires --base\n"
	}
	if args.base != nil && (strings.TrimSpace(*args.base) == "" || strings.HasPrefix(*args.base, "-")) {
		return fmt.Sprintf("error: invalid --base git revision: %s\n", pythonRepr(*args.base))
	}
	if args.head != nil && (strings.TrimSpace(*args.head) == "" || strings.HasPrefix(*args.head, "-")) {
		return fmt.Sprintf("error: invalid --head git revision: %s\n", pythonRepr(*args.head))
	}
	return ""
}

func pythonRepr(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func isRegularFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func resolveRepoRoot(override *string) (string, error) {
	if override != nil {
		return resolvePath(*override)
	}
	return defaultRepoRoot()
}

func resolvePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return abs, nil
}

func defaultRepoRoot() (string, error) {
	if cwd, err := os.Getwd(); err == nil {
		if root, ok := findRepoRoot(cwd); ok {
			return root, nil
		}
	}
	return "", errors.New("cannot locate repository root (pass --repo-root)")
}

func findRepoRoot(start string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		if isRegularFile(filepath.Join(dir, "scripts", "altharness", "go.mod")) ||
			isRegularFile(filepath.Join(dir, "scripts", "lint", "run.sh")) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func decodeIgnore(b []byte) string {
	return strings.ToValidUTF8(string(b), "")
}

// splitLines matches str.splitlines for \n, \r, and \r\n: a trailing break
// does not create an extra empty element, and the empty string yields nil.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	ended := strings.HasSuffix(s, "\n")
	parts := strings.Split(s, "\n")
	if ended && len(parts) > 0 {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func trimRightSpace(s string) string {
	return strings.TrimRightFunc(s, unicode.IsSpace)
}

func withinRoot(root, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
