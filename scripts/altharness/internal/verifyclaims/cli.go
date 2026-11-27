// Package verifyclaims implements VERIFY-06: claim-by-claim evidence verification,
// implementation-report template checks, plecto WASM Component Model WIT comparison,
// and recap-worker clippy + #[allow(...)] inventory evidence generation.
package verifyclaims

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	exitOK      = 0
	exitInvalid = 1
	exitUsage   = 2

	defaultCommandTimeout = 5 * time.Minute
	templateRelPath       = "scripts/agent-harness/templates/implementation-report.md.tmpl"
	defaultPlectoBaseRev  = "ad0850966~1"
	defaultRecapProjRel   = "recap-worker/recap-worker"
)

// Issue represents a single structured validation or verification finding.
type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Run executes the verify-claims CLI. args excludes argv[0].
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if ctx == nil {
		if stderr != nil {
			fmt.Fprintln(stderr, "error: nil context")
		}
		return exitUsage
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: the following arguments are required: subcommand")
		writeHelp(stderr)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		writeHelp(stdout)
		return exitOK
	case "validate":
		return cmdValidate(ctx, args[1:], stdout, stderr)
	case "check-template":
		return cmdCheckTemplate(ctx, args[1:], stdout, stderr)
	case "plecto-wit":
		return cmdPlectoWIT(ctx, args[1:], stdout, stderr)
	case "recap-clippy":
		return cmdRecapClippy(ctx, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "error: argument subcommand: invalid choice: %q (choose from validate, check-template, plecto-wit, recap-clippy)\n", args[0])
		return exitUsage
	}
}

func writeHelp(w io.Writer) {
	fmt.Fprint(w, `Verify VERIFY-06 claim-by-claim evidence, implementation report templates, and build artifacts.

subcommands:
  validate <report.md> [--repo-root DIR] [--format text|json]
  check-template [--repo-root DIR] [--format text|json]
  plecto-wit [--repo-root DIR] [--base-commit REV] [--artifact-dir DIR] [--before-wasm PATH --after-wasm PATH] [--evidence-dir DIR] [--format text|json]
  recap-clippy [--repo-root DIR] [--project-dir REL] [--evidence-dir DIR] [--format text|json]
`)
}

type parsedCLI struct {
	positionals []string
	values      map[string]string
	bools       map[string]bool
}

func parseCLI(args []string, boolFlags map[string]bool) (parsedCLI, string) {
	out := parsedCLI{values: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out.positionals = append(out.positionals, args[i+1:]...)
			break
		}
		if a == "-h" || a == "--help" {
			return out, "help"
		}
		if !strings.HasPrefix(a, "-") {
			out.positionals = append(out.positionals, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		val := ""
		hasVal := false
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			val = name[eq+1:]
			name = name[:eq]
			hasVal = true
		}
		if boolFlags[name] {
			out.bools[name] = true
			continue
		}
		if !hasVal {
			if i+1 >= len(args) {
				return out, "error: argument --" + name + ": expected one argument"
			}
			i++
			val = args[i]
		}
		out.values[name] = val
	}
	return out, ""
}

func checkUnknownFlags(p parsedCLI, allowed ...string) string {
	ok := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		ok[a] = struct{}{}
	}
	for name := range p.values {
		if _, found := ok[name]; !found {
			return "error: unrecognized arguments: --" + name
		}
	}
	for name := range p.bools {
		if _, found := ok[name]; !found {
			return "error: unrecognized arguments: --" + name
		}
	}
	return ""
}

func parseFormat(p parsedCLI) (string, string) {
	f, ok := p.values["format"]
	if !ok || f == "" {
		return "text", ""
	}
	if f != "text" && f != "json" {
		return "", fmt.Sprintf("error: argument --format: invalid choice: %q (choose from text, json)", f)
	}
	return f, ""
}

func resolveRepoRoot(flagVal string) (string, error) {
	if strings.TrimSpace(flagVal) != "" {
		abs, err := filepath.Abs(flagVal)
		if err != nil {
			return "", fmt.Errorf("resolve --repo-root: %w", err)
		}
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			return "", fmt.Errorf("repo root directory not found: %s", flagVal)
		}
		return abs, nil
	}
	var starts []string
	if env := strings.TrimSpace(os.Getenv("ALT_HARNESS_REPO_ROOT")); env != "" {
		starts = append(starts, env)
	}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		starts = append(starts, filepath.Dir(file))
	}
	seen := map[string]struct{}{}
	for _, start := range starts {
		dir := start
		for {
			if _, ok := seen[dir]; ok {
				break
			}
			seen[dir] = struct{}{}
			if st, err := os.Stat(filepath.Join(dir, "scripts", "altharness", "go.mod")); err == nil && !st.IsDir() {
				abs, err := filepath.Abs(dir)
				if err != nil {
					return "", err
				}
				return abs, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", errors.New("repo root with scripts/altharness/go.mod not found")
}

func writeJSONBytes(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return buf.Bytes(), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sha256File(path string) (string, int64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	return sha256Hex(raw), int64(len(raw)), nil
}

func isSymlink(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

func isWithinDir(path, root string) bool {
	p := filepath.Clean(path)
	r := filepath.Clean(root)
	if p == r {
		return true
	}
	rel, err := filepath.Rel(r, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveSafeRepoPath validates that rel is a repo-relative path with no
// traversal, absolute prefix, backslash, or symlink component escaping repoRoot.
func resolveSafeRepoPath(repoRoot, rawRel string) (string, string, error) {
	trimmed := strings.TrimSpace(rawRel)
	if trimmed == "" || strings.ContainsRune(trimmed, 0) {
		return "", "empty or NUL path", errors.New("unsafe path")
	}
	if strings.HasPrefix(trimmed, "/") || filepath.IsAbs(trimmed) || strings.Contains(trimmed, `\`) {
		return "", fmt.Sprintf("path %q must be repo-relative (no absolute paths or backslashes)", rawRel), errors.New("unsafe path")
	}
	parts := strings.Split(trimmed, "/")
	cleanParts := make([]string, 0, len(parts))
	for _, seg := range parts {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." {
			return "", fmt.Sprintf("path %q contains forbidden parent traversal '..'", rawRel), errors.New("unsafe path")
		}
		cleanParts = append(cleanParts, seg)
	}
	if len(cleanParts) == 0 {
		return "", fmt.Sprintf("path %q resolves to repo root instead of a file", rawRel), errors.New("unsafe path")
	}
	rootResolved, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return "", fmt.Sprintf("cannot resolve repo root: %v", err), err
	}
	cur := repoRoot
	for _, seg := range cleanParts {
		cur = filepath.Join(cur, seg)
		if isSymlink(cur) {
			return "", fmt.Sprintf("path %q traverses symlink component %q", rawRel, seg), errors.New("symlink")
		}
	}
	if !isWithinDir(cur, repoRoot) {
		return "", fmt.Sprintf("path %q escapes repo root", rawRel), errors.New("escape")
	}
	if _, err := os.Lstat(cur); err == nil {
		resolved, err := filepath.EvalSymlinks(cur)
		if err != nil {
			return "", fmt.Sprintf("cannot resolve %q: %v", rawRel, err), err
		}
		if !isWithinDir(resolved, rootResolved) {
			return "", fmt.Sprintf("resolved path %q escapes repo root", rawRel), errors.New("escape")
		}
	}
	return cur, "", nil
}

func ensureNoSymlinkAncestors(target string) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	cur := filepath.Clean(abs)
	var stack []string
	for {
		stack = append(stack, cur)
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	tmpRoot := filepath.Clean(os.TempDir())
	for i := len(stack) - 1; i >= 0; i-- {
		p := stack[i]
		st, lerr := os.Lstat(p)
		if lerr != nil {
			if errors.Is(lerr, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("stat %q: %w", p, lerr)
		}
		if st.Mode()&os.ModeSymlink != 0 {
			if p == tmpRoot {
				continue
			}
			return fmt.Errorf("evidence-dir must not traverse or target a symlink: %s", p)
		}
	}
	return nil
}

func ensureSafeEvidenceDir(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" || strings.ContainsRune(dir, 0) {
		return "", errors.New("evidence-dir must be non-empty")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve evidence-dir: %w", err)
	}
	if err := ensureNoSymlinkAncestors(abs); err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, fsperm.Dir); err != nil {
		return "", fmt.Errorf("create evidence-dir %s: %w", abs, err)
	}
	if err := ensureNoSymlinkAncestors(abs); err != nil {
		return "", err
	}
	return abs, nil
}

func writeEvidenceFile(dir, name string, data []byte) error {
	if strings.Contains(name, "/") || strings.Contains(name, `\`) || strings.Contains(name, "..") {
		return fmt.Errorf("unsafe evidence filename: %q", name)
	}
	dest := filepath.Join(dir, name)
	if isSymlink(dest) {
		return fmt.Errorf("refusing to overwrite symlink: %s", dest)
	}
	return os.WriteFile(dest, data, fsperm.File)
}

func runExec(ctx context.Context, dir string, extraEnv []string, timeout time.Duration, name string, args ...string) ([]byte, []byte, int, error) {
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
	if dir != "" {
		cmd.Dir = dir
	}
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), stderr.Bytes(), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.Bytes(), stderr.Bytes(), exitErr.ExitCode(), nil
	}
	return stdout.Bytes(), stderr.Bytes(), -1, err
}

func cmdValidate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := ctx.Err(); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitInvalid
	}
	p, perr := parseCLI(args, nil)
	if perr == "help" {
		writeHelp(stdout)
		return exitOK
	}
	if perr != "" {
		fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if msg := checkUnknownFlags(p, "repo-root", "format"); msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	format, msg := parseFormat(p)
	if msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if len(p.positionals) != 1 {
		fmt.Fprintln(stderr, "error: the following arguments are required: report_path")
		return exitUsage
	}
	reportPath := p.positionals[0]
	st, err := os.Stat(reportPath)
	if err != nil || st.IsDir() {
		fmt.Fprintf(stderr, "error: report file not found: %s\n", reportPath)
		return exitUsage
	}
	root, err := resolveRepoRoot(p.values["repo-root"])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}
	res, err := validateReport(reportPath, root)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}
	return emitValidateResult(stdout, stderr, "validate", res, format)
}

func cmdCheckTemplate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := ctx.Err(); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitInvalid
	}
	p, perr := parseCLI(args, nil)
	if perr == "help" {
		writeHelp(stdout)
		return exitOK
	}
	if perr != "" {
		fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if msg := checkUnknownFlags(p, "repo-root", "format"); msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	format, msg := parseFormat(p)
	if msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if len(p.positionals) != 0 {
		fmt.Fprintf(stderr, "error: unrecognized arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	root, err := resolveRepoRoot(p.values["repo-root"])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}
	res := checkTemplate(root)
	return emitValidateResult(stdout, stderr, "check-template", res, format)
}

func cmdPlectoWIT(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := ctx.Err(); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitInvalid
	}
	p, perr := parseCLI(args, nil)
	if perr == "help" {
		writeHelp(stdout)
		return exitOK
	}
	if perr != "" {
		fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if msg := checkUnknownFlags(p, "repo-root", "base-commit", "artifact-dir", "before-wasm", "after-wasm", "evidence-dir", "format"); msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	format, msg := parseFormat(p)
	if msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if len(p.positionals) != 0 {
		fmt.Fprintf(stderr, "error: unrecognized arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	beforeWasm := strings.TrimSpace(p.values["before-wasm"])
	afterWasm := strings.TrimSpace(p.values["after-wasm"])
	if (beforeWasm == "") != (afterWasm == "") {
		fmt.Fprintln(stderr, "error: --before-wasm and --after-wasm must be provided together")
		return exitUsage
	}
	baseRev := strings.TrimSpace(p.values["base-commit"])
	if strings.HasPrefix(baseRev, "-") {
		fmt.Fprintf(stderr, "error: invalid --base-commit revision: %q\n", baseRev)
		return exitUsage
	}
	if baseRev == "" {
		baseRev = defaultPlectoBaseRev
	}
	var root string
	if beforeWasm == "" {
		var err error
		root, err = resolveRepoRoot(p.values["repo-root"])
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitUsage
		}
	} else if v := strings.TrimSpace(p.values["repo-root"]); v != "" {
		var err error
		root, err = resolveRepoRoot(v)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitUsage
		}
	}
	evDir := strings.TrimSpace(p.values["evidence-dir"])
	if evDir == "" && root != "" {
		evDir = filepath.Join(root, ".harness", "evidence", "verify-06", "plecto-wit")
	}
	return runPlectoWIT(ctx, plectoWITOptions{
		repoRoot:    root,
		baseCommit:  baseRev,
		artifactDir: strings.TrimSpace(p.values["artifact-dir"]),
		beforeWasm:  beforeWasm,
		afterWasm:   afterWasm,
		evidenceDir: evDir,
		format:      format,
	}, stdout, stderr)
}

func cmdRecapClippy(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := ctx.Err(); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitInvalid
	}
	p, perr := parseCLI(args, nil)
	if perr == "help" {
		writeHelp(stdout)
		return exitOK
	}
	if perr != "" {
		fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if msg := checkUnknownFlags(p, "repo-root", "project-dir", "evidence-dir", "format"); msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	format, msg := parseFormat(p)
	if msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if len(p.positionals) != 0 {
		fmt.Fprintf(stderr, "error: unrecognized arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	root, err := resolveRepoRoot(p.values["repo-root"])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}
	projRel := strings.TrimSpace(p.values["project-dir"])
	if projRel == "" {
		projRel = defaultRecapProjRel
	}
	evDir := strings.TrimSpace(p.values["evidence-dir"])
	if evDir == "" {
		evDir = filepath.Join(root, ".harness", "evidence", "verify-06", "recap-clippy")
	}
	return runRecapClippy(ctx, recapClippyOptions{
		repoRoot:    root,
		projectRel:  projRel,
		evidenceDir: evDir,
		format:      format,
	}, stdout, stderr)
}

func emitValidateResult(stdout, stderr io.Writer, kind string, res ValidateResult, format string) int {
	if format == "json" {
		raw, err := writeJSONBytes(res)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitUsage
		}
		_, _ = stdout.Write(raw)
		if res.Valid {
			return exitOK
		}
		return exitInvalid
	}
	if res.Valid {
		fmt.Fprintf(stdout, "OK: verify-claims %s %s (claims=%d, evidence_paths=%d)\n", kind, res.Path, res.ClaimsChecked, res.EvidencePathsChecked)
		return exitOK
	}
	for _, iss := range res.Errors {
		fmt.Fprintf(stderr, "ERROR [%s] %s: %s\n", iss.Code, iss.Path, iss.Message)
	}
	fmt.Fprintf(stderr, "INVALID: %d error(s) in %s\n", len(res.Errors), res.Path)
	return exitInvalid
}
