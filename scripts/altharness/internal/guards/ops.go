package guards

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

var (
	dockerValueFlags = map[string]struct{}{
		"-H": {}, "--host": {}, "-c": {}, "--context": {}, "-l": {}, "--log-level": {}, "--config": {},
		"--tlscacert": {}, "--tlscert": {}, "--tlskey": {},
	}
	composeValueFlags = map[string]struct{}{
		"-f": {}, "--file": {}, "-p": {}, "--project-name": {}, "--project-directory": {}, "--env-file": {},
		"--profile": {}, "--ansi": {}, "--progress": {}, "--parallel": {}, "-H": {}, "--host": {},
		"-c": {}, "--context": {}, "-l": {}, "--log-level": {}, "--config": {}, "--tlscacert": {},
		"--tlscert": {}, "--tlskey": {}, "--wait-timeout": {}, "-t": {}, "--timeout": {}, "--scale": {},
		"--exit-code-from": {},
	}
	pruneGroups = map[string]struct{}{"system": {}, "image": {}, "container": {}, "volume": {}, "network": {}, "builder": {}, "buildx": {}}
	opsPriority = map[string]int{"O4": 4, "O1": 3, "O2": 2, "O3": 1}
	opsReasons  = map[string]string{
		"O1": "Rule O1 (PM-2026-054, PM-2026-046): Confirm destructive Docker prune or volume/image/network removal because unconditional cleanup has swept live sidecars, image tags, and sibling resources.",
		"O2": "Rule O2 (PM-2026-028, PM-2026-054): Confirm container rm/stop/kill because removing or stopping running containers has left certificate renewers and sidecars absent.",
		"O3": "Rule O3 (PM-2026-001, PM-2026-005): Confirm docker compose up/down/rm/restart on the production project because compose lifecycle changes can recreate dependent stateful or GPU containers as a side effect.",
		"O4": "Rule O4 (PM-2026-031): Confirm docker compose up with --build or --force-recreate because the release path builds and gates images and a direct rebuild skips the contract gate.",
	}
)

const (
	opsUnreadable   = "Hook payload or command was unreadable and could not be analysed, so confirm execution manually before proceeding."
	opsDenyGuidance = "Do not retry or rephrase the command to get around the guard; instead tell the user which command is needed and why so the user can run it, or quote the flagged words if they are literal text such as a search pattern rather than a command."
	opsMaxDepth     = 32
)

func pickOps(current, candidate string) string {
	if candidate == "" {
		return current
	}
	if current == "" || opsPriority[candidate] > opsPriority[current] {
		return candidate
	}
	return current
}

func parseComposeArgs(args []string) (project string, hasProject bool, sub string, build bool) {
	i := 0
	for i < len(args) {
		tok := args[i]
		if tok == "--" {
			if sub != "" {
				break
			}
			if i+1 < len(args) {
				sub = args[i+1]
				i += 2
				continue
			}
			break
		}
		if tok == "-p" || tok == "--project-name" {
			hasProject = true
			if i+1 < len(args) {
				project = args[i+1]
				i += 2
				continue
			}
			project = ""
			i++
			continue
		}
		if strings.HasPrefix(tok, "--project-name=") || strings.HasPrefix(tok, "-p=") {
			hasProject = true
			project = strings.SplitN(tok, "=", 2)[1]
			i++
			continue
		}
		if _, ok := composeValueFlags[tok]; ok {
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "-") {
			if tok == "--build" || tok == "--force-recreate" {
				build = true
			}
			i++
			continue
		}
		if sub == "" {
			sub = tok
		}
		i++
	}
	return project, hasProject, sub, build
}

func classifyCompose(args []string, envVars map[string]string) string {
	project, hasProject, sub, build := parseComposeArgs(args)
	switch sub {
	case "up", "down", "rm", "stop", "kill", "restart":
	default:
		return ""
	}
	effective := ""
	if hasProject {
		effective = project
	} else {
		effective = envVars["COMPOSE_PROJECT_NAME"]
	}
	if strings.TrimSpace(effective) != "" && strings.TrimSpace(effective) != "alt" && !strings.Contains(effective, "$") {
		return ""
	}
	if sub == "stop" || sub == "kill" {
		return "O2"
	}
	if sub == "up" && build {
		return "O4"
	}
	return "O3"
}

func firstPositional(tokens []string, start int, valueFlags map[string]struct{}) (string, int) {
	i := start
	for i < len(tokens) {
		tok := tokens[i]
		if tok == "--" {
			if i+1 < len(tokens) {
				return tokens[i+1], i + 2
			}
			return "", len(tokens)
		}
		if _, ok := valueFlags[tok]; ok {
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "-") {
			i++
			continue
		}
		return tok, i + 1
	}
	return "", len(tokens)
}

func classifyDockerSegment(tokens []string, envVars map[string]string) string {
	if len(tokens) == 0 {
		return ""
	}
	exe := baseName(tokens[0])
	if exe == "docker-compose" {
		return classifyCompose(tokens[1:], envVars)
	}
	if exe != "docker" {
		return ""
	}
	sub, next := firstPositional(tokens, 1, dockerValueFlags)
	if sub == "" {
		return ""
	}
	if sub == "compose" {
		return classifyCompose(tokens[next:], envVars)
	}
	if sub == "rmi" {
		return "O1"
	}
	if sub == "rm" || sub == "stop" || sub == "kill" {
		return "O2"
	}
	action, _ := firstPositional(tokens, next, map[string]struct{}{})
	if _, ok := pruneGroups[sub]; ok && action == "prune" {
		return "O1"
	}
	if (sub == "volume" || sub == "image" || sub == "network") && (action == "rm" || action == "remove") {
		return "O1"
	}
	if sub == "container" && (action == "rm" || action == "remove" || action == "stop" || action == "kill") {
		return "O2"
	}
	return ""
}

func extractShellCScript(tokens []Token, start int) (string, bool) {
	i := start + 1
	for i < len(tokens) {
		tok := tokens[i].Text
		if tok == "--" {
			return "", false
		}
		if strings.HasPrefix(tok, "-") && !strings.HasPrefix(tok, "--") {
			if strings.Contains(tok[1:], "c") {
				if i+1 < len(tokens) {
					return tokens[i+1].Text, true
				}
				return "", false
			}
			i++
			continue
		}
		if strings.HasPrefix(tok, "--") {
			i++
			continue
		}
		return "", false
	}
	return "", false
}

func classifyOpsTokens(tokens []Token, depth int) (string, error) {
	envVars := map[string]string{}
	best := ""
	for idx, tok := range tokens {
		if envAssignRE.MatchString(tok.Text) {
			parts := strings.SplitN(tok.Text, "=", 2)
			envVars[parts[0]] = parts[1]
			continue
		}
		if tok.Quoted {
			continue
		}
		base := baseName(tok.Text)
		if base == "docker" || base == "docker-compose" {
			texts := make([]string, len(tokens)-idx)
			for i, t := range tokens[idx:] {
				texts[i] = t.Text
			}
			best = pickOps(best, classifyDockerSegment(texts, envVars))
		} else if isShellName(base) {
			script, ok := extractShellCScript(tokens, idx)
			if ok {
				rule, err := ClassifyOps(script, depth+1)
				if err != nil {
					return "", err
				}
				best = pickOps(best, rule)
			}
		}
	}
	return best, nil
}

// ClassifyOps returns O1..O4 or "" for a shell command.
func ClassifyOps(command string, depth int) (string, error) {
	if depth > opsMaxDepth {
		return "", fmt.Errorf("command nesting exceeded maximum recursion depth")
	}
	if strings.TrimSpace(command) == "" {
		return "", nil
	}
	best := ""
	for _, seg := range splitShellSegments(command) {
		rule, err := classifyOpsTokens(seg, depth)
		if err != nil {
			return "", err
		}
		best = pickOps(best, rule)
	}
	for _, sub := range extractSubstitutions(command) {
		rule, err := ClassifyOps(sub, depth+1)
		if err != nil {
			return "", err
		}
		best = pickOps(best, rule)
	}
	return best, nil
}

func classifyOpsArgv(argv []string) (string, error) {
	if len(argv) == 0 {
		return "", nil
	}
	if isShellName(baseName(argv[0])) {
		tokens := make([]Token, len(argv))
		for i, part := range argv {
			tokens[i] = Token{Text: part}
		}
		if script, ok := extractShellCScript(tokens, 0); ok {
			return ClassifyOps(script, 1)
		}
	}
	tokens := make([]Token, len(argv))
	for i, part := range argv {
		tokens[i] = Token{Text: part}
	}
	return classifyOpsTokens(tokens, 0)
}

func opsJSON(decision, reason string) string {
	payload := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       decision,
			"permissionDecisionReason": reason,
		},
	}
	return pythonJSON(payload) + "\n"
}

// RunOps is the docker/compose PreToolUse guard. It exits 0 and writes a JSON decision on match.
func RunOps(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	_ = ctx
	if stdout == nil {
		stdout = io.Discard
	}
	onMatch := "ask"
	for i := 0; i < len(args); i++ {
		if args[i] == "--on-match" && i+1 < len(args) {
			onMatch = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(args[i], "--on-match=") {
			onMatch = strings.TrimPrefix(args[i], "--on-match=")
		}
	}
	if onMatch != "ask" && onMatch != "deny" {
		onMatch = "ask"
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprint(stdout, opsJSON(onMatch, opsUnreadable))
		return 0
	}
	if !validUTF8(string(raw)) {
		fmt.Fprint(stdout, opsJSON(onMatch, opsUnreadable))
		return 0
	}
	cmdStr, cmdList, ignore, perr := parseShellHook(raw)
	if perr != nil {
		fmt.Fprint(stdout, opsJSON(onMatch, opsUnreadable))
		return 0
	}
	if ignore {
		return 0
	}
	var rule string
	var cerr error
	if cmdList != nil {
		rule, cerr = classifyOpsArgv(cmdList)
	} else {
		rule, cerr = ClassifyOps(cmdStr, 0)
	}
	if cerr != nil {
		fmt.Fprint(stdout, opsJSON(onMatch, opsUnreadable))
		return 0
	}
	if rule != "" {
		reason := opsReasons[rule]
		if onMatch == "deny" {
			reason = reason + " " + opsDenyGuidance
		}
		fmt.Fprint(stdout, opsJSON(onMatch, reason))
	}
	return 0
}

func parseShellHook(raw []byte) (string, []string, bool, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", nil, false, fmt.Errorf("invalid JSON")
	}
	tool, _ := payload["tool_name"].(string)
	if strings.TrimSpace(tool) == "" {
		return "", nil, false, fmt.Errorf("missing tool_name")
	}
	if tool != "Bash" && tool != "shell" {
		return "", nil, true, nil
	}
	input, ok := payload["tool_input"].(map[string]any)
	if !ok {
		return "", nil, false, fmt.Errorf("missing tool_input")
	}
	switch cmd := input["command"].(type) {
	case string:
		return cmd, nil, false, nil
	case []any:
		out := make([]string, 0, len(cmd))
		for _, part := range cmd {
			s, ok := part.(string)
			if !ok {
				return "", nil, false, fmt.Errorf("invalid command")
			}
			out = append(out, s)
		}
		return "", out, false, nil
	default:
		return "", nil, false, fmt.Errorf("invalid command")
	}
}

func pythonJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	var b strings.Builder
	inString := false
	escape := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		b.WriteByte(c)
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			continue
		}
		if c == ':' || c == ',' {
			b.WriteByte(' ')
		}
	}
	return b.String()
}
