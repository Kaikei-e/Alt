package lang

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"alt/scripts/altharness/internal/hygiene"
	"alt/scripts/altharness/internal/packet"
	"alt/scripts/altharness/internal/pytestrand"
)

// RunHostFinal is scripts/verify/host-final-check.sh.
func RunHostFinal(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	opt, errText := parseHostArgs(args)
	if errText != "" {
		fmt.Fprint(stderr, errText)
		if !opt.help {
			fmt.Fprint(stderr, hostUsage)
		}
		return 2
	}
	if opt.help {
		fmt.Fprint(stderr, hostUsage)
		return 0
	}
	if len(opt.targets) == 0 {
		fmt.Fprintln(stderr, "error: at least one --target is required")
		fmt.Fprint(stderr, hostUsage)
		return 2
	}
	for _, target := range opt.targets {
		if target == "" || strings.HasPrefix(target, "-") || target == "all" {
			fmt.Fprintf(stderr, "error: invalid --target '%s': must be a concrete lint target (not flag '-*' or 'all')\n", target)
			return 2
		}
	}
	root := opt.repo
	if root == "" {
		found, rc := mustRepo(stderr)
		if rc != 0 {
			return rc
		}
		root = found
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "error: repo root not found: %s\n", root)
		return 2
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if opt.handoff != "" && opt.packet == "" {
		fmt.Fprintln(stderr, "error: --packet is required when --handoff is provided")
		return 2
	}
	runSh := filepath.Join(root, "scripts", "lint", "run.sh")
	if _, err := os.Stat(runSh); err != nil {
		fmt.Fprintf(stderr, "error: missing lint runner: %s\n", runSh)
		return 2
	}
	if opt.testCmd != "" {
		if hasBackgroundAmpersand(opt.testCmd) {
			fmt.Fprintln(stderr, "error: --test-cmd must run in foreground only (no & or nohup)")
			return 2
		}
		if bad := matchForbiddenAutofix(opt.testCmd); bad != "" {
			fmt.Fprintf(stderr, "error: --test-cmd contains forbidden GUARD-01 autofix (%s)\n", bad)
			return 2
		}
	}
	goDirs := make([]string, 0, len(opt.goTestDirs))
	for _, rawDir := range opt.goTestDirs {
		resolved, err := resolveRepoSubdir(root, rawDir, "--go-test-dir")
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		goDirs = append(goDirs, resolved)
	}
	pyDirs := make([]string, 0, len(opt.pyTestDirs))
	for _, rawDir := range opt.pyTestDirs {
		resolved, err := resolveRepoSubdir(root, rawDir, "--py-test-dir")
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		pyDirs = append(pyDirs, resolved)
	}
	policy := ""
	if opt.packet != "" {
		if _, err := os.Stat(opt.packet); err != nil {
			fmt.Fprintf(stderr, "error: packet file not found: %s\n", opt.packet)
			return 2
		}
		if doc, err := packet.LoadDocument(opt.packet); err == nil {
			if ep, ok := doc.Data["execution_policy"].(map[string]any); ok {
				if pol, ok := ep["policy"].(string); ok && (pol == "fix-existing" || pol == "block-new") {
					policy = pol
				}
			}
		}
	}
	fmt.Fprintln(stdout, "==> [host-final-check] 1/5 worktree hygiene")
	if code := hygiene.Run(ctx, []string{"--repo-root", root}, stdout, stderr); code != 0 {
		return code
	}
	fmt.Fprintln(stdout, "==> [host-final-check] 2/5 targeted lint")
	for _, target := range opt.targets {
		argv := []string{"bash", runSh}
		if policy != "" {
			fmt.Fprintf(stdout, "--> ./scripts/lint/run.sh --policy %s %s\n", policy, target)
			argv = append(argv, "--policy", policy, target)
		} else {
			fmt.Fprintf(stdout, "--> ./scripts/lint/run.sh %s\n", target)
			argv = append(argv, target)
		}
		if code, err := run(ctx, stdout, stderr, root, nil, argv...); err != nil || code != 0 {
			if err != nil {
				fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: bash")
				return 1
			}
			return code
		}
	}
	if !opt.skipRepo {
		fmt.Fprintln(stdout, "==> [host-final-check] 3/5 repo lint")
		if code, err := run(ctx, stdout, stderr, root, nil, "bash", runSh, "repo"); err != nil || code != 0 {
			if err != nil {
				fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: bash")
				return 1
			}
			return code
		}
	} else {
		fmt.Fprintln(stdout, "==> [host-final-check] 3/5 repo lint (skipped via --skip-repo-lint)")
	}
	hasRandTests := len(goDirs) > 0 || len(pyDirs) > 0
	if opt.testCmd != "" || hasRandTests {
		fmt.Fprintln(stdout, "==> [host-final-check] 4/5 host test command")
		if opt.testCmd != "" {
			if code, err := run(ctx, stdout, stderr, root, nil, "bash", "-c", opt.testCmd); err != nil || code != 0 {
				if err != nil {
					fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: bash")
					return 1
				}
				return code
			}
		}
		if hasRandTests {
			seed := opt.randomSeed
			if !opt.hasRandomSeed {
				seed = time.Now().UnixNano()
				if seed <= 0 {
					seed = 1
				}
			}
			seedStr := strconv.FormatInt(seed, 10)
			fmt.Fprintf(stdout, "random_seed=%s\n", seedStr)
			for _, dir := range goDirs {
				rel := dir
				if r, err := filepath.Rel(root, dir); err == nil {
					rel = filepath.ToSlash(r)
				}
				fmt.Fprintf(stdout, "--> go test -shuffle=%s -race -count=1 ./... (dir=%s, random_seed=%s)\n", seedStr, rel, seedStr)
				if code, err := run(ctx, stdout, stderr, dir, nil, "go", "test", "-shuffle="+seedStr, "-race", "-count=1", "./..."); err != nil || code != 0 {
					if err != nil {
						fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: go")
						return 1
					}
					return code
				}
			}
			for _, dir := range pyDirs {
				rel := dir
				if r, err := filepath.Rel(root, dir); err == nil {
					rel = filepath.ToSlash(r)
				}
				fmt.Fprintf(stdout, "--> pytest-rand --seed %s %s (random_seed=%s)\n", seedStr, rel, seedStr)
				if code := pytestrand.Run(ctx, []string{"--seed", seedStr, dir}, stdout, stderr); code != 0 {
					return code
				}
			}
		}
	} else {
		fmt.Fprintln(stdout, "==> [host-final-check] 4/5 host test command (not specified)")
	}
	if opt.handoff != "" {
		fmt.Fprintln(stdout, "==> [host-final-check] 5/5 LOOP-01 verify-handoff")
		argv := []string{"verify-handoff", opt.handoff, "--packet", opt.packet, "--repo-root", root}
		if opt.baseCommit != "" {
			argv = append(argv, "--base-commit", opt.baseCommit)
		}
		if opt.runsDir != "" {
			argv = append(argv, "--runs-dir", opt.runsDir)
		}
		if opt.noRerun {
			argv = append(argv, "--no-rerun")
		}
		if code := packet.Run(ctx, argv, stdout, stderr); code != 0 {
			return code
		}
	} else {
		fmt.Fprintln(stdout, "==> [host-final-check] 5/5 LOOP-01 verify-handoff (not specified)")
	}
	if !opt.allowDirty {
		fmt.Fprintln(stdout, "==> [host-final-check] final git status --porcelain check")
		out, errText, code, err := capture(ctx, root, nil, "git", "-C", root, "status", "--porcelain", "--", ":(exclude).harness")
		if err != nil || code != 0 {
			fmt.Fprintf(stderr, "error: git status failed: %s\n", strings.TrimSpace(errText))
			return 1
		}
		if strings.TrimSpace(out) != "" {
			fmt.Fprintln(stderr, "ERROR: dirty worktree detected after final checks:")
			fmt.Fprint(stderr, out)
			if !strings.HasSuffix(out, "\n") {
				fmt.Fprintln(stderr)
			}
			return 1
		}
	}
	fmt.Fprintf(stdout, "OK: host-final-check passed (%s)\n", strings.Join(opt.targets, " "))
	return 0
}

const hostUsage = `Usage: host-final-check.sh --target <lint-target> [options]

Options:
  --target <target>     Lint target for ./scripts/lint/run.sh <target> (repeatable)
  --repo-root <dir>     Repository root directory (default: auto-detected)
  --test-cmd <cmd>      Optional host test command to execute synchronously
  --go-test-dir <dir>   Run 'go test -shuffle=<seed> -race -count=1 ./...' in <dir> (repeatable)
  --py-test-dir <dir>   Run randomized pytest via pytest-rand in <dir> (repeatable)
  --random-seed <int64> Deterministic positive seed for --go-test-dir / --py-test-dir
  --handoff <path>      Optional TaskHandoff file to verify via LOOP-01 verify-handoff
  --packet <path>       TaskPacket file (required when --handoff is specified)
  --base-commit <rev>   Base git commit passed to verify-handoff
  --runs-dir <dir>      Runs directory for verify-handoff evidence JSON
  --no-rerun            Pass --no-rerun to verify-handoff
  --skip-repo-lint      Skip ./scripts/lint/run.sh repo (e.g. for fast unit tests)
  --allow-dirty         Do not fail on non-empty git status --porcelain
  -h, --help            Show this help message
`

type hostOpts struct {
	targets       []string
	repo          string
	testCmd       string
	goTestDirs    []string
	pyTestDirs    []string
	randomSeed    int64
	hasRandomSeed bool
	handoff       string
	packet        string
	baseCommit    string
	runsDir       string
	noRerun       bool
	skipRepo      bool
	allowDirty    bool
	help          bool
}

func parseHostArgs(args []string) (hostOpts, string) {
	var opt hostOpts
	take := func(i *int, name string) (string, string) {
		if *i+1 >= len(args) {
			return "", fmt.Sprintf("error: %s requires a value\n", name)
		}
		*i++
		return args[*i], ""
	}
	parseSeed := func(raw string) string {
		v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || v <= 0 {
			return fmt.Sprintf("error: invalid --random-seed '%s': must be a positive integer\n", raw)
		}
		opt.randomSeed = v
		opt.hasRandomSeed = true
		return ""
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--target":
			val, errText := take(&i, "--target")
			if errText != "" {
				return opt, errText
			}
			opt.targets = append(opt.targets, val)
		case arg == "--repo-root":
			val, errText := take(&i, "--repo-root")
			if errText != "" {
				return opt, errText
			}
			opt.repo = val
		case arg == "--test-cmd":
			val, errText := take(&i, "--test-cmd")
			if errText != "" {
				return opt, errText
			}
			opt.testCmd = val
		case arg == "--go-test-dir":
			val, errText := take(&i, "--go-test-dir")
			if errText != "" {
				return opt, errText
			}
			opt.goTestDirs = append(opt.goTestDirs, val)
		case strings.HasPrefix(arg, "--go-test-dir="):
			opt.goTestDirs = append(opt.goTestDirs, strings.TrimPrefix(arg, "--go-test-dir="))
		case arg == "--py-test-dir":
			val, errText := take(&i, "--py-test-dir")
			if errText != "" {
				return opt, errText
			}
			opt.pyTestDirs = append(opt.pyTestDirs, val)
		case strings.HasPrefix(arg, "--py-test-dir="):
			opt.pyTestDirs = append(opt.pyTestDirs, strings.TrimPrefix(arg, "--py-test-dir="))
		case arg == "--random-seed":
			val, errText := take(&i, "--random-seed")
			if errText != "" {
				return opt, errText
			}
			if errText := parseSeed(val); errText != "" {
				return opt, errText
			}
		case strings.HasPrefix(arg, "--random-seed="):
			if errText := parseSeed(strings.TrimPrefix(arg, "--random-seed=")); errText != "" {
				return opt, errText
			}
		case arg == "--handoff":
			val, errText := take(&i, "--handoff")
			if errText != "" {
				return opt, errText
			}
			opt.handoff = val
		case arg == "--packet":
			val, errText := take(&i, "--packet")
			if errText != "" {
				return opt, errText
			}
			opt.packet = val
		case arg == "--base-commit":
			val, errText := take(&i, "--base-commit")
			if errText != "" {
				return opt, errText
			}
			opt.baseCommit = val
		case arg == "--runs-dir":
			val, errText := take(&i, "--runs-dir")
			if errText != "" {
				return opt, errText
			}
			opt.runsDir = val
		case arg == "--no-rerun":
			opt.noRerun = true
		case arg == "--skip-repo-lint":
			opt.skipRepo = true
		case arg == "--allow-dirty":
			opt.allowDirty = true
		case arg == "-h" || arg == "--help":
			opt.help = true
			return opt, ""
		default:
			if strings.HasPrefix(arg, "-") {
				return opt, fmt.Sprintf("error: unknown option: %s\n", arg)
			}
			opt.targets = append(opt.targets, arg)
		}
	}
	return opt, ""
}

func resolveRepoSubdir(root, raw, flagName string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "-") || strings.ContainsRune(trimmed, '\x00') {
		return "", fmt.Errorf("invalid %s '%s': must be a directory inside repository root", flagName, raw)
	}
	for _, seg := range strings.Split(filepath.ToSlash(trimmed), "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid %s '%s': parent traversal '..' is not allowed", flagName, raw)
		}
	}
	rootResolved := root
	if r, err := filepath.EvalSymlinks(root); err == nil {
		rootResolved = r
	} else if abs, err := filepath.Abs(root); err == nil {
		rootResolved = abs
	}
	rootResolved = filepath.Clean(rootResolved)

	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootResolved, candidate)
	}
	candidate = filepath.Clean(candidate)

	if !pathWithinDir(candidate, rootResolved) {
		rootAbs, _ := filepath.Abs(root)
		if !pathWithinDir(candidate, filepath.Clean(rootAbs)) {
			return "", fmt.Errorf("invalid %s '%s': path escapes repository root %s", flagName, raw, root)
		}
	}
	info, err := os.Stat(candidate)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("invalid %s '%s': directory not found", flagName, raw)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("invalid %s '%s': %w", flagName, raw, err)
	}
	resolved = filepath.Clean(resolved)
	if !pathWithinDir(resolved, rootResolved) {
		return "", fmt.Errorf("invalid %s '%s': resolved path escapes repository root %s", flagName, raw, root)
	}
	return resolved, nil
}

func pathWithinDir(target, base string) bool {
	target = filepath.Clean(target)
	base = filepath.Clean(base)
	if target == base {
		return true
	}
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

var (
	nohupRe = regexp.MustCompile("(?:^|[\\s`('\"])nohup\\s+")
	autofix = []struct {
		label string
		re    *regexp.Regexp
	}{
		{"golangci-lint --fix", regexp.MustCompile(`\bgolangci-lint\b[^\n;|&]*--fix\b`)},
		{"cargo clippy --fix", regexp.MustCompile(`\bcargo\s+clippy\b[^\n;|&]*--fix\b`)},
		{"cargo fix", regexp.MustCompile(`\bcargo\s+fix\b`)},
		{"ruff check --unsafe-fixes", regexp.MustCompile(`\bruff\s+check\b[^\n;|&]*--unsafe-fixes\b`)},
		{"biome --unsafe", regexp.MustCompile(`\bbiome\s+(?:check|lint)\b[^\n;|&]*--unsafe\b`)},
	}
)

func hasBackgroundAmpersand(command string) bool {
	if nohupRe.MatchString(command) {
		return true
	}
	var quote byte
	escaped := false
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			escaped = false
			continue
		}
		if quote == '"' && ch == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '#' && (i == 0 || command[i-1] == ' ' || command[i-1] == '\t') {
			break
		}
		if ch == '&' {
			var prev byte
			if i > 0 {
				prev = command[i-1]
			}
			var next byte
			if i+1 < len(command) {
				next = command[i+1]
			}
			if prev == '>' || prev == '<' {
				continue
			}
			if next == '&' || next == '>' {
				i++
				continue
			}
			return true
		}
	}
	return false
}

func matchForbiddenAutofix(text string) string {
	for _, item := range autofix {
		if item.re.MatchString(text) {
			return item.label
		}
	}
	return ""
}
