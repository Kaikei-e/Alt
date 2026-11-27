package loopcorrect

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"alt/scripts/altharness/internal/corpus"
	"alt/scripts/altharness/internal/guards"
	"alt/scripts/altharness/internal/tomlx"
)

var (
	classTagRE   = regexp.MustCompile(`(?:^|[\s,;(\[])class=([A-Za-z0-9][A-Za-z0-9._-]{0,63})`)
	corpusFindRE = regexp.MustCompile(`C-(\d{2,})`)
	errorCodeRE  = regexp.MustCompile(`\[([A-Z][A-Z0-9_-]{1,63})\]`)
)

type decisionRow struct {
	TS         string
	Phase      string
	Decision   string
	Why        string
	Evidence   string
	Result     string
	SourceFile string
}

type corpusIndex struct {
	byID       map[string]string   // C-NN -> C-NN
	bySignal   map[string][]string // signal -> []C-NN
	byCategory map[string][]string // category -> []C-NN
}

func cmdScan(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	_ = ctx
	p, err := parseFlags(args,
		map[string]bool{
			"--decisions-file": true,
			"--runs-dir":       true,
			"--branch":         true,
			"--threshold":      true,
			"--repo-root":      true,
			"--format":         true,
		},
		nil,
	)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if p.help {
		writeUsage(stdout)
		return exitOK
	}
	if len(p.positionals) > 0 {
		writef(stderr, "error: unrecognized arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	format, err := parseFormatFlag(p)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	threshold := defaultThreshold
	if raw, ok := p.values["--threshold"]; ok {
		n, convErr := strconv.Atoi(strings.TrimSpace(raw))
		if convErr != nil || n < 1 {
			writef(stderr, "error: --threshold must be a positive integer, got %q\n", raw)
			return exitUsage
		}
		threshold = n
	}
	decisionsFile := strings.TrimSpace(p.values["--decisions-file"])
	runsDir := strings.TrimSpace(p.values["--runs-dir"])
	branch := strings.TrimSpace(p.values["--branch"])
	if decisionsFile != "" && runsDir != "" {
		writeln(stderr, "error: --decisions-file and --runs-dir are mutually exclusive")
		return exitUsage
	}

	repoRoot := guards.ResolveRepoRoot(p.values["--repo-root"], "")

	files, err := resolveScanFiles(repoRoot, decisionsFile, runsDir, branch)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}

	report, err := ScanFiles(repoRoot, files, threshold)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitFail
	}

	if format == "json" {
		raw, err := marshalPrettyJSON(report)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return exitFail
		}
		_, _ = stdout.Write(raw)
		return exitOK
	}
	writeScanText(stdout, report)
	return exitOK
}

func resolveScanFiles(repoRoot, decisionsFile, runsDir, branch string) ([]string, error) {
	if decisionsFile != "" {
		info, err := os.Stat(decisionsFile)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("decisions file not found: %s", decisionsFile)
		}
		return []string{decisionsFile}, nil
	}
	if runsDir != "" {
		info, err := os.Stat(runsDir)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("runs directory not found: %s", runsDir)
		}
		entries, err := os.ReadDir(runsDir)
		if err != nil {
			return nil, fmt.Errorf("read runs directory %s: %w", runsDir, err)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		var out []string
		for _, ent := range entries {
			if !ent.IsDir() {
				continue
			}
			candidate := filepath.Join(runsDir, ent.Name(), "decisions.tsv")
			if st, err := os.Stat(candidate); err == nil && st.Mode().IsRegular() {
				out = append(out, candidate)
			}
		}
		return out, nil
	}
	runDir, err := guards.RunDirForBranch(repoRoot, branch)
	if err != nil {
		return nil, err
	}
	candidate := filepath.Join(runDir, "decisions.tsv")
	if st, err := os.Stat(candidate); err == nil && st.Mode().IsRegular() {
		return []string{candidate}, nil
	}
	return []string{}, nil
}

// ScanFiles parses one or more decisions.tsv files, filters failure rows
// (`reverted` or `mismatch`), classifies them deterministically, and checks
// existing CORPUS-01 mappings.
func ScanFiles(repoRoot string, files []string, threshold int) (ScanReport, error) {
	if threshold < 1 {
		threshold = defaultThreshold
	}
	cidx := loadCorpusIndex(repoRoot)

	type groupAcc struct {
		classKey     string
		matchSource  string
		count        int
		corpusSet    map[string]struct{}
		evidenceList []string
		evidenceSet  map[string]struct{}
		timestamps   []string
		phaseSet     map[string]struct{}
		results      map[string]int
		samples      []string
		sampleSet    map[string]struct{}
		sourceSet    map[string]struct{}
	}

	groups := map[string]*groupAcc{}
	totalFailures := 0
	scannedFiles := make([]string, 0, len(files))

	for _, f := range files {
		rows, err := readDecisionRows(f)
		if err != nil {
			return ScanReport{}, fmt.Errorf("read %s: %w", f, err)
		}
		scannedFiles = append(scannedFiles, displayRelPath(repoRoot, f))
		for _, row := range rows {
			if row.Result != "reverted" && row.Result != "mismatch" {
				continue
			}
			totalFailures++
			classKey, source := ExtractFailureClassKey(row.Phase, row.Decision, row.Why, row.Evidence)
			acc, ok := groups[classKey]
			if !ok {
				acc = &groupAcc{
					classKey:    classKey,
					matchSource: source,
					corpusSet:   map[string]struct{}{},
					evidenceSet: map[string]struct{}{},
					phaseSet:    map[string]struct{}{},
					results:     map[string]int{},
					sampleSet:   map[string]struct{}{},
					sourceSet:   map[string]struct{}{},
				}
				groups[classKey] = acc
			}
			acc.count++
			if row.TS != "" {
				acc.timestamps = append(acc.timestamps, row.TS)
			}
			if row.Phase != "" {
				acc.phaseSet[row.Phase] = struct{}{}
			}
			acc.results[row.Result]++
			acc.sourceSet[displayRelPath(repoRoot, row.SourceFile)] = struct{}{}
			if ev := strings.TrimSpace(row.Evidence); ev != "" {
				if _, seen := acc.evidenceSet[ev]; !seen {
					acc.evidenceSet[ev] = struct{}{}
					acc.evidenceList = append(acc.evidenceList, ev)
				}
			}
			if dec := strings.TrimSpace(row.Decision); dec != "" && len(acc.samples) < 3 {
				if _, seen := acc.sampleSet[dec]; !seen {
					acc.sampleSet[dec] = struct{}{}
					acc.samples = append(acc.samples, dec)
				}
			}
			for _, cid := range cidx.matchRow(classKey, row) {
				acc.corpusSet[cid] = struct{}{}
			}
		}
	}

	recurring := make([]ClassSummary, 0)
	singles := make([]ClassSummary, 0)
	for _, acc := range groups {
		cids := sortedKeys(acc.corpusSet)
		phases := sortedKeys(acc.phaseSet)
		sources := sortedKeys(acc.sourceSet)
		ev := acc.evidenceList
		if ev == nil {
			ev = []string{}
		}
		ts := acc.timestamps
		if ts == nil {
			ts = []string{}
		}
		samples := acc.samples
		if samples == nil {
			samples = []string{}
		}
		summary := ClassSummary{
			ClassKey:        acc.classKey,
			MatchSource:     acc.matchSource,
			Count:           acc.count,
			CorpusMapped:    len(cids) > 0,
			CorpusIDs:       cids,
			Evidence:        ev,
			Timestamps:      ts,
			Phases:          phases,
			Results:         acc.results,
			SampleDecisions: samples,
			SourceFiles:     sources,
		}
		if acc.count >= threshold {
			recurring = append(recurring, summary)
		} else {
			singles = append(singles, summary)
		}
	}

	sortSummaries := func(s []ClassSummary) {
		sort.Slice(s, func(i, j int) bool {
			if s[i].Count != s[j].Count {
				return s[i].Count > s[j].Count
			}
			return s[i].ClassKey < s[j].ClassKey
		})
	}
	sortSummaries(recurring)
	sortSummaries(singles)

	return ScanReport{
		Threshold:            threshold,
		TotalFailuresScanned: totalFailures,
		FilesScanned:         scannedFiles,
		RecurringClasses:     recurring,
		SingleOccurrences:    singles,
	}, nil
}

// ExtractFailureClassKey deterministically derives the failure class key in
// strict precedence order:
//  1. Explicit `class=<id>` token in decision or why
//  2. Explicit corpus ID `C-NN` in decision, why, or evidence
//  3. Explicit bracketed error code `[CODE_NAME]` in decision or why
//  4. Normalized fallback signature from phase + first clause of why
func ExtractFailureClassKey(phase, decision, why, evidence string) (string, string) {
	// 1. Explicit class=<id> in decision or why.
	for _, field := range []string{decision, why} {
		if m := classTagRE.FindStringSubmatch(field); len(m) == 2 {
			token := strings.TrimRight(m[1], ".")
			if token != "" {
				return token, "class_tag"
			}
		}
	}
	// 2. Explicit corpus ID C-NN in decision, why, or evidence.
	for _, field := range []string{decision, why, evidence} {
		if cid := findFirstCorpusID(field); cid != "" {
			return cid, "corpus_id"
		}
	}
	// 3. Explicit bracketed error code [CODE_NAME] in decision or why.
	for _, field := range []string{decision, why} {
		if m := errorCodeRE.FindStringSubmatch(field); len(m) == 2 {
			return m[1], "error_code"
		}
	}
	// 4. Normalized fallback signature from phase + first clause of why.
	phaseSlug := normalizeSlug(phase, 24)
	if phaseSlug == "" {
		phaseSlug = "unknown"
	}
	clauseSlug := normalizeSlug(extractFirstClause(why), maxFallbackClauseRunes)
	if clauseSlug == "" {
		clauseSlug = "unspecified"
	}
	return phaseSlug + ":" + clauseSlug, "fallback"
}

func findFirstCorpusID(text string) string {
	ids := findAllCorpusIDs(text)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func findAllCorpusIDs(text string) []string {
	var out []string
	for _, loc := range corpusFindRE.FindAllStringSubmatchIndex(text, -1) {
		if loc[0] > 0 && isCorpusLeftIdent(text[loc[0]-1]) {
			continue
		}
		if loc[1] < len(text) && text[loc[1]] >= '0' && text[loc[1]] <= '9' {
			continue
		}
		out = append(out, text[loc[0]:loc[1]])
	}
	return out
}

func isCorpusLeftIdent(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '-' || b == '_'
}

func extractFirstClause(why string) string {
	s := strings.TrimSpace(why)
	if s == "" {
		return ""
	}
	cut := len(s)
	for _, sep := range []string{";", "。", "|", "\n", "\r", ". "} {
		if idx := strings.Index(s, sep); idx >= 0 && idx < cut {
			cut = idx
		}
	}
	return strings.TrimRight(strings.TrimSpace(s[:cut]), ".")
}

func normalizeSlug(s string, maxRunes int) string {
	var b strings.Builder
	prevDash := false
	runes := 0
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if maxRunes > 0 && runes >= maxRunes {
				break
			}
			b.WriteRune(r)
			prevDash = false
			runes++
			continue
		}
		if !prevDash && runes > 0 {
			if maxRunes > 0 && runes >= maxRunes {
				break
			}
			b.WriteByte('-')
			prevDash = true
			runes++
		}
	}
	return strings.Trim(b.String(), "-")
}

func readDecisionRows(path string) ([]decisionRow, error) {
	raw, err := readFileInParentRoot(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	start := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "ts\tphase\tdecision\twhy\tevidence\tresult" {
		start = 1
	}
	var rows []decisionRow
	for _, line := range lines[start:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		for len(parts) < 6 {
			parts = append(parts, "")
		}
		rows = append(rows, decisionRow{
			TS:         strings.TrimSpace(parts[0]),
			Phase:      strings.TrimSpace(parts[1]),
			Decision:   strings.TrimSpace(parts[2]),
			Why:        strings.TrimSpace(parts[3]),
			Evidence:   strings.TrimSpace(parts[4]),
			Result:     strings.TrimSpace(parts[5]),
			SourceFile: path,
		})
	}
	return rows, nil
}

func loadCorpusIndex(repoRoot string) corpusIndex {
	idx := corpusIndex{
		byID:       map[string]string{},
		bySignal:   map[string][]string{},
		byCategory: map[string][]string{},
	}
	corpusDir := filepath.Join(repoRoot, corpusDirRel)
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		return idx
	}
	corpusRoot, err := os.OpenRoot(corpusDir)
	if err != nil {
		return idx
	}
	defer func() { _ = corpusRoot.Close() }()
	for _, ent := range entries {
		name := ent.Name()
		if !corpusIDRE.MatchString(name) {
			continue
		}
		caseDir := filepath.Join(corpusDir, name)
		if isSymlink(caseDir) {
			continue
		}
		metaPath := filepath.Join(caseDir, "meta.toml")
		st, err := os.Lstat(metaPath)
		if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
			continue
		}
		idx.byID[name] = name
		raw, err := readAllInRoot(corpusRoot, filepath.Join(name, "meta.toml"))
		if err != nil {
			continue
		}
		doc, err := tomlx.ParseBytes(raw)
		if err != nil {
			continue
		}
		if cat, ok := doc["category"].(string); ok && cat != "" {
			idx.byCategory[cat] = append(idx.byCategory[cat], name)
		}
		if det, ok := doc["detection"].(map[string]any); ok {
			if sig, ok := det["expected_signal"].(string); ok && strings.TrimSpace(sig) != "" {
				idx.bySignal[strings.TrimSpace(sig)] = append(idx.bySignal[strings.TrimSpace(sig)], name)
			}
			if sigs, ok := det["expected_signals"].(map[string]any); ok {
				for _, v := range sigs {
					if sig, ok := v.(string); ok && strings.TrimSpace(sig) != "" {
						idx.bySignal[strings.TrimSpace(sig)] = append(idx.bySignal[strings.TrimSpace(sig)], name)
					}
				}
			}
		}
	}
	return idx
}

func (c corpusIndex) matchRow(classKey string, row decisionRow) []string {
	found := map[string]struct{}{}
	if id, ok := c.byID[classKey]; ok {
		found[id] = struct{}{}
	}
	for _, id := range c.bySignal[classKey] {
		found[id] = struct{}{}
	}
	if _, isValidCat := corpus.ValidCategories[classKey]; isValidCat {
		for _, id := range c.byCategory[classKey] {
			found[id] = struct{}{}
		}
	}
	for _, field := range []string{row.Decision, row.Why, row.Evidence} {
		for _, cid := range findAllCorpusIDs(field) {
			if id, ok := c.byID[cid]; ok {
				found[id] = struct{}{}
			}
		}
	}
	return sortedKeys(found)
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func displayRelPath(repoRoot, target string) string {
	repoResolved := resolveExistingPath(repoRoot)
	targetResolved := resolveExistingPath(target)
	if repoResolved != "" && targetResolved != "" && isWithinDir(targetResolved, repoResolved) {
		if rel, err := filepath.Rel(repoResolved, targetResolved); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(target)
}

func writeScanText(w io.Writer, rep ScanReport) {
	writef(w, "LOOP-07 /correct scan (threshold=%d, failures_scanned=%d, files=%d)\n",
		rep.Threshold, rep.TotalFailuresScanned, len(rep.FilesScanned))
	writef(w, "  recurring_classes : %d\n", len(rep.RecurringClasses))
	for _, c := range rep.RecurringClasses {
		corpusTag := "UNMAPPED"
		if c.CorpusMapped {
			corpusTag = strings.Join(c.CorpusIDs, ",")
		}
		writef(w, "    - %s (count=%d, source=%s, corpus=%s, evidence=%s)\n",
			c.ClassKey, c.Count, c.MatchSource, corpusTag, strings.Join(c.Evidence, ", "))
	}
	writef(w, "  single_occurrences: %d\n", len(rep.SingleOccurrences))
	for _, c := range rep.SingleOccurrences {
		writef(w, "    - %s (count=%d, source=%s)\n", c.ClassKey, c.Count, c.MatchSource)
	}
}
