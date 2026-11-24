package suppress

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const checkUsage = `usage: check_suppressions.py [-h] [--repo-root REPO_ROOT]
                             [--allowlist ALLOWLIST] [--today TODAY]
                             [--python-noqa-baseline PYTHON_NOQA_BASELINE]
                             [--no-python-noqa-baseline]
                             [--update-python-noqa-baseline] [--base BASE]
`

const checkHelp = checkUsage + `
Verify lint suppression allowlists and source-code suppression governance.

options:
  -h, --help            show this help message and exit
  --repo-root REPO_ROOT
                        Repository root directory (default: detected repo
                        root).
  --allowlist ALLOWLIST
                        Path to suppression_allowlist.toml (default: <repo-
                        root>/scripts/lint/suppression_allowlist.toml).
  --today TODAY         Override current date (YYYY-MM-DD) for deterministic
                        expiration checks.
  --python-noqa-baseline PYTHON_NOQA_BASELINE
                        Path to python_noqa_baseline.toml (default: <repo-
                        root>/scripts/lint/ratchet/python_noqa_baseline.toml
                        if present).
  --no-python-noqa-baseline
                        Disable reasonless Python # noqa baseline (require
                        reasons on every line).
  --update-python-noqa-baseline
                        Write/update python_noqa_baseline.toml from current
                        reasonless Python # noqa lines.
  --base BASE           Git base revision for detecting expires extensions
                        without a same-diff ADR update.
`

type checkArgs struct {
	repoRoot       string
	allowlist      string
	today          string
	todaySet       bool
	baseline       string
	baselineSet    bool
	noBaseline     bool
	updateBaseline bool
	base           string
	baseSet        bool
}

// RunCheck is the CLI for lint suppression governance.
func RunCheck(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		fmt.Fprint(stderr, "ERROR: suppression check failed to execute: nil context\n")
		return exitUsage
	}
	parsed, code, ok := parseCheckArgs(args, stdout, stderr)
	if !ok {
		return code
	}
	repo, err := cleanDir(parsed.repoRoot)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return exitUsage
	}
	if parsed.updateBaseline {
		return updatePythonNoqaBaseline(ctx, parsed, repo, stdout, stderr)
	}
	today, allowPath, baseRev, errMsg := resolveCheckInputs(ctx, parsed, repo)
	if errMsg != "" {
		fmt.Fprintf(stderr, "ERROR: %s\n", errMsg)
		return exitUsage
	}
	baseline, blErr := resolveBaseline(parsed, repo)
	if blErr != "" {
		fmt.Fprintf(stderr, "ERROR: %s\n", blErr)
		return exitUsage
	}
	violations, err := checkRepo(ctx, repo, allowPath, today, baseline, baseRev)
	if err != nil {
		if gitMissing(err) || err.Error() == "git" {
			fmt.Fprintf(stderr, "ERROR: suppression check failed to execute: git\n")
			return exitUsage
		}
		fmt.Fprintf(stderr, "ERROR: suppression check failed to execute: %s\n", err.Error())
		return exitUsage
	}
	return reportviolations(stdout, stderr, violations)
}

func parseCheckArgs(args []string, stdout, stderr io.Writer) (checkArgs, int, bool) {
	var a checkArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" || arg == "--help" {
			fmt.Fprint(stdout, checkHelp)
			return a, exitClean, false
		}
		name, val, hasVal, consumed := splitFlag(arg)
		switch name {
		case "--repo-root", "--allowlist", "--today", "--python-noqa-baseline", "--base":
			if !hasVal {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
					fmt.Fprint(stderr, checkUsage)
					fmt.Fprintf(stderr, "check_suppressions.py: error: argument %s: expected one argument\n", name)
					return a, exitUsage, false
				}
				i++
				val = args[i]
			}
			_ = consumed
			switch name {
			case "--repo-root":
				a.repoRoot = val
			case "--allowlist":
				a.allowlist = val
			case "--today":
				a.today = val
				a.todaySet = true
			case "--python-noqa-baseline":
				a.baseline = val
				a.baselineSet = true
			case "--base":
				a.base = val
				a.baseSet = true
			}
		case "--no-python-noqa-baseline":
			a.noBaseline = true
		case "--update-python-noqa-baseline":
			a.updateBaseline = true
		default:
			fmt.Fprint(stderr, checkUsage)
			fmt.Fprintf(stderr, "check_suppressions.py: error: unrecognized arguments: %s\n", arg)
			return a, exitUsage, false
		}
	}
	if a.repoRoot == "" {
		a.repoRoot = defaultRepo(context.Background())
	}
	return a, 0, true
}

func splitFlag(arg string) (name, val string, hasVal bool, consumed bool) {
	if !strings.HasPrefix(arg, "--") {
		return arg, "", false, false
	}
	if i := strings.IndexByte(arg, '='); i >= 0 {
		return arg[:i], arg[i+1:], true, true
	}
	return arg, "", false, false
}

func resolveCheckInputs(ctx context.Context, a checkArgs, repo string) (time.Time, string, string, string) {
	var today time.Time
	if !a.todaySet {
		today = todayUTC(nil)
	} else if d, ok := parseISODate(a.today); ok {
		today = d
	} else {
		return time.Time{}, "", "", fmt.Sprintf("invalid --today date %s (expected YYYY-MM-DD)", pyQuote(a.today))
	}
	allowPath := a.allowlist
	if allowPath == "" {
		allowPath = filepath.Join(repo, "scripts", "lint", "suppression_allowlist.toml")
	} else {
		cleaned, err := cleanDir(allowPath)
		if err != nil {
			return time.Time{}, "", "", err.Error()
		}
		// cleanDir on a file EvalSymlinks the file itself when it exists.
		allowPath = cleaned
		if _, err := os.Stat(a.allowlist); err != nil {
			abs, absErr := filepath.Abs(a.allowlist)
			if absErr == nil {
				allowPath = abs
			}
		}
	}
	if !isRegularFile(allowPath) {
		return time.Time{}, "", "", fmt.Sprintf("allowlist not found: %s", allowPath)
	}
	var base string
	if a.baseSet {
		resolved, msg, err := validateGitRev(ctx, repo, a.base)
		if err != nil {
			return time.Time{}, "", "", "suppression check failed to execute: " + err.Error()
		}
		if msg != "" {
			return time.Time{}, "", "", msg
		}
		base = resolved
	} else {
		resolved, err := resolveAutoBase(ctx, repo)
		if err != nil {
			return time.Time{}, "", "", "suppression check failed to execute: " + err.Error()
		}
		base = resolved
	}
	return today, allowPath, base, ""
}

func resolveBaseline(a checkArgs, repo string) (map[noqaKey]int, string) {
	if a.noBaseline {
		return map[noqaKey]int{}, ""
	}
	path := ""
	if a.baselineSet {
		cleaned, err := filepath.Abs(a.baseline)
		if err != nil {
			return nil, err.Error()
		}
		if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
			cleaned = resolved
		}
		if !isRegularFile(cleaned) {
			return nil, fmt.Sprintf("python_noqa baseline not found: %s", cleaned)
		}
		path = cleaned
	} else {
		candidate := filepath.Join(repo, "scripts", "lint", "ratchet", "python_noqa_baseline.toml")
		if isRegularFile(candidate) {
			path = candidate
		}
	}
	if path == "" {
		return map[noqaKey]int{}, ""
	}
	text, err := readFileStrict(path)
	if err != nil {
		return nil, err.Error()
	}
	counts, err := loadPythonNoqaBaseline(text)
	if err != nil {
		return nil, err.Error()
	}
	return counts, ""
}

func loadPythonNoqaBaseline(text string) (map[noqaKey]int, error) {
	data, err := parseTOML(text)
	if err != nil {
		return nil, err
	}
	result := map[noqaKey]int{}
	entries, _ := asSlice(data["entries"])
	for _, raw := range entries {
		entry, ok := asMap(raw)
		if !ok {
			continue
		}
		pathVal, pathOK := entry["path"].(string)
		codesVal, codesOK := stringList(entry["codes"])
		shaVal, shaOK := entry["line_sha256"].(string)
		countVal, countOK := asPositiveInt(entry["count"])
		if !countOK {
			if _, present := entry["count"]; !present {
				countVal, countOK = 1, true
			}
		}
		if pathOK && codesOK && shaOK && countOK && countVal > 0 {
			codes := splitUpperCodes(strings.Join(codesVal, ","))
			key := noqaKey{pathVal, codesKey(codes), shaVal}
			result[key] += countVal
		}
	}
	return result, nil
}

func asPositiveInt(v any) (int, bool) {
	switch t := v.(type) {
	case int64:
		return int(t), true
	case int:
		return t, true
	case uint64:
		return int(t), true
	default:
		return 0, false
	}
}

func updatePythonNoqaBaseline(ctx context.Context, a checkArgs, repo string, stdout, stderr io.Writer) int {
	target := filepath.Join(repo, "scripts", "lint", "ratchet", "python_noqa_baseline.toml")
	if a.baselineSet {
		abs, err := filepath.Abs(a.baseline)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
			return exitUsage
		}
		target = abs
	}
	counts, err := collectPythonNoqaReasonless(ctx, repo)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: suppression check failed to execute: %s\n", err.Error())
		return exitUsage
	}
	if err := os.MkdirAll(filepath.Dir(target), fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return exitUsage
	}
	if err := os.WriteFile(target, []byte(renderPythonNoqaBaseline(counts)), fsperm.File); err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return exitUsage
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	fmt.Fprintf(stdout, "Updated %s (%d unique fingerprints, %d total occurrences).\n", target, len(counts), total)
	return exitClean
}

func renderPythonNoqaBaseline(counts map[noqaKey]int) string {
	type row struct {
		key   noqaKey
		count int
		codes []string
	}
	rows := make([]row, 0, len(counts))
	for key, count := range counts {
		var codes []string
		if key.Codes != "" {
			codes = strings.Split(key.Codes, "\x00")
		}
		rows = append(rows, row{key, count, codes})
	}
	sort.Slice(rows, func(i, j int) bool { return noqaLess(rows[i].key, rows[j].key) })
	var b strings.Builder
	b.WriteString("# Frozen baseline of pre-existing reasonless Python `# noqa: CODE` / `# nosec B<NNN>` occurrences (CHECK-03).\n")
	b.WriteString("# Auto-generated by `./scripts/altharness/alt-harness suppressions --update-python-noqa-baseline`.\n")
	b.WriteString("# New reasonless `# noqa: CODE` / `# nosec B<NNN>` lines are rejected; removing existing ones lowers this ledger.\n")
	b.WriteString("\nversion = 1\n\n")
	for _, row := range rows {
		parts := make([]string, len(row.codes))
		for i, c := range row.codes {
			enc, _ := json.Marshal(c)
			parts[i] = string(enc)
		}
		pathEnc, _ := json.Marshal(row.key.Path)
		shaEnc, _ := json.Marshal(row.key.SHA)
		fmt.Fprintf(&b, "[[entries]]\npath = %s\ncodes = [%s]\nline_sha256 = %s\ncount = %d\n\n",
			pathEnc, strings.Join(parts, ", "), shaEnc, row.count)
	}
	return b.String()
}

func sortRows(rows []struct {
	key   noqaKey
	count int
	codes []string
}) {
	// Compare path, then codes lexicographically, then sha — the same order as a Python tuple.
	for i := 1; i < len(rows); i++ {
		j := i
		for j > 0 && noqaLess(rows[j].key, rows[j-1].key) {
			rows[j], rows[j-1] = rows[j-1], rows[j]
			j--
		}
	}
}

func noqaLess(a, b noqaKey) bool {
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	ac := strings.Split(a.Codes, "\x00")
	bc := strings.Split(b.Codes, "\x00")
	if a.Codes == "" {
		ac = nil
	}
	if b.Codes == "" {
		bc = nil
	}
	n := len(ac)
	if len(bc) < n {
		n = len(bc)
	}
	for i := 0; i < n; i++ {
		if ac[i] != bc[i] {
			return ac[i] < bc[i]
		}
	}
	if len(ac) != len(bc) {
		return len(ac) < len(bc)
	}
	return a.SHA < b.SHA
}

func collectPythonNoqaReasonless(ctx context.Context, repo string) (map[noqaKey]int, error) {
	files, err := gitLsFiles(ctx, repo)
	if err != nil {
		return nil, err
	}
	counts := map[noqaKey]int{}
	for _, rel := range files {
		if !strings.HasSuffix(rel, ".py") || isExcludedSourcePath(rel) {
			continue
		}
		full, ok := safeRepoFile(repo, rel)
		if !ok {
			continue
		}
		text, err := readFileReplace(full)
		if err != nil {
			return nil, err
		}
		if isGeneratedSourceText(text) {
			continue
		}
		lines := splitLines(text)
		for _, c := range scanPyComments(text) {
			if pyFileNoqaRE.MatchString(c.raw) {
				continue
			}
			src := c.raw
			if c.line >= 1 && c.line <= len(lines) {
				src = lines[c.line-1]
			}
			shared := ""
			if m := pyInlineNoqaRE.FindStringSubmatch(c.raw); m != nil {
				rest := m[1]
				idx := pyNoqaCodesRE.FindStringSubmatchIndex(rest)
				if idx != nil && !pyTrailingBadRE.MatchString(rest) {
					codes := splitUpperCodes(rest[idx[2]:idx[3]])
					shared = extractPythonNoqaReason(rest[idx[3]:])
					if !hasSufficientReason(shared) {
						counts[noqaKey{rel, codesKey(codes), lineFingerprintSHA256(src)}]++
					}
				}
			}
			if m := pyNosecRE.FindStringSubmatch(c.raw); m != nil {
				rest := m[1]
				idx := pyNosecCodesRE.FindStringSubmatchIndex(rest)
				if idx == nil {
					continue
				}
				found := bCodeRE.FindAllString(rest[idx[2]:idx[3]], -1)
				var codes []string
				seen := map[string]struct{}{}
				for _, ccode := range found {
					up := strings.ToUpper(ccode)
					if _, ok := seen[up]; ok {
						continue
					}
					seen[up] = struct{}{}
					codes = append(codes, up)
				}
				sortStrings(codes)
				own := extractPythonNosecReason(rest[idx[3]:])
				effective := shared
				if hasSufficientReason(own) {
					effective = own
				}
				if !hasSufficientReason(effective) {
					counts[noqaKey{rel, codesKey(codes), lineFingerprintSHA256(src)}]++
				}
			}
		}
	}
	return counts, nil
}

func checkRepo(ctx context.Context, repo, allowPath string, today time.Time, baseline map[noqaKey]int, baseRev string) ([]violation, error) {
	allowText, err := readFileStrict(allowPath)
	if err != nil {
		return nil, err
	}
	al, violations, err := loadAllowlist(allowText, allowlistRel(allowPath, repo), repo, today)
	if err != nil {
		return nil, err
	}
	files, err := gitLsFiles(ctx, repo)
	if err != nil {
		return nil, err
	}
	for _, rel := range files {
		if strings.HasPrefix(rel, "stopped-using-k8s/") || strings.HasPrefix(rel, "scripts/harness/corpus/") {
			continue
		}
		full, ok := safeRepoFile(repo, rel)
		if !ok {
			continue
		}
		name := filepath.Base(rel)
		switch name {
		case "pyproject.toml":
			body, err := readFileStrict(full)
			if err != nil {
				return nil, err
			}
			extra, err := checkPyproject(rel, body, al)
			if err != nil {
				return nil, err
			}
			violations = append(violations, extra...)
		case ".golangci.yml", ".golangci.yaml":
			body, err := readFileStrict(full)
			if err != nil {
				return nil, err
			}
			extra, err := checkGolangci(rel, body, al)
			if err != nil {
				return nil, err
			}
			violations = append(violations, extra...)
		case "Cargo.toml":
			body, err := readFileStrict(full)
			if err != nil {
				return nil, err
			}
			extra, err := checkCargoToml(rel, body, al)
			if err != nil {
				return nil, err
			}
			violations = append(violations, extra...)
		default:
			if isExcludedSourcePath(rel) {
				continue
			}
			body, err := readFileReplace(full)
			if err != nil {
				return nil, err
			}
			violations = append(violations, checkSourceFile(rel, body, repo, today, baseline)...)
		}
	}
	if baseRev != "" {
		changed, err := changedADRIDs(ctx, repo, baseRev)
		if err != nil {
			return nil, err
		}
		extra, err := checkAllowlistExpiresExtensions(ctx, repo, allowPath, baseRev, changed)
		if err != nil {
			return nil, err
		}
		violations = append(violations, extra...)
		extra, err = checkSourceBroadExpires(ctx, repo, baseRev, changed)
		if err != nil {
			return nil, err
		}
		violations = append(violations, extra...)
	}
	sortViolations(violations)
	return violations, nil
}

func changedADRIDs(ctx context.Context, repo, base string) (map[string]struct{}, error) {
	changed := map[string]struct{}{}
	diffOut, _, _, err := gitCmd(ctx, repo, "diff", "--name-only", "--diff-filter=AMCR", base, "--", "docs/ADR")
	if err != nil {
		return nil, err
	}
	untracked, _, _, err := gitCmd(ctx, repo, "ls-files", "--others", "--exclude-standard", "--", "docs/ADR")
	if err != nil {
		return nil, err
	}
	for _, blob := range []string{diffOut, untracked} {
		for _, raw := range strings.Split(blob, "\n") {
			rel := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
			m := adrPathRE.FindStringSubmatch(rel)
			if m == nil {
				continue
			}
			if isRegularFile(filepath.Join(repo, filepath.FromSlash(rel))) {
				changed[m[1]] = struct{}{}
			}
		}
	}
	return changed, nil
}

type allowExpiresRec struct {
	label    string
	key      string
	loose    string
	hasLoose bool
	exp      time.Time
	adr      string
}

func extractAllowlistExpires(data map[string]any) []allowExpiresRec {
	var records []allowExpiresRec
	if ruff, ok := asMap(data["ruff"]); ok {
		if exp, ok := dateField(ruff["expires"]); ok {
			records = append(records, allowExpiresRec{label: "ruff", key: sig("ruff"), exp: exp, adr: adrField(ruff)})
		}
	}
	if items, ok := asSlice(data["bandit"]); ok {
		for idx, raw := range items {
			entry, ok := asMap(raw)
			if !ok {
				continue
			}
			exp, expOK := dateField(entry["expires"])
			path, pathOK := entry["path"].(string)
			if expOK && pathOK {
				records = append(records, allowExpiresRec{
					label: fmt.Sprintf("bandit[%d]", idx),
					key:   sig("bandit", path),
					exp:   exp, adr: adrField(entry),
				})
			}
		}
	}
	if items, ok := asSlice(data["golangci_exclusions"]); ok {
		for idx, raw := range items {
			entry, ok := asMap(raw)
			if !ok {
				continue
			}
			exp, ok := dateField(entry["expires"])
			if !ok {
				continue
			}
			linters := stringListLoose(entry["linters"])
			sortStrings(linters)
			config := ".golangci.yml"
			if s, ok := entry["config"].(string); ok {
				config = s
			}
			base := sig("golangci_exclusions", config, sprintOpt(entry["path"]), sprintOpt(entry["path_except"]), sprintOpt(entry["text"]), sprintOpt(entry["source"]))
			full := base + "\x1f" + strings.Join(linters, ",")
			records = append(records, allowExpiresRec{
				label: fmt.Sprintf("golangci_exclusions[%d]", idx),
				key:   full, loose: base, hasLoose: true, exp: exp, adr: adrField(entry),
			})
		}
	}
	if items, ok := asSlice(data["cargo_lint_allows"]); ok {
		for idx, raw := range items {
			entry, ok := asMap(raw)
			if !ok {
				continue
			}
			exp, expOK := dateField(entry["expires"])
			path, pathOK := entry["path"].(string)
			section, sectionOK := entry["section"].(string)
			if expOK && pathOK && sectionOK {
				records = append(records, allowExpiresRec{
					label: fmt.Sprintf("cargo_lint_allows[%d]", idx),
					key:   sig("cargo_lint_allows", path, section),
					exp:   exp, adr: adrField(entry),
				})
			}
		}
	}
	return records
}

func sig(parts ...string) string { return strings.Join(parts, "\x1f") }

func sprintOpt(v any) string {
	if v == nil {
		return "\x00"
	}
	return fmt.Sprint(v)
}

func dateField(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, false
	}
	return parseISODate(s)
}

func adrField(entry map[string]any) string {
	s, ok := entry["adr"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func checkAllowlistExpiresExtensions(ctx context.Context, repo, allowPath, base string, changed map[string]struct{}) ([]violation, error) {
	rel := allowlistRel(allowPath, repo)
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
		return nil, nil
	}
	baseText, ok, err := gitShow(ctx, repo, base, rel)
	if err != nil || !ok {
		return nil, err
	}
	baseData, err := parseTOML(baseText)
	if err != nil {
		return nil, nil
	}
	curText, err := readFileStrict(allowPath)
	if err != nil {
		return nil, nil
	}
	curData, err := parseTOML(curText)
	if err != nil {
		return nil, nil
	}
	baseExact := map[string]time.Time{}
	baseLoose := map[string]time.Time{}
	for _, rec := range extractAllowlistExpires(baseData) {
		baseExact[rec.key] = rec.exp
		if rec.hasLoose {
			if prev, ok := baseLoose[rec.loose]; !ok || rec.exp.Before(prev) {
				baseLoose[rec.loose] = rec.exp
			}
		}
	}
	var violations []violation
	for _, rec := range extractAllowlistExpires(curData) {
		prev, ok := baseExact[rec.key]
		if !ok && rec.hasLoose {
			prev, ok = baseLoose[rec.loose]
		}
		if !ok || !rec.exp.After(prev) {
			continue
		}
		adrNum := ""
		if m := adrExactRE.FindStringSubmatch(rec.adr); m != nil {
			adrNum = m[1]
		}
		if adrNum != "" {
			if _, changedADR := changed[adrNum]; changedADR {
				continue
			}
		}
		cited := "a cited docs/ADR/NNNNNN.md"
		if adrNum != "" {
			cited = "docs/ADR/" + adrNum + ".md"
		}
		violations = append(violations, violation{
			Path: rel, Rule: "config.allowlist.expires-extended-without-adr-update",
			Message: fmt.Sprintf("%s: 'expires' extended from %s to %s without modifying or adding %s in the same diff",
				rec.label, prev.Format("2006-01-02"), rec.exp.Format("2006-01-02"), cited),
		})
	}
	return violations, nil
}

func checkSourceBroadExpires(ctx context.Context, repo, base string, changed map[string]struct{}) ([]violation, error) {
	diffOut, err := gitChecked(ctx, repo, "diff", "--name-only", base, "--")
	if err != nil {
		// Python returns no violations when git diff exits non-zero, and fails closed on a missing git.
		if ctx.Err() != nil || gitMissing(err) || err.Error() == "git" {
			return nil, err
		}
		return nil, nil
	}
	var violations []violation
	for _, rel := range nonEmptyLines(diffOut) {
		rel = strings.TrimSpace(rel)
		if rel == "" || isExcludedSourcePath(rel) {
			continue
		}
		full, ok := safeRepoFile(repo, rel)
		if !ok {
			continue
		}
		baseText, found, err := gitShow(ctx, repo, base, rel)
		if err != nil || !found {
			if err != nil {
				return nil, err
			}
			continue
		}
		baseRecs := collectBroad(rel, baseText)
		if len(baseRecs) == 0 {
			continue
		}
		curText, err := readFileReplace(full)
		if err != nil {
			return nil, err
		}
		curRecs := collectBroad(rel, curText)
		if len(curRecs) == 0 {
			continue
		}
		baseBy := map[string][]broadRecord{}
		curBy := map[string][]broadRecord{}
		var order []string
		for _, rec := range baseRecs {
			baseBy[rec.desc] = append(baseBy[rec.desc], rec)
		}
		for _, rec := range curRecs {
			if _, seen := curBy[rec.desc]; !seen {
				order = append(order, rec.desc)
			}
			curBy[rec.desc] = append(curBy[rec.desc], rec)
		}
		for _, desc := range order {
			if baseGroup, ok := baseBy[desc]; ok {
				violations = append(violations, compareBroadGroup(rel, baseGroup, curBy[desc], changed)...)
			}
		}
	}
	return violations, nil
}

func reportviolations(stdout, stderr io.Writer, vs []violation) int {
	if len(vs) > 0 {
		for _, v := range vs {
			fmt.Fprintf(stderr, "%s: [%s] %s\n", v.loc(), v.Rule, v.Message)
		}
		fmt.Fprintf(stderr, "\nFAIL: %d suppression governance violation(s) found.\n", len(vs))
		return exitViolations
	}
	fmt.Fprintln(stdout, "OK: suppression governance and config allowlist checks clean.")
	return exitClean
}
