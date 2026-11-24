package packet

import (
	"alt/scripts/altharness/internal/screener"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	goTestFileRe    = regexp.MustCompile(`(?:^|/)[^/]+_test\.go$`)
	pyTestFileRe    = regexp.MustCompile(`(?:^|/)(?:tests?/[^/].*\.py|test_[^/]+\.py|test-[^/]+\.py|[^/]+_test\.py)$`)
	rustTestPathRe  = regexp.MustCompile(`(?:^|/)(?:tests/.*\.rs|tests\.rs|[^/]+_tests?\.rs)$`)
	tsTestFileRe    = regexp.MustCompile(`(?i)(?:^|/)(?:(?:__tests__|tests|e2e)/.*\.(?:[cm]?[jt]sx?|svelte)|[^/]+\.(?:spec|test)\.(?:[cm]?[jt]sx?|svelte)|[^/]+_test\.[cm]?[jt]sx?)$`)
	sourcePathExtRe = regexp.MustCompile(`(?i)\.(?:go|py|rs|[cm]?[jt]sx?|svelte)$`)
	identRe         = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	goTestPrefixRe  = regexp.MustCompile(`^(?:Test|Benchmark|Fuzz|Example)[A-Za-z0-9_]*$`)
	paramSuffixRe   = regexp.MustCompile(`\[[^\]]*\]$`)
	rustTestAttrRe  = regexp.MustCompile(`#\[\s*(?:(?:[\w:]+::)?test\b|rstest\b|proptest\b)`)
)

type testNameQuery struct {
	raw          string
	pathHint     string
	leaf         string
	parentSymbol string
	goParent     string
	goSubtests   []string
}

func parseTestNameQuery(testName string) testNameQuery {
	cleaned := strings.Trim(strings.TrimSpace(testName), "`\"'")
	cleaned = strings.TrimSpace(cleaned)
	pathHint := ""
	symbol := cleaned
	if strings.Contains(cleaned, "::") {
		var parts []string
		for _, p := range strings.Split(cleaned, "::") {
			p = strings.TrimSpace(p)
			if p != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) > 0 && sourcePathExtRe.MatchString(parts[0]) {
			pathHint = normalizeRepoRel(parts[0])
			parts = parts[1:]
		}
		if len(parts) > 0 {
			leaf := strings.TrimSpace(paramSuffixRe.ReplaceAllString(parts[len(parts)-1], ""))
			parent := ""
			if len(parts) >= minQualifiedParts {
				parent = strings.TrimSpace(paramSuffixRe.ReplaceAllString(parts[len(parts)-2], ""))
			}
			return testNameQuery{raw: cleaned, pathHint: pathHint, leaf: leaf, parentSymbol: parent}
		}
	}
	if strings.Contains(symbol, "/") {
		var slash []string
		for _, p := range strings.Split(symbol, "/") {
			p = strings.TrimSpace(p)
			if p != "" {
				slash = append(slash, p)
			}
		}
		if len(slash) > 0 && goTestPrefixRe.MatchString(slash[0]) {
			parent := slash[0]
			subs := append([]string{}, slash[1:]...)
			leaf := parent
			if len(subs) > 0 {
				leaf = subs[len(subs)-1]
			}
			return testNameQuery{raw: cleaned, leaf: leaf, parentSymbol: parent, goParent: parent, goSubtests: subs}
		}
	}
	if strings.Contains(symbol, " > ") {
		var gt []string
		for _, p := range strings.Split(symbol, " > ") {
			p = strings.TrimSpace(p)
			if p != "" {
				gt = append(gt, p)
			}
		}
		if len(gt) > 0 {
			leaf := strings.TrimSpace(paramSuffixRe.ReplaceAllString(gt[len(gt)-1], ""))
			parent := ""
			if len(gt) >= minQualifiedParts {
				parent = gt[len(gt)-2]
			}
			return testNameQuery{raw: cleaned, leaf: leaf, parentSymbol: parent}
		}
	}
	if strings.Contains(symbol, ".") && !strings.Contains(symbol, " ") {
		var dots []string
		for _, p := range strings.Split(symbol, ".") {
			p = strings.TrimSpace(p)
			if p != "" {
				dots = append(dots, p)
			}
		}
		if len(dots) >= minQualifiedParts {
			leaf := strings.TrimSpace(paramSuffixRe.ReplaceAllString(dots[len(dots)-1], ""))
			return testNameQuery{raw: cleaned, leaf: leaf, parentSymbol: dots[len(dots)-2]}
		}
	}
	leaf := strings.TrimSpace(paramSuffixRe.ReplaceAllString(symbol, ""))
	return testNameQuery{raw: cleaned, leaf: leaf}
}

func classifyTestFileLang(rel string) string {
	for _, prefix := range excludedGeneratedPrefixes {
		if strings.HasPrefix(rel, prefix) {
			return ""
		}
	}
	switch {
	case goTestFileRe.MatchString(rel):
		return "go"
	case pyTestFileRe.MatchString(rel):
		return "python"
	case strings.HasSuffix(rel, ".rs"):
		return "rust"
	case tsTestFileRe.MatchString(rel):
		return "typescript"
	default:
		return ""
	}
}

func collectRepoRelPaths(ctx context.Context, repo string) ([]string, error) {
	if _, err := lookGit(); err == nil {
		if _, statErr := os.Stat(filepath.Join(repo, ".git")); statErr == nil {
			stdout, _, code, runErr := runGit(ctx, repo, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
			if runErr != nil {
				return nil, runErr
			}
			if code == 0 {
				return splitNull(stdout), nil
			}
		}
	}
	var rels []string
	stack := []string{repo}
	for len(stack) > 0 {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		entries, err := os.ReadDir(cur)
		if err != nil {
			continue
		}
		for _, ent := range entries {
			if ent.Type()&os.ModeSymlink != 0 {
				continue
			}
			path := filepath.Join(cur, ent.Name())
			if ent.IsDir() {
				if _, skip := excludedTestScanDirs[ent.Name()]; skip {
					continue
				}
				stack = append(stack, path)
				continue
			}
			rel, err := filepath.Rel(repo, path)
			if err != nil {
				continue
			}
			rels = append(rels, filepath.ToSlash(rel))
		}
	}
	return rels, nil
}

type testFile struct {
	rel  string
	full string
	lang string
}

func listRepoTestFiles(ctx context.Context, repo string) ([]testFile, error) {
	rels, err := collectRepoRelPaths(ctx, repo)
	if err != nil {
		return nil, err
	}
	uniq := map[string]struct{}{}
	for _, rel := range rels {
		uniq[rel] = struct{}{}
	}
	var keys []string
	for rel := range uniq {
		keys = append(keys, rel)
	}
	sort.Strings(keys)
	var out []testFile
	for _, rel := range keys {
		lang := classifyTestFileLang(rel)
		if lang == "" {
			continue
		}
		full := filepath.Join(repo, filepath.FromSlash(rel))
		if isRegularNonSymlink(full) && withinDir(full, repo) {
			out = append(out, testFile{rel: rel, full: full, lang: lang})
		}
	}
	return out, nil
}

func goSubtestPresent(text, sub string) bool {
	escaped := strings.ReplaceAll(regexp.QuoteMeta(sub), "_", `[\s_/-]`)
	re := regexp.MustCompile(`(?:["` + "`" + `]` + escaped + `["` + "`" + `])`)
	return re.MatchString(text)
}

func goTestFuncPresent(text, name string) bool {
	if !goTestPrefixRe.MatchString(name) {
		return false
	}
	re := regexp.MustCompile(`\bfunc\s+(?:\([^)]+\)\s+)?` + regexp.QuoteMeta(name) + `\s*\(`)
	return re.MatchString(text)
}

func matchesGoTest(text string, q testNameQuery) bool {
	if q.goParent != "" {
		if !goTestFuncPresent(text, q.goParent) {
			return false
		}
		for _, sub := range q.goSubtests {
			if !goSubtestPresent(text, sub) {
				return false
			}
		}
		return true
	}
	if identRe.MatchString(q.leaf) && goTestFuncPresent(text, q.leaf) {
		return true
	}
	if q.leaf != "" {
		re := regexp.MustCompile(`\bt\.Run\s*\(\s*["` + "`" + `]` + regexp.QuoteMeta(q.leaf) + `["` + "`" + `]`)
		return re.MatchString(text)
	}
	return false
}

func matchesPythonTest(text string, q testNameQuery) bool {
	if !identRe.MatchString(q.leaf) || !strings.HasPrefix(q.leaf, "test") {
		return false
	}
	re := regexp.MustCompile(`(?m)^\s*(?:async\s+)?def\s+` + regexp.QuoteMeta(q.leaf) + `\s*\(`)
	if !re.MatchString(text) {
		return false
	}
	if q.parentSymbol != "" && identRe.MatchString(q.parentSymbol) {
		cls := regexp.MustCompile(`(?m)^\s*class\s+` + regexp.QuoteMeta(q.parentSymbol) + `\b`)
		return cls.MatchString(text)
	}
	return true
}

func matchesRustTest(rel, text string, q testNameQuery) bool {
	if !identRe.MatchString(q.leaf) || !strings.Contains(text, q.leaf) {
		return false
	}
	re := regexp.MustCompile(`\b(?:async\s+)?fn\s+` + regexp.QuoteMeta(q.leaf) + `\s*[<(]`)
	lines := splitLines(text)
	dedicated := rustTestPathRe.MatchString(rel)
	for _, loc := range re.FindAllStringIndex(text, -1) {
		lineIdx := strings.Count(text[:loc[0]], "\n")
		start := lineIdx - rustAttrLookbackLines
		if start < 0 {
			start = 0
		}
		if start > len(lines) {
			start = len(lines)
		}
		end := lineIdx
		if end > len(lines) {
			end = len(lines)
		}
		preceding := strings.Join(lines[start:end], "\n")
		if rustTestAttrRe.MatchString(preceding) {
			return true
		}
		if dedicated && strings.HasPrefix(q.leaf, "test_") {
			return true
		}
	}
	return false
}

func matchesTSTest(text string, q testNameQuery) bool {
	titles := []string{}
	if q.leaf != "" {
		titles = append(titles, q.leaf)
	}
	if q.raw != "" && q.raw != q.leaf {
		titles = append(titles, q.raw)
	}
	for _, title := range titles {
		re := regexp.MustCompile(`(?:Deno\.)?(?:test|it|describe)(?:\.(?:only|each))?\s*\(\s*["'` + "`" + `]` + regexp.QuoteMeta(title) + `["'` + "`" + `]`)
		if re.MatchString(text) {
			return true
		}
	}
	if identRe.MatchString(q.leaf) {
		re := regexp.MustCompile(`\bDeno\.test\s*\(\s*(?:async\s+)?function\s+` + regexp.QuoteMeta(q.leaf) + `\b`)
		return re.MatchString(text)
	}
	return false
}

func candidateTestExists(ctx context.Context, repo, testName string, files []testFile, cache map[string]string) bool {
	if ctx.Err() != nil {
		return false
	}
	q := parseTestNameQuery(testName)
	if q.leaf == "" {
		return false
	}
	cands := files
	if q.pathHint != "" {
		hint := q.pathHint
		cands = nil
		for _, item := range files {
			if item.rel == hint || strings.HasSuffix(item.rel, "/"+hint) {
				cands = append(cands, item)
			}
		}
	}
	for _, item := range cands {
		text, ok := cache[item.full]
		if !ok {
			b, err := os.ReadFile(item.full)
			if err != nil {
				text = ""
			} else {
				text = validUTF8(b)
			}
			cache[item.full] = text
		}
		switch item.lang {
		case "go":
			if matchesGoTest(text, q) {
				return true
			}
		case "python":
			if matchesPythonTest(text, q) {
				return true
			}
		case "rust":
			if matchesRustTest(item.rel, text, q) {
				return true
			}
		case "typescript":
			if matchesTSTest(text, q) {
				return true
			}
		}
	}
	return false
}

func valuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return false
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok || bok {
		return aok && bok && as == bs
	}
	return strictEqual(a, b)
}

func candidateMatchesResponse(candidate, response map[string]any) bool {
	candID, cidOK := asString(candidate["id"])
	respID, ridOK := asString(response["candidate_id"])
	if cidOK && candID != "" && ridOK && respID == candID {
		return true
	}
	cFile, cfOK := asString(candidate["file"])
	rFile, rfOK := asString(response["file"])
	if !cfOK || !rfOK {
		return false
	}
	return normalizeRepoRel(cFile) == normalizeRepoRel(rFile) &&
		valuesEqual(candidate["function"], response["function"]) &&
		valuesEqual(candidate["kind"], response["kind"])
}

func validateBehaviorCandidates(ctx context.Context, data map[string]any, screener []map[string]any, issues *[]Issue, repoRoot string) error {
	impact, _ := asMap(data["runtime_behavior_impact"])
	var hasChange any
	var raw []any
	if impact != nil {
		hasChange = impact["has_change"]
		raw, _ = asSlice(impact["candidate_responses"])
	}
	type respItem struct {
		idx int
		m   map[string]any
	}
	var responses []respItem
	for idx, item := range raw {
		if m, ok := asMap(item); ok {
			responses = append(responses, respItem{idx, m})
		}
	}
	var files []testFile
	cache := map[string]string{}
	loaded := false
	for _, resp := range responses {
		if resp.m["disposition"] != "intended" {
			continue
		}
		if hasChange == false {
			*issues = append(*issues, Issue{
				Code:    "CONTRADICTORY_BEHAVIOR_IMPACT",
				Path:    fmt.Sprintf("$.runtime_behavior_impact.candidate_responses[%d].disposition", resp.idx),
				Message: "runtime_behavior_impact.has_change=false contradicts candidate response with disposition='intended'",
			})
		}
		testName, ok := asString(resp.m["test_name"])
		if !ok || isPlaceholder(testName) {
			*issues = append(*issues, Issue{
				Code:    "MISSING_CANDIDATE_TEST_NAME",
				Path:    fmt.Sprintf("$.runtime_behavior_impact.candidate_responses[%d].test_name", resp.idx),
				Message: "candidate response with disposition='intended' requires a non-empty, non-placeholder test_name",
			})
			continue
		}
		if repoRoot == "" {
			continue
		}
		if !loaded {
			var err error
			files, err = listRepoTestFiles(ctx, repoRoot)
			if err != nil {
				return err
			}
			loaded = true
		}
		if !candidateTestExists(ctx, repoRoot, testName, files, cache) {
			*issues = append(*issues, Issue{
				Code:    "NONEXISTENT_CANDIDATE_TEST",
				Path:    fmt.Sprintf("$.runtime_behavior_impact.candidate_responses[%d].test_name", resp.idx),
				Message: fmt.Sprintf("candidate response test_name %q was not found in any test file under %s", testName, repoRoot),
			})
		}
	}
	if len(screener) == 0 {
		return nil
	}
	for _, cand := range screener {
		matched := -1
		for i, resp := range responses {
			if candidateMatchesResponse(cand, resp.m) {
				matched = i
				break
			}
		}
		label := fmt.Sprintf("id=%v (%v:%v [%v])", cand["id"], cand["file"], cand["function"], cand["kind"])
		if matched < 0 {
			*issues = append(*issues, Issue{
				Code:    "UNANSWERED_BEHAVIOR_CANDIDATE",
				Path:    "$.runtime_behavior_impact.candidate_responses",
				Message: fmt.Sprintf("active CHECK-01 behavior candidate %s has no matching response", label),
			})
			continue
		}
		resp := responses[matched]
		if resp.m["disposition"] == "unintended_reverted" {
			*issues = append(*issues, Issue{
				Code:    "UNREVERTED_UNINTENDED_CANDIDATE",
				Path:    fmt.Sprintf("$.runtime_behavior_impact.candidate_responses[%d]", resp.idx),
				Message: fmt.Sprintf("candidate %s is marked 'unintended_reverted' but is still present in the active screener report", label),
			})
		}
	}
	return nil
}

func parseScreenerCandidates(raw, source string) ([]map[string]any, error) {
	parsed, err := parseJSONBytes([]byte(raw))
	if err != nil {
		return nil, pErr("invalid screener report JSON (%s): %s", source, err.Error())
	}
	items, ok := asSlice(parsed)
	if !ok {
		return nil, pErr("screener report (%s) must be a JSON array of candidate objects", source)
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, ok := asMap(item)
		if !ok {
			return nil, pErr("screener report (%s) must be a JSON array of candidate objects", source)
		}
		out = append(out, m)
	}
	return out, nil
}

func runBehaviorScreener(ctx context.Context, repo, base string) ([]map[string]any, error) {
	if strings.TrimSpace(base) == "" || strings.HasPrefix(base, "-") {
		return nil, pErr("invalid --base-commit revision: %q", base)
	}
	var stdout, stderr bytes.Buffer
	rc := screener.Run(ctx, []string{"--repo-root", repo, "--base", base}, &stdout, &stderr)
	if rc != 0 {
		return nil, pErr("behavior screener failed (rc=%d): %s", rc, strings.TrimSpace(stderr.String()))
	}
	return parseScreenerCandidates(stdout.String(), "--base-commit")
}
