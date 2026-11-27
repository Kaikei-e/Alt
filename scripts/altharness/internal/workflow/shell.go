package workflow

import (
	"path"
	"regexp"
	"strings"
)

var (
	pythonBinRE    = regexp.MustCompile(`^python(?:\d+(?:\.\d+)*)?$`)
	envAssignRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=.*$`)
	composeAuditRE = regexp.MustCompile(`^compose-.*-audit\.py$`)
)

var shellLeadPrefixes = map[string]struct{}{
	"if": {}, "then": {}, "elif": {}, "else": {}, "do": {}, "while": {}, "until": {},
	"!": {}, "time": {}, "exec": {}, "command": {}, "builtin": {}, "nohup": {},
}

var directLintTools = map[string]struct{}{
	"golangci-lint": {}, "gofmt": {}, "gosec": {}, "govulncheck": {}, "ruff": {},
	"bandit": {}, "pyrefly": {}, "biome": {}, "svelte-check": {}, "actionlint": {},
	"docdag": {}, "promtool": {}, "amtool": {}, "lkal": {}, "tauto": {},
}

var wrappedStaticScripts = map[string]struct{}{
	"compose-feature-gate.py": {}, "compose-image-audit.py": {}, "check-compose-variables.sh": {},
	"test_compose_wiring.sh": {}, "kisage-env.py": {}, "regen-service-registry.py": {},
	"observability-validate.sh": {}, "harness_selfcheck.py": {}, "postmortem_ledger.py": {},
	"migration_destructive_gate.py": {}, "adr_supersede_gate.py": {}, "check-suites.sh": {},
	"workflow-safety-check.py": {},
}

var altHarnessStaticSubcommands = map[string]struct{}{
	"selfcheck": {}, "postmortem": {}, "migration-gate": {}, "adr-gate": {},
	"workflow": {}, "ratchet": {}, "suppressions": {}, "generated": {},
	"hygiene": {}, "pyver": {}, "sync-go": {}, "pki": {}, "lang": {},
}

var goRunLintPackages = map[string]struct{}{
	"actionlint": {}, "golangci-lint": {}, "gosec": {}, "govulncheck": {},
}

var uvRunValueFlags = map[string]struct{}{
	"--python": {}, "-p": {}, "--with": {}, "--with-editable": {}, "--directory": {},
	"--project": {}, "--package": {}, "--group": {}, "--extra": {}, "--python-preference": {},
	"--index-url": {}, "--extra-index-url": {},
}

func isWrappedStaticScript(target string) bool {
	base := path.Base(target)
	if _, ok := wrappedStaticScripts[base]; ok {
		return true
	}
	return composeAuditRE.MatchString(base)
}

func findDirectLintInvocations(runBody string) []string {
	normalized := strings.ReplaceAll(runBody, "\\\n", " ")
	segments := extractShellSegments(normalized)
	var found []string
	seen := map[string]struct{}{}
	for _, seg := range segments {
		hit := detectDirectLintCommand(seg)
		if hit == "" {
			continue
		}
		if _, ok := seen[hit]; ok {
			continue
		}
		seen[hit] = struct{}{}
		found = append(found, hit)
	}
	return found
}

func extractShellSegments(script string) [][]string {
	segs, _ := extractShellFrom(script, 0, false)
	return segs
}

func extractShellFrom(script string, start int, stopOnRparen bool) ([][]string, int) {
	var segments [][]string
	var tokens []string
	var buf []byte
	inToken := false
	inSingle := false
	inDouble := false
	i := start
	n := len(script)
	flushToken := func() {
		if inToken || len(buf) > 0 {
			tokens = append(tokens, string(buf))
			buf = buf[:0]
			inToken = false
		}
	}
	flushSegment := func() {
		flushToken()
		if len(tokens) > 0 {
			segments = append(segments, append([]string{}, tokens...))
			tokens = tokens[:0]
		}
	}
	for i < n {
		ch := script[i]
		if inSingle {
			if ch == '\'' {
				inSingle = false
			} else {
				buf = append(buf, ch)
			}
			i++
			continue
		}
		if inDouble {
			if ch == '\\' && i+1 < n {
				if script[i+1] == '\n' {
					i += 2
					continue
				}
				buf = append(buf, script[i+1])
				i += 2
				continue
			}
			if ch == '"' {
				inDouble = false
				i++
				continue
			}
			if ch == '$' && i+1 < n && script[i+1] == '(' {
				sub, next := extractShellFrom(script, i+2, true)
				segments = append(segments, sub...)
				i = next
				continue
			}
			buf = append(buf, ch)
			i++
			continue
		}
		if ch == '\\' && i+1 < n {
			if script[i+1] == '\n' {
				i += 2
				continue
			}
			buf = append(buf, script[i+1])
			inToken = true
			i += 2
			continue
		}
		if ch == '\'' {
			inSingle = true
			inToken = true
			i++
			continue
		}
		if ch == '"' {
			inDouble = true
			inToken = true
			i++
			continue
		}
		if ch == '#' && !inToken {
			for i < n && script[i] != '\n' {
				i++
			}
			continue
		}
		if stopOnRparen && ch == ')' {
			flushSegment()
			return segments, i + 1
		}
		if ch == '$' && i+1 < n && script[i+1] == '(' {
			sub, next := extractShellFrom(script, i+2, true)
			segments = append(segments, sub...)
			i = next
			continue
		}
		if ch == '\n' || ch == ';' || ch == '(' || ch == ')' || ch == '{' || ch == '}' {
			flushSegment()
			i++
			continue
		}
		if ch == '&' || ch == '|' {
			flushSegment()
			if i+1 < n && script[i+1] == ch {
				i += 2
			} else {
				i++
			}
			continue
		}
		if isShellSpace(ch) {
			flushToken()
			i++
			continue
		}
		buf = append(buf, ch)
		inToken = true
		i++
	}
	flushSegment()
	return segments, i
}

func isShellSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\v' || ch == '\f'
}

func stripShellPrefixes(tokens []string) []string {
	i := 0
	for i < len(tokens) {
		tok := tokens[i]
		if _, ok := shellLeadPrefixes[tok]; ok || envAssignRE.MatchString(tok) {
			i++
			continue
		}
		if tok == "env" {
			i++
			for i < len(tokens) {
				arg := tokens[i]
				if envAssignRE.MatchString(arg) {
					i++
					continue
				}
				if arg == "-u" || arg == "-C" || arg == "-S" || arg == "--unset" || arg == "--chdir" {
					i += 2
					continue
				}
				if strings.HasPrefix(arg, "-") {
					i++
					continue
				}
				break
			}
			continue
		}
		break
	}
	if i > len(tokens) {
		return nil
	}
	return tokens[i:]
}

func firstPositional(args []string, valueFlags map[string]struct{}) (string, []string, bool) {
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if _, ok := valueFlags[arg]; ok {
			i += 2
			continue
		}
		if strings.HasPrefix(arg, "-") {
			i++
			continue
		}
		return arg, args[i+1:], true
	}
	if i < len(args) {
		return args[i], args[i+1:], true
	}
	return "", nil, false
}

func detectDirectLintCommand(tokens []string) string {
	core := stripShellPrefixes(tokens)
	if len(core) == 0 {
		return ""
	}
	cmd := path.Base(core[0])
	args := core[1:]
	if _, ok := directLintTools[cmd]; ok || isWrappedStaticScript(cmd) {
		return cmd
	}
	switch cmd {
	case "alt-harness":
		sub, _, ok := firstPositional(args, nil)
		if ok {
			if _, hit := altHarnessStaticSubcommands[sub]; hit {
				return "alt-harness " + sub
			}
		}
		return ""
	case "go":
		sub, rest, ok := firstPositional(args, nil)
		if !ok {
			return ""
		}
		if sub == "vet" {
			return "go vet"
		}
		if sub == "run" {
			pkg, _, ok := firstPositional(rest, nil)
			if ok && pkg != "" {
				name := pkg
				if j := strings.IndexByte(name, '@'); j >= 0 {
					name = name[:j]
				}
				name = strings.TrimRight(name, "/")
				if k := strings.LastIndex(name, "/"); k >= 0 {
					name = name[k+1:]
				}
				if _, hit := goRunLintPackages[name]; hit {
					return "go run " + name
				}
			}
		}
		return ""
	case "cargo":
		idx := 0
		for idx < len(args) && strings.HasPrefix(args[idx], "+") {
			idx++
		}
		sub, _, ok := firstPositional(args[idx:], nil)
		if ok && (sub == "fmt" || sub == "clippy") {
			return "cargo " + sub
		}
		return ""
	case "deno":
		sub, _, ok := firstPositional(args, nil)
		if ok && (sub == "fmt" || sub == "lint") {
			return "deno " + sub
		}
		return ""
	case "buf":
		sub, _, ok := firstPositional(args, nil)
		if ok && (sub == "lint" || sub == "breaking") {
			return "buf " + sub
		}
		return ""
	case "uv":
		sub, rest, ok := firstPositional(args, nil)
		if ok && sub == "run" {
			inner, innerRest, ok := firstPositional(rest, uvRunValueFlags)
			if ok && inner != "" {
				return detectDirectLintCommand(append([]string{inner}, innerRest...))
			}
		}
		return ""
	}
	if pythonBinRE.MatchString(cmd) {
		i := 0
		for i < len(args) {
			if args[i] == "-m" && i+1 < len(args) {
				modRoot := args[i+1]
				if j := strings.IndexByte(modRoot, '.'); j >= 0 {
					modRoot = modRoot[:j]
				}
				if modRoot == "ruff" || modRoot == "bandit" || modRoot == "pyrefly" {
					return cmd + " -m " + modRoot
				}
				return ""
			}
			if args[i] == "-c" {
				return ""
			}
			if args[i] == "-W" || args[i] == "-X" || args[i] == "-Q" {
				i += 2
				continue
			}
			if !strings.HasPrefix(args[i], "-") {
				if isWrappedStaticScript(args[i]) {
					return path.Base(args[i])
				}
				return ""
			}
			i++
		}
		return ""
	}
	if cmd == "bash" || cmd == "sh" {
		i := 0
		for i < len(args) {
			if args[i] == "-c" {
				return ""
			}
			if args[i] == "-o" || args[i] == "+o" {
				i += 2
				continue
			}
			if strings.HasPrefix(args[i], "-") || strings.HasPrefix(args[i], "+") {
				i++
				continue
			}
			if isWrappedStaticScript(args[i]) {
				return path.Base(args[i])
			}
			return ""
		}
		return ""
	}
	if cmd == "bunx" || cmd == "npx" {
		inner, _, ok := firstPositional(args, map[string]struct{}{"-p": {}, "--package": {}})
		if ok && inner != "" {
			baseSrc := inner
			if !strings.HasPrefix(inner, "@") {
				if j := strings.IndexByte(inner, '@'); j >= 0 {
					baseSrc = inner[:j]
				}
			}
			innerBase := path.Base(baseSrc)
			if innerBase == "biome" || innerBase == "svelte-check" || strings.HasPrefix(inner, "@biomejs/biome") {
				return cmd + " " + innerBase
			}
		}
		return ""
	}
	if (cmd == "poetry" || cmd == "pipenv" || cmd == "pdm") && len(args) > 0 && args[0] == "run" {
		return detectDirectLintCommand(args[1:])
	}
	return ""
}
