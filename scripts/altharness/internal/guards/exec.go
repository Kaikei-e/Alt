package guards

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	execBlockedRC      = 2
	execMaxRecursion   = 32
	execRulePriorityE1 = 3
	execRulePriorityE2 = 2
	execRulePriorityE3 = 1
	pushBlockMessage   = "BLOCKED: git/gh push-class commands are forbidden by user policy. Ask the user to run push manually."
)

var (
	controlKeywords   = map[string]struct{}{"if": {}, "then": {}, "elif": {}, "else": {}, "do": {}, "while": {}, "until": {}, "!": {}, "{": {}}
	simpleWrappers    = map[string]struct{}{"time": {}, "command": {}, "exec": {}, "builtin": {}}
	shellValueFlags   = map[string]struct{}{"--rcfile": {}, "--init-file": {}, "-o": {}, "+o": {}}
	falseBools        = map[string]struct{}{"false": {}, "0": {}, "no": {}, "off": {}}
	cargoValueFlags   = map[string]struct{}{"-C": {}, "--manifest-path": {}, "--target-dir": {}, "--config": {}, "--color": {}, "-Z": {}}
	makeValueFlags    = map[string]struct{}{"-C": {}, "-f": {}, "-I": {}, "-j": {}, "-l": {}, "-o": {}, "-W": {}, "--directory": {}, "--file": {}, "--makefile": {}, "--jobs": {}}
	gitValueFlags     = map[string]struct{}{"-C": {}, "-c": {}, "--git-dir": {}, "--work-tree": {}, "--namespace": {}, "--super-prefix": {}, "--config-env": {}, "--exec-path": {}}
	gitAttachedFlags  = []string{"-C", "-c"}
	wrapperValueFlags = map[string]map[string]struct{}{
		"env":     {"-u": {}, "--unset": {}, "-C": {}, "--chdir": {}},
		"sudo":    {"-u": {}, "-g": {}, "-C": {}, "-h": {}, "-p": {}, "-r": {}, "-t": {}, "-T": {}, "-U": {}, "--user": {}, "--group": {}},
		"timeout": {"-k": {}, "--kill-after": {}, "-s": {}, "--signal": {}},
		"nice":    {"-n": {}, "--adjustment": {}},
		"xargs":   {"-a": {}, "-d": {}, "-E": {}, "-I": {}, "-L": {}, "-n": {}, "-P": {}, "-s": {}, "--arg-file": {}, "--delimiter": {}, "--eof": {}, "--replace": {}, "--max-lines": {}, "--max-args": {}, "--max-procs": {}, "--max-chars": {}},
		"npx":     {"-p": {}, "--package": {}},
		"bunx":    {"-p": {}, "--package": {}},
		"bun":     {"-p": {}, "--package": {}, "--cwd": {}, "-c": {}, "--config": {}},
		"pnpm":    {"-C": {}, "--dir": {}, "-F": {}, "--filter": {}, "--prefix": {}, "-p": {}, "--package": {}},
		"uv":      {"-p": {}, "-w": {}, "--project": {}, "--directory": {}, "--python": {}, "--with": {}, "--from": {}, "--group": {}, "--extra": {}, "--package": {}},
	}
	e1FlagRules = map[string][2]string{
		"golangci-lint": {"--fix", "golangci-lint --fix"},
		"clippy-driver": {"--fix", "clippy-driver --fix"},
		"ruff":          {"--unsafe-fixes", "ruff --unsafe-fixes"},
		"biome":         {"--unsafe", "biome --unsafe"},
	}
)

type execVerdict struct {
	rule   string
	detail string
}

func ruleRank(rule string) int {
	switch rule {
	case "E1":
		return execRulePriorityE1
	case "E2":
		return execRulePriorityE2
	case "E3":
		return execRulePriorityE3
	default:
		return 0
	}
}

func pickHigher(current, candidate *execVerdict) *execVerdict {
	if candidate == nil {
		return current
	}
	if current == nil || ruleRank(candidate.rule) > ruleRank(current.rule) {
		return candidate
	}
	return current
}

func isEnabledFlag(arg, flagName string) bool {
	if arg == flagName {
		return true
	}
	prefix := flagName + "="
	if strings.HasPrefix(arg, prefix) {
		val := strings.ToLower(strings.TrimSpace(arg[len(prefix):]))
		_, off := falseBools[val]
		return !off
	}
	return false
}

func skipWrapperFlags(tokens []Token, start int, valueFlags map[string]struct{}, skipOne bool) int {
	idx := start
	n := len(tokens)
	for idx < n {
		text := tokens[idx].Text
		if text == "--" {
			idx++
			break
		}
		if _, ok := valueFlags[text]; ok {
			idx += 2
			continue
		}
		if strings.HasPrefix(text, "-") {
			idx++
			continue
		}
		break
	}
	if skipOne && idx < n {
		idx++
	}
	return idx
}

func unwrapPythonModule(tokens []Token, idx int) (int, bool) {
	n := len(tokens)
	j := idx + 1
	for j < n {
		tok := tokens[j].Text
		if tok == "-m" && j+1 < n {
			return j + 1, true
		}
		if strings.HasPrefix(tok, "-") {
			j++
			continue
		}
		return 0, false
	}
	return 0, false
}

func unwrapEnv(tokens []Token, idx int, extra *[]string) int {
	j := idx + 1
	n := len(tokens)
	for j < n {
		text := tokens[j].Text
		if text == "--" {
			return j + 1
		}
		if text == "-S" || text == "--split-string" {
			if j+1 < n {
				splitStr := tokens[j+1].Text
				*extra = append(*extra, splitStr)
				var tail []string
				for _, t := range tokens[j+2:] {
					tail = append(tail, t.Text)
				}
				if len(tail) > 0 {
					*extra = append(*extra, splitStr+" "+strings.Join(tail, " "))
				}
				j += 2
				continue
			}
			return n
		}
		if strings.HasPrefix(text, "--split-string=") {
			*extra = append(*extra, strings.SplitN(text, "=", 2)[1])
			j++
			continue
		}
		if strings.HasPrefix(text, "-S") && len(text) > len("-S") {
			*extra = append(*extra, text[2:])
			j++
			continue
		}
		if _, ok := wrapperValueFlags["env"][text]; ok {
			j += 2
			continue
		}
		if strings.HasPrefix(text, "-") {
			j++
			continue
		}
		break
	}
	return j
}

func unwrapPkgRunner(base string, tokens []Token, idx int) (int, bool) {
	n := len(tokens)
	switch base {
	case "uvx":
		return skipWrapperFlags(tokens, idx+1, wrapperValueFlags["uv"], false), true
	case "uv":
		j := skipWrapperFlags(tokens, idx+1, wrapperValueFlags["uv"], false)
		if j < n && tokens[j].Text == "run" {
			return skipWrapperFlags(tokens, j+1, wrapperValueFlags["uv"], false), true
		}
		if j < n && tokens[j].Text == "tool" {
			k := skipWrapperFlags(tokens, j+1, wrapperValueFlags["uv"], false)
			if k < n && tokens[k].Text == "run" {
				return skipWrapperFlags(tokens, k+1, wrapperValueFlags["uv"], false), true
			}
		}
	case "bun", "pnpm":
		subcmds := map[string]struct{}{"dlx": {}, "exec": {}}
		if base == "bun" {
			subcmds = map[string]struct{}{"x": {}, "exec": {}}
		}
		j := skipWrapperFlags(tokens, idx+1, wrapperValueFlags[base], false)
		if j < n {
			if _, ok := subcmds[tokens[j].Text]; ok {
				return skipWrapperFlags(tokens, j+1, wrapperValueFlags[base], false), true
			}
		}
	}
	return 0, false
}

func unwrapCommandPrefix(tokens []Token) ([]Token, map[string]string, bool, []string) {
	envVars := map[string]string{}
	var extra []string
	sawNohup := false
	idx := 0
	n := len(tokens)
	for idx < n {
		tok := tokens[idx]
		if envAssignRE.MatchString(tok.Text) {
			parts := strings.SplitN(tok.Text, "=", 2)
			envVars[parts[0]] = parts[1]
			idx++
			continue
		}
		base := baseName(tok.Text)
		if !tok.Quoted {
			if _, ok := controlKeywords[base]; ok {
				idx++
				continue
			}
		}
		if base == "nohup" {
			sawNohup = true
			idx++
			continue
		}
		if base == "env" {
			idx = unwrapEnv(tokens, idx, &extra)
			continue
		}
		if _, ok := simpleWrappers[base]; ok {
			idx = skipWrapperFlags(tokens, idx+1, map[string]struct{}{}, false)
			continue
		}
		if base == "sudo" || base == "nice" || base == "xargs" || base == "npx" || base == "bunx" || base == "timeout" {
			idx = skipWrapperFlags(tokens, idx+1, wrapperValueFlags[base], base == "timeout")
			continue
		}
		if base == "uv" || base == "uvx" || base == "bun" || base == "pnpm" {
			if next, ok := unwrapPkgRunner(base, tokens, idx); ok {
				idx = next
				continue
			}
		}
		if base == "python" || base == "python3" {
			if next, ok := unwrapPythonModule(tokens, idx); ok {
				idx = next
				continue
			}
		}
		break
	}
	if idx > n {
		idx = n
	}
	return tokens[idx:], envVars, sawNohup, extra
}

func checkCargoE1(args []string) string {
	i := 0
	n := len(args)
	sub := ""
	for i < n {
		tok := args[i]
		if strings.HasPrefix(tok, "+") && sub == "" {
			i++
			continue
		}
		if _, ok := cargoValueFlags[tok]; ok && sub == "" {
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "-") && sub == "" {
			i++
			continue
		}
		sub = tok
		i++
		break
	}
	if sub == "fix" {
		return "cargo fix"
	}
	if sub == "clippy" {
		for _, a := range args[i:] {
			if isEnabledFlag(a, "--fix") {
				return "cargo clippy --fix"
			}
		}
	}
	return ""
}

func normalizeToolName(raw string) string {
	name := baseName(raw)
	if strings.Contains(name, "@") && !strings.HasPrefix(name, "@") {
		return strings.SplitN(name, "@", 2)[0]
	}
	return name
}

func checkE1Command(cmd []string) string {
	if len(cmd) == 0 {
		return ""
	}
	base := normalizeToolName(cmd[0])
	args := cmd[1:]
	if base == "cargo" {
		return checkCargoE1(args)
	}
	spec, ok := e1FlagRules[base]
	if !ok {
		return ""
	}
	for _, a := range args {
		if isEnabledFlag(a, spec[0]) {
			return spec[1]
		}
	}
	return ""
}

func isRunShPath(token string) bool {
	parts := strings.Split(strings.ReplaceAll(token, `\`, "/"), "/")
	var clean []string
	for _, p := range parts {
		if p != "" {
			clean = append(clean, p)
		}
	}
	if len(clean) < 3 {
		return false
	}
	tail := clean[len(clean)-3:]
	return tail[0] == "scripts" && tail[1] == "lint" && tail[2] == "run.sh"
}

func checkRunShAll(args []string) string {
	i := 0
	n := len(args)
	for i < n {
		tok := args[i]
		if tok == "--policy" {
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "--policy=") {
			i++
			continue
		}
		if tok == "--" {
			i++
			break
		}
		if strings.HasPrefix(tok, "-") {
			i++
			continue
		}
		if tok == "all" {
			return "scripts/lint/run.sh all"
		}
		return ""
	}
	if i < n && args[i] == "all" {
		return "scripts/lint/run.sh all"
	}
	return ""
}

func checkMakeLint(args []string, envVars map[string]string) string {
	var targets []string
	targetVar, hasTarget := envVars["TARGET"]
	_ = hasTarget
	i := 0
	for i < len(args) {
		tok := args[i]
		if _, ok := makeValueFlags[tok]; ok {
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "-") {
			i++
			continue
		}
		if strings.Contains(tok, "=") && !strings.HasPrefix(tok, "=") {
			parts := strings.SplitN(tok, "=", 2)
			if parts[0] == "TARGET" {
				targetVar = parts[1]
			}
			i++
			continue
		}
		targets = append(targets, tok)
		i++
	}
	hasLint := false
	for _, t := range targets {
		if t == "lint" {
			hasLint = true
		}
	}
	if !hasLint {
		return ""
	}
	if strings.TrimSpace(targetVar) == "" || strings.TrimSpace(targetVar) == "all" {
		return "make lint"
	}
	return ""
}

func extractShellCScriptExec(tokens []Token, start int) (string, bool) {
	i := start + 1
	n := len(tokens)
	for i < n {
		tok := tokens[i].Text
		if tok == "--" {
			return "", false
		}
		if _, ok := shellValueFlags[tok]; ok {
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "--") {
			i++
			continue
		}
		if len(tok) >= 2 && (tok[0] == '-' || tok[0] == '+') {
			flags := tok[1:]
			hasC := tok[0] == '-' && strings.ContainsRune(flags, 'c')
			hasO := strings.ContainsRune(flags, 'o')
			if hasC {
				argIdx := i + 1
				if hasO {
					argIdx = i + 2
				}
				if argIdx < n {
					return tokens[argIdx].Text, true
				}
				return "", false
			}
			if hasO {
				i += 2
				continue
			}
			i++
			continue
		}
		return "", false
	}
	return "", false
}

func checkE2Command(cmd []string, envVars map[string]string) string {
	if len(cmd) == 0 {
		return ""
	}
	if isRunShPath(cmd[0]) {
		return checkRunShAll(cmd[1:])
	}
	base := baseName(cmd[0])
	if base == "make" {
		return checkMakeLint(cmd[1:], envVars)
	}
	if isShellName(base) {
		i := 1
		for i < len(cmd) {
			tok := cmd[i]
			if _, ok := shellValueFlags[tok]; ok {
				i += 2
				continue
			}
			if strings.HasPrefix(tok, "--") {
				i++
				continue
			}
			if len(tok) >= 2 && (tok[0] == '-' || tok[0] == '+') {
				if strings.ContainsRune(tok[1:], 'o') {
					i += 2
					continue
				}
				i++
				continue
			}
			break
		}
		if i < len(cmd) && isRunShPath(cmd[i]) {
			return checkRunShAll(cmd[i+1:])
		}
	}
	return ""
}

func extractNestedScripts(remaining []Token) []string {
	if len(remaining) == 0 {
		return nil
	}
	base := baseName(remaining[0].Text)
	if base == "eval" {
		var args []string
		for _, t := range remaining[1:] {
			args = append(args, t.Text)
		}
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		joined := strings.TrimSpace(strings.Join(args, " "))
		if joined == "" {
			return nil
		}
		return []string{joined}
	}
	if isShellName(base) {
		script, ok := extractShellCScriptExec(remaining, 0)
		if !ok {
			return nil
		}
		return []string{script}
	}
	return nil
}

// checkPushCmd inspects an unwrapped command argv for forbidden git/gh
// push-class operations (handling attached flags like -C/path and --git-dir=...).
func checkPushCmd(cmd []string) string {
	if len(cmd) == 0 {
		return ""
	}
	base := baseName(cmd[0])
	switch base {
	case "git":
		return checkGitPushCmd(cmd[1:])
	case "gh":
		return checkGhPushCmd(cmd[1:])
	default:
		return ""
	}
}

func checkGitPushCmd(args []string) string {
	i := 0
	for i < len(args) {
		tok := args[i]
		if tok == "--" {
			i++
			break
		}
		if _, ok := gitValueFlags[tok]; ok {
			i += 2
			continue
		}
		attached := false
		for _, pfx := range gitAttachedFlags {
			if strings.HasPrefix(tok, pfx) && len(tok) > len(pfx) {
				attached = true
				break
			}
		}
		if attached || strings.HasPrefix(tok, "-") {
			i++
			continue
		}
		break
	}
	if i >= len(args) {
		return ""
	}
	sub := args[i]
	rest := args[i+1:]
	switch sub {
	case "push":
		return "git push"
	case "remote":
		action, _ := firstPositional(rest, 0, map[string]struct{}{})
		switch action {
		case "add", "set-url", "update":
			return "git remote " + action
		}
	case "fetch":
		for _, a := range rest {
			if a == "push" || a == "--push" {
				return "git fetch ... push"
			}
		}
	}
	return ""
}

func checkGhPushCmd(args []string) string {
	group, next := firstPositional(args, 0, map[string]struct{}{})
	if group == "" {
		return ""
	}
	action, _ := firstPositional(args, next, map[string]struct{}{})
	switch group {
	case "pr":
		if action == "create" || action == "merge" {
			return "gh pr " + action
		}
	case "release":
		if action == "create" || action == "upload" {
			return "gh release " + action
		}
	case "repo":
		if action == "sync" {
			return "gh repo sync"
		}
	case "workflow":
		if action == "run" {
			return "gh workflow run"
		}
	case "run":
		if action == "rerun" {
			return "gh run rerun"
		}
	}
	return ""
}

func checkPushSegment(seg []Token, depth int) (string, error) {
	remaining, _, _, extra := unwrapCommandPrefix(seg)
	for _, script := range extra {
		hit, err := CheckPushCommand(script, depth+1)
		if err != nil || hit != "" {
			return hit, err
		}
	}
	if len(remaining) == 0 {
		return "", nil
	}
	cmd := make([]string, len(remaining))
	for i, t := range remaining {
		cmd[i] = t.Text
	}
	if hit := checkPushCmd(cmd); hit != "" {
		return hit, nil
	}
	for _, nestedScript := range extractNestedScripts(remaining) {
		hit, err := CheckPushCommand(nestedScript, depth+1)
		if err != nil || hit != "" {
			return hit, err
		}
	}
	return "", nil
}

// CheckPushCommand detects forbidden git/gh push-class operations using the
// token-aware shell parser (handling quotes, ANSI-C escapes, wrappers, and
// attached git flags like -C/path or --git-dir=...).
func CheckPushCommand(command string, depth int) (string, error) {
	if depth > execMaxRecursion {
		return "", fmt.Errorf("command nesting exceeded maximum recursion depth")
	}
	if strings.TrimSpace(command) == "" {
		return "", nil
	}
	for _, seg := range splitShellSegments(command) {
		hit, err := checkPushSegment(seg, depth)
		if err != nil || hit != "" {
			return hit, err
		}
	}
	for _, sub := range extractSubstitutions(command) {
		hit, err := CheckPushCommand(sub, depth+1)
		if err != nil || hit != "" {
			return hit, err
		}
	}
	return "", nil
}

// CheckPushArgv detects forbidden git/gh push-class operations in argv form.
func CheckPushArgv(argv []string) (string, error) {
	if len(argv) == 0 {
		return "", nil
	}
	tokens := make([]Token, len(argv))
	for i, part := range argv {
		tokens[i] = Token{Text: part, Quoted: false}
	}
	return checkPushSegment(tokens, 0)
}

func classifySegment(seg []Token, enforceE2, enforceE3 bool, depth int) (*execVerdict, error) {
	remaining, envVars, sawNohup, extra := unwrapCommandPrefix(seg)
	var best *execVerdict
	if enforceE3 && sawNohup {
		best = pickHigher(best, &execVerdict{"E3", "nohup"})
	}
	for _, script := range extra {
		nested, err := ClassifyCommand(script, enforceE2, enforceE3, depth+1)
		if err != nil {
			return nil, err
		}
		best = pickHigher(best, nested)
	}
	if len(remaining) == 0 {
		return best, nil
	}
	cmd := make([]string, len(remaining))
	for i, t := range remaining {
		cmd[i] = t.Text
	}
	if e1 := checkE1Command(cmd); e1 != "" {
		best = pickHigher(best, &execVerdict{"E1", e1})
	}
	if enforceE2 {
		if e2 := checkE2Command(cmd, envVars); e2 != "" {
			best = pickHigher(best, &execVerdict{"E2", e2})
		}
	}
	for _, nestedScript := range extractNestedScripts(remaining) {
		nested, err := ClassifyCommand(nestedScript, enforceE2, enforceE3, depth+1)
		if err != nil {
			return nil, err
		}
		best = pickHigher(best, nested)
	}
	return best, nil
}

// ClassifyCommand reports an EXEC-01 verdict. depth is for tests; callers pass 0.
func ClassifyCommand(command string, enforceE2, enforceE3 bool, depth int) (*execVerdict, error) {
	if depth > execMaxRecursion {
		return nil, fmt.Errorf("command nesting exceeded maximum recursion depth")
	}
	if strings.TrimSpace(command) == "" {
		return nil, nil
	}
	var best *execVerdict
	if enforceE3 && hasBackgroundAmpersand(command) {
		best = pickHigher(best, &execVerdict{"E3", "background &"})
	}
	for _, seg := range splitShellSegments(command) {
		nested, err := classifySegment(seg, enforceE2, enforceE3, depth)
		if err != nil {
			return nil, err
		}
		best = pickHigher(best, nested)
	}
	for _, sub := range extractSubstitutions(command) {
		nested, err := ClassifyCommand(sub, enforceE2, enforceE3, depth+1)
		if err != nil {
			return nil, err
		}
		best = pickHigher(best, nested)
	}
	return best, nil
}

func classifyArgv(argv []string, enforceE2, enforceE3 bool) (*execVerdict, error) {
	if len(argv) == 0 {
		return nil, nil
	}
	tokens := make([]Token, len(argv))
	for i, part := range argv {
		tokens[i] = Token{Text: part, Quoted: false}
	}
	return classifySegment(tokens, enforceE2, enforceE3, 0)
}

func formatExecBlock(rule, detail string) string {
	switch rule {
	case "E1":
		return fmt.Sprintf("BLOCKED [EXEC-01]: 一括自動修正 (%s) は禁止されています。対象箇所を手動で修正し ./scripts/lint/run.sh <target> で検証してください。\n", detail)
	case "E2":
		return "BLOCKED [EXEC-01]: 全体検査はホストが行います。./scripts/lint/run.sh <target> を実行してください。\n"
	default:
		return "BLOCKED [EXEC-01]: 前景同期で実行してください（背景実行 & / nohup は禁止）。\n"
	}
}

// RunExec is the EXEC-01 PreToolUse hook. Exit 2 blocks and exit 0 allows.
func RunExec(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	_ = ctx
	_ = args
	_ = stdout
	if stderr == nil {
		stderr = io.Discard
	}
	if stdin == nil {
		fmt.Fprintln(stderr, "BLOCKED [EXEC-01]: hook payload could not be parsed (missing stdin).")
		return execBlockedRC
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "BLOCKED [EXEC-01]: hook payload could not be parsed (%v).\n", err)
		return execBlockedRC
	}
	kind, cmdStr, cmdList, watchErr := parseExecHookResult(raw)
	if watchErr != nil {
		fmt.Fprintf(stderr, "BLOCKED [EXEC-01]: hook payload could not be parsed (%s).\n", watchErr.Error())
		return execBlockedRC
	}
	if kind == execIgnore {
		return 0
	}
	var pushHit string
	var pushErr error
	if cmdList != nil {
		pushHit, pushErr = CheckPushArgv(cmdList)
	} else {
		pushHit, pushErr = CheckPushCommand(cmdStr, 0)
	}
	if pushErr != nil {
		fmt.Fprintf(stderr, "BLOCKED [EXEC-01]: command analysis failed (%s).\n", pushErr.Error())
		return execBlockedRC
	}
	if pushHit != "" {
		fmt.Fprintln(stderr, pushBlockMessage)
		return execBlockedRC
	}
	repoRoot := ResolveRepoRoot("", "")
	packet, pktErr := LoadActivePacket(repoRoot, "")
	if pktErr != "" {
		fmt.Fprintf(stderr, "BLOCKED [EXEC-01]: %s.\n", pktErr)
		return execBlockedRC
	}
	isWorker := kind == execWorker
	enforceE2 := isWorker || (packet != nil && packetBool(packet, "forbid_repo_wide_test_build", true))
	enforceE3 := isWorker || (packet != nil && packetBool(packet, "foreground_only", true))
	var verdict *execVerdict
	var cerr error
	if cmdList != nil {
		verdict, cerr = classifyArgv(cmdList, enforceE2, enforceE3)
	} else {
		verdict, cerr = ClassifyCommand(cmdStr, enforceE2, enforceE3, 0)
	}
	if cerr != nil {
		fmt.Fprintf(stderr, "BLOCKED [EXEC-01]: command analysis failed (%s).\n", cerr.Error())
		return execBlockedRC
	}
	if verdict != nil {
		fmt.Fprint(stderr, formatExecBlock(verdict.rule, verdict.detail))
		return execBlockedRC
	}
	return 0
}

const (
	execIgnore = iota
	execString
	execList
	execWorker
)

func parseExecHookResult(raw []byte) (int, string, []string, error) {
	if !validUTF8(string(raw)) {
		return 0, "", nil, fmt.Errorf("invalid UTF-8 stdin")
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, "", nil, fmt.Errorf("invalid JSON (%s)", jsonErrMsg(err))
	}
	tool, _ := payload["tool_name"].(string)
	if strings.TrimSpace(tool) == "" {
		return 0, "", nil, fmt.Errorf("missing tool_name")
	}
	if tool != "Bash" && tool != "shell" {
		return execIgnore, "", nil, nil
	}
	input, ok := payload["tool_input"].(map[string]any)
	if !ok {
		return 0, "", nil, fmt.Errorf("missing tool_input for %s", tool)
	}
	worker := nonEmptyString(payload["agent_id"]) || nonEmptyString(payload["subagent_type"]) || strings.EqualFold(strings.TrimSpace(os.Getenv("ALT_HARNESS_ROLE")), "worker")
	kind := execString
	if worker {
		kind = execWorker
	}
	switch cmd := input["command"].(type) {
	case string:
		return kind, cmd, nil, nil
	case []any:
		out := make([]string, 0, len(cmd))
		for _, part := range cmd {
			s, ok := part.(string)
			if !ok {
				return 0, "", nil, fmt.Errorf("missing or invalid command for %s", tool)
			}
			out = append(out, s)
		}
		return kind, "", out, nil
	default:
		return 0, "", nil, fmt.Errorf("missing or invalid command for %s", tool)
	}
}

func nonEmptyString(v any) bool {
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) != ""
}

func jsonErrMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func packetBool(packet map[string]any, key string, fallback bool) bool {
	v, ok := packet[key]
	if !ok {
		return fallback
	}
	b, ok := v.(bool)
	if !ok {
		return fallback
	}
	return b
}
