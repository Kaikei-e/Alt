// Package flakyhunt implements `alt-harness flaky-hunt` (VERIFY-05), a pure-Go
// flaky test hunter for Go and Python test suites with seed capture, failure
// extraction, deterministic repro command generation, and structured evidence.
package flakyhunt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"alt/scripts/altharness/internal/fsperm"
)

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2

	schemaVersion    = "verify-05/v1"
	defaultRounds    = 5
	defaultCount     = 1
	defaultGoPkg     = "./..."
	defaultPytestCmd = "uv run pytest"
)

// Options configures a flaky-hunt run without mutating process globals.
type Options struct {
	Stdout io.Writer
	Stderr io.Writer
	Env    map[string]string
}

// Report is the structured JSON evidence emitted by `alt-harness flaky-hunt`.
type Report struct {
	SchemaVersion string         `json:"schema_version"`
	Module        string         `json:"module"`
	Lang          string         `json:"lang"`
	Package       string         `json:"package"`
	Rounds        int            `json:"rounds"`
	Count         int            `json:"count"`
	Race          bool           `json:"race"`
	StressProcs   int            `json:"stress_procs,omitempty"`
	RequestedSeed string         `json:"requested_seed,omitempty"`
	StartedAt     string         `json:"started_at"`
	FinishedAt    string         `json:"finished_at"`
	DurationMS    int64          `json:"duration_ms"`
	PassedRounds  int            `json:"passed_rounds"`
	FailedRounds  int            `json:"failed_rounds"`
	Status        string         `json:"status"`
	EvidenceDir   string         `json:"evidence_dir"`
	RoundResults  []RoundSummary `json:"round_results"`
	Failures      []Failure      `json:"failures"`
}

// RoundSummary records the outcome and captured shuffle seed(s) for one round.
type RoundSummary struct {
	Round        int               `json:"round"`
	ExitCode     int               `json:"exit_code"`
	Passed       bool              `json:"passed"`
	DurationMS   int64             `json:"duration_ms"`
	Seed         string            `json:"seed,omitempty"`
	PackageSeeds map[string]string `json:"package_seeds,omitempty"`
	LogPath      string            `json:"log_path,omitempty"`
}

// Failure describes one failing test, data race, panic, timeout, or build error.
type Failure struct {
	Round        int    `json:"round"`
	Seed         string `json:"seed"`
	TestName     string `json:"test_name"`
	Package      string `json:"package"`
	Kind         string `json:"kind"`
	ReproCommand string `json:"repro_command"`
	LogPath      string `json:"log_path,omitempty"`
	Snippet      string `json:"snippet,omitempty"`
}

type config struct {
	module      string
	lang        string
	rounds      int
	count       int
	seed        string
	pkgs        []string
	pkgExplicit bool
	race        bool
	stressProcs int
	evidenceDir string
	repoRoot    string
	format      string
	pytestCmd   string
	extraArgs   []string
	showHelp    bool
}

const usageText = `Usage: alt-harness flaky-hunt <module> [options] [-- <extra-args>...]

Options:
  --lang auto|go|python    Test language (default: auto)
  --rounds N               Number of randomized rounds (default: 5)
  --count N                Test count per round (default: 1)
  --seed SEED              Explicit shuffle seed (integer) or "on" for random
  --pkg PKG                Go package pattern or Python test path (repeatable, default: ./...)
  --race[=true|false]      Enable Go race detector (default: true)
  --stress-procs N         Set GOMAXPROCS=N during Go runs for contention testing
  --evidence-dir DIR       Output directory for report.json and failure logs
  --repo-root DIR          Repository root (default: auto-detected)
  --format text|json       Output format (default: text)
  --pytest-cmd CMD         Pytest command for Python mode (default: "uv run pytest")
  -h, --help               Show this help message
`

// Run executes `alt-harness flaky-hunt` using the current process environment.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return RunWithOptions(ctx, args, Options{
		Stdout: stdout,
		Stderr: stderr,
	})
}

// RunWithOptions executes `alt-harness flaky-hunt` with explicit streams and environment.
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

	cfg, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n%s", err, usageText)
		return exitUsage
	}
	if cfg.showHelp {
		fmt.Fprint(stdout, usageText)
		return exitOK
	}

	repoReal, err := resolveRepoRoot(cfg.repoRoot)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return exitUsage
	}

	relModule, modReal, err := validateModulePath(repoReal, cfg.module)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return exitUsage
	}

	lang, err := resolveLanguage(modReal, cfg.lang)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return exitUsage
	}

	evidenceDir, err := resolveEvidenceDir(repoReal, relModule, cfg.evidenceDir)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return exitUsage
	}
	if err := os.MkdirAll(evidenceDir, fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "ERROR: failed to create evidence directory %s: %v\n", evidenceDir, err)
		return exitFail
	}

	env := buildEnvMap(opts.Env)
	started := time.Now().UTC()

	pkgDisplay := defaultGoPkg
	if len(cfg.pkgs) > 0 {
		pkgDisplay = strings.Join(cfg.pkgs, " ")
	} else if lang == "python" {
		pkgDisplay = "."
	}

	rep := Report{
		SchemaVersion: schemaVersion,
		Module:        relModule,
		Lang:          lang,
		Package:       pkgDisplay,
		Rounds:        cfg.rounds,
		Count:         cfg.count,
		Race:          lang == "go" && cfg.race,
		StressProcs:   cfg.stressProcs,
		RequestedSeed: cfg.seed,
		StartedAt:     started.Format(time.RFC3339Nano),
		EvidenceDir:   evidenceDir,
		RoundResults:  make([]RoundSummary, 0, cfg.rounds),
		Failures:      make([]Failure, 0),
	}

	var runErr error
	switch lang {
	case "go":
		runErr = runGoHunt(ctx, cfg, relModule, modReal, evidenceDir, env, &rep)
	case "python":
		runErr = runPythonHunt(ctx, cfg, relModule, modReal, evidenceDir, env, &rep)
	default:
		fmt.Fprintf(stderr, "ERROR: unsupported language %q\n", lang)
		return exitUsage
	}

	finished := time.Now().UTC()
	rep.FinishedAt = finished.Format(time.RFC3339Nano)
	rep.DurationMS = finished.Sub(started).Milliseconds()
	if rep.FailedRounds == 0 && len(rep.Failures) == 0 && runErr == nil {
		rep.Status = "PASS"
	} else {
		rep.Status = "FAIL"
	}

	if writeErr := writeReportJSON(evidenceDir, &rep); writeErr != nil {
		fmt.Fprintf(stderr, "ERROR: failed to write evidence report: %v\n", writeErr)
		return exitFail
	}

	if runErr != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", runErr)
	}

	if err := emitOutput(stdout, cfg.format, &rep); err != nil {
		fmt.Fprintf(stderr, "ERROR: failed to render report: %v\n", err)
		return exitFail
	}

	if rep.Status != "PASS" {
		return exitFail
	}
	return exitOK
}

func parseArgs(args []string) (config, error) {
	cfg := config{
		lang:      "auto",
		rounds:    defaultRounds,
		count:     defaultCount,
		race:      true,
		format:    "text",
		pytestCmd: defaultPytestCmd,
	}

	runnerArgs := args
	for idx, a := range args {
		if a == "--" {
			runnerArgs = args[:idx]
			cfg.extraArgs = args[idx+1:]
			break
		}
	}

	take := func(i *int, flag string) (string, error) {
		if *i+1 >= len(runnerArgs) {
			return "", fmt.Errorf("%s requires a value", flag)
		}
		*i++
		return runnerArgs[*i], nil
	}

	var positionals []string
	for i := 0; i < len(runnerArgs); i++ {
		arg := runnerArgs[i]
		switch {
		case arg == "-h" || arg == "--help":
			cfg.showHelp = true
			return cfg, nil
		case arg == "--lang":
			v, err := take(&i, "--lang")
			if err != nil {
				return cfg, err
			}
			cfg.lang = v
		case strings.HasPrefix(arg, "--lang="):
			cfg.lang = strings.TrimPrefix(arg, "--lang=")
		case arg == "--rounds":
			v, err := take(&i, "--rounds")
			if err != nil {
				return cfg, err
			}
			n, err := parsePositiveInt("--rounds", v)
			if err != nil {
				return cfg, err
			}
			cfg.rounds = n
		case strings.HasPrefix(arg, "--rounds="):
			n, err := parsePositiveInt("--rounds", strings.TrimPrefix(arg, "--rounds="))
			if err != nil {
				return cfg, err
			}
			cfg.rounds = n
		case arg == "--count":
			v, err := take(&i, "--count")
			if err != nil {
				return cfg, err
			}
			n, err := parsePositiveInt("--count", v)
			if err != nil {
				return cfg, err
			}
			cfg.count = n
		case strings.HasPrefix(arg, "--count="):
			n, err := parsePositiveInt("--count", strings.TrimPrefix(arg, "--count="))
			if err != nil {
				return cfg, err
			}
			cfg.count = n
		case arg == "--seed":
			v, err := take(&i, "--seed")
			if err != nil {
				return cfg, err
			}
			cfg.seed = v
		case strings.HasPrefix(arg, "--seed="):
			cfg.seed = strings.TrimPrefix(arg, "--seed=")
		case arg == "--pkg":
			v, err := take(&i, "--pkg")
			if err != nil {
				return cfg, err
			}
			cfg.pkgs = append(cfg.pkgs, v)
			cfg.pkgExplicit = true
		case strings.HasPrefix(arg, "--pkg="):
			cfg.pkgs = append(cfg.pkgs, strings.TrimPrefix(arg, "--pkg="))
			cfg.pkgExplicit = true
		case arg == "--race":
			cfg.race = true
		case arg == "--no-race":
			cfg.race = false
		case strings.HasPrefix(arg, "--race="):
			raw := strings.TrimPrefix(arg, "--race=")
			b, err := strconv.ParseBool(raw)
			if err != nil {
				return cfg, fmt.Errorf("invalid --race %q: must be true or false", raw)
			}
			cfg.race = b
		case arg == "--stress-procs":
			v, err := take(&i, "--stress-procs")
			if err != nil {
				return cfg, err
			}
			n, err := parsePositiveInt("--stress-procs", v)
			if err != nil {
				return cfg, err
			}
			cfg.stressProcs = n
		case strings.HasPrefix(arg, "--stress-procs="):
			n, err := parsePositiveInt("--stress-procs", strings.TrimPrefix(arg, "--stress-procs="))
			if err != nil {
				return cfg, err
			}
			cfg.stressProcs = n
		case arg == "--evidence-dir":
			v, err := take(&i, "--evidence-dir")
			if err != nil {
				return cfg, err
			}
			cfg.evidenceDir = v
		case strings.HasPrefix(arg, "--evidence-dir="):
			cfg.evidenceDir = strings.TrimPrefix(arg, "--evidence-dir=")
		case arg == "--repo-root":
			v, err := take(&i, "--repo-root")
			if err != nil {
				return cfg, err
			}
			cfg.repoRoot = v
		case strings.HasPrefix(arg, "--repo-root="):
			cfg.repoRoot = strings.TrimPrefix(arg, "--repo-root=")
		case arg == "--format":
			v, err := take(&i, "--format")
			if err != nil {
				return cfg, err
			}
			cfg.format = v
		case strings.HasPrefix(arg, "--format="):
			cfg.format = strings.TrimPrefix(arg, "--format=")
		case arg == "--pytest-cmd":
			v, err := take(&i, "--pytest-cmd")
			if err != nil {
				return cfg, err
			}
			cfg.pytestCmd = v
		case strings.HasPrefix(arg, "--pytest-cmd="):
			cfg.pytestCmd = strings.TrimPrefix(arg, "--pytest-cmd=")
		case strings.HasPrefix(arg, "-"):
			return cfg, fmt.Errorf("unknown flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}

	if len(positionals) != 1 {
		return cfg, fmt.Errorf("expected exactly 1 <module> argument, got %d", len(positionals))
	}
	cfg.module = positionals[0]

	switch cfg.lang {
	case "auto", "go", "python":
	default:
		return cfg, fmt.Errorf("invalid --lang %q: must be auto, go, or python", cfg.lang)
	}
	switch cfg.format {
	case "text", "json":
	default:
		return cfg, fmt.Errorf("invalid --format %q: must be text or json", cfg.format)
	}
	if cfg.seed != "" && cfg.seed != "on" {
		if _, err := strconv.ParseInt(cfg.seed, 10, 64); err != nil {
			return cfg, fmt.Errorf("invalid --seed %q: must be an integer or \"on\"", cfg.seed)
		}
	}
	if strings.TrimSpace(cfg.pytestCmd) == "" {
		return cfg, errors.New("--pytest-cmd must not be empty")
	}

	return cfg, nil
}

func parsePositiveInt(flag, raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid %s %q: must be an integer >= 1", flag, raw)
	}
	return n, nil
}

func resolveRepoRoot(explicit string) (string, error) {
	if explicit != "" {
		if strings.ContainsRune(explicit, '\x00') {
			return "", errors.New("invalid --repo-root: contains NUL byte")
		}
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return "", fmt.Errorf("failed to resolve --repo-root %q: %w", explicit, err)
		}
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			return "", fmt.Errorf("--repo-root does not exist or is not a directory: %s", abs)
		}
		realPath, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("failed to canonicalize --repo-root %q: %w", abs, err)
		}
		return realPath, nil
	}

	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		dir := start
		for {
			if hasRepoMarker(dir) {
				if realPath, err := filepath.EvalSymlinks(dir); err == nil {
					return realPath, nil
				}
				return filepath.Abs(dir)
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", errors.New("repository root not found; pass --repo-root")
}

func hasRepoMarker(dir string) bool {
	return isRegularFile(filepath.Join(dir, "scripts", "altharness", "alt-harness")) ||
		isRegularFile(filepath.Join(dir, "scripts", "lint", "run.sh"))
}

func isRegularFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// validateModulePath enforces strict repo-root containment (CWE-22 / CWE-59):
// rejects absolute paths, NUL bytes, ".." path traversal segments, and any
// symlink resolving outside repoReal.
func validateModulePath(repoReal, rawModule string) (string, string, error) {
	trimmed := strings.TrimSpace(rawModule)
	if trimmed == "" || strings.ContainsRune(trimmed, '\x00') {
		return "", "", errors.New("invalid module path: must be a non-empty relative path")
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "\\") {
		return "", "", fmt.Errorf("module path must be relative to repository root, got %q", rawModule)
	}
	slashPath := strings.ReplaceAll(trimmed, "\\", "/")
	for _, seg := range strings.Split(slashPath, "/") {
		if seg == ".." {
			return "", "", fmt.Errorf("module path traversal is not allowed: %q", rawModule)
		}
	}

	cleaned := filepath.Clean(filepath.FromSlash(slashPath))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("invalid module path %q: must name a subdirectory inside repository root", rawModule)
	}

	joined := filepath.Join(repoReal, cleaned)
	if _, lerr := os.Lstat(joined); lerr != nil {
		if errors.Is(lerr, os.ErrNotExist) {
			return "", "", fmt.Errorf("module directory does not exist: %s", filepath.ToSlash(cleaned))
		}
		return "", "", fmt.Errorf("cannot inspect module directory %q: %w", rawModule, lerr)
	}

	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve module directory %q (broken symlink or escape): %w", rawModule, err)
	}
	if !isPathInside(resolved, repoReal) {
		return "", "", fmt.Errorf("module symlink escapes repository root: %q -> %q", rawModule, resolved)
	}

	st, err := os.Stat(resolved)
	if err != nil || !st.IsDir() {
		return "", "", fmt.Errorf("module path is not a directory: %s", filepath.ToSlash(cleaned))
	}

	relCanon, err := filepath.Rel(repoReal, resolved)
	if err != nil || !isRelativeInside(relCanon) {
		return "", "", fmt.Errorf("module path escapes repository root: %q", rawModule)
	}
	return filepath.ToSlash(cleaned), resolved, nil
}

func resolveEvidenceDir(repoReal, relModule, explicit string) (string, error) {
	if explicit == "" {
		return filepath.Join(repoReal, ".harness", "evidence", "flaky-hunt", safeModuleName(relModule)), nil
	}
	if strings.ContainsRune(explicit, '\x00') {
		return "", errors.New("invalid --evidence-dir: contains NUL byte")
	}
	slashPath := strings.ReplaceAll(explicit, "\\", "/")
	for _, seg := range strings.Split(slashPath, "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid --evidence-dir %q: path traversal is not allowed", explicit)
		}
	}
	if filepath.IsAbs(explicit) {
		return filepath.Clean(explicit), nil
	}
	joined := filepath.Join(repoReal, filepath.FromSlash(slashPath))
	resolved, err := resolveNearestAncestor(joined)
	if err != nil {
		return "", fmt.Errorf("invalid --evidence-dir %q: %w", explicit, err)
	}
	if !isPathInside(resolved, repoReal) {
		return "", fmt.Errorf("--evidence-dir symlink escapes repository root: %q", explicit)
	}
	return joined, nil
}

func resolveNearestAncestor(target string) (string, error) {
	cur := filepath.Clean(target)
	var suffix []string
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("cannot resolve %q", target)
		}
		suffix = append(suffix, filepath.Base(cur))
		cur = parent
	}
}

func isPathInside(target, base string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(target))
	if err != nil {
		return false
	}
	return isRelativeInside(rel)
}

func isRelativeInside(rel string) bool {
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}

func safeModuleName(relModule string) string {
	s := strings.ReplaceAll(filepath.ToSlash(relModule), "/", "__")
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '_' || ch == '-' {
			b.WriteByte(ch)
		} else {
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		return "module"
	}
	return out
}

func resolveLanguage(modDir, requested string) (string, error) {
	if requested != "auto" {
		return requested, nil
	}
	if isRegularFile(filepath.Join(modDir, "go.mod")) {
		return "go", nil
	}
	for _, marker := range []string{"pyproject.toml", "pytest.ini", "setup.cfg"} {
		if isRegularFile(filepath.Join(modDir, marker)) {
			return "python", nil
		}
	}
	return "", fmt.Errorf("could not auto-detect language in %s (missing go.mod or pyproject.toml); specify --lang go|python", modDir)
}

func buildEnvMap(override map[string]string) map[string]string {
	env := make(map[string]string)
	if override != nil {
		for k, v := range override {
			env[k] = v
		}
		return env
	}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return env
}

func writeReportJSON(evidenceDir string, rep *Report) error {
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(filepath.Join(evidenceDir, "report.json"), raw, fsperm.File)
}

func writeFailureLog(evidenceDir string, round int, content string) (string, error) {
	name := fmt.Sprintf("round-%02d-failure.log", round)
	path := filepath.Join(evidenceDir, name)
	if err := os.WriteFile(path, []byte(content), fsperm.File); err != nil {
		return "", err
	}
	return path, nil
}

func emitOutput(w io.Writer, format string, rep *Report) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	fmt.Fprintf(w, "FLAKY-HUNT [%s] module=%s lang=%s rounds=%d passed=%d failed=%d failures=%d duration_ms=%d\n",
		rep.Status, rep.Module, rep.Lang, rep.Rounds, rep.PassedRounds, rep.FailedRounds, len(rep.Failures), rep.DurationMS)
	fmt.Fprintf(w, "evidence_dir: %s\n", rep.EvidenceDir)
	for _, f := range rep.Failures {
		fmt.Fprintf(w, "  - round=%d kind=%s pkg=%s test=%s seed=%s\n",
			f.Round, f.Kind, f.Package, f.TestName, f.Seed)
		fmt.Fprintf(w, "    repro: %s\n", f.ReproCommand)
		if f.LogPath != "" {
			fmt.Fprintf(w, "    log:   %s\n", f.LogPath)
		}
	}
	return nil
}
