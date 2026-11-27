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

	// Signal 2: Deterministic offline characterization test.
	res.Signals.CharTest = runCharTestSignal(ctx, wsAbs, sc)

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
	rc, dur, outB, errB, runErr := runBashForeground(ctx, cmdStr, wsDir, time.Duration(defaultCharTimeoutS)*time.Second)
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

func runBashForeground(ctx context.Context, command, cwd string, timeout time.Duration) (int, float64, []byte, []byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	outStr, errStr, code, err := execx.Combined(runCtx, execx.Cmd{
		Args: []string{"bash", "-c", command},
		Dir:  cwd,
		Env:  []string{"PYTHONDONTWRITEBYTECODE=1"},
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

func runTranscriptAuditSignal(wsDir, transcriptPath string, requireTranscript bool, requiredReads, modifiedFiles, extraClaimed []string) TranscriptAuditSignal {
	sig := TranscriptAuditSignal{
		ExpectedReads:    []string{},
		ObservedReads:    []string{},
		UnverifiedClaims: []string{},
		Errors:           []Issue{},
	}
	expectedSet := map[string]struct{}{}
	for _, group := range [][]string{requiredReads, modifiedFiles, extraClaimed} {
		for _, raw := range group {
			norm := normalizeRelPath(raw)
			if norm != "" && !unsafeRelPath(norm) {
				expectedSet[norm] = struct{}{}
			}
		}
	}
	for rel := range expectedSet {
		sig.ExpectedReads = append(sig.ExpectedReads, rel)
	}
	sort.Strings(sig.ExpectedReads)

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
	if len(sig.ExpectedReads) == 0 {
		sig.Fidelity = 1.0
	} else {
		sig.Fidelity = float64(verified) / float64(len(sig.ExpectedReads))
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
		var rawPaths []string
		collectReadToolPaths(evt, &rawPaths)
		for _, p := range rawPaths {
			if norm, ok := normalizeWorkspacePath(p, absWS, resolvedWS); ok {
				observed[norm] = struct{}{}
			}
		}
	}
	if err := sc.Err(); err != nil {
		issues = append(issues, Issue{
			Code:    "TRANSCRIPT_READ_ERROR",
			Path:    transcriptPath,
			Message: err.Error(),
		})
	}
	return observed, issues
}

func collectReadToolPaths(node any, out *[]string) {
	switch t := node.(type) {
	case map[string]any:
		if isReadToolObject(t) {
			extractPathsFromToolObject(t, out)
		}
		for _, v := range t {
			collectReadToolPaths(v, out)
		}
	case []any:
		for _, elem := range t {
			collectReadToolPaths(elem, out)
		}
	}
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
	norm := normalizeRelPath(p)
	if norm == "" || unsafeRelPath(norm) {
		return "", false
	}
	return norm, true
}

// parseHandoffMetadata extracts modified_files paths and candidate_responses
// from either a JSON handoff or a Markdown handoff with YAML frontmatter / fenced JSON.
func parseHandoffMetadata(path string) (handoffSummary, error) {
	raw, err := readRegularFile(path)
	if err != nil {
		return handoffSummary{}, err
	}
	text := strings.TrimPrefix(string(raw), "\uFEFF")
	trimmed := strings.TrimSpace(text)

	// Case 1: Pure JSON handoff.
	if strings.HasPrefix(trimmed, "{") {
		var m map[string]any
		if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
			return handoffSummary{}, err
		}
		return summaryFromMap(m), nil
	}

	// Case 2: Markdown with YAML frontmatter.
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		end := -1
		for i := 1; i < len(lines); i++ {
			s := strings.TrimSpace(lines[i])
			if s == "---" || s == "..." {
				end = i
				break
			}
		}
		if end > 1 {
			return parseHandoffYAMLFrontmatter(lines[1:end]), nil
		}
	}

	// Case 3: Markdown with fenced ```json block.
	inJSON := false
	var block []string
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if !inJSON && strings.HasPrefix(strings.ToLower(s), "```json") {
			inJSON = true
			continue
		}
		if inJSON && strings.HasPrefix(s, "```") {
			var m map[string]any
			if err := json.Unmarshal([]byte(strings.Join(block, "\n")), &m); err == nil {
				return summaryFromMap(m), nil
			}
			break
		}
		if inJSON {
			block = append(block, line)
		}
	}
	return handoffSummary{}, nil
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
				out.ModifiedFiles = append(out.ModifiedFiles, normalizeRelPath(p))
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

func parseHandoffYAMLFrontmatter(lines []string) handoffSummary {
	var out handoffSummary
	section := ""
	sectionIndent := -1
	var curResp *candidateResponse

	flushResp := func() {
		if curResp != nil {
			out.CandidateResponses = append(out.CandidateResponses, *curResp)
			curResp = nil
		}
	}

	for _, rawLine := range lines {
		if strings.TrimSpace(rawLine) == "" || strings.HasPrefix(strings.TrimSpace(rawLine), "#") {
			continue
		}
		indent := len(rawLine) - len(strings.TrimLeft(rawLine, " "))
		trimmed := strings.TrimSpace(rawLine)

		if indent == 0 {
			flushResp()
			section = ""
			sectionIndent = -1
			if strings.HasPrefix(trimmed, "modified_files:") {
				section = "modified_files"
				sectionIndent = 0
			} else if strings.HasPrefix(trimmed, "runtime_behavior_impact:") {
				section = "runtime_behavior_impact"
				sectionIndent = 0
			}
			continue
		}

		if section == "runtime_behavior_impact" && strings.HasPrefix(trimmed, "candidate_responses:") {
			section = "candidate_responses"
			sectionIndent = indent
			continue
		}

		if section == "candidate_responses" && indent <= sectionIndent {
			flushResp()
			section = "runtime_behavior_impact"
			continue
		}

		if section == "modified_files" {
			kv := strings.TrimPrefix(trimmed, "- ")
			kv = strings.TrimSpace(kv)
			if k, v, ok := splitYAMLKV(kv); ok && k == "path" && v != "" {
				out.ModifiedFiles = append(out.ModifiedFiles, normalizeRelPath(v))
			}
			continue
		}

		if section == "candidate_responses" {
			isItemStart := strings.HasPrefix(trimmed, "- ")
			kv := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if isItemStart {
				flushResp()
				curResp = &candidateResponse{}
			}
			if curResp == nil {
				curResp = &candidateResponse{}
			}
			if k, v, ok := splitYAMLKV(kv); ok {
				switch k {
				case "candidate_id":
					curResp.CandidateID = v
				case "file":
					curResp.File = v
				case "function":
					curResp.Function = v
				case "kind":
					curResp.Kind = v
				case "disposition":
					curResp.Disposition = v
				case "test_name":
					curResp.TestName = v
				}
			}
		}
	}
	flushResp()
	return out
}

func splitYAMLKV(s string) (string, string, bool) {
	idx := strings.IndexByte(s, ':')
	if idx <= 0 {
		return "", "", false
	}
	k := strings.TrimSpace(s[:idx])
	v := strings.TrimSpace(s[idx+1:])
	if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
		v = v[1 : len(v)-1]
	}
	return k, strings.TrimSpace(v), true
}
