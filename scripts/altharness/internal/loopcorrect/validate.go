package loopcorrect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"alt/scripts/altharness/internal/guards"
)

var trivialHigherRungReasons = map[string]struct{}{
	"": {}, "tbd": {}, "n/a": {}, "na": {}, "none": {}, "null": {}, "todo": {},
	"hard": {}, "too hard": {}, "too hard to do": {}, "too difficult": {},
	"hard to implement": {}, "cannot": {}, "can't": {}, "cant": {},
	"cannot be done": {}, "impossible": {}, "not possible": {}, "not feasible": {},
	"not applicable": {}, "skip": {}, "skipped": {}, "unnecessary": {},
	"not needed": {}, "unknown": {}, "wip": {}, "fixme": {}, "xxx": {},
	"...": {}, "-": {}, "--": {}, "—": {},
	"なし": {}, "無し": {}, "未定": {}, "未記入": {}, "難しい": {}, "無理": {},
	"できない": {}, "不可": {}, "不要": {},
}

var placeholderReasonPrefixes = []string{
	"tbd", "todo", "fixme", "n/a", "xxx", "未定", "未記入",
}

func cmdValidate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	_ = ctx
	p, err := parseFlags(args,
		map[string]bool{
			"--repo-root": true,
			"--format":    true,
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
	format, err := parseFormatFlag(p)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if len(p.positionals) != 1 {
		writeln(stderr, "error: validate requires exactly one <disposition.json> argument")
		return exitUsage
	}
	filePath := p.positionals[0]
	repoRoot := guards.ResolveRepoRoot(p.values["--repo-root"], "")

	_, report, exitCode, loadErr := loadAndValidateDisposition(filePath, repoRoot)
	if loadErr != "" {
		writeln(stderr, loadErr)
		return exitCode
	}
	emitValidationReport(stdout, stderr, format, report)
	if report.Valid {
		return exitOK
	}
	return exitFail
}

func loadAndValidateDisposition(filePath, repoRoot string) (*Disposition, ValidationReport, int, string) {
	info, err := os.Lstat(filePath)
	if err != nil {
		return nil, ValidationReport{}, exitUsage, fmt.Sprintf("error: disposition file not found: %s", filePath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, ValidationReport{}, exitUsage, fmt.Sprintf("error: disposition file must not be a symlink: %s", filePath)
	}
	if !info.Mode().IsRegular() {
		return nil, ValidationReport{}, exitUsage, fmt.Sprintf("error: disposition path is not a regular file: %s", filePath)
	}
	raw, err := readFileInParentRoot(filePath)
	if err != nil {
		return nil, ValidationReport{}, exitUsage, fmt.Sprintf("error: failed to read %s: %v", filePath, err)
	}
	var disp Disposition
	if err := json.Unmarshal(raw, &disp); err != nil {
		report := ValidationReport{
			Valid: false,
			File:  filePath,
			Issues: []ValidationIssue{{
				Code:    "PARSE_ERROR",
				Path:    "$",
				Message: fmt.Sprintf("malformed disposition JSON: %v", err),
			}},
		}
		return nil, report, exitFail, ""
	}
	report := ValidateDisposition(&disp, filePath, repoRoot)
	return &disp, report, exitOK, ""
}

// ValidateDisposition enforces all LOOP-07 /correct fail-closed invariants.
func ValidateDisposition(disp *Disposition, filePath, repoRoot string) ValidationReport {
	issues := make([]ValidationIssue, 0)
	if disp == nil {
		issues = append(issues, ValidationIssue{
			Code:    "PARSE_ERROR",
			Path:    "$",
			Message: "disposition must not be nil",
		})
		return ValidationReport{Valid: false, File: filePath, Issues: issues}
	}

	milestoneID := disp.MilestoneID
	if !milestoneIDRE.MatchString(milestoneID) || strings.Contains(milestoneID, "..") {
		issues = append(issues, ValidationIssue{
			Code:    "INVALID_MILESTONE_ID",
			Path:    "milestone_id",
			Message: fmt.Sprintf("milestone_id %q must match ^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$ and contain no '..'", milestoneID),
		})
	}

	if strings.TrimSpace(disp.Service) == "" || strings.ContainsAny(disp.Service, "\t\r\n") {
		issues = append(issues, ValidationIssue{
			Code:    "MISSING_SERVICE",
			Path:    "service",
			Message: "service must be a non-empty single-line string",
		})
	}

	if len(disp.Classes) == 0 {
		issues = append(issues, ValidationIssue{
			Code:    "EMPTY_CLASSES",
			Path:    "classes",
			Message: "classes must contain at least one recurring failure class",
		})
	}

	astGrepIDs := loadAstGrepRuleIDs(repoRoot)
	seenClassIDs := map[string]struct{}{}

	for i, c := range disp.Classes {
		prefix := fmt.Sprintf("classes[%d]", i)
		validateClassDisposition(c, prefix, repoRoot, astGrepIDs, seenClassIDs, &issues)
	}

	return ValidationReport{
		Valid:       len(issues) == 0,
		File:        filePath,
		MilestoneID: disp.MilestoneID,
		Service:     disp.Service,
		ClassCount:  len(disp.Classes),
		Issues:      issues,
	}
}

func validateClassDisposition(
	c ClassDisposition,
	prefix string,
	repoRoot string,
	astGrepIDs map[string]struct{},
	seenClassIDs map[string]struct{},
	issues *[]ValidationIssue,
) {
	classID := strings.TrimSpace(c.ClassID)
	if classID == "" {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_CLASS_ID",
			Path:    prefix + ".class_id",
			Message: "class_id must not be empty",
		})
	} else if strings.ContainsAny(c.ClassID, "\t\r\n") {
		*issues = append(*issues, ValidationIssue{
			Code:    "INVALID_CLASS_ID",
			Path:    prefix + ".class_id",
			Message: "class_id must not contain tabs or newlines",
		})
	} else {
		if _, dup := seenClassIDs[classID]; dup {
			*issues = append(*issues, ValidationIssue{
				Code:    "DUPLICATE_CLASS_ID",
				Path:    prefix + ".class_id",
				Message: fmt.Sprintf("duplicate class_id %q", classID),
			})
		} else {
			seenClassIDs[classID] = struct{}{}
		}
	}

	if strings.TrimSpace(c.Title) == "" {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_TITLE",
			Path:    prefix + ".title",
			Message: "title must not be empty",
		})
	}

	if c.Occurrences < defaultThreshold {
		*issues = append(*issues, ValidationIssue{
			Code:    "BELOW_CLASS_THRESHOLD",
			Path:    prefix + ".occurrences",
			Message: fmt.Sprintf("occurrences (%d) must be >= %d (2-strike class promotion rule)", c.Occurrences, defaultThreshold),
		})
	}

	if len(c.Evidence) == 0 {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_EVIDENCE",
			Path:    prefix + ".evidence",
			Message: "evidence must contain at least one concrete pointer",
		})
	} else {
		for j, ev := range c.Evidence {
			if strings.ContainsAny(ev, "\t\r\n") || !guards.IsValidEvidencePointer(ev) {
				*issues = append(*issues, ValidationIssue{
					Code:    "INVALID_EVIDENCE_POINTER",
					Path:    fmt.Sprintf("%s.evidence[%d]", prefix, j),
					Message: fmt.Sprintf("evidence %q must be a concrete pointer (file path, commit:<sha>, cmd:<command>, #<seq>, ADR/PM/C id, or URL)", ev),
				})
			}
		}
	}

	validateCorpusRef(c.CorpusID, prefix, repoRoot, issues)
	validateRungAndHigherRejections(c.ChosenRung, c.HigherRungsRejected, prefix, issues)
	validateEnforcementRef(c.EnforcementRef, prefix, repoRoot, astGrepIDs, issues)
}

func validateCorpusRef(corpusID, prefix, repoRoot string, issues *[]ValidationIssue) {
	trimmed := strings.TrimSpace(corpusID)
	if !corpusIDRE.MatchString(trimmed) {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_CORPUS_CASE",
			Path:    prefix + ".corpus_id",
			Message: fmt.Sprintf("corpus_id %q must match ^C-\\d{2,}$ and reference an existing CORPUS-01 case", corpusID),
		})
		return
	}
	caseDir := filepath.Join(repoRoot, corpusDirRel, trimmed)
	metaPath := filepath.Join(caseDir, "meta.toml")
	if isSymlink(caseDir) || isSymlink(metaPath) {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_CORPUS_CASE",
			Path:    prefix + ".corpus_id",
			Message: fmt.Sprintf("corpus case %s must not be a symlink", trimmed),
		})
		return
	}
	st, err := os.Stat(metaPath)
	if err != nil || !st.Mode().IsRegular() {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_CORPUS_CASE",
			Path:    prefix + ".corpus_id",
			Message: fmt.Sprintf("corpus case %s not found on disk (%s/%s/meta.toml missing)", trimmed, corpusDirRel, trimmed),
		})
	}
}

func validateRungAndHigherRejections(chosen string, rejected map[string]string, prefix string, issues *[]ValidationIssue) {
	chosenRank, ok := rungRank[chosen]
	if !ok {
		*issues = append(*issues, ValidationIssue{
			Code:    "INVALID_RUNG",
			Path:    prefix + ".chosen_rung",
			Message: fmt.Sprintf("chosen_rung %q must be one of ARCH, CHECK, VERIFY, LEDGER", chosen),
		})
		return
	}

	// Reject unknown rung keys in higher_rungs_rejected to prevent typos.
	if len(rejected) > 0 {
		keys := make([]string, 0, len(rejected))
		for k := range rejected {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if _, valid := rungRank[k]; !valid {
				*issues = append(*issues, ValidationIssue{
					Code:    "INVALID_RUNG",
					Path:    prefix + ".higher_rungs_rejected." + k,
					Message: fmt.Sprintf("unknown rung key %q in higher_rungs_rejected (valid: ARCH, CHECK, VERIFY, LEDGER)", k),
				})
			}
		}
	}

	// Enforce higher-rung rejection rule: every rung strictly above chosen_rung
	// must have a substantive non-placeholder explanation.
	for _, rung := range rungOrder {
		if rungRank[rung] >= chosenRank {
			break
		}
		reason, exists := rejected[rung]
		if !exists || !isSubstantiveRejectionReason(reason) {
			*issues = append(*issues, ValidationIssue{
				Code: "MISSING_HIGHER_RUNG_REASON",
				Path: prefix + ".higher_rungs_rejected." + rung,
				Message: fmt.Sprintf(
					"choosing rung %s requires a substantive non-placeholder explanation (>= %d runes) for why higher rung %s did not work",
					chosen, minHigherRungReasonRune, rung,
				),
			})
		}
	}
}

func isSubstantiveRejectionReason(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if utf8.RuneCountInString(trimmed) < minHigherRungReasonRune {
		return false
	}
	lower := strings.ToLower(trimmed)
	stripped := strings.TrimRight(lower, ".!?。！？ ")
	if _, trivial := trivialHigherRungReasons[stripped]; trivial {
		return false
	}
	for _, prefix := range placeholderReasonPrefixes {
		if strings.HasPrefix(lower, prefix) {
			rest := lower[len(prefix):]
			if rest == "" {
				return false
			}
			first, _ := utf8.DecodeRuneInString(rest)
			if !unicode.IsLetter(first) && !unicode.IsDigit(first) {
				return false
			}
		}
	}
	distinct := map[rune]struct{}{}
	for _, r := range trimmed {
		if !unicode.IsSpace(r) {
			distinct[unicode.ToLower(r)] = struct{}{}
		}
	}
	return len(distinct) >= minDistinctReasonRunes
}

func validateEnforcementRef(raw, prefix, repoRoot string, astGrepIDs map[string]struct{}, issues *[]ValidationIssue) {
	ref := strings.TrimSpace(raw)
	if ref == "" || strings.ContainsAny(ref, "\t\r\n") {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_ENFORCEMENT_REF",
			Path:    prefix + ".enforcement_ref",
			Message: "enforcement_ref must be a non-empty concrete reference (repo file path, alt-harness <subcommand>, or ast-grep:<id>)",
		})
		return
	}

	// 1. ast-grep:<id>
	if strings.HasPrefix(ref, "ast-grep:") {
		id := strings.TrimSpace(strings.TrimPrefix(ref, "ast-grep:"))
		if id == "" {
			*issues = append(*issues, ValidationIssue{
				Code:    "MISSING_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: "enforcement_ref 'ast-grep:' is missing a rule ID",
			})
			return
		}
		if _, ok := astGrepIDs[id]; !ok {
			*issues = append(*issues, ValidationIssue{
				Code:    "MISSING_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: fmt.Sprintf("ast-grep rule %q not found under %s/*.yml", id, astGrepRulesRel),
			})
		}
		return
	}

	// 2. scripts/altharness/alt-harness <subcommand> or alt-harness <subcommand>
	fields := strings.Fields(ref)
	head := strings.TrimPrefix(fields[0], "./")
	if head == altHarnessRel || head == "alt-harness" {
		if len(fields) < 2 {
			*issues = append(*issues, ValidationIssue{
				Code:    "MISSING_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: fmt.Sprintf("enforcement_ref %q must specify an alt-harness subcommand", ref),
			})
			return
		}
		sub := strings.TrimRight(fields[1], ".,;:")
		if _, ok := validAltHarnessSubcommands[sub]; !ok {
			*issues = append(*issues, ValidationIssue{
				Code:    "MISSING_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: fmt.Sprintf("unknown alt-harness subcommand %q in enforcement_ref %q", sub, ref),
			})
		}
		return
	}

	// 3. Existing repo file path (optional #fragment anchor allowed, e.g. .agents/rules/rule-ledger.md#R-27).
	candidate := head
	if hashIdx := strings.IndexByte(candidate, '#'); hashIdx >= 0 {
		candidate = candidate[:hashIdx]
	}
	if candidate == "" || filepath.IsAbs(candidate) || strings.Contains(candidate, "..") {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_ENFORCEMENT_REF",
			Path:    prefix + ".enforcement_ref",
			Message: fmt.Sprintf("enforcement_ref %q must be a repo-relative file path, alt-harness <subcommand>, or ast-grep:<id>", ref),
		})
		return
	}
	fullPath := filepath.Join(repoRoot, filepath.FromSlash(candidate))
	repoResolved := resolveExistingPath(repoRoot)
	targetResolved := resolveExistingPath(fullPath)
	if !isWithinDir(targetResolved, repoResolved) {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_ENFORCEMENT_REF",
			Path:    prefix + ".enforcement_ref",
			Message: fmt.Sprintf("enforcement_ref %q escapes repository root", ref),
		})
		return
	}
	st, err := os.Stat(fullPath)
	if err != nil || !st.Mode().IsRegular() {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_ENFORCEMENT_REF",
			Path:    prefix + ".enforcement_ref",
			Message: fmt.Sprintf("enforcement_ref %q does not match an existing repo file, alt-harness subcommand, or ast-grep rule", ref),
		})
	}
}

func loadAstGrepRuleIDs(repoRoot string) map[string]struct{} {
	ids := map[string]struct{}{}
	dir := filepath.Join(repoRoot, astGrepRulesRel)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ids
	}
	rulesRoot, err := os.OpenRoot(dir)
	if err != nil {
		return ids
	}
	defer func() { _ = rulesRoot.Close() }()
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		raw, err := readAllInRoot(rulesRoot, name)
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

func emitValidationReport(stdout, stderr io.Writer, format string, rep ValidationReport) {
	if format == "json" {
		raw, err := marshalPrettyJSON(rep)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return
		}
		_, _ = stdout.Write(raw)
		return
	}
	if rep.Valid {
		writef(stdout, "OK: disposition %s valid (milestone=%s, service=%s, classes=%d)\n",
			rep.File, rep.MilestoneID, rep.Service, rep.ClassCount)
		return
	}
	writef(stderr, "FAIL: disposition %s failed validation (%d issue(s)):\n", rep.File, len(rep.Issues))
	for _, iss := range rep.Issues {
		writef(stderr, "  [%s] %s: %s\n", iss.Code, iss.Path, iss.Message)
	}
}
