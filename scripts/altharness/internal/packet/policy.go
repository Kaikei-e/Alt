package packet

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	timeFieldRe      = regexp.MustCompile(`(?i)(?:^|_)(?:timeout|minutes?|seconds?|hours?|duration|deadline|time_limit|time_budget|ttl)(?:$|_)`)
	nohupRe          = regexp.MustCompile("(?:^|[\\s`('\"])nohup\\s+")
	prohibitionCueRe = regexp.MustCompile(`(?i)(?:禁止|不可|しない|使わない|実行しない|避ける|原則禁止|❌|\b(?:ng|do\s+not|don't|never|forbid(?:den)?|prohibit(?:ed)?|must\s+not|disallow(?:ed)?)\b)`)
	shellNGCommentRe = regexp.MustCompile(`(?i)(?:#\s*(?:NG|禁止|DO\s+NOT|FORBIDDEN)|❌)`)
	bgProseRe        = regexp.MustCompile(`(?i)(?:バックグラウンド(?:で|実行)|背景実行|run\s+in\s+(?:the\s+)?background)`)
	inlineCodeRe     = regexp.MustCompile("`([^`\n]+)`")
	runShCallRe      = regexp.MustCompile(`(?:^|[\s;&|()])(?:\./)?scripts/lint/run\.sh\s+(?:--policy(?:=|\s+)(?:"[^"\n]+"|'[^'\n]+'|[^\s;&|()'"]+)\s+)?(?:"([^"\n]+)"|'([^'\n]+)'|([^\s;&|()'"]+))`)
	makeLintRe       = regexp.MustCompile(`(?:^|[\s;&|()])make\s+([^\n;&|)]*)`)
	makeLintTargetRe = regexp.MustCompile(`\bTARGET=(?:"([^"\n]*)"|'([^'\n]*)'|([^\s;&|()'"#]+))`)
	cmdLintPolicyRe  = regexp.MustCompile(`(?:^|[\s;&|()])(?:LINT_POLICY=|--policy(?:=|\s+))(?:"([^"\n]+)"|'([^'\n]+)'|([^\s;&|()'"]+))`)
	packetIDRe       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$`)
	targetBodyRe     = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	allZeroSHARe     = regexp.MustCompile(`^0+$`)

	packetHeadings = []headingRule{
		{regexp.MustCompile(`^##\s+1\.\s*範囲と作業量制約`), "## 1. 範囲と作業量制約"},
		{regexp.MustCompile(`^##\s+2\.\s*実行・検証手順`), "## 2. 実行・検証手順"},
		{regexp.MustCompile(`^##\s+3\.\s*Hand-?off\s*記述必須項目`), "## 3. Hand-off 記述必須項目"},
	}
	handoffHeadings = []headingRule{
		{regexp.MustCompile(`^#{2,3}\s+(?:1\.\s*)?.*修正ファイルと行数`), "## 1. 修正ファイルと行数"},
		{regexp.MustCompile(`^#{2,3}\s+(?:2\.\s*)?.*実行時挙動の変更可能性の有無`), "## 2. 実行時挙動の変更可能性の有無"},
		{regexp.MustCompile(`^#{2,3}\s+(?:3\.\s*)?.*変更理由`), "## 3. 変更理由"},
	}
)

type headingRule struct {
	re    *regexp.Regexp
	label string
}

var shellFenceLangs = map[string]struct{}{
	"": {}, "sh": {}, "bash": {}, "zsh": {}, "shell": {}, "console": {},
}

type autofixPat struct {
	label string
	re    *regexp.Regexp
}

var forbiddenAutofix = []autofixPat{
	{"golangci-lint --fix", regexp.MustCompile(`\bgolangci-lint\b[^\n;|&]*--fix\b`)},
	{"cargo clippy --fix", regexp.MustCompile(`\bcargo\s+clippy\b[^\n;|&]*--fix\b`)},
	{"cargo fix", regexp.MustCompile(`\bcargo\s+fix\b`)},
	{"ruff check --unsafe-fixes", regexp.MustCompile(`\bruff\s+check\b[^\n;|&]*--unsafe-fixes\b`)},
	{"biome --unsafe", regexp.MustCompile(`\bbiome\s+(?:check|lint)\b[^\n;|&]*--unsafe\b`)},
}

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
		if ch == '#' && (i == 0 || isASCIISpace(command[i-1])) {
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
	for _, item := range forbiddenAutofix {
		if item.re.MatchString(text) {
			return item.label
		}
	}
	return ""
}

func normalizeTarget(target string) string {
	target = strings.TrimSpace(target)
	target = strings.TrimPrefix(target, "./")
	return strings.TrimRight(target, "/")
}

func validTarget(target string) bool {
	if target == "" || strings.Contains(target, "..") || strings.HasPrefix(target, "/") {
		return false
	}
	return targetBodyRe.MatchString(target)
}

func extractPacketTargets(data map[string]any) []string {
	scope, ok := asMap(data["scope"])
	if !ok {
		return nil
	}
	switch raw := scope["target"].(type) {
	case string:
		return []string{normalizeTarget(raw)}
	case []any:
		var out []string
		for _, item := range raw {
			s, ok := item.(string)
			if ok {
				out = append(out, normalizeTarget(s))
			}
		}
		return out
	default:
		return nil
	}
}

func firstGroup(m []string) string {
	for i := 1; i < len(m); i++ {
		if m[i] != "" {
			return m[i]
		}
	}
	return ""
}

func extractInvokedLintTargets(command string) []string {
	var targets []string
	for _, m := range runShCallRe.FindAllStringSubmatch(command, -1) {
		raw := firstGroup(m)
		if raw != "" {
			targets = append(targets, normalizeTarget(raw))
		}
	}
	for _, m := range makeLintRe.FindAllStringSubmatch(command, -1) {
		args := ""
		if len(m) > 1 {
			args = m[1]
		}
		var tokens []string
		for _, tok := range strings.Fields(args) {
			if strings.HasPrefix(tok, "-") || strings.Contains(tok, "=") {
				continue
			}
			tokens = append(tokens, tok)
		}
		hasLint := false
		for _, tok := range tokens {
			if tok == "lint" {
				hasLint = true
				break
			}
		}
		if !hasLint {
			continue
		}
		tm := makeLintTargetRe.FindStringSubmatch(args)
		if tm == nil {
			targets = append(targets, "all")
			continue
		}
		val := firstGroup(tm)
		if val == "" {
			val = "all"
		}
		targets = append(targets, normalizeTarget(val))
	}
	return targets
}

func checkVerificationCommand(command, jsonPath string, expected []string, forbidRepoWide bool, issues *[]Issue) {
	if bad := matchForbiddenAutofix(command); bad != "" {
		*issues = append(*issues, Issue{
			Code:    "FORBIDDEN_AUTOFIX_COMMAND",
			Path:    jsonPath,
			Message: fmt.Sprintf("verification command contains forbidden GUARD-01 bulk autofix (%s)", bad),
		})
	}
	if hasBackgroundAmpersand(command) {
		*issues = append(*issues, Issue{
			Code:    "FORBIDDEN_BACKGROUND_EXECUTION",
			Path:    jsonPath,
			Message: "verification command must run in foreground only (no '&' or 'nohup')",
		})
	}
	invoked := extractInvokedLintTargets(command)
	if forbidRepoWide {
		for _, t := range invoked {
			if t == "all" {
				*issues = append(*issues, Issue{
					Code:    "FORBIDDEN_REPO_WIDE_COMMAND",
					Path:    jsonPath,
					Message: "forbid_repo_wide_test_build=true forbids repo-wide lint ('./scripts/lint/run.sh all' or 'make lint')",
				})
				break
			}
		}
	}
	if len(expected) == 0 {
		return
	}
	invokedSet := map[string]struct{}{}
	for _, t := range invoked {
		invokedSet[t] = struct{}{}
	}
	for _, target := range expected {
		if _, ok := invokedSet[target]; !ok {
			*issues = append(*issues, Issue{
				Code:    "MISSING_VERIFICATION_TARGET",
				Path:    jsonPath,
				Message: fmt.Sprintf("verification command must invoke './scripts/lint/run.sh %s'", target),
			})
		}
	}
}

func checkMarkdownHeadings(body []string, required []headingRule, issues *[]Issue) {
	nonFenced, _ := splitMarkdownBlocks(body)
	var positions []struct {
		line  int
		label string
	}
	for _, rule := range required {
		found := -1
		for _, line := range nonFenced {
			if rule.re.MatchString(strings.TrimSpace(line.text)) {
				found = line.no
				break
			}
		}
		if found < 0 {
			*issues = append(*issues, Issue{
				Code:    "MISSING_MARKDOWN_SECTION",
				Path:    "markdown",
				Message: fmt.Sprintf("missing required heading '%s'", rule.label),
			})
		} else {
			positions = append(positions, struct {
				line  int
				label string
			}{found, rule.label})
		}
	}
	if len(positions) != len(required) {
		return
	}
	sorted := true
	for i := 1; i < len(positions); i++ {
		if positions[i].line < positions[i-1].line {
			sorted = false
			break
		}
	}
	if !sorted {
		*issues = append(*issues, Issue{
			Code:    "MARKDOWN_SECTION_ORDER",
			Path:    "markdown",
			Message: "required Markdown headings appear out of order",
		})
	}
}

func checkPacketMarkdownBody(body []string, issues *[]Issue) {
	nonFenced, blocks := splitMarkdownBlocks(body)
	for _, block := range blocks {
		if _, ok := shellFenceLangs[strings.ToLower(block.lang)]; !ok {
			continue
		}
		for _, line := range block.lines {
			stripped := strings.TrimSpace(line.text)
			if stripped == "" || strings.HasPrefix(stripped, "#") || shellNGCommentRe.MatchString(line.text) {
				continue
			}
			if bad := matchForbiddenAutofix(line.text); bad != "" {
				*issues = append(*issues, Issue{
					Code:    "FORBIDDEN_AUTOFIX_COMMAND",
					Path:    fmt.Sprintf("markdown:line:%d", line.no),
					Message: fmt.Sprintf("shell block instructs forbidden GUARD-01 autofix (%s)", bad),
				})
			}
			if hasBackgroundAmpersand(line.text) {
				*issues = append(*issues, Issue{
					Code:    "FORBIDDEN_BACKGROUND_EXECUTION",
					Path:    fmt.Sprintf("markdown:line:%d", line.no),
					Message: "shell block instructs forbidden background execution ('&' or 'nohup')",
				})
			}
		}
	}
	for _, line := range nonFenced {
		if prohibitionCueRe.MatchString(line.text) {
			continue
		}
		if bad := matchForbiddenAutofix(line.text); bad != "" {
			*issues = append(*issues, Issue{
				Code:    "FORBIDDEN_AUTOFIX_COMMAND",
				Path:    fmt.Sprintf("markdown:line:%d", line.no),
				Message: fmt.Sprintf("packet body affirmatively instructs forbidden autofix (%s)", bad),
			})
		}
		inlineBG := false
		for _, span := range inlineCodeRe.FindAllStringSubmatch(line.text, -1) {
			if len(span) > 1 && len(strings.TrimSpace(span[1])) > 1 && hasBackgroundAmpersand(span[1]) {
				inlineBG = true
				break
			}
		}
		if inlineBG || bgProseRe.MatchString(line.text) || nohupRe.MatchString(line.text) {
			*issues = append(*issues, Issue{
				Code:    "FORBIDDEN_BACKGROUND_EXECUTION",
				Path:    fmt.Sprintf("markdown:line:%d", line.no),
				Message: "packet body affirmatively instructs background execution",
			})
		}
	}
}

func expandBraces(pattern string) []string {
	start := -1
	depth := 0
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start != -1 {
				prefix := pattern[:start]
				inner := pattern[start+1 : i]
				suffix := pattern[i+1:]
				opts := splitBraceOptions(inner)
				var expanded []string
				for _, opt := range opts {
					expanded = append(expanded, expandBraces(prefix+opt+suffix)...)
				}
				if len(expanded) == 0 {
					return []string{""}
				}
				return expanded
			}
		}
	}
	return []string{pattern}
}

func splitBraceOptions(inner string) []string {
	var opts []string
	var buf []byte
	depth := 0
	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		switch ch {
		case '{':
			depth++
			buf = append(buf, ch)
		case '}':
			depth--
			buf = append(buf, ch)
		case ',':
			if depth == 0 {
				opts = append(opts, string(buf))
				buf = buf[:0]
				continue
			}
			buf = append(buf, ch)
		default:
			buf = append(buf, ch)
		}
	}
	opts = append(opts, string(buf))
	return opts
}

func globToRegex(pattern string) *regexp.Regexp {
	pat := strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	if strings.HasSuffix(pat, "/") {
		pat += "**"
	}
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pat); {
		if strings.HasPrefix(pat[i:], "**/") {
			b.WriteString(`(?:.+/)?`)
			i += 3
			continue
		}
		if strings.HasPrefix(pat[i:], "/**") && i+3 == len(pat) {
			b.WriteString(`/.+`)
			i += 3
			continue
		}
		if strings.HasPrefix(pat[i:], "**") {
			b.WriteString(`.*`)
			i += 2
			continue
		}
		switch pat[i] {
		case '*':
			b.WriteString(`[^/]*`)
		case '?':
			b.WriteString(`[^/]`)
		default:
			b.WriteString(regexp.QuoteMeta(pat[i : i+1]))
		}
		i++
	}
	b.WriteByte('$')
	return regexp.MustCompile(b.String())
}

func matchesAnyGlob(filePath string, patterns []string) bool {
	norm := normalizeRepoRel(filePath)
	for _, pattern := range patterns {
		for _, alt := range expandBraces(pattern) {
			if globToRegex(normalizeRepoRel(alt)).MatchString(norm) {
				return true
			}
		}
	}
	return false
}

func stringList(v any) []string {
	items, ok := asSlice(v)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range items {
		s, ok := item.(string)
		if ok {
			out = append(out, s)
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func extractCommandPolicies(command string) []string {
	var policies []string
	for _, m := range cmdLintPolicyRe.FindAllStringSubmatch(command, -1) {
		val := strings.TrimSpace(firstGroup(m))
		if val != "" {
			policies = append(policies, val)
		}
	}
	return policies
}

func checkPolicyCommandMatch(command, declared, jsonPath string, issues *[]Issue) {
	explicit := extractCommandPolicies(command)
	if declared == "block-new" && len(explicit) == 0 {
		*issues = append(*issues, Issue{
			Code:    "POLICY_COMMAND_MISMATCH",
			Path:    jsonPath,
			Message: "execution_policy.policy='block-new' requires verification command to explicitly include '--policy block-new' or 'LINT_POLICY=block-new'",
		})
	}
	for _, item := range explicit {
		if item != declared {
			*issues = append(*issues, Issue{
				Code:    "POLICY_COMMAND_MISMATCH",
				Path:    jsonPath,
				Message: fmt.Sprintf("execution_policy.policy=%q conflicts with command policy %q", declared, item),
			})
		}
	}
}

func resolveCommandBudget(policy map[string]any) int64 {
	raw, ok := asInt(policy["max_command_budget_s"])
	if !ok {
		if _, present := policy["max_command_budget_s"]; !present {
			return defaultCommandBudgetS
		}
		return defaultCommandBudgetS
	}
	if raw >= 1 && raw <= maxBudgetSeconds {
		return raw
	}
	return defaultCommandBudgetS
}

func validateMeasuredExecution(policy map[string]any, verifyCmd any, issues *[]Issue) {
	measuredVal, present := policy["measured"]
	if !present || measuredVal == nil {
		filtered := (*issues)[:0]
		for _, item := range *issues {
			if item.Path == "$.execution_policy.measured" && (item.Code == "SCHEMA_REQUIRED" || item.Code == "SCHEMA_TYPE") {
				continue
			}
			filtered = append(filtered, item)
		}
		*issues = append(filtered, Issue{
			Code:    "UNMEASURED_VERIFICATION_COMMAND",
			Path:    "$.execution_policy.measured",
			Message: "LOOP-02: execution_policy.measured is required; run './scripts/altharness/alt-harness packet measure <packet>' before dispatch",
		})
		return
	}
	measured, ok := asMap(measuredVal)
	if !ok {
		return
	}
	budget := resolveCommandBudget(policy)
	if duration, ok := asFloat(measured["duration_s"]); ok && !isBool(measured["duration_s"]) && duration > float64(budget) {
		*issues = append(*issues, Issue{
			Code:    "VERIFICATION_BUDGET_EXCEEDED",
			Path:    "$.execution_policy.measured.duration_s",
			Message: fmt.Sprintf("measured duration_s=%v exceeds max_command_budget_s=%d", duration, budget),
		})
	}
	if at, ok := asString(measured["at_commit"]); ok && allZeroSHARe.MatchString(at) {
		*issues = append(*issues, Issue{
			Code:    "PLACEHOLDER_MEASURED_COMMIT",
			Path:    "$.execution_policy.measured.at_commit",
			Message: "measured.at_commit must be a real non-zero git commit SHA; run './scripts/altharness/alt-harness packet measure'",
		})
	}
	cmdSHA, shaOK := asString(measured["command_sha256"])
	verify, verifyOK := verifyCmd.(string)
	if shaOK && cmdSHA != "" && verifyOK && strings.TrimSpace(verify) != "" {
		expected := commandSHA256(verify)
		if cmdSHA != expected {
			*issues = append(*issues, Issue{
				Code:    "STALE_MEASURED_COMMAND",
				Path:    "$.execution_policy.measured.command_sha256",
				Message: fmt.Sprintf("measured command_sha256=%s does not match verification_command (%s); re-run './scripts/altharness/alt-harness packet measure'", cmdSHA, expected),
			})
		}
	}
}
