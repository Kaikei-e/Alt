package loopcorrect

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/guards"
)

func cmdRecord(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	_ = ctx
	p, err := parseFlags(args,
		map[string]bool{
			"--branch":         true,
			"--decisions-file": true,
			"--runs-dir":       true,
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
	format, err := parseFormatFlag(p)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if len(p.positionals) != 1 {
		writeln(stderr, "error: record requires exactly one <disposition.json> argument")
		return exitUsage
	}
	filePath := p.positionals[0]
	repoRoot := guards.ResolveRepoRoot(p.values["--repo-root"], "")

	// Step 1: Validate fail-closed BEFORE touching any output path or decisions.tsv.
	disp, valReport, exitCode, loadErr := loadAndValidateDisposition(filePath, repoRoot)
	if loadErr != "" {
		writeln(stderr, loadErr)
		return exitCode
	}
	if !valReport.Valid {
		emitValidationReport(stdout, stderr, format, valReport)
		return exitFail
	}

	// Step 2: Resolve runDir and decisionsPath safely.
	runDir, decisionsPath, err := resolveRecordTargets(
		repoRoot,
		strings.TrimSpace(p.values["--branch"]),
		strings.TrimSpace(p.values["--decisions-file"]),
		strings.TrimSpace(p.values["--runs-dir"]),
	)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitFail
	}

	recReport, err := RecordDisposition(repoRoot, runDir, decisionsPath, disp)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitFail
	}

	if format == "json" {
		raw, err := marshalPrettyJSON(recReport)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return exitFail
		}
		_, _ = stdout.Write(raw)
		return exitOK
	}
	writef(stdout, "recorded /correct milestone %s (%d class(es)):\n  markdown : %s\n  json     : %s\n  decisions: %s\n",
		recReport.MilestoneID, recReport.RowsAppended, recReport.MarkdownPath, recReport.JSONPath, recReport.DecisionsFile)
	return exitOK
}

func resolveRecordTargets(repoRoot, branch, decisionsFile, runsDir string) (string, string, error) {
	if runsDir != "" {
		if err := rejectSymlinkOrNonDir(runsDir, "runs-dir"); err != nil {
			return "", "", err
		}
		if isWithinDir(filepath.Clean(runsDir), filepath.Clean(repoRoot)) {
			if err := rejectSymlinkChainUnder(repoRoot, runsDir); err != nil {
				return "", "", err
			}
		}
		safeBranch := guards.SanitizeBranchName(guards.ResolveGitBranch(repoRoot, branch))
		runDir := filepath.Join(runsDir, safeBranch)
		if !isWithinDir(runDir, runsDir) {
			return "", "", fmt.Errorf("resolved run directory escapes runs-dir: %s", runDir)
		}
		if err := rejectSymlinkChainUnder(runsDir, runDir); err != nil {
			return "", "", err
		}
		decPath := decisionsFile
		if decPath == "" {
			decPath = filepath.Join(runDir, "decisions.tsv")
		}
		if err := validateRecordDecisionsTarget(repoRoot, runsDir, runDir, decPath); err != nil {
			return "", "", err
		}
		return runDir, decPath, nil
	}
	if decisionsFile != "" {
		runDir := filepath.Dir(decisionsFile)
		if err := validateRecordDecisionsTarget(repoRoot, "", runDir, decisionsFile); err != nil {
			return "", "", err
		}
		return runDir, decisionsFile, nil
	}
	runDir, err := guards.RunDirForBranch(repoRoot, branch)
	if err != nil {
		return "", "", err
	}
	if err := rejectSymlinkChainUnder(repoRoot, runDir); err != nil {
		return "", "", err
	}
	decPath := filepath.Join(runDir, "decisions.tsv")
	if err := validateRecordDecisionsTarget(repoRoot, "", runDir, decPath); err != nil {
		return "", "", err
	}
	return runDir, decPath, nil
}

func validateRecordDecisionsTarget(repoRoot, runsDir, runDir, decisionsPath string) error {
	parentDir := filepath.Dir(decisionsPath)
	if err := rejectSymlinkOrNonDir(parentDir, "decisions parent directory"); err != nil {
		return err
	}
	if err := rejectSymlinkOrDir(decisionsPath, "decisions file"); err != nil {
		return err
	}
	cleanDec := filepath.Clean(decisionsPath)
	if repoRoot != "" && isWithinDir(cleanDec, filepath.Clean(repoRoot)) {
		if err := rejectSymlinkChainUnder(repoRoot, cleanDec); err != nil {
			return err
		}
	}
	if runsDir != "" && isWithinDir(cleanDec, filepath.Clean(runsDir)) {
		if err := rejectSymlinkChainUnder(runsDir, cleanDec); err != nil {
			return err
		}
	}
	if runDir != "" && isWithinDir(cleanDec, filepath.Clean(runDir)) {
		if err := rejectSymlinkChainUnder(runDir, cleanDec); err != nil {
			return err
		}
	}
	return nil
}

// RecordDisposition writes <runDir>/correct/<milestone_id>.{md,json} (checking
// non-symlink and path containment via os.OpenRoot) and appends one row per
// class to decisions.tsv.
func RecordDisposition(repoRoot, runDir, decisionsPath string, disp *Disposition) (RecordReport, error) {
	// Pre-flight all symlink and containment checks BEFORE creating directories or files.
	if err := rejectSymlinkOrNonDir(runDir, "run directory"); err != nil {
		return RecordReport{}, err
	}
	cleanRunDir := filepath.Clean(runDir)
	cleanRepo := filepath.Clean(repoRoot)
	runUnderRepo := repoRoot != "" && isWithinDir(cleanRunDir, cleanRepo)
	if runUnderRepo {
		if err := rejectSymlinkChainUnder(repoRoot, cleanRunDir); err != nil {
			return RecordReport{}, err
		}
	}

	correctDir := filepath.Join(runDir, "correct")
	if err := rejectSymlinkOrNonDir(correctDir, "correct directory"); err != nil {
		return RecordReport{}, err
	}

	mdName := disp.MilestoneID + ".md"
	jsonName := disp.MilestoneID + ".json"
	mdPath := filepath.Join(correctDir, mdName)
	jsonPath := filepath.Join(correctDir, jsonName)
	for _, target := range []string{mdPath, jsonPath} {
		if !isWithinDir(filepath.Clean(target), filepath.Clean(correctDir)) {
			return RecordReport{}, fmt.Errorf("artifact path escapes correct directory: %s", target)
		}
		if err := rejectSymlinkOrDir(target, "artifact path"); err != nil {
			return RecordReport{}, err
		}
	}

	if err := validateRecordDecisionsTarget(repoRoot, "", runDir, decisionsPath); err != nil {
		return RecordReport{}, err
	}

	if err := os.MkdirAll(runDir, fsperm.Dir); err != nil {
		return RecordReport{}, fmt.Errorf("create run directory %s: %w", runDir, err)
	}
	runDirReal, err := filepath.EvalSymlinks(runDir)
	if err != nil {
		return RecordReport{}, fmt.Errorf("resolve run directory %s: %w", runDir, err)
	}
	if runUnderRepo {
		repoReal := resolveExistingPath(repoRoot)
		if !isWithinDir(runDirReal, repoReal) {
			return RecordReport{}, fmt.Errorf("run directory escapes repository root: %s", runDirReal)
		}
	}

	if err := os.MkdirAll(correctDir, fsperm.Dir); err != nil {
		return RecordReport{}, fmt.Errorf("create correct directory %s: %w", correctDir, err)
	}
	correctDirReal, err := filepath.EvalSymlinks(correctDir)
	if err != nil {
		return RecordReport{}, fmt.Errorf("resolve correct directory %s: %w", correctDir, err)
	}
	if !isWithinDir(correctDirReal, runDirReal) {
		return RecordReport{}, fmt.Errorf("correct directory escapes run directory: %s", correctDirReal)
	}

	decParent := filepath.Dir(decisionsPath)
	if err := os.MkdirAll(decParent, fsperm.Dir); err != nil {
		return RecordReport{}, fmt.Errorf("create decisions directory %s: %w", decParent, err)
	}
	if err := validateRecordDecisionsTarget(repoRoot, "", runDir, decisionsPath); err != nil {
		return RecordReport{}, err
	}
	decParentReal, err := filepath.EvalSymlinks(decParent)
	if err != nil {
		return RecordReport{}, fmt.Errorf("resolve decisions directory %s: %w", decParent, err)
	}
	if isWithinDir(filepath.Clean(decisionsPath), cleanRunDir) && !isWithinDir(decParentReal, runDirReal) {
		return RecordReport{}, fmt.Errorf("decisions directory escapes run directory: %s", decParentReal)
	}
	if repoRoot != "" && isWithinDir(filepath.Clean(decisionsPath), cleanRepo) && !isWithinDir(decParentReal, resolveExistingPath(repoRoot)) {
		return RecordReport{}, fmt.Errorf("decisions directory escapes repository root: %s", decParentReal)
	}

	correctRoot, err := os.OpenRoot(correctDir)
	if err != nil {
		return RecordReport{}, fmt.Errorf("open correct directory root %s: %w", correctDir, err)
	}
	defer func() { _ = correctRoot.Close() }()

	mdContent := RenderDispositionMarkdown(disp, time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	if err := writeFileInRoot(correctRoot, mdName, []byte(mdContent)); err != nil {
		return RecordReport{}, fmt.Errorf("write markdown artifact %s: %w", mdPath, err)
	}

	jsonBytes, err := marshalPrettyJSON(disp)
	if err != nil {
		return RecordReport{}, err
	}
	if err := writeFileInRoot(correctRoot, jsonName, jsonBytes); err != nil {
		return RecordReport{}, fmt.Errorf("write json artifact %s: %w", jsonPath, err)
	}

	if err := rejectSymlinkOrDir(decisionsPath, "decisions file"); err != nil {
		return RecordReport{}, err
	}

	evidenceRef := displayRelPath(repoRoot, mdPath)
	appended := 0
	for _, c := range disp.Classes {
		decision := sanitizeSingleLine(fmt.Sprintf("correct %s: rung=%s enforcement=%s", c.ClassID, c.ChosenRung, c.EnforcementRef))
		why := sanitizeSingleLine(fmt.Sprintf("occurrences=%d; rejected_higher=%s", c.Occurrences, formatRejectedHigherTSV(c.ChosenRung, c.HigherRungsRejected)))
		if err := guards.AppendDecisionToFile(decisionsPath, "correct", decision, why, evidenceRef, "info"); err != nil {
			return RecordReport{}, fmt.Errorf("append decision for %s: %w", c.ClassID, err)
		}
		appended++
	}

	return RecordReport{
		Recorded:      true,
		MilestoneID:   disp.MilestoneID,
		Service:       disp.Service,
		MarkdownPath:  displayRelPath(repoRoot, mdPath),
		JSONPath:      displayRelPath(repoRoot, jsonPath),
		DecisionsFile: displayRelPath(repoRoot, decisionsPath),
		EvidenceRef:   evidenceRef,
		RowsAppended:  appended,
	}, nil
}

func writeFileInRoot(root *os.Root, name string, data []byte) error {
	if info, err := root.Lstat(name); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact path must not be a symlink: %s", name)
		}
		if info.IsDir() {
			return fmt.Errorf("artifact path is a directory: %s", name)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat artifact path %s: %w", name, err)
	}
	f, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fsperm.File)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", name, err)
	}
	return nil
}

// RenderDispositionMarkdown renders the canonical LOOP-07 /correct Markdown table.
func RenderDispositionMarkdown(disp *Disposition, recordedAt string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# LOOP-07 `/correct` Disposition: %s\n\n", disp.MilestoneID)
	fmt.Fprintf(&b, "- **Service**: `%s`\n", sanitizeSingleLine(disp.Service))
	if recordedAt != "" {
		fmt.Fprintf(&b, "- **Recorded**: `%s`\n", recordedAt)
	}
	fmt.Fprintf(&b, "- **Classes**: %d\n\n", len(disp.Classes))
	b.WriteString("| クラス | 発生回数 | 証拠 | 選んだ段 | 上の段を使わなかった理由 | コーパス | 強制手段 |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, c := range disp.Classes {
		classCell := escapeMDCell(fmt.Sprintf("`%s`: %s", c.ClassID, c.Title))
		evParts := make([]string, 0, len(c.Evidence))
		for _, ev := range c.Evidence {
			evParts = append(evParts, "`"+strings.TrimSpace(ev)+"`")
		}
		evCell := escapeMDCell(strings.Join(evParts, ", "))
		rungCell := escapeMDCell("`" + c.ChosenRung + "`")
		rejectedCell := escapeMDCell(formatRejectedHigherMD(c.ChosenRung, c.HigherRungsRejected))
		corpusCell := escapeMDCell("`" + c.CorpusID + "`")
		enfCell := escapeMDCell("`" + c.EnforcementRef + "`")
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | %s | %s |\n",
			classCell, c.Occurrences, evCell, rungCell, rejectedCell, corpusCell, enfCell)
	}
	hasNotes := false
	for _, c := range disp.Classes {
		if strings.TrimSpace(c.Notes) != "" {
			if !hasNotes {
				b.WriteString("\n## Notes\n\n")
				hasNotes = true
			}
			fmt.Fprintf(&b, "- **`%s`**: %s\n", sanitizeSingleLine(c.ClassID), sanitizeSingleLine(c.Notes))
		}
	}
	return b.String()
}

func formatRejectedHigherTSV(chosenRung string, rejected map[string]string) string {
	chosenRank := rungRank[chosenRung]
	var parts []string
	for _, rung := range rungOrder {
		if rungRank[rung] >= chosenRank {
			break
		}
		if reason, ok := rejected[rung]; ok && strings.TrimSpace(reason) != "" {
			parts = append(parts, rung+"="+sanitizeSingleLine(reason))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " | ")
}

func formatRejectedHigherMD(chosenRung string, rejected map[string]string) string {
	chosenRank := rungRank[chosenRung]
	var parts []string
	for _, rung := range rungOrder {
		if rungRank[rung] >= chosenRank {
			break
		}
		if reason, ok := rejected[rung]; ok && strings.TrimSpace(reason) != "" {
			parts = append(parts, fmt.Sprintf("**%s**: %s", rung, sanitizeSingleLine(reason)))
		}
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, "; ")
}

func escapeMDCell(s string) string {
	clean := sanitizeSingleLine(s)
	return strings.ReplaceAll(clean, "|", `\|`)
}
