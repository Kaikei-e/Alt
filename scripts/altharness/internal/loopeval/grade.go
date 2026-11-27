package loopeval

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"alt/scripts/altharness/internal/execx"
	"alt/scripts/altharness/internal/packet"
	"alt/scripts/altharness/internal/screener"
)

type gradeOptions struct {
	RepoRoot          string
	ScenarioID        string
	Workspace         string
	HandoffPath       string
	PacketPath        string
	TranscriptPath    string
	BaseCommit        string
	NoRerun           bool
	RequireTranscript bool
	ClaimedReads      []string
}

type handoffSummary struct {
	ModifiedFiles      []string
	CandidateResponses []candidateResponse
}

type candidateResponse struct {
	CandidateID string
	File        string
	Function    string
	Kind        string
	Disposition string
	TestName    string
}

func gradeWorkspace(ctx context.Context, opt gradeOptions) (GradeResult, error) {
	sc, err := loadScenario(opt.RepoRoot, opt.ScenarioID)
	if err != nil {
		return GradeResult{}, err
	}
	wsAbs, err := resolvePath(opt.Workspace)
	if err != nil {
		return GradeResult{}, err
	}
	if st, err := os.Stat(wsAbs); err != nil || !st.IsDir() {
		return GradeResult{}, fmt.Errorf("workspace directory not found: %s", opt.Workspace)
	}
	if !isRegularNonSymlink(opt.HandoffPath) {
		return GradeResult{}, fmt.Errorf("handoff file not found: %s", opt.HandoffPath)
	}
	pktPath := opt.PacketPath
	if pktPath == "" {
		pktPath = filepath.Join(sc.Dir, "packet.md")
	}
	if !isRegularNonSymlink(pktPath) {
		return GradeResult{}, fmt.Errorf("packet file not found: %s", pktPath)
	}
	if opt.TranscriptPath != "" && !isRegularNonSymlink(opt.TranscriptPath) {
		return GradeResult{}, fmt.Errorf("transcript file not found: %s", opt.TranscriptPath)
	}

	baseCommit := strings.TrimSpace(opt.BaseCommit)
	if baseCommit == "" {
		resolved, err := resolveWorkspaceRootCommit(ctx, wsAbs)
		if err != nil {
			return GradeResult{}, fmt.Errorf("resolve workspace base commit: %w", err)
		}
		baseCommit = resolved
	} else if strings.HasPrefix(baseCommit, "-") {
		return GradeResult{}, fmt.Errorf("invalid --base-commit revision: %q", baseCommit)
	}

	hoSummary, err := parseHandoffMetadata(opt.HandoffPath)
	if err != nil {
		// Continue grading; LOOP-01 will report parse failures cleanly.
		hoSummary = handoffSummary{}
	}

	res := GradeResult{
		ScenarioID:           sc.ID,
		CorpusID:             sc.CorpusID,
		NeutralWorkspaceName: sc.NeutralWorkspaceName,
		Workspace:            wsAbs,
		BaseCommit:           baseCommit,
	}

	// Signal 1: CHECK-01 behavior-change screener.
	res.Signals.Check01 = runCheck01Signal(ctx, wsAbs, baseCommit, sc.Check01ExpectedSignal, hoSummary.CandidateResponses)

	// Signal 2: Deterministic offline characterization test + non-empty diff enforcement.
	res.Signals.CharTest = runCharTestSignal(ctx, wsAbs, sc)
	if sc.RequireNonEmptyDiff {
		changed, chErr := workspaceChangedFiles(ctx, wsAbs, baseCommit)
		if chErr != nil {
			res.Signals.CharTest.Passed = false
			res.Signals.CharTest.Errors = append(res.Signals.CharTest.Errors, Issue{
				Code:    "NOOP_CHECK_ERROR",
				Path:    wsAbs,
				Message: fmt.Sprintf("failed to inspect workspace diff against %s: %v", baseCommit, chErr),
			})
		} else if len(changed) == 0 {
			res.Signals.CharTest.Passed = false
			res.Signals.CharTest.Errors = append(res.Signals.CharTest.Errors, Issue{
				Code:    "NOOP_ZERO_EDITS",
				Path:    sc.TargetFile,
				Message: fmt.Sprintf("scenario %s requires task edits (require_non_empty_diff=true), but 0 files were modified relative to %s", sc.ID, baseCommit),
			})
		}
	}

	// Signal 3: LOOP-01 packet verify-handoff.
	res.Signals.Loop01 = runLoop01Signal(ctx, wsAbs, opt.HandoffPath, pktPath, baseCommit, opt.NoRerun)

	// Signal 4: Transcript read-claim audit.
	res.Signals.TranscriptReadAudit = runTranscriptAuditSignal(
		wsAbs,
		opt.TranscriptPath,
		opt.RequireTranscript,
		sc.RequiredReads,
		hoSummary.ModifiedFiles,
		opt.ClaimedReads,
	)

	// Compute deterministic score and pass/fail.
	score := 0
	maxScore := 3
	if res.Signals.Check01.Passed {
		score++
	}
	if res.Signals.CharTest.Passed {
		score++
	}
	if res.Signals.Loop01.Passed {
		score++
	}
	passed := res.Signals.Check01.Passed && res.Signals.CharTest.Passed && res.Signals.Loop01.Passed
	if res.Signals.TranscriptReadAudit.Evaluated || opt.RequireTranscript {
		maxScore = 4
		if res.Signals.TranscriptReadAudit.Passed {
			score++
		}
		passed = passed && res.Signals.TranscriptReadAudit.Passed
	}
	res.Score = score
	res.MaxScore = maxScore
	res.Passed = passed
	return res, nil
}

func resolveWorkspaceRootCommit(ctx context.Context, wsDir string) (string, error) {
	out, stderr, code, err := runGit(ctx, wsDir, "rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("git rev-list failed: %s", strings.TrimSpace(stderr))
	}
	lines := strings.Fields(out)
	if len(lines) == 0 {
		return "", errors.New("workspace has no commits")
	}
	return lines[len(lines)-1], nil
}

func runCheck01Signal(ctx context.Context, wsDir, baseCommit, expectedSignal string, responses []candidateResponse) Check01Signal {
	sig := Check01Signal{
		ExpectedSignal: expectedSignal,
		Candidates:     []ScreenerCandidate{},
		Errors:         []Issue{},
	}
	var outBuf, errBuf bytes.Buffer
	rc := screener.Run(ctx, []string{"--repo-root", wsDir, "--base", baseCommit}, &outBuf, &errBuf)
	if rc != 0 {
		sig.Errors = append(sig.Errors, Issue{
			Code:    "CHECK01_EXEC_ERROR",
			Path:    wsDir,
			Message: fmt.Sprintf("screener exited %d: %s", rc, strings.TrimSpace(errBuf.String())),
		})
		return sig
	}
	var cands []ScreenerCandidate
	if err := json.Unmarshal(outBuf.Bytes(), &cands); err != nil {
		sig.Errors = append(sig.Errors, Issue{
			Code:    "CHECK01_PARSE_ERROR",
			Path:    wsDir,
			Message: fmt.Sprintf("parse screener JSON: %v", err),
		})
		return sig
	}
	if cands == nil {
		cands = []ScreenerCandidate{}
	}
	sig.Candidates = cands
	sig.CandidatesCount = len(cands)

	for _, cand := range cands {
		if expectedSignal != "" && cand.Kind == expectedSignal {
			sig.ExpectedSignalTriggered = true
			sig.Errors = append(sig.Errors, Issue{
				Code:    "CHECK01_REGRESSION_DETECTED",
				Path:    cand.File,
				Message: fmt.Sprintf("expected regression signal %q triggered in %s:%s", expectedSignal, cand.File, cand.Function),
			})
			continue
		}
		if !candidateAddressed(cand, responses) {
			sig.Errors = append(sig.Errors, Issue{
				Code:    "CHECK01_UNADDRESSED_CANDIDATE",
				Path:    cand.File,
				Message: fmt.Sprintf("behavior candidate %s (%s:%s [%s]) is not addressed as intended with a test", cand.ID, cand.File, cand.Function, cand.Kind),
			})
		}
	}
	sig.Passed = len(sig.Errors) == 0
	return sig
}

func candidateAddressed(cand ScreenerCandidate, responses []candidateResponse) bool {
	for _, r := range responses {
		if r.Disposition != "intended" || strings.TrimSpace(r.TestName) == "" {
			continue
		}
		if r.CandidateID != "" && r.CandidateID == cand.ID {
			return true
		}
		if normalizeRelPath(r.File) == normalizeRelPath(cand.File) && r.Function == cand.Function && r.Kind == cand.Kind {
			return true
		}
	}
	return false
}

func runCharTestSignal(ctx context.Context, wsDir string, sc Scenario) CharTestSignal {
	sig := CharTestSignal{
		Command: sc.CharTestCommand,
		Errors:  []Issue{},
	}
	if !sc.HasCharTest {
		sig.Passed = true
		return sig
	}
	cmdStr := strings.TrimSpace(sc.CharTestCommand)
	if cmdStr == "" {
		sig.ExitCode = 2
		sig.Errors = append(sig.Errors, Issue{
			Code:    "EMPTY_CHAR_TEST_COMMAND",
			Path:    sc.ID,
			Message: "scenario has_char_test=true but char_test_command is empty",
		})
		return sig
	}
	if bad := matchForbiddenAutofix(cmdStr); bad != "" {
		sig.ExitCode = 2
		sig.Errors = append(sig.Errors, Issue{
			Code:    "FORBIDDEN_AUTOFIX_COMMAND",
			Path:    sc.ID,
			Message: fmt.Sprintf("char_test_command contains forbidden autofix (%s)", bad),
		})
		return sig
	}
	if hasBackgroundExecution(cmdStr) {
		sig.ExitCode = 2
		sig.Errors = append(sig.Errors, Issue{
			Code:    "FORBIDDEN_BACKGROUND_EXECUTION",
			Path:    sc.ID,
			Message: "char_test_command must run synchronously in foreground",
		})
		return sig
	}

	var (
		rc     int
		dur    float64
		outB   []byte
		errB   []byte
		runErr error
	)
	if usesCharTestScript(cmdStr) {
		scriptPath, ok := resolveTrustedCharTestScript(sc)
		if !ok {
			sig.ExitCode = 2
			sig.Errors = append(sig.Errors, Issue{
				Code:    "MISSING_CHAR_TEST_SCRIPT",
				Path:    sc.ID,
				Message: "trusted test_characterization.sh not found in scenario directory",
			})
			return sig
		}
		rc, dur, outB, errB, runErr = runScriptForeground(ctx, scriptPath, wsDir, time.Duration(defaultCharTimeoutS)*time.Second)
	} else {
		rc, dur, outB, errB, runErr = runBashForeground(ctx, cmdStr, wsDir, time.Duration(defaultCharTimeoutS)*time.Second)
	}
	sig.ExitCode = rc
	sig.DurationS = dur
	snippetSrc := string(errB)
	if strings.TrimSpace(snippetSrc) == "" {
		snippetSrc = string(outB)
	}
	sig.OutputSnippet = tailRunes(snippetSrc, outputSnippetLimit)
	if runErr != nil {
		sig.Errors = append(sig.Errors, Issue{
			Code:    "CHAR_TEST_EXEC_ERROR",
			Path:    sc.ID,
			Message: runErr.Error(),
		})
		return sig
	}
	if rc != 0 {
		sig.Errors = append(sig.Errors, Issue{
			Code:    "CHAR_TEST_FAILED",
			Path:    sc.ID,
			Message: fmt.Sprintf("characterization test exited %d: %s", rc, sig.OutputSnippet),
		})
		return sig
	}
	sig.Passed = true
	return sig
}

func usesCharTestScript(cmdStr string) bool {
	fields := strings.Fields(cmdStr)
	if len(fields) == 1 {
		return fields[0] == "test_characterization.sh" || fields[0] == "./test_characterization.sh"
	}
	if len(fields) == 2 && (fields[0] == "bash" || fields[0] == "sh") {
		return fields[1] == "test_characterization.sh" || fields[1] == "./test_characterization.sh"
	}
	return strings.Contains(cmdStr, "test_characterization.sh")
}

func resolveTrustedCharTestScript(sc Scenario) (string, bool) {
	if strings.TrimSpace(sc.Dir) == "" {
		return "", false
	}
	cand := filepath.Join(sc.Dir, "test_characterization.sh")
	if !isRegularNonSymlink(cand) || !withinDir(cand, sc.Dir) {
		return "", false
	}
	abs, err := resolvePath(cand)
	if err != nil {
		return "", false
	}
	return abs, true
}

func runScriptForeground(ctx context.Context, scriptPath, cwd string, timeout time.Duration) (int, float64, []byte, []byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	outStr, errStr, code, err := execx.Combined(runCtx, execx.Cmd{
		Args: []string{"bash", scriptPath},
		Dir:  cwd,
		Env:  []string{"PYTHONDONTWRITEBYTECODE=1", "WORKSPACE_DIR=" + cwd},
	})
	elapsed := float64(int64((float64(time.Since(start))/float64(time.Second))*1000+0.5)) / 1000
	if err != nil {
		if ctx.Err() != nil {
			return 1, elapsed, []byte(outStr), []byte(errStr), ctx.Err()
		}
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return timeoutExitCode, elapsed, []byte(outStr), []byte(errStr), nil
		}
		return code, elapsed, []byte(outStr), []byte(errStr), err
	}
	return code, elapsed, []byte(outStr), []byte(errStr), nil
}

func runBashForeground(ctx context.Context, command, cwd string, timeout time.Duration) (int, float64, []byte, []byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	outStr, errStr, code, err := execx.Combined(runCtx, execx.Cmd{
		Args: []string{"bash", "-c", command},
		Dir:  cwd,
		Env:  []string{"PYTHONDONTWRITEBYTECODE=1", "WORKSPACE_DIR=" + cwd},
	})
	elapsed := float64(int64((float64(time.Since(start))/float64(time.Second))*1000+0.5)) / 1000
	if err != nil {
		if ctx.Err() != nil {
			return 1, elapsed, []byte(outStr), []byte(errStr), ctx.Err()
		}
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return timeoutExitCode, elapsed, []byte(outStr), []byte(errStr), nil
		}
		return code, elapsed, []byte(outStr), []byte(errStr), err
	}
	return code, elapsed, []byte(outStr), []byte(errStr), nil
}

func workspaceChangedFiles(ctx context.Context, wsDir, baseCommit string) ([]string, error) {
	seen := map[string]struct{}{}
	diffOut, diffErr, code, err := runGit(ctx, wsDir, "diff", "--name-only", baseCommit, "--")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("git diff --name-only failed (rc=%d): %s", code, strings.TrimSpace(diffErr))
	}
	for _, line := range strings.Split(diffOut, "\n") {
		if rel := strings.TrimSpace(line); rel != "" {
			seen[rel] = struct{}{}
		}
	}
	untrackedOut, untrackedErr, code, err := runGit(ctx, wsDir, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("git ls-files failed (rc=%d): %s", code, strings.TrimSpace(untrackedErr))
	}
	for _, line := range strings.Split(untrackedOut, "\n") {
		if rel := strings.TrimSpace(line); rel != "" {
			seen[rel] = struct{}{}
		}
	}
	changed := make([]string, 0, len(seen))
	for rel := range seen {
		changed = append(changed, rel)
	}
	sort.Strings(changed)
	return changed, nil
}

func runLoop01Signal(ctx context.Context, wsDir, handoffPath, packetPath, baseCommit string, noRerun bool) Loop01Signal {
	sig := Loop01Signal{
		NoRerun: noRerun,
		Errors:  []Issue{},
	}
	tmpRuns, err := os.MkdirTemp("", "alt-loopeval-runs-")
	if err != nil {
		sig.ExitCode = 2
		sig.Errors = append(sig.Errors, Issue{
			Code:    "LOOP01_SETUP_ERROR",
			Path:    handoffPath,
			Message: err.Error(),
		})
		return sig
	}
	defer func() { _ = os.RemoveAll(tmpRuns) }()

	args := []string{
		"verify-handoff",
		handoffPath,
		"--packet", packetPath,
		"--base-commit", baseCommit,
		"--repo-root", wsDir,
		"--runs-dir", tmpRuns,
		"--format", "json",
	}
	if noRerun {
		args = append(args, "--no-rerun")
	}
	var outBuf, errBuf bytes.Buffer
	rc := packet.Run(ctx, args, &outBuf, &errBuf)
	sig.ExitCode = rc
	if rc == 0 {
		sig.Passed = true
		return sig
	}
	var pktRes struct {
		Errors []Issue `json:"errors"`
	}
	if json.Unmarshal(outBuf.Bytes(), &pktRes) == nil && len(pktRes.Errors) > 0 {
		sig.Errors = append(sig.Errors, pktRes.Errors...)
	} else {
		detail := strings.TrimSpace(errBuf.String())
		if detail == "" {
			detail = tailRunes(outBuf.String(), outputSnippetLimit)
		}
		sig.Errors = append(sig.Errors, Issue{
			Code:    "LOOP01_VERIFY_FAILED",
			Path:    handoffPath,
			Message: fmt.Sprintf("packet verify-handoff exited %d: %s", rc, detail),
		})
	}
	return sig
}

func isUnsafeClaimedPath(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return true
	}
	if strings.HasPrefix(trimmed, "/") || strings.Contains(trimmed, `\`) || filepath.IsAbs(trimmed) {
		return true
	}
	clean := strings.TrimPrefix(trimmed, "./")
	return unsafeRelPath(clean)
}

func runTranscriptAuditSignal(wsDir, transcriptPath string, requireTranscript bool, requiredReads, modifiedFiles, extraClaimed []string) TranscriptAuditSignal {
	sig := TranscriptAuditSignal{
		ExpectedReads:    []string{},
		ObservedReads:    []string{},
		UnverifiedClaims: []string{},
		Errors:           []Issue{},
	}
	expectedSet := map[string]struct{}{}
	unsafeSet := map[string]struct{}{}
	for _, group := range [][]string{requiredReads, modifiedFiles, extraClaimed} {
		for _, raw := range group {
			trimmed := strings.TrimSpace(raw)
			if isUnsafeClaimedPath(trimmed) {
				if trimmed == "" {
					trimmed = raw
				}
				unsafeSet[trimmed] = struct{}{}
				continue
			}
			norm := normalizeRelPath(strings.TrimPrefix(trimmed, "./"))
			expectedSet[norm] = struct{}{}
		}
	}
	for rel := range expectedSet {
		sig.ExpectedReads = append(sig.ExpectedReads, rel)
	}
	sort.Strings(sig.ExpectedReads)

	unsafeClaims := make([]string, 0, len(unsafeSet))
	for bad := range unsafeSet {
		unsafeClaims = append(unsafeClaims, bad)
	}
	sort.Strings(unsafeClaims)

	if strings.TrimSpace(transcriptPath) == "" {
		if requireTranscript {
			sig.Evaluated = true
			sig.Passed = false
			sig.Fidelity = 0.0
			sig.Errors = append(sig.Errors, Issue{
				Code:    "MISSING_TRANSCRIPT",
				Path:    "$",
				Message: "--require-transcript was set but no --transcript JSONL file was supplied",
			})
			for _, bad := range unsafeClaims {
				sig.UnverifiedClaims = append(sig.UnverifiedClaims, bad)
				sig.Errors = append(sig.Errors, Issue{
					Code:    "UNVERIFIED_FILE_READ_CLAIM",
					Path:    bad,
					Message: fmt.Sprintf("claimed read path %q is unsafe or escapes workspace", bad),
				})
			}
			return sig
		}
		if len(unsafeClaims) > 0 {
			sig.Evaluated = true
			sig.Passed = false
			sig.Fidelity = 0.0
			for _, bad := range unsafeClaims {
				sig.UnverifiedClaims = append(sig.UnverifiedClaims, bad)
				sig.Errors = append(sig.Errors, Issue{
					Code:    "UNVERIFIED_FILE_READ_CLAIM",
					Path:    bad,
					Message: fmt.Sprintf("claimed read path %q is unsafe or escapes workspace", bad),
				})
			}
			return sig
		}
		sig.Evaluated = false
		sig.Passed = false
		return sig
	}

	sig.Evaluated = true
	observedSet, parseErrs := parseTranscriptReadPaths(transcriptPath, wsDir)
	for rel := range observedSet {
		sig.ObservedReads = append(sig.ObservedReads, rel)
	}
	sort.Strings(sig.ObservedReads)
	sig.Errors = append(sig.Errors, parseErrs...)

	verified := 0
	for _, req := range sig.ExpectedReads {
		if _, ok := observedSet[req]; ok {
			verified++
		} else {
			sig.UnverifiedClaims = append(sig.UnverifiedClaims, req)
			sig.Errors = append(sig.Errors, Issue{
				Code:    "UNVERIFIED_FILE_READ_CLAIM",
				Path:    req,
				Message: fmt.Sprintf("file %q was required or modified without a matching Read tool invocation in transcript", req),
			})
		}
	}
	for _, bad := range unsafeClaims {
		sig.UnverifiedClaims = append(sig.UnverifiedClaims, bad)
		sig.Errors = append(sig.Errors, Issue{
			Code:    "UNVERIFIED_FILE_READ_CLAIM",
			Path:    bad,
			Message: fmt.Sprintf("claimed read path %q is unsafe or escapes workspace", bad),
		})
	}
	totalClaims := len(sig.ExpectedReads) + len(unsafeClaims)
	if totalClaims == 0 {
		sig.Fidelity = 1.0
	} else {
		sig.Fidelity = float64(verified) / float64(totalClaims)
	}
	sig.Passed = len(sig.Errors) == 0
	return sig
}

var readToolNames = map[string]struct{}{
	"read":      {},
	"read_file": {},
	"view_file": {},
	"file_read": {},
	"open_file": {},
}

var readPathKeys = map[string]struct{}{
	"file_path":     {},
	"filepath":      {},
	"path":          {},
	"absolute_path": {},
	"absolutepath":  {},
	"target_file":   {},
	"filename":      {},
}

var failedToolStatuses = map[string]struct{}{
	"error":     {},
	"failed":    {},
	"failure":   {},
	"denied":    {},
	"rejected":  {},
	"cancelled": {},
	"canceled":  {},
	"timeout":   {},
}

type recordedReadCall struct {
	callID string
	paths  []string
}

func parseTranscriptReadPaths(transcriptPath, wsDir string) (map[string]struct{}, []Issue) {
	observed := map[string]struct{}{}
	var issues []Issue

	raw, err := readRegularFile(transcriptPath)
	if err != nil {
		issues = append(issues, Issue{
			Code:    "TRANSCRIPT_READ_ERROR",
			Path:    transcriptPath,
			Message: err.Error(),
		})
		return observed, issues
	}

	absWS, _ := filepath.Abs(wsDir)
	resolvedWS, _ := resolvePath(wsDir)

	var calls []recordedReadCall
	failedCallIDs := map[string]struct{}{}

	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var evt any
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			issues = append(issues, Issue{
				Code:    "TRANSCRIPT_PARSE_ERROR",
				Path:    fmt.Sprintf("%s:%d", transcriptPath, lineNo),
				Message: fmt.Sprintf("invalid JSONL record: %v", err),
			})
			continue
		}
		obj, ok := evt.(map[string]any)
		if !ok {
			continue
		}
		collectFailedToolResultIDs(obj, failedCallIDs)
		collectEnvelopeReadCalls(obj, &calls)
	}
	if err := sc.Err(); err != nil {
		issues = append(issues, Issue{
			Code:    "TRANSCRIPT_READ_ERROR",
			Path:    transcriptPath,
			Message: err.Error(),
		})
	}

	for _, call := range calls {
		if call.callID != "" {
			if _, failed := failedCallIDs[call.callID]; failed {
				continue
			}
		}
		for _, p := range call.paths {
			if norm, ok := normalizeWorkspacePath(p, absWS, resolvedWS); ok {
				observed[norm] = struct{}{}
			}
		}
	}
	return observed, issues
}

func isUserOrSystemRole(m map[string]any) bool {
	role := strings.ToLower(strings.TrimSpace(strField(m, "role")))
	return role == "user" || role == "system"
}

func isToolInvocationFailed(m map[string]any) bool {
	if isErr, ok := m["is_error"].(bool); ok && isErr {
		return true
	}
	if errVal, exists := m["error"]; exists && errVal != nil {
		switch v := errVal.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return true
			}
		case bool:
			if v {
				return true
			}
		default:
			return true
		}
	}
	if status := strings.ToLower(strings.TrimSpace(strField(m, "status"))); status != "" {
		if _, bad := failedToolStatuses[status]; bad {
			return true
		}
	}
	if okVal, exists := m["ok"]; exists {
		if b, ok := okVal.(bool); ok && !b {
			return true
		}
	}
	if sucVal, exists := m["success"]; exists {
		if b, ok := sucVal.(bool); ok && !b {
			return true
		}
	}
	return false
}

func toolCallID(m map[string]any) string {
	for _, k := range []string{"id", "call_id", "tool_use_id"} {
		if id := strField(m, k); id != "" {
			return id
		}
	}
	return ""
}

func collectFailedToolResultIDs(evt map[string]any, failed map[string]struct{}) {
	t := strings.ToLower(strings.TrimSpace(strField(evt, "type")))
	if (t == "tool_result" || t == "function_call_output") && isToolInvocationFailed(evt) {
		if id := toolCallID(evt); id != "" {
			failed[id] = struct{}{}
		}
	}
	if t == "response_item" {
		if payload, ok := evt["payload"].(map[string]any); ok {
			pt := strings.ToLower(strings.TrimSpace(strField(payload, "type")))
			if (pt == "tool_result" || pt == "function_call_output") && isToolInvocationFailed(payload) {
				if id := toolCallID(payload); id != "" {
					failed[id] = struct{}{}
				}
			}
		}
	}
	if msg, ok := evt["message"].(map[string]any); ok {
		if content, ok := msg["content"].([]any); ok {
			for _, item := range content {
				obj, ok := item.(map[string]any)
				if !ok {
					continue
				}
				it := strings.ToLower(strings.TrimSpace(strField(obj, "type")))
				if (it == "tool_result" || it == "function_call_output") && isToolInvocationFailed(obj) {
					if id := toolCallID(obj); id != "" {
						failed[id] = struct{}{}
					}
				}
			}
		}
	}
}

func isValidEnvelopeToolType(t string) bool {
	return t == "tool_use" || t == "function_call" || t == "tool_call"
}

func appendReadToolObject(obj map[string]any, fallbackID string, out *[]recordedReadCall) {
	if isUserOrSystemRole(obj) || isToolInvocationFailed(obj) || !isReadToolObject(obj) {
		return
	}
	var paths []string
	extractPathsFromToolObject(obj, &paths)
	if len(paths) == 0 {
		return
	}
	id := toolCallID(obj)
	if id == "" {
		id = fallbackID
	}
	*out = append(*out, recordedReadCall{callID: id, paths: paths})
}

func collectEnvelopeReadCalls(evt map[string]any, out *[]recordedReadCall) {
	if isUserOrSystemRole(evt) || isToolInvocationFailed(evt) {
		return
	}
	outerType := strings.ToLower(strings.TrimSpace(strField(evt, "type")))
	if outerType == "user" || outerType == "system" || outerType == "tool_result" || outerType == "function_call_output" {
		return
	}

	// Shape A: Claude Code assistant message.content[] tool_use items.
	if msg, ok := evt["message"].(map[string]any); ok {
		if !isUserOrSystemRole(msg) && !isToolInvocationFailed(msg) {
			if content, ok := msg["content"].([]any); ok {
				for _, item := range content {
					obj, ok := item.(map[string]any)
					if !ok {
						continue
					}
					itemType := strings.ToLower(strings.TrimSpace(strField(obj, "type")))
					if isValidEnvelopeToolType(itemType) {
						appendReadToolObject(obj, "", out)
					}
				}
			}
			collectOpenAIToolCalls(msg["tool_calls"], out)
		}
	}

	// Shape B: Assistant/OpenAI tool_calls[] array.
	collectOpenAIToolCalls(evt["tool_calls"], out)

	// Shape C: Codex rollout response_item.payload function_call envelope.
	if outerType == "response_item" {
		if payload, ok := evt["payload"].(map[string]any); ok {
			pType := strings.ToLower(strings.TrimSpace(strField(payload, "type")))
			if isValidEnvelopeToolType(pType) {
				appendReadToolObject(payload, "", out)
			}
		}
		return
	}

	// Shape D: Explicit top-level tool invocation object.
	if outerType != "" && !isValidEnvelopeToolType(outerType) {
		return
	}
	if !isExplicitTopLevelToolCall(evt, outerType) {
		return
	}
	appendReadToolObject(evt, "", out)
}

func collectOpenAIToolCalls(raw any, out *[]recordedReadCall) {
	items, ok := raw.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok || isUserOrSystemRole(obj) || isToolInvocationFailed(obj) {
			continue
		}
		if fn, ok := obj["function"].(map[string]any); ok {
			appendReadToolObject(fn, toolCallID(obj), out)
			continue
		}
		if isReadToolObject(obj) {
			appendReadToolObject(obj, "", out)
		}
	}
}

func isExplicitTopLevelToolCall(m map[string]any, outerType string) bool {
	if isValidEnvelopeToolType(outerType) {
		return true
	}
	if _, hasTool := m["tool"]; hasTool {
		return true
	}
	if _, hasToolName := m["tool_name"]; hasToolName {
		return true
	}
	if _, hasName := m["name"]; hasName {
		for _, k := range []string{"input", "args", "arguments", "parameters"} {
			if _, ok := m[k]; ok {
				return true
			}
		}
	}
	return false
}

func isReadToolObject(m map[string]any) bool {
	for _, key := range []string{"name", "tool", "tool_name"} {
		if raw, ok := m[key].(string); ok {
			if _, isRead := readToolNames[strings.ToLower(strings.TrimSpace(raw))]; isRead {
				return true
			}
		}
	}
	return false
}

func extractPathsFromToolObject(m map[string]any, out *[]string) {
	// Check direct keys (e.g. {"tool_name":"Read","file_path":"..."}).
	extractPathFieldsFromMap(m, out)
	// Check parameter containers (input, args, arguments, parameters).
	for _, containerKey := range []string{"input", "args", "arguments", "parameters"} {
		val, ok := m[containerKey]
		if !ok {
			continue
		}
		switch c := val.(type) {
		case map[string]any:
			extractPathFieldsFromMap(c, out)
		case string:
			trimmed := strings.TrimSpace(c)
			if strings.HasPrefix(trimmed, "{") {
				var parsed map[string]any
				if json.Unmarshal([]byte(trimmed), &parsed) == nil {
					extractPathFieldsFromMap(parsed, out)
				}
			}
		}
	}
}

func extractPathFieldsFromMap(m map[string]any, out *[]string) {
	for k, v := range m {
		if _, match := readPathKeys[strings.ToLower(strings.TrimSpace(k))]; !match {
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			*out = append(*out, strings.TrimSpace(s))
		}
	}
}

func normalizeWorkspacePath(rawPath, absWS, resolvedWS string) (string, bool) {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return "", false
	}
	if filepath.IsAbs(p) {
		cleanP := filepath.Clean(p)
		for _, base := range []string{absWS, resolvedWS} {
			if base == "" {
				continue
			}
			if rel, err := filepath.Rel(base, cleanP); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return normalizeRelPath(filepath.ToSlash(rel)), true
			}
		}
		if resolvedP, err := resolvePath(cleanP); err == nil && resolvedWS != "" {
			if rel, err := filepath.Rel(resolvedWS, resolvedP); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return normalizeRelPath(filepath.ToSlash(rel)), true
			}
		}
		return "", false
	}
	if isUnsafeClaimedPath(p) {
		return "", false
	}
	norm := normalizeRelPath(strings.TrimPrefix(p, "./"))
	if norm == "" || unsafeRelPath(norm) {
		return "", false
	}
	return norm, true
}

// parseHandoffMetadata loads the TaskHandoff using packet.LoadDocument so
// JSON, YAML frontmatter, inline YAML arrays, and metadata-frontmatter +
// fenced ```json blocks are parsed identically to LOOP-01 verify-handoff.
func parseHandoffMetadata(path string) (handoffSummary, error) {
	doc, err := packet.LoadDocument(path)
	if err != nil {
		return handoffSummary{}, err
	}
	return summaryFromMap(doc.Data), nil
}

func summaryFromMap(m map[string]any) handoffSummary {
	var out handoffSummary
	if mf, ok := m["modified_files"].([]any); ok {
		for _, item := range mf {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if p, ok := obj["path"].(string); ok && strings.TrimSpace(p) != "" {
				out.ModifiedFiles = append(out.ModifiedFiles, strings.TrimSpace(p))
			}
		}
	}
	if impact, ok := m["runtime_behavior_impact"].(map[string]any); ok {
		if resps, ok := impact["candidate_responses"].([]any); ok {
			for _, item := range resps {
				obj, ok := item.(map[string]any)
				if !ok {
					continue
				}
				cr := candidateResponse{
					CandidateID: strField(obj, "candidate_id"),
					File:        strField(obj, "file"),
					Function:    strField(obj, "function"),
					Kind:        strField(obj, "kind"),
					Disposition: strField(obj, "disposition"),
					TestName:    strField(obj, "test_name"),
				}
				out.CandidateResponses = append(out.CandidateResponses, cr)
			}
		}
	}
	return out
}

func strField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}
