// Package verifylive implements `alt-harness verify-live` (VERIFY-04):
// production-isolated live loopback verification probes, feature-map index
// hygiene doctor, durable evidence persistence under `.harness/evidence/<task>/`,
// and C-07 live SSE corpus evaluation.
//
// Pure Go standard library only (ADR-001028).
package verifylive

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"alt/scripts/altharness/internal/fsperm"
)

const (
	// ExitOK indicates all checks and probes passed.
	ExitOK = 0
	// ExitFail indicates a failed doctor check, probe, or corpus check.
	ExitFail = 1
	// ExitUsage indicates invalid CLI arguments or unsafe inputs.
	ExitUsage = 2

	// SignalC07SSEHang is emitted when C-07's buggy after.py hangs the live SSE stream.
	SignalC07SSEHang = "VERIFY_04_SSE_TERMINATION_HANG"

	defaultC07Timeout = 1500 * time.Millisecond
)

var taskNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// Options configures a verify-live invocation without mutating process globals.
type Options struct {
	Stdout io.Writer
	Stderr io.Writer
	Env    map[string]string
}

// CheckResult is one doctor preflight check outcome.
type CheckResult struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// DoctorReport is the structured result of `verify-live doctor`.
type DoctorReport struct {
	OK            bool          `json:"ok"`
	IsolationMode string        `json:"isolation_mode"`
	RepoRoot      string        `json:"repo_root"`
	EvidenceDir   string        `json:"evidence_dir"`
	Checks        []CheckResult `json:"checks"`
	Features      []string      `json:"features"`
}

// ProbeCheck is one assertion inside a live probe.
type ProbeCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// ProbeResult is the structured evidence for one executed live probe.
type ProbeResult struct {
	Name         string           `json:"name"`
	Service      string           `json:"service"`
	OK           bool             `json:"ok"`
	DurationMS   int64            `json:"duration_ms"`
	BindAddr     string           `json:"bind_addr,omitempty"`
	Checks       []ProbeCheck     `json:"checks"`
	Error        string           `json:"error,omitempty"`
	EvidenceJSON string           `json:"evidence_json,omitempty"`
	EvidenceLog  string           `json:"evidence_log,omitempty"`
	C07Mutation  *C07CorpusResult `json:"c07_mutation,omitempty"`
}

// RunSummary is written to `.harness/evidence/<task>/summary.json`.
type RunSummary struct {
	Task          string        `json:"task"`
	Timestamp     string        `json:"timestamp"`
	RepoRoot      string        `json:"repo_root"`
	GitHead       string        `json:"git_head"`
	IsolationMode string        `json:"isolation_mode"`
	OK            bool          `json:"ok"`
	Doctor        DoctorReport  `json:"doctor"`
	Probes        []ProbeResult `json:"probes"`
	EvidenceDir   string        `json:"evidence_dir"`
}

// C07CorpusResult records live SSE before/after evaluation for corpus case C-07.
type C07CorpusResult struct {
	BeforePath   string `json:"before_path"`
	AfterPath    string `json:"after_path"`
	BeforePassed bool   `json:"before_passed"`
	AfterPassed  bool   `json:"after_passed"`
	Detected     bool   `json:"detected"`
	Signal       string `json:"signal,omitempty"`
	BeforeDetail string `json:"before_detail"`
	AfterDetail  string `json:"after_detail"`
}

// Run executes `alt-harness verify-live` using the current process environment.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return RunWithOptions(ctx, args, Options{
		Stdout: stdout,
		Stderr: stderr,
	})
}

// RunWithOptions executes `alt-harness verify-live` with injectable streams and env overrides.
func RunWithOptions(ctx context.Context, args []string, opts Options) int {
	if ctx == nil {
		ctx = context.Background()
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) == 0 {
		printUsage(stderr)
		return ExitUsage
	}
	switch args[0] {
	case "doctor":
		return runDoctorCLI(ctx, args[1:], opts.Env, stdout, stderr)
	case "run":
		return runProbesCLI(ctx, args[1:], opts.Env, stdout, stderr)
	case "check-c07-corpus":
		return runCheckC07CLI(ctx, args[1:], opts.Env, stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return ExitOK
	default:
		_, _ = fmt.Fprintf(stderr, "ERROR: unknown verify-live subcommand %q\n", args[0])
		printUsage(stderr)
		return ExitUsage
	}
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: alt-harness verify-live <doctor|run|check-c07-corpus> [flags]")
	_, _ = fmt.Fprintln(w, "  doctor           [--hygiene-only] [--repo-root DIR] [--evidence-dir DIR] [--project NAME] [--format text|json]")
	_, _ = fmt.Fprintln(w, "  run              --task NAME [--probe all|sse|body-limit|auth-hub|inference-proxy] [--check-c07-mutation] [--repo-root DIR] [--evidence-dir DIR] [--project NAME] [--format text|json]")
	_, _ = fmt.Fprintln(w, "  check-c07-corpus --before PATH --after PATH [--timeout DURATION] [--format text|json]")
}

// ValidateTaskName enforces CWE-22 path traversal safety on task identifiers.
func ValidateTaskName(task string) error {
	if task == "" {
		return errors.New("--task is required")
	}
	if strings.Contains(task, "..") || strings.ContainsAny(task, `/\`) {
		return fmt.Errorf("invalid --task %q: path separators and '..' are forbidden", task)
	}
	if !taskNameRE.MatchString(task) {
		return fmt.Errorf("invalid --task %q: must match %s", task, taskNameRE.String())
	}
	return nil
}

func runDoctorCLI(ctx context.Context, args []string, envOverrides map[string]string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify-live doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repoFlag := fs.String("repo-root", "", "repository root directory")
	evidenceFlag := fs.String("evidence-dir", "", "evidence root directory")
	projectFlag := fs.String("project", "", "compose project name to validate")
	hygieneOnlyFlag := fs.Bool("hygiene-only", false, "validate isolation, skills, and feature-map hygiene without writing probe files or requiring service venvs")
	formatFlag := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return ExitUsage
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "ERROR: unexpected positional arguments: %v\n", fs.Args())
		return ExitUsage
	}
	if *formatFlag != "text" && *formatFlag != "json" {
		_, _ = fmt.Fprintf(stderr, "ERROR: invalid --format %q (want text or json)\n", *formatFlag)
		return ExitUsage
	}
	repoRoot, err := resolveRepoRoot(ctx, *repoFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return ExitFail
	}
	evidenceDir := resolveEvidenceDir(repoRoot, *evidenceFlag)
	report := RunDoctorWithOptions(ctx, repoRoot, evidenceDir, *projectFlag, envOverrides, DoctorOptions{
		HygieneOnly:      *hygieneOnlyFlag,
		RequireSSEPython: !*hygieneOnlyFlag,
	})
	if err := writeDoctorOutput(stdout, report, *formatFlag); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return ExitFail
	}
	if !report.OK {
		return ExitFail
	}
	return ExitOK
}

func runProbesCLI(ctx context.Context, args []string, envOverrides map[string]string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify-live run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	taskFlag := fs.String("task", "", "task identifier for evidence subdirectory")
	probeFlag := fs.String("probe", "all", "probe target: all|sse|body-limit|auth-hub|inference-proxy")
	c07MutationFlag := fs.Bool("check-c07-mutation", false, "also verify C-07 mutation fails live SSE probe")
	repoFlag := fs.String("repo-root", "", "repository root directory")
	evidenceFlag := fs.String("evidence-dir", "", "evidence root directory")
	projectFlag := fs.String("project", "", "compose project name guard check")
	formatFlag := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return ExitUsage
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "ERROR: unexpected positional arguments: %v\n", fs.Args())
		return ExitUsage
	}
	if err := ValidateTaskName(*taskFlag); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return ExitUsage
	}
	if *formatFlag != "text" && *formatFlag != "json" {
		_, _ = fmt.Fprintf(stderr, "ERROR: invalid --format %q (want text or json)\n", *formatFlag)
		return ExitUsage
	}
	probesToRun, err := expandProbeSelection(*probeFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return ExitUsage
	}

	repoRoot, err := resolveRepoRoot(ctx, *repoFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return ExitFail
	}
	evidenceRoot := resolveEvidenceDir(repoRoot, *evidenceFlag)
	if err := ensureNoSymlinkPath(evidenceRoot); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: unsafe --evidence-dir: %v\n", err)
		return ExitFail
	}
	taskDir, err := safeTaskEvidenceDir(evidenceRoot, *taskFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return ExitUsage
	}
	if err := ensureNoSymlinkPath(taskDir); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: unsafe task evidence dir: %v\n", err)
		return ExitFail
	}

	requireSSEPython := false
	for _, p := range probesToRun {
		if p == "sse" {
			requireSSEPython = true
			break
		}
	}
	doctor := RunDoctorWithOptions(ctx, repoRoot, evidenceRoot, *projectFlag, envOverrides, DoctorOptions{
		RequireSSEPython: requireSSEPython,
	})
	summary := RunSummary{
		Task:          *taskFlag,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		RepoRoot:      repoRoot,
		GitHead:       resolveGitHead(ctx, repoRoot),
		IsolationMode: doctor.IsolationMode,
		Doctor:        doctor,
		EvidenceDir:   taskDir,
	}

	if !doctor.OK {
		summary.OK = false
		_ = writeRunSummaryOutput(stdout, summary, *formatFlag)
		return ExitFail
	}

	if err := os.MkdirAll(taskDir, fsperm.Dir); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: create task evidence dir %s: %v\n", taskDir, err)
		return ExitFail
	}
	if err := ensureNoSymlinkPath(taskDir); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: unsafe task evidence dir %s: %v\n", taskDir, err)
		return ExitFail
	}
	if err := writeJSONFile(filepath.Join(taskDir, "doctor.json"), doctor); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: write doctor.json: %v\n", err)
		return ExitFail
	}

	allOK := true
	for _, probeName := range probesToRun {
		res, logText := executeProbe(ctx, repoRoot, probeName, *c07MutationFlag)
		jsonPath := filepath.Join(taskDir, probeName+".json")
		logPath := filepath.Join(taskDir, probeName+".log")
		res.EvidenceJSON = jsonPath
		res.EvidenceLog = logPath
		if writeErr := writeRegularFile(logPath, []byte(logText)); writeErr != nil {
			res.OK = false
			res.Error = fmt.Sprintf("write evidence log: %v", writeErr)
		}
		if writeErr := writeJSONFile(jsonPath, res); writeErr != nil {
			res.OK = false
			res.Error = fmt.Sprintf("write evidence json: %v", writeErr)
		}
		if !res.OK {
			allOK = false
		}
		summary.Probes = append(summary.Probes, res)
	}
	summary.OK = allOK

	if err := writeJSONFile(filepath.Join(taskDir, "summary.json"), summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: write summary.json: %v\n", err)
		return ExitFail
	}
	if err := writeRunSummaryOutput(stdout, summary, *formatFlag); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return ExitFail
	}
	if !summary.OK {
		return ExitFail
	}
	return ExitOK
}

func runCheckC07CLI(ctx context.Context, args []string, envOverrides map[string]string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify-live check-c07-corpus", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	beforeFlag := fs.String("before", "", "path to fixed before.py")
	afterFlag := fs.String("after", "", "path to buggy after.py")
	repoFlag := fs.String("repo-root", "", "optional repository root for default C-07 paths")
	timeoutFlag := fs.Duration("timeout", defaultC07Timeout, "per-stream SSE read timeout")
	formatFlag := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return ExitUsage
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "ERROR: unexpected positional arguments: %v\n", fs.Args())
		return ExitUsage
	}
	if *formatFlag != "text" && *formatFlag != "json" {
		_, _ = fmt.Fprintf(stderr, "ERROR: invalid --format %q (want text or json)\n", *formatFlag)
		return ExitUsage
	}
	if iso := checkIsolation("", envOverrides); !iso.OK {
		_, _ = fmt.Fprintf(stderr, "ERROR: %s\n", iso.Detail)
		return ExitFail
	}
	beforePath := *beforeFlag
	afterPath := *afterFlag
	if beforePath == "" || afterPath == "" {
		repoRoot, err := resolveRepoRoot(ctx, *repoFlag)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR: --before and --after are required (or valid --repo-root): %v\n", err)
			return ExitUsage
		}
		if beforePath == "" {
			beforePath = filepath.Join(repoRoot, "scripts", "harness", "corpus", "C-07", "before.py")
		}
		if afterPath == "" {
			afterPath = filepath.Join(repoRoot, "scripts", "harness", "corpus", "C-07", "after.py")
		}
	}
	if *timeoutFlag <= 0 {
		_, _ = fmt.Fprintln(stderr, "ERROR: --timeout must be positive")
		return ExitUsage
	}

	res, err := CheckC07Corpus(ctx, beforePath, afterPath, *timeoutFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return ExitFail
	}
	if *formatFlag == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR: encode json: %v\n", err)
			return ExitFail
		}
	} else {
		status := "FAIL"
		if res.Detected {
			status = "PASS"
		}
		_, _ = fmt.Fprintf(stdout, "check-c07-corpus: %s\n", status)
		_, _ = fmt.Fprintf(stdout, "  before (%s): passed=%v (%s)\n", res.BeforePath, res.BeforePassed, res.BeforeDetail)
		_, _ = fmt.Fprintf(stdout, "  after  (%s): passed=%v (%s)\n", res.AfterPath, res.AfterPassed, res.AfterDetail)
		if res.Signal != "" {
			_, _ = fmt.Fprintf(stdout, "  signal: %s\n", res.Signal)
		}
	}
	if !res.Detected {
		return ExitFail
	}
	return ExitOK
}

func expandProbeSelection(probe string) ([]string, error) {
	switch probe {
	case "all":
		return []string{"sse", "body-limit", "auth-hub", "inference-proxy"}, nil
	case "sse", "body-limit", "auth-hub", "inference-proxy":
		return []string{probe}, nil
	default:
		return nil, fmt.Errorf("unknown --probe %q (want all|sse|body-limit|auth-hub|inference-proxy)", probe)
	}
}

func safeTaskEvidenceDir(evidenceRoot, task string) (string, error) {
	absRoot, err := filepath.Abs(evidenceRoot)
	if err != nil {
		return "", fmt.Errorf("resolve evidence dir: %w", err)
	}
	taskDir := filepath.Join(absRoot, task)
	rel, err := filepath.Rel(absRoot, taskDir)
	if err != nil || rel != task || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("task evidence directory %q escapes root %q", taskDir, absRoot)
	}
	return taskDir, nil
}

func resolveEvidenceDir(repoRoot, override string) string {
	if strings.TrimSpace(override) != "" {
		if abs, err := filepath.Abs(override); err == nil {
			return abs
		}
		return override
	}
	return filepath.Join(repoRoot, ".harness", "evidence")
}

func resolveRepoRoot(ctx context.Context, override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve --repo-root: %w", err)
		}
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			return "", fmt.Errorf("repo root does not exist: %s", abs)
		}
		return abs, nil
	}
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for {
			if st, statErr := os.Stat(filepath.Join(dir, "scripts", "altharness", "go.mod")); statErr == nil && !st.IsDir() {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	if _, err := exec.LookPath("git"); err == nil {
		out, gerr := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
		if gerr == nil {
			if top := strings.TrimSpace(string(out)); top != "" {
				return top, nil
			}
		}
	}
	return "", errors.New("could not determine repository root; pass --repo-root")
}

func resolveGitHead(ctx context.Context, repoRoot string) string {
	if _, err := exec.LookPath("git"); err != nil {
		return "unknown"
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	head := strings.TrimSpace(string(out))
	if head == "" {
		return "unknown"
	}
	return head
}

func ensureNoSymlinkPath(target string) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve path %q: %w", target, err)
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
	// Walk top-down; allow canonical OS temp prefixes (e.g. /var -> /private/var)
	// only above the first non-system dir, while strictly rejecting symlinks at or
	// under repo/evidence targets.
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
			return fmt.Errorf("refusing symlink component %q", p)
		}
	}
	return nil
}

func writeRegularFile(path string, data []byte) error {
	if err := ensureNoSymlinkPath(path); err != nil {
		return err
	}
	return os.WriteFile(path, data, fsperm.File)
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeRegularFile(path, data)
}

func writeDoctorOutput(w io.Writer, report DoctorReport, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	status := "OK"
	if !report.OK {
		status = "FAILED"
	}
	_, _ = fmt.Fprintf(w, "verify-live doctor: %s (isolation=%s, features=%d)\n", status, report.IsolationMode, len(report.Features))
	for _, c := range report.Checks {
		mark := "PASS"
		if !c.OK {
			mark = "FAIL"
		}
		_, _ = fmt.Fprintf(w, "  [%s] %s: %s\n", mark, c.Name, c.Detail)
	}
	return nil
}

func writeRunSummaryOutput(w io.Writer, summary RunSummary, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}
	status := "OK"
	if !summary.OK {
		status = "FAILED"
	}
	_, _ = fmt.Fprintf(w, "verify-live run [%s]: %s (evidence=%s)\n", summary.Task, status, summary.EvidenceDir)
	if !summary.Doctor.OK {
		_ = writeDoctorOutput(w, summary.Doctor, "text")
		return nil
	}
	for _, p := range summary.Probes {
		mark := "PASS"
		if !p.OK {
			mark = "FAIL"
		}
		_, _ = fmt.Fprintf(w, "  [%s] %-16s (%dms, %s)\n", mark, p.Name, p.DurationMS, p.Service)
		if p.Error != "" {
			_, _ = fmt.Fprintf(w, "         error: %s\n", p.Error)
		}
		for _, c := range p.Checks {
			cmark := "ok"
			if !c.OK {
				cmark = "FAIL"
			}
			_, _ = fmt.Fprintf(w, "         - [%s] %s: %s\n", cmark, c.Name, c.Detail)
		}
	}
	return nil
}
