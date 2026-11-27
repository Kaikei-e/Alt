package loopreview

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"alt/scripts/altharness/internal/guards"
)

var (
	reviewIDRE        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$`)
	lineSuffixRE      = regexp.MustCompile(`(?::\d+(?:[:-]\d+)?|#L\d+(?:-L\d+)?)$`)
	providerPrefixRE  = regexp.MustCompile(`^(?:us|eu|ap|apac|global|ca|me|sa|af|aws|gcp|azure|bedrock|vertex|openrouter|anthropic|google|openai|meta|deepseek|mistral|cohere|amazon|xai|alibaba|qwen)[.:]`)
	nonTokenRE        = regexp.MustCompile(`[^a-z0-9.]+`)
	multiDashRE       = regexp.MustCompile(`-{2,}`)
	geminiTierRE      = regexp.MustCompile(`^gemini-(?:\d+(?:[.-]\d+)*-)?(neon|carbon|pro|flash|ultra)(?:$|-.*)`)
	geminiShortTierRE = regexp.MustCompile(`^(?:\d+(?:[.-]\d+)*-)?(neon|carbon)(?:$|-.*)`)
	geminiBaseRE      = regexp.MustCompile(`^gemini(?:$|-(?:\d+(?:[.-]\d+)*|latest|exp|preview).*)$`)
	claudeTierRE      = regexp.MustCompile(`^claude-(?:\d+(?:[.-]\d+)*-)?(opus|sonnet|haiku)(?:$|-.*)`)
	openaiORe         = regexp.MustCompile(`^(o\d+)(?:$|-.*)`)
	deepseekRE        = regexp.MustCompile(`^(deepseek-[a-z0-9]+)(?:$|-.*)`)
	llamaRE           = regexp.MustCompile(`^(llama-\d+(?:\.\d+)?)(?:$|-.*)`)
	qwenRE            = regexp.MustCompile(`^(qwen\d*(?:\.\d+)?)(?:$|-.*)`)
	trailingVariantRE = regexp.MustCompile(`-(?:xhigh|high|medium|med|low|fast|mini|nano|pro|lite|preview|latest|thinking|exp|instruct|chat|\d{8}|\d{4}-\d{2}-\d{2}|v\d+)$`)

	validBuckets = map[string]struct{}{
		"fix":         {},
		"investigate": {},
		"note":        {},
		"dismiss":     {},
	}

	validFocusStatuses = map[string]struct{}{
		"safe":     {},
		"fixed":    {},
		"deferred": {},
	}

	placeholderValues = map[string]struct{}{
		"":            {},
		"tbd":         {},
		"n/a":         {},
		"na":          {},
		"-":           {},
		"--":          {},
		"—":           {},
		"todo":        {},
		"none":        {},
		"null":        {},
		"done":        {},
		"ok":          {},
		"なし":          {},
		"無し":          {},
		"未記入":         {},
		"未定":          {},
		"xxx":         {},
		"...":         {},
		"placeholder": {},
		"<sha>":       {},
		"<todo>":      {},
	}
)

// NormalizeModelFamily normalizes a model identifier or display name into its
// lineage/family key so same-family reviews (e.g. gemini-4-neon-xhigh vs
// Gemini 4.0 Neon (XHigh) or anthropic.claude-opus-4-6 vs claude-opus-5-5-high)
// can be deterministically rejected.
func NormalizeModelFamily(model string) string {
	s := strings.TrimSpace(strings.ToLower(model))
	if s == "" {
		return ""
	}
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		s = s[idx+1:]
	}
	for {
		next := providerPrefixRE.ReplaceAllString(s, "")
		if next == s {
			break
		}
		s = next
	}
	s = nonTokenRE.ReplaceAllString(s, "-")
	s = multiDashRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-.")
	if s == "" {
		return ""
	}
	if m := geminiTierRE.FindStringSubmatch(s); m != nil {
		return "gemini-" + m[1]
	}
	if m := geminiShortTierRE.FindStringSubmatch(s); m != nil {
		return "gemini-" + m[1]
	}
	if geminiBaseRE.MatchString(s) {
		return "gemini"
	}
	if m := claudeTierRE.FindStringSubmatch(s); m != nil {
		return "claude-" + m[1]
	}
	if s == "gpt" || strings.HasPrefix(s, "gpt-") {
		return "gpt"
	}
	if s == "codex" || strings.HasPrefix(s, "codex-") {
		return "codex"
	}
	if m := openaiORe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if m := deepseekRE.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if m := llamaRE.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if m := qwenRE.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	for {
		next := trailingVariantRE.ReplaceAllString(s, "")
		if next == s || next == "" {
			break
		}
		s = next
	}
	return s
}

func hasFormulaTrigger(s string) bool {
	if s == "" {
		return false
	}
	switch s[0] {
	case '=', '+', '@':
		return true
	case '-':
		if len(s) >= 2 {
			c := s[1]
			return (c >= '0' && c <= '9') || c == '=' || c == '+' || c == '@' || c == '('
		}
	}
	return false
}

func isPlaceholder(s string) bool {
	cleaned := strings.TrimSpace(s)
	if cleaned == "" {
		return true
	}
	_, ok := placeholderValues[strings.ToLower(cleaned)]
	return ok
}

func stripLineSuffix(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "file://") ||
		strings.HasPrefix(lower, "cmd:") ||
		strings.HasPrefix(lower, "commit:") {
		return s
	}
	return lineSuffixRE.ReplaceAllString(s, "")
}

// IsValidEvidence verifies that raw is a concrete evidence pointer, accepting
// file:line and file:line-end references alongside guards.IsValidEvidencePointer
// while rejecting TSV/control characters and spreadsheet formula triggers.
func IsValidEvidence(raw string) bool {
	if strings.ContainsAny(raw, "\t\r\n\x00") {
		return false
	}
	cleaned := strings.TrimSpace(raw)
	if isPlaceholder(cleaned) || hasFormulaTrigger(cleaned) {
		return false
	}
	if guards.IsValidEvidencePointer(cleaned) {
		return true
	}
	stripped := stripLineSuffix(cleaned)
	if stripped == cleaned || hasFormulaTrigger(stripped) {
		return false
	}
	return guards.IsValidEvidencePointer(stripped)
}

// LoadAndValidateReport reads reportPath (rejecting symlinks and directories),
// parses JSON, and runs full fail-closed LOOP-04 validation.
func LoadAndValidateReport(reportPath string, requireFocusAll bool) (*Report, []byte, ValidationResult, int, error) {
	info, err := os.Lstat(reportPath)
	if err != nil {
		return nil, nil, ValidationResult{}, exitUsage, fmt.Errorf("report file not found: %s", reportPath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, ValidationResult{}, exitUsage, fmt.Errorf("report file must not be a symlink: %s", reportPath)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, ValidationResult{}, exitUsage, fmt.Errorf("report path is not a regular file: %s", reportPath)
	}
	raw, err := readRegularFileSafe(reportPath)
	if err != nil {
		return nil, nil, ValidationResult{}, exitUsage, fmt.Errorf("read report %s: %w", reportPath, err)
	}
	var report Report
	if err := json.Unmarshal(raw, &report); err != nil {
		res := ValidationResult{
			Valid:            false,
			Path:             reportPath,
			ReviewerFamilies: []string{},
			BucketCounts:     emptyBucketCounts(),
			Errors: []Issue{{
				Code:    "PARSE_ERROR",
				Path:    reportPath,
				Message: fmt.Sprintf("invalid JSON: %v", err),
			}},
		}
		return nil, raw, res, exitFail, nil
	}
	res := ValidateReport(&report, reportPath, requireFocusAll)
	if !res.Valid {
		return &report, raw, res, exitFail, nil
	}
	return &report, raw, res, exitOK, nil
}

func emptyBucketCounts() map[string]int {
	return map[string]int{
		"fix":         0,
		"investigate": 0,
		"note":        0,
		"dismiss":     0,
	}
}

// ValidateReport enforces all LOOP-04 fail-closed invariants on report.
func ValidateReport(report *Report, path string, requireFocusAll bool) ValidationResult {
	res := ValidationResult{
		Path:             path,
		ReviewerFamilies: []string{},
		BucketCounts:     emptyBucketCounts(),
		Errors:           []Issue{},
	}
	if report == nil {
		res.Errors = append(res.Errors, Issue{
			Code:    "PARSE_ERROR",
			Path:    path,
			Message: "report is nil",
		})
		return res
	}

	res.ReviewID = strings.TrimSpace(report.ReviewID)
	res.FocusVerdictsCount = len(report.FocusVerdicts)
	res.FindingsCount = len(report.Findings)

	if !reviewIDRE.MatchString(report.ReviewID) {
		res.Errors = append(res.Errors, Issue{
			Code:    "INVALID_REVIEW_ID",
			Path:    "review_id",
			Message: fmt.Sprintf("review_id %q must match %s (CWE-22 protection)", report.ReviewID, reviewIDRE.String()),
		})
	}
	if report.AutoApplied {
		res.Errors = append(res.Errors, Issue{
			Code:    "AUTO_APPLY_FORBIDDEN",
			Path:    "auto_applied",
			Message: "auto_applied must be false; LOOP-04 /interrogate never auto-applies fixes",
		})
	}
	if strings.TrimSpace(report.BaseCommit) != "" && isPlaceholder(report.BaseCommit) {
		res.Errors = append(res.Errors, Issue{
			Code:    "PLACEHOLDER_VALUE",
			Path:    "base_commit",
			Message: fmt.Sprintf("base_commit %q must not be a placeholder", report.BaseCommit),
		})
	}
	if strings.TrimSpace(report.HeadCommit) != "" && isPlaceholder(report.HeadCommit) {
		res.Errors = append(res.Errors, Issue{
			Code:    "PLACEHOLDER_VALUE",
			Path:    "head_commit",
			Message: fmt.Sprintf("head_commit %q must not be a placeholder", report.HeadCommit),
		})
	}

	if len(report.FocusVerdicts) == 0 && len(report.Findings) == 0 {
		res.Errors = append(res.Errors, Issue{
			Code:    "EMPTY_REVIEW_REPORT",
			Path:    "report",
			Message: "review report must contain at least one focus_verdict or finding",
		})
	}

	authorFam := validateModelFamilies(report, &res)
	fixFocusIDs := collectFixFocusIDs(report.Findings)
	validateFocusVerdicts(report, requireFocusAll, fixFocusIDs, &res)
	validateFindings(report, authorFam, &res)

	res.Valid = len(res.Errors) == 0
	return res
}

func collectFixFocusIDs(findings []Finding) map[string][]string {
	out := map[string][]string{}
	for _, f := range findings {
		if strings.TrimSpace(f.Bucket) != "fix" {
			continue
		}
		fid := strings.TrimSpace(f.FocusID)
		if fid == "" {
			continue
		}
		id := strings.TrimSpace(f.ID)
		if id == "" {
			id = "(unnamed)"
		}
		out[fid] = append(out[fid], id)
	}
	return out
}

func validateModelFamilies(report *Report, res *ValidationResult) string {
	authorRaw := strings.TrimSpace(report.AuthorModel)
	authorFam := NormalizeModelFamily(authorRaw)
	if isPlaceholder(authorRaw) || authorFam == "" {
		res.Errors = append(res.Errors, Issue{
			Code:    "SINGLE_MODEL_FAMILY_REVIEW",
			Path:    "author_model",
			Message: "author_model must be a non-empty model identifier",
		})
	} else {
		res.AuthorFamily = authorFam
	}

	if len(report.Reviewers) == 0 {
		res.Errors = append(res.Errors, Issue{
			Code:    "SINGLE_MODEL_FAMILY_REVIEW",
			Path:    "reviewers",
			Message: "reviewers must include at least one reviewer model from a distinct model family",
		})
		return authorFam
	}

	famSet := map[string]struct{}{}
	hasDistinctFamily := false
	for i, rev := range report.Reviewers {
		revTrim := strings.TrimSpace(rev)
		revFam := NormalizeModelFamily(revTrim)
		if isPlaceholder(revTrim) || revFam == "" {
			res.Errors = append(res.Errors, Issue{
				Code:    "EMPTY_REVIEWER_MODEL",
				Path:    fmt.Sprintf("reviewers[%d]", i),
				Message: "reviewer model must be a non-empty model identifier",
			})
			continue
		}
		famSet[revFam] = struct{}{}
		if authorFam != "" && revFam != authorFam {
			hasDistinctFamily = true
		}
	}
	fams := make([]string, 0, len(famSet))
	for fam := range famSet {
		fams = append(fams, fam)
	}
	sort.Strings(fams)
	res.ReviewerFamilies = fams

	if authorFam != "" && !hasDistinctFamily {
		res.Errors = append(res.Errors, Issue{
			Code:    "SINGLE_MODEL_FAMILY_REVIEW",
			Path:    "reviewers",
			Message: fmt.Sprintf("at least one reviewer must belong to a different model family than author_model (%q -> family %q); got families %v", report.AuthorModel, authorFam, fams),
		})
	}
	return authorFam
}

func validateFocusVerdicts(report *Report, requireFocusAll bool, fixFocusIDs map[string][]string, res *ValidationResult) {
	seen := map[string]int{}
	for i, fv := range report.FocusVerdicts {
		prefix := fmt.Sprintf("focus_verdicts[%d]", i)
		fid := strings.TrimSpace(fv.FocusID)
		if isPlaceholder(fid) {
			res.Errors = append(res.Errors, Issue{
				Code:    "EMPTY_FOCUS_ID",
				Path:    prefix + ".focus_id",
				Message: "focus_id must be non-empty and not a placeholder",
			})
		} else if prev, dup := seen[fid]; dup {
			res.Errors = append(res.Errors, Issue{
				Code:    "DUPLICATE_FOCUS_ID",
				Path:    prefix + ".focus_id",
				Message: fmt.Sprintf("duplicate focus_id %q (first seen at focus_verdicts[%d])", fid, prev),
			})
		} else {
			seen[fid] = i
		}

		status := strings.TrimSpace(fv.Status)
		if _, ok := validFocusStatuses[status]; !ok {
			res.Errors = append(res.Errors, Issue{
				Code:    "INVALID_FOCUS_STATUS",
				Path:    prefix + ".status",
				Message: fmt.Sprintf("invalid focus status %q (must be one of safe, fixed, deferred)", fv.Status),
			})
		}

		if isPlaceholder(fv.Summary) {
			code := "EMPTY_FOCUS_SUMMARY"
			msg := "focus verdict summary must be non-empty and not a placeholder"
			if status == "deferred" {
				code = "DEFERRED_MISSING_RATIONALE"
				msg = "deferred focus verdict must explain why it was deferred in summary"
			}
			res.Errors = append(res.Errors, Issue{
				Code:    code,
				Path:    prefix + ".summary",
				Message: msg,
			})
		}

		if !IsValidEvidence(fv.Evidence) {
			res.Errors = append(res.Errors, Issue{
				Code:    "INVALID_EVIDENCE",
				Path:    prefix + ".evidence",
				Message: fmt.Sprintf("focus verdict evidence %q must be a concrete pointer (file path, file:line, commit:<sha>, cmd:<command>, ADR/PM/C id, or URL)", fv.Evidence),
			})
		}

		if status == "safe" {
			if fixIDs := fixFocusIDs[fid]; len(fixIDs) > 0 {
				res.Errors = append(res.Errors, Issue{
					Code:    "CONTRADICTORY_SAFE_VERDICT",
					Path:    prefix + ".status",
					Message: fmt.Sprintf("focus_id %q cannot have status='safe' while fix finding(s) %v exist for the same focus_id (must be 'fixed' or 'deferred')", fid, fixIDs),
				})
			}
			validateBlastRadiusProof(fv.BlastRadiusProof, prefix, res)
		}
	}

	if requireFocusAll {
		for _, item := range CanonicalFocusAreas() {
			if _, ok := seen[item.ID]; !ok {
				res.Errors = append(res.Errors, Issue{
					Code:    "MISSING_FOCUS_ITEM",
					Path:    "focus_verdicts",
					Message: fmt.Sprintf("required LOOP-04 focus area %s (%s) is missing from focus_verdicts", item.ID, item.Title),
				})
			}
		}
	}
}

func validateBlastRadiusProof(brp *BlastRadiusProof, prefix string, res *ValidationResult) {
	if brp == nil {
		res.Errors = append(res.Errors, Issue{
			Code:    "UNPROVEN_SAFE_CLAIM",
			Path:    prefix + ".blast_radius_proof",
			Message: "status='safe' requires blast_radius_proof proving safety by running real code/tests (never prose-only)",
		})
		return
	}
	if isPlaceholder(brp.Assumption) {
		res.Errors = append(res.Errors, Issue{
			Code:    "UNPROVEN_SAFE_CLAIM",
			Path:    prefix + ".blast_radius_proof.assumption",
			Message: "blast_radius_proof.assumption must be non-empty and not a placeholder",
		})
	}
	if isPlaceholder(brp.Command) {
		res.Errors = append(res.Errors, Issue{
			Code:    "UNPROVEN_SAFE_CLAIM",
			Path:    prefix + ".blast_radius_proof.command",
			Message: "blast_radius_proof.command must be non-empty and not a placeholder",
		})
	}
	if brp.ExitCode == nil || *brp.ExitCode != 0 {
		msg := "blast_radius_proof.exit_code is required and must be 0"
		if brp.ExitCode != nil {
			msg = fmt.Sprintf("blast_radius_proof.exit_code must be 0 to prove safety, got %d", *brp.ExitCode)
		}
		res.Errors = append(res.Errors, Issue{
			Code:    "UNPROVEN_SAFE_CLAIM",
			Path:    prefix + ".blast_radius_proof.exit_code",
			Message: msg,
		})
	}
	if !IsValidEvidence(brp.Evidence) {
		res.Errors = append(res.Errors, Issue{
			Code:    "UNPROVEN_SAFE_CLAIM",
			Path:    prefix + ".blast_radius_proof.evidence",
			Message: fmt.Sprintf("blast_radius_proof.evidence %q must be a concrete pointer", brp.Evidence),
		})
	}
}

func validateFindings(report *Report, authorFam string, res *ValidationResult) {
	declaredReviewers := make(map[string]struct{}, len(report.Reviewers))
	for _, rev := range report.Reviewers {
		key := strings.ToLower(strings.TrimSpace(rev))
		if key != "" {
			declaredReviewers[key] = struct{}{}
		}
	}

	seen := map[string]int{}
	for i, f := range report.Findings {
		prefix := fmt.Sprintf("findings[%d]", i)
		id := strings.TrimSpace(f.ID)
		if isPlaceholder(id) {
			res.Errors = append(res.Errors, Issue{
				Code:    "EMPTY_FINDING_ID",
				Path:    prefix + ".id",
				Message: "finding id must be non-empty and not a placeholder",
			})
		} else if prev, dup := seen[id]; dup {
			res.Errors = append(res.Errors, Issue{
				Code:    "DUPLICATE_FINDING_ID",
				Path:    prefix + ".id",
				Message: fmt.Sprintf("duplicate finding id %q (first seen at findings[%d])", id, prev),
			})
		} else {
			seen[id] = i
		}

		if isPlaceholder(f.FocusID) {
			res.Errors = append(res.Errors, Issue{
				Code:    "EMPTY_FINDING_FOCUS_ID",
				Path:    prefix + ".focus_id",
				Message: "finding focus_id must be non-empty and not a placeholder",
			})
		}
		revTrim := strings.TrimSpace(f.ReviewerModel)
		revFam := NormalizeModelFamily(revTrim)
		if isPlaceholder(revTrim) || revFam == "" {
			res.Errors = append(res.Errors, Issue{
				Code:    "EMPTY_REVIEWER_MODEL",
				Path:    prefix + ".reviewer_model",
				Message: "finding reviewer_model must be a non-empty model identifier",
			})
		} else {
			if _, ok := declaredReviewers[strings.ToLower(revTrim)]; !ok {
				res.Errors = append(res.Errors, Issue{
					Code:    "UNDECLARED_FINDING_REVIEWER",
					Path:    prefix + ".reviewer_model",
					Message: fmt.Sprintf("finding reviewer_model %q must match a reviewer declared in reviewers", f.ReviewerModel),
				})
			}
			if authorFam != "" && revFam == authorFam {
				res.Errors = append(res.Errors, Issue{
					Code:    "SINGLE_MODEL_FAMILY_REVIEW",
					Path:    prefix + ".reviewer_model",
					Message: fmt.Sprintf("finding reviewer_model %q (family %q) must belong to a different model family than author_model (%q -> family %q)", f.ReviewerModel, revFam, report.AuthorModel, authorFam),
				})
			}
		}
		bucket := strings.TrimSpace(f.Bucket)
		if _, ok := validBuckets[bucket]; !ok {
			res.Errors = append(res.Errors, Issue{
				Code:    "INVALID_BUCKET",
				Path:    prefix + ".bucket",
				Message: fmt.Sprintf("invalid bucket %q (must be one of fix, investigate, note, dismiss)", f.Bucket),
			})
		} else {
			res.BucketCounts[bucket]++
		}
		if isPlaceholder(f.Summary) {
			res.Errors = append(res.Errors, Issue{
				Code:    "EMPTY_FINDING_SUMMARY",
				Path:    prefix + ".summary",
				Message: "finding summary must be non-empty and not a placeholder",
			})
		}
		if isPlaceholder(f.Rationale) {
			res.Errors = append(res.Errors, Issue{
				Code:    "EMPTY_FINDING_RATIONALE",
				Path:    prefix + ".rationale",
				Message: "finding rationale must be non-empty and not a placeholder",
			})
		}
		if !IsValidEvidence(f.Evidence) {
			res.Errors = append(res.Errors, Issue{
				Code:    "INVALID_EVIDENCE",
				Path:    prefix + ".evidence",
				Message: fmt.Sprintf("finding evidence %q must be a concrete pointer (file path, file:line, commit:<sha>, cmd:<command>, ADR/PM/C id, or URL)", f.Evidence),
			})
		}
		if bucket == "fix" {
			validateFixRepro(f.Repro, prefix, res)
		}
	}
}

func validateFixRepro(repro *ReproProof, prefix string, res *ValidationResult) {
	if repro == nil {
		res.Errors = append(res.Errors, Issue{
			Code:    "FIX_MISSING_REPRO_PROOF",
			Path:    prefix + ".repro",
			Message: "finding with bucket='fix' requires repro proof with command or test_name, non-zero exit_code, and concrete proof_evidence",
		})
		return
	}
	if isPlaceholder(repro.Command) && isPlaceholder(repro.TestName) {
		res.Errors = append(res.Errors, Issue{
			Code:    "FIX_MISSING_REPRO_PROOF",
			Path:    prefix + ".repro",
			Message: "repro must specify a non-empty command or test_name",
		})
	}
	if !IsValidEvidence(repro.ProofEvidence) {
		res.Errors = append(res.Errors, Issue{
			Code:    "FIX_MISSING_REPRO_PROOF",
			Path:    prefix + ".repro.proof_evidence",
			Message: fmt.Sprintf("repro.proof_evidence %q must be a concrete pointer", repro.ProofEvidence),
		})
	}
	if repro.ExitCode == nil || *repro.ExitCode == 0 {
		msg := "repro.exit_code is required and must be non-zero to prove the bug reproduces"
		if repro.ExitCode != nil {
			msg = fmt.Sprintf("repro.exit_code must be non-zero to prove failure reproduction, got %d", *repro.ExitCode)
		}
		res.Errors = append(res.Errors, Issue{
			Code:    "FIX_MISSING_REPRO_PROOF",
			Path:    prefix + ".repro.exit_code",
			Message: msg,
		})
	}
}
