package suppress

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const diffUsage = `usage: suppression_diff_report.py [-h] [--base BASE] [--head HEAD]
                                  [--repo-root REPO_ROOT]
                                  [--summary-file SUMMARY_FILE]
                                  [base_pos]
`

const diffHelp = diffUsage + `
Render a Markdown PR Job Summary table of suppression diffs.

positional arguments:
  base_pos              Optional positional base git revision (alternative to
                        --base).

options:
  -h, --help            show this help message and exit
  --base BASE           Base git revision to compare against (default: HEAD).
  --head HEAD           Optional head git revision (default: compare against
                        working tree).
  --repo-root REPO_ROOT
                        Repository root directory (default: detected repo
                        root).
  --summary-file SUMMARY_FILE
                        Append Markdown report to this file (e.g.
                        $GITHUB_STEP_SUMMARY).
`

// RunDiff is the suppression_diff_report.py CLI.
// args is the argument vector without the program name.
func RunDiff(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		fmt.Fprint(stderr, "ERROR: failed to build suppression diff report: nil context\n")
		return exitUsage
	}
	flags, err := parseDiffArgs(args)
	if err != nil {
		fmt.Fprint(stderr, diffUsage)
		fmt.Fprintf(stderr, "suppression_diff_report.py: error: %s\n", err.Error())
		return exitUsage
	}
	if flags.help {
		fmt.Fprint(stdout, diffHelp)
		return exitClean
	}
	baseRev := "HEAD"
	if flags.base != nil {
		baseRev = *flags.base
	} else if flags.basePos != nil {
		baseRev = *flags.basePos
	}
	repo := defaultRepo(ctx)
	if flags.repoRoot != nil {
		repo = *flags.repoRoot
	}
	repo = resolvePath(repo)
	report, err := buildDiffReport(ctx, repo, baseRev, flags.head)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: failed to build suppression diff report: %s\n", err)
		return exitUsage
	}
	fmt.Fprint(stdout, report)
	if flags.summary != nil {
		if err := appendSummary(*flags.summary, report); err != nil {
			fmt.Fprintf(stderr, "ERROR: failed to build suppression diff report: %s\n", err)
			return exitUsage
		}
	}
	return exitClean
}

func appendSummary(path, report string) error {
	if err := os.MkdirAll(filepath.Dir(path), fsperm.Dir); err != nil {
		return err
	}
	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fsperm.File)
	if err != nil {
		return err
	}
	defer f.Close()
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(report)
	return err
}

type suppressionRecord struct {
	surface string
	path    string
	line    int
	kind    string
	rule    string
	reason  string
	expires string
}

func (r suppressionRecord) identity() string {
	return strings.Join([]string{r.surface, r.path, r.kind, r.rule, r.reason, r.expires}, "\x00")
}

func buildDiffReport(ctx context.Context, repo, baseRev string, headRev *string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	baseSHA, err := validateDiffRev(ctx, repo, baseRev)
	if err != nil {
		return "", err
	}
	var headSHA string
	if headRev != nil {
		headSHA, err = validateDiffRev(ctx, repo, *headRev)
		if err != nil {
			return "", err
		}
	}
	paths, err := changedPaths(ctx, repo, baseSHA, headRev, headSHA)
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	var beforeChanged, afterChanged []suppressionRecord
	for _, rel := range paths {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if rel != allowlistRelPath && isExcludedSourcePath(rel) {
			continue
		}
		if before, ok, err := gitShow(ctx, repo, baseSHA, rel); err != nil {
			return "", err
		} else if ok {
			recs, err := extractFileRecords(rel, before, false)
			if err != nil {
				return "", err
			}
			beforeChanged = append(beforeChanged, recs...)
		}
		after, ok, err := readAfter(ctx, repo, headRev, headSHA, rel)
		if err != nil {
			return "", err
		}
		if ok {
			recs, err := extractFileRecords(rel, after, true)
			if err != nil {
				return "", err
			}
			afterChanged = append(afterChanged, recs...)
		}
	}
	added, removed := diffRecords(beforeChanged, afterChanged)
	totals, err := afterSurfaceTotals(ctx, repo, headRev, headSHA)
	if err != nil {
		return "", err
	}
	addedBy := map[string]int{}
	removedBy := map[string]int{}
	for _, r := range added {
		addedBy[r.surface]++
	}
	for _, r := range removed {
		removedBy[r.surface]++
	}
	headLabel := "working-tree"
	if headRev != nil {
		headLabel = *headRev
	}
	var lines []string
	lines = append(lines,
		"## Suppression Governance Diff Report",
		"",
		fmt.Sprintf("Base: `%s` -> Head: `%s`", baseRev, headLabel),
		"",
		"| Surface | Before | After | Delta |",
		"| --- | ---: | ---: | ---: |",
	)
	totalBefore, totalAfter := 0, 0
	for _, item := range []struct{ surface, label string }{
		{"config", "Config allowlist"},
		{"broad", "Broad source"},
		{"inline", "Inline source"},
	} {
		afterN := totals[item.surface]
		delta := addedBy[item.surface] - removedBy[item.surface]
		beforeN := afterN - delta
		totalBefore += beforeN
		totalAfter += afterN
		lines = append(lines, fmt.Sprintf("| %s | %d | %d | %s |", item.label, beforeN, afterN, fmtDelta(delta)))
	}
	totalDelta := totalAfter - totalBefore
	lines = append(lines, fmt.Sprintf("| **Total** | **%d** | **%d** | **%s** |", totalBefore, totalAfter, fmtDelta(totalDelta)))
	lines = append(lines, "", fmt.Sprintf("### Added Suppressions (%d)", len(added)), "")
	lines = append(lines, renderRecordTable(added)...)
	lines = append(lines, fmt.Sprintf("### Removed Suppressions (%d)", len(removed)), "")
	lines = append(lines, renderRecordTable(removed)...)
	return strings.Join(lines, "\n"), nil
}

func fmtDelta(delta int) string {
	if delta >= 0 {
		return fmt.Sprintf("+%d", delta)
	}
	return fmt.Sprintf("%d", delta)
}

func renderRecordTable(records []suppressionRecord) []string {
	if len(records) == 0 {
		return []string{"_None_", ""}
	}
	lines := []string{
		"| Location | Kind / Rule | Reason | Expires |",
		"| --- | --- | --- | --- |",
	}
	for _, rec := range records {
		loc := fmt.Sprintf("`%s`", rec.path)
		if rec.line > 0 {
			loc = fmt.Sprintf("`%s:%d`", rec.path, rec.line)
		}
		kindRule := fmt.Sprintf("`%s` (`%s`)", rec.kind, mdEscape(rec.rule))
		exp := "-"
		if rec.expires != "-" {
			exp = "`" + rec.expires + "`"
		}
		lines = append(lines, fmt.Sprintf("| %s | %s | %s | %s |", loc, kindRule, mdEscape(rec.reason), exp))
	}
	lines = append(lines, "")
	return lines
}

func mdEscape(text string) string {
	clipped := text
	if len([]rune(clipped)) > maxReasonDisplay {
		rs := []rune(clipped)
		clipped = string(rs[:maxReasonDisplay-3]) + "..."
	}
	clipped = strings.ReplaceAll(clipped, "|", "\\|")
	clipped = strings.ReplaceAll(clipped, "\n", " ")
	return clipped
}

func diffRecords(before, after []suppressionRecord) (added, removed []suppressionRecord) {
	beforeCounts := map[string]int{}
	afterCounts := map[string]int{}
	for _, r := range before {
		beforeCounts[r.identity()]++
	}
	for _, r := range after {
		afterCounts[r.identity()]++
	}
	seenAfter := map[string]int{}
	for _, rec := range after {
		key := rec.identity()
		seenAfter[key]++
		if seenAfter[key] > beforeCounts[key] {
			added = append(added, rec)
		}
	}
	seenBefore := map[string]int{}
	for _, rec := range before {
		key := rec.identity()
		seenBefore[key]++
		if seenBefore[key] > afterCounts[key] {
			removed = append(removed, rec)
		}
	}
	sortRecords(added)
	sortRecords(removed)
	return added, removed
}

func sortRecords(rs []suppressionRecord) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].surface != rs[j].surface {
			return rs[i].surface < rs[j].surface
		}
		if rs[i].path != rs[j].path {
			return rs[i].path < rs[j].path
		}
		if rs[i].line != rs[j].line {
			return rs[i].line < rs[j].line
		}
		if rs[i].kind != rs[j].kind {
			return rs[i].kind < rs[j].kind
		}
		return rs[i].rule < rs[j].rule
	})
}

func changedPaths(ctx context.Context, repo, baseSHA string, headRev *string, headSHA string) ([]string, error) {
	var res string
	var err error
	if headRev != nil {
		res, err = gitChecked(ctx, repo, "diff", "--name-only", baseSHA, headSHA, "--")
	} else {
		res, err = gitChecked(ctx, repo, "diff", "--name-only", baseSHA, "--")
	}
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, line := range gitLines(res) {
		seen[line] = struct{}{}
	}
	if headRev == nil {
		untracked, err := gitChecked(ctx, repo, "ls-files", "--others", "--exclude-standard")
		if err != nil {
			return nil, err
		}
		for _, line := range gitLines(untracked) {
			seen[line] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for line := range seen {
		out = append(out, line)
	}
	return out, nil
}

func readAfter(ctx context.Context, repo string, headRev *string, headSHA, rel string) (string, bool, error) {
	if headRev != nil {
		return gitShow(ctx, repo, headSHA, rel)
	}
	full, ok := safeWorktreeFile(repo, rel)
	if !ok {
		return "", false, nil
	}
	text, err := readFileReplace(full)
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

func afterSurfaceTotals(ctx context.Context, repo string, headRev *string, headSHA string) (map[string]int, error) {
	var files []string
	var err error
	if headRev != nil {
		files, err = gitList(ctx, repo, "ls-tree", "-r", "--name-only", headSHA, "--")
	} else {
		files, err = gitList(ctx, repo, "ls-files", "--cached", "--others", "--exclude-standard")
	}
	if err != nil {
		return nil, err
	}
	totals := map[string]int{}
	for _, rel := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if rel != allowlistRelPath && isExcludedSourcePath(rel) {
			continue
		}
		text, ok, err := readAfter(ctx, repo, headRev, headSHA, rel)
		if err != nil || !ok {
			if err != nil {
				return nil, err
			}
			continue
		}
		recs, err := extractFileRecords(rel, text, true)
		if err != nil {
			return nil, err
		}
		for _, rec := range recs {
			totals[rec.surface]++
		}
	}
	return totals, nil
}

func extractFileRecords(rel, text string, strict bool) ([]suppressionRecord, error) {
	if rel == allowlistRelPath {
		return extractAllowlistRecords(text, strict)
	}
	if isExcludedSourcePath(rel) || isGeneratedSource(text) {
		return nil, nil
	}
	switch {
	case strings.HasSuffix(rel, ".py"):
		return extractPythonRecords(rel, text), nil
	case strings.HasSuffix(rel, ".go"):
		return extractGoRecords(rel, text), nil
	case strings.HasSuffix(rel, ".rs"):
		return extractRustRecords(rel, text), nil
	case hasExt(rel, tsExts):
		return extractTSRecords(rel, text), nil
	default:
		return nil, nil
	}
}

func cleanReason(raw string) string {
	compact := strings.Join(strings.Fields(raw), " ")
	if compact == "" {
		return "-"
	}
	return compact
}

func extractExpiresDate(text string) string {
	m := expiresDateRe.FindStringSubmatch(text)
	if m == nil {
		return "-"
	}
	return m[1]
}

func extractAllowlistRecords(text string, strict bool) ([]suppressionRecord, error) {
	data, err := parseTOML(text)
	if err != nil {
		if strict {
			return nil, err
		}
		return nil, nil
	}
	var records []suppressionRecord
	if arr, ok := data["bandit"].([]any); ok {
		for _, item := range arr {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			skips := joinAny(entry["skips"], ", ")
			if skips == "" {
				skips = "bandit"
			}
			path := allowlistRelPath
			if s, ok := entry["path"].(string); ok && s != "" {
				path = s
			} else if entry["path"] != nil {
				path = pyStr(entry["path"])
			}
			records = append(records, suppressionRecord{
				surface: "config", path: path, kind: "config_bandit", rule: skips,
				reason: cleanReason(strOr(entry["reason"], "")), expires: strOr(entry["expires"], "-"),
			})
		}
	}
	if arr, ok := data["golangci_exclusions"].([]any); ok {
		for _, item := range arr {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			linters := joinAny(entry["linters"], ", ")
			scope := "*"
			if entry["path"] != nil {
				scope = pyStr(entry["path"])
			} else if entry["path_except"] != nil {
				scope = pyStr(entry["path_except"])
			}
			cfg := ".golangci.yml"
			if entry["config"] != nil {
				cfg = pyStr(entry["config"])
			}
			records = append(records, suppressionRecord{
				surface: "config", path: cfg, kind: "config_golangci", rule: fmt.Sprintf("%s (%s)", linters, scope),
				reason: cleanReason(strOr(entry["reason"], "")), expires: strOr(entry["expires"], "-"),
			})
		}
	}
	if arr, ok := data["cargo_lint_allows"].([]any); ok {
		for _, item := range arr {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			section := "lints"
			if entry["section"] != nil {
				section = pyStr(entry["section"])
			}
			allows := joinAny(entry["allows"], ", ")
			path := "Cargo.toml"
			if entry["path"] != nil {
				path = pyStr(entry["path"])
			}
			records = append(records, suppressionRecord{
				surface: "config", path: path, kind: "config_cargo", rule: fmt.Sprintf("%s: %s", section, allows),
				reason: cleanReason(strOr(entry["reason"], "")), expires: strOr(entry["expires"], "-"),
			})
		}
	}
	return records, nil
}

func strOr(v any, fallback string) string {
	if v == nil {
		return fallback
	}
	s := pyStr(v)
	if s == "" {
		return fallback
	}
	return s
}

func joinAny(v any, sep string) string {
	arr, ok := v.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, len(arr))
	for i, el := range arr {
		parts[i] = pyStr(el)
	}
	return strings.Join(parts, sep)
}

func extractPythonRecords(rel, text string) []suppressionRecord {
	comments := pythonComments(text)
	byLine := map[int]string{}
	for _, c := range comments {
		byLine[c.line] = stripCommentDelimiters(c.raw)
	}
	var records []suppressionRecord
	for _, c := range comments {
		if m := pyFileNoqaRe.FindStringSubmatch(c.raw); m != nil {
			preceding := precedingCommentText(byLine, c.line)
			context := strings.TrimSpace(preceding + " " + m[1])
			rule := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(m[1]), ":"))
			if rule == "" {
				rule = "all"
			}
			reasonSrc := m[1]
			if preceding != "" {
				reasonSrc = preceding
			}
			records = append(records, suppressionRecord{
				surface: "broad", path: rel, line: c.line, kind: "broad_python_noqa", rule: rule,
				reason: cleanReason(reasonSrc), expires: extractExpiresDate(context),
			})
			continue
		}
		m := pyInlineNoqaRe.FindStringSubmatch(c.raw)
		if m == nil {
			continue
		}
		rest := m[1]
		rule := "bare"
		reason := strings.TrimSpace(rest)
		if idx := pyNoqaCodesRe.FindStringSubmatchIndex(rest); idx != nil {
			parts := strings.Split(rest[idx[2]:idx[3]], ",")
			var codes []string
			for _, p := range parts {
				p = strings.ToUpper(strings.TrimSpace(p))
				if p != "" {
					codes = append(codes, p)
				}
			}
			rule = strings.Join(codes, ", ")
			tail := strings.TrimSpace(pySecondaryRe.ReplaceAllString(rest[idx[3]:], ""))
			reason = strings.TrimSpace(reasonPrefixRe.ReplaceAllString(tail, ""))
		}
		records = append(records, suppressionRecord{
			surface: "inline", path: rel, line: c.line, kind: "python_noqa", rule: rule,
			reason: cleanReason(reason), expires: extractExpiresDate(c.raw),
		})
	}
	return records
}

func extractGoRecords(rel, text string) []suppressionRecord {
	comments := scanGoComments(text)
	byLine := map[int]string{}
	for _, c := range comments {
		if c.line == c.endLine {
			byLine[c.line] = stripCommentDelimiters(c.raw)
		}
	}
	var records []suppressionRecord
	for _, c := range comments {
		if m := goNolintRe.FindStringSubmatch(c.raw); m != nil {
			linters := strings.TrimSpace(m[1])
			if linters == "" {
				linters = "all"
			}
			rest := m[2]
			if strings.HasSuffix(rest, "*/") {
				rest = rest[:len(rest)-2]
			}
			reason := ""
			if i := strings.Index(rest, "//"); i >= 0 {
				reason = strings.TrimSpace(rest[i+2:])
			}
			isBroad := c.before || strings.EqualFold(linters, "all")
			preceding := ""
			if isBroad {
				preceding = precedingCommentText(byLine, c.line)
			}
			combined := strings.TrimSpace(preceding + " " + reason)
			kind := "go_nolint"
			surface := "inline"
			shown := reason
			if isBroad {
				kind = "broad_go_nolint"
				surface = "broad"
				shown = combined
			}
			records = append(records, suppressionRecord{
				surface: surface, path: rel, line: c.line, kind: kind, rule: linters,
				reason: cleanReason(shown), expires: extractExpiresDate(combined),
			})
			continue
		}
		m := goNosecRe.FindStringSubmatch(c.raw)
		if m == nil {
			continue
		}
		cleaned := strings.TrimSpace(strings.TrimRight(m[1], "*/"))
		rule := cleaned
		reason := ""
		if parsed := goNosecRulesRe.FindStringSubmatch(cleaned); parsed != nil {
			found := gCodeRe.FindAllString(parsed[1], -1)
			rule = strings.Join(found, ", ")
			if rule == "" {
				rule = "nosec"
			}
			reason = strings.TrimSpace(parsed[2])
		}
		if rule == "" {
			rule = "nosec"
		}
		records = append(records, suppressionRecord{
			surface: "inline", path: rel, line: c.line, kind: "go_nosec", rule: rule,
			reason: cleanReason(reason), expires: extractExpiresDate(c.raw),
		})
	}
	return records
}

func extractRustRecords(rel, text string) []suppressionRecord {
	tokens := scanRustTokens(text)
	var records []suppressionRecord
	for idx, tok := range tokens {
		if tok.kind == "ATTR_INNER" {
			if m := rustInnerAllowCapRe.FindStringSubmatch(tok.text); m != nil {
				ctx := rustContextForAttr(tokens, idx)
				records = append(records, suppressionRecord{
					surface: "broad", path: rel, line: tok.startLine, kind: "broad_rust_inner_allow",
					rule: strings.Join(strings.Fields(m[1]), " "), reason: cleanReason(ctx), expires: extractExpiresDate(ctx),
				})
			}
			continue
		}
		if tok.kind != "ATTR_OUTER" {
			continue
		}
		m := rustOuterAllowCapRe.FindStringSubmatch(tok.text)
		if m == nil {
			continue
		}
		isMod := rustAttrAppliesToMod(tokens, idx)
		ctx := rustContextForAttr(tokens, idx)
		kind := "rust_allow"
		surface := "inline"
		if isMod {
			kind = "broad_rust_mod_allow"
			surface = "broad"
		}
		records = append(records, suppressionRecord{
			surface: surface, path: rel, line: tok.startLine, kind: kind,
			rule: strings.Join(strings.Fields(m[1]), " "), reason: cleanReason(ctx), expires: extractExpiresDate(ctx),
		})
	}
	return records
}

func extractTSRecords(rel, text string) []suppressionRecord {
	comments := scanTSComments(text, strings.HasSuffix(rel, ".svelte"))
	byLine := map[int]string{}
	for _, c := range comments {
		if c.line == c.endLine {
			byLine[c.line] = stripCommentDelimiters(c.raw)
		}
	}
	var records []suppressionRecord
	for _, c := range comments {
		body := stripCommentDelimiters(c.raw)
		preceding := precedingCommentText(byLine, c.line)
		matched := false
		for _, item := range []struct {
			re   interface{ FindStringSubmatch(string) []string }
			kind string
		}{
			{tsBiomeAllRe, "broad_ts_biome"},
			{tsDenoFileRe, "broad_ts_deno"},
			{tsEslintBroadRe, "broad_ts_eslint"},
		} {
			m := item.re.FindStringSubmatch(body)
			if m == nil {
				continue
			}
			combined := strings.TrimSpace(preceding + " " + m[1])
			rule := strings.TrimSpace(m[1])
			if rule == "" {
				rule = "all"
			}
			records = append(records, suppressionRecord{
				surface: "broad", path: rel, line: c.line, kind: item.kind, rule: rule,
				reason: cleanReason(combined), expires: extractExpiresDate(combined),
			})
			matched = true
			break
		}
		if matched {
			continue
		}
		if m := tsBiomeLineRe.FindStringSubmatch(body); m != nil {
			spec := tsBiomeSpecRe.FindStringSubmatch(strings.TrimSpace(m[1]))
			rule := "biome"
			reason := strings.TrimSpace(m[1])
			if spec != nil {
				rule = spec[1]
				reason = ""
				if len(spec) > 2 {
					reason = strings.TrimSpace(spec[2])
				}
			}
			records = append(records, suppressionRecord{
				surface: "inline", path: rel, line: c.line, kind: "ts_biome_ignore", rule: rule,
				reason: cleanReason(reason), expires: extractExpiresDate(body),
			})
			continue
		}
		if m := tsDenoLineRe.FindStringSubmatch(body); m != nil {
			rest := strings.TrimSpace(m[1])
			rule := "deno"
			reason := ""
			if rest != "" {
				parts := strings.Fields(rest)
				if len(parts) > 0 {
					rule = parts[0]
				}
				if len(parts) > 1 {
					reason = strings.Join(parts[1:], " ")
				}
			}
			records = append(records, suppressionRecord{
				surface: "inline", path: rel, line: c.line, kind: "ts_deno_ignore", rule: rule,
				reason: cleanReason(reason), expires: extractExpiresDate(body),
			})
		}
	}
	return records
}

type diffFlags struct {
	basePos  *string
	base     *string
	head     *string
	repoRoot *string
	summary  *string
	help     bool
}

func parseDiffArgs(argv []string) (diffFlags, error) {
	var f diffFlags
	var positionals []string
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if a == "--" {
			positionals = append(positionals, argv[i+1:]...)
			break
		}
		if a == "-h" || a == "--help" {
			f.help = true
			return f, nil
		}
		name, val, hasVal, ok := splitAssign(a)
		if !ok {
			positionals = append(positionals, a)
			continue
		}
		switch name {
		case "--base":
			v, ni, err := takeValue(argv, i, val, hasVal, name)
			if err != nil {
				return f, err
			}
			i = ni
			f.base = &v
		case "--head":
			v, ni, err := takeValue(argv, i, val, hasVal, name)
			if err != nil {
				return f, err
			}
			i = ni
			f.head = &v
		case "--repo-root":
			v, ni, err := takeValue(argv, i, val, hasVal, name)
			if err != nil {
				return f, err
			}
			i = ni
			f.repoRoot = &v
		case "--summary-file":
			v, ni, err := takeValue(argv, i, val, hasVal, name)
			if err != nil {
				return f, err
			}
			i = ni
			f.summary = &v
		default:
			return f, fmt.Errorf("unrecognized arguments: %s", a)
		}
	}
	if len(positionals) > 1 {
		return f, fmt.Errorf("unrecognized arguments: %s", strings.Join(positionals[1:], " "))
	}
	if len(positionals) == 1 {
		f.basePos = &positionals[0]
	}
	return f, nil
}
