package loopreview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/guards"
)

var idRefEvidenceRE = regexp.MustCompile(`(?i)^(?:ADR-\d{6}|PM-\d{4}-\d{3}|C-\d{2,}|#\d+)$`)

// RecordOptions configures `loop-review record`.
type RecordOptions struct {
	RepoRoot      string
	Branch        string
	RunsDir       string
	DecisionsFile string
}

func sanitizeTSVCell(s string) string {
	r := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ")
	return strings.TrimSpace(r.Replace(s))
}

func normalizeEvidenceForTSV(raw string) string {
	cleaned := sanitizeTSVCell(raw)
	if guards.IsValidEvidencePointer(cleaned) {
		return cleaned
	}
	stripped := stripLineSuffix(cleaned)
	if guards.IsValidEvidencePointer(stripped) {
		if idRefEvidenceRE.MatchString(stripped) {
			return stripped
		}
		if !strings.Contains(cleaned, "/") && !strings.ContainsAny(cleaned, " \t") {
			candidate := "./" + cleaned
			if guards.IsValidEvidencePointer(candidate) {
				return candidate
			}
		}
		return stripped
	}
	return cleaned
}

func isWithinDir(path, root string) bool {
	p := filepath.Clean(path)
	r := filepath.Clean(root)
	if p == r {
		return true
	}
	rel, err := filepath.Rel(r, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolveRunAndDecisionsPaths(opts RecordOptions) (runDir, runsBase, decisionsPath string, err error) {
	if strings.TrimSpace(opts.RunsDir) != "" {
		absRuns, aerr := filepath.Abs(opts.RunsDir)
		if aerr != nil {
			return "", "", "", fmt.Errorf("resolve --runs-dir: %w", aerr)
		}
		if err := ensureNoSymlinkPath(absRuns); err != nil {
			return "", "", "", err
		}
		if st, serr := os.Lstat(absRuns); serr == nil && !st.IsDir() {
			return "", "", "", fmt.Errorf("runs directory path is not a directory: %s", absRuns)
		}
		safeBranch := guards.SanitizeBranchName(guards.ResolveGitBranch(opts.RepoRoot, opts.Branch))
		runDir = filepath.Join(absRuns, safeBranch)
		runsBase = absRuns
		if !isWithinDir(runDir, runsBase) {
			return "", "", "", fmt.Errorf("run directory escapes --runs-dir: %s", runDir)
		}
	} else {
		rd, rerr := guards.RunDirForBranch(opts.RepoRoot, opts.Branch)
		if rerr != nil {
			return "", "", "", rerr
		}
		runDir = rd
		runsBase = filepath.Join(opts.RepoRoot, ".harness", "runs")
		if err := checkNoSymlinkUnderBase(opts.RepoRoot, runsBase); err != nil {
			return "", "", "", err
		}
	}

	if err := checkNoSymlinkUnderBase(runsBase, runDir); err != nil {
		return "", "", "", err
	}

	if strings.TrimSpace(opts.DecisionsFile) != "" {
		absDec, aerr := filepath.Abs(opts.DecisionsFile)
		if aerr != nil {
			return "", "", "", fmt.Errorf("resolve --decisions-file: %w", aerr)
		}
		if err := ensureNoSymlinkPath(absDec); err != nil {
			return "", "", "", err
		}
		if st, serr := os.Lstat(absDec); serr == nil {
			if st.Mode()&os.ModeSymlink != 0 {
				return "", "", "", fmt.Errorf("refusing to overwrite symlink: %s", absDec)
			}
			if st.IsDir() {
				return "", "", "", fmt.Errorf("decisions file path is a directory: %s", absDec)
			}
		}
		decisionsPath = absDec
	} else {
		decisionsPath = filepath.Join(runDir, "decisions.tsv")
		if err := checkNoSymlinkUnderBase(runsBase, decisionsPath); err != nil {
			return "", "", "", err
		}
	}
	return runDir, runsBase, decisionsPath, nil
}

func ensureDecisionsInitialized(decisionsPath string) error {
	if err := ensureNoSymlinkPath(decisionsPath); err != nil {
		return err
	}
	st, err := os.Lstat(decisionsPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return guards.AppendDecisionToFile(
			decisionsPath,
			"init",
			"policy=fix-existing; goal=auto-initialized harness run; completion=./scripts/lint/run.sh <target> exit 0",
			"auto-initialized LOOP-03 decision log (override via harness-run init --force)",
			"docs/ADR/001038.md",
			"info",
		)
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to overwrite symlink: %s", decisionsPath)
	}
	if st.IsDir() {
		return fmt.Errorf("decisions file path is a directory: %s", decisionsPath)
	}
	if st.Size() == 0 {
		if err := writeFileInDirSafe(filepath.Dir(decisionsPath), filepath.Base(decisionsPath), []byte("ts\tphase\tdecision\twhy\tevidence\tresult\n")); err != nil {
			return err
		}
		return guards.AppendDecisionToFile(
			decisionsPath,
			"init",
			"policy=fix-existing; goal=auto-initialized harness run; completion=./scripts/lint/run.sh <target> exit 0",
			"auto-initialized LOOP-03 decision log (override via harness-run init --force)",
			"docs/ADR/001038.md",
			"info",
		)
	}
	return nil
}

func checkNoSymlinkUnderBase(base, target string) error {
	cleanBase := filepath.Clean(base)
	cleanTarget := filepath.Clean(target)
	if !isWithinDir(cleanTarget, cleanBase) {
		return fmt.Errorf("path %s escapes base %s", cleanTarget, cleanBase)
	}
	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return err
	}
	if isSymlink(cleanBase) {
		return fmt.Errorf("runs base must not be a symlink: %s", cleanBase)
	}
	if rel == "." {
		return nil
	}
	cur := cleanBase
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if seg == "" || seg == "." {
			continue
		}
		cur = filepath.Join(cur, seg)
		if isSymlink(cur) {
			return fmt.Errorf("refusing to traverse or overwrite symlink: %s", cur)
		}
	}
	if _, err := os.Lstat(cleanTarget); err == nil {
		resolvedBase, berr := filepath.EvalSymlinks(cleanBase)
		resolvedTarget, terr := filepath.EvalSymlinks(cleanTarget)
		if berr == nil && terr == nil && !isWithinDir(resolvedTarget, resolvedBase) {
			return fmt.Errorf("resolved path %s escapes base %s", resolvedTarget, resolvedBase)
		}
	}
	return nil
}

// RecordReport saves a validated LOOP-04 report under <runDir>/reviews/<review_id>.json
// and appends 1 row per finding and 1 row per focus verdict to decisions.tsv.
// Callers MUST validate report first; this function also defensively verifies review_id.
func RecordReport(report *Report, rawJSON []byte, opts RecordOptions) (RecordResult, error) {
	if report == nil {
		return RecordResult{}, errors.New("report is nil")
	}
	if !reviewIDRE.MatchString(report.ReviewID) {
		return RecordResult{}, fmt.Errorf("unsafe review_id %q", report.ReviewID)
	}

	runDir, runsBase, decisionsPath, err := resolveRunAndDecisionsPaths(opts)
	if err != nil {
		return RecordResult{}, err
	}

	reviewsDir := filepath.Join(runDir, "reviews")
	fileName := report.ReviewID + ".json"
	destPath := filepath.Join(reviewsDir, fileName)
	if err := checkNoSymlinkUnderBase(runsBase, reviewsDir); err != nil {
		return RecordResult{}, err
	}
	if err := checkNoSymlinkUnderBase(runsBase, destPath); err != nil {
		return RecordResult{}, err
	}
	if st, serr := os.Lstat(reviewsDir); serr == nil && !st.IsDir() {
		return RecordResult{}, fmt.Errorf("reviews path is not a directory: %s", reviewsDir)
	}
	if st, serr := os.Lstat(destPath); serr == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			return RecordResult{}, fmt.Errorf("refusing to overwrite symlink: %s", destPath)
		}
		if st.IsDir() {
			return RecordResult{}, fmt.Errorf("review report destination is a directory: %s", destPath)
		}
	}

	if err := os.MkdirAll(reviewsDir, fsperm.Dir); err != nil {
		return RecordResult{}, fmt.Errorf("create reviews dir %s: %w", reviewsDir, err)
	}
	// Re-check symlink & resolved containment after directory creation.
	if err := checkNoSymlinkUnderBase(runsBase, reviewsDir); err != nil {
		return RecordResult{}, err
	}
	resolvedBase, err := filepath.EvalSymlinks(runsBase)
	if err != nil {
		return RecordResult{}, fmt.Errorf("resolve runs base %s: %w", runsBase, err)
	}
	resolvedRunDir, err := filepath.EvalSymlinks(runDir)
	if err != nil {
		return RecordResult{}, fmt.Errorf("resolve run dir %s: %w", runDir, err)
	}
	resolvedReviewsDir, err := filepath.EvalSymlinks(reviewsDir)
	if err != nil {
		return RecordResult{}, fmt.Errorf("resolve reviews dir %s: %w", reviewsDir, err)
	}
	if !isWithinDir(resolvedRunDir, resolvedBase) || !isWithinDir(resolvedReviewsDir, resolvedRunDir) {
		return RecordResult{}, fmt.Errorf("reviews directory %s escapes %s", resolvedReviewsDir, resolvedRunDir)
	}
	if err := writeFileInDirSafe(reviewsDir, fileName, rawJSON); err != nil {
		return RecordResult{}, fmt.Errorf("write report copy %s: %w", destPath, err)
	}

	if err := ensureDecisionsInitialized(decisionsPath); err != nil {
		return RecordResult{}, fmt.Errorf("initialize decisions.tsv: %w", err)
	}

	canonMap := canonicalFocusMap()
	findingsRecorded := 0
	for _, f := range report.Findings {
		decision := fmt.Sprintf("[%s/%s] %s: %s",
			sanitizeTSVCell(f.FocusID),
			sanitizeTSVCell(f.Bucket),
			sanitizeTSVCell(f.ID),
			sanitizeTSVCell(f.Summary),
		)
		why := fmt.Sprintf("%s (reviewer=%s)",
			sanitizeTSVCell(f.Rationale),
			sanitizeTSVCell(f.ReviewerModel),
		)
		evidence := normalizeEvidenceForTSV(f.Evidence)
		if err := guards.AppendDecisionToFile(decisionsPath, "interrogate", decision, why, evidence, "info"); err != nil {
			return RecordResult{}, fmt.Errorf("append finding %s to decisions.tsv: %w", f.ID, err)
		}
		findingsRecorded++
	}

	verdictsRecorded := 0
	for _, fv := range report.FocusVerdicts {
		fid := sanitizeTSVCell(fv.FocusID)
		title := sanitizeTSVCell(fv.Title)
		if title == "" {
			if canon, ok := canonMap[fid]; ok {
				title = canon.Title
			} else {
				title = fid
			}
		}
		decision := fmt.Sprintf("focus %s (%s): %s", fid, sanitizeTSVCell(fv.Status), title)
		why := sanitizeTSVCell(fv.Summary)
		evidence := normalizeEvidenceForTSV(fv.Evidence)
		if err := guards.AppendDecisionToFile(decisionsPath, "loop-04", decision, why, evidence, "info"); err != nil {
			return RecordResult{}, fmt.Errorf("append focus verdict %s to decisions.tsv: %w", fid, err)
		}
		verdictsRecorded++
	}

	return RecordResult{
		Recorded:             true,
		ReviewID:             report.ReviewID,
		SavedReportPath:      destPath,
		DecisionsFile:        decisionsPath,
		FindingsRecorded:     findingsRecorded,
		FocusVerdictsWritten: verdictsRecorded,
	}, nil
}
