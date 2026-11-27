package loopreview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"alt/scripts/altharness/internal/guards"
)

func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func writeln(w io.Writer, msg string) {
	_, _ = fmt.Fprintln(w, msg)
}

// Run executes the `alt-harness loop-review` CLI. args excludes argv[0].
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
		return exitFail
	}
	if len(args) == 0 {
		writeln(stderr, "error: subcommand is required (brief, validate, record, check-coverage)")
		writeHelp(stderr)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		writeHelp(stdout)
		return exitOK
	case "brief":
		return cmdBrief(ctx, args[1:], stdout, stderr)
	case "validate":
		return cmdValidate(ctx, args[1:], stdout, stderr)
	case "record":
		return cmdRecord(ctx, args[1:], stdout, stderr)
	case "check-coverage":
		return cmdCheckCoverage(ctx, args[1:], stdout, stderr)
	default:
		writef(stderr, "error: unknown subcommand %q (choose from brief, validate, record, check-coverage)\n", args[0])
		return exitUsage
	}
}

func writeHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `LOOP-04 adversarial review engine (brief, validate, record, check-coverage).

subcommands:
  brief [--base REV] [--focus-file PATH] [--out PATH] [--repo-root DIR] [--format json|markdown]
  validate <report.json> [--require-focus-all] [--repo-root DIR] [--format text|json]
  record <report.json> [--branch NAME] [--decisions-file PATH] [--runs-dir DIR] [--repo-root DIR] [--require-focus-all] [--format text|json]
  check-coverage [--decisions-file PATH] [--branch NAME] [--runs-dir DIR] [--repo-root DIR] [--strict-resolved] [--format text|json]
`)
}

type parsedFlags struct {
	positionals []string
	values      map[string]string
	bools       map[string]bool
	help        bool
}

func parseFlags(args []string, valueFlags, boolFlags map[string]bool) (parsedFlags, error) {
	out := parsedFlags{
		values: map[string]string{},
		bools:  map[string]bool{},
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-h" || a == "--help" {
			out.help = true
			return out, nil
		}
		if a == "--" {
			out.positionals = append(out.positionals, args[i+1:]...)
			break
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
		if valueFlags[name] {
			if !hasEq {
				if i+1 >= len(args) {
					return out, fmt.Errorf("argument %s: expected one argument", name)
				}
				i++
				val = args[i]
			}
			out.values[name] = val
			continue
		}
		return out, fmt.Errorf("unrecognized argument: %s", a)
	}
	return out, nil
}

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return buf.Bytes(), nil
}

func resolveRepoDir(override string) (string, error) {
	repo := guards.ResolveRepoRoot(override, "")
	if strings.TrimSpace(repo) == "" {
		return "", fmt.Errorf("could not determine repository root")
	}
	info, err := os.Stat(repo)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("repo root directory not found: %s", repo)
	}
	return repo, nil
}

func cmdBrief(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	p, err := parseFlags(args,
		map[string]bool{
			"--base":       true,
			"--focus-file": true,
			"--out":        true,
			"--repo-root":  true,
			"--format":     true,
		},
		nil,
	)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if p.help {
		writeHelp(stdout)
		return exitOK
	}
	if len(p.positionals) != 0 {
		writef(stderr, "error: unexpected positional arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	format := p.values["--format"]
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "markdown" {
		writef(stderr, "error: invalid --format %q (choose from json, markdown)\n", format)
		return exitUsage
	}
	repoRoot, err := resolveRepoDir(p.values["--repo-root"])
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	brief, err := BuildBrief(ctx, BriefOptions{
		RepoRoot:  repoRoot,
		BaseRev:   p.values["--base"],
		FocusFile: p.values["--focus-file"],
	})
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	var payload []byte
	if format == "markdown" {
		payload = []byte(RenderBriefMarkdown(brief))
	} else {
		payload, err = marshalJSON(brief)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return exitFail
		}
	}
	if outPath := strings.TrimSpace(p.values["--out"]); outPath != "" {
		if err := writeSafeFile(outPath, payload); err != nil {
			writef(stderr, "error: %v\n", err)
			return exitFail
		}
		return exitOK
	}
	if _, err := stdout.Write(payload); err != nil {
		writef(stderr, "error: %v\n", err)
		return exitFail
	}
	return exitOK
}

func cmdValidate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := ctx.Err(); err != nil {
		writef(stderr, "error: %v\n", err)
		return exitFail
	}
	p, err := parseFlags(args,
		map[string]bool{
			"--repo-root": true,
			"--format":    true,
		},
		map[string]bool{
			"--require-focus-all": true,
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
	if len(p.positionals) != 1 {
		writeln(stderr, "error: validate requires exactly one <report.json> path")
		return exitUsage
	}
	format := p.values["--format"]
	if format == "" {
		format = "text"
	}
	if format != "text" && format != "json" {
		writef(stderr, "error: invalid --format %q (choose from text, json)\n", format)
		return exitUsage
	}
	if v := strings.TrimSpace(p.values["--repo-root"]); v != "" {
		if _, err := resolveRepoDir(v); err != nil {
			writef(stderr, "error: %v\n", err)
			return exitUsage
		}
	}
	_, _, res, rc, err := LoadAndValidateReport(p.positionals[0], p.bools["--require-focus-all"])
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return rc
	}
	return emitValidationResult(stdout, stderr, res, format)
}

func emitValidationResult(stdout, stderr io.Writer, res ValidationResult, format string) int {
	if format == "json" {
		raw, err := marshalJSON(res)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return exitFail
		}
		_, _ = stdout.Write(raw)
		if res.Valid {
			return exitOK
		}
		return exitFail
	}
	if res.Valid {
		writef(stdout, "OK: loop-review validate %s (focus_verdicts=%d, findings=%d, author_family=%s, reviewer_families=%s)\n",
			res.ReviewID, res.FocusVerdictsCount, res.FindingsCount, res.AuthorFamily, strings.Join(res.ReviewerFamilies, ","))
		return exitOK
	}
	for _, iss := range res.Errors {
		writef(stderr, "ERROR [%s] %s: %s\n", iss.Code, iss.Path, iss.Message)
	}
	writef(stderr, "INVALID: %d error(s) in %s\n", len(res.Errors), res.Path)
	return exitFail
}

func cmdRecord(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := ctx.Err(); err != nil {
		writef(stderr, "error: %v\n", err)
		return exitFail
	}
	p, err := parseFlags(args,
		map[string]bool{
			"--branch":         true,
			"--decisions-file": true,
			"--runs-dir":       true,
			"--repo-root":      true,
			"--format":         true,
		},
		map[string]bool{
			"--require-focus-all": true,
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
	if len(p.positionals) != 1 {
		writeln(stderr, "error: record requires exactly one <report.json> path")
		return exitUsage
	}
	format := p.values["--format"]
	if format == "" {
		format = "text"
	}
	if format != "text" && format != "json" {
		writef(stderr, "error: invalid --format %q (choose from text, json)\n", format)
		return exitUsage
	}
	repoRoot, err := resolveRepoDir(p.values["--repo-root"])
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}

	report, rawJSON, valRes, rc, err := LoadAndValidateReport(p.positionals[0], p.bools["--require-focus-all"])
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return rc
	}
	if !valRes.Valid {
		return emitValidationResult(stdout, stderr, valRes, format)
	}

	recRes, err := RecordReport(report, rawJSON, RecordOptions{
		RepoRoot:      repoRoot,
		Branch:        p.values["--branch"],
		RunsDir:       p.values["--runs-dir"],
		DecisionsFile: p.values["--decisions-file"],
	})
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitFail
	}
	if format == "json" {
		raw, merr := marshalJSON(recRes)
		if merr != nil {
			writef(stderr, "error: %v\n", merr)
			return exitFail
		}
		_, _ = stdout.Write(raw)
		return exitOK
	}
	writef(stdout, "OK: recorded LOOP-04 review %s -> %s (decisions=%s, findings=%d, focus_verdicts=%d)\n",
		recRes.ReviewID, recRes.SavedReportPath, recRes.DecisionsFile, recRes.FindingsRecorded, recRes.FocusVerdictsWritten)
	return exitOK
}

func cmdCheckCoverage(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := ctx.Err(); err != nil {
		writef(stderr, "error: %v\n", err)
		return exitFail
	}
	p, err := parseFlags(args,
		map[string]bool{
			"--decisions-file": true,
			"--branch":         true,
			"--runs-dir":       true,
			"--repo-root":      true,
			"--format":         true,
		},
		map[string]bool{
			"--strict-resolved": true,
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
	if len(p.positionals) != 0 {
		writef(stderr, "error: unexpected positional arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	format := p.values["--format"]
	if format == "" {
		format = "text"
	}
	if format != "text" && format != "json" {
		writef(stderr, "error: invalid --format %q (choose from text, json)\n", format)
		return exitUsage
	}

	decisionsPath := strings.TrimSpace(p.values["--decisions-file"])
	if decisionsPath == "" {
		repoRoot, rerr := resolveRepoDir(p.values["--repo-root"])
		if rerr != nil {
			writef(stderr, "error: %v\n", rerr)
			return exitUsage
		}
		if runsDir := strings.TrimSpace(p.values["--runs-dir"]); runsDir != "" {
			absRuns, aerr := filepath.Abs(runsDir)
			if aerr != nil {
				writef(stderr, "error: resolve --runs-dir: %v\n", aerr)
				return exitUsage
			}
			safeBranch := guards.SanitizeBranchName(guards.ResolveGitBranch(repoRoot, p.values["--branch"]))
			decisionsPath = filepath.Join(absRuns, safeBranch, "decisions.tsv")
		} else {
			runDir, derr := guards.RunDirForBranch(repoRoot, p.values["--branch"])
			if derr != nil {
				writef(stderr, "error: %v\n", derr)
				return exitFail
			}
			decisionsPath = filepath.Join(runDir, "decisions.tsv")
		}
	}

	cov := CheckCoverageWithOptions(decisionsPath, CoverageOptions{
		StrictResolved: p.bools["--strict-resolved"],
	})
	if format == "json" {
		raw, merr := marshalJSON(cov)
		if merr != nil {
			writef(stderr, "error: %v\n", merr)
			return exitFail
		}
		_, _ = stdout.Write(raw)
		if cov.Covered {
			return exitOK
		}
		return exitFail
	}
	if cov.Covered {
		writef(stdout, "OK: LOOP-04 coverage %d/%d (%s) in %s\n",
			cov.CoveredCount, cov.TotalRequired, strings.Join(cov.CoveredIDs, ", "), cov.DecisionsFile)
		return exitOK
	}
	if cov.Error != "" {
		writef(stderr, "ERROR: %s\n", cov.Error)
	}
	writef(stderr, "MISSING LOOP-04 coverage (%d/%d): missing %s\n",
		cov.CoveredCount, cov.TotalRequired, strings.Join(cov.MissingIDs, ", "))
	for _, item := range cov.MissingItems {
		writef(stderr, "  - %s: %s\n", item.ID, item.Title)
	}
	return exitFail
}
