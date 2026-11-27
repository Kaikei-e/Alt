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

	"alt/scripts/altharness/internal/corpus"
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
	"too hard", "too difficult", "wip", "placeholder",
	"do later", "not possible", "cannot be done", "will fix later",
	"skip", "skipped", "skipping",
}

type enforcementKind int

const (
	enforcementInvalid enforcementKind = iota
	enforcementAstGrep
	enforcementAltHarness
	enforcementFile
)

type parsedEnforcementRef struct {
	kind       enforcementKind
	raw        string
	subcommand string
	normPath   string
	anchor     string
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
	ruleLedgerIDs := loadRuleLedgerIDs(repoRoot)
	seenClassIDs := map[string]struct{}{}

	for i, c := range disp.Classes {
		prefix := fmt.Sprintf("classes[%d]", i)
		validateClassDisposition(c, prefix, repoRoot, astGrepIDs, ruleLedgerIDs, seenClassIDs, &issues)
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
	ruleLedgerIDs map[string]struct{},
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
	parsedRef := validateEnforcementRef(c.EnforcementRef, prefix, repoRoot, astGrepIDs, ruleLedgerIDs, issues)
	validateRungEnforcementCoherence(c.ChosenRung, parsedRef, prefix, issues)
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
	if err := rejectSymlinkChainUnder(repoRoot, metaPath); err != nil {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_CORPUS_CASE",
			Path:    prefix + ".corpus_id",
			Message: fmt.Sprintf("corpus case %s must not traverse symlinks: %v", trimmed, err),
		})
		return
	}
	st, err := os.Lstat(metaPath)
	if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_CORPUS_CASE",
			Path:    prefix + ".corpus_id",
			Message: fmt.Sprintf("corpus case %s not found on disk (%s/%s/meta.toml missing)", trimmed, corpusDirRel, trimmed),
		})
		return
	}

	caseRoot, err := os.OpenRoot(caseDir)
	if err != nil {
		*issues = append(*issues, ValidationIssue{
			Code:    "UNVERIFIED_CORPUS_CASE",
			Path:    prefix + ".corpus_id",
			Message: fmt.Sprintf("corpus case %s could not be opened safely: %v", trimmed, err),
		})
		return
	}
	defer func() { _ = caseRoot.Close() }()

	raw, err := readAllInRoot(caseRoot, "meta.toml")
	if err != nil {
		*issues = append(*issues, ValidationIssue{
			Code:    "UNVERIFIED_CORPUS_CASE",
			Path:    prefix + ".corpus_id",
			Message: fmt.Sprintf("corpus case %s meta.toml unreadable: %v", trimmed, err),
		})
		return
	}
	doc, err := corpus.ParseTOML(string(raw))
	if err != nil {
		*issues = append(*issues, ValidationIssue{
			Code:    "UNVERIFIED_CORPUS_CASE",
			Path:    prefix + ".corpus_id",
			Message: fmt.Sprintf("corpus case %s meta.toml parse error: %v", trimmed, err),
		})
		return
	}

	var reasons []string
	if idVal, ok := doc["id"].(string); !ok || idVal != trimmed {
		reasons = append(reasons, fmt.Sprintf("meta.toml id %q does not match directory %q", idVal, trimmed))
	}

	origin, ok := doc["origin"].(map[string]any)
	if !ok {
		reasons = append(reasons, "missing [origin] table")
	} else {
		baseCommit, _ := origin["base_commit"].(string)
		bc := strings.TrimSpace(baseCommit)
		if bc == "" || bc == "0000000" || isAllZeroHex(bc) {
			reasons = append(reasons, fmt.Sprintf("origin.base_commit %q is missing or placeholder", baseCommit))
		}
		branch, _ := origin["branch"].(string)
		br := strings.TrimSpace(branch)
		if br == "" || strings.HasPrefix(strings.ToUpper(br), "TODO") {
			reasons = append(reasons, fmt.Sprintf("origin.branch %q is missing or placeholder TODO", branch))
		}
	}

	beforeFile, _ := doc["before_file"].(string)
	afterFile, _ := doc["after_file"].(string)
	if !isPlainCaseFilename(beforeFile) {
		reasons = append(reasons, fmt.Sprintf("before_file %q must be a plain filename inside case dir", beforeFile))
	} else if err := checkRegularNonSymlinkInRoot(caseRoot, beforeFile); err != nil {
		reasons = append(reasons, fmt.Sprintf("before_file %q invalid: %v", beforeFile, err))
	}
	if !isPlainCaseFilename(afterFile) {
		reasons = append(reasons, fmt.Sprintf("after_file %q must be a plain filename inside case dir", afterFile))
	} else if err := checkRegularNonSymlinkInRoot(caseRoot, afterFile); err != nil {
		reasons = append(reasons, fmt.Sprintf("after_file %q invalid: %v", afterFile, err))
	}
	if beforeFile != "" && beforeFile == afterFile {
		reasons = append(reasons, "before_file and after_file must be distinct")
	}

	det, ok := doc["detection"].(map[string]any)
	if !ok {
		reasons = append(reasons, "missing [detection] table")
	} else {
		caught, isBool := det["currently_caught"].(bool)
		if !isBool || !caught {
			reasons = append(reasons, "detection.currently_caught must be true (unverified scaffold stubs are not accepted)")
		}
	}

	if len(reasons) > 0 {
		*issues = append(*issues, ValidationIssue{
			Code:    "UNVERIFIED_CORPUS_CASE",
			Path:    prefix + ".corpus_id",
			Message: fmt.Sprintf("corpus case %s failed verification: %s", trimmed, strings.Join(reasons, "; ")),
		})
	}
}

func isAllZeroHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != '0' {
			return false
		}
	}
	return true
}

func isPlainCaseFilename(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.Contains(name, "/") && !strings.Contains(name, `\`) && !strings.Contains(name, "..")
}

func checkRegularNonSymlinkInRoot(root *os.Root, name string) error {
	st, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("must not be a symlink")
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("must be a regular file")
	}
	return nil
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
	normalized := strings.Join(strings.Fields(lower), " ")
	stripped := strings.TrimRight(normalized, ".!?。！？ ")
	if _, trivial := trivialHigherRungReasons[stripped]; trivial {
		return false
	}
	for _, prefix := range placeholderReasonPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			rest := normalized[len(prefix):]
			if rest == "" {
				return false
			}
			first, _ := utf8.DecodeRuneInString(rest)
			if !unicode.IsLetter(first) {
				return false
			}
		}
	}
	letterCount := 0
	distinct := map[rune]struct{}{}
	for _, r := range trimmed {
		if unicode.IsLetter(r) {
			letterCount++
		}
		if !unicode.IsSpace(r) {
			distinct[unicode.ToLower(r)] = struct{}{}
		}
	}
	return letterCount >= minReasonLetters && len(distinct) >= minDistinctReasonRunes
}

func validateEnforcementRef(
	raw, prefix, repoRoot string,
	astGrepIDs, ruleLedgerIDs map[string]struct{},
	issues *[]ValidationIssue,
) parsedEnforcementRef {
	ref := strings.TrimSpace(raw)
	if ref == "" || strings.ContainsAny(ref, "\t\r\n") {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_ENFORCEMENT_REF",
			Path:    prefix + ".enforcement_ref",
			Message: "enforcement_ref must be a non-empty concrete reference (repo file path, alt-harness <subcommand>, or ast-grep:<id>)",
		})
		return parsedEnforcementRef{}
	}

	fields := strings.Fields(ref)

	// 1. ast-grep:<id> (exact single token, no whitespace or trailing garbage).
	if strings.HasPrefix(ref, "ast-grep:") || strings.HasPrefix(fields[0], "ast-grep:") {
		if len(fields) != 1 {
			*issues = append(*issues, ValidationIssue{
				Code:    "INVALID_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: fmt.Sprintf("ast-grep enforcement_ref %q must be a single token 'ast-grep:<id>' without whitespace or extra arguments", ref),
			})
			return parsedEnforcementRef{}
		}
		id := strings.TrimPrefix(ref, "ast-grep:")
		if id == "" {
			*issues = append(*issues, ValidationIssue{
				Code:    "MISSING_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: "enforcement_ref 'ast-grep:' is missing a rule ID",
			})
			return parsedEnforcementRef{}
		}
		if _, ok := astGrepIDs[id]; !ok {
			*issues = append(*issues, ValidationIssue{
				Code:    "MISSING_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: fmt.Sprintf("ast-grep rule %q not found under %s/*.yml", id, astGrepRulesRel),
			})
			return parsedEnforcementRef{}
		}
		return parsedEnforcementRef{kind: enforcementAstGrep, raw: ref}
	}

	// 2. scripts/altharness/alt-harness <subcommand> or alt-harness <subcommand> (exact 2 tokens).
	head := strings.TrimPrefix(fields[0], "./")
	if head == altHarnessRel || head == "alt-harness" {
		if len(fields) < 2 {
			*issues = append(*issues, ValidationIssue{
				Code:    "MISSING_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: fmt.Sprintf("enforcement_ref %q must specify an alt-harness subcommand", ref),
			})
			return parsedEnforcementRef{}
		}
		if len(fields) != 2 {
			*issues = append(*issues, ValidationIssue{
				Code:    "INVALID_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: fmt.Sprintf("alt-harness enforcement_ref %q must be exactly two tokens without trailing arguments", ref),
			})
			return parsedEnforcementRef{}
		}
		sub := fields[1]
		if _, ok := validAltHarnessSubcommands[sub]; !ok {
			*issues = append(*issues, ValidationIssue{
				Code:    "MISSING_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: fmt.Sprintf("unknown alt-harness subcommand %q in enforcement_ref %q", sub, ref),
			})
			return parsedEnforcementRef{}
		}
		return parsedEnforcementRef{kind: enforcementAltHarness, raw: ref, subcommand: sub}
	}

	// 3. Existing repo file path (exact single token; optional #fragment anchor allowed).
	if len(fields) != 1 {
		*issues = append(*issues, ValidationIssue{
			Code:    "INVALID_ENFORCEMENT_REF",
			Path:    prefix + ".enforcement_ref",
			Message: fmt.Sprintf("enforcement_ref %q must be a single token without trailing arguments", ref),
		})
		return parsedEnforcementRef{}
	}

	candidate := head
	anchor := ""
	if hashIdx := strings.IndexByte(candidate, '#'); hashIdx >= 0 {
		anchor = candidate[hashIdx+1:]
		candidate = candidate[:hashIdx]
		if anchor == "" || strings.Contains(anchor, "#") {
			*issues = append(*issues, ValidationIssue{
				Code:    "INVALID_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: fmt.Sprintf("enforcement_ref %q has an invalid empty or malformed anchor", ref),
			})
			return parsedEnforcementRef{}
		}
	}
	if candidate == "" || filepath.IsAbs(candidate) || strings.HasPrefix(candidate, "/") || strings.Contains(candidate, "..") || strings.Contains(candidate, `\`) {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_ENFORCEMENT_REF",
			Path:    prefix + ".enforcement_ref",
			Message: fmt.Sprintf("enforcement_ref %q must be a repo-relative file path, alt-harness <subcommand>, or ast-grep:<id>", ref),
		})
		return parsedEnforcementRef{}
	}
	normPath := filepath.ToSlash(filepath.Clean(candidate))
	if normPath == "." || normPath == ".." || strings.HasPrefix(normPath, "../") {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_ENFORCEMENT_REF",
			Path:    prefix + ".enforcement_ref",
			Message: fmt.Sprintf("enforcement_ref %q must be a repo-relative file path, alt-harness <subcommand>, or ast-grep:<id>", ref),
		})
		return parsedEnforcementRef{}
	}
	fullPath := filepath.Join(repoRoot, filepath.FromSlash(normPath))
	repoResolved := resolveExistingPath(repoRoot)
	targetResolved := resolveExistingPath(fullPath)
	if !isWithinDir(targetResolved, repoResolved) {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_ENFORCEMENT_REF",
			Path:    prefix + ".enforcement_ref",
			Message: fmt.Sprintf("enforcement_ref %q escapes repository root", ref),
		})
		return parsedEnforcementRef{}
	}
	st, err := os.Stat(fullPath)
	if err != nil || !st.Mode().IsRegular() {
		*issues = append(*issues, ValidationIssue{
			Code:    "MISSING_ENFORCEMENT_REF",
			Path:    prefix + ".enforcement_ref",
			Message: fmt.Sprintf("enforcement_ref %q does not match an existing repo file, alt-harness subcommand, or ast-grep rule", ref),
		})
		return parsedEnforcementRef{}
	}
	if normPath == ruleLedgerRel && anchor != "" {
		if !ruleLedgerIDRE.MatchString(anchor) {
			*issues = append(*issues, ValidationIssue{
				Code:    "INVALID_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: fmt.Sprintf("rule-ledger anchor %q in %q must match ^R-\\d{2,}$", anchor, ref),
			})
		} else if _, exists := ruleLedgerIDs[anchor]; !exists {
			*issues = append(*issues, ValidationIssue{
				Code:    "INVALID_ENFORCEMENT_REF",
				Path:    prefix + ".enforcement_ref",
				Message: fmt.Sprintf("rule-ledger anchor %q not found in %s", anchor, ruleLedgerRel),
			})
		}
	}
	return parsedEnforcementRef{
		kind:     enforcementFile,
		raw:      ref,
		normPath: normPath,
		anchor:   anchor,
	}
}

func validateRungEnforcementCoherence(chosenRung string, ref parsedEnforcementRef, prefix string, issues *[]ValidationIssue) {
	if _, valid := rungRank[chosenRung]; !valid || ref.kind == enforcementInvalid {
		return
	}
	if !enforcementMatchesRung(chosenRung, ref) {
		*issues = append(*issues, ValidationIssue{
			Code:    "RUNG_ENFORCEMENT_MISMATCH",
			Path:    prefix + ".enforcement_ref",
			Message: fmt.Sprintf("enforcement_ref %q does not satisfy chosen_rung %s", ref.raw, chosenRung),
		})
	}
}

func enforcementMatchesRung(rung string, ref parsedEnforcementRef) bool {
	switch rung {
	case "ARCH":
		return matchesArchEnforcement(ref)
	case "CHECK":
		return matchesCheckEnforcement(ref)
	case "VERIFY":
		return matchesVerifyEnforcement(ref)
	case "LEDGER":
		return matchesLedgerEnforcement(ref)
	default:
		return false
	}
}

func isDocOrProsePath(normPath string) bool {
	lower := strings.ToLower(normPath)
	base := filepath.Base(normPath)
	if strings.HasSuffix(lower, ".md") {
		return true
	}
	if strings.HasPrefix(normPath, "docs/") || strings.HasPrefix(normPath, ".agents/rules/") || strings.HasPrefix(normPath, ".claude/rules/") {
		return true
	}
	return base == "AGENTS.md" || base == "CLAUDE.md" || base == "README.md"
}

func isSkillPath(normPath string) bool {
	return strings.HasPrefix(normPath, ".claude/skills/") || strings.HasPrefix(normPath, ".agents/skills/")
}

func isSkillScriptPath(normPath string) bool {
	return isSkillPath(normPath) && strings.Contains(normPath, "/scripts/") && !strings.HasSuffix(strings.ToLower(normPath), ".md")
}

func matchesArchEnforcement(ref parsedEnforcementRef) bool {
	switch ref.kind {
	case enforcementAstGrep:
		return true
	case enforcementAltHarness:
		_, ok := archAltHarnessSubcommands[ref.subcommand]
		return ok
	case enforcementFile:
		if isDocOrProsePath(ref.normPath) || isSkillPath(ref.normPath) {
			return false
		}
		ext := strings.ToLower(filepath.Ext(ref.normPath))
		_, ok := archAllowedFileExts[ext]
		return ok
	default:
		return false
	}
}

func matchesCheckEnforcement(ref parsedEnforcementRef) bool {
	switch ref.kind {
	case enforcementAstGrep:
		return true
	case enforcementAltHarness:
		if strings.HasPrefix(ref.subcommand, "guard-") {
			return true
		}
		_, ok := checkAltHarnessSubcommands[ref.subcommand]
		return ok
	case enforcementFile:
		if isDocOrProsePath(ref.normPath) {
			return false
		}
		return strings.HasPrefix(ref.normPath, "scripts/") ||
			strings.HasPrefix(ref.normPath, ".claude/hooks/") ||
			isSkillScriptPath(ref.normPath)
	default:
		return false
	}
}

func matchesVerifyEnforcement(ref parsedEnforcementRef) bool {
	switch ref.kind {
	case enforcementAltHarness:
		_, ok := verifyAltHarnessSubcommands[ref.subcommand]
		return ok
	case enforcementFile:
		if isDocOrProsePath(ref.normPath) {
			return false
		}
		if strings.HasPrefix(ref.normPath, "scripts/verify/") ||
			strings.HasPrefix(ref.normPath, "scripts/tests/") ||
			strings.HasPrefix(ref.normPath, "scripts/harness/eval/") {
			return true
		}
		base := filepath.Base(ref.normPath)
		return strings.HasSuffix(base, "_test.go") ||
			strings.HasSuffix(base, "_test.py") ||
			(strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py")) ||
			strings.Contains(base, ".test.") ||
			strings.Contains(base, ".spec.")
	default:
		return false
	}
}

func matchesLedgerEnforcement(ref parsedEnforcementRef) bool {
	if matchesArchEnforcement(ref) || matchesCheckEnforcement(ref) || matchesVerifyEnforcement(ref) {
		return true
	}
	if ref.kind != enforcementFile {
		return false
	}
	if ref.normPath == ruleLedgerRel ||
		strings.HasPrefix(ref.normPath, ".agents/rules/") ||
		ref.normPath == "AGENTS.md" ||
		ref.normPath == "CLAUDE.md" ||
		isSkillPath(ref.normPath) {
		return true
	}
	return false
}

func loadRuleLedgerIDs(repoRoot string) map[string]struct{} {
	ids := map[string]struct{}{}
	fullPath := filepath.Join(repoRoot, filepath.FromSlash(ruleLedgerRel))
	if !isWithinDir(resolveExistingPath(fullPath), resolveExistingPath(repoRoot)) {
		return ids
	}
	raw, err := readFileInParentRoot(fullPath)
	if err != nil {
		return ids
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if m := ruleLedgerRowRE.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			ids[m[1]] = struct{}{}
		}
	}
	return ids
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
