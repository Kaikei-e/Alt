package corpus

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Run is the corpus CLI. args excludes the program name. Exit codes match corpus.py.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		fmt.Fprintln(stderr, "error: nil context")
		return ExitFail
	}
	opts, errText, code := parseArgs(args)
	if errText != "" {
		fmt.Fprint(stderr, errText)
		return code
	}
	if opts.help {
		fmt.Fprint(stdout, usageText())
		return ExitOK
	}
	if opts.minCases < 0 {
		fmt.Fprintln(stderr, "error: --min-cases must be >= 0")
		return ExitUsage
	}
	repoRoot := opts.repoRoot
	if repoRoot == "" {
		repoRoot = defaultRepoRoot()
	}
	repoRoot = resolvePath(repoRoot)
	corpusDir := opts.corpusDir
	if corpusDir == "" {
		corpusDir = filepath.Join(repoRoot, "scripts", "harness", "corpus")
	}
	cases, validationErrors := DiscoverCases(corpusDir, opts.minCases, opts.allowEmpty)
	for _, err := range validationErrors {
		fmt.Fprintf(stderr, "VALIDATION ERROR: %s\n", err)
	}
	timeout := time.Duration(SubprocessTimeoutSecond) * time.Second
	evaluations := evaluateCasesParallel(ctx, cases, repoRoot, timeout)
	if ctx.Err() != nil {
		fmt.Fprintf(stderr, "VALIDATION ERROR: context canceled: %v\n", ctx.Err())
		return ExitFail
	}
	for _, ev := range evaluations {
		fmt.Fprintf(stdout, "  %-13s  %s  [%s]  %s\n", strings.ToUpper(ev.Status), ev.CaseID, ev.Detail, ev.Title)
	}
	fmt.Fprintln(stdout, FormatCaptureRateLine(evaluations, len(validationErrors)))
	for _, ev := range evaluations {
		if ev.Status == "fail" {
			return ExitFail
		}
	}
	if len(validationErrors) > 0 {
		return ExitFail
	}
	return ExitOK
}

const maxCorpusWorkers = 8

func evaluateCasesParallel(ctx context.Context, cases []Case, repoRoot string, timeout time.Duration) []CaseEvaluation {
	if len(cases) == 0 {
		return nil
	}
	workers := runtime.NumCPU()
	if workers > maxCorpusWorkers {
		workers = maxCorpusWorkers
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(cases) {
		workers = len(cases)
	}
	evaluations := make([]CaseEvaluation, len(cases))
	jobs := make(chan int, len(cases))
	for i := range cases {
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if ctx.Err() != nil {
					return
				}
				evaluations[idx] = EvaluateCase(ctx, cases[idx], repoRoot, nil, timeout)
			}
		}()
	}
	wg.Wait()
	return evaluations
}

type cliOpts struct {
	corpusDir  string
	repoRoot   string
	minCases   int
	allowEmpty bool
	help       bool
}

func usageText() string {
	return "" +
		"usage: corpus [--corpus-dir PATH] [--repo-root PATH] [--min-cases N] [--allow-empty-corpus]\n" +
		"\n" +
		"Validate CORPUS-01 failure cases and report check capture rate.\n"
}

func parseArgs(args []string) (cliOpts, string, int) {
	opts := cliOpts{minCases: DefaultMinCases}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			opts.help = true
			return opts, "", ExitOK
		case arg == "--allow-empty-corpus":
			opts.allowEmpty = true
		case arg == "--corpus-dir" || arg == "--repo-root" || arg == "--min-cases":
			if i+1 >= len(args) {
				return opts, fmt.Sprintf("error: %s requires a value\n", arg), ExitUsage
			}
			i++
			val := args[i]
			switch arg {
			case "--corpus-dir":
				opts.corpusDir = val
			case "--repo-root":
				opts.repoRoot = val
			default:
				n, err := strconv.Atoi(val)
				if err != nil {
					return opts, fmt.Sprintf("error: argument --min-cases: invalid int value: %s\n", pyRepr(val)), ExitUsage
				}
				opts.minCases = n
			}
		case strings.HasPrefix(arg, "--corpus-dir="):
			opts.corpusDir = strings.TrimPrefix(arg, "--corpus-dir=")
		case strings.HasPrefix(arg, "--repo-root="):
			opts.repoRoot = strings.TrimPrefix(arg, "--repo-root=")
		case strings.HasPrefix(arg, "--min-cases="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--min-cases="))
			if err != nil {
				return opts, fmt.Sprintf("error: argument --min-cases: invalid int value: %s\n", pyRepr(strings.TrimPrefix(arg, "--min-cases="))), ExitUsage
			}
			opts.minCases = n
		default:
			return opts, fmt.Sprintf("error: unrecognized arguments: %s\n", arg), ExitUsage
		}
	}
	return opts, "", ExitOK
}

func defaultRepoRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}

func resolvePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return filepath.Clean(abs)
}
