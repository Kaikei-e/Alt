package loopeval

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"alt/scripts/altharness/internal/guards"
)

func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func writeln(w io.Writer, s string) {
	_, _ = fmt.Fprintln(w, s)
}

func writestr(w io.Writer, s string) {
	_, _ = fmt.Fprint(w, s)
}

// Run executes the LOOP-05 packet-template evaluation CLI. args excludes argv[0].
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		writeln(stderr, "error: nil context")
		return exitUsage
	}
	if err := ctx.Err(); err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if len(args) == 0 {
		writeln(stderr, "error: the following arguments are required: subcommand")
		writeHelp(stderr)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		writeHelp(stdout)
		return exitOK
	case "list":
		return cmdList(ctx, args[1:], stdout, stderr)
	case "validate":
		return cmdValidate(ctx, args[1:], stdout, stderr)
	case "stage":
		return cmdStage(ctx, args[1:], stdout, stderr)
	case "grade":
		return cmdGrade(ctx, args[1:], stdout, stderr)
	case "compare":
		return cmdCompare(ctx, args[1:], stdout, stderr)
	default:
		writef(stderr, "error: argument subcommand: invalid choice: %q (choose from list, validate, stage, grade, compare)\n", args[0])
		return exitUsage
	}
}

func writeHelp(w io.Writer) {
	writestr(w, `LOOP-05 packet-template evaluation suite grounded in CORPUS-01.

subcommands:
  list [--repo-root DIR] [--format text|json]
  validate [--repo-root DIR] [--min-scenarios 5] [--format text|json]
  stage <scenario-id> --out-dir DIR [--repo-root DIR] [--format text|json]
  grade --scenario <id> --workspace <dir> --handoff <path> [--packet <path>] [--transcript <jsonl>] [--base-commit <sha>] [--claimed-read <path>] [--no-rerun] [--require-transcript] [--repo-root DIR] [--format text|json]
  compare --before <path> --after <path> [--min-trials 3] [--template-version <label>] [--branch NAME] [--decisions-file PATH] [--runs-dir DIR] [--record] [--repo-root DIR] [--format text|json]
`)
}

type parsedFlags struct {
	positionals []string
	values      map[string]string
	multi       map[string][]string
	bools       map[string]bool
	help        bool
}

func parseFlags(args []string, valueFlags, multiFlags, boolFlags map[string]bool) (parsedFlags, error) {
	out := parsedFlags{
		values: map[string]string{},
		multi:  map[string][]string{},
		bools:  map[string]bool{},
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out.positionals = append(out.positionals, args[i+1:]...)
			break
		}
		if a == "-h" || a == "--help" {
			out.help = true
			return out, nil
		}
		if !strings.HasPrefix(a, "-") {
			out.positionals = append(out.positionals, a)
			continue
		}
		name := a
		val := ""
		hasEq := false
		if eq := strings.IndexByte(a, '='); eq >= 0 {
			name = a[:eq]
			val = a[eq+1:]
			hasEq = true
		}
		if boolFlags[name] {
			if hasEq {
				return out, fmt.Errorf("argument %s does not accept a value", name)
			}
			out.bools[name] = true
			continue
		}
		if valueFlags[name] || multiFlags[name] {
			if !hasEq {
				if i+1 >= len(args) {
					return out, fmt.Errorf("argument %s: expected one argument", name)
				}
				i++
				val = args[i]
			}
			if multiFlags[name] {
				out.multi[name] = append(out.multi[name], val)
			} else {
				out.values[name] = val
			}
			continue
		}
		return out, fmt.Errorf("unrecognized arguments: %s", a)
	}
	return out, nil
}

func takeFormat(p parsedFlags) (string, error) {
	format, ok := p.values["--format"]
	if !ok || format == "" {
		return "text", nil
	}
	if format != "text" && format != "json" {
		return "", fmt.Errorf("argument --format: invalid choice: %q (choose from text, json)", format)
	}
	return format, nil
}

func resolveRoot(override string) string {
	return guards.ResolveRepoRoot(override, "")
}

func cmdList(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	p, err := parseFlags(args, map[string]bool{"--repo-root": true, "--format": true}, nil, nil)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if p.help {
		writeHelp(stdout)
		return exitOK
	}
	if len(p.positionals) > 0 {
		writef(stderr, "error: unrecognized arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	format, err := takeFormat(p)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	repo := resolveRoot(p.values["--repo-root"])
	ids, _, err := discoverScenarioDirs(repo)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	scenarios := make([]Scenario, 0, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			writef(stderr, "error: %v\n", err)
			return exitFail
		}
		sc, err := loadScenario(repo, id)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return exitFail
		}
		scenarios = append(scenarios, sc)
	}
	if format == "json" {
		payload := map[string]any{
			"scenarios_dir": scenariosDir(repo),
			"count":         len(scenarios),
			"scenarios":     scenarios,
		}
		raw, err := marshalJSON(payload)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return exitUsage
		}
		_, _ = stdout.Write(raw)
		return exitOK
	}
	for _, sc := range scenarios {
		writef(stdout, "%s\t%s\t%s\t%s\t%s\t%s\n", sc.ID, sc.CorpusID, sc.Language, sc.NeutralWorkspaceName, sc.TargetFile, sc.Title)
	}
	return exitOK
}

func cmdValidate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	p, err := parseFlags(args, map[string]bool{
		"--repo-root":     true,
		"--min-scenarios": true,
		"--format":        true,
	}, nil, nil)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if p.help {
		writeHelp(stdout)
		return exitOK
	}
	if len(p.positionals) > 0 {
		writef(stderr, "error: unrecognized arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	format, err := takeFormat(p)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	minScenarios := defaultMinScenarios
	if rawMin, ok := p.values["--min-scenarios"]; ok {
		n, convErr := strconv.Atoi(rawMin)
		if convErr != nil || n <= 0 {
			writef(stderr, "error: --min-scenarios must be a positive integer (got %q)\n", rawMin)
			return exitUsage
		}
		minScenarios = n
	}
	repo := resolveRoot(p.values["--repo-root"])
	rep, err := validateAllScenarios(ctx, repo, minScenarios)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if format == "json" {
		raw, err := marshalJSON(rep)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return exitUsage
		}
		_, _ = stdout.Write(raw)
		if rep.Valid {
			return exitOK
		}
		return exitFail
	}
	if rep.Valid {
		writef(stdout, "OK: loop-eval scenarios valid (count=%d, min=%d)\n", rep.ScenarioCount, rep.MinScenarios)
		return exitOK
	}
	for _, issue := range rep.Errors {
		writef(stderr, "ERROR [%s] %s: %s\n", issue.Code, issue.Path, issue.Message)
	}
	writef(stderr, "INVALID: %d error(s) in loop-eval scenarios\n", len(rep.Errors))
	return exitFail
}

func cmdStage(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	p, err := parseFlags(args, map[string]bool{
		"--out-dir":   true,
		"--repo-root": true,
		"--format":    true,
	}, nil, nil)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if p.help {
		writeHelp(stdout)
		return exitOK
	}
	if len(p.positionals) != 1 {
		writeln(stderr, "error: stage requires exactly one <scenario-id>")
		return exitUsage
	}
	outDir, ok := p.values["--out-dir"]
	if !ok || strings.TrimSpace(outDir) == "" {
		writeln(stderr, "error: stage requires --out-dir DIR")
		return exitUsage
	}
	format, err := takeFormat(p)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	repo := resolveRoot(p.values["--repo-root"])
	res, err := stageScenario(ctx, repo, p.positionals[0], outDir)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if format == "json" {
		raw, err := marshalJSON(res)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return exitUsage
		}
		_, _ = stdout.Write(raw)
		return exitOK
	}
	writef(stdout, "staged %s -> %s (base_commit=%s, target_file=%s)\n", res.ScenarioID, res.WorkspaceDir, res.BaseCommit, res.TargetFile)
	return exitOK
}

func cmdGrade(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	p, err := parseFlags(
		args,
		map[string]bool{
			"--scenario":    true,
			"--workspace":   true,
			"--handoff":     true,
			"--packet":      true,
			"--transcript":  true,
			"--base-commit": true,
			"--repo-root":   true,
			"--format":      true,
		},
		map[string]bool{
			"--claimed-read": true,
		},
		map[string]bool{
			"--no-rerun":           true,
			"--require-transcript": true,
		},
	)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if p.help {
		writeHelp(stdout)
		return exitOK
	}
	if len(p.positionals) > 0 {
		writef(stderr, "error: unrecognized arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	format, err := takeFormat(p)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	scenarioID := strings.TrimSpace(p.values["--scenario"])
	wsDir := strings.TrimSpace(p.values["--workspace"])
	handoffPath := strings.TrimSpace(p.values["--handoff"])
	if scenarioID == "" || wsDir == "" || handoffPath == "" {
		writeln(stderr, "error: grade requires --scenario, --workspace, and --handoff")
		return exitUsage
	}
	repo := resolveRoot(p.values["--repo-root"])
	res, err := gradeWorkspace(ctx, gradeOptions{
		RepoRoot:          repo,
		ScenarioID:        scenarioID,
		Workspace:         wsDir,
		HandoffPath:       handoffPath,
		PacketPath:        strings.TrimSpace(p.values["--packet"]),
		TranscriptPath:    strings.TrimSpace(p.values["--transcript"]),
		BaseCommit:        strings.TrimSpace(p.values["--base-commit"]),
		NoRerun:           p.bools["--no-rerun"],
		RequireTranscript: p.bools["--require-transcript"],
		ClaimedReads:      p.multi["--claimed-read"],
	})
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if format == "json" {
		raw, err := marshalJSON(res)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return exitUsage
		}
		_, _ = stdout.Write(raw)
		if res.Passed {
			return exitOK
		}
		return exitFail
	}
	status := "FAIL"
	if res.Passed {
		status = "PASS"
	}
	writef(stdout, "%s: %s score=%d/%d (check01=%v char_test=%v loop01=%v transcript_evaluated=%v transcript_pass=%v)\n",
		status, res.ScenarioID, res.Score, res.MaxScore,
		res.Signals.Check01.Passed, res.Signals.CharTest.Passed, res.Signals.Loop01.Passed,
		res.Signals.TranscriptReadAudit.Evaluated, res.Signals.TranscriptReadAudit.Passed)
	if !res.Passed {
		for _, list := range [][]Issue{
			res.Signals.Check01.Errors,
			res.Signals.CharTest.Errors,
			res.Signals.Loop01.Errors,
			res.Signals.TranscriptReadAudit.Errors,
		} {
			for _, issue := range list {
				writef(stderr, "ERROR [%s] %s: %s\n", issue.Code, issue.Path, issue.Message)
			}
		}
		return exitFail
	}
	return exitOK
}

func cmdCompare(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := ctx.Err(); err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	p, err := parseFlags(
		args,
		map[string]bool{
			"--before":           true,
			"--after":            true,
			"--min-trials":       true,
			"--template-version": true,
			"--branch":           true,
			"--decisions-file":   true,
			"--runs-dir":         true,
			"--repo-root":        true,
			"--format":           true,
		},
		nil,
		map[string]bool{
			"--record": true,
		},
	)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if p.help {
		writeHelp(stdout)
		return exitOK
	}
	if len(p.positionals) > 0 {
		writef(stderr, "error: unrecognized arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	format, err := takeFormat(p)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	beforePath := strings.TrimSpace(p.values["--before"])
	afterPath := strings.TrimSpace(p.values["--after"])
	if beforePath == "" || afterPath == "" {
		writeln(stderr, "error: compare requires --before and --after")
		return exitUsage
	}
	minTrials := defaultMinTrials
	if rawMin, ok := p.values["--min-trials"]; ok {
		n, convErr := strconv.Atoi(rawMin)
		if convErr != nil || n <= 0 {
			writef(stderr, "error: --min-trials must be a positive integer (got %q)\n", rawMin)
			return exitUsage
		}
		minTrials = n
	}
	repo := resolveRoot(p.values["--repo-root"])
	rep, rc, err := compareGradeSets(compareOptions{
		RepoRoot:        repo,
		BeforePath:      beforePath,
		AfterPath:       afterPath,
		MinTrials:       minTrials,
		TemplateVersion: p.values["--template-version"],
		Branch:          p.values["--branch"],
		DecisionsFile:   p.values["--decisions-file"],
		RunsDir:         p.values["--runs-dir"],
		Record:          p.bools["--record"],
	})
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return rc
	}
	if format == "json" {
		raw, err := marshalJSON(rep)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return exitUsage
		}
		_, _ = stdout.Write(raw)
		return rc
	}
	if !rep.Valid {
		for _, issue := range rep.Errors {
			writef(stderr, "ERROR [%s] %s: %s\n", issue.Code, issue.Path, issue.Message)
		}
		return rc
	}
	writef(stdout, "verdict=%s scenarios=%d min_trials=%d pass_rate=%.4f->%.4f (%+.4f) mean_score=%.4f->%.4f (%+.4f)\n",
		rep.Verdict, len(rep.Scenarios), rep.MinTrials,
		rep.BeforePassRate, rep.AfterPassRate, rep.DeltaPassRate,
		rep.BeforeMeanScore, rep.AfterMeanScore, rep.DeltaMeanScore)
	for _, sc := range rep.Scenarios {
		writef(stdout, "  %s [%s] trials=%d/%d pass=%.4f->%.4f score=%.4f->%.4f\n",
			sc.ScenarioID, sc.Status, sc.BeforeTrials, sc.AfterTrials,
			sc.BeforePassRate, sc.AfterPassRate, sc.BeforeMeanScore, sc.AfterMeanScore)
	}
	if rep.RecordedArtifact != "" {
		writef(stdout, "recorded=%s decisions=%s\n", rep.RecordedArtifact, rep.DecisionsFile)
	}
	return rc
}
