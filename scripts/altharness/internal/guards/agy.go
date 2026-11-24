package guards

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Test seams for external command hooks in agy.go.
var (
	agyRunSelfcheckFn = defaultAgyRunSelfcheck
	agyRunPreflightFn = defaultAgyRunPreflight
	agyRunLayersFn    = defaultAgyRunLayers
	agyFormatFileFn   = defaultAgyFormatFile
)

// RunAgyHook implements `alt-harness agy-hook <session-start|pre-shell|pre-edit|post-edit|stop>`.
// Antigravity treats any non-zero exit code as a hook crash, so every hook subcommand
// emits valid JSON on stdout and returns exit code 0.
func RunAgyHook(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stdout, `{"decision":"deny","reason":"usage: alt-harness agy-hook <session-start|pre-shell|pre-edit|post-edit|stop>"}`)
		return 0
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "session-start":
		return runAgySessionStart(ctx, stdin, stdout)
	case "pre-shell":
		return runAgyPreShell(ctx, rest, stdin, stdout)
	case "pre-edit":
		return runAgyPreEdit(ctx, stdin, stdout)
	case "post-edit":
		return runAgyPostEdit(ctx, stdin, stdout)
	case "stop":
		return runAgyStop(ctx, stdin, stdout)
	default:
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("BLOCKED: unknown agy-hook subcommand %q.", sub),
		})
		return 0
	}
}

func writeAgyJSON(w io.Writer, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintln(w, `{"decision":"deny","reason":"BLOCKED: failed to encode hook response JSON."}`)
		return
	}
	fmt.Fprintln(w, string(raw))
}

func readAgyPayload(stdin io.Reader) (map[string]any, error) {
	if stdin == nil {
		return nil, fmt.Errorf("missing stdin")
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return nil, err
	}
	if !validUTF8(string(raw)) {
		return nil, fmt.Errorf("invalid UTF-8 stdin")
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON (%s)", err.Error())
	}
	if payload == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return payload, nil
}

func agyCwd(payload map[string]any) string {
	if s, ok := payload["cwd"].(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	if ws, ok := payload["workspacePaths"].([]any); ok {
		for _, item := range ws {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func agyRepoRoot(payload map[string]any) string {
	return ResolveRepoRoot("", agyCwd(payload))
}

func agySessionID(payload map[string]any) string {
	for _, key := range []string{"conversationId", "session_id"} {
		if s, ok := payload[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return "default"
}

func agyToolCall(payload map[string]any) (toolName string, args map[string]any, ok bool) {
	if tc, exists := payload["toolCall"].(map[string]any); exists {
		name, _ := tc["name"].(string)
		a, _ := tc["args"].(map[string]any)
		if strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name), a, true
		}
	}
	if name, exists := payload["tool_name"].(string); exists && strings.TrimSpace(name) != "" {
		a, _ := payload["tool_input"].(map[string]any)
		return strings.TrimSpace(name), a, true
	}
	return "", nil, false
}

func runAgySessionStart(ctx context.Context, stdin io.Reader, stdout io.Writer) int {
	payload, _ := readAgyPayload(stdin)
	if payload == nil {
		payload = map[string]any{}
	}
	repo := agyRepoRoot(payload)
	cwd := agyCwd(payload)
	if cwd == "" {
		cwd = repo
	}
	sessionID := agySessionID(payload)
	eventName := stringOr(payload["hookEventName"], "SessionStart")

	var msgs []string
	preflightPayload := map[string]any{
		"hook_event_name": eventName,
		"session_id":      sessionID,
		"cwd":             cwd,
	}
	if out, _, _ := agyRunPreflightFn(ctx, repo, preflightPayload); strings.TrimSpace(out) != "" {
		msgs = append(msgs, strings.TrimSpace(out))
	}
	if scOut := strings.TrimSpace(agyRunSelfcheckFn(ctx, repo)); scOut != "" {
		msgs = append(msgs, scOut)
	}
	if len(msgs) == 0 {
		writeAgyJSON(stdout, map[string]any{"injectSteps": []any{}})
		return 0
	}
	writeAgyJSON(stdout, map[string]any{
		"injectSteps": []any{
			map[string]any{
				"systemMessage": map[string]any{
					"systemMessage": strings.Join(msgs, "\n"),
				},
			},
		},
	})
	return 0
}

func runAgyPreShell(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) int {
	_ = ctx
	onMatch := "ask"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--on-match":
			if i+1 >= len(args) {
				writeAgyJSON(stdout, map[string]any{
					"decision": "deny",
					"reason":   "BLOCKED: --on-match requires a value (ask or deny).",
				})
				return 0
			}
			onMatch = args[i+1]
			i++
		case strings.HasPrefix(arg, "--on-match="):
			onMatch = strings.TrimPrefix(arg, "--on-match=")
		default:
			writeAgyJSON(stdout, map[string]any{
				"decision": "deny",
				"reason":   fmt.Sprintf("BLOCKED: unknown agy-hook pre-shell flag %q.", arg),
			})
			return 0
		}
	}
	if onMatch != "ask" && onMatch != "deny" {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("BLOCKED: invalid --on-match value %q (expected ask or deny).", onMatch),
		})
		return 0
	}

	payload, err := readAgyPayload(stdin)
	if err != nil {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("BLOCKED: hook payload could not be parsed (%v).", err),
		})
		return 0
	}
	toolName, toolArgs, ok := agyToolCall(payload)
	if !ok {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   "BLOCKED: hook payload missing toolCall/tool_name.",
		})
		return 0
	}
	if toolName != "run_command" && toolName != "Bash" && toolName != "shell" {
		writeAgyJSON(stdout, map[string]any{"decision": "allow"})
		return 0
	}
	if toolArgs == nil {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("BLOCKED: missing tool args for %s.", toolName),
		})
		return 0
	}

	cmdStr, cmdList, cmdOK := extractAgyCommand(toolArgs)
	if !cmdOK {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("BLOCKED: missing or invalid command for %s.", toolName),
		})
		return 0
	}
	isDaemon, _ := toolArgs["IsDaemon"].(bool)

	// 1. Hard-block git/gh push-class commands.
	var pushHit string
	var pushErr error
	if cmdList != nil {
		pushHit, pushErr = CheckPushArgv(cmdList)
	} else {
		pushHit, pushErr = CheckPushCommand(cmdStr, 0)
	}
	if pushErr != nil {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("BLOCKED: shell command analysis failed (%v).", pushErr),
		})
		return 0
	}
	if pushHit != "" {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   pushBlockMessage,
		})
		return 0
	}

	// 2. Ops guard (O1..O4).
	var opsRule string
	var opsErr error
	if cmdList != nil {
		opsRule, opsErr = classifyOpsArgv(cmdList)
	} else {
		opsRule, opsErr = ClassifyOps(cmdStr, 0)
	}
	if opsErr != nil {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   opsUnreadable,
		})
		return 0
	}
	if opsRule != "" {
		reason := opsReasons[opsRule]
		if onMatch == "deny" {
			reason = reason + " " + opsDenyGuidance
		}
		writeAgyJSON(stdout, map[string]any{
			"decision": onMatch,
			"reason":   reason,
		})
		return 0
	}

	// 3. EXEC-01 guard (E1/E2/E3 + IsDaemon background check).
	repo := agyRepoRoot(payload)
	packet, pktErr := LoadActivePacket(repo, "")
	if pktErr != "" {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("BLOCKED [EXEC-01]: %s.", pktErr),
		})
		return 0
	}
	isWorker := nonEmptyString(payload["agent_id"]) || nonEmptyString(payload["subagent_type"]) || strings.EqualFold(strings.TrimSpace(os.Getenv("ALT_HARNESS_ROLE")), "worker")
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
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("BLOCKED [EXEC-01]: command analysis failed (%v).", cerr),
		})
		return 0
	}
	if enforceE3 && isDaemon {
		verdict = pickHigher(verdict, &execVerdict{"E3", "IsDaemon=true"})
	}
	if verdict != nil {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   strings.TrimSpace(formatExecBlock(verdict.rule, verdict.detail)),
		})
		return 0
	}
	writeAgyJSON(stdout, map[string]any{"decision": "allow"})
	return 0
}

func extractAgyCommand(args map[string]any) (string, []string, bool) {
	if raw, ok := args["CommandLine"]; ok {
		if s, ok := raw.(string); ok {
			return s, nil, true
		}
		return "", nil, false
	}
	if raw, ok := args["command"]; ok {
		switch v := raw.(type) {
		case string:
			return v, nil, true
		case []any:
			out := make([]string, 0, len(v))
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return "", nil, false
				}
				out = append(out, s)
			}
			return "", out, true
		}
	}
	return "", nil, false
}

var watchedAgyEditTools = map[string]struct{}{
	"write_file":                 {},
	"edit_file":                  {},
	"write_to_file":              {},
	"replace_file_content":       {},
	"multi_replace_file_content": {},
	"Edit":                       {},
	"Write":                      {},
	"NotebookEdit":               {},
	"apply_patch":                {},
}

func extractAgyEditTargets(toolName string, args map[string]any) ([]string, map[string]string, error) {
	if args == nil {
		return nil, nil, fmt.Errorf("missing tool args for %s", toolName)
	}
	var ordered []string
	seen := map[string]struct{}{}
	added := map[string]string{}
	addTarget := func(path, text string) {
		p := strings.TrimSpace(path)
		if p == "" {
			return
		}
		appendUnique(&ordered, seen, p)
		if prev, ok := added[p]; ok && prev != "" && text != "" {
			added[p] = prev + "\n" + text
		} else if !ok || text != "" {
			added[p] = text
		}
	}

	switch toolName {
	case "write_file":
		if p, ok := args["Path"].(string); ok {
			content, _ := args["Content"].(string)
			addTarget(p, content)
		}
	case "edit_file":
		if p, ok := args["Path"].(string); ok {
			rep, _ := args["Replacement"].(string)
			addTarget(p, rep)
		}
	case "write_to_file":
		if p, ok := args["TargetFile"].(string); ok {
			code, _ := args["CodeContent"].(string)
			addTarget(p, code)
		}
	case "replace_file_content":
		if p, ok := args["TargetFile"].(string); ok {
			rep, _ := args["ReplacementContent"].(string)
			addTarget(p, rep)
		}
	case "multi_replace_file_content":
		if p, ok := args["TargetFile"].(string); ok {
			var chunks []string
			if rawChunks, ok := args["ReplacementChunks"].([]any); ok {
				for _, item := range rawChunks {
					if m, ok := item.(map[string]any); ok {
						if rep, ok := m["ReplacementContent"].(string); ok && rep != "" {
							chunks = append(chunks, rep)
						}
					}
				}
			}
			addTarget(p, strings.Join(chunks, "\n"))
		}
	case "Edit", "Write", "NotebookEdit":
		text := addedTextForClaude(toolName, args)
		for _, field := range []string{"file_path", "notebook_path"} {
			if p, ok := args[field].(string); ok {
				addTarget(p, text)
			}
		}
	case "apply_patch":
		collectApplyPatch(args, &ordered, seen, added)
	}
	if len(ordered) == 0 {
		return nil, nil, fmt.Errorf("%s yielded no target path", toolName)
	}
	return ordered, added, nil
}

func isInsideArtifactDir(rawPath, cwd, artifactDir string) bool {
	artTrim := strings.TrimSpace(artifactDir)
	if artTrim == "" {
		return false
	}
	artResolved := resolveExisting(artTrim)
	if artResolved == "" {
		return false
	}
	candidate := rawPath
	if !filepath.IsAbs(candidate) {
		base := strings.TrimSpace(cwd)
		if base == "" {
			if wd, err := os.Getwd(); err == nil {
				base = wd
			}
		}
		if base != "" {
			candidate = filepath.Join(base, candidate)
		}
	}
	resolved, err := resolveNonStrict(candidate)
	if err != nil || resolved == "" {
		return false
	}
	return pathInside(resolved, artResolved)
}

func runAgyPreEdit(ctx context.Context, stdin io.Reader, stdout io.Writer) int {
	payload, err := readAgyPayload(stdin)
	if err != nil {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("BLOCKED: guard could not read the hook payload (%v).", err),
		})
		return 0
	}
	toolName, toolArgs, ok := agyToolCall(payload)
	if !ok {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   "BLOCKED: guard could not read the hook payload: missing toolCall/tool_name.",
		})
		return 0
	}
	if _, watch := watchedAgyEditTools[toolName]; !watch {
		writeAgyJSON(stdout, map[string]any{"decision": "allow"})
		return 0
	}
	paths, addedText, err := extractAgyEditTargets(toolName, toolArgs)
	if err != nil {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("BLOCKED: guard could not read the hook payload: %v.", err),
		})
		return 0
	}

	cwd := agyCwd(payload)
	repo := agyRepoRoot(payload)
	artifactDir, _ := payload["artifactDirectoryPath"].(string)

	// 1. Protected paths (R-01) — always enforced, even inside artifactDirectoryPath.
	for _, p := range paths {
		if msg := CheckProtectedPathInCwd(p, cwd); msg != "" {
			writeAgyJSON(stdout, map[string]any{"decision": "deny", "reason": msg})
			return 0
		}
	}

	// 2. Projector event-time invariants.
	for _, p := range paths {
		if !IsGuardedPathInCwd(p, cwd) {
			continue
		}
		if call := FindEventTimeViolation(addedText[p]); call != "" {
			msg := fmt.Sprintf("Blocked edit to %s: found '%s' in projector code. Projectors must be reproject-safe: derive time from the event's OccurredAt and read the version id carried in the event payload (see ADR-000468 and ADR-000447).", p, call)
			writeAgyJSON(stdout, map[string]any{"decision": "deny", "reason": msg})
			return 0
		}
	}

	// 3. Packet scope & generated-code guard (EXEC-02), exempting brain artifacts.
	packet, pktErr := LoadActivePacket(repo, "")
	if pktErr != "" {
		writeAgyJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("BLOCKED [EXEC-02]: %s.", pktErr),
		})
		return 0
	}
	for _, p := range paths {
		if isInsideArtifactDir(p, cwd, artifactDir) {
			continue
		}
		if msg := checkScopePath(p, repo, packet, cwd); msg != "" {
			writeAgyJSON(stdout, map[string]any{"decision": "deny", "reason": msg})
			return 0
		}
	}

	// 4. ADR preflight hook (first edit per scope held once with receipt).
	sessionID := agySessionID(payload)
	agentID, _ := payload["agent_id"].(string)
	effectiveCwd := cwd
	if effectiveCwd == "" {
		effectiveCwd = repo
	}
	for _, p := range paths {
		if isInsideArtifactDir(p, cwd, artifactDir) {
			continue
		}
		pfPayload := map[string]any{
			"hook_event_name": "PreToolUse",
			"session_id":      sessionID,
			"cwd":             effectiveCwd,
			"tool_name":       "Write",
			"tool_input":      map[string]any{"file_path": p},
		}
		if strings.TrimSpace(agentID) != "" {
			pfPayload["agent_id"] = agentID
		}
		_, pfStderr, pfCode := agyRunPreflightFn(ctx, repo, pfPayload)
		if pfCode == 2 {
			reason := strings.TrimSpace(pfStderr)
			if reason == "" {
				reason = "ADR PREFLIGHT: edit held; review governing ADRs and retry."
			}
			writeAgyJSON(stdout, map[string]any{"decision": "deny", "reason": reason})
			return 0
		}
	}

	writeAgyJSON(stdout, map[string]any{"decision": "allow"})
	return 0
}

func runAgyPostEdit(ctx context.Context, stdin io.Reader, stdout io.Writer) int {
	payload, err := readAgyPayload(stdin)
	if err != nil {
		fmt.Fprintln(stdout, "{}")
		return 0
	}
	toolName, toolArgs, ok := agyToolCall(payload)
	if !ok {
		fmt.Fprintln(stdout, "{}")
		return 0
	}
	if _, watch := watchedAgyEditTools[toolName]; !watch {
		fmt.Fprintln(stdout, "{}")
		return 0
	}
	paths, _, err := extractAgyEditTargets(toolName, toolArgs)
	if err != nil {
		fmt.Fprintln(stdout, "{}")
		return 0
	}
	cwd := agyCwd(payload)
	repo := resolveExisting(agyRepoRoot(payload))
	for _, rawPath := range paths {
		candidate := rawPath
		if !filepath.IsAbs(candidate) {
			base := cwd
			if base == "" {
				base = repo
			}
			candidate = filepath.Join(base, candidate)
		}
		resolved, err := resolveNonStrict(candidate)
		if err != nil || resolved == "" {
			continue
		}
		if repo != "" && !pathInside(resolved, repo) {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		agyFormatFileFn(ctx, repo, resolved)
	}
	fmt.Fprintln(stdout, "{}")
	return 0
}

func runAgyStop(ctx context.Context, stdin io.Reader, stdout io.Writer) int {
	payload, _ := readAgyPayload(stdin)
	if payload == nil {
		payload = map[string]any{}
	}
	repo := agyRepoRoot(payload)
	cwd := agyCwd(payload)
	if cwd == "" {
		cwd = repo
	}
	continuationActive, _ := payload["continuationActive"].(bool)
	terminationReason, _ := payload["terminationReason"].(string)

	// Record run telemetry if .harness/runs/<branch>/active-packet.json exists.
	if runDir, err := RunDirForBranch(repo, ""); err == nil {
		if st, err := os.Stat(filepath.Join(runDir, activePacketFilename)); err == nil && st.Mode().IsRegular() {
			record := map[string]any{
				"ts":                     utcNow(),
				"hook_event_name":        stringOr(payload["hookEventName"], "Stop"),
				"session_id":             agySessionID(payload),
				"agent_id":               stringOr(payload["agent_id"], "agy"),
				"subagent_type":          "antigravity",
				"agent_transcript_path":  stringOr(payload["transcriptPath"], ""),
				"cwd":                    cwd,
				"termination_reason":     terminationReason,
				"continuation_active":    continuationActive,
				"last_assistant_message": "",
			}
			_ = appendSubagentRecord(repo, "", record)
		}
	}

	if !continuationActive && terminationReason == "NO_TOOL_CALL" {
		if code, output := agyRunLayersFn(ctx, repo); code != 0 {
			reason := strings.TrimSpace(output)
			if reason == "" {
				reason = "Clean Architecture layer check failed (check_layers.sh --changed); fix violations before stopping."
			}
			writeAgyJSON(stdout, map[string]any{
				"decision": "continue",
				"reason":   reason,
			})
			return 0
		}
	}

	writeAgyJSON(stdout, map[string]any{"decision": "allow"})
	return 0
}

func defaultAgyRunSelfcheck(ctx context.Context, repoRoot string) string {
	var cmd *exec.Cmd
	if exe, err := os.Executable(); err == nil && filepath.Base(exe) == "alt-harness" {
		cmd = exec.CommandContext(ctx, exe, "selfcheck", "--hook", "--root", repoRoot)
	} else {
		launcher := filepath.Join(repoRoot, "scripts", "altharness", "alt-harness")
		if info, err := os.Stat(launcher); err == nil && !info.IsDir() {
			cmd = exec.CommandContext(ctx, launcher, "selfcheck", "--hook", "--root", repoRoot)
		}
	}
	if cmd == nil {
		return ""
	}
	cmd.Dir = repoRoot
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

// PreflightRunner optionally wires `alt-harness preflight` in-process so agy-hook
// can invoke ADR preflight without spawning a subprocess.
var PreflightRunner func(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int

func defaultAgyRunPreflight(ctx context.Context, repoRoot string, payload map[string]any) (string, string, int) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", "", 0
	}
	if PreflightRunner != nil {
		var outBuf, errBuf bytes.Buffer
		code := PreflightRunner(ctx, []string{"--hook"}, bytes.NewReader(encoded), &outBuf, &errBuf)
		return outBuf.String(), errBuf.String(), code
	}
	var cmd *exec.Cmd
	if exe, err := os.Executable(); err == nil && filepath.Base(exe) == "alt-harness" {
		cmd = exec.CommandContext(ctx, exe, "preflight", "--hook")
	} else {
		launcher := filepath.Join(repoRoot, "scripts", "altharness", "alt-harness")
		if info, err := os.Stat(launcher); err == nil && !info.IsDir() {
			cmd = exec.CommandContext(ctx, launcher, "preflight", "--hook")
		}
	}
	if cmd == nil {
		return "", "", 0
	}
	cmd.Dir = repoRoot
	cmd.Stdin = bytes.NewReader(encoded)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		} else {
			code = 1
		}
	}
	return outBuf.String(), errBuf.String(), code
}

func defaultAgyRunLayers(ctx context.Context, repoRoot string) (int, string) {
	script := filepath.Join(repoRoot, ".agents", "skills", "clean-architecture", "scripts", "check_layers.sh")
	if info, err := os.Stat(script); err != nil || info.IsDir() {
		return 0, ""
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		return 0, ""
	}
	cmd := exec.CommandContext(ctx, bash, script, "--changed")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	code := 1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	return code, string(out)
}

func defaultAgyFormatFile(ctx context.Context, repoRoot, filePath string) {
	ext := strings.ToLower(filepath.Ext(filePath))
	var cmd *exec.Cmd
	switch ext {
	case ".go":
		if bin, err := exec.LookPath("gofmt"); err == nil {
			cmd = exec.CommandContext(ctx, bin, "-w", filePath)
		}
	case ".py":
		if bin, err := exec.LookPath("uv"); err == nil {
			cmd = exec.CommandContext(ctx, bin, "run", "--no-sync", "ruff", "format", "--quiet", filePath)
		}
	case ".ts", ".tsx", ".svelte":
		if bin, err := exec.LookPath("npx"); err == nil {
			cmd = exec.CommandContext(ctx, bin, "--no-install", "biome", "format", "--write", filePath)
		}
	}
	if cmd == nil {
		return
	}
	if repoRoot != "" {
		cmd.Dir = repoRoot
	}
	_ = cmd.Run()
}
