package preflight

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validateSummary = "OK: 4 docs, 1 typed edges, no cycles"

// TestMain lets the compiled test binary act as a fast, zero-Python docdag stub
// when invoked with ALT_DOCDAG_STUB_MODE=1.
func TestMain(m *testing.M) {
	if os.Getenv("ALT_DOCDAG_STUB_MODE") == "1" {
		os.Exit(runDocdagStub(os.Args[1:]))
	}
	os.Exit(m.Run())
}

type stubBindingRow struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Path             string   `json:"path"`
	AffectedServices []string `json:"affected_services"`
}

type stubConfig struct {
	ValidateExit   int              `json:"validate_exit"`
	ValidateStdout string           `json:"validate_stdout"`
	ValidateStderr string           `json:"validate_stderr"`
	QueryExit      int              `json:"query_exit"`
	QueryStderr    string           `json:"query_stderr"`
	BindingRows    []stubBindingRow `json:"binding_rows"`
	ReportFields   bool             `json:"report_fields"`
}

type stubInvocation struct {
	Argv []string `json:"argv"`
	Cwd  string   `json:"cwd"`
}

func runDocdagStub(args []string) int {
	cfgPath := os.Getenv("ALT_DOCDAG_STUB_CONFIG")
	logPath := os.Getenv("ALT_DOCDAG_STUB_LOG")
	cwd, _ := os.Getwd()
	if logPath != "" {
		rec, _ := json.Marshal(stubInvocation{Argv: args, Cwd: cwd})
		if fh, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			_, _ = fh.Write(append(rec, '\n'))
			_ = fh.Close()
		}
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stub failed to read config: %v\n", err)
		return 2
	}
	var cfg stubConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "stub failed to parse config: %v\n", err)
		return 2
	}
	for _, a := range args {
		if a == "validate" {
			if cfg.ValidateStdout != "" {
				fmt.Fprint(os.Stdout, cfg.ValidateStdout)
			}
			if cfg.ValidateStderr != "" {
				fmt.Fprint(os.Stderr, cfg.ValidateStderr)
			}
			return cfg.ValidateExit
		}
	}
	if len(args) == 4 && args[0] == "query" && args[1] == "--binding" && args[2] == "--format" && args[3] == "json" {
		if cfg.QueryExit != 0 {
			qErr := cfg.QueryStderr
			if qErr == "" {
				qErr = "query failed\n"
			}
			fmt.Fprint(os.Stderr, qErr)
			return cfg.QueryExit
		}
		records := make([]map[string]any, 0, len(cfg.BindingRows))
		for _, row := range cfg.BindingRows {
			rec := map[string]any{
				"id":     row.ID,
				"title":  row.Title,
				"status": "accepted",
				"path":   row.Path,
			}
			if cfg.ReportFields {
				rec["fields"] = map[string]any{
					"affected_services": row.AffectedServices,
				}
			}
			records = append(records, rec)
		}
		enc, _ := json.Marshal(records)
		_, _ = os.Stdout.Write(enc)
		return 0
	}
	fmt.Fprintf(os.Stderr, "unexpected stub docdag args: %v\n", args)
	return 2
}

type preflightFixture struct {
	t            *testing.T
	tmpRoot      string
	repo         string
	binDir       string
	stateDir     string
	outsideDir   string
	docdagLog    string
	docdagConfig string
	bindingRows  []stubBindingRow
}

func renderADR(adrID, title, affectedServicesBlock, status string) string {
	if status == "" {
		status = "accepted"
	}
	capStatus := strings.ToUpper(status[:1]) + status[1:]
	return fmt.Sprintf(`---
title: %s
date: 2026-01-15
status: %s
%s
---
# ADR-%s: %s

## Status

%s

## Context

Context for %s.

## Decision

Decision for %s.

## Consequences

Consequences for %s.
`, title, status, affectedServicesBlock, adrID, title, capStatus, adrID, adrID, adrID)
}

func bindingRow(adrID, title string, services []string) stubBindingRow {
	return stubBindingRow{
		ID:               adrID,
		Title:            title,
		Path:             fmt.Sprintf("docs/ADR/%s.md", adrID),
		AffectedServices: services,
	}
}

func newFixture(t *testing.T) *preflightFixture {
	t.Helper()
	rawTmp := t.TempDir()
	tmpRoot, err := filepath.EvalSymlinks(rawTmp)
	if err != nil {
		tmpRoot = rawTmp
	}
	f := &preflightFixture{
		t:            t,
		tmpRoot:      tmpRoot,
		repo:         filepath.Join(tmpRoot, "repo"),
		binDir:       filepath.Join(tmpRoot, "bin"),
		stateDir:     filepath.Join(tmpRoot, "state"),
		outsideDir:   filepath.Join(tmpRoot, "outside"),
		docdagLog:    filepath.Join(tmpRoot, "docdag_invocations.jsonl"),
		docdagConfig: filepath.Join(tmpRoot, "docdag_config.json"),
	}
	for _, d := range []string{f.repo, f.binDir, f.outsideDir, filepath.Join(f.repo, "docs", "ADR")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustWriteFile(t, filepath.Join(f.outsideDir, "external.py"), "x = 1\n")
	mustWriteFile(t, filepath.Join(f.repo, "docdag.yaml"), "preset: adr\ndir: docs/ADR\nid_width: 6\nfilename: \"{id}.md\"\n")
	mustWriteFile(t, filepath.Join(f.repo, ".gitignore"), "ignored/\n")

	adrDir := filepath.Join(f.repo, "docs", "ADR")
	mustWriteFile(t, filepath.Join(adrDir, "000001.md"), renderADR("000001", "Backend baseline architecture", "affected_services: [alt-backend]", "accepted"))
	mustWriteFile(t, filepath.Join(adrDir, "000002.md"), renderADR("000002", "Harvester and MQ hub contract", "affected_services:\n  - alt-harvester\n  - \"mq-hub\"", "accepted"))
	mustWriteFile(t, filepath.Join(adrDir, "000003.md"), renderADR("000003", "Tag generator pipeline rules", "affected_services: [tag-generator]", "accepted"))
	mustWriteFile(t, filepath.Join(adrDir, "000004.md"), renderADR("000004", "Superseded backend record", "affected_services: [alt-backend]", "superseded"))

	f.git("init", "-q")
	f.git("config", "user.name", "Test User")
	f.git("config", "user.email", "test@example.com")
	f.git("config", "commit.gpgsign", "false")
	f.git("add", ".")
	f.git("commit", "-q", "-m", "initial fixture")

	f.bindingRows = []stubBindingRow{
		bindingRow("000001", "Backend baseline architecture", []string{"alt-backend"}),
		bindingRow("000002", "Harvester and MQ hub contract", []string{"alt-harvester", "mq-hub"}),
		bindingRow("000003", "Tag generator pipeline rules", []string{"tag-generator"}),
	}
	f.configureDocdag(stubConfig{
		ValidateStdout: validateSummary + "\n",
		ReportFields:   true,
	})
	f.writeDocdagStub()
	return f
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *preflightFixture) git(args ...string) {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func (f *preflightFixture) configureDocdag(cfg stubConfig) {
	f.t.Helper()
	if cfg.BindingRows != nil {
		f.bindingRows = cfg.BindingRows
	} else {
		cfg.BindingRows = f.bindingRows
	}
	if cfg.ValidateStdout == "" && cfg.ValidateExit == 0 {
		cfg.ValidateStdout = validateSummary + "\n"
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.docdagConfig, raw, 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *preflightFixture) writeDocdagStub() {
	f.t.Helper()
	exe, err := os.Executable()
	if err != nil {
		f.t.Fatal(err)
	}
	stubPath := filepath.Join(f.binDir, "docdag")
	script := fmt.Sprintf("#!/bin/sh\nALT_DOCDAG_STUB_MODE=1 ALT_DOCDAG_STUB_CONFIG=%q ALT_DOCDAG_STUB_LOG=%q exec %q \"$@\"\n",
		f.docdagConfig, f.docdagLog, exe)
	if err := os.WriteFile(stubPath, []byte(script), 0o755); err != nil {
		f.t.Fatal(err)
	}
}

func (f *preflightFixture) addBindingADR(adrID, title string, services []string, commit bool) {
	f.t.Helper()
	row := bindingRow(adrID, title, services)
	mustWriteFile(f.t, filepath.Join(f.repo, row.Path), renderADR(adrID, title, fmt.Sprintf("affected_services: [%s]", strings.Join(services, ", ")), "accepted"))
	if commit {
		f.git("add", row.Path)
		f.git("commit", "-q", "-m", "add "+adrID)
	}
	f.bindingRows = append(f.bindingRows, row)
	f.configureDocdag(stubConfig{ReportFields: true})
}

func (f *preflightFixture) docdagInvocations() []stubInvocation {
	f.t.Helper()
	raw, err := os.ReadFile(f.docdagLog)
	if err != nil {
		return nil
	}
	var res []stubInvocation
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var inv stubInvocation
		if err := json.Unmarshal([]byte(line), &inv); err != nil {
			f.t.Fatalf("parse invocation line %q: %v", line, err)
		}
		res = append(res, inv)
	}
	return res
}

func (f *preflightFixture) buildPathWithoutDocdag() string {
	f.t.Helper()
	essentialDir := filepath.Join(f.tmpRoot, "essential_bin")
	if err := os.MkdirAll(essentialDir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		f.t.Fatalf("git must exist on PATH: %v", err)
	}
	link := filepath.Join(essentialDir, "git")
	if _, err := os.Lstat(link); os.IsNotExist(err) {
		if err := os.Symlink(gitBin, link); err != nil {
			f.t.Fatal(err)
		}
	}
	entries := []string{essentialDir}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "docdag")); err == nil {
			continue
		}
		entries = append(entries, dir)
	}
	return strings.Join(entries, string(os.PathListSeparator))
}

type runResult struct {
	code   int
	stdout string
	stderr string
}

type runOpts struct {
	stdin        string
	cwd          string
	anchorDir    string
	envOverrides map[string]*string
	pathValue    string
}

func strPtr(s string) *string { return &s }

func (f *preflightFixture) run(args []string, ro runOpts) runResult {
	f.t.Helper()
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	delete(env, "ALT_ADR_PREFLIGHT")
	env["ALT_ADR_PREFLIGHT_STATE_DIR"] = f.stateDir
	env["GIT_CEILING_DIRECTORIES"] = f.tmpRoot
	if ro.pathValue != "" {
		env["PATH"] = ro.pathValue
	} else {
		env["PATH"] = f.binDir + string(os.PathListSeparator) + env["PATH"]
	}
	for k, v := range ro.envOverrides {
		if v == nil {
			delete(env, k)
		} else {
			env[k] = *v
		}
	}
	cwd := ro.cwd
	if cwd == "" {
		cwd = f.repo
	}
	anchor := ro.anchorDir
	if anchor == "" {
		anchor = f.repo
	}
	var outBuf, errBuf bytes.Buffer
	code := RunWithOptions(context.Background(), args, Options{
		Stdin:     strings.NewReader(ro.stdin),
		Stdout:    &outBuf,
		Stderr:    &errBuf,
		Cwd:       cwd,
		Env:       env,
		AnchorDir: anchor,
	})
	return runResult{code: code, stdout: outBuf.String(), stderr: errBuf.String()}
}

func (f *preflightFixture) makePretoolPayload(filePath, sessionID, agentID, toolName string) string {
	if sessionID == "" {
		sessionID = "sess-1"
	}
	if toolName == "" {
		toolName = "Edit"
	}
	payload := map[string]any{
		"session_id":      sessionID,
		"cwd":             f.repo,
		"hook_event_name": "PreToolUse",
		"tool_name":       toolName,
		"tool_input":      map[string]any{"file_path": filePath},
	}
	if agentID != "" {
		payload["agent_id"] = agentID
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func (f *preflightFixture) makeSessionStartPayload(sessionID string) string {
	if sessionID == "" {
		sessionID = "sess-1"
	}
	raw, _ := json.Marshal(map[string]any{
		"session_id":      sessionID,
		"cwd":             f.repo,
		"hook_event_name": "SessionStart",
		"source":          "startup",
	})
	return string(raw)
}

func firstLine(s string) string {
	if line, _, ok := strings.Cut(s, "\n"); ok {
		return line
	}
	return s
}

func findFixLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "FIX:") {
			out = append(out, line)
		}
	}
	return out
}

func Test01FirstPretooluseHoldsAndListsGoverningADRs(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	payload := f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", "")
	res := f.run([]string{"--hook"}, runOpts{stdin: payload})

	if res.code != 2 {
		t.Fatalf("code=%d want 2 stderr=%q", res.code, res.stderr)
	}
	if res.stdout != "" {
		t.Fatalf("stdout=%q want empty", res.stdout)
	}
	if !strings.HasPrefix(res.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("stderr=%q", res.stderr)
	}
	if !strings.Contains(firstLine(res.stderr), "alt-backend") {
		t.Fatalf("first line missing alt-backend: %q", firstLine(res.stderr))
	}
	for _, want := range []string{
		validateSummary,
		"  000002  Harvester and MQ hub contract",
		"  000001  Backend baseline architecture",
		"docdag context",
		"--depth 2 --budget 1500",
		"./scripts/altharness/alt-harness preflight --service alt-backend",
	} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, res.stderr)
		}
	}
	if strings.Index(res.stderr, "  000002  ") >= strings.Index(res.stderr, "  000001  ") {
		t.Errorf("expected 000002 before 000001:\n%s", res.stderr)
	}
	for _, forbid := range []string{"000003", "000004"} {
		if strings.Contains(res.stderr, forbid) {
			t.Errorf("stderr should not contain %q:\n%s", forbid, res.stderr)
		}
	}
	lower := strings.ToLower(res.stderr)
	if !strings.Contains(lower, "supersede") || !strings.Contains(lower, "retry") {
		t.Errorf("missing supersede/retry guidance:\n%s", res.stderr)
	}
	if len(res.stderr) >= 6000 {
		t.Errorf("stderr len=%d >= 6000", len(res.stderr))
	}
}

func Test02RepeatPretooluseExits0WithoutInvokingDocdag(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	payload := f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", "")
	first := f.run([]string{"--hook"}, runOpts{stdin: payload})
	if first.code != 2 || !strings.HasPrefix(first.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("first=%+v", first)
	}
	callsAfterFirst := len(f.docdagInvocations())
	if callsAfterFirst == 0 {
		t.Fatalf("expected docdag invocations after first call")
	}

	second := f.run([]string{"--hook"}, runOpts{stdin: payload})
	if second.code != 0 || second.stdout != "" || second.stderr != "" {
		t.Fatalf("second=%+v", second)
	}
	if len(f.docdagInvocations()) != callsAfterFirst {
		t.Fatalf("docdag called again on cached receipt")
	}
}

func Test03DifferentScopeInSameSessionIsHeld(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	first := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", "")})
	if first.code != 2 {
		t.Fatalf("first=%+v", first)
	}
	second := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(filepath.Join(f.repo, "tag-generator/app/main.py"), "", "", "")})
	if second.code != 2 || !strings.HasPrefix(second.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("second=%+v", second)
	}
	if !strings.Contains(firstLine(second.stderr), "tag-generator") ||
		!strings.Contains(second.stderr, "  000003  Tag generator pipeline rules") ||
		strings.Contains(second.stderr, "  000001  ") ||
		strings.Contains(second.stderr, "  000002  ") {
		t.Fatalf("unexpected second stderr:\n%s", second.stderr)
	}
}

func Test04SameScopeWithDifferentAgentIDIsHeldAgain(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	first := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", "")})
	if first.code != 2 {
		t.Fatalf("first=%+v", first)
	}
	second := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "worker-subagent-1", "")})
	if second.code != 2 || !strings.HasPrefix(second.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("second=%+v", second)
	}
}

func Test05ModifyingADRCorpusInvalidatesReceipt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	payload := f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", "")
	if res := f.run([]string{"--hook"}, runOpts{stdin: payload}); res.code != 2 {
		t.Fatalf("first=%+v", res)
	}
	if res := f.run([]string{"--hook"}, runOpts{stdin: payload}); res.code != 0 {
		t.Fatalf("second=%+v", res)
	}
	adr1 := filepath.Join(f.repo, "docs/ADR/000001.md")
	orig, err := os.ReadFile(adr1)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, adr1, string(orig)+"\nClarified invariant.\n")

	third := f.run([]string{"--hook"}, runOpts{stdin: payload})
	if third.code != 2 || !strings.HasPrefix(third.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("third=%+v", third)
	}
}

func Test06ValidateExit1BlocksAndRetryRemainsBlocked(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.configureDocdag(stubConfig{
		ValidateExit:   1,
		ValidateStdout: "ERROR docs/ADR/000001.md: missing section Consequences\n",
		ReportFields:   true,
	})
	payload := f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", "")

	first := f.run([]string{"--hook"}, runOpts{stdin: payload})
	if first.code != 2 || !strings.HasPrefix(first.stderr, "ADR PREFLIGHT BLOCKED") {
		t.Fatalf("first=%+v", first)
	}
	if !strings.Contains(first.stderr, "ERROR docs/ADR/000001.md: missing section Consequences") {
		t.Fatalf("missing error line: %s", first.stderr)
	}
	fixLines := findFixLines(first.stderr)
	if len(fixLines) == 0 || !strings.Contains(fixLines[0], "docdag validate") {
		t.Fatalf("missing FIX line: %s", first.stderr)
	}

	retry := f.run([]string{"--hook"}, runOpts{stdin: payload})
	if retry.code != 2 || !strings.HasPrefix(retry.stderr, "ADR PREFLIGHT BLOCKED") {
		t.Fatalf("retry=%+v", retry)
	}
}

func Test07ValidateExit3FixMentionsStaleConfigOrBinary(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.configureDocdag(stubConfig{
		ValidateExit:   3,
		ValidateStdout: "docdag: unknown configuration field\n",
		ReportFields:   true,
	})
	res := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", "")})
	if res.code != 2 || !strings.HasPrefix(res.stderr, "ADR PREFLIGHT BLOCKED") {
		t.Fatalf("res=%+v", res)
	}
	fixLines := findFixLines(res.stderr)
	if len(fixLines) == 0 {
		t.Fatalf("missing FIX line: %s", res.stderr)
	}
	fixLower := strings.ToLower(fixLines[0])
	if !strings.Contains(fixLower, "stale") || (!strings.Contains(fixLower, "config") && !strings.Contains(fixLower, "binary")) {
		t.Fatalf("unexpected FIX line: %s", fixLines[0])
	}
}

func Test08MissingDocdagOnPathExits2WithInstallFix(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	mustWriteFile(t, filepath.Join(f.repo, ".github/workflows/adr-graph.yml"), "env:\n  DOCDAG_VERSION: v9.9.9\n")
	res := f.run([]string{"--hook"}, runOpts{
		stdin:     f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", ""),
		pathValue: f.buildPathWithoutDocdag(),
	})
	if res.code != 2 || !strings.HasPrefix(res.stderr, "ADR PREFLIGHT BLOCKED") {
		t.Fatalf("res=%+v", res)
	}
	if !strings.Contains(res.stderr, "docdag is not on PATH") {
		t.Fatalf("stderr=%s", res.stderr)
	}
	fixLines := findFixLines(res.stderr)
	if len(fixLines) == 0 ||
		!strings.Contains(fixLines[0], "go install github.com/Kaikei-e/DocDag/cmd/docdag@v9.9.9") ||
		!strings.Contains(fixLines[0], "DOCDAG_VERSION") ||
		!strings.Contains(fixLines[0], ".github/workflows/adr-graph.yml") {
		t.Fatalf("unexpected FIX line: %v", fixLines)
	}
}

func Test09KillSwitchDisablesPretooluseAndReportsOnSessionStart(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	disabled := map[string]*string{"ALT_ADR_PREFLIGHT": strPtr("disabled")}
	pre := f.run([]string{"--hook"}, runOpts{
		stdin:        f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", ""),
		envOverrides: disabled,
	})
	if pre.code != 0 || pre.stdout != "" || pre.stderr != "" || len(f.docdagInvocations()) != 0 {
		t.Fatalf("pre=%+v invocations=%d", pre, len(f.docdagInvocations()))
	}
	start := f.run([]string{"--hook"}, runOpts{
		stdin:        f.makeSessionStartPayload(""),
		envOverrides: disabled,
	})
	if start.code != 0 || !strings.Contains(start.stdout, "adr_preflight_disabled") || len(f.docdagInvocations()) != 0 {
		t.Fatalf("start=%+v invocations=%d", start, len(f.docdagInvocations()))
	}
}

func Test10ExemptPathsExit0WithoutInvokingDocdag(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	targets := []string{
		filepath.Join(f.repo, "docs/ADR/000005.md"),
		filepath.Join(f.outsideDir, "external.py"),
		filepath.Join(f.repo, "ignored/file.txt"),
	}
	for _, target := range targets {
		res := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(target, "", "", "")})
		if res.code != 0 || res.stdout != "" || res.stderr != "" {
			t.Fatalf("target=%s res=%+v", target, res)
		}
	}
	if len(f.docdagInvocations()) != 0 {
		t.Fatalf("expected 0 docdag invocations, got %d", len(f.docdagInvocations()))
	}
}

func Test11RootLevelFileGetsRepoRootScopeWithZeroADRWord(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	res := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(filepath.Join(f.repo, "Makefile"), "", "", "")})
	if res.code != 2 || !strings.HasPrefix(res.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("res=%+v", res)
	}
	if !strings.Contains(firstLine(res.stderr), "repo-root") ||
		!strings.Contains(res.stderr, "Scope repo-root: 0 governing binding ADRs") ||
		!strings.Contains(res.stderr, "docdag query --binding --fields id,title") {
		t.Fatalf("unexpected stderr:\n%s", res.stderr)
	}
}

func Test12RealWorktreeUsesWorktreeRootAndIsolatedReceipt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	mainEdit := f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/main.go"), "", "", "")
	if res := f.run([]string{"--hook"}, runOpts{stdin: mainEdit}); res.code != 2 {
		t.Fatalf("mainHeld=%+v", res)
	}
	if res := f.run([]string{"--hook"}, runOpts{stdin: mainEdit}); res.code != 0 {
		t.Fatalf("mainRetry=%+v", res)
	}

	wtRoot := filepath.Join(f.repo, ".claude", "worktrees", "w1")
	f.git("worktree", "add", "-q", "-b", "wt-w1", wtRoot)
	if _, err := os.Stat(filepath.Join(wtRoot, "docs/ADR/000001.md")); err != nil {
		t.Fatal(err)
	}

	wtPayload := f.makePretoolPayload(filepath.Join(wtRoot, "alt-backend/x.go"), "", "", "")
	res := f.run([]string{"--hook"}, runOpts{stdin: wtPayload})
	if res.code != 2 || !strings.HasPrefix(res.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("wt res=%+v", res)
	}
	fl := firstLine(res.stderr)
	if !strings.Contains(fl, "alt-backend") || strings.Contains(fl, ".claude") {
		t.Fatalf("unexpected first line: %s", fl)
	}
	if !strings.Contains(res.stderr, "  000002  ") || !strings.Contains(res.stderr, "  000001  ") {
		t.Fatalf("missing ADRs: %s", res.stderr)
	}
	invs := f.docdagInvocations()
	wantCwd, _ := filepath.EvalSymlinks(wtRoot)
	if len(invs) == 0 || invs[len(invs)-1].Cwd != wantCwd {
		t.Fatalf("last cwd=%q want %q", invs[len(invs)-1].Cwd, wantCwd)
	}

	if follow := f.run([]string{"--hook"}, runOpts{stdin: wtPayload}); follow.code != 0 {
		t.Fatalf("followup=%+v", follow)
	}
}

func Test13CodexApplyPatchHoldsBothScopesThenAllowsFollowup(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	patchCmd := "*** Begin Patch\n*** Update File: tag-generator/a.py\n@@\n-old = 1\n+old = 2\n*** Add File: mq-hub/b.go\n+package main\n*** End Patch\n"
	raw, _ := json.Marshal(map[string]any{
		"session_id":      "sess-codex",
		"cwd":             f.repo,
		"hook_event_name": "PreToolUse",
		"tool_name":       "apply_patch",
		"tool_input":      map[string]any{"command": patchCmd},
	})
	first := f.run([]string{"--hook"}, runOpts{stdin: string(raw)})
	if first.code != 2 || !strings.HasPrefix(first.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("first=%+v", first)
	}
	fl := firstLine(first.stderr)
	if !strings.Contains(fl, "tag-generator") || !strings.Contains(fl, "mq-hub") ||
		!strings.Contains(first.stderr, "  000003  ") || !strings.Contains(first.stderr, "  000002  ") {
		t.Fatalf("unexpected stderr:\n%s", first.stderr)
	}
	second := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(filepath.Join(f.repo, "mq-hub/c.go"), "sess-codex", "", "")})
	if second.code != 0 || second.stderr != "" {
		t.Fatalf("second=%+v", second)
	}
}

func Test14GitHubDirectoryUsesCiCdAlias(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addBindingADR("000005", "CI workflow pinning policy", []string{"ci-cd"}, true)
	res := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(filepath.Join(f.repo, ".github/workflows/x.yml"), "", "", "")})
	if res.code != 2 || !strings.HasPrefix(res.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("res=%+v", res)
	}
	if !strings.Contains(firstLine(res.stderr), ".github") ||
		!strings.Contains(res.stderr, "  000005  CI workflow pinning policy") ||
		!strings.Contains(res.stderr, "--service ci-cd") {
		t.Fatalf("unexpected stderr:\n%s", res.stderr)
	}
}

func Test15SessionStartPrintsSummaryAndClearsSessionReceipts(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	editPayload := f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "sess-restart", "", "")
	if res := f.run([]string{"--hook"}, runOpts{stdin: editPayload}); res.code != 2 {
		t.Fatalf("held=%+v", res)
	}
	if res := f.run([]string{"--hook"}, runOpts{stdin: editPayload}); res.code != 0 {
		t.Fatalf("allowed=%+v", res)
	}

	otherSession := filepath.Join(f.stateDir, "other-session")
	mustWriteFile(t, filepath.Join(otherSession, "main", "alt-backend"), "old")
	eightDaysAgo := time.Now().Add(-8 * 24 * time.Hour)
	_ = os.Chtimes(otherSession, eightDaysAgo, eightDaysAgo)

	start := f.run([]string{"--hook"}, runOpts{stdin: f.makeSessionStartPayload("sess-restart")})
	if start.code != 0 || !strings.HasPrefix(start.stdout, "adr_preflight_enabled") || !strings.Contains(start.stdout, validateSummary) {
		t.Fatalf("start=%+v", start)
	}
	if _, err := os.Stat(otherSession); err != nil {
		t.Fatalf("other session should still exist: %v", err)
	}

	heldAgain := f.run([]string{"--hook"}, runOpts{stdin: editPayload})
	if heldAgain.code != 2 || !strings.HasPrefix(heldAgain.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("heldAgain=%+v", heldAgain)
	}
}

func Test16SessionStartWhenValidateFailsExits0AndWarnsBlocked(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.configureDocdag(stubConfig{
		ValidateExit:   1,
		ValidateStdout: "ERROR docs/ADR/000002.md: dangling link [[999999]]\n",
		ReportFields:   true,
	})
	res := f.run([]string{"--hook"}, runOpts{stdin: f.makeSessionStartPayload("")})
	if res.code != 0 || !strings.HasPrefix(res.stdout, "adr_preflight_enabled") ||
		!strings.Contains(strings.ToLower(res.stdout), "blocked") ||
		!strings.Contains(res.stdout, "ERROR docs/ADR/000002.md: dangling link [[999999]]") {
		t.Fatalf("res=%+v", res)
	}
}

func Test17MalformedStdinOnHookExits0(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	res := f.run([]string{"--hook"}, runOpts{stdin: "{not valid json"})
	if res.code != 0 || res.stdout != "" || res.stderr != "" {
		t.Fatalf("res=%+v", res)
	}
}

func Test18CLIServiceListsExactMatchingBindingRows(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	backend := f.run([]string{"--service", "alt-backend"}, runOpts{})
	if backend.code != 0 {
		t.Fatalf("backend=%+v", backend)
	}
	lines := strings.Split(strings.TrimSuffix(backend.stdout, "\n"), "\n")
	if len(lines) != 1 || lines[0] != "000001\tBackend baseline architecture\tdocs/ADR/000001.md" {
		t.Fatalf("lines=%q", lines)
	}
	none := f.run([]string{"--service", "nosuch"}, runOpts{})
	if none.code != 0 || none.stdout != "" {
		t.Fatalf("none=%+v", none)
	}
}

func Test19CLIPathExits1WhenValidateFailsAnd0WithBriefWhenValid(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.configureDocdag(stubConfig{
		ValidateExit:   1,
		ValidateStdout: "ERROR docs/ADR/000001.md: cycle detected\n",
		ReportFields:   true,
	})
	bad := f.run([]string{"alt-backend/app/x.go"}, runOpts{})
	if bad.code != 1 || !strings.Contains(bad.stdout, "ERROR docs/ADR/000001.md: cycle detected") {
		t.Fatalf("bad=%+v", bad)
	}

	f.configureDocdag(stubConfig{ValidateExit: 0, ReportFields: true})
	good := f.run([]string{"alt-backend/app/x.go"}, runOpts{})
	if good.code != 0 {
		t.Fatalf("good=%+v", good)
	}
	for _, want := range []string{
		validateSummary,
		"alt-backend",
		"  000002  Harvester and MQ hub contract",
		"  000001  Backend baseline architecture",
		"docdag context",
		"--depth 2 --budget 1500",
		"--service",
	} {
		if !strings.Contains(good.stdout, want) {
			t.Errorf("good stdout missing %q:\n%s", want, good.stdout)
		}
	}
	for _, forbid := range []string{"000003", "000004", "ADR PREFLIGHT (not an error)"} {
		if strings.Contains(good.stdout, forbid) {
			t.Errorf("good stdout should not contain %q", forbid)
		}
	}
	if !strings.Contains(strings.ToLower(good.stdout), "supersede") || strings.Contains(strings.ToLower(good.stdout), "retry") {
		t.Errorf("unexpected supersede/retry in CLI stdout:\n%s", good.stdout)
	}
}

func Test20BriefWith30BindingADRsListsOnly8NewestAndStatesCount(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for num := 101; num <= 130; num++ {
		adrID := fmt.Sprintf("%06d", num)
		title := fmt.Sprintf("Search indexer governance decision %s %s", adrID, strings.Repeat("detail-", 12))
		f.addBindingADR(adrID, strings.TrimSpace(title), []string{"search-indexer"}, false)
	}
	f.git("add", "docs/ADR")
	f.git("commit", "-q", "-m", "add 30 search-indexer ADRs")

	res := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(filepath.Join(f.repo, "search-indexer/app/main.go"), "", "", "")})
	if res.code != 2 || !strings.HasPrefix(res.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("res=%+v", res)
	}
	if !strings.Contains(res.stderr, "Scope search-indexer: 30 governing binding ADRs (8 newest shown):") {
		t.Fatalf("missing count header:\n%s", res.stderr)
	}
	for n := 123; n <= 130; n++ {
		if !strings.Contains(res.stderr, fmt.Sprintf("  %06d  ", n)) {
			t.Errorf("missing newest %06d", n)
		}
	}
	for n := 101; n <= 122; n++ {
		if strings.Contains(res.stderr, fmt.Sprintf("%06d", n)) {
			t.Errorf("unexpected older %06d", n)
		}
	}
	if len(res.stderr) >= 6000 {
		t.Fatalf("len=%d >= 6000", len(res.stderr))
	}
}

func Test21CommittingDocdagYamlOnlyChangeReholdsScope(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	payload := f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", "")
	if res := f.run([]string{"--hook"}, runOpts{stdin: payload}); res.code != 2 {
		t.Fatalf("first=%+v", res)
	}
	if res := f.run([]string{"--hook"}, runOpts{stdin: payload}); res.code != 0 {
		t.Fatalf("second=%+v", res)
	}
	cfgPath := filepath.Join(f.repo, "docdag.yaml")
	orig, _ := os.ReadFile(cfgPath)
	mustWriteFile(t, cfgPath, string(orig)+"acyclic_union: true\n")
	f.git("add", "docdag.yaml")
	f.git("commit", "-q", "-m", "update docdag.yaml only")

	third := f.run([]string{"--hook"}, runOpts{stdin: payload})
	if third.code != 2 || !strings.HasPrefix(third.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("third=%+v", third)
	}
}

func Test22StateDirAsRegularFileReportsInternalError(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	blocker := filepath.Join(f.tmpRoot, "state_blocker_file")
	mustWriteFile(t, blocker, "not a directory\n")

	res := f.run([]string{"--hook"}, runOpts{
		stdin:        f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", ""),
		envOverrides: map[string]*string{"ALT_ADR_PREFLIGHT_STATE_DIR": strPtr(blocker)},
	})
	if res.code != 2 || !strings.HasPrefix(res.stderr, "ADR PREFLIGHT BLOCKED") ||
		!strings.Contains(res.stderr, "internal error") ||
		!strings.Contains(res.stderr, "ALT_ADR_PREFLIGHT=disabled") {
		t.Fatalf("res=%+v", res)
	}
}

func Test23UnrelatedGitRepoIsExempt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	otherRepo := filepath.Join(f.tmpRoot, "other_repo")
	mustWriteFile(t, filepath.Join(otherRepo, "docdag.yaml"), "preset: adr\ndir: docs/ADR\nid_width: 6\nfilename: \"{id}.md\"\n")
	mustWriteFile(t, filepath.Join(otherRepo, "docs/ADR/000001.md"), renderADR("000001", "Other repo decision", "affected_services: [other-svc]", "accepted"))
	for _, gargs := range [][]string{
		{"init", "-q"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
		{"config", "commit.gpgsign", "false"},
		{"add", "."},
		{"commit", "-q", "-m", "init other repo"},
	} {
		cmd := exec.Command("git", gargs...)
		cmd.Dir = otherRepo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", gargs, err, out)
		}
	}

	raw, _ := json.Marshal(map[string]any{
		"session_id":      "sess-other",
		"cwd":             otherRepo,
		"hook_event_name": "PreToolUse",
		"tool_name":       "Edit",
		"tool_input":      map[string]any{"file_path": filepath.Join(otherRepo, "other-svc/app.py")},
	})
	res := f.run([]string{"--hook"}, runOpts{stdin: string(raw), cwd: otherRepo})
	if res.code != 0 || res.stdout != "" || res.stderr != "" || len(f.docdagInvocations()) != 0 {
		t.Fatalf("res=%+v invs=%d", res, len(f.docdagInvocations()))
	}

	cliRes := f.run([]string{filepath.Join(otherRepo, "other-svc/app.py")}, runOpts{cwd: otherRepo})
	if cliRes.code != 0 || !strings.Contains(cliRes.stdout, "No non-exempt scopes") {
		t.Fatalf("cliRes=%+v", cliRes)
	}
}

func Test24SessionStartValidatesScriptCheckoutNotPayloadCwd(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	raw, _ := json.Marshal(map[string]any{
		"session_id":      "sess-cwd-mismatch",
		"cwd":             f.outsideDir,
		"hook_event_name": "SessionStart",
		"source":          "startup",
	})
	res := f.run([]string{"--hook"}, runOpts{stdin: string(raw), cwd: f.outsideDir})
	if res.code != 0 || !strings.HasPrefix(res.stdout, "adr_preflight_enabled") || !strings.Contains(res.stdout, validateSummary) {
		t.Fatalf("res=%+v", res)
	}
	invs := f.docdagInvocations()
	wantCwd, _ := filepath.EvalSymlinks(f.repo)
	if len(invs) == 0 || invs[len(invs)-1].Cwd != wantCwd {
		t.Fatalf("last cwd=%q want %q", invs[len(invs)-1].Cwd, wantCwd)
	}
}

func Test25DocdagYamlIsExemptEvenWhenValidateExits3(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.configureDocdag(stubConfig{ValidateExit: 3, ValidateStdout: "docdag: invalid config\n", ReportFields: true})

	cfgRes := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(filepath.Join(f.repo, "docdag.yaml"), "", "", "")})
	if cfgRes.code != 0 || cfgRes.stdout != "" || cfgRes.stderr != "" || len(f.docdagInvocations()) != 0 {
		t.Fatalf("cfgRes=%+v invs=%d", cfgRes, len(f.docdagInvocations()))
	}

	cliCfg := f.run([]string{"docdag.yaml"}, runOpts{})
	if cliCfg.code != 1 {
		t.Fatalf("cliCfg=%+v", cliCfg)
	}

	f.configureDocdag(stubConfig{ValidateExit: 0, ReportFields: true})
	cliClean := f.run([]string{"docdag.yaml"}, runOpts{})
	if cliClean.code != 0 || !strings.Contains(cliClean.stdout, "No non-exempt scopes") {
		t.Fatalf("cliClean=%+v", cliClean)
	}

	f.configureDocdag(stubConfig{ValidateExit: 3, ValidateStdout: "docdag: invalid config\n", ReportFields: true})
	otherRoot := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(filepath.Join(f.repo, "Makefile"), "", "", "")})
	if otherRoot.code != 2 || !strings.HasPrefix(otherRoot.stderr, "ADR PREFLIGHT BLOCKED") {
		t.Fatalf("otherRoot=%+v", otherRoot)
	}
}

func Test26PatchSpanningMainAndWorktreeSameScopeKeysReceiptByRoot(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	wtRoot := filepath.Join(f.repo, ".claude", "worktrees", "w-shared")
	f.git("worktree", "add", "-q", "-b", "wt-shared", wtRoot)

	patchCmd := fmt.Sprintf("*** Begin Patch\n*** Add File: %s\n+package a\n*** Add File: %s\n+package b\n*** End Patch\n",
		filepath.Join(f.repo, "alt-backend/a.go"),
		filepath.Join(wtRoot, "alt-backend/b.go"))
	raw, _ := json.Marshal(map[string]any{
		"session_id":      "sess-multi-root",
		"cwd":             f.repo,
		"hook_event_name": "PreToolUse",
		"tool_name":       "apply_patch",
		"tool_input":      map[string]any{"command": patchCmd},
	})
	first := f.run([]string{"--hook"}, runOpts{stdin: string(raw)})
	if first.code != 2 || !strings.HasPrefix(first.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("first=%+v", first)
	}
	callsAfterFirst := len(f.docdagInvocations())
	second := f.run([]string{"--hook"}, runOpts{stdin: string(raw)})
	if second.code != 0 || second.stderr != "" || len(f.docdagInvocations()) != callsAfterFirst {
		t.Fatalf("second=%+v calls=%d/%d", second, len(f.docdagInvocations()), callsAfterFirst)
	}
}

func Test27SecondEditToAlreadyDirtyADRReholdsScope(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	payload := f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", "")
	if f.run([]string{"--hook"}, runOpts{stdin: payload}).code != 2 {
		t.Fatalf("expected hold")
	}
	if f.run([]string{"--hook"}, runOpts{stdin: payload}).code != 0 {
		t.Fatalf("expected allow")
	}

	adr1 := filepath.Join(f.repo, "docs/ADR/000001.md")
	orig, _ := os.ReadFile(adr1)
	mustWriteFile(t, adr1, string(orig)+"\nFirst dirty edit.\n")

	if f.run([]string{"--hook"}, runOpts{stdin: payload}).code != 2 {
		t.Fatalf("expected hold after first dirty edit")
	}
	if f.run([]string{"--hook"}, runOpts{stdin: payload}).code != 0 {
		t.Fatalf("expected allow after first dirty edit")
	}

	mustWriteFile(t, adr1, string(orig)+"\nSecond dirty edit with new bytes.\n")
	reheld := f.run([]string{"--hook"}, runOpts{stdin: payload})
	if reheld.code != 2 || !strings.HasPrefix(reheld.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("reheld=%+v", reheld)
	}
}

func Test28UncommittedDocdagYamlCausesFingerprintInternalError(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.git("rm", "--cached", "-q", "docdag.yaml")
	f.git("commit", "-q", "-m", "drop docdag.yaml from HEAD")

	res := f.run([]string{"--hook"}, runOpts{stdin: f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", "")})
	if res.code != 2 || !strings.HasPrefix(res.stderr, "ADR PREFLIGHT BLOCKED") ||
		!strings.Contains(res.stderr, "internal error") ||
		!strings.Contains(res.stderr, "ALT_ADR_PREFLIGHT=disabled") {
		t.Fatalf("res=%+v", res)
	}
}

func Test29SessionStartInternalExceptionExits0WithMessage(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	res := f.run([]string{"--hook"}, runOpts{
		stdin:     f.makeSessionStartPayload(""),
		cwd:       f.outsideDir,
		anchorDir: f.outsideDir,
	})
	if res.code != 0 || !strings.HasPrefix(res.stdout, "adr_preflight_enabled: internal error (") ||
		!strings.Contains(res.stdout, "code edits will be blocked until it is fixed") {
		t.Fatalf("res=%+v", res)
	}
}

func Test30ServiceStaleDocdagDoesNotMisreportMissingDocdag(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.configureDocdag(stubConfig{ReportFields: false})

	stale := f.run([]string{"--service", "alt-backend"}, runOpts{})
	if stale.code != 2 || strings.Contains(stale.stderr, "docdag is not on PATH") ||
		!strings.Contains(stale.stderr, "older than v0.6.0") ||
		!strings.Contains(stale.stderr, "go install github.com/Kaikei-e/DocDag") {
		t.Fatalf("stale=%+v", stale)
	}

	missing := f.run([]string{"--service", "alt-backend"}, runOpts{pathValue: f.buildPathWithoutDocdag()})
	if missing.code != 2 || !strings.Contains(missing.stderr, "docdag is not on PATH") {
		t.Fatalf("missing=%+v", missing)
	}
}

func Test31BriefTruncatesOver6000CharsWith12Scopes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	scopes := make([]string, 0, 12)
	for i := 1; i <= 12; i++ {
		scopes = append(scopes, fmt.Sprintf("service-%02d", i))
	}
	adrCounter := 100
	for _, scope := range scopes {
		for item := 0; item < 8; item++ {
			adrCounter++
			adrID := fmt.Sprintf("%06d", adrCounter)
			title := fmt.Sprintf("Scope %s item %d %s", scope, item, strings.Repeat("x", 135))
			f.addBindingADR(adrID, title, []string{scope}, false)
		}
	}
	f.git("add", "docs/ADR")
	f.git("commit", "-q", "-m", "add 96 ADRs across 12 scopes")

	patchLines := []string{"*** Begin Patch"}
	for _, scope := range scopes {
		patchLines = append(patchLines, fmt.Sprintf("*** Add File: %s/main.go", scope), "+package main")
	}
	patchLines = append(patchLines, "*** End Patch")

	raw, _ := json.Marshal(map[string]any{
		"session_id":      "sess-trunc",
		"cwd":             f.repo,
		"hook_event_name": "PreToolUse",
		"tool_name":       "apply_patch",
		"tool_input":      map[string]any{"command": strings.Join(patchLines, "\n")},
	})
	res := f.run([]string{"--hook"}, runOpts{stdin: string(raw)})
	if res.code != 2 || !strings.HasPrefix(res.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("res=%+v", res)
	}
	if len(res.stderr) >= 6000 {
		t.Fatalf("len=%d >= 6000", len(res.stderr))
	}
	for _, want := range []string{
		"... (brief truncated; run the --service command per scope)",
		"docdag context <id> --depth 2 --budget 1500",
		"List all for a service:",
		"If a planned change contradicts a binding ADR, stop and supersede it with a new ADR rather than contradicting it silently.",
	} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("missing %q:\n%s", want, res.stderr)
		}
	}
	trimmedLines := strings.Split(strings.TrimSpace(res.stderr), "\n")
	if !strings.Contains(trimmedLines[len(trimmedLines)-1], "retrying the same edit now succeeds") {
		t.Fatalf("last line=%q", trimmedLines[len(trimmedLines)-1])
	}
}

func Test32RelativeFilePathResolvesAgainstPayloadCwd(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	subCwd := filepath.Join(f.repo, "alt-backend")
	if err := os.MkdirAll(subCwd, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"session_id":      "sess-rel",
		"cwd":             subCwd,
		"hook_event_name": "PreToolUse",
		"tool_name":       "Edit",
		"tool_input":      map[string]any{"file_path": "app/relative.go"},
	})
	res := f.run([]string{"--hook"}, runOpts{stdin: string(raw), cwd: f.outsideDir})
	if res.code != 2 || !strings.HasPrefix(res.stderr, "ADR PREFLIGHT (not an error)") ||
		!strings.Contains(firstLine(res.stderr), "alt-backend") ||
		!strings.Contains(res.stderr, "  000002  ") {
		t.Fatalf("res=%+v", res)
	}
}

func Test33NotebookPathPayloadIsGated(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	raw, _ := json.Marshal(map[string]any{
		"session_id":      "sess-nb",
		"cwd":             f.repo,
		"hook_event_name": "PreToolUse",
		"tool_name":       "NotebookEdit",
		"tool_input":      map[string]any{"notebook_path": filepath.Join(f.repo, "tag-generator/explore.ipynb")},
	})
	res := f.run([]string{"--hook"}, runOpts{stdin: string(raw)})
	if res.code != 2 || !strings.HasPrefix(res.stderr, "ADR PREFLIGHT (not an error)") ||
		!strings.Contains(firstLine(res.stderr), "tag-generator") ||
		!strings.Contains(res.stderr, "  000003  ") {
		t.Fatalf("res=%+v", res)
	}
}

func Test34ApplyPatchDeleteAndMoveToAreGated(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	patchCmd := "*** Begin Patch\n*** Delete File: alt-backend/old.go\n*** Update File: tag-generator/src.py\n*** Move to: mq-hub/moved.py\n@@\n-a = 1\n+a = 2\n*** End Patch\n"
	raw, _ := json.Marshal(map[string]any{
		"session_id":      "sess-del-move",
		"cwd":             f.repo,
		"hook_event_name": "PreToolUse",
		"tool_name":       "apply_patch",
		"tool_input":      map[string]any{"command": patchCmd},
	})
	res := f.run([]string{"--hook"}, runOpts{stdin: string(raw)})
	if res.code != 2 {
		t.Fatalf("res=%+v", res)
	}
	fl := firstLine(res.stderr)
	for _, want := range []string{"alt-backend", "tag-generator", "mq-hub"} {
		if !strings.Contains(fl, want) {
			t.Errorf("first line %q missing %q", fl, want)
		}
	}
}

func Test35CLIChangedReportsServiceScopeOnlyAndExits1OnInvalid(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	svcFile := filepath.Join(f.repo, "tag-generator", "worker.py")
	mustWriteFile(t, svcFile, "v = 1\n")
	f.git("add", "tag-generator/worker.py")
	f.git("commit", "-q", "-m", "add tracked service file")

	mustWriteFile(t, svcFile, "v = 2\n")
	mustWriteFile(t, filepath.Join(f.repo, "docs", "notes.md"), "untracked\n")

	good := f.run([]string{"--changed"}, runOpts{})
	if good.code != 0 || !strings.Contains(good.stdout, "Scope tag-generator:") ||
		!strings.Contains(good.stdout, "  000003  ") ||
		strings.Contains(good.stdout, "Scope docs:") {
		t.Fatalf("good=%+v", good)
	}

	f.configureDocdag(stubConfig{ValidateExit: 1, ValidateStdout: "ERROR docs/ADR/000001.md: broken\n", ReportFields: true})
	bad := f.run([]string{"--changed"}, runOpts{})
	if bad.code != 1 || !strings.Contains(bad.stdout, "ERROR docs/ADR/000001.md: broken") {
		t.Fatalf("bad=%+v", bad)
	}
}

func Test36PretooluseQueryFailureBlocksWithoutWritingReceipt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.configureDocdag(stubConfig{QueryExit: 1, QueryStderr: "query boom\n", ReportFields: true})
	payload := f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "", "", "")

	bad := f.run([]string{"--hook"}, runOpts{stdin: payload})
	if bad.code != 2 || !strings.HasPrefix(bad.stderr, "ADR PREFLIGHT BLOCKED") || !strings.Contains(bad.stderr, "internal error") {
		t.Fatalf("bad=%+v", bad)
	}

	f.configureDocdag(stubConfig{QueryExit: 0, ReportFields: true})
	retry := f.run([]string{"--hook"}, runOpts{stdin: payload})
	if retry.code != 2 || !strings.HasPrefix(retry.stderr, "ADR PREFLIGHT (not an error)") {
		t.Fatalf("retry=%+v", retry)
	}
}

func Test37SessionStartMissingDocdagExits0AndCarriesFix(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	mustWriteFile(t, filepath.Join(f.repo, ".github/workflows/adr-graph.yml"), "env:\n  DOCDAG_VERSION: v9.9.9\n")
	res := f.run([]string{"--hook"}, runOpts{
		stdin:     f.makeSessionStartPayload(""),
		pathValue: f.buildPathWithoutDocdag(),
	})
	if res.code != 0 || !strings.HasPrefix(res.stdout, "adr_preflight_enabled") ||
		!strings.Contains(strings.ToLower(res.stdout), "blocked") ||
		!strings.Contains(res.stdout, "FIX:") ||
		!strings.Contains(res.stdout, "docdag@v9.9.9") {
		t.Fatalf("res=%+v", res)
	}
}

func Test38AgyCamelCaseSessionStartAndPretoolusePayloads(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ssRaw, _ := json.Marshal(map[string]any{
		"conversationId": "agy-conv-42",
		"hookEventName":  "SessionStart",
		"cwd":            f.repo,
	})
	ssRes := f.run([]string{"--hook"}, runOpts{stdin: string(ssRaw)})
	if ssRes.code != 0 || !strings.HasPrefix(ssRes.stdout, "adr_preflight_enabled:") {
		t.Fatalf("ssRes=%+v", ssRes)
	}

	cases := []struct {
		toolName string
		args     map[string]any
	}{
		{"write_file", map[string]any{"Path": filepath.Join(f.repo, "alt-backend/app/a.go"), "Content": "x"}},
		{"edit_file", map[string]any{"Path": filepath.Join(f.repo, "tag-generator/b.py"), "Replacement": "y"}},
		{"replace_file_content", map[string]any{"TargetFile": filepath.Join(f.repo, "mq-hub/c.go"), "ReplacementContent": "z"}},
	}
	for _, tc := range cases {
		raw, _ := json.Marshal(map[string]any{
			"conversationId": "agy-conv-42",
			"hookEventName":  "PreToolUse",
			"cwd":            f.repo,
			"toolCall": map[string]any{
				"id":   "tc-1",
				"name": tc.toolName,
				"args": tc.args,
			},
		})
		first := f.run([]string{"--hook"}, runOpts{stdin: string(raw)})
		if first.code != 2 || !strings.HasPrefix(first.stderr, "ADR PREFLIGHT (not an error)") {
			t.Fatalf("%s first=%+v", tc.toolName, first)
		}
		retry := f.run([]string{"--hook"}, runOpts{stdin: string(raw)})
		if retry.code != 0 {
			t.Fatalf("%s retry=%+v", tc.toolName, retry)
		}
	}
}

func Test39CLIUsageAndXDGStateFallback(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, badArgs := range [][]string{
		nil,
		{"--unknown"},
		{"--hook", "extra"},
		{"--changed", "extra"},
		{"--service"},
		{"--service", "   "},
		{"--"},
	} {
		res := f.run(badArgs, runOpts{})
		if res.code != 2 || !strings.Contains(res.stderr, "Usage:") {
			t.Errorf("args=%v got code=%d stderr=%q", badArgs, res.code, res.stderr)
		}
	}

	// XDG_STATE_HOME fallback when ALT_ADR_PREFLIGHT_STATE_DIR is unset.
	xdgDir := filepath.Join(f.tmpRoot, "xdg_state")
	payload := f.makePretoolPayload(filepath.Join(f.repo, "alt-backend/app/x.go"), "sess-xdg", "", "")
	env := map[string]*string{
		"ALT_ADR_PREFLIGHT_STATE_DIR": nil,
		"XDG_STATE_HOME":              strPtr(xdgDir),
	}
	if res := f.run([]string{"--hook"}, runOpts{stdin: payload, envOverrides: env}); res.code != 2 {
		t.Fatalf("first xdg=%+v", res)
	}
	if res := f.run([]string{"--hook"}, runOpts{stdin: payload, envOverrides: env}); res.code != 0 {
		t.Fatalf("second xdg=%+v", res)
	}
	if _, err := os.Stat(filepath.Join(xdgDir, "alt-adr-preflight", "sess-xdg")); err != nil {
		t.Fatalf("expected receipt under XDG_STATE_HOME: %v", err)
	}
}

func Test40RealRepoServiceAltBackendContains001010(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("docdag"); err != nil {
		t.Skip("docdag binary not found on PATH")
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(repoRoot, "docdag.yaml")); err != nil {
		t.Skipf("docdag.yaml not found at %s", repoRoot)
	}
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	delete(env, "ALT_ADR_PREFLIGHT")
	env["ALT_ADR_PREFLIGHT_STATE_DIR"] = t.TempDir()

	var outBuf, errBuf bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"--service", "alt-backend"}, Options{
		Stdout:    &outBuf,
		Stderr:    &errBuf,
		Cwd:       repoRoot,
		Env:       env,
		AnchorDir: repoRoot,
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "001010") {
		t.Fatalf("stdout missing 001010:\n%s", outBuf.String())
	}
}
