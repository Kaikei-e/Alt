package packet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const defaultTemplateBaseCommit = "1111111111111111111111111111111111111111"

var templateBaseCommitRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// Run executes the task-packet CLI. args excludes argv[0].
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if ctx == nil {
		fmt.Fprintln(stderr, "error: nil context")
		return exitUsage
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: the following arguments are required: subcommand")
		return exitUsage
	}
	if args[0] == "-h" || args[0] == "--help" {
		writeHelp(stdout)
		return exitOK
	}
	rest := args[1:]
	switch args[0] {
	case "validate-packet":
		return cmdValidatePacket(ctx, rest, stdout, stderr)
	case "validate-handoff":
		return cmdValidateHandoff(ctx, rest, stdout, stderr)
	case "list-generated-forbidden-paths":
		return cmdListGenerated(ctx, rest, stdout, stderr)
	case "render-template":
		return cmdRenderTemplate(rest, stdout, stderr)
	case "check-templates":
		return cmdCheckTemplates(rest, stdout, stderr)
	case "measure":
		return cmdMeasure(ctx, rest, stdout, stderr)
	case "verify-handoff":
		return cmdVerifyHandoff(ctx, rest, stdout, stderr)
	case "verify-sequence":
		return cmdVerifySequence(ctx, rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "error: argument subcommand: invalid choice: %q (choose from validate-packet, validate-handoff, list-generated-forbidden-paths, render-template, check-templates, measure, verify-handoff, verify-sequence)\n", args[0])
		return exitUsage
	}
}

func writeHelp(w io.Writer) {
	fmt.Fprint(w, `Validate and render Alt coding-agent Task Packets and Handoffs.

subcommands:
  validate-packet <path> [--format text|json]
  validate-handoff <handoff> [--packet PATH] [--screener-report PATH | --base-commit REV] [--repo-root DIR] [--format text|json]
  list-generated-forbidden-paths [--repo-root DIR] [--format text|json]
  render-template --kind packet|handoff [--packet-id ID] [--target TARGET] [--base-commit SHA] [--format markdown|json]
  check-templates [--repo-root DIR]
  measure <packet> [--turn-limit-s N] [--log-run] [--runs-dir DIR] [--repo-root DIR] [--format text|json]
  verify-handoff <handoff> --packet PATH [--base-commit REV] [--repo-root DIR] [--runs-dir DIR] [--no-rerun] [--format text|json]
  verify-sequence [--base-commit REV] --test-command CMD [--packet PATH] [--behavior-preserving | --bugfix] [--repo-root DIR] [--format text|json]
`)
}

type parsedArgs struct {
	positionals []string
	values      map[string]string
	bools       map[string]bool
}

func parseCLI(args []string, boolFlags map[string]bool) (parsedArgs, string) {
	out := parsedArgs{values: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out.positionals = append(out.positionals, args[i+1:]...)
			break
		}
		if a == "-h" || a == "--help" {
			return out, "help"
		}
		if !strings.HasPrefix(a, "-") {
			out.positionals = append(out.positionals, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		val := ""
		hasVal := false
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			val = name[eq+1:]
			name = name[:eq]
			hasVal = true
		}
		if boolFlags[name] {
			out.bools[name] = true
			continue
		}
		if !hasVal {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return out, "error: argument --" + name + ": expected one argument"
			}
			i++
			val = args[i]
		}
		out.values[name] = val
	}
	return out, ""
}

func takeFormat(p parsedArgs, choices ...string) (string, string) {
	format, ok := p.values["format"]
	if !ok {
		return "", ""
	}
	for _, c := range choices {
		if format == c {
			return format, ""
		}
	}
	return "", fmt.Sprintf("error: argument --format: invalid choice: %q (choose from %s)", format, strings.Join(choices, ", "))
}

func unknownFlags(p parsedArgs, allowed ...string) string {
	ok := map[string]struct{}{}
	for _, name := range allowed {
		ok[name] = struct{}{}
	}
	for name := range p.values {
		if _, found := ok[name]; !found {
			return "error: unrecognized arguments: --" + name
		}
	}
	for name := range p.bools {
		if _, found := ok[name]; !found {
			return "error: unrecognized arguments: --" + name
		}
	}
	return ""
}

func emitResult(stdout, stderr io.Writer, kind, target string, packetID any, issues []Issue, format string) int {
	valid := len(issues) == 0
	if format == "json" {
		errs := make([]any, 0, len(issues))
		for _, item := range issues {
			errs = append(errs, map[string]any{"code": item.Code, "path": item.Path, "message": item.Message})
		}
		payload := map[string]any{
			"valid": valid, "kind": kind, "path": target, "packet_id": packetID, "errors": errs,
		}
		raw, err := writeJSON(payload)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err.Error())
			return exitUsage
		}
		_, _ = stdout.Write(raw)
		if valid {
			return exitOK
		}
		return exitInvalid
	}
	if valid {
		fmt.Fprintf(stdout, "OK: %s %s (packet_id=%s)\n", kind, target, formatPacketID(packetID))
		return exitOK
	}
	for _, item := range issues {
		fmt.Fprintf(stderr, "ERROR [%s] %s: %s\n", item.Code, item.Path, item.Message)
	}
	fmt.Fprintf(stderr, "INVALID: %d error(s) in %s\n", len(issues), target)
	return exitInvalid
}

func reportErr(stderr io.Writer, err error) int {
	if err == nil {
		return exitOK
	}
	fmt.Fprintf(stderr, "error: %s\n", err.Error())
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return exitInvalid
	}
	return exitUsage
}

func loadFail(stdout, stderr io.Writer, kind, path, format string, err error) int {
	if isParseErr(err) {
		return emitResult(stdout, stderr, kind, path, nil, []Issue{{Code: "PARSE_ERROR", Path: "$", Message: err.Error()}}, format)
	}
	fmt.Fprintf(stderr, "error: unable to read file: %s\n", err.Error())
	return exitUsage
}

func cmdValidatePacket(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := ctx.Err(); err != nil {
		return reportErr(stderr, err)
	}
	p, perr := parseCLI(args, nil)
	if perr == "help" {
		writeHelp(stdout)
		return exitOK
	}
	if perr != "" {
		fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if msg := unknownFlags(p, "format"); msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	format, msg := takeFormat(p, "text", "json")
	if msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if format == "" {
		format = "text"
	}
	if len(p.positionals) != 1 {
		fmt.Fprintln(stderr, "error: the following arguments are required: path")
		return exitUsage
	}
	path := p.positionals[0]
	if !isFile(path) {
		fmt.Fprintf(stderr, "error: file not found: %s\n", path)
		return exitUsage
	}
	schema, err := loadSchema(schemaPath())
	if err != nil {
		fmt.Fprintf(stderr, "error: unable to read %s: %s\n", path, err.Error())
		return exitUsage
	}
	doc, err := loadDocument(path)
	if err != nil {
		return loadFail(stdout, stderr, "packet", path, format, err)
	}
	issues, err := validatePacketDoc(doc, schema)
	if err != nil {
		return emitResult(stdout, stderr, "packet", path, doc.Data["packet_id"], []Issue{{Code: "PARSE_ERROR", Path: "$", Message: err.Error()}}, format)
	}
	return emitResult(stdout, stderr, "packet", path, doc.Data["packet_id"], issues, format)
}

func cmdValidateHandoff(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	p, perr := parseCLI(args, nil)
	if perr == "help" {
		writeHelp(stdout)
		return exitOK
	}
	if perr != "" {
		fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if msg := unknownFlags(p, "packet", "screener-report", "base-commit", "repo-root", "format"); msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if _, a := p.values["screener-report"]; a {
		if _, b := p.values["base-commit"]; b {
			fmt.Fprintln(stderr, "error: argument --base-commit: not allowed with argument --screener-report")
			return exitUsage
		}
	}
	format, msg := takeFormat(p, "text", "json")
	if msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if format == "" {
		format = "text"
	}
	if len(p.positionals) != 1 {
		fmt.Fprintln(stderr, "error: the following arguments are required: handoff_path")
		return exitUsage
	}
	handoffPath := p.positionals[0]
	if !isFile(handoffPath) {
		fmt.Fprintf(stderr, "error: handoff file not found: %s\n", handoffPath)
		return exitUsage
	}
	packetPath := p.values["packet"]
	if packetPath != "" && !isFile(packetPath) {
		fmt.Fprintf(stderr, "error: packet file not found: %s\n", packetPath)
		return exitUsage
	}
	screenerPath := p.values["screener-report"]
	if screenerPath != "" && !isFile(screenerPath) {
		fmt.Fprintf(stderr, "error: screener report file not found: %s\n", screenerPath)
		return exitUsage
	}
	root := repoRoot()
	if v, ok := p.values["repo-root"]; ok {
		root = v
	}
	if !isDir(root) {
		fmt.Fprintf(stderr, "error: repo root directory not found: %s\n", root)
		return exitUsage
	}
	if base, ok := p.values["base-commit"]; ok && (strings.TrimSpace(base) == "" || strings.HasPrefix(base, "-")) {
		fmt.Fprintf(stderr, "error: invalid --base-commit revision: %q\n", base)
		return exitUsage
	}
	schema, err := loadSchema(schemaPath())
	if err != nil {
		fmt.Fprintf(stderr, "error: unable to read file: %s\n", err.Error())
		return exitUsage
	}
	handoffDoc, err := loadDocument(handoffPath)
	if err != nil {
		return loadFail(stdout, stderr, "handoff", handoffPath, format, err)
	}
	var packetDoc *Document
	if packetPath != "" {
		doc, err := loadDocument(packetPath)
		if err != nil {
			return loadFail(stdout, stderr, "handoff", handoffPath, format, err)
		}
		packetDoc = doc
	}
	var candidates []map[string]any
	if screenerPath != "" {
		b, err := os.ReadFile(screenerPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: unable to read file: %s\n", err.Error())
			return exitUsage
		}
		candidates, err = parseScreenerCandidates(string(b), screenerPath)
		if err != nil {
			return emitResult(stdout, stderr, "handoff", handoffPath, nil, []Issue{{Code: "PARSE_ERROR", Path: "$", Message: err.Error()}}, format)
		}
	} else if base, ok := p.values["base-commit"]; ok {
		candidates, err = runBehaviorScreener(ctx, root, base)
		if err != nil {
			return emitResult(stdout, stderr, "handoff", handoffPath, handoffDoc.Data["packet_id"], []Issue{{Code: "PARSE_ERROR", Path: "$", Message: err.Error()}}, format)
		}
	}
	issues, err := validateHandoffDoc(ctx, handoffDoc, schema, packetDoc, candidates, root)
	if err != nil {
		return reportErr(stderr, err)
	}
	if base, ok := p.values["base-commit"]; ok && base != "HEAD" {
		allow := false
		if packetDoc != nil {
			if scope, ok := asMap(packetDoc.Data["scope"]); ok {
				if b, ok := scope["allow_generated_code_edits"].(bool); ok && b {
					allow = true
				}
			}
		}
		if err := appendGenerated(ctx, handoffDoc.Data, root, allow, &issues, base); err != nil {
			return reportErr(stderr, err)
		}
	}
	return emitResult(stdout, stderr, "handoff", handoffPath, handoffDoc.Data["packet_id"], issues, format)
}

func cmdMeasure(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	p, perr := parseCLI(args, map[string]bool{"log-run": true})
	if perr == "help" {
		writeHelp(stdout)
		return exitOK
	}
	if perr != "" {
		fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if msg := unknownFlags(p, "repo-root", "format", "turn-limit-s", "log-run", "runs-dir"); msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	format, msg := takeFormat(p, "text", "json")
	if msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if format == "" {
		format = "text"
	}
	var turnLimit *int64
	var budgetOverride *int64
	if rawTL, ok := p.values["turn-limit-s"]; ok {
		tl, err := strconv.ParseInt(strings.TrimSpace(rawTL), 10, 64)
		if err != nil || tl <= 0 {
			_, _ = fmt.Fprintf(stderr, "error: invalid --turn-limit-s %q: must be a positive integer\n", rawTL)
			return exitUsage
		}
		b := budgetFromTurnLimit(tl)
		turnLimit = &tl
		budgetOverride = &b
	}
	if len(p.positionals) != 1 {
		fmt.Fprintln(stderr, "error: the following arguments are required: packet_path")
		return exitUsage
	}
	packetPath := p.positionals[0]
	if !isFile(packetPath) {
		fmt.Fprintf(stderr, "error: packet file not found: %s\n", packetPath)
		return exitUsage
	}
	root := repoRoot()
	if v, ok := p.values["repo-root"]; ok {
		root = resolveExisting(v)
	}
	if !isDir(root) {
		fmt.Fprintf(stderr, "error: repo root directory not found: %s\n", root)
		return exitUsage
	}
	doc, err := loadDocument(packetPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: unable to load packet %s: %s\n", packetPath, err.Error())
		return exitUsage
	}
	policy, ok := asMap(doc.Data["execution_policy"])
	verifyCmd, _ := asString(nil)
	if ok {
		verifyCmd, _ = asString(policy["verification_command"])
	}
	if !ok || strings.TrimSpace(verifyCmd) == "" {
		return emitResult(stdout, stderr, "measure", packetPath, doc.Data["packet_id"], []Issue{{
			Code: "MISSING_VERIFICATION_COMMAND", Path: "$.execution_policy.verification_command",
			Message: "packet missing non-empty execution_policy.verification_command",
		}}, format)
	}
	var pre []Issue
	forbid := false
	if b, ok := policy["forbid_repo_wide_test_build"].(bool); ok && b {
		forbid = true
	}
	checkVerificationCommand(verifyCmd, "$.execution_policy.verification_command", nil, forbid, &pre)
	if len(pre) > 0 {
		return emitResult(stdout, stderr, "measure", packetPath, doc.Data["packet_id"], pre, format)
	}
	budget := resolveCommandBudget(policy)
	if budgetOverride != nil {
		budget = *budgetOverride
	}
	code, duration, _, _, runErr := runBash(ctx, verifyCmd, root, timeDur(budget+measureTimeoutSlackS))
	if runErr != nil {
		fmt.Fprintf(stderr, "error: measure failed: %s\n", runErr.Error())
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			return exitInvalid
		}
		return exitUsage
	}
	head, err := currentHeadSHA(ctx, root)
	if err != nil {
		fmt.Fprintf(stderr, "error: measure failed: %s\n", err.Error())
		return exitUsage
	}
	measured := map[string]any{
		"duration_s":     duration,
		"exit_code":      int64(code),
		"at_commit":      head,
		"command_sha256": commandSHA256(verifyCmd),
	}
	if err := writeMeasuredToPacket(packetPath, doc, measured, budgetOverride); err != nil {
		fmt.Fprintf(stderr, "error: measure failed: %s\n", err.Error())
		return exitUsage
	}
	if p.bools["log-run"] {
		runs := filepath.Join(root, ".harness", "runs")
		if v, ok := p.values["runs-dir"]; ok {
			if isSymlink(v) {
				_, _ = fmt.Fprintf(stderr, "error: measure log-run failed: --runs-dir must not be a symlink: %s\n", v)
				return exitUsage
			}
			runs = v
		}
		if err := appendMeasureDecision(root, runs, packetPath, doc.Data["packet_id"], duration, budget, turnLimit, code); err != nil {
			_, _ = fmt.Fprintf(stderr, "error: measure log-run failed: %s\n", err.Error())
			return exitUsage
		}
	}
	var issues []Issue
	if duration > float64(budget) {
		issues = append(issues, Issue{
			Code: "VERIFICATION_BUDGET_EXCEEDED", Path: "$.execution_policy.measured.duration_s",
			Message: fmt.Sprintf("verification_command took %vs, exceeding max_command_budget_s=%ds", duration, budget),
		})
	}
	return emitMeasureResult(stdout, stderr, packetPath, doc.Data["packet_id"], issues, format, duration, code, budget, turnLimit)
}

func emitMeasureResult(stdout, stderr io.Writer, target string, packetID any, issues []Issue, format string, duration float64, exitCode int, budget int64, turnLimit *int64) int {
	valid := len(issues) == 0
	if format == "json" {
		errs := make([]any, 0, len(issues))
		for _, item := range issues {
			errs = append(errs, map[string]any{"code": item.Code, "path": item.Path, "message": item.Message})
		}
		payload := map[string]any{
			"valid":                valid,
			"kind":                 "measure",
			"path":                 target,
			"packet_id":            packetID,
			"errors":               errs,
			"duration_s":           duration,
			"exit_code":            exitCode,
			"max_command_budget_s": budget,
		}
		if turnLimit != nil {
			payload["turn_limit_s"] = *turnLimit
		}
		raw, err := writeJSON(payload)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %s\n", err.Error())
			return exitUsage
		}
		_, _ = stdout.Write(raw)
		if valid {
			return exitOK
		}
		return exitInvalid
	}
	if valid {
		if turnLimit != nil {
			_, _ = fmt.Fprintf(stdout, "OK: measure %s (packet_id=%s, turn_limit_s=%d, max_command_budget_s=%d)\n", target, formatPacketID(packetID), *turnLimit, budget)
		} else {
			_, _ = fmt.Fprintf(stdout, "OK: measure %s (packet_id=%s)\n", target, formatPacketID(packetID))
		}
		return exitOK
	}
	for _, item := range issues {
		_, _ = fmt.Fprintf(stderr, "ERROR [%s] %s: %s\n", item.Code, item.Path, item.Message)
	}
	_, _ = fmt.Fprintf(stderr, "INVALID: %d error(s) in %s\n", len(issues), target)
	return exitInvalid
}

func cmdVerifyHandoff(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	p, perr := parseCLI(args, map[string]bool{"no-rerun": true})
	if perr == "help" {
		writeHelp(stdout)
		return exitOK
	}
	if perr != "" {
		fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if msg := unknownFlags(p, "packet", "base-commit", "repo-root", "runs-dir", "no-rerun", "format"); msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	format, msg := takeFormat(p, "text", "json")
	if msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if format == "" {
		format = "text"
	}
	if len(p.positionals) != 1 {
		fmt.Fprintln(stderr, "error: the following arguments are required: handoff_path")
		return exitUsage
	}
	if _, ok := p.values["packet"]; !ok {
		fmt.Fprintln(stderr, "error: the following arguments are required: --packet")
		return exitUsage
	}
	handoffPath := p.positionals[0]
	packetPath := p.values["packet"]
	root := repoRoot()
	if v, ok := p.values["repo-root"]; ok {
		root = resolveExisting(v)
	}
	if !isFile(handoffPath) {
		fmt.Fprintf(stderr, "error: handoff file not found: %s\n", handoffPath)
		return exitUsage
	}
	if !isFile(packetPath) {
		fmt.Fprintf(stderr, "error: packet file not found: %s\n", packetPath)
		return exitUsage
	}
	if !isDir(root) {
		fmt.Fprintf(stderr, "error: repo root directory not found: %s\n", root)
		return exitUsage
	}
	if base, ok := p.values["base-commit"]; ok && (strings.TrimSpace(base) == "" || strings.HasPrefix(base, "-")) {
		fmt.Fprintf(stderr, "error: invalid --base-commit revision: %q\n", base)
		return exitUsage
	}
	schema, err := loadSchema(schemaPath())
	if err != nil {
		fmt.Fprintf(stderr, "error: unable to read file: %s\n", err.Error())
		return exitUsage
	}
	handoffDoc, err := loadDocument(handoffPath)
	if err != nil {
		return loadFail(stdout, stderr, "verify-handoff", handoffPath, format, err)
	}
	packetDoc, err := loadDocument(packetPath)
	if err != nil {
		return loadFail(stdout, stderr, "verify-handoff", handoffPath, format, err)
	}
	runs := filepath.Join(root, ".harness", "runs")
	if v, ok := p.values["runs-dir"]; ok {
		if isSymlink(v) {
			_, _ = fmt.Fprintf(stderr, "error: --runs-dir must not be a symlink: %s\n", v)
			return exitUsage
		}
		runs = resolveExisting(v)
	}
	preIssues, err := preValidateVerifyHandoff(handoffDoc, packetDoc, schema, root, runs, !p.bools["no-rerun"])
	if err != nil {
		return emitResult(stdout, stderr, "verify-handoff", handoffPath, handoffDoc.Data["packet_id"], []Issue{{Code: "VERIFY_HANDOFF_ERROR", Path: "$", Message: err.Error()}}, format)
	}
	if len(preIssues) > 0 {
		return emitResult(stdout, stderr, "verify-handoff", handoffPath, handoffDoc.Data["packet_id"], preIssues, format)
	}
	rawBase := p.values["base-commit"]
	if rawBase == "" {
		if s, ok := asString(packetDoc.Data["base_commit"]); ok {
			rawBase = s
		}
	}
	if strings.TrimSpace(rawBase) == "" {
		return emitResult(stdout, stderr, "verify-handoff", handoffPath, handoffDoc.Data["packet_id"], []Issue{{
			Code: "MISSING_BASE_COMMIT", Path: "$.base_commit",
			Message: "verify-handoff requires --base-commit or packet.base_commit",
		}}, format)
	}
	resolved, err := resolveGitCommit(ctx, root, strings.TrimSpace(rawBase))
	if err != nil {
		return reportErr(stderr, err)
	}
	if resolved == "" {
		return emitResult(stdout, stderr, "verify-handoff", handoffPath, handoffDoc.Data["packet_id"], []Issue{{
			Code: "INVALID_BASE_COMMIT", Path: "$.base_commit",
			Message: fmt.Sprintf("unable to resolve base_commit %q in %s", rawBase, root),
		}}, format)
	}
	screener, err := runBehaviorScreener(ctx, root, resolved)
	if err != nil {
		return emitResult(stdout, stderr, "verify-handoff", handoffPath, handoffDoc.Data["packet_id"], []Issue{{Code: "VERIFY_HANDOFF_ERROR", Path: "$", Message: err.Error()}}, format)
	}
	issues, err := validateHandoffDoc(ctx, handoffDoc, schema, packetDoc, screener, root)
	if err != nil {
		return emitResult(stdout, stderr, "verify-handoff", handoffPath, handoffDoc.Data["packet_id"], []Issue{{Code: "VERIFY_HANDOFF_ERROR", Path: "$", Message: err.Error()}}, format)
	}
	allow := false
	if scope, ok := asMap(packetDoc.Data["scope"]); ok {
		if b, ok := scope["allow_generated_code_edits"].(bool); ok && b {
			allow = true
		}
	}
	if err := appendGenerated(ctx, handoffDoc.Data, root, allow, &issues, resolved); err != nil {
		issues = append(issues, Issue{Code: "VERIFY_HANDOFF_ERROR", Path: "$", Message: err.Error()})
		return emitResult(stdout, stderr, "verify-handoff", handoffPath, handoffDoc.Data["packet_id"], issues, format)
	}
	actual, err := verifyHandoffActualFiles(ctx, handoffDoc.Data, packetDoc.Data, root, resolved, &issues)
	if err != nil {
		issues = append(issues, Issue{Code: "VERIFY_HANDOFF_ERROR", Path: "$", Message: err.Error()})
		return emitResult(stdout, stderr, "verify-handoff", handoffPath, handoffDoc.Data["packet_id"], issues, format)
	}
	if err := verifyHandoffAST(ctx, handoffDoc.Data, packetDoc.Data, root, resolved, actual, &issues); err != nil {
		issues = append(issues, Issue{Code: "VERIFY_HANDOFF_ERROR", Path: "$", Message: err.Error()})
		return emitResult(stdout, stderr, "verify-handoff", handoffPath, handoffDoc.Data["packet_id"], issues, format)
	}
	if !p.bools["no-rerun"] {
		if err := verifyHandoffHostRerun(ctx, handoffDoc.Data, packetDoc.Data, root, runs, resolved, &issues); err != nil {
			issues = append(issues, Issue{Code: "VERIFY_HANDOFF_ERROR", Path: "$", Message: err.Error()})
			return emitResult(stdout, stderr, "verify-handoff", handoffPath, handoffDoc.Data["packet_id"], issues, format)
		}
	}
	if err := verifyHandoffLintFix(ctx, packetDoc.Data, root, resolved, actual, screener, &issues); err != nil {
		issues = append(issues, Issue{Code: "VERIFY_HANDOFF_ERROR", Path: "$", Message: err.Error()})
	}
	return emitResult(stdout, stderr, "verify-handoff", handoffPath, handoffDoc.Data["packet_id"], issues, format)
}

func cmdVerifySequence(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	p, perr := parseCLI(args, map[string]bool{"behavior-preserving": true, "bugfix": true})
	if perr == "help" {
		writeHelp(stdout)
		return exitOK
	}
	if perr != "" {
		fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if msg := unknownFlags(p, "base-commit", "test-command", "packet", "behavior-preserving", "bugfix", "repo-root", "format"); msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if p.bools["behavior-preserving"] && p.bools["bugfix"] {
		fmt.Fprintln(stderr, "error: argument --bugfix: not allowed with argument --behavior-preserving")
		return exitUsage
	}
	format, msg := takeFormat(p, "text", "json")
	if msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if format == "" {
		format = "text"
	}
	if len(p.positionals) != 0 {
		fmt.Fprintf(stderr, "error: unrecognized arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	root := repoRoot()
	if v, ok := p.values["repo-root"]; ok {
		root = resolveExisting(v)
	}
	if !isDir(root) {
		fmt.Fprintf(stderr, "error: repo root directory not found: %s\n", root)
		return exitUsage
	}
	flagBase, hasFlagBase := p.values["base-commit"]
	if hasFlagBase && (strings.TrimSpace(flagBase) == "" || strings.HasPrefix(strings.TrimSpace(flagBase), "-")) {
		fmt.Fprintf(stderr, "error: invalid --base-commit revision: %q\n", flagBase)
		return exitUsage
	}
	testCmd, ok := p.values["test-command"]
	if !ok || strings.TrimSpace(testCmd) == "" {
		fmt.Fprintln(stderr, "error: --test-command must be non-empty")
		return exitUsage
	}
	packetPath := p.values["packet"]
	if packetPath != "" && !isFile(packetPath) {
		fmt.Fprintf(stderr, "error: packet file not found: %s\n", packetPath)
		return exitUsage
	}
	cfg, err := resolveSequenceConfig(flagBase, hasFlagBase, p.bools["bugfix"], p.bools["behavior-preserving"], packetPath)
	if err != nil {
		return loadFail(stdout, stderr, "verify-sequence", packetPath, format, err)
	}
	preserving := cfg.preserving
	packetID := cfg.packetID
	base := strings.TrimSpace(cfg.baseCommit)
	if base == "" {
		if packetPath != "" {
			return emitResult(stdout, stderr, "verify-sequence", root, packetID, []Issue{{
				Code: "MISSING_BASE_COMMIT", Path: "$.base_commit",
				Message: "verify-sequence requires --base-commit or packet.base_commit",
			}}, format)
		}
		fmt.Fprintf(stderr, "error: invalid --base-commit revision: %q\n", flagBase)
		return exitUsage
	}
	var cmdIssues []Issue
	checkVerificationCommand(testCmd, "$.test_command", nil, false, &cmdIssues)
	if len(cmdIssues) > 0 {
		return emitResult(stdout, stderr, "verify-sequence", root, packetID, cmdIssues, format)
	}
	if strings.HasPrefix(base, "-") {
		return emitResult(stdout, stderr, "verify-sequence", root, packetID, []Issue{{
			Code: "INVALID_BASE_COMMIT", Path: "$.base_commit",
			Message: fmt.Sprintf("unable to resolve base_commit %q in %s", base, root),
		}}, format)
	}
	baseSHA, err := resolveGitCommit(ctx, root, base)
	if err != nil {
		return reportErr(stderr, err)
	}
	headSHA, err := resolveGitCommit(ctx, root, "HEAD")
	if err != nil {
		return reportErr(stderr, err)
	}
	if baseSHA == "" || headSHA == "" {
		return emitResult(stdout, stderr, "verify-sequence", root, packetID, []Issue{{
			Code: "INVALID_BASE_COMMIT", Path: "$.base_commit",
			Message: fmt.Sprintf("unable to resolve --base-commit %q or HEAD in %s", base, root),
		}}, format)
	}
	commits, c1Files, issues, err := checkSequenceCommits(ctx, root, baseSHA)
	if err != nil {
		return emitResult(stdout, stderr, "verify-sequence", root, packetID, []Issue{{Code: "VERIFY_SEQUENCE_ERROR", Path: "$", Message: err.Error()}}, format)
	}
	if len(issues) == 0 {
		baseRC, snippet, err := runTestInDetachedWorktree(ctx, root, baseSHA, testCmd, commits[0], c1Files)
		if err != nil {
			return emitResult(stdout, stderr, "verify-sequence", root, packetID, []Issue{{Code: "VERIFY_SEQUENCE_ERROR", Path: "$", Message: err.Error()}}, format)
		}
		short := shortSHA(baseSHA)
		if preserving && baseRC != 0 {
			issues = append(issues, Issue{
				Code: "SEQUENCE_BASELINE_TEST_FAILED", Path: "base:" + short,
				Message: fmt.Sprintf("commit 1 test failed on base_commit %s (rc=%d): %s", short, baseRC, snippet),
			})
		} else if !preserving && baseRC == 0 {
			issues = append(issues, Issue{
				Code: "SEQUENCE_BUGFIX_NOT_RED", Path: "base:" + short,
				Message: fmt.Sprintf("bugfix commit 1 test unexpectedly passed (rc=0) on base_commit %s", short),
			})
		}
	}
	if len(issues) == 0 {
		finalRC, snippet, err := runTestInDetachedWorktree(ctx, root, headSHA, testCmd, "", nil)
		if err != nil {
			return emitResult(stdout, stderr, "verify-sequence", root, packetID, []Issue{{Code: "VERIFY_SEQUENCE_ERROR", Path: "$", Message: err.Error()}}, format)
		}
		if finalRC != 0 {
			short := shortSHA(headSHA)
			issues = append(issues, Issue{
				Code: "SEQUENCE_FINAL_TEST_FAILED", Path: "head:" + short,
				Message: fmt.Sprintf("test command failed on final HEAD %s (rc=%d): %s", short, finalRC, snippet),
			})
		}
	}
	return emitResult(stdout, stderr, "verify-sequence", root, packetID, issues, format)
}

func cmdListGenerated(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	p, perr := parseCLI(args, nil)
	if perr == "help" {
		writeHelp(stdout)
		return exitOK
	}
	if perr != "" {
		fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if msg := unknownFlags(p, "repo-root", "format"); msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	format, msg := takeFormat(p, "text", "json")
	if msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if format == "" {
		format = "text"
	}
	root := repoRoot()
	if v, ok := p.values["repo-root"]; ok {
		root = resolveExisting(v)
	}
	if !isDir(root) {
		fmt.Fprintf(stderr, "error: repo root directory not found: %s\n", root)
		return exitUsage
	}
	paths, err := listGeneratedForbiddenPaths(ctx, root)
	if err != nil {
		return reportErr(stderr, err)
	}
	if format == "json" {
		raw, err := writeJSON(paths)
		if err != nil {
			return reportErr(stderr, err)
		}
		stdout.Write(raw)
		return exitOK
	}
	for _, rel := range paths {
		fmt.Fprintln(stdout, rel)
	}
	return exitOK
}

func cmdRenderTemplate(args []string, stdout, stderr io.Writer) int {
	p, perr := parseCLI(args, nil)
	if perr == "help" {
		writeHelp(stdout)
		return exitOK
	}
	if perr != "" {
		fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if msg := unknownFlags(p, "kind", "packet-id", "target", "base-commit", "format"); msg != "" {
		_, _ = fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	format, msg := takeFormat(p, "markdown", "json")
	if msg != "" {
		_, _ = fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if format == "" {
		format = "markdown"
	}
	kind, ok := p.values["kind"]
	if !ok {
		_, _ = fmt.Fprintln(stderr, "error: the following arguments are required: --kind")
		return exitUsage
	}
	if kind != "packet" && kind != "handoff" {
		_, _ = fmt.Fprintf(stderr, "error: argument --kind: invalid choice: %q (choose from packet, handoff)\n", kind)
		return exitUsage
	}
	packetID := defaultPacketID
	if v, ok := p.values["packet-id"]; ok {
		packetID = v
	}
	if !packetIDRe.MatchString(packetID) {
		_, _ = fmt.Fprintf(stderr, "error: invalid --packet-id %q (must match %s)\n", packetID, packetIDRe.String())
		return exitUsage
	}
	target := ""
	if v, ok := p.values["target"]; ok {
		norm := normalizeTarget(v)
		if norm == "" || !validTarget(norm) {
			_, _ = fmt.Fprintf(stderr, "error: invalid --target %q\n", v)
			return exitUsage
		}
		target = norm
	}
	baseCommit := defaultTemplateBaseCommit
	if v, ok := p.values["base-commit"]; ok {
		trimmed := strings.TrimSpace(v)
		if !templateBaseCommitRe.MatchString(trimmed) {
			_, _ = fmt.Fprintf(stderr, "error: invalid --base-commit %q (must match %s)\n", v, templateBaseCommitRe.String())
			return exitUsage
		}
		baseCommit = trimmed
	}
	tmpl := packetTemplatePath()
	if kind == "handoff" {
		tmpl = handoffTemplatePath()
	}
	b, err := os.ReadFile(tmpl)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: template not found: %s\n", tmpl)
		return exitUsage
	}
	rendered := strings.ReplaceAll(string(b), defaultPacketID, packetID)
	if target != "" {
		rendered = strings.ReplaceAll(rendered, `target: "`+defaultTarget+`"`, `target: "`+target+`"`)
		rendered = strings.ReplaceAll(rendered, defaultTemplateVerifyCmd, "./scripts/lint/run.sh "+target)
	}
	if kind == "packet" {
		rendered = strings.ReplaceAll(rendered, `base_commit: "`+defaultTemplateBaseCommit+`"`, `base_commit: "`+baseCommit+`"`)
		lines := splitLines(rendered)
		fm, body, err := extractFrontmatter(lines)
		if err == nil && fm != nil {
			stripped := stripMeasuredFromFrontmatter(fm)
			rebuilt := append([]string{"---"}, stripped...)
			rebuilt = append(rebuilt, "---")
			rebuilt = append(rebuilt, body...)
			rendered = strings.Join(rebuilt, "\n") + "\n"
		}
	}
	if format == "json" {
		doc, err := parseDocumentText(rendered, false)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %s\n", err.Error())
			return exitUsage
		}
		if kind == "packet" {
			doc.Data["base_commit"] = baseCommit
			if policy, ok := asMap(doc.Data["execution_policy"]); ok {
				delete(policy, "measured")
			}
		}
		raw, err := writeJSON(doc.Data)
		if err != nil {
			return reportErr(stderr, err)
		}
		_, _ = stdout.Write(raw)
		return exitOK
	}
	_, _ = fmt.Fprint(stdout, rendered)
	return exitOK
}

func cmdCheckTemplates(args []string, stdout, stderr io.Writer) int {
	p, perr := parseCLI(args, nil)
	if perr == "help" {
		writeHelp(stdout)
		return exitOK
	}
	if perr != "" {
		fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if msg := unknownFlags(p, "repo-root"); msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitUsage
	}
	if len(p.positionals) != 0 {
		fmt.Fprintf(stderr, "error: unrecognized arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	root := repoRoot()
	if v, ok := p.values["repo-root"]; ok {
		root = resolveExisting(v)
		if !isDir(root) {
			fmt.Fprintf(stderr, "error: repo root directory not found: %s\n", root)
			return exitUsage
		}
	}
	harness := filepath.Join(root, "scripts", "agent-harness")
	schemaP := filepath.Join(harness, "task_packet.schema.json")
	pktTmplP := filepath.Join(harness, "templates", "task-packet.md.tmpl")
	hoTmplP := filepath.Join(harness, "templates", "task-handoff.md.tmpl")

	var issues []Issue
	if !isFile(schemaP) {
		issues = append(issues, Issue{Code: "MISSING_SCHEMA", Path: schemaP, Message: "schema file missing"})
	}
	if !isFile(pktTmplP) {
		issues = append(issues, Issue{Code: "MISSING_TEMPLATE", Path: pktTmplP, Message: "packet template missing"})
	}
	if !isFile(hoTmplP) {
		issues = append(issues, Issue{Code: "MISSING_TEMPLATE", Path: hoTmplP, Message: "handoff template missing"})
	}
	if len(issues) > 0 {
		for _, item := range issues {
			fmt.Fprintf(stderr, "ERROR [%s] %s: %s\n", item.Code, item.Path, item.Message)
		}
		return exitInvalid
	}
	schema, err := loadSchema(schemaP)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR [SELF_CHECK_ERROR] $: %s\n", err.Error())
		fmt.Fprintln(stderr, "INVALID: 1 template self-check error(s)")
		return exitInvalid
	}
	if s, _ := asString(schema["$schema"]); s != schemaDraft202012 {
		issues = append(issues, Issue{Code: "SCHEMA_DRAFT", Path: "$schema", Message: fmt.Sprintf("expected %s, got %v", schemaDraft202012, schema["$schema"])})
	}
	defs, _ := asMap(schema["$defs"])
	for _, name := range []string{"TaskPacket", "TaskHandoff"} {
		if defs == nil || defs[name] == nil {
			issues = append(issues, Issue{Code: "SCHEMA_DEFS", Path: "$defs." + name, Message: fmt.Sprintf("missing definition '%s'", name)})
		}
	}
	packetDoc, err := loadDocument(pktTmplP)
	if err != nil {
		issues = append(issues, Issue{Code: "SELF_CHECK_ERROR", Path: "$", Message: err.Error()})
	} else if handoffDoc, err := loadDocument(hoTmplP); err != nil {
		issues = append(issues, Issue{Code: "SELF_CHECK_ERROR", Path: "$", Message: err.Error()})
	} else {
		pktIssues, err := validatePacketDoc(packetDoc, schema)
		if err != nil {
			issues = append(issues, Issue{Code: "SELF_CHECK_ERROR", Path: "$", Message: err.Error()})
		} else {
			issues = append(issues, pktIssues...)
		}
		hoIssues, err := validateHandoffDoc(context.Background(), handoffDoc, schema, packetDoc, nil, "")
		if err != nil {
			issues = append(issues, Issue{Code: "SELF_CHECK_ERROR", Path: "$", Message: err.Error()})
		} else {
			issues = append(issues, hoIssues...)
		}
	}
	if len(issues) > 0 {
		for _, item := range issues {
			fmt.Fprintf(stderr, "ERROR [%s] %s: %s\n", item.Code, item.Path, item.Message)
		}
		fmt.Fprintf(stderr, "INVALID: %d template self-check error(s)\n", len(issues))
		return exitInvalid
	}
	fmt.Fprintf(stdout, "OK: schema and bundled templates validated (%s, %s)\n", filepath.Base(pktTmplP), filepath.Base(hoTmplP))
	return exitOK
}

// LoadDocument reads a packet or handoff document.
func LoadDocument(path string) (*Document, error) { return loadDocument(path) }

// HasBackgroundAmpersand reports a foreground-violating shell command.
func HasBackgroundAmpersand(command string) bool { return hasBackgroundAmpersand(command) }

// MatchForbiddenAutofix returns the GUARD-01 label when text contains a forbidden autofix.
func MatchForbiddenAutofix(text string) string { return matchForbiddenAutofix(text) }
