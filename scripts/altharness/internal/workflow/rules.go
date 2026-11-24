package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	ghaOpen         = "$" + "{{"
	clientProject   = "client"
	browserEnv      = "VITEST_BROWSER"
	frontendPkgJSON = "alt-frontend-sv/package.json"
	minSwallowLen   = 10
)

var (
	githubHostedRE = regexp.MustCompile(`^(ubuntu|windows|macos)-[a-z0-9.-]+$`)
	shaPinRE       = regexp.MustCompile(`@[0-9a-f]{40}$`)
	dockerDigestRE = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
	untrustedRE    = regexp.MustCompile(`github\.(?:event\.(?:issue|pull_request|comment|review|review_comment|discussion|pages|commits|head_commit|workflow_run|inputs)?|head_ref|ref_name)`)
	templateExprRE = regexp.MustCompile(`(?s)` + regexp.QuoteMeta(ghaOpen) + `(.*?)}}`)
	globCharRE     = regexp.MustCompile(`[*?+\[]`)
	unsupportedRE  = regexp.MustCompile(`[?+\[]`)
	runScriptRE    = regexp.MustCompile(`\b(?:bun|bunx|npm|pnpm|yarn)\s+run\s+([A-Za-z0-9:_.-]+)`)
	inlineEnvRE    = regexp.MustCompile(`\b([A-Z_][A-Z0-9_]*)=(\S+)`)
	projectFlagRE  = regexp.MustCompile(`--project(?:=|\s+)([A-Za-z0-9:_.-]+)`)
	commandSplitRE = regexp.MustCompile(`&&|\|\||;|\n|\|`)
	vitestRE       = regexp.MustCompile(`\bvitest\b`)
	playwrightRE   = regexp.MustCompile(`\bplaywright\b`)
	lintJobNameRE  = regexp.MustCompile(`(?i)(?:^|[-_])(?:lint|quality-gates?|static-checks)(?:$|[-_])|^quality$`)
	lintRunCmdRE   = regexp.MustCompile(`(?:^|[;&|(\s"'/])(?:\./)?(?:scripts/lint/run\.sh|scripts/altharness/alt-harness)\b|\bmake\s+(?:[^\n;|&]*\s+)?lint\b`)
	lintStepIfRE   = regexp.MustCompile(`(?i)\bjob-type\s*==\s*['"]lint['"]`)
	swallowOKLine  = regexp.MustCompile(`^#\s*swallow-ok:(.*)$`)
	swallowOKSame  = regexp.MustCompile(`(?:^|\s)#\s*swallow-ok:(.*)$`)
)

var forbiddenTriggers = map[string]struct{}{
	"pull_request_target": {},
	"workflow_run":        {},
}

var forbiddenLintActions = map[string]struct{}{
	"golangci/golangci-lint-action": {},
	"rhysd/actionlint":              {},
	"reviewdog/action-actionlint":   {},
	"raven-actions/actionlint":      {},
	"astral-sh/ruff-action":         {},
	"chartboost/ruff-action":        {},
	"bufbuild/buf-lint-action":      {},
	"bufbuild/buf-breaking-action":  {},
	"actions-rs/clippy-check":       {},
	"clechasseur/rs-clippy-check":   {},
	"securego/gosec":                {},
}

var forbiddenLintImages = map[string]struct{}{
	"actionlint": {}, "golangci-lint": {}, "gosec": {}, "govulncheck": {},
	"ruff": {}, "bandit": {}, "pyrefly": {}, "biome": {}, "svelte-check": {}, "buf": {},
}

func checkWorkflowDoc(path string, doc any) []string {
	errors, _ := checkWorkflowDocDetailsIn(path, doc, ".")
	return errors
}

func checkWorkflowDocIn(path string, doc any, repoRoot string) []string {
	errors, _ := checkWorkflowDocDetailsIn(path, doc, repoRoot)
	return errors
}

func checkWorkflowText(path, text string) []string {
	errors, _ := checkWorkflowTextDetails(path, text, true)
	return errors
}

func checkWorkflowTextDetails(path, text string, includeLint bool) ([]string, int) {
	var errors []string
	bad := 0
	lines := splitWorkflowLines(text)
	for idx, raw := range lines {
		stripped := strings.TrimLeftFunc(raw, unicode.IsSpace)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}
		code := stripInlineComment(raw)
		if len(findSwallows(code)) > 0 && !hasSwallowOK(lines, idx) {
			lineno := idx + 1
			errors = append(errors, fmt.Sprintf(
				"::error file=%s,line=%d::%s:%d: swallowed failure without a reason. Add '# swallow-ok: <why ignoring this exit code is safe>' on the line above, or let the step fail.",
				path, lineno, path, lineno,
			))
			bad++
		}
	}
	if includeLint {
		doc, err := parseYAML(text)
		if err == nil {
			if m, ok := doc.(ymap); ok {
				if _, isJobs := m.get("jobs"); isJobs {
					if jobs, ok := asMap(mustGet(m, "jobs")); ok {
						_ = jobs
					}
					if _, ok := asMap(mustGet(m, "jobs")); ok {
						lintErrors, lintBad := checkLintSwallows(path, m)
						errors = append(errors, lintErrors...)
						bad += lintBad
					}
				}
			}
		}
	}
	return errors, bad
}

func mustGet(m ymap, key string) any {
	v, _ := m.get(key)
	return v
}

func checkWorkflowDocDetails(path string, doc any) ([]string, int) {
	return checkWorkflowDocDetailsIn(path, doc, ".")
}

func checkWorkflowDocDetailsIn(path string, doc any, repoRoot string) ([]string, int) {
	m, ok := doc.(ymap)
	if !ok {
		return nil, 0
	}
	var errors []string
	bad := 0
	names := triggerNames(m)
	var used []string
	for name := range names {
		if _, forbid := forbiddenTriggers[name]; forbid {
			used = append(used, name)
		}
	}
	sort.Strings(used)
	if len(used) > 0 {
		for _, t := range used {
			errors = append(errors, fmt.Sprintf("::error file=%s::workflow uses forbidden trigger %s (ADR-000763 security hardening).", path, pyReprString(t)))
		}
		bad++
	}

	required := requiredWorkflowPaths(path, m)
	if triggers, ok := triggerValue(m); ok {
		if tmap, ok := triggers.(ymap); ok {
			for _, trigName := range []string{"push", "pull_request"} {
				rawCfg, exists := tmap.get(trigName)
				cfg, ok := rawCfg.(ymap)
				if !exists || !ok {
					continue
				}
				pathsRaw, exists := cfg.get("paths")
				paths, ok := pathsRaw.([]any)
				if !exists || !ok {
					continue
				}
				for _, rawEntry := range paths {
					entry, ok := rawEntry.(string)
					if !ok {
						continue
					}
					if unsupportedRE.MatchString(entry) {
						errors = append(errors, fmt.Sprintf("::error file=%s::on.%s.paths entry %s uses unsupported glob pattern character ('?', '+', or '['); use only literal paths, '*', and '**'.", path, trigName, pyReprString(entry)))
						bad++
					}
					target := resolvePathFilterTarget(entry)
					if _, err := os.Stat(joinRepo(repoRoot, target)); err != nil {
						errors = append(errors, fmt.Sprintf("::error file=%s::on.%s.paths entry %s resolves to missing path %s.", path, trigName, pyReprString(entry), pyReprString(target)))
						bad++
					}
				}
				for _, req := range required {
					if !pathsFilterMatches(paths, req) {
						errors = append(errors, fmt.Sprintf("::error file=%s::on.%s.paths does not match %s (workflows using `paths:` must trigger on their own file and every local reusable workflow they call).", path, trigName, pyReprString(req)))
						bad++
					}
				}
			}
		}
	}

	jobs, _ := asMap(mustGet(m, "jobs"))
	for i := range jobs.k {
		jobName := jobs.k[i]
		job, ok := jobs.v[i].(ymap)
		if !ok {
			continue
		}
		if _, hasUses := job.get("uses"); hasUses {
			if _, hasRuns := job.get("runs-on"); !hasRuns {
				continue
			}
		}
		labels, hadExpr := extractLabels(mustGet(job, "runs-on"))
		if _, exists := job.get("runs-on"); !exists {
			labels, hadExpr = nil, false
		}
		if hadExpr {
			errors = append(errors, fmt.Sprintf("::error file=%s::job %s has an expression-derived runs-on value. Static analysis cannot confirm it is GitHub-hosted — pin the runner literal (ADR-000763: this repo registers no self-hosted runner).", path, pyReprString(jobName)))
			bad++
		} else if len(labels) == 0 || !allHosted(labels) {
			errors = append(errors, fmt.Sprintf("::error file=%s::job %s runs on %s. Only static GitHub-hosted labels are allowed here; self-hosted runners live in the private deploy repo (ADR-000763 security hardening).", path, pyReprString(jobName), pyList(labels)))
			bad++
		}
		if !hasValidTimeout(job) {
			errors = append(errors, fmt.Sprintf("::error file=%s::job %s has no valid `timeout-minutes` (must be a positive number so hung jobs do not burn the 360-minute GitHub default).", path, pyReprString(jobName)))
			bad++
		}
		if container, exists := job.get("container"); exists && container != nil {
			img := containerImage(container)
			if !imageDigestPinned(img) {
				errors = append(errors, fmt.Sprintf("::error file=%s::job %s container image %s is not pinned by @sha256: digest.", path, pyReprString(jobName), pyRepr(img)))
				bad++
			}
		}
		if servicesRaw, exists := job.get("services"); exists {
			if services, ok := servicesRaw.(ymap); ok {
				for si := range services.k {
					svcName := services.k[si]
					img := serviceImage(services.v[si])
					if !imageDigestPinned(img) {
						errors = append(errors, fmt.Sprintf("::error file=%s::job %s service %s image %s is not pinned by @sha256: digest.", path, pyReprString(jobName), pyReprString(svcName), pyRepr(img)))
						bad++
					}
				}
			}
		}
	}

	if _, exists := m.get("permissions"); !exists {
		errors = append(errors, fmt.Sprintf("::error file=%s::workflow has no top-level `permissions:` block. Declare it (`contents: read` unless a job needs more) so the token scope does not depend on the repository default.", path))
		bad++
	}

	for i := range jobs.k {
		jobName := jobs.k[i]
		job, ok := jobs.v[i].(ymap)
		if !ok {
			continue
		}
		steps, _ := asSeq(mustGet(job, "steps"))
		for _, stepRaw := range steps {
			step, ok := stepRaw.(ymap)
			if !ok {
				continue
			}
			if usesRaw, exists := step.get("uses"); exists {
				if uses, ok := usesRaw.(string); ok {
					if strings.HasPrefix(uses, "actions/checkout@") && !persistCredentialsFalse(step) {
						errors = append(errors, fmt.Sprintf("::error file=%s::job %s checks out without `persist-credentials: false`. Nothing in this repo pushes from CI, so the GITHUB_TOKEN must not be written into .git/config.", path, pyReprString(jobName)))
						bad++
					}
					if !isPinned(uses) {
						errors = append(errors, fmt.Sprintf("::error file=%s::job %s uses %s, which is not pinned to a commit SHA (or @sha256 digest for docker://). Tags and branches are mutable supply-chain inputs.", path, pyReprString(jobName), pyReprString(uses)))
						bad++
					}
					if forbiddenLintUses(step) {
						errors = append(errors, fmt.Sprintf("::error file=%s::job %s uses lint action/image %s; invoke ./scripts/lint/run.sh instead so CI and local development run the same command.", path, pyReprString(jobName), pyReprString(uses)))
						bad++
					}
				}
			}
			if runRaw, exists := step.get("run"); exists {
				if run, ok := runRaw.(string); ok {
					for _, match := range templateExprRE.FindAllStringSubmatch(run, -1) {
						if len(match) < 2 {
							continue
						}
						expr := match[1]
						if untrustedRE.MatchString(expr) {
							errors = append(errors, fmt.Sprintf("::error file=%s::job %s interpolates %s into a `run:` body. github.event.* and github.head_ref are attacker-controlled on pull_request; pass them through `env:` and read the variable in the shell.", path, pyReprString(jobName), pyReprString(strings.TrimSpace(expr))))
							bad++
						}
					}
					for _, tool := range findDirectLintInvocations(run) {
						errors = append(errors, fmt.Sprintf("::error file=%s::job %s invokes %s directly in a `run:` block; invoke ./scripts/lint/run.sh instead so CI and local development run the same command.", path, pyReprString(jobName), pyReprString(tool)))
						bad++
					}
				}
			}
		}
	}

	lintErrors, lintBad := checkLintSwallows(path, m)
	errors = append(errors, lintErrors...)
	bad += lintBad
	return errors, bad
}

func triggerValue(doc ymap) (any, bool) {
	if v, ok := doc.get("on"); ok {
		return v, true
	}
	if v, ok := doc.get("true"); ok {
		return v, true
	}
	return nil, false
}

func triggerNames(doc ymap) map[string]struct{} {
	out := map[string]struct{}{}
	v, ok := triggerValue(doc)
	if !ok || v == nil {
		return out
	}
	switch t := v.(type) {
	case ymap:
		for _, k := range t.k {
			out[k] = struct{}{}
		}
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok {
				out[s] = struct{}{}
			}
		}
	case string:
		out[t] = struct{}{}
	}
	return out
}

func requiredWorkflowPaths(path string, doc ymap) []string {
	own := strings.TrimSpace(path)
	own = strings.TrimPrefix(own, "./")
	if !strings.Contains(own, "/") {
		own = ".github/workflows/" + own
	}
	required := []string{own}
	jobs, _ := asMap(mustGet(doc, "jobs"))
	for i := range jobs.k {
		job, ok := jobs.v[i].(ymap)
		if !ok {
			continue
		}
		usesRaw, ok := job.get("uses")
		if !ok {
			continue
		}
		uses, ok := usesRaw.(string)
		if !ok || !strings.HasPrefix(uses, "./.github/workflows/") {
			continue
		}
		reusable := uses[2:]
		seen := false
		for _, existing := range required {
			if existing == reusable {
				seen = true
				break
			}
		}
		if !seen {
			required = append(required, reusable)
		}
	}
	return required
}

func resolvePathFilterTarget(entry string) string {
	clean := strings.TrimLeft(entry, "!")
	loc := globCharRE.FindStringIndex(clean)
	if loc == nil {
		clean = strings.TrimRight(clean, "/")
		if clean == "" {
			return "."
		}
		return clean
	}
	prefix := clean[:loc[0]]
	if strings.HasSuffix(prefix, "/") {
		prefix = strings.TrimRight(prefix, "/")
		if prefix == "" {
			return "."
		}
		return prefix
	}
	if strings.Contains(prefix, "/") {
		prefix = prefix[:strings.LastIndex(prefix, "/")]
		if prefix == "" {
			return "."
		}
		return prefix
	}
	return "."
}

func githubGlobMatches(pattern, candidate string) bool {
	pat := strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	cand := strings.TrimPrefix(strings.TrimSpace(candidate), "./")
	if unsupportedRE.MatchString(pat) {
		return false
	}
	if !globCharRE.MatchString(pat) {
		return strings.TrimRight(pat, "/") == strings.TrimRight(cand, "/")
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pat); {
		switch {
		case strings.HasPrefix(pat[i:], "**/"):
			b.WriteString(`(?:.+/)?`)
			i += 3
		case strings.HasPrefix(pat[i:], "/**") && i+3 == len(pat):
			b.WriteString(`/.+`)
			i += 3
		case strings.HasPrefix(pat[i:], "**"):
			b.WriteString(`.*`)
			i += 2
		case pat[i] == '*':
			b.WriteString(`[^/]*`)
			i++
		default:
			b.WriteString(regexp.QuoteMeta(pat[i : i+1]))
			i++
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(cand)
}

func pathsFilterMatches(paths []any, candidate string) bool {
	matched := false
	for _, raw := range paths {
		entry, ok := raw.(string)
		if !ok {
			continue
		}
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, "!") {
			neg := strings.TrimSpace(strings.TrimLeft(entry, "!"))
			if neg != "" && githubGlobMatches(neg, candidate) {
				matched = false
			}
		} else if githubGlobMatches(entry, candidate) {
			matched = true
		}
	}
	return matched
}

func extractLabels(runsOn any) ([]string, bool) {
	switch t := runsOn.(type) {
	case nil:
		return nil, false
	case string:
		return []string{t}, strings.Contains(t, ghaOpen)
	case []any:
		labels := make([]string, 0, len(t))
		had := false
		for _, item := range t {
			s := pyStr(item)
			labels = append(labels, s)
			if strings.Contains(s, ghaOpen) {
				had = true
			}
		}
		return labels, had
	case ymap:
		var labels []string
		if raw, ok := t.get("labels"); ok {
			switch lab := raw.(type) {
			case string:
				labels = []string{lab}
			case []any:
				for _, item := range lab {
					labels = append(labels, pyStr(item))
				}
			}
		}
		had := false
		for _, label := range labels {
			if strings.Contains(label, ghaOpen) {
				had = true
			}
		}
		if group, ok := t.get("group"); ok {
			if s, ok := group.(string); ok && strings.Contains(s, ghaOpen) {
				had = true
			}
		}
		return labels, had
	default:
		return nil, false
	}
}

func allHosted(labels []string) bool {
	if len(labels) == 0 {
		return false
	}
	for _, label := range labels {
		if !githubHostedRE.MatchString(label) {
			return false
		}
	}
	return true
}

func isPinned(uses string) bool {
	if strings.HasPrefix(uses, "./") {
		return true
	}
	if strings.HasPrefix(uses, "docker://") {
		return dockerDigestRE.MatchString(uses)
	}
	return shaPinRE.MatchString(uses)
}

func imageDigestPinned(image any) bool {
	s, ok := image.(string)
	if !ok {
		return false
	}
	stripped := strings.TrimSpace(s)
	return stripped != "" && !strings.HasPrefix(stripped, "@") && dockerDigestRE.MatchString(stripped)
}

func hasValidTimeout(job ymap) bool {
	timeout, ok := job.get("timeout-minutes")
	if !ok || timeout == nil {
		return false
	}
	switch t := timeout.(type) {
	case string:
		return strings.Contains(t, ghaOpen)
	case bool:
		return false
	case int64:
		return t > 0
	case int:
		return t > 0
	case float64:
		return t > 0
	default:
		return false
	}
}

func containerImage(container any) any {
	switch t := container.(type) {
	case string:
		return t
	case ymap:
		img, _ := t.get("image")
		return img
	default:
		return nil
	}
}

func serviceImage(svc any) any {
	switch t := svc.(type) {
	case string:
		return t
	case ymap:
		img, _ := t.get("image")
		return img
	default:
		return nil
	}
}

func persistCredentialsFalse(step ymap) bool {
	withRaw, ok := step.get("with")
	if !ok {
		return false
	}
	with, ok := withRaw.(ymap)
	if !ok {
		return false
	}
	persist, ok := with.get("persist-credentials")
	if !ok {
		return false
	}
	b, ok := persist.(bool)
	return ok && !b
}

func forbiddenLintUses(step ymap) bool {
	usesRaw, ok := step.get("uses")
	if !ok {
		return false
	}
	uses, ok := usesRaw.(string)
	if !ok {
		return false
	}
	ref := strings.TrimSpace(uses)
	if strings.HasPrefix(ref, "./") {
		return false
	}
	refBase := ref
	if i := strings.IndexByte(refBase, '@'); i >= 0 {
		refBase = refBase[:i]
	}
	refBase = strings.ToLower(refBase)
	if strings.HasPrefix(refBase, "docker://") {
		imagePath := strings.TrimPrefix(refBase, "docker://")
		imageName := imagePath
		if i := strings.LastIndex(imageName, "/"); i >= 0 {
			imageName = imageName[i+1:]
		}
		if i := strings.IndexByte(imageName, ':'); i >= 0 {
			imageName = imageName[:i]
		}
		_, hit := forbiddenLintImages[imageName]
		return hit
	}
	if refBase == "bufbuild/buf-action" {
		return !setupOnlyTrue(step)
	}
	if _, hit := forbiddenLintActions[refBase]; hit {
		return true
	}
	tail := refBase
	if i := strings.LastIndex(tail, "/"); i >= 0 {
		tail = tail[i+1:]
	}
	switch tail {
	case "golangci-lint-action", "actionlint", "ruff-action", "buf-lint-action", "buf-breaking-action", "clippy-check":
		return true
	default:
		return false
	}
}

func setupOnlyTrue(step ymap) bool {
	withRaw, ok := step.get("with")
	if !ok {
		return false
	}
	with, ok := withRaw.(ymap)
	if !ok {
		return false
	}
	v, ok := with.get("setup_only")
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func isDisabled(node ymap) bool {
	if v, ok := node.get("continue-on-error"); ok {
		if b, ok := v.(bool); ok && b {
			return true
		}
		if s, ok := v.(string); ok && s == "true" {
			return true
		}
	}
	v, ok := node.get("if")
	if !ok {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(pyStr(v)))
	return s == "false" || s == "${{ false }}"
}

func loadFrontendScripts() map[string]string {
	return loadFrontendScriptsIn(".")
}

func loadFrontendScriptsIn(repoRoot string) map[string]string {
	raw, err := os.ReadFile(joinRepo(repoRoot, frontendPkgJSON))
	if err != nil {
		return nil
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	scripts, _ := doc["scripts"].(map[string]any)
	if scripts == nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for k, v := range scripts {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func expandRunScripts(command string, scripts map[string]string, depth int) string {
	if depth > 3 {
		return command
	}
	expanded := command
	for _, match := range runScriptRE.FindAllStringSubmatch(command, -1) {
		body := scripts[match[1]]
		if body != "" {
			expanded += " ;; " + expandRunScripts(body, scripts, depth+1)
		}
	}
	return expanded
}

func collectsClientProject(command string, inherited map[string]string) bool {
	for _, segment := range commandSplitRE.Split(command, -1) {
		if !vitestRE.MatchString(segment) || playwrightRE.MatchString(segment) {
			continue
		}
		env := map[string]string{}
		for k, v := range inherited {
			env[k] = v
		}
		for _, match := range inlineEnvRE.FindAllStringSubmatch(segment, -1) {
			env[match[1]] = match[2]
		}
		if env[browserEnv] != "true" {
			continue
		}
		projects := projectFlagRE.FindAllStringSubmatch(segment, -1)
		if len(projects) == 0 {
			return true
		}
		for _, project := range projects {
			if project[1] == clientProject {
				return true
			}
		}
	}
	return false
}

func iterRunSteps(doc ymap) []runStep {
	var stepsOut []runStep
	workflowEnv := envMap(mustGet(doc, "env"))
	jobs, _ := asMap(mustGet(doc, "jobs"))
	for i := range jobs.k {
		jobName := jobs.k[i]
		job, ok := jobs.v[i].(ymap)
		if !ok {
			continue
		}
		jobEnv := envMap(mustGet(job, "env"))
		jobBlocking := !isDisabled(job)
		steps, _ := asSeq(mustGet(job, "steps"))
		for _, stepRaw := range steps {
			step, ok := stepRaw.(ymap)
			if !ok {
				continue
			}
			runRaw, exists := step.get("run")
			run, ok := runRaw.(string)
			if !exists || !ok {
				continue
			}
			stepEnv := envMap(mustGet(step, "env"))
			env := map[string]string{}
			for k, v := range workflowEnv {
				env[k] = v
			}
			for k, v := range jobEnv {
				env[k] = v
			}
			for k, v := range stepEnv {
				env[k] = v
			}
			stepsOut = append(stepsOut, runStep{
				job:      jobName,
				command:  run,
				env:      env,
				blocking: jobBlocking && !isDisabled(step),
			})
		}
	}
	return stepsOut
}

type runStep struct {
	job      string
	command  string
	env      map[string]string
	blocking bool
}

func envMap(v any) map[string]string {
	m, ok := v.(ymap)
	if !ok {
		return map[string]string{}
	}
	out := map[string]string{}
	for i, k := range m.k {
		out[pyStr(k)] = pyStr(m.v[i])
	}
	return out
}

func isLintJobName(name string) bool {
	return lintJobNameRE.MatchString(name)
}

func stripInlineComment(raw string) string {
	inSingle, inDouble, escaped := false, false, false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && !inSingle {
			escaped = true
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if ch == '#' && !inSingle && !inDouble {
			if i == 0 || isSpaceByte(raw[i-1]) {
				return raw[:i]
			}
		}
	}
	return raw
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}

func isLintStep(step ymap) bool {
	if raw, ok := step.get("if"); ok {
		if s, ok := raw.(string); ok && lintStepIfRE.MatchString(s) {
			return true
		}
	}
	runRaw, ok := step.get("run")
	run, isStr := runRaw.(string)
	if !ok || !isStr {
		return false
	}
	for _, rawLine := range splitWorkflowLines(run) {
		stripped := strings.TrimLeftFunc(rawLine, unicode.IsSpace)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}
		if lintRunCmdRE.MatchString(stripInlineComment(rawLine)) {
			return true
		}
	}
	return false
}

func forbiddenLintContinueOnError(m ymap) bool {
	val, ok := m.get("continue-on-error")
	if !ok {
		return false
	}
	if val == nil {
		return false
	}
	if b, ok := val.(bool); ok && !b {
		return false
	}
	return true
}

func findRunSwallows(runBody string) []string {
	var hits []string
	seen := map[string]struct{}{}
	for _, rawLine := range splitWorkflowLines(runBody) {
		stripped := strings.TrimLeftFunc(rawLine, unicode.IsSpace)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}
		code := stripInlineComment(rawLine)
		for _, token := range findSwallows(code) {
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			hits = append(hits, token)
		}
	}
	return hits
}

func checkLintSwallows(path string, doc ymap) ([]string, int) {
	var errors []string
	bad := 0
	jobs, _ := asMap(mustGet(doc, "jobs"))
	for i := range jobs.k {
		jobName := jobs.k[i]
		job, ok := jobs.v[i].(ymap)
		if !ok {
			continue
		}
		var steps []ymap
		rawSteps, _ := asSeq(mustGet(job, "steps"))
		for _, stepRaw := range rawSteps {
			if step, ok := stepRaw.(ymap); ok {
				steps = append(steps, step)
			}
		}
		jobIsLint := isLintJobName(jobName)
		jobHasLint := false
		for _, step := range steps {
			if isLintStep(step) {
				jobHasLint = true
				break
			}
		}
		if (jobIsLint || jobHasLint) && forbiddenLintContinueOnError(job) {
			coe, _ := job.get("continue-on-error")
			errors = append(errors, fmt.Sprintf("::error file=%s::lint job %s sets continue-on-error: %s; swallowing lint exit codes is unconditionally forbidden even with '# swallow-ok:'.", path, pyReprString(jobName), pyRepr(coe)))
			bad++
		}
		for _, step := range steps {
			if !(jobIsLint || isLintStep(step)) {
				continue
			}
			if forbiddenLintContinueOnError(step) {
				coe, _ := step.get("continue-on-error")
				errors = append(errors, fmt.Sprintf("::error file=%s::lint step in job %s sets continue-on-error: %s; swallowing lint exit codes is unconditionally forbidden even with '# swallow-ok:'.", path, pyReprString(jobName), pyRepr(coe)))
				bad++
			}
			if runRaw, ok := step.get("run"); ok {
				if run, ok := runRaw.(string); ok {
					for _, swallow := range findRunSwallows(run) {
						errors = append(errors, fmt.Sprintf("::error file=%s::lint step in job %s swallows exit code via %s; swallowing lint exit codes is unconditionally forbidden even with '# swallow-ok:'.", path, pyReprString(jobName), pyReprString(swallow)))
						bad++
					}
				}
			}
		}
	}
	return errors, bad
}

func hasSwallowOK(lines []string, idx int) bool {
	if m := swallowOKSame.FindStringSubmatch(strings.TrimRightFunc(lines[idx], unicode.IsSpace)); m != nil {
		if len(strings.TrimSpace(m[1])) >= minSwallowLen {
			return true
		}
	}
	prev := idx - 1
	for prev >= 0 && strings.TrimSpace(lines[prev]) == "" {
		prev--
	}
	if prev >= 0 {
		if m := swallowOKLine.FindStringSubmatch(strings.TrimSpace(lines[prev])); m != nil {
			if len(strings.TrimSpace(m[1])) >= minSwallowLen {
				return true
			}
		}
	}
	return false
}

func findSwallows(code string) []string {
	var hits []string
	seen := map[string]struct{}{}
	add := func(s string) {
		s = normalizeWS(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		hits = append(hits, s)
	}
	for i := 0; i < len(code); {
		if strings.HasPrefix(code[i:], "||") {
			j := i + 2
			for j < len(code) && (code[j] == ' ' || code[j] == '\t') {
				j++
			}
			rest := code[j:]
			if hasWord(rest, "true") {
				add(code[i : j+4])
				i = j + 4
				continue
			}
			if hasWord(rest, "echo") {
				add(code[i : j+4])
				i = j + 4
				continue
			}
			if strings.HasPrefix(rest, "exit") && wordBoundary(rest, 4) {
				k := 4
				for k < len(rest) && (rest[k] == ' ' || rest[k] == '\t') {
					k++
					if k > 4 {
						// at least one space consumed below
					}
				}
				spaces := 0
				k = 4
				for k < len(rest) && (rest[k] == ' ' || rest[k] == '\t') {
					spaces++
					k++
				}
				if spaces > 0 && strings.HasPrefix(rest[k:], "0") && wordBoundary(rest[k:], 1) {
					add(code[i : j+k+1])
					i = j + k + 1
					continue
				}
			}
			if strings.HasPrefix(rest, ":") && colonBoundary(rest, 1) {
				add(code[i : j+1])
				i = j + 1
				continue
			}
		}
		if end, ok := matchSet(code, i); ok {
			add(code[i:end])
			i = end
			continue
		}
		if end, ok := matchContinueOnError(code, i); ok {
			add(code[i:end])
			i = end
			continue
		}
		i++
	}
	return hits
}

func hasWord(s, word string) bool {
	return strings.HasPrefix(s, word) && wordBoundary(s, len(word))
}

func wordBoundary(s string, n int) bool {
	if n > len(s) {
		return false
	}
	if n == len(s) {
		return true
	}
	return !isWordByte(s[n])
}

func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func colonBoundary(s string, n int) bool {
	if n > len(s) {
		return false
	}
	if n == len(s) {
		return true
	}
	c := s[n]
	return c == ' ' || c == '\t' || c == ';' || c == '#' || c == ')'
}

func matchSet(code string, i int) (int, bool) {
	start := i
	if i > 0 {
		prev := code[i-1]
		if prev != ';' && prev != '&' && prev != '|' && prev != '(' && !isSpaceByte(prev) {
			return 0, false
		}
	}
	if !strings.HasPrefix(code[i:], "set") || !wordBoundary(code[i:], 3) {
		return 0, false
	}
	j := i + 3
	if j >= len(code) || !isSpaceByte(code[j]) {
		return 0, false
	}
	for j < len(code) && isSpaceByte(code[j]) {
		j++
	}
	for {
		if j < len(code) && code[j] == '-' {
			k := j + 1
			if k >= len(code) || !isASCIILetter(code[k]) {
				break
			}
			for k < len(code) && isASCIILetter(code[k]) {
				k++
			}
			if k >= len(code) || !isSpaceByte(code[k]) {
				break
			}
			for k < len(code) && isSpaceByte(code[k]) {
				k++
			}
			j = k
			continue
		}
		break
	}
	if strings.HasPrefix(code[j:], "+e") && setTail(code, j+2) {
		return j + 2, true
	}
	if strings.HasPrefix(code[j:], "+o") && j+2 < len(code) && isSpaceByte(code[j+2]) {
		k := j + 2
		for k < len(code) && isSpaceByte(code[k]) {
			k++
		}
		if strings.HasPrefix(code[k:], "errexit") && setTail(code, k+len("errexit")) {
			return k + len("errexit"), true
		}
	}
	_ = start
	return 0, false
}

func setTail(code string, n int) bool {
	if n > len(code) {
		return false
	}
	if n == len(code) {
		return true
	}
	c := code[n]
	return c == ' ' || c == '\t' || c == ';' || c == '#' || c == ')'
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func matchContinueOnError(code string, i int) (int, bool) {
	const key = "continue-on-error"
	if i > 0 && isWordByte(code[i-1]) {
		return 0, false
	}
	if !strings.HasPrefix(code[i:], key) {
		return 0, false
	}
	j := i + len(key)
	for j < len(code) && isSpaceByte(code[j]) {
		j++
	}
	if j >= len(code) || code[j] != ':' {
		return 0, false
	}
	j++
	for j < len(code) && isSpaceByte(code[j]) {
		j++
	}
	if strings.HasPrefix(code[j:], "true") && wordBoundary(code[j:], 4) {
		return j + 4, true
	}
	return 0, false
}

func normalizeWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func asMap(v any) (ymap, bool) {
	m, ok := v.(ymap)
	return m, ok
}

func asSeq(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

func pyStr(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case string:
		return t
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case float64:
		return strconvFormat(t)
	default:
		return fmt.Sprint(t)
	}
}

func strconvFormat(f float64) string {
	return fmt.Sprintf("%v", f)
}

func pyRepr(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case string:
		return pyReprString(t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case float64:
		return fmt.Sprintf("%v", t)
	case []string:
		return pyList(t)
	case []any:
		parts := make([]string, len(t))
		for i, item := range t {
			parts[i] = pyRepr(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprint(t)
	}
}

func pyList(ss []string) string {
	if ss == nil {
		ss = []string{}
	}
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = pyReprString(s)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func pyReprString(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, "\"") {
		quote = '"'
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte(quote)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case rune(quote):
			b.WriteByte('\\')
			b.WriteByte(quote)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte(quote)
	return b.String()
}

func splitWorkflowLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}
