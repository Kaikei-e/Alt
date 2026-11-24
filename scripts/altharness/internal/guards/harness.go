package guards

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"alt/scripts/altharness/internal/execx"
)

const (
	harnessExitOK    = 0
	harnessExitError = 1
	harnessExitUsage = 2

	parallelismBaseDefault = 2
	parallelismWindow      = 10
	parallelismMax         = 8
	parallelismMin         = 1
	parallelismStep        = 2
	scaleUpMaxReverted     = 1
	scaleDownMinReverted   = 3
	scaleDownMinMismatch   = 2
	maxAssistantMsgChars   = 4000
	detachedSHALen         = 12
	activePacketFilename   = "active-packet.json"
	decisionsFilename      = "decisions.tsv"
	subagentsFilename      = "subagents.jsonl"
)

var (
	tsvColumns          = []string{"ts", "phase", "decision", "why", "evidence", "result"}
	terminalResults     = map[string]struct{}{"adopted": {}, "reverted": {}, "mismatch": {}, "cancelled": {}}
	validResults        = map[string]struct{}{"adopted": {}, "reverted": {}, "mismatch": {}, "cancelled": {}, "info": {}}
	validPolicies       = map[string]struct{}{"fix-existing": {}, "block-new": {}}
	placeholderEvidence = map[string]struct{}{
		"": {}, "tbd": {}, "n/a": {}, "na": {}, "-": {}, "--": {}, "—": {}, "todo": {}, "none": {},
		"null": {}, "done": {}, "ok": {}, "なし": {}, "無し": {}, "未記入": {}, "未定": {}, "xxx": {}, "...": {},
	}
	pointerRE  = regexp.MustCompile(`(?i)^(?:#\d+|(?:ADR-\d{6}|PM-\d{4}-\d{3}|C-\d{2,})(?:\b|$)|commit:[0-9a-fA-F]{4,40}|cmd:\S.*|https?://\S+|file://\S+|[^\s]+/[^\s]+|[^\s]+\.(?:py|go|rs|ts|tsx|js|jsx|svelte|sh|md|json|jsonl|tsv|toml|yaml|yml|txt|log|sql|proto))$`)
	packetIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$`)
)

// SanitizeBranchName collapses a git branch into one safe directory component.
func SanitizeBranchName(branch string) string {
	cleaned := regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(strings.TrimSpace(branch), "-")
	cleaned = regexp.MustCompile(`\.{2,}`).ReplaceAllString(cleaned, "-")
	cleaned = regexp.MustCompile(`-{2,}`).ReplaceAllString(cleaned, "-")
	cleaned = strings.Trim(cleaned, ".-")
	if cleaned == "" {
		return "default"
	}
	return cleaned
}

// ResolveRepoRoot finds the repository root without spawning a subprocess.
func ResolveRepoRoot(override, startDir string) string {
	if strings.TrimSpace(override) != "" {
		return resolveExisting(override)
	}
	if env := strings.TrimSpace(os.Getenv("ALT_HARNESS_REPO_ROOT")); env != "" {
		return resolveExisting(env)
	}
	probe := startDir
	if probe == "" {
		if cwd, err := os.Getwd(); err == nil {
			probe = cwd
		}
	}
	probe = resolveExisting(probe)
	dir := probe
	for dir != "" {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if claude := strings.TrimSpace(os.Getenv("CLAUDE_PROJECT_DIR")); claude != "" {
		p := resolveExisting(claude)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return probe
}

func resolveExisting(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return filepath.Clean(abs)
}

func readGitHead(repoRoot string) string {
	dot := filepath.Join(repoRoot, ".git")
	info, err := os.Lstat(dot)
	if err != nil {
		return ""
	}
	headFile := filepath.Join(dot, "HEAD")
	if !info.IsDir() {
		raw, err := os.ReadFile(dot)
		if err != nil {
			return ""
		}
		line := strings.TrimSpace(strings.Split(string(raw), "\n")[0])
		if !strings.HasPrefix(line, "gitdir:") {
			return ""
		}
		gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
		if !filepath.IsAbs(gitDir) {
			gitDir = resolveExisting(filepath.Join(repoRoot, gitDir))
		}
		headFile = filepath.Join(gitDir, "HEAD")
	}
	raw, err := os.ReadFile(headFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// ResolveGitBranch reads the current branch from .git/HEAD.
func ResolveGitBranch(repoRoot, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if env := strings.TrimSpace(os.Getenv("ALT_HARNESS_BRANCH")); env != "" {
		return env
	}
	head := readGitHead(repoRoot)
	if head == "" {
		return "default"
	}
	if strings.HasPrefix(head, "ref: refs/heads/") {
		name := strings.TrimSpace(strings.TrimPrefix(head, "ref: refs/heads/"))
		if name == "" {
			return "default"
		}
		return name
	}
	if strings.HasPrefix(head, "ref: ") {
		name := strings.TrimSpace(strings.TrimPrefix(head, "ref: "))
		if name == "" {
			return "default"
		}
		return name
	}
	if len(head) > detachedSHALen {
		head = head[:detachedSHALen]
	}
	return "detached-" + head
}

// RunDirForBranch returns <repo>/.harness/runs/<sanitized-branch>.
func RunDirForBranch(repoRoot, branch string) (string, error) {
	runsBase := resolveExisting(filepath.Join(repoRoot, ".harness", "runs"))
	safe := SanitizeBranchName(ResolveGitBranch(repoRoot, branch))
	target := resolveExisting(filepath.Join(runsBase, safe))
	if !pathWithin(target, runsBase) && target != runsBase {
		// target may not exist yet; compare lexically
		rel, err := filepath.Rel(runsBase, filepath.Join(filepath.Join(repoRoot, ".harness", "runs"), safe))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return "", fmt.Errorf("resolved run directory escapes .harness/runs: %s", target)
		}
	}
	return filepath.Join(repoRoot, ".harness", "runs", safe), nil
}

// LoadActivePacket loads active-packet.json when active=true.
// A malformed file or symlink returns an error. A missing file returns (nil, "").
func LoadActivePacket(repoRoot, branch string) (map[string]any, string) {
	var pktPath string
	if env := strings.TrimSpace(os.Getenv("ALT_ACTIVE_PACKET")); env != "" {
		pktPath = env
	} else {
		runDir, err := RunDirForBranch(repoRoot, branch)
		if err != nil {
			return nil, err.Error()
		}
		pktPath = filepath.Join(runDir, activePacketFilename)
	}
	info, err := os.Lstat(pktPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ""
		}
		return nil, fmt.Sprintf("could not resolve active packet path: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Sprintf("active packet file must not be a symlink: %s", pktPath)
	}
	if strings.TrimSpace(os.Getenv("ALT_ACTIVE_PACKET")) == "" {
		resolved, err := filepath.EvalSymlinks(pktPath)
		if err != nil {
			return nil, fmt.Sprintf("could not resolve active packet path: %v", err)
		}
		repoResolved := resolveExisting(repoRoot)
		if !pathWithin(resolved, repoResolved) && resolved != repoResolved {
			return nil, fmt.Sprintf("active packet file escapes repository root: %s", pktPath)
		}
	}
	raw, err := os.ReadFile(pktPath)
	if err != nil {
		return nil, fmt.Sprintf("malformed active packet JSON (%s): %v", pktPath, err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Sprintf("malformed active packet JSON (%s): %v", pktPath, err)
	}
	if data["active"] != true {
		return nil, ""
	}
	return data, ""
}

func validateTSVField(name, value string) (string, error) {
	if strings.ContainsAny(value, "\t\n\r") {
		return "", fmt.Errorf("field '%s' must not contain tabs or newlines", name)
	}
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return "", fmt.Errorf("field '%s' must not be empty", name)
	}
	return cleaned, nil
}

// IsValidEvidencePointer reports whether evidence is a concrete pointer.
func IsValidEvidencePointer(evidence string) bool {
	cleaned := strings.TrimSpace(evidence)
	if _, ok := placeholderEvidence[strings.ToLower(cleaned)]; ok {
		return false
	}
	return pointerRE.MatchString(cleaned)
}

// Parallelism is the LOOP-06 recommendation.
type Parallelism struct {
	Base        int
	Delta       int
	Recommended int
	WindowSize  int
	Counts      map[string]int
	Reason      string
}

// ComputeRecommendedParallelism starts at base (default 2), adds 2 after 10 clean packets, and subtracts 2 on trouble.
func ComputeRecommendedParallelism(outcomes []string, base int) Parallelism {
	if base < parallelismMin {
		base = parallelismMin
	}
	if base > parallelismMax {
		base = parallelismMax
	}
	var terminal []string
	for _, r := range outcomes {
		if _, ok := terminalResults[r]; ok {
			terminal = append(terminal, r)
		}
	}
	window := terminal
	if len(window) > parallelismWindow {
		window = window[len(window)-parallelismWindow:]
	}
	counts := map[string]int{"adopted": 0, "reverted": 0, "mismatch": 0, "cancelled": 0}
	for _, item := range window {
		counts[item]++
	}
	out := Parallelism{Base: base, WindowSize: len(window), Counts: counts}
	if len(window) == 0 {
		out.Recommended = base
		out.Reason = "no terminal packet outcomes recorded yet; holding base parallelism"
		return out
	}
	if counts["reverted"] >= scaleDownMinReverted || counts["mismatch"] >= scaleDownMinMismatch {
		out.Delta = -parallelismStep
		out.Recommended = base + out.Delta
		if out.Recommended < parallelismMin {
			out.Recommended = parallelismMin
		}
		out.Reason = fmt.Sprintf("last %d outcomes had reverted=%d, mismatch=%d; scaling down (-2, floor %d)", len(window), counts["reverted"], counts["mismatch"], parallelismMin)
		return out
	}
	if len(window) >= parallelismWindow && counts["adopted"] > 0 && counts["reverted"] <= scaleUpMaxReverted && counts["mismatch"] == 0 {
		out.Delta = parallelismStep
		out.Recommended = base + out.Delta
		if out.Recommended > parallelismMax {
			out.Recommended = parallelismMax
		}
		out.Reason = fmt.Sprintf("last %d outcomes had adopted=%d, reverted=%d, mismatch=0; scaling up (+2, cap %d)", len(window), counts["adopted"], counts["reverted"], parallelismMax)
		return out
	}
	out.Recommended = base
	out.Reason = fmt.Sprintf("last %d outcomes in steady/warm-up band (adopted=%d, reverted=%d, mismatch=%d, required_window=%d); holding parallelism", len(window), counts["adopted"], counts["reverted"], counts["mismatch"], parallelismWindow)
	return out
}

func utcNow() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

func ensureDecisions(runDir string) (string, error) {
	if err := os.MkdirAll(runDir, fsperm.Dir); err != nil {
		return "", err
	}
	path := filepath.Join(runDir, decisionsFilename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		header := strings.Join(tsvColumns, "\t") + "\n"
		if err := os.WriteFile(path, []byte(header), fsperm.File); err != nil {
			return "", err
		}
	}
	return path, nil
}

func appendDecision(runDir, phase, decision, why, evidence, result string) (string, error) {
	phase, err := validateTSVField("phase", phase)
	if err != nil {
		return "", err
	}
	decision, err = validateTSVField("decision", decision)
	if err != nil {
		return "", err
	}
	why, err = validateTSVField("why", why)
	if err != nil {
		return "", err
	}
	evidence, err = validateTSVField("evidence", evidence)
	if err != nil {
		return "", err
	}
	if !IsValidEvidencePointer(evidence) {
		return "", fmt.Errorf("evidence '%s' must be a concrete pointer (file path, commit:<sha>, cmd:<command>, #<seq>, ADR/PM/C id, or URL)", evidence)
	}
	if _, ok := validResults[result]; !ok {
		return "", fmt.Errorf("invalid result '%s'", result)
	}
	path, err := ensureDecisions(runDir)
	if err != nil {
		return "", err
	}
	row := strings.Join([]string{utcNow(), phase, decision, why, evidence, result}, "\t") + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, fsperm.File)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(row); err != nil {
		return "", err
	}
	return path, nil
}

func readDecisions(runDir string) []map[string]string {
	path := filepath.Join(runDir, decisionsFilename)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	header := strings.Join(tsvColumns, "\t")
	start := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == header {
		start = 1
	}
	var rows []map[string]string
	for _, line := range lines[start:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		for len(parts) < len(tsvColumns) {
			parts = append(parts, "")
		}
		row := map[string]string{}
		for i, col := range tsvColumns {
			row[col] = parts[i]
		}
		rows = append(rows, row)
	}
	return rows
}

// RunHarness is the harness_run.py CLI.
func RunHarness(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, harnessUsage())
		if len(args) == 0 {
			fmt.Fprintln(stderr, "error: a subcommand is required")
			return harnessExitUsage
		}
		return harnessExitOK
	}
	sub := args[0]
	rest := args[1:]
	repoFlag, branch, rest := takeCommon(rest)
	repo := ResolveRepoRoot(repoFlag, "")
	switch sub {
	case "init":
		return cmdInit(repo, branch, rest, stdout, stderr)
	case "log":
		return cmdLog(repo, branch, rest, stdout, stderr)
	case "deactivate-packet":
		return cmdDeactivate(repo, branch, stdout)
	case "status":
		return cmdStatus(repo, branch, rest, stdout, stderr)
	case "subagent-stop":
		return cmdSubagentStop(ctx, repo, branch, stdin)
	case "activate-packet":
		return cmdActivate(ctx, repo, branch, rest, stdout, stderr)
	case "record-handoff":
		return cmdHandoff(ctx, repo, branch, rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "error: unknown subcommand %s\n", sub)
		return harnessExitUsage
	}
}

func harnessUsage() string {
	return "usage: harness <init|log|activate-packet|deactivate-packet|record-handoff|subagent-stop|status> [flags]\n"
}

func takeCommon(args []string) (repo, branch string, rest []string) {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--repo-root" && i+1 < len(args):
			repo = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--repo-root="):
			repo = strings.TrimPrefix(args[i], "--repo-root=")
		case args[i] == "--branch" && i+1 < len(args):
			branch = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--branch="):
			branch = strings.TrimPrefix(args[i], "--branch=")
		default:
			rest = append(rest, args[i])
		}
	}
	return repo, branch, rest
}

func flagValue(args []string, name string) (string, bool) {
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			return args[i+1], true
		}
		if strings.HasPrefix(args[i], name+"=") {
			return strings.TrimPrefix(args[i], name+"="), true
		}
	}
	return "", false
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

func cmdInit(repo, branch string, args []string, stdout, stderr io.Writer) int {
	policy, ok := flagValue(args, "--policy")
	goal, okG := flagValue(args, "--goal")
	completion, okC := flagValue(args, "--completion")
	if !ok || !okG || !okC {
		fmt.Fprintln(stderr, "error: init requires --policy, --goal, and --completion")
		return harnessExitUsage
	}
	if _, ok := validPolicies[policy]; !ok {
		fmt.Fprintf(stderr, "error: invalid policy %s\n", policy)
		return harnessExitUsage
	}
	runDir, err := RunDirForBranch(repo, branch)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	decisions := filepath.Join(runDir, decisionsFilename)
	if _, err := os.Stat(decisions); err == nil && !hasFlag(args, "--force") {
		fmt.Fprintf(stderr, "error: run already initialized at %s (pass --force to reinitialize)\n", decisions)
		return harnessExitError
	}
	goal, err = validateTSVField("goal", goal)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitUsage
	}
	completion, err = validateTSVField("completion", completion)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitUsage
	}
	if err := os.MkdirAll(runDir, fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	for _, sub := range []string{"packets", "handoffs", "verify", "evidence"} {
		if err := os.MkdirAll(filepath.Join(runDir, sub), fsperm.Dir); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return harnessExitError
		}
	}
	header := strings.Join(tsvColumns, "\t") + "\n"
	if err := os.WriteFile(decisions, []byte(header), fsperm.File); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	rel, err := filepath.Rel(resolveExisting(repo), decisions)
	if err != nil {
		rel = decisions
	}
	rel = filepath.ToSlash(rel)
	if _, err := appendDecision(runDir, "init", "policy="+policy+"; goal="+goal+"; completion="+completion, "LOOP-03 run initialization: record policy and completion criteria", rel, "info"); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitUsage
	}
	fmt.Fprintf(stdout, "initialized harness run at %s\n", runDir)
	return harnessExitOK
}

func cmdLog(repo, branch string, args []string, stdout, stderr io.Writer) int {
	phase, ok1 := flagValue(args, "--phase")
	decision, ok2 := flagValue(args, "--decision")
	why, ok3 := flagValue(args, "--why")
	evidence, ok4 := flagValue(args, "--evidence")
	result, ok5 := flagValue(args, "--result")
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
		fmt.Fprintln(stderr, "error: log requires --phase, --decision, --why, --evidence, and --result")
		return harnessExitUsage
	}
	runDir, err := RunDirForBranch(repo, branch)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	path, err := appendDecision(runDir, phase, decision, why, evidence, result)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitUsage
	}
	fmt.Fprintf(stdout, "logged decision to %s\n", path)
	return harnessExitOK
}

func cmdDeactivate(repo, branch string, stdout io.Writer) int {
	runDir, err := RunDirForBranch(repo, branch)
	if err != nil {
		fmt.Fprintf(stdout, "no active packet to deactivate\n")
		return harnessExitOK
	}
	state := filepath.Join(runDir, activePacketFilename)
	raw, err := os.ReadFile(state)
	if err != nil {
		fmt.Fprintf(stdout, "no active packet to deactivate in %s\n", runDir)
		return harnessExitOK
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil || data == nil {
		data = map[string]any{}
	}
	data["active"] = false
	data["deactivated_at"] = utcNow()
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return harnessExitError
	}
	if err := os.WriteFile(state, append(encoded, '\n'), fsperm.File); err != nil {
		return harnessExitError
	}
	fmt.Fprintf(stdout, "deactivated active packet in %s\n", runDir)
	return harnessExitOK
}

func cmdStatus(repo, branch string, args []string, stdout, stderr io.Writer) int {
	format, _ := flagValue(args, "--format")
	if format == "" {
		format = "text"
	}
	base := parallelismBaseDefault
	if raw, ok := flagValue(args, "--base-parallelism"); ok {
		var n int
		if _, err := fmt.Sscan(raw, &n); err != nil {
			fmt.Fprintf(stderr, "error: invalid --base-parallelism\n")
			return harnessExitUsage
		}
		base = n
	}
	currentDir, err := RunDirForBranch(repo, branch)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	rows := readDecisions(currentDir)
	var outcomes []string
	for _, row := range rows {
		outcomes = append(outcomes, row["result"])
	}
	par := ComputeRecommendedParallelism(outcomes, base)
	if format == "json" {
		payload := map[string]any{
			"repo_root":      repo,
			"current_branch": ResolveGitBranch(repo, branch),
			"current_run": map[string]any{
				"branch":          filepath.Base(currentDir),
				"decisions_count": len(rows),
				"parallelism": map[string]any{
					"base":        par.Base,
					"delta":       par.Delta,
					"recommended": par.Recommended,
					"window_size": par.WindowSize,
					"counts":      par.Counts,
					"reason":      par.Reason,
				},
			},
		}
		fmt.Fprintln(stdout, pythonJSON(payload))
		return harnessExitOK
	}
	fmt.Fprintf(stdout, "parallelism   : %d (base=%d, delta=%+d; %s)\n", par.Recommended, par.Base, par.Delta, par.Reason)
	return harnessExitOK
}

func cmdSubagentStop(ctx context.Context, repo, branch string, stdin io.Reader) int {
	_ = ctx
	if stdin == nil {
		return harnessExitOK
	}
	raw, err := io.ReadAll(stdin)
	if err != nil || strings.TrimSpace(string(raw)) == "" {
		return harnessExitOK
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return harnessExitOK
	}
	last, _ := payload["last_assistant_message"].(string)
	if len(last) > maxAssistantMsgChars {
		last = last[:maxAssistantMsgChars] + "...(truncated)"
	}
	record := map[string]any{
		"ts":                     utcNow(),
		"hook_event_name":        stringOr(payload["hook_event_name"], "SubagentStop"),
		"session_id":             stringOr(payload["session_id"], ""),
		"agent_id":               stringOr(payload["agent_id"], ""),
		"subagent_type":          firstString(payload["subagent_type"], payload["agent_type"]),
		"agent_transcript_path":  stringOr(payload["agent_transcript_path"], ""),
		"cwd":                    stringOr(payload["cwd"], ""),
		"last_assistant_message": last,
	}
	runDir, err := RunDirForBranch(repo, branch)
	if err != nil {
		return harnessExitError
	}
	if err := os.MkdirAll(runDir, fsperm.Dir); err != nil {
		return harnessExitError
	}
	f, err := os.OpenFile(filepath.Join(runDir, subagentsFilename), os.O_CREATE|os.O_APPEND|os.O_WRONLY, fsperm.File)
	if err != nil {
		return harnessExitError
	}
	defer f.Close()
	enc, _ := json.Marshal(record)
	_, _ = f.Write(append(enc, '\n'))
	return harnessExitOK
}

func stringOr(v any, fallback string) string {
	s, ok := v.(string)
	if !ok {
		return fallback
	}
	return s
}

func firstString(vals ...any) string {
	for _, v := range vals {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func cmdActivate(ctx context.Context, repo, branch string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(stderr, "error: activate-packet requires a packet path")
		return harnessExitUsage
	}
	src := args[0]
	info, err := os.Stat(src)
	if err != nil || !info.Mode().IsRegular() {
		fmt.Fprintf(stderr, "error: packet file not found: %s\n", src)
		return harnessExitUsage
	}
	data, issues, rc, errText := validatePacketViaPython(ctx, repo, src)
	if errText != "" && rc != harnessExitOK && data == nil {
		fmt.Fprint(stderr, errText)
		return rc
	}
	if len(issues) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "error: packet %s failed validation (%d issue(s)):\n", src, len(issues))
		for _, issue := range issues {
			fmt.Fprintf(&b, "  [%s] %s: %s\n", issue.code, issue.path, issue.message)
		}
		fmt.Fprint(stderr, b.String())
		return harnessExitError
	}
	if data == nil {
		fmt.Fprintf(stderr, "error: failed to parse packet %s\n", src)
		return harnessExitUsage
	}
	packetID, _ := data["packet_id"].(string)
	if !packetIDRE.MatchString(packetID) {
		fmt.Fprintf(stderr, "error: failed to extract packet_id from %s: invalid or missing packet_id: %#v\n", src, data["packet_id"])
		return harnessExitUsage
	}
	scope, _ := data["scope"].(map[string]any)
	if scope == nil {
		fmt.Fprintln(stderr, "error: packet missing 'scope' mapping")
		return harnessExitUsage
	}
	allowed := stringList(scope["allowed_paths"])
	if len(allowed) == 0 {
		fmt.Fprintln(stderr, "error: packet scope.allowed_paths must be a non-empty list of strings")
		return harnessExitUsage
	}
	forbidden := stringList(scope["forbidden_paths"])
	execPolicy, _ := data["execution_policy"].(map[string]any)
	if execPolicy == nil {
		execPolicy = map[string]any{}
	}
	runDir, err := RunDirForBranch(repo, branch)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	packets := filepath.Join(runDir, "packets")
	if err := os.MkdirAll(packets, fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	dest := filepath.Join(packets, packetID+".md")
	raw, err := os.ReadFile(src)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	if err := os.WriteFile(dest, raw, fsperm.File); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	rel, _ := filepath.Rel(resolveExisting(repo), dest)
	rel = filepath.ToSlash(rel)
	state := map[string]any{
		"active":                      true,
		"packet_id":                   packetID,
		"packet_file":                 rel,
		"activated_at":                utcNow(),
		"allowed_paths":               allowed,
		"forbidden_paths":             forbidden,
		"allow_generated_code_edits":  scope["allow_generated_code_edits"] == true,
		"forbid_repo_wide_test_build": boolOr(execPolicy["forbid_repo_wide_test_build"], true),
		"foreground_only":             boolOr(execPolicy["foreground_only"], true),
	}
	encoded, _ := json.MarshalIndent(state, "", "  ")
	statePath := filepath.Join(runDir, activePacketFilename)
	if err := os.WriteFile(statePath, append(encoded, '\n'), fsperm.File); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	if _, err := appendDecision(runDir, "dispatch", "activate packet "+packetID, fmt.Sprintf("dispatch bounded task packet (allowed_paths=%d)", len(allowed)), rel, "info"); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitUsage
	}
	fmt.Fprintf(stdout, "activated packet %s -> %s\n", packetID, statePath)
	return harnessExitOK
}

func boolOr(v any, fallback bool) bool {
	b, ok := v.(bool)
	if !ok {
		return fallback
	}
	return b
}

type packetIssue struct {
	code, path, message string
}

func validatePacketViaPython(ctx context.Context, repo, src string) (map[string]any, []packetIssue, int, string) {
	if ctx == nil {
		return nil, nil, harnessExitError, "error: nil context\n"
	}
	script := `import json, sys
sys.path.insert(0, sys.argv[1])
import task_packet
from pathlib import Path
src = Path(sys.argv[2])
try:
    doc = task_packet.load_document(src)
except Exception as exc:
    json.dump({"error": str(exc)}, sys.stdout)
    raise SystemExit(0)
schema = task_packet.load_schema()
issues = task_packet.validate_packet_doc(doc, schema)
json.dump({"data": doc.data, "issues": [{"code": i.code, "path": i.path, "message": i.message} for i in issues]}, sys.stdout)
`
	stdout, stderr, code, err := execx.Combined(ctx, execx.Cmd{Args: []string{"python3", "-c", script, filepath.Join(repo, "scripts", "agent-harness"), src}, Dir: repo})
	if err != nil {
		return nil, nil, harnessExitError, fmt.Sprintf("error: failed to parse packet %s: %v\n", src, err)
	}
	if code != 0 && strings.TrimSpace(stdout) == "" {
		return nil, nil, harnessExitUsage, fmt.Sprintf("error: failed to parse packet %s: %s\n", src, strings.TrimSpace(stderr))
	}
	var payload struct {
		Error  string         `json:"error"`
		Data   map[string]any `json:"data"`
		Issues []struct {
			Code    string `json:"code"`
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		return nil, nil, harnessExitUsage, fmt.Sprintf("error: failed to parse packet %s: %v\n", src, err)
	}
	if payload.Error != "" {
		return nil, nil, harnessExitUsage, fmt.Sprintf("error: failed to parse packet %s: %s\n", src, payload.Error)
	}
	var issues []packetIssue
	for _, item := range payload.Issues {
		issues = append(issues, packetIssue{code: item.Code, path: item.Path, message: item.Message})
	}
	return payload.Data, issues, harnessExitOK, ""
}

func cmdHandoff(ctx context.Context, repo, branch string, args []string, stdout, stderr io.Writer) int {
	var path, result, why string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--result" && i+1 < len(args):
			result = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--result="):
			result = strings.TrimPrefix(args[i], "--result=")
		case args[i] == "--why" && i+1 < len(args):
			why = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--why="):
			why = strings.TrimPrefix(args[i], "--why=")
		case !strings.HasPrefix(args[i], "-") && path == "":
			path = args[i]
		}
	}
	if path == "" {
		fmt.Fprintln(stderr, "error: record-handoff requires a handoff path")
		return harnessExitUsage
	}
	if _, ok := terminalResults[result]; !ok {
		fmt.Fprintf(stderr, "error: invalid result %s\n", result)
		return harnessExitUsage
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		fmt.Fprintf(stderr, "error: handoff file not found: %s\n", path)
		return harnessExitUsage
	}
	data, _, _, errText := validatePacketViaPython(ctx, repo, path)
	if data == nil {
		if errText == "" {
			errText = fmt.Sprintf("error: failed to parse handoff %s\n", path)
		}
		fmt.Fprint(stderr, strings.Replace(errText, "packet", "handoff", 1))
		return harnessExitUsage
	}
	packetID, _ := data["packet_id"].(string)
	if !packetIDRE.MatchString(packetID) {
		fmt.Fprintf(stderr, "error: failed to parse handoff %s: invalid or missing packet_id\n", path)
		return harnessExitUsage
	}
	runDir, err := RunDirForBranch(repo, branch)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	if err := os.MkdirAll(filepath.Join(runDir, "handoffs"), fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".md" && ext != ".json" {
		ext = ".md"
	}
	dest := filepath.Join(runDir, "handoffs", packetID+ext)
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	if err := os.WriteFile(dest, raw, fsperm.File); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitError
	}
	_ = deactivateQuiet(runDir)
	rel, _ := filepath.Rel(resolveExisting(repo), dest)
	rel = filepath.ToSlash(rel)
	if why == "" {
		why = "recorded handoff outcome for " + packetID
	}
	if _, err := appendDecision(runDir, "handoff", "handoff "+packetID+": "+result, why, rel, result); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return harnessExitUsage
	}
	fmt.Fprintf(stdout, "recorded handoff %s (%s) -> %s\n", packetID, result, dest)
	return harnessExitOK
}

func deactivateQuiet(runDir string) bool {
	state := filepath.Join(runDir, activePacketFilename)
	raw, err := os.ReadFile(state)
	if err != nil {
		return false
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil || data == nil {
		data = map[string]any{}
	}
	data["active"] = false
	data["deactivated_at"] = utcNow()
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return false
	}
	return os.WriteFile(state, append(encoded, '\n'), fsperm.File) == nil
}
