// Package verifydiff implements VERIFY-02 differential property-based testing
// for split long functions: extracting pre-split code via `git show` into an
// isolated temporary directory, invoking a differential runner with seeded
// inputs, recording structured evidence JSON and LOOP-03 decisions.tsv rows,
// and verifying no temporary old-implementation scaffolds remain tracked.
package verifydiff

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/guards"
)

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2

	defaultTimeoutSeconds = 180
	maxExcerptBytes       = 4000
	shortSHALen           = 12
	tempScaffoldMarker    = "# VERIFY-02-" + "TEMP-OLD-IMPL"
)

// Evidence is the durable JSON record written to .harness/evidence/verify-02/<label>.json.
type Evidence struct {
	BaseCommit      string  `json:"base_commit"`
	BaseCommitInput string  `json:"base_commit_input"`
	File            string  `json:"file"`
	Label           string  `json:"label"`
	Seed            int64   `json:"seed"`
	RunnerCmd       string  `json:"runner_cmd"`
	ExitCode        int     `json:"exit_code"`
	Passed          bool    `json:"passed"`
	DurationS       float64 `json:"duration_s"`
	OutputSHA256    string  `json:"output_sha256"`
	StdoutExcerpt   string  `json:"stdout_excerpt"`
	StderrExcerpt   string  `json:"stderr_excerpt,omitempty"`
	EvidencePath    string  `json:"evidence_path"`
	DecisionsFile   string  `json:"decisions_file"`
	RecordedAt      string  `json:"recorded_at"`
}

// CleanFinding reports a single tracked temporary differential scaffold violation.
type CleanFinding struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// CleanReport is the structured output of `verify-diff check-clean`.
type CleanReport struct {
	RepoRoot     string         `json:"repo_root"`
	TrackedCount int            `json:"tracked_count"`
	Clean        bool           `json:"clean"`
	Findings     []CleanFinding `json:"findings"`
}

// Run dispatches `alt-harness verify-diff <run|check-clean> ...`.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stdout, usageText())
		if len(args) == 0 {
			_, _ = fmt.Fprintln(stderr, "error: subcommand required (run | check-clean)")
			return exitUsage
		}
		return exitOK
	}
	switch args[0] {
	case "run":
		return runDiff(ctx, args[1:], stdout, stderr)
	case "check-clean":
		return runCheckClean(ctx, args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "error: unknown verify-diff subcommand %q\n%s", args[0], usageText())
		return exitUsage
	}
}

func usageText() string {
	return "usage:\n" +
		"  alt-harness verify-diff run --base-commit REV --file PATH --runner-cmd CMD --seed N " +
		"[--repo-root DIR] [--decisions-file PATH] [--evidence-dir DIR] [--label NAME] " +
		"[--timeout SEC] [--format text|json]\n" +
		"  alt-harness verify-diff check-clean [--repo-root DIR] [--format text|json]\n"
}

type runFlags struct {
	baseCommit    string
	file          string
	runnerCmd     string
	seed          int64
	hasSeed       bool
	repoRoot      string
	decisionsFile string
	evidenceDir   string
	label         string
	format        string
	timeoutSec    int
	help          bool
}

func parseRunFlags(args []string) (runFlags, error) {
	cfg := runFlags{
		format:     "text",
		timeoutSec: defaultTimeoutSeconds,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" || arg == "--help" {
			cfg.help = true
			return cfg, nil
		}
		name, val, hasEq := splitFlag(arg)
		nextVal := func() (string, error) {
			if hasEq {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("flag %s requires a value", name)
			}
			i++
			return args[i], nil
		}
		switch name {
		case "--base-commit":
			v, err := nextVal()
			if err != nil {
				return cfg, err
			}
			cfg.baseCommit = v
		case "--file":
			v, err := nextVal()
			if err != nil {
				return cfg, err
			}
			cfg.file = v
		case "--runner-cmd":
			v, err := nextVal()
			if err != nil {
				return cfg, err
			}
			cfg.runnerCmd = v
		case "--seed":
			v, err := nextVal()
			if err != nil {
				return cfg, err
			}
			n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return cfg, fmt.Errorf("invalid --seed %q: %w", v, err)
			}
			cfg.seed = n
			cfg.hasSeed = true
		case "--repo-root":
			v, err := nextVal()
			if err != nil {
				return cfg, err
			}
			cfg.repoRoot = v
		case "--decisions-file":
			v, err := nextVal()
			if err != nil {
				return cfg, err
			}
			cfg.decisionsFile = v
		case "--evidence-dir":
			v, err := nextVal()
			if err != nil {
				return cfg, err
			}
			cfg.evidenceDir = v
		case "--label":
			v, err := nextVal()
			if err != nil {
				return cfg, err
			}
			cfg.label = v
		case "--format":
			v, err := nextVal()
			if err != nil {
				return cfg, err
			}
			if v != "text" && v != "json" {
				return cfg, fmt.Errorf("invalid --format %q (expected text or json)", v)
			}
			cfg.format = v
		case "--timeout":
			v, err := nextVal()
			if err != nil {
				return cfg, err
			}
			sec, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || sec <= 0 {
				return cfg, fmt.Errorf("invalid --timeout %q: must be positive integer seconds", v)
			}
			cfg.timeoutSec = sec
		default:
			return cfg, fmt.Errorf("unrecognized argument %q", arg)
		}
	}
	if cfg.baseCommit == "" || cfg.file == "" || cfg.runnerCmd == "" || !cfg.hasSeed {
		return cfg, errors.New("run requires --base-commit, --file, --runner-cmd, and --seed")
	}
	return cfg, nil
}

func splitFlag(arg string) (name, val string, hasEq bool) {
	if idx := strings.IndexByte(arg, '='); idx >= 0 {
		return arg[:idx], arg[idx+1:], true
	}
	return arg, "", false
}

type resolvedRunSpec struct {
	baseInput     string
	runnerArgv    []string
	repoResolved  string
	relFile       string
	absNewFile    string
	label         string
	evidenceDir   string
	decisionsPath string
}

func resolveRunSpec(cfg runFlags) (resolvedRunSpec, int, error) {
	baseInput, err := validateBaseCommitInput(cfg.baseCommit)
	if err != nil {
		return resolvedRunSpec{}, exitUsage, err
	}
	runnerArgv, err := splitShellWords(cfg.runnerCmd)
	if err != nil {
		return resolvedRunSpec{}, exitUsage, err
	}
	repoRoot := guards.ResolveRepoRoot(cfg.repoRoot, "")
	repoResolved, err := resolveExistingDir(repoRoot)
	if err != nil {
		return resolvedRunSpec{}, exitUsage, fmt.Errorf("invalid --repo-root: %w", err)
	}
	relFile, absNewFile, err := validateRepoFilePath(repoResolved, cfg.file)
	if err != nil {
		return resolvedRunSpec{}, exitUsage, err
	}
	label := SanitizeLabel(relFile)
	if strings.TrimSpace(cfg.label) != "" {
		label, err = validateExplicitLabel(cfg.label)
		if err != nil {
			return resolvedRunSpec{}, exitUsage, err
		}
	}
	defaultEvidenceDir := filepath.Join(repoResolved, ".harness", "evidence", "verify-02")
	evidenceDir, err := resolveSafeOutputPath(repoResolved, cfg.evidenceDir, defaultEvidenceDir)
	if err != nil {
		return resolvedRunSpec{}, exitUsage, fmt.Errorf("invalid --evidence-dir: %w", err)
	}
	runDir, err := guards.RunDirForBranch(repoResolved, "")
	if err != nil {
		return resolvedRunSpec{}, exitFail, fmt.Errorf("could not resolve harness run dir: %w", err)
	}
	branchDecisions := filepath.Join(runDir, "decisions.tsv")
	defaultDecisionsFile := filepath.Join(evidenceDir, "decisions.tsv")
	if st, serr := os.Lstat(branchDecisions); serr == nil && st.Mode().IsRegular() {
		// Append into the active branch run only when it has already been initialized
		// via `alt-harness harness-run init` (avoids creating an uninitialized header-only
		// branch decisions.tsv that blocks subsequent `harness-run init`).
		defaultDecisionsFile = branchDecisions
	}
	decisionsPath, err := resolveSafeOutputPath(repoResolved, cfg.decisionsFile, defaultDecisionsFile)
	if err != nil {
		return resolvedRunSpec{}, exitUsage, fmt.Errorf("invalid --decisions-file: %w", err)
	}
	return resolvedRunSpec{
		baseInput:     baseInput,
		runnerArgv:    runnerArgv,
		repoResolved:  repoResolved,
		relFile:       relFile,
		absNewFile:    absNewFile,
		label:         label,
		evidenceDir:   evidenceDir,
		decisionsPath: decisionsPath,
	}, exitOK, nil
}

func runDiff(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, err := parseRunFlags(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}
	if cfg.help {
		_, _ = fmt.Fprint(stdout, usageText())
		return exitOK
	}

	spec, errCode, err := resolveRunSpec(cfg)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return errCode
	}

	resolvedCommit, err := resolveGitCommit(ctx, spec.repoResolved, spec.baseInput)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitFail
	}

	oldContent, err := gitShowFile(ctx, spec.repoResolved, resolvedCommit, spec.relFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitFail
	}

	tmpDir, err := os.MkdirTemp("", "alt-verify-diff-*")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: failed to create temp dir: %v\n", err)
		return exitFail
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	oldFilePath := filepath.Join(tmpDir, "old_"+filepath.Base(spec.relFile))
	if err := os.WriteFile(oldFilePath, oldContent, fsperm.File); err != nil {
		_, _ = fmt.Fprintf(stderr, "error: failed to materialize pre-split file: %v\n", err)
		return exitFail
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.timeoutSec)*time.Second)
	defer cancel()

	start := time.Now()
	runnerStdout, runnerStderr, exitCode, execErr := executeRunner(
		runCtx,
		spec.repoResolved,
		spec.runnerArgv,
		map[string]string{
			"ALT_DIFF_OLD_FILE":    oldFilePath,
			"ALT_DIFF_NEW_FILE":    spec.absNewFile,
			"ALT_DIFF_REL_FILE":    spec.relFile,
			"ALT_DIFF_BASE_COMMIT": resolvedCommit,
			"ALT_DIFF_SEED":        strconv.FormatInt(cfg.seed, 10),
			"ALT_DIFF_TMPDIR":      tmpDir,
			"ALT_DIFF_REPO_ROOT":   spec.repoResolved,
		},
	)
	durationS := math.Round(time.Since(start).Seconds()*10000) / 10000
	if execErr != nil && exitCode == 0 {
		exitCode = exitFail
		if len(runnerStderr) == 0 {
			runnerStderr = []byte(execErr.Error() + "\n")
		}
	}

	ev, err := recordEvidenceAndDecision(spec, cfg, resolvedCommit, runnerStdout, runnerStderr, exitCode, durationS)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitFail
	}
	if err := renderEvidence(stdout, ev, cfg.format); err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitFail
	}
	if !ev.Passed {
		return exitFail
	}
	return exitOK
}

func recordEvidenceAndDecision(
	spec resolvedRunSpec,
	cfg runFlags,
	resolvedCommit string,
	runnerStdout, runnerStderr []byte,
	exitCode int,
	durationS float64,
) (Evidence, error) {
	hasher := sha256.New()
	_, _ = hasher.Write(runnerStdout)
	_, _ = hasher.Write(runnerStderr)
	outputSHA := hex.EncodeToString(hasher.Sum(nil))

	evAbsPath := filepath.Join(spec.evidenceDir, spec.label+".json")
	evPointer := toRepoPointer(spec.repoResolved, evAbsPath)
	decisionsPointer := toRepoPointer(spec.repoResolved, spec.decisionsPath)

	ev := Evidence{
		BaseCommit:      resolvedCommit,
		BaseCommitInput: spec.baseInput,
		File:            spec.relFile,
		Label:           spec.label,
		Seed:            cfg.seed,
		RunnerCmd:       cfg.runnerCmd,
		ExitCode:        exitCode,
		Passed:          exitCode == 0,
		DurationS:       durationS,
		OutputSHA256:    outputSHA,
		StdoutExcerpt:   truncateExcerpt(string(runnerStdout), maxExcerptBytes),
		StderrExcerpt:   truncateExcerpt(string(runnerStderr), maxExcerptBytes),
		EvidencePath:    evPointer,
		DecisionsFile:   decisionsPointer,
		RecordedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	if err := writeEvidenceJSON(evAbsPath, ev); err != nil {
		return ev, fmt.Errorf("failed to write evidence JSON: %w", err)
	}

	resultStr := "mismatch"
	if ev.Passed {
		resultStr = "info"
	}
	shortBase := resolvedCommit
	if len(shortBase) > shortSHALen {
		shortBase = shortBase[:shortSHALen]
	}
	decisionText := sanitizeTSVValue(fmt.Sprintf("verify-diff %s (seed=%d, base=%s)", spec.relFile, cfg.seed, shortBase))
	whyText := sanitizeTSVValue(fmt.Sprintf("VERIFY-02 differential equivalence check against %s", spec.baseInput))
	if err := guards.AppendDecisionToFile(spec.decisionsPath, "verify-02", decisionText, whyText, evPointer, resultStr); err != nil {
		return ev, fmt.Errorf("failed to append decisions.tsv: %w", err)
	}
	return ev, nil
}

func executeRunner(
	ctx context.Context,
	repoRoot string,
	argv []string,
	extraEnv map[string]string,
) (stdoutBuf, stderrBuf []byte, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = repoRoot
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
	env := os.Environ()
	for k, v := range extraEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	var outB, errB bytes.Buffer
	cmd.Stdout = &outB
	cmd.Stderr = &errB

	runErr := cmd.Run()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return outB.Bytes(), errB.Bytes(), exitErr.ExitCode(), nil
		}
		return outB.Bytes(), errB.Bytes(), exitFail, runErr
	}
	return outB.Bytes(), errB.Bytes(), exitOK, nil
}

func resolveGitCommit(ctx context.Context, repoRoot, rev string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	cmd.Dir = repoRoot
	var outB, errB bytes.Buffer
	cmd.Stdout = &outB
	cmd.Stderr = &errB
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("could not resolve --base-commit %q: %s", rev, strings.TrimSpace(errB.String()))
	}
	sha := strings.TrimSpace(outB.String())
	if sha == "" {
		return "", fmt.Errorf("empty commit hash for %q", rev)
	}
	return sha, nil
}

func gitShowFile(ctx context.Context, repoRoot, commitSHA, relSlash string) ([]byte, error) {
	spec := commitSHA + ":" + relSlash
	cmd := exec.CommandContext(ctx, "git", "show", "--end-of-options", spec)
	cmd.Dir = repoRoot
	var outB, errB bytes.Buffer
	cmd.Stdout = &outB
	cmd.Stderr = &errB
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git show %s failed: %s", spec, strings.TrimSpace(errB.String()))
	}
	return outB.Bytes(), nil
}

func writeEvidenceJSON(path string, ev Evidence) error {
	if err := checkNoSymlinkAncestors(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), fsperm.Dir); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), fsperm.File)
}

func toRepoPointer(repoResolved, absPath string) string {
	if rel, err := filepath.Rel(repoResolved, absPath); err == nil {
		slash := filepath.ToSlash(rel)
		if slash != ".." && !strings.HasPrefix(slash, "../") {
			return slash
		}
	}
	return filepath.ToSlash(absPath)
}

func sanitizeTSVValue(s string) string {
	r := strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")
	return strings.TrimSpace(r.Replace(s))
}

func truncateExcerpt(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes] + "...(truncated)"
}

func renderEvidence(stdout io.Writer, ev Evidence, format string) error {
	if format == "json" {
		raw, err := json.MarshalIndent(ev, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, string(raw))
		return err
	}
	status := "FAIL"
	if ev.Passed {
		status = "PASS"
	}
	_, err := fmt.Fprintf(
		stdout,
		"VERIFY-02 %s file=%s base=%s seed=%d exit=%d duration=%.4fs sha256=%s evidence=%s\n",
		status,
		ev.File,
		ev.BaseCommit,
		ev.Seed,
		ev.ExitCode,
		ev.DurationS,
		ev.OutputSHA256,
		ev.EvidencePath,
	)
	return err
}

func runCheckClean(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	repoFlag := ""
	format := "text"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" || arg == "--help" {
			_, _ = fmt.Fprint(stdout, usageText())
			return exitOK
		}
		name, val, hasEq := splitFlag(arg)
		nextVal := func() (string, error) {
			if hasEq {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("flag %s requires a value", name)
			}
			i++
			return args[i], nil
		}
		switch name {
		case "--repo-root":
			v, err := nextVal()
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
				return exitUsage
			}
			repoFlag = v
		case "--format":
			v, err := nextVal()
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
				return exitUsage
			}
			if v != "text" && v != "json" {
				_, _ = fmt.Fprintf(stderr, "error: invalid --format %q\n", v)
				return exitUsage
			}
			format = v
		default:
			_, _ = fmt.Fprintf(stderr, "error: unrecognized argument %q\n", arg)
			return exitUsage
		}
	}

	repoRoot := guards.ResolveRepoRoot(repoFlag, "")
	repoResolved, err := resolveExistingDir(repoRoot)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: invalid --repo-root: %v\n", err)
		return exitUsage
	}

	tracked, err := gitTrackedFiles(ctx, repoResolved)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitFail
	}

	findings := make([]CleanFinding, 0)
	for _, rel := range tracked {
		if reason := checkScaffoldPath(rel); reason != "" {
			findings = append(findings, CleanFinding{Path: rel, Reason: reason})
			continue
		}
		full := filepath.Join(repoResolved, filepath.FromSlash(rel))
		if hasScaffoldMarker(full) {
			findings = append(findings, CleanFinding{
				Path:   rel,
				Reason: "contains temporary differential scaffold marker " + tempScaffoldMarker,
			})
		}
	}

	report := CleanReport{
		RepoRoot:     repoResolved,
		TrackedCount: len(tracked),
		Clean:        len(findings) == 0,
		Findings:     findings,
	}

	if format == "json" {
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
			return exitFail
		}
		_, _ = fmt.Fprintln(stdout, string(raw))
	} else if report.Clean {
		_, _ = fmt.Fprintf(stdout, "VERIFY-02 check-clean OK (%d tracked files scanned)\n", report.TrackedCount)
	} else {
		_, _ = fmt.Fprintf(stderr, "VERIFY-02 check-clean FAILED (%d temporary scaffold(s) tracked in git):\n", len(report.Findings))
		for _, f := range report.Findings {
			_, _ = fmt.Fprintf(stderr, "  - %s: %s\n", f.Path, f.Reason)
		}
	}

	if !report.Clean {
		return exitFail
	}
	return exitOK
}

func gitTrackedFiles(ctx context.Context, repoRoot string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z")
	cmd.Dir = repoRoot
	var outB, errB bytes.Buffer
	cmd.Stdout = &outB
	cmd.Stderr = &errB
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git ls-files failed: %s", strings.TrimSpace(errB.String()))
	}
	raw := outB.Bytes()
	var files []string
	for _, entry := range bytes.Split(raw, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		files = append(files, filepath.ToSlash(string(entry)))
	}
	return files, nil
}

func checkScaffoldPath(rel string) string {
	base := strings.ToLower(filepath.Base(rel))
	if strings.Contains(base, "_old_scaffold") {
		return "filename matches *_old_scaffold*"
	}
	if strings.Contains(base, ".verify02_old.") || strings.HasSuffix(base, ".verify02_old") {
		return "filename matches *.verify02_old.*"
	}
	return ""
}

func hasScaffoldMarker(absPath string) bool {
	st, err := os.Lstat(absPath)
	if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		return false
	}
	// Skip large files (> 2 MiB) to avoid scanning binary fixtures unnecessarily.
	if st.Size() > 2*1024*1024 {
		return false
	}
	f, err := os.Open(absPath)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == tempScaffoldMarker ||
			strings.HasPrefix(line, tempScaffoldMarker+" ") ||
			strings.HasPrefix(line, tempScaffoldMarker+":") {
			return true
		}
	}
	return false
}
