package lang

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"alt/scripts/altharness/internal/hygiene"
	"alt/scripts/altharness/internal/packet"
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
	defaultRoot, _ := repoRoot()
	hygieneScript := filepath.Join(root, "scripts", "lint", "check_worktree_hygiene.py")
	if _, err := os.Stat(hygieneScript); err != nil && defaultRoot != "" {
		hygieneScript = filepath.Join(defaultRoot, "scripts", "lint", "check_worktree_hygiene.py")
	}
	taskPacket := filepath.Join(root, "scripts", "agent-harness", "task_packet.py")
	if _, err := os.Stat(taskPacket); err != nil && defaultRoot != "" {
		taskPacket = filepath.Join(defaultRoot, "scripts", "agent-harness", "task_packet.py")
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
	if _, err := os.Stat(hygieneScript); err == nil {
		if code, err := run(ctx, stdout, stderr, root, nil, "python3", hygieneScript, "--repo-root", root); err != nil || code != 0 {
			if err != nil {
				fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: python3")
				return 1
			}
			return code
		}
	} else if code := hygiene.Run(ctx, []string{"--repo-root", root}, stdout, stderr); code != 0 {
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
	if opt.testCmd != "" {
		fmt.Fprintln(stdout, "==> [host-final-check] 4/5 host test command")
		if code, err := run(ctx, stdout, stderr, root, nil, "bash", "-c", opt.testCmd); err != nil || code != 0 {
			if err != nil {
				fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: bash")
				return 1
			}
			return code
		}
	} else {
		fmt.Fprintln(stdout, "==> [host-final-check] 4/5 host test command (not specified)")
	}
	if opt.handoff != "" {
		fmt.Fprintln(stdout, "==> [host-final-check] 5/5 LOOP-01 verify-handoff")
		argv := []string{"python3", taskPacket, "verify-handoff", opt.handoff, "--packet", opt.packet, "--repo-root", root}
		if opt.baseCommit != "" {
			argv = append(argv, "--base-commit", opt.baseCommit)
		}
		if opt.runsDir != "" {
			argv = append(argv, "--runs-dir", opt.runsDir)
		}
		if opt.noRerun {
			argv = append(argv, "--no-rerun")
		}
		if code, err := run(ctx, stdout, stderr, root, nil, argv...); err != nil || code != 0 {
			if err != nil {
				fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: python3")
				return 1
			}
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
	targets    []string
	repo       string
	testCmd    string
	handoff    string
	packet     string
	baseCommit string
	runsDir    string
	noRerun    bool
	skipRepo   bool
	allowDirty bool
	help       bool
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
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--target":
			val, errText := take(&i, "--target")
			if errText != "" {
				return opt, errText
			}
			opt.targets = append(opt.targets, val)
		case "--repo-root":
			val, errText := take(&i, "--repo-root")
			if errText != "" {
				return opt, errText
			}
			opt.repo = val
		case "--test-cmd":
			val, errText := take(&i, "--test-cmd")
			if errText != "" {
				return opt, errText
			}
			opt.testCmd = val
		case "--handoff":
			val, errText := take(&i, "--handoff")
			if errText != "" {
				return opt, errText
			}
			opt.handoff = val
		case "--packet":
			val, errText := take(&i, "--packet")
			if errText != "" {
				return opt, errText
			}
			opt.packet = val
		case "--base-commit":
			val, errText := take(&i, "--base-commit")
			if errText != "" {
				return opt, errText
			}
			opt.baseCommit = val
		case "--runs-dir":
			val, errText := take(&i, "--runs-dir")
			if errText != "" {
				return opt, errText
			}
			opt.runsDir = val
		case "--no-rerun":
			opt.noRerun = true
		case "--skip-repo-lint":
			opt.skipRepo = true
		case "--allow-dirty":
			opt.allowDirty = true
		case "-h", "--help":
			opt.help = true
			return opt, ""
		default:
			if strings.HasPrefix(args[i], "-") {
				return opt, fmt.Sprintf("error: unknown option: %s\n", args[i])
			}
			opt.targets = append(opt.targets, args[i])
		}
	}
	return opt, ""
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
