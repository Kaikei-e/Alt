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
		if isSymlink(runsDir) {
			return "", "", fmt.Errorf("runs-dir must not be a symlink: %s", runsDir)
		}
		safeBranch := guards.SanitizeBranchName(guards.ResolveGitBranch(repoRoot, branch))
		runDir := filepath.Join(runsDir, safeBranch)
		if !isWithinDir(runDir, runsDir) {
			return "", "", fmt.Errorf("resolved run directory escapes runs-dir: %s", runDir)
		}
		decPath := decisionsFile
		if decPath == "" {
			decPath = filepath.Join(runDir, "decisions.tsv")
		}
		return runDir, decPath, nil
	}
	if decisionsFile != "" {
		return filepath.Dir(decisionsFile), decisionsFile, nil
	}
	runDir, err := guards.RunDirForBranch(repoRoot, branch)
	if err != nil {
		return "", "", err
	}
	return runDir, filepath.Join(runDir, "decisions.tsv"), nil
}

// RecordDisposition writes <runDir>/correct/<milestone_id>.{md,json} (checking
// non-symlink and path containment) and appends one row per class to decisions.tsv.
func RecordDisposition(repoRoot, runDir, decisionsPath string, disp *Disposition) (RecordReport, error) {
	if isSymlink(runDir) {
		return RecordReport{}, fmt.Errorf("run directory must not be a symlink: %s", runDir)
	}
	if err := os.MkdirAll(runDir, fsperm.Dir); err != nil {
		return RecordReport{}, fmt.Errorf("create run directory %s: %w", runDir, err)
	}
	runDirReal, err := filepath.EvalSymlinks(runDir)
	if err != nil {
		return RecordReport{}, fmt.Errorf("resolve run directory %s: %w", runDir, err)
	}

	correctDir := filepath.Join(runDir, "correct")
	if info, err := os.Lstat(correctDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return RecordReport{}, fmt.Errorf("correct directory must not be a symlink: %s", correctDir)
		}
		if !info.IsDir() {
			return RecordReport{}, fmt.Errorf("correct path exists and is not a directory: %s", correctDir)
		}
	} else if !os.IsNotExist(err) {
		return RecordReport{}, fmt.Errorf("stat correct directory %s: %w", correctDir, err)
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

	mdPath := filepath.Join(correctDir, disp.MilestoneID+".md")
	jsonPath := filepath.Join(correctDir, disp.MilestoneID+".json")
	for _, target := range []string{mdPath, jsonPath} {
		if !isWithinDir(filepath.Clean(target), filepath.Clean(correctDir)) {
			return RecordReport{}, fmt.Errorf("artifact path escapes correct directory: %s", target)
		}
		if info, err := os.Lstat(target); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return RecordReport{}, fmt.Errorf("artifact path must not be a symlink: %s", target)
			}
			if info.IsDir() {
				return RecordReport{}, fmt.Errorf("artifact path is a directory: %s", target)
			}
		} else if !os.IsNotExist(err) {
			return RecordReport{}, fmt.Errorf("stat artifact path %s: %w", target, err)
		}
	}

	if info, err := os.Lstat(decisionsPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return RecordReport{}, fmt.Errorf("decisions file must not be a symlink: %s", decisionsPath)
		}
		if info.IsDir() {
			return RecordReport{}, fmt.Errorf("decisions path is a directory: %s", decisionsPath)
		}
	}

	mdContent := RenderDispositionMarkdown(disp, time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	if err := os.WriteFile(mdPath, []byte(mdContent), fsperm.File); err != nil {
		return RecordReport{}, fmt.Errorf("write markdown artifact %s: %w", mdPath, err)
	}

	jsonBytes, err := marshalPrettyJSON(disp)
	if err != nil {
		return RecordReport{}, err
	}
	if err := os.WriteFile(jsonPath, jsonBytes, fsperm.File); err != nil {
		return RecordReport{}, fmt.Errorf("write json artifact %s: %w", jsonPath, err)
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
