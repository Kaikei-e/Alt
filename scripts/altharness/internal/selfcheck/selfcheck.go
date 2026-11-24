// Package selfcheck ports scripts/agent-harness/harness_selfcheck.py.
package selfcheck

import (
	"alt/scripts/altharness/internal/guards"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxClaudeMDLines = 200
	maxAgentsMDBytes = 32768
	failurePatterns  = ".claude/skills/failure-pattern-review/references/patterns.md"
	failureSkill     = ".claude/skills/failure-pattern-review/SKILL.md"
	ruleLedgerRel    = ".agents/rules/rule-ledger.md"
	ratchetRel       = "scripts/lint/ratchet/suppressions.toml"
	astGrepRulesRel  = "scripts/lint/ast-grep/rules"
)

var requiredLeadins = []string{
	"**機構.**", "**先例.**", "**問い.**", "**確認コマンド.**", "**機械的に見ているもの.**",
}

var (
	badEnvVars      = []string{"CLAUDE_TOOL_INPUT", "CLAUDE_FILE_PATHS"}
	repoScriptRefRE = regexp.MustCompile(`(?:\$CLAUDE_PROJECT_DIR|\$\{CLAUDE_PROJECT_DIR\}|\$\(git rev-parse --show-toplevel\))/([^\s'"]+)`)
	sharedMarkerRE  = regexp.MustCompile(`^\s*<!--\s*shared:([A-Za-z0-9_.-]+):(start|end)\s*-->\s*$`)
	fpHeadingRE     = regexp.MustCompile(`^##\s+FP-(\d{2})\s+(.+?)\s*$`)
	h2RE            = regexp.MustCompile(`^##(?:\s|$)`)
	leadinBreakRE   = regexp.MustCompile(`\*\*(?:機構|先例|問い|確認コマンド|機械的に見ているもの)\.\*\*`)
	backtickRE      = regexp.MustCompile("`([^`\n]+)`")
	maxJudgmentFMRE = regexp.MustCompile(`^\s*max_judgment_rules\s*:\s*([+-]?\d+)\s*$`)
	maxJudgmentCMRE = regexp.MustCompile(`<!--\s*max_judgment_rules\s*:\s*([+-]?\d+)\s*-->`)
	astGrepIDRE     = regexp.MustCompile(`^\s*id\s*:\s*([A-Za-z0-9_.-]+)\s*$`)
	sepRowRE        = regexp.MustCompile(`^\|\s*:?-{2,}`)
	yamlItemRE      = regexp.MustCompile(`^\s*-\s+(.*)$`)
	pathsKeyRE      = regexp.MustCompile(`^paths\s*:`)
	fileLikeExts    = []string{".py", ".sh", ".yml", ".yaml", ".toml", ".md"}
	ledgerHeaders   = []string{"ID", "規則", "強制手段", "由来"}
)

// Finding is one self-check failure.
type Finding struct {
	ID, Path, Message string
}

// Run is the harness_selfcheck.py CLI.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		fmt.Fprintln(stderr, "error: nil context")
		return 1
	}
	root := ""
	hook := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--root":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "error: --root requires a value")
				return 2
			}
			i++
			root = args[i]
		case "--hook":
			hook = true
		case "-h", "--help":
			fmt.Fprintln(stdout, "usage: selfcheck [--root dir] [--hook]")
			return 0
		default:
			fmt.Fprintf(stderr, "error: unrecognized arguments: %s\n", args[i])
			return 2
		}
	}
	if root == "" {
		root = defaultRoot(ctx)
	} else if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	findings, meta := runSelfcheck(ctx, root)
	if hook {
		if len(findings) > 0 {
			ids := make([]string, len(findings))
			for i, f := range findings {
				ids[i] = f.ID
			}
			fmt.Fprintf(stdout, "agent_harness_selfcheck_FAILED: %d finding(s): %s run python3 scripts/agent-harness/harness_selfcheck.py\n", len(findings), strings.Join(ids, ", "))
		} else {
			fmt.Fprintf(stdout, "agent_harness_selfcheck: OK (codex_hooks=%v, claude_hooks=%v, agy_hooks=%v, guarded_globs=%v, shared_blocks=%v)\n", meta["codex_hooks"], meta["claude_hooks"], meta["agy_hooks"], meta["guarded_globs"], meta["shared_blocks"])
		}
		return 0
	}
	if present, _ := meta["claude_present"].(bool); !present {
		fmt.Fprintln(stdout, "NOTE: Claude hooks were not checked because .claude/settings.json is absent.")
	}
	if len(findings) > 0 {
		for _, f := range findings {
			fmt.Fprintf(stdout, "%s %s: %s\n", f.ID, f.Path, f.Message)
		}
		return 1
	}
	fmt.Fprintf(stdout, "agent_harness_selfcheck: OK (codex_hooks=%v, claude_hooks=%v, agy_hooks=%v, guarded_globs=%v, shared_blocks=%v)\n", meta["codex_hooks"], meta["claude_hooks"], meta["agy_hooks"], meta["guarded_globs"], meta["shared_blocks"])
	return 0
}

func defaultRoot(ctx context.Context) string {
	git, err := exec.LookPath("git")
	if err == nil {
		cmd := exec.CommandContext(ctx, git, "rev-parse", "--show-toplevel")
		if out, err := cmd.Output(); err == nil {
			if s := strings.TrimSpace(string(out)); s != "" {
				return s
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

func runSelfcheck(ctx context.Context, root string) ([]Finding, map[string]any) {
	var findings []Finding
	tracked := gitLsFiles(ctx, root)
	claudePresent, codexHooks, claudeHooks, agyHooks := checkHooksPair(root, &findings)
	nonTest := make([]string, 0, len(tracked))
	for _, f := range tracked {
		if !strings.HasSuffix(f, "_test.go") {
			nonTest = append(nonTest, f)
		}
	}
	for _, pat := range guards.GuardedGlobs {
		rx := globToRegex(pat)
		hit := false
		for _, f := range nonTest {
			if rx.MatchString(f) {
				hit = true
				break
			}
		}
		if !hit {
			findings = append(findings, Finding{"H5", "scripts/agent-harness/guard_event_time.py", fmt.Sprintf("GUARDED_GLOBS entry '%s' matches no tracked non-test file", pat)})
		}
	}
	rulesDir := filepath.Join(root, ".claude", "rules")
	if st, err := os.Stat(rulesDir); err == nil && st.IsDir() {
		_ = filepath.Walk(rulesDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			text, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			for _, pat := range extractFrontmatterPaths(string(text)) {
				if !matchesRuleGlob(pat, tracked) {
					findings = append(findings, Finding{"H6", rel, fmt.Sprintf("paths entry '%s' matches no tracked file", pat)})
				}
			}
			return nil
		})
	}
	shared := checkSharedBlocks(root, &findings)
	checkDocSizes(root, &findings)
	checkFailurePatterns(root, &findings)
	checkRuleLedger(root, &findings)
	meta := map[string]any{
		"claude_present": claudePresent,
		"codex_hooks":    codexHooks,
		"claude_hooks":   claudeHooks,
		"agy_hooks":      agyHooks,
		"guarded_globs":  len(guards.GuardedGlobs),
		"shared_blocks":  shared,
	}
	return findings, meta
}

func gitLsFiles(ctx context.Context, root string) []string {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, git, "-C", root, "ls-files", "-z")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range strings.Split(string(out), "\x00") {
		if entry != "" {
			files = append(files, entry)
		}
	}
	return files
}

func checkHooksPair(root string, findings *[]Finding) (bool, int, any, int) {
	codexPath := filepath.Join(root, ".codex", "hooks.json")
	codexCount := 0
	if st, err := os.Stat(codexPath); err == nil && st.Mode().IsRegular() {
		if cfg := loadHookConfig(codexPath, ".codex/hooks.json", findings); cfg != nil {
			codexCount = checkHookFile(root, ".codex/hooks.json", cfg, false, findings)
		}
	} else {
		*findings = append(*findings, Finding{"H4", ".codex/hooks.json", ".codex/hooks.json is missing"})
	}
	claudePath := filepath.Join(root, ".claude", "settings.json")
	present := false
	var claudeHooks any = "absent"
	if st, err := os.Stat(claudePath); err == nil && st.Mode().IsRegular() {
		present = true
		if cfg := loadHookConfig(claudePath, ".claude/settings.json", findings); cfg != nil {
			claudeHooks = checkHookFile(root, ".claude/settings.json", cfg, true, findings)
		}
	}
	agyPath := filepath.Join(root, ".agents", "hooks.json")
	agyCount := 0
	if st, err := os.Stat(agyPath); err == nil && st.Mode().IsRegular() {
		if cfg := loadHookConfig(agyPath, ".agents/hooks.json", findings); cfg != nil {
			agyCount = checkAgyHookFile(root, ".agents/hooks.json", cfg, findings)
		}
	} else {
		*findings = append(*findings, Finding{"H4", ".agents/hooks.json", ".agents/hooks.json is missing"})
	}
	return present, codexCount, claudeHooks, agyCount
}

var (
	agyFlatEvents    = map[string]bool{"SessionStart": true, "Stop": true, "PreInvocation": true, "PostInvocation": true}
	agyGroupedEvents = map[string]bool{"PreToolUse": true, "PostToolUse": true}
)

func checkAgyHookFile(root, label string, cfg map[string]any, findings *[]Finding) int {
	if _, hasWrongRoot := cfg["hooks"]; hasWrongRoot {
		*findings = append(*findings, Finding{"H4", label, `Antigravity hooks.json must use named top-level hook keys, not a "hooks" wrapper`})
		return 0
	}
	total := 0
	var hasSessionStart, hasPreShell, hasPreEdit, hasPostEdit, hasStop bool

	names := make([]string, 0, len(cfg))
	for k := range cfg {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, name := range names {
		spec, ok := cfg[name].(map[string]any)
		if !ok {
			*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("top-level hook %q must be a JSON object", name)})
			continue
		}
		if enabled, ok := spec["enabled"].(bool); ok && !enabled {
			continue
		}
		for event, rawEntries := range spec {
			if event == "enabled" {
				continue
			}
			if agyFlatEvents[event] {
				list, ok := rawEntries.([]any)
				if !ok {
					*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("%s.%s must be a flat array of handler objects", name, event)})
					continue
				}
				for _, item := range list {
					h, ok := item.(map[string]any)
					if !ok {
						*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("%s.%s entry must be a handler object", name, event)})
						continue
					}
					if _, nested := h["hooks"]; nested {
						*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("%s.%s must be flat handler objects, not grouped {matcher, hooks}", name, event)})
						continue
					}
					cmd, ok := h["command"].(string)
					if !ok || strings.TrimSpace(cmd) == "" {
						*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("%s.%s handler missing non-empty command", name, event)})
						continue
					}
					if t, hasType := h["type"].(string); hasType && t != "" && t != "command" {
						continue
					}
					total++
					checkHookCommandRefs(root, label, cmd, findings)
					if event == "SessionStart" && strings.Contains(cmd, "agy_hook.py") && strings.Contains(cmd, "session-start") {
						hasSessionStart = true
					}
					if event == "Stop" && strings.Contains(cmd, "agy_hook.py") && strings.Contains(cmd, "stop") {
						hasStop = true
					}
				}
				continue
			}
			if agyGroupedEvents[event] {
				list, ok := rawEntries.([]any)
				if !ok {
					*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("%s.%s must be a grouped array of {matcher, hooks} objects", name, event)})
					continue
				}
				for _, item := range list {
					grp, ok := item.(map[string]any)
					if !ok {
						*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("%s.%s entry must be an object", name, event)})
						continue
					}
					nested, ok := grp["hooks"].([]any)
					if !ok {
						*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("%s.%s entry must have a 'hooks' array (grouped format)", name, event)})
						continue
					}
					matcher := grp["matcher"]
					for _, hRaw := range nested {
						h, ok := hRaw.(map[string]any)
						if !ok {
							*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("%s.%s nested hook must be an object", name, event)})
							continue
						}
						cmd, ok := h["command"].(string)
						if !ok || strings.TrimSpace(cmd) == "" {
							*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("%s.%s handler missing non-empty command", name, event)})
							continue
						}
						if t, hasType := h["type"].(string); hasType && t != "" && t != "command" {
							continue
						}
						total++
						checkHookCommandRefs(root, label, cmd, findings)
						if event == "PreToolUse" && strings.Contains(cmd, "agy_hook.py") {
							if strings.Contains(cmd, "pre-shell") && matcherMatches(matcher, "run_command") {
								hasPreShell = true
							}
							if strings.Contains(cmd, "pre-edit") && matcherMatches(matcher, "write_file") && matcherMatches(matcher, "edit_file") {
								hasPreEdit = true
							}
						}
						if event == "PostToolUse" && strings.Contains(cmd, "agy_hook.py") {
							if strings.Contains(cmd, "post-edit") && matcherMatches(matcher, "write_file") && matcherMatches(matcher, "edit_file") {
								hasPostEdit = true
							}
						}
					}
				}
				continue
			}
			*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("%s contains unsupported event type %q", name, event)})
		}
	}

	if !hasSessionStart {
		*findings = append(*findings, Finding{"H4", label, `SessionStart hooks do not reference agy_hook.py session-start`})
	}
	if !hasPreShell {
		*findings = append(*findings, Finding{"H4", label, `PreToolUse hooks do not reference agy_hook.py pre-shell with a matcher matching "run_command"`})
	}
	if !hasPreEdit {
		*findings = append(*findings, Finding{"H4", label, `PreToolUse hooks do not reference agy_hook.py pre-edit with a matcher matching "write_file" and "edit_file"`})
	}
	if !hasPostEdit {
		*findings = append(*findings, Finding{"H4", label, `PostToolUse hooks do not reference agy_hook.py post-edit with a matcher matching "write_file" and "edit_file"`})
	}
	if !hasStop {
		*findings = append(*findings, Finding{"H4", label, `Stop hooks do not reference agy_hook.py stop`})
	}
	return total
}

func checkHookCommandRefs(root, label, cmd string, findings *[]Finding) {
	for _, env := range badEnvVars {
		if strings.Contains(cmd, env) {
			*findings = append(*findings, Finding{"H1", label, fmt.Sprintf("hook command references non-existent %s; read JSON from stdin instead", env)})
		}
	}
	for _, m := range repoScriptRefRE.FindAllStringSubmatch(cmd, -1) {
		if len(m) < 2 {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, m[1])); err != nil {
			*findings = append(*findings, Finding{"H2", label, fmt.Sprintf("hook command references missing repo script '%s'", m[1])})
		}
	}
}

func loadHookConfig(path, label string, findings *[]Finding) map[string]any {
	raw, err := os.ReadFile(path)
	if err != nil {
		*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("could not parse hook config: %v", err)})
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		*findings = append(*findings, Finding{"H4", label, fmt.Sprintf("could not parse hook config: %v", err)})
		return nil
	}
	if data == nil {
		*findings = append(*findings, Finding{"H4", label, "hook config must be a JSON object"})
		return nil
	}
	return data
}

func checkHookFile(root, label string, cfg map[string]any, isClaude bool, findings *[]Finding) int {
	hooksMap, ok := cfg["hooks"].(map[string]any)
	if !ok {
		*findings = append(*findings, Finding{"H4", label, "missing 'hooks' object"})
		return 0
	}
	total := 0
	var pre []string
	bashOps, bashExec := false, false
	for event, entries := range hooksMap {
		for _, pair := range iterHooks(entries) {
			hook := pair.hook
			if hook["type"] != "command" {
				continue
			}
			total++
			cmd, _ := hook["command"].(string)
			if event == "PreToolUse" {
				pre = append(pre, cmd)
				if isClaude && hook["onFailure"] != "block" {
					*findings = append(*findings, Finding{"H3", label, `PreToolUse command hook lacks "onFailure": "block"`})
				}
				if strings.Contains(cmd, "|| exit 0") || strings.Contains(cmd, "|| true") {
					*findings = append(*findings, Finding{"H3", label, `PreToolUse command hook swallows failure with "|| exit 0" or "|| true" (fail-open)`})
				}
				if strings.Contains(cmd, "guard_ops_commands.py") {
					if !isClaude && !strings.Contains(cmd, "--on-match deny") {
						*findings = append(*findings, Finding{"H4", label, `PreToolUse guard_ops_commands.py entry must pass "--on-match deny" because Codex does not support the ask decision and the call would go through`})
					}
					if matcherMatches(pair.matcher, "Bash") {
						bashOps = true
					}
				}
				if strings.Contains(cmd, "guard_exec_commands.py") && matcherMatches(pair.matcher, "Bash") {
					bashExec = true
				}
			}
			for _, env := range badEnvVars {
				if strings.Contains(cmd, env) {
					*findings = append(*findings, Finding{"H1", label, fmt.Sprintf("hook command references non-existent %s; read JSON from stdin instead", env)})
				}
			}
			for _, m := range repoScriptRefRE.FindAllStringSubmatch(cmd, -1) {
				if len(m) < 2 {
					continue
				}
				if _, err := os.Stat(filepath.Join(root, m[1])); err != nil {
					*findings = append(*findings, Finding{"H2", label, fmt.Sprintf("hook command references missing repo script '%s'", m[1])})
				}
			}
		}
	}
	for _, required := range []string{"guard_protected_paths.py", "guard_event_time.py", "guard_packet_scope.py"} {
		found := false
		for _, cmd := range pre {
			if strings.Contains(cmd, required) {
				found = true
				break
			}
		}
		if !found {
			*findings = append(*findings, Finding{"H4", label, "PreToolUse hooks do not reference " + required})
		}
	}
	if !bashOps {
		*findings = append(*findings, Finding{"H4", label, `PreToolUse hooks do not reference guard_ops_commands.py with a matcher matching "Bash"`})
	}
	if !bashExec {
		*findings = append(*findings, Finding{"H4", label, `PreToolUse hooks do not reference guard_exec_commands.py with a matcher matching "Bash"`})
	}
	return total
}

type hookPair struct {
	matcher any
	hook    map[string]any
}

func iterHooks(entries any) []hookPair {
	list, ok := entries.([]any)
	if !ok {
		return nil
	}
	var out []hookPair
	for _, group := range list {
		g, ok := group.(map[string]any)
		if !ok {
			continue
		}
		matcher := g["matcher"]
		if nested, ok := g["hooks"].([]any); ok {
			for _, hook := range nested {
				if h, ok := hook.(map[string]any); ok {
					out = append(out, hookPair{matcher, h})
				}
			}
			continue
		}
		if _, hasType := g["type"]; hasType {
			out = append(out, hookPair{matcher, g})
			continue
		}
		if _, hasCmd := g["command"]; hasCmd {
			out = append(out, hookPair{matcher, g})
		}
	}
	return out
}

func matcherMatches(matcher any, tool string) bool {
	if matcher == nil {
		return true
	}
	pat, ok := matcher.(string)
	if !ok {
		return false
	}
	pat = strings.TrimSpace(pat)
	if pat == "" || pat == "*" {
		return true
	}
	rx, err := regexp.Compile("^(?:" + pat + ")$")
	if err != nil {
		return pat == tool
	}
	return rx.MatchString(tool)
}

func checkSharedBlocks(root string, findings *[]Finding) int {
	claude := extractShared(filepath.Join(root, "CLAUDE.md"), "CLAUDE.md", findings)
	agents := extractShared(filepath.Join(root, "AGENTS.md"), "AGENTS.md", findings)
	if claude == nil || agents == nil {
		return 0
	}
	names := map[string]struct{}{}
	for k := range claude {
		names[k] = struct{}{}
	}
	for k := range agents {
		names[k] = struct{}{}
	}
	var all []string
	for k := range names {
		all = append(all, k)
	}
	sort.Strings(all)
	if len(all) == 0 {
		*findings = append(*findings, Finding{"H7", "CLAUDE.md", "no shared blocks (<!-- shared:NAME:start/end -->) found in CLAUDE.md and AGENTS.md"})
		return 0
	}
	count := 0
	for _, name := range all {
		c, cok := claude[name]
		a, aok := agents[name]
		if cok && !aok {
			*findings = append(*findings, Finding{"H7", "AGENTS.md", fmt.Sprintf("shared block '%s' exists in CLAUDE.md but is missing from AGENTS.md", name)})
		} else if aok && !cok {
			*findings = append(*findings, Finding{"H7", "CLAUDE.md", fmt.Sprintf("shared block '%s' exists in AGENTS.md but is missing from CLAUDE.md", name)})
		} else if string(c) != string(a) {
			*findings = append(*findings, Finding{"H7", "AGENTS.md", fmt.Sprintf("shared block '%s' differs between CLAUDE.md and AGENTS.md", name)})
		} else {
			count++
		}
	}
	return count
}

func extractShared(path, label string, findings *[]Finding) map[string][]byte {
	raw, err := os.ReadFile(path)
	if err != nil {
		*findings = append(*findings, Finding{"H7", label, label + " does not exist"})
		return nil
	}
	blocks := map[string][]byte{}
	open := ""
	openStart := 0
	offset := 0
	for _, rawLine := range splitKeepEnds(raw) {
		line := strings.TrimRight(string(rawLine), "\r\n")
		m := sharedMarkerRE.FindStringSubmatch(line)
		lineEnd := offset + len(rawLine)
		if m != nil {
			name, kind := m[1], m[2]
			if kind == "start" {
				if open != "" {
					*findings = append(*findings, Finding{"H7", label, fmt.Sprintf("nested shared block '%s' inside '%s'", name, open)})
				} else {
					open = name
					openStart = lineEnd
				}
			} else if open == "" {
				*findings = append(*findings, Finding{"H7", label, fmt.Sprintf("unexpected shared block end marker for '%s'", name)})
			} else if open != name {
				*findings = append(*findings, Finding{"H7", label, fmt.Sprintf("mismatched shared block end '%s' (expected '%s')", name, open)})
				open = ""
			} else if _, dup := blocks[name]; dup {
				*findings = append(*findings, Finding{"H7", label, fmt.Sprintf("duplicate shared block '%s'", name)})
				open = ""
			} else {
				blocks[name] = append([]byte(nil), raw[openStart:offset]...)
				open = ""
			}
		}
		offset = lineEnd
	}
	if open != "" {
		*findings = append(*findings, Finding{"H7", label, fmt.Sprintf("unclosed shared block '%s'", open)})
	}
	return blocks
}

func splitKeepEnds(raw []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\n' {
			lines = append(lines, raw[start:i+1])
			start = i + 1
			continue
		}
		if raw[i] == '\r' {
			end := i + 1
			if i+1 < len(raw) && raw[i+1] == '\n' {
				end = i + 2
				i++
			}
			lines = append(lines, raw[start:end])
			start = end
		}
	}
	if start < len(raw) {
		lines = append(lines, raw[start:])
	}
	return lines
}

func checkDocSizes(root string, findings *[]Finding) {
	if raw, err := os.ReadFile(filepath.Join(root, "CLAUDE.md")); err == nil {
		n := len(strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n"))
		if strings.HasSuffix(string(raw), "\n") {
			n--
		}
		if n > maxClaudeMDLines {
			*findings = append(*findings, Finding{"H8", "CLAUDE.md", fmt.Sprintf("CLAUDE.md has %d lines (limit is %d)", n, maxClaudeMDLines)})
		}
	}
	if st, err := os.Stat(filepath.Join(root, "AGENTS.md")); err == nil && st.Size() > maxAgentsMDBytes {
		*findings = append(*findings, Finding{"H8", "AGENTS.md", fmt.Sprintf("AGENTS.md is %d bytes (limit is %d)", st.Size(), maxAgentsMDBytes)})
	}
	skills := filepath.Join(root, ".agents", "skills")
	entries, err := os.ReadDir(skills)
	if err != nil {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, ent := range entries {
		rel := filepath.ToSlash(filepath.Join(".agents", "skills", ent.Name()))
		full := filepath.Join(skills, ent.Name())
		info, err := os.Stat(full)
		if err != nil || !info.IsDir() {
			*findings = append(*findings, Finding{"H9", rel, "entry under .agents/skills does not resolve to a directory containing SKILL.md"})
			continue
		}
		if st, err := os.Stat(filepath.Join(full, "SKILL.md")); err != nil || !st.Mode().IsRegular() {
			*findings = append(*findings, Finding{"H9", rel, "entry under .agents/skills does not resolve to a directory containing SKILL.md"})
		}
	}
}

func checkFailurePatterns(root string, findings *[]Finding) {
	patternsPath := filepath.Join(root, failurePatterns)
	skillPath := filepath.Join(root, failureSkill)
	raw, err := os.ReadFile(patternsPath)
	if err != nil {
		*findings = append(*findings, Finding{"H10", failurePatterns, failurePatterns + " is missing"})
		if _, err := os.Stat(skillPath); err != nil {
			*findings = append(*findings, Finding{"H10", failureSkill, failureSkill + " is missing"})
		}
		return
	}
	text := string(raw)
	lines := strings.Split(text, "\n")
	nonFence := nonFenced(lines)
	sections := parseFPSections(lines, nonFence, findings)
	if len(sections) == 0 {
		*findings = append(*findings, Finding{"H10", failurePatterns, "no '## FP-NN <name>' sections found"})
	} else {
		checkFPSequence(lines, sections, findings)
		checkFPLeadins(nonFence, sections, findings)
	}
	checkPrecedents(root, text, findings)
	checkRouting(skillPath, sections, findings)
}

type fpSection struct {
	num        int
	nn         string
	start, end int
}

func nonFenced(lines []string) [][2]any {
	var out [][2]any
	in := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "```") {
			in = !in
			continue
		}
		if !in {
			out = append(out, [2]any{i, line})
		}
	}
	return out
}

func parseFPSections(lines []string, nonFence [][2]any, findings *[]Finding) []fpSection {
	var h2 [][2]any
	for _, pair := range nonFence {
		line := pair[1].(string)
		if h2RE.MatchString(line) {
			h2 = append(h2, pair)
		}
	}
	var sections []fpSection
	for pos, pair := range h2 {
		idx := pair[0].(int)
		line := pair[1].(string)
		end := len(lines)
		if pos+1 < len(h2) {
			end = h2[pos+1][0].(int)
		}
		m := fpHeadingRE.FindStringSubmatch(line)
		if m == nil {
			*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("invalid section heading '%s' (expected '## FP-NN <name>')", line)})
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			n = 0
		}
		sections = append(sections, fpSection{n, m[1], idx, end})
	}
	return sections
}

func checkFPSequence(lines []string, sections []fpSection, findings *[]Finding) {
	seen := map[int]struct{}{}
	prev := 0
	reported := false
	for _, sec := range sections {
		if sec.num < 1 {
			*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("invalid section number 'FP-%s' (numbering must start at FP-01)", sec.nn)})
			continue
		}
		if _, ok := seen[sec.num]; ok {
			*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("duplicate section 'FP-%s'", sec.nn)})
		} else {
			seen[sec.num] = struct{}{}
		}
		if sec.num < prev && !reported {
			*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("section 'FP-%s' is out of order", sec.nn)})
			reported = true
		}
		prev = sec.num
	}
	if len(seen) > 0 {
		maxN := 0
		for n := range seen {
			if n > maxN {
				maxN = n
			}
		}
		for n := 1; n <= maxN; n++ {
			if _, ok := seen[n]; !ok {
				*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("missing section 'FP-%02d' (sections must be contiguous from FP-01)", n)})
			}
		}
	}
	if len(sections) == 0 {
		return
	}
	last := sections[len(sections)-1].nn
	expected := "FP-01 \u301c FP-" + last
	first := ""
	if len(lines) > 0 {
		first = lines[0]
	}
	if !titleHasRange(first, expected) {
		*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("first-line title must contain '%s'", expected)})
	}
}

// titleHasRange matches Python's title regex: the expected range must not be
// glued to an identifier character on the left or another digit on the right,
// so "FP-020" does not satisfy a required "FP-02".
func titleHasRange(first, expected string) bool {
	for i := 0; i+len(expected) <= len(first); i++ {
		if first[i:i+len(expected)] != expected {
			continue
		}
		if i > 0 {
			prev := first[i-1]
			if (prev >= 'A' && prev <= 'Z') || (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') || prev == '-' {
				continue
			}
		}
		end := i + len(expected)
		if end < len(first) && first[end] >= '0' && first[end] <= '9' {
			continue
		}
		return true
	}
	return false
}

func checkFPLeadins(nonFence [][2]any, sections []fpSection, findings *[]Finding) {
	for _, sec := range sections {
		var body strings.Builder
		first := true
		for _, pair := range nonFence {
			idx := pair[0].(int)
			if idx <= sec.start || idx >= sec.end {
				continue
			}
			if !first {
				body.WriteByte('\n')
			}
			first = false
			body.WriteString(pair[1].(string))
		}
		text := body.String()
		var positions []struct {
			at   int
			lead string
		}
		ok := true
		for _, lead := range requiredLeadins {
			count := strings.Count(text, lead)
			if count == 0 {
				*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("FP-%s is missing lead-in %s", sec.nn, lead)})
				ok = false
			} else if count > 1 {
				*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("FP-%s contains lead-in %s %d times (expected 1)", sec.nn, lead, count)})
				ok = false
			} else {
				positions = append(positions, struct {
					at   int
					lead string
				}{strings.Index(text, lead), lead})
			}
		}
		if ok {
			order := append([]struct {
				at   int
				lead string
			}(nil), positions...)
			sort.Slice(order, func(i, j int) bool { return order[i].at < order[j].at })
			for i, lead := range requiredLeadins {
				if order[i].lead != lead {
					*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("FP-%s lead-ins are out of order", sec.nn)})
					break
				}
			}
		}
		if pos := strings.Index(text, "**先例.**"); pos >= 0 {
			after := text[pos+len("**先例.**"):]
			block := after
			if m := leadinBreakRE.FindStringIndex(after); m != nil {
				block = after[:m[0]]
			}
			for _, abbrev := range findAbbreviated(block) {
				*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("FP-%s 先例 contains abbreviated id '%s'; write each ADR/PM id in full", sec.nn, abbrev)})
			}
		}
	}
}

func checkPrecedents(root, text string, findings *[]Finding) {
	seenADR := map[string]struct{}{}
	for _, digits := range findIDs(text, regexp.MustCompile(`ADR-(\d{6})`), 4, false) {
		if _, ok := seenADR[digits]; ok {
			continue
		}
		seenADR[digits] = struct{}{}
		rel := "docs/ADR/" + digits + ".md"
		if st, err := os.Stat(filepath.Join(root, rel)); err != nil || !st.Mode().IsRegular() {
			*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("precedent ADR-%s has no file %s", digits, rel)})
		}
	}
	pmDir := filepath.Join(root, "docs", "postmortems")
	seenPM := map[string]struct{}{}
	for _, suffix := range findIDs(text, regexp.MustCompile(`PM-(\d{4}-\d{3})`), 3, false) {
		if _, ok := seenPM[suffix]; ok {
			continue
		}
		seenPM[suffix] = struct{}{}
		pmID := "PM-" + suffix
		var matches []string
		if entries, err := os.ReadDir(pmDir); err == nil {
			for _, ent := range entries {
				if strings.HasPrefix(ent.Name(), pmID+"-") && strings.HasSuffix(ent.Name(), ".md") && !ent.IsDir() {
					matches = append(matches, ent.Name())
				}
			}
		}
		sort.Strings(matches)
		if len(matches) == 0 {
			*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("precedent %s has no file matching docs/postmortems/%s-*.md", pmID, pmID)})
		} else if len(matches) > 1 {
			rels := make([]string, len(matches))
			for i, name := range matches {
				rels[i] = "docs/postmortems/" + name
			}
			*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("precedent %s matches multiple files: %s", pmID, strings.Join(rels, ", "))})
		}
	}
	seenPath := map[string]struct{}{}
	for _, candidate := range findRepoPaths(text) {
		candidate = strings.TrimRight(candidate, ".,;:")
		if strings.ContainsAny(candidate, "<>*") {
			continue
		}
		if _, ok := seenPath[candidate]; ok {
			continue
		}
		seenPath[candidate] = struct{}{}
		if _, err := os.Stat(filepath.Join(root, candidate)); err != nil {
			*findings = append(*findings, Finding{"H10", failurePatterns, fmt.Sprintf("referenced path '%s' does not exist", candidate)})
		}
	}
}

func checkRouting(skillPath string, sections []fpSection, findings *[]Finding) {
	rows := routingRows(skillPath, findings)
	if rows == nil {
		return
	}
	var table []string
	seen := map[string]struct{}{}
	fpRE := regexp.MustCompile(`FP-(\d+)`)
	for _, row := range rows {
		for _, digits := range findIDs(row, fpRE, 3, false) {
			id := "FP-" + digits
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			table = append(table, id)
		}
	}
	var pattern []string
	pset := map[string]struct{}{}
	for _, sec := range sections {
		id := "FP-" + sec.nn
		if _, ok := pset[id]; ok {
			continue
		}
		pset[id] = struct{}{}
		pattern = append(pattern, id)
	}
	for _, id := range table {
		if _, ok := pset[id]; !ok {
			*findings = append(*findings, Finding{"H10", failureSkill, fmt.Sprintf("routing table references '%s' which has no section in patterns.md", id)})
		}
	}
	for _, id := range pattern {
		if _, ok := seen[id]; !ok {
			*findings = append(*findings, Finding{"H10", failureSkill, fmt.Sprintf("section '%s' in patterns.md does not appear in '## 経路表'", id)})
		}
	}
}

func routingRows(skillPath string, findings *[]Finding) []string {
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		*findings = append(*findings, Finding{"H10", failureSkill, failureSkill + " is missing"})
		return nil
	}
	lines := strings.Split(string(raw), "\n")
	nonFence := nonFenced(lines)
	start := -1
	end := len(lines)
	for _, pair := range nonFence {
		idx := pair[0].(int)
		line := pair[1].(string)
		if start < 0 {
			if strings.TrimSpace(line) == "## 経路表" {
				start = idx
			}
			continue
		}
		if h2RE.MatchString(line) {
			end = idx
			break
		}
	}
	if start < 0 {
		*findings = append(*findings, Finding{"H10", failureSkill, "missing '## 経路表' heading"})
		return nil
	}
	var route []string
	for _, pair := range nonFence {
		idx := pair[0].(int)
		line := strings.TrimSpace(pair[1].(string))
		if idx > start && idx < end && strings.HasPrefix(line, "|") {
			route = append(route, line)
		}
	}
	var data []string
	seenSep := false
	for _, row := range route {
		if !seenSep {
			if sepRowRE.MatchString(row) {
				seenSep = true
			}
			continue
		}
		data = append(data, row)
	}
	if len(data) == 0 {
		*findings = append(*findings, Finding{"H10", failureSkill, "no routing table rows found under '## 経路表'"})
		return nil
	}
	return data
}

func checkRuleLedger(root string, findings *[]Finding) {
	path := filepath.Join(root, ruleLedgerRel)
	raw, err := os.ReadFile(path)
	if err != nil {
		*findings = append(*findings, Finding{"H11", ruleLedgerRel, ruleLedgerRel + " is missing"})
		return
	}
	text := string(raw)
	maxJ := extractMaxJudgment(text)
	if maxJ < 0 {
		*findings = append(*findings, Finding{"H11", ruleLedgerRel, "missing non-negative integer 'max_judgment_rules' in frontmatter or metadata comment"})
	}
	rows := parseLedgerRows(strings.Split(text, "\n"), findings)
	if rows == nil {
		return
	}
	astIDs := loadAstGrepIDs(root)
	seen := map[string]struct{}{}
	judgment := 0
	for _, row := range rows {
		if row[0] != "" {
			if _, ok := seen[row[0]]; ok {
				*findings = append(*findings, Finding{"H11", ruleLedgerRel, fmt.Sprintf("duplicate rule ID '%s' in rule ledger", row[0])})
			} else {
				seen[row[0]] = struct{}{}
			}
		}
		if checkLedgerRow(root, row, astIDs, findings) {
			judgment++
		}
	}
	if maxJ >= 0 {
		if judgment > maxJ {
			*findings = append(*findings, Finding{"H11", ruleLedgerRel, fmt.Sprintf("judgment rule count (%d) exceeds max_judgment_rules (%d); mechanize rules instead of adding prose-only judgment rules", judgment, maxJ)})
		} else if judgment < maxJ {
			*findings = append(*findings, Finding{"H11", ruleLedgerRel, fmt.Sprintf("judgment rule count (%d) is below max_judgment_rules (%d); lower max_judgment_rules to %d in the same commit", judgment, maxJ, judgment)})
		}
		crossCheckRatchet(root, maxJ, findings)
	}
}

// ExtractMaxJudgmentRules reads the H11 ceiling. The live ledger ceiling is 6
// and the checker only allows that ceiling to decrease.
func ExtractMaxJudgmentRules(text string) (int, bool) {
	n := extractMaxJudgment(text)
	if n < 0 {
		return 0, false
	}
	return n, true
}

func extractMaxJudgment(text string) int {
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && strings.TrimSpace(strings.TrimPrefix(lines[0], "\ufeff")) == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				for _, line := range lines[1:i] {
					if m := maxJudgmentFMRE.FindStringSubmatch(line); m != nil {
						n := 0
						fmt.Sscan(m[1], &n)
						if n >= 0 {
							return n
						}
						return -1
					}
				}
				break
			}
		}
	}
	for _, line := range lines {
		if m := maxJudgmentCMRE.FindStringSubmatch(line); m != nil {
			n := 0
			fmt.Sscan(m[1], &n)
			if n >= 0 {
				return n
			}
			return -1
		}
	}
	return -1
}

func parseLedgerRows(lines []string, findings *[]Finding) [][4]string {
	nonFence := nonFenced(lines)
	var rows [][4]string
	found := false
	for pos := 0; pos < len(nonFence); pos++ {
		line := nonFence[pos][1].(string)
		if strings.HasPrefix(strings.TrimSpace(line), "|") && stringSliceEqual(splitCells(line), ledgerHeaders) && pos+1 < len(nonFence) && sepRowRE.MatchString(strings.TrimSpace(nonFence[pos+1][1].(string))) {
			found = true
			pos += 2
			for pos < len(nonFence) {
				row := nonFence[pos][1].(string)
				if !strings.HasPrefix(strings.TrimSpace(row), "|") {
					break
				}
				cells := splitCells(row)
				if len(cells) != 4 {
					*findings = append(*findings, Finding{"H11", ruleLedgerRel, fmt.Sprintf("table row has %d columns (expected 4): %s", len(cells), strings.TrimSpace(row))})
				} else {
					rows = append(rows, [4]string{cells[0], cells[1], cells[2], cells[3]})
				}
				pos++
			}
			break
		}
	}
	if !found {
		*findings = append(*findings, Finding{"H11", ruleLedgerRel, "missing canonical Markdown table with columns '| ID | 規則 | 強制手段 | 由来 |'"})
		return nil
	}
	if len(rows) == 0 {
		*findings = append(*findings, Finding{"H11", ruleLedgerRel, "rule ledger table has 0 data rows"})
		return nil
	}
	return rows
}

func splitCells(line string) []string {
	s := strings.TrimSpace(line)
	if strings.HasPrefix(s, "|") {
		s = s[1:]
	}
	if strings.HasSuffix(s, "|") {
		s = s[:len(s)-1]
	}
	parts := strings.Split(s, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func checkLedgerRow(root string, row [4]string, astIDs map[string]struct{}, findings *[]Finding) bool {
	id, desc, enforcement, origin := row[0], row[1], row[2], row[3]
	if id == "" || desc == "" || enforcement == "" || origin == "" {
		label := id
		if label == "" {
			label = "<empty>"
		}
		*findings = append(*findings, Finding{"H11", ruleLedgerRel, fmt.Sprintf("row '%s' contains empty required column(s)", label)})
	}
	seenC := map[string]struct{}{}
	for _, corpusID := range findCorpus(origin) {
		if _, ok := seenC[corpusID]; ok {
			continue
		}
		seenC[corpusID] = struct{}{}
		meta := filepath.Join(root, "scripts", "harness", "corpus", corpusID, "meta.toml")
		if st, err := os.Stat(meta); err != nil || !st.Mode().IsRegular() {
			*findings = append(*findings, Finding{"H11", ruleLedgerRel, fmt.Sprintf("row '%s' cites corpus case %s but scripts/harness/corpus/%s/meta.toml does not exist", id, corpusID, corpusID)})
		}
	}
	seenA := map[string]struct{}{}
	for _, digits := range findIDs(origin, regexp.MustCompile(`ADR-(\d{6})`), 4, false) {
		if _, ok := seenA[digits]; ok {
			continue
		}
		seenA[digits] = struct{}{}
		rel := "docs/ADR/" + digits + ".md"
		if st, err := os.Stat(filepath.Join(root, rel)); err != nil || !st.Mode().IsRegular() {
			*findings = append(*findings, Finding{"H11", ruleLedgerRel, fmt.Sprintf("row '%s' cites ADR-%s but %s does not exist", id, digits, rel)})
		}
	}
	if strings.HasPrefix(strings.TrimSpace(enforcement), "判断") {
		return true
	}
	checkEnforcement(root, id, enforcement, astIDs, findings)
	return false
}

func checkEnforcement(root, id, enforcement string, astIDs map[string]struct{}, findings *[]Finding) {
	concrete := 0
	for _, span := range backtickRE.FindAllStringSubmatch(enforcement, -1) {
		token := strings.TrimSpace(span[1])
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, "ast-grep:") {
			concrete++
			name := strings.TrimSpace(strings.TrimPrefix(token, "ast-grep:"))
			if name == "" || !hasKey(astIDs, name) {
				*findings = append(*findings, Finding{"H11", ruleLedgerRel, fmt.Sprintf("row '%s' references missing ast-grep rule '%s' (not found in %s/*.yml)", id, name, astGrepRulesRel)})
			}
			continue
		}
		candidate := token
		if i := strings.IndexAny(candidate, " \t"); i >= 0 {
			candidate = candidate[:i]
		}
		candidate = strings.TrimRight(candidate, ".,;:")
		if strings.Contains(candidate, "/") {
			concrete++
			if st, err := os.Stat(filepath.Join(root, candidate)); err != nil || !st.Mode().IsRegular() {
				*findings = append(*findings, Finding{"H11", ruleLedgerRel, fmt.Sprintf("row '%s' references missing or non-file repo path '%s'", id, candidate)})
			}
		} else if hasFileExt(candidate) {
			if st, err := os.Stat(filepath.Join(root, candidate)); err != nil || !st.Mode().IsRegular() {
				*findings = append(*findings, Finding{"H11", ruleLedgerRel, fmt.Sprintf("row '%s' references missing file token '%s'", id, candidate)})
			}
		}
	}
	if concrete == 0 {
		*findings = append(*findings, Finding{"H11", ruleLedgerRel, fmt.Sprintf("row '%s' is not marked '判断' and cites no backticked repo path or ast-grep:<id> in 強制手段", id)})
	}
}

func crossCheckRatchet(root string, maxJ int, findings *[]Finding) {
	path := filepath.Join(root, ratchetRel)
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	ratchetVal, found, parseErr := parseGovernanceMaxJudgmentTOML(string(raw))
	if parseErr != nil {
		*findings = append(*findings, Finding{"H11", ratchetRel, fmt.Sprintf("failed to parse %s for max_judgment_rules cross-check: %v", ratchetRel, parseErr)})
		return
	}
	if !found || ratchetVal != maxJ {
		var shown any
		if found {
			shown = ratchetVal
		}
		*findings = append(*findings, Finding{"H11", ratchetRel, fmt.Sprintf("[governance].max_judgment_rules in %s (%v) does not match %s (%d)", ratchetRel, shown, ruleLedgerRel, maxJ)})
	}
}

// parseGovernanceMaxJudgmentTOML extracts [governance].max_judgment_rules from
// suppressions.toml using only the Go standard library (zero external TOML dep).
func parseGovernanceMaxJudgmentTOML(text string) (int, bool, error) {
	section := ""
	found := false
	value := 0
	for lineNo, rawLine := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(stripTOMLComment(rawLine))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return 0, false, fmt.Errorf("line %d: unclosed table header %q", lineNo+1, line)
			}
			inner := strings.TrimSpace(line[1 : len(line)-1])
			if strings.HasPrefix(inner, "[") && strings.HasSuffix(inner, "]") {
				inner = strings.TrimSpace(inner[1 : len(inner)-1])
			}
			if inner == "" {
				return 0, false, fmt.Errorf("line %d: empty table header", lineNo+1)
			}
			section = inner
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			return 0, false, fmt.Errorf("line %d: expected key = value", lineNo+1)
		}
		key := strings.TrimSpace(line[:eq])
		rawVal := strings.TrimSpace(line[eq+1:])
		if section == "governance" && key == "max_judgment_rules" {
			cleaned := strings.ReplaceAll(rawVal, "_", "")
			n, err := strconv.Atoi(cleaned)
			if err != nil {
				return 0, false, nil
			}
			value = n
			found = true
		}
	}
	return value, found, nil
}

func stripTOMLComment(line string) string {
	quote := byte(0)
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if quote != 0 {
			if quote == '"' {
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '#' {
			return line[:i]
		}
	}
	return line
}

func loadAstGrepIDs(root string) map[string]struct{} {
	dir := filepath.Join(root, astGrepRulesRel)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return map[string]struct{}{}
	}
	var names []string
	for _, ent := range entries {
		if strings.HasSuffix(ent.Name(), ".yml") && !ent.IsDir() {
			names = append(names, ent.Name())
		}
	}
	sort.Strings(names)
	ids := map[string]struct{}{}
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if m := astGrepIDRE.FindStringSubmatch(line); m != nil {
				ids[m[1]] = struct{}{}
				break
			}
		}
	}
	return ids
}

func hasKey(m map[string]struct{}, k string) bool {
	_, ok := m[k]
	return ok
}

func hasFileExt(name string) bool {
	for _, ext := range fileLikeExts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func findIDs(text string, re *regexp.Regexp, prefixLen int, allowDigitAfter bool) []string {
	var out []string
	for _, loc := range re.FindAllStringSubmatchIndex(text, -1) {
		if loc[0] > 0 && isIdentByte(text[loc[0]-1]) {
			continue
		}
		if !allowDigitAfter && loc[1] < len(text) && text[loc[1]] >= '0' && text[loc[1]] <= '9' {
			continue
		}
		if len(loc) >= 4 {
			out = append(out, text[loc[2]:loc[3]])
		}
	}
	_ = prefixLen
	return out
}

func findCorpus(text string) []string {
	re := regexp.MustCompile(`C-(\d{2,})`)
	var out []string
	for _, loc := range re.FindAllStringSubmatchIndex(text, -1) {
		if loc[0] > 0 && isIdentByte(text[loc[0]-1]) {
			continue
		}
		if loc[1] < len(text) && text[loc[1]] >= '0' && text[loc[1]] <= '9' {
			continue
		}
		out = append(out, text[loc[0]:loc[1]])
	}
	return out
}

func findAbbreviated(text string) []string {
	re := regexp.MustCompile(`(?:ADR-\d{6}|PM-\d{4}-\d{3})(?:\s*[/、]\s*(?:ADR-\d{6}|PM-\d{4}-\d{3}))*\s*[/、]\s*(\d{3,6})`)
	var out []string
	for _, loc := range re.FindAllStringSubmatchIndex(text, -1) {
		if loc[0] > 0 && isIdentByte(text[loc[0]-1]) {
			continue
		}
		if loc[1] < len(text) && text[loc[1]] >= '0' && text[loc[1]] <= '9' {
			continue
		}
		out = append(out, text[loc[2]:loc[3]])
	}
	return out
}

func findRepoPaths(text string) []string {
	re := regexp.MustCompile(`(?:scripts|\.github|\.claude/skills)/[^\s` + "`" + `'\"()（）,、。]+`)
	var out []string
	for _, loc := range re.FindAllStringIndex(text, -1) {
		if loc[0] > 0 {
			prev := text[loc[0]-1]
			if (prev >= 'A' && prev <= 'Z') || (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') || strings.ContainsRune("_./-", rune(prev)) {
				continue
			}
		}
		out = append(out, text[loc[0]:loc[1]])
	}
	return out
}

func isIdentByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '-'
}

func globToRegex(pattern string) *regexp.Regexp {
	pat := strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pat); {
		switch {
		case strings.HasPrefix(pat[i:], "**/"):
			b.WriteString("(?:.+/)?")
			i += 3
		case strings.HasPrefix(pat[i:], "/**") && i+3 == len(pat):
			b.WriteString("/.+")
			i += 3
		case strings.HasPrefix(pat[i:], "**"):
			b.WriteString(".*")
			i += 2
		case pat[i] == '*':
			b.WriteString("[^/]*")
			i++
		case pat[i] == '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(pat[i : i+1]))
			i++
		}
	}
	b.WriteByte('$')
	return regexp.MustCompile(b.String())
}

func matchesRuleGlob(pattern string, files []string) bool {
	for _, alt := range expandBraces(pattern) {
		rx := globToRegex(alt)
		for _, f := range files {
			if rx.MatchString(f) {
				return true
			}
		}
	}
	return false
}

func expandBraces(pattern string) []string {
	start, depth := -1, 0
	for idx, ch := range pattern {
		if ch == '{' {
			if depth == 0 {
				start = idx
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 && start >= 0 {
				prefix, inner, suffix := pattern[:start], pattern[start+1:idx], pattern[idx+1:]
				var opts []string
				var buf strings.Builder
				innerDepth := 0
				for _, c := range inner {
					switch c {
					case '{':
						innerDepth++
						buf.WriteRune(c)
					case '}':
						innerDepth--
						buf.WriteRune(c)
					case ',':
						if innerDepth == 0 {
							opts = append(opts, buf.String())
							buf.Reset()
						} else {
							buf.WriteRune(c)
						}
					default:
						buf.WriteRune(c)
					}
				}
				opts = append(opts, buf.String())
				var out []string
				for _, opt := range opts {
					out = append(out, expandBraces(prefix+opt+suffix)...)
				}
				return out
			}
		}
	}
	return []string{pattern}
}

func extractFrontmatterPaths(text string) []string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(strings.TrimPrefix(lines[0], "\ufeff")) != "---" {
		return nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil
	}
	fm := lines[1:end]
	var paths []string
	for i := 0; i < len(fm); i++ {
		line := fm[i]
		if !pathsKeyRE.MatchString(line) {
			continue
		}
		rest := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
			paths = append(paths, splitInlineYAML(rest[1:len(rest)-1])...)
		} else if rest != "" {
			paths = append(paths, stripYAMLQuotes(rest))
		} else {
			items, next := parseBlockYAML(fm, i+1)
			paths = append(paths, items...)
			i = next
		}
	}
	return paths
}

func stripYAMLQuotes(raw string) string {
	val := strings.TrimSpace(raw)
	if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
		return val[1 : len(val)-1]
	}
	if i := strings.Index(val, " #"); i >= 0 {
		val = strings.TrimRight(val[:i], " ")
	}
	return val
}

func splitInlineYAML(inner string) []string {
	var items []string
	var buf strings.Builder
	var quote byte
	depth := 0
	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		if quote != 0 {
			buf.WriteByte(ch)
			if ch == quote && (i == 0 || inner[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			buf.WriteByte(ch)
			continue
		}
		if ch == '{' {
			depth++
			buf.WriteByte(ch)
			continue
		}
		if ch == '}' {
			if depth > 0 {
				depth--
			}
			buf.WriteByte(ch)
			continue
		}
		if ch == ',' && depth == 0 {
			if token := stripYAMLQuotes(buf.String()); token != "" {
				items = append(items, token)
			}
			buf.Reset()
			continue
		}
		buf.WriteByte(ch)
	}
	if token := stripYAMLQuotes(buf.String()); token != "" {
		items = append(items, token)
	}
	return items
}

func parseBlockYAML(lines []string, start int) ([]string, int) {
	var items []string
	j := start
	for j < len(lines) {
		sub := lines[j]
		if strings.TrimSpace(sub) == "" || strings.HasPrefix(strings.TrimLeft(sub, " \t"), "#") {
			j++
			continue
		}
		if sub != "" && sub[0] != ' ' && sub[0] != '\t' {
			break
		}
		if m := yamlItemRE.FindStringSubmatch(sub); m != nil {
			if item := stripYAMLQuotes(m[1]); item != "" {
				items = append(items, item)
			}
		}
		j++
	}
	if j == start {
		return items, start
	}
	return items, j - 1
}

func init() {
	_ = utf8.UTFMax
}
