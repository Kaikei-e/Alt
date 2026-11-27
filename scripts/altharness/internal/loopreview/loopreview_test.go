package loopreview_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/guards"
	"alt/scripts/altharness/internal/loopreview"
)

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs %s: %v", path, err)
	}
	root, err := os.OpenRoot(filepath.Dir(abs))
	if err != nil {
		t.Fatalf("open root %s: %v", filepath.Dir(abs), err)
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(filepath.Base(abs))
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := loopreview.Run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func makeValidFullReport() map[string]any {
	focusTitles := []struct{ id, title string }{
		{"F-01", "errorlint が書き換えた条件式"},
		{"F-02", "protogetter の nil ガード"},
		{"F-03", "auth-hub の設定ファイル読み込み"},
		{"F-04", "altctl のパス封じ込め"},
		{"F-05", "nilerr の縮退箇所"},
		{"F-06", "news-creator のゲートウェイ"},
		{"F-07", "長大関数の分割"},
	}
	verdicts := make([]map[string]any, 0, len(focusTitles))
	for i, f := range focusTitles {
		switch i {
		case 0, 1, 2, 3, 4:
			verdicts = append(verdicts, map[string]any{
				"focus_id": f.id,
				"title":    f.title,
				"status":   "safe",
				"summary":  "Verified safety assumption for " + f.id,
				"evidence": "alt-backend/app/middleware/dos_protection_middleware_test.go",
				"blast_radius_proof": map[string]any{
					"assumption": "Predicate preserved across callers for " + f.id,
					"command":    "go -C alt-backend/app test ./middleware/...",
					"exit_code":  0,
					"evidence":   "alt-backend/app/middleware/dos_protection_middleware_test.go",
				},
			})
		case 5:
			verdicts = append(verdicts, map[string]any{
				"focus_id": f.id,
				"title":    f.title,
				"status":   "fixed",
				"summary":  "Fixed QueueFullError propagation\twith\nmultiline note",
				"evidence": "news-creator/app/news_creator/gateway/ollama_gateway.py:42",
			})
		case 6:
			verdicts = append(verdicts, map[string]any{
				"focus_id": f.id,
				"title":    f.title,
				"status":   "deferred",
				"summary":  "Deferred until recap-worker differential harness completes in follow-up packet",
				"evidence": "docs/review/verify-02-differential-evidence.md",
			})
		}
	}
	return map[string]any{
		"review_id":      "LOOP04-2026-11-28",
		"base_commit":    "eb978e137",
		"head_commit":    "f3ab6fba1",
		"author_model":   "gemini-4-neon-xhigh",
		"reviewers":      []string{"gemini-4-carbon-high"},
		"auto_applied":   false,
		"focus_verdicts": verdicts,
		"findings": []map[string]any{
			{
				"id":             "REV-01",
				"focus_id":       "F-06",
				"reviewer_model": "gemini-4-carbon-high",
				"bucket":         "fix",
				"summary":        "QueueFullError swallowed\ton timeout",
				"rationale":      "Broad except masks backpressure\nreproduced by unit test",
				"evidence":       "ollama_gateway.py:118",
				"repro": map[string]any{
					"command":        "uv run pytest tests/gateway/test_ollama_gateway.py -k queue_full",
					"test_name":      "test_queue_full_not_swallowed",
					"exit_code":      1,
					"proof_evidence": ".harness/evidence/loop-04/rev-01.log",
				},
			},
			{
				"id":             "REV-02",
				"focus_id":       "F-05",
				"reviewer_model": "gemini-4-carbon-high",
				"bucket":         "investigate",
				"summary":        "Check fallback return when cache lookup fails",
				"rationale":      "Cache miss vs transport error may behave identically",
				"evidence":       "alt-backend/app/gateway/cache.go:55-68",
			},
			{
				"id":             "REV-03",
				"focus_id":       "F-03",
				"reviewer_model": "gemini-4-carbon-high",
				"bucket":         "note",
				"summary":        "Config loader enforces 0600 via fsperm.Secret",
				"rationale":      "Documented for future contributors",
				"evidence":       "auth-hub/config/config.go:19",
			},
			{
				"id":             "REV-04",
				"focus_id":       "F-01",
				"reviewer_model": "gemini-4-carbon-high",
				"bucket":         "dismiss",
				"summary":        "Suspected missing nil guard on HTTPError",
				"rationale":      "errors.As guarantees non-nil target when returning true",
				"evidence":       "alt-backend/app/middleware/dos_protection_middleware.go:88",
			},
		},
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), fsperm.Dir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, raw, fsperm.File); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

func TestNormalizeModelFamily(t *testing.T) {
	t.Parallel()
	cases := []struct {
		model string
		want  string
	}{
		{"gemini-4-neon-xhigh", "gemini-neon"},
		{"gemini-4-neon-high", "gemini-neon"},
		{"Gemini 4 Neon (XHigh)", "gemini-neon"},
		{"Gemini 4.0 Neon (XHigh)", "gemini-neon"},
		{"Gemini Neon (XHigh)", "gemini-neon"},
		{"neon-xhigh", "gemini-neon"},
		{"google/gemini-4-neon-pro", "gemini-neon"},
		{"google.gemini-4.0-neon-xhigh", "gemini-neon"},
		{"gemini-4-carbon-high", "gemini-carbon"},
		{"Gemini 4 Carbon High", "gemini-carbon"},
		{"Gemini 4.0 Carbon (High)", "gemini-carbon"},
		{"Gemini Carbon (High)", "gemini-carbon"},
		{"carbon-high", "gemini-carbon"},
		{"google.gemini-4.0-carbon-high", "gemini-carbon"},
		{"anthropic.claude-opus-4-6", "claude-opus"},
		{"us.anthropic.claude-opus-4-6-v1:0", "claude-opus"},
		{"claude-opus-5-5-high", "claude-opus"},
		{"Claude 4.6 Opus", "claude-opus"},
		{"claude-opus", "claude-opus"},
		{"claude-opus-4-1", "claude-opus"},
		{"claude-opus-4-5-20260101", "claude-opus"},
		{"anthropic.claude-sonnet-4-5", "claude-sonnet"},
		{"claude-sonnet-5-5-high", "claude-sonnet"},
		{"Claude Sonnet 4.5", "claude-sonnet"},
		{"claude-sonnet-4-5", "claude-sonnet"},
		{"claude-3-5-sonnet-20241022", "claude-sonnet"},
		{"anthropic.claude-haiku-4-5", "claude-haiku"},
		{"anthropic/claude-haiku-4-5", "claude-haiku"},
		{"Claude Haiku", "claude-haiku"},
		{"gpt-5", "gpt"},
		{"gpt-5.2", "gpt"},
		{"gpt-4o", "gpt"},
		{"gpt-4o-mini", "gpt"},
		{"openai/gpt-5", "gpt"},
		{"openai.gpt-5.2", "gpt"},
		{"openai/gpt-5-codex", "gpt"},
		{"custom-alpha-xhigh", "custom-alpha"},
		{"custom-alpha-low", "custom-alpha"},
		{"---", ""},
		{"...", ""},
		{"google/", ""},
		{"anthropic.", ""},
	}
	for _, tc := range cases {
		if got := loopreview.NormalizeModelFamily(tc.model); got != tc.want {
			t.Errorf("NormalizeModelFamily(%q) = %q, want %q", tc.model, got, tc.want)
		}
	}
}

func TestValidateValidReport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")
	writeJSON(t, reportPath, makeValidFullReport())

	code, stdout, stderr := runCLI(t, "validate", reportPath, "--require-focus-all", "--format", "json")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout=%s stderr=%s", code, stdout, stderr)
	}
	var res struct {
		Valid              bool           `json:"valid"`
		ReviewID           string         `json:"review_id"`
		AuthorFamily       string         `json:"author_family"`
		ReviewerFamilies   []string       `json:"reviewer_families"`
		FocusVerdictsCount int            `json:"focus_verdicts_count"`
		FindingsCount      int            `json:"findings_count"`
		BucketCounts       map[string]int `json:"bucket_counts"`
		Errors             []any          `json:"errors"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("unmarshal validate json: %v\nraw=%s", err, stdout)
	}
	if !res.Valid || res.ReviewID != "LOOP04-2026-11-28" || res.FocusVerdictsCount != 7 || res.FindingsCount != 4 {
		t.Fatalf("unexpected validation result: %+v", res)
	}
	if res.AuthorFamily != "gemini-neon" || len(res.ReviewerFamilies) != 1 || res.ReviewerFamilies[0] != "gemini-carbon" {
		t.Fatalf("unexpected families: author=%q reviewers=%v", res.AuthorFamily, res.ReviewerFamilies)
	}
	if res.BucketCounts["fix"] != 1 || res.BucketCounts["investigate"] != 1 || res.BucketCounts["note"] != 1 || res.BucketCounts["dismiss"] != 1 {
		t.Fatalf("unexpected bucket counts: %+v", res.BucketCounts)
	}

	// Text format should also succeed cleanly.
	code, stdout, stderr = runCLI(t, "validate", reportPath, "--require-focus-all", "--format", "text")
	if code != 0 || !strings.Contains(stdout, "OK:") {
		t.Fatalf("text format failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestValidateRejectsViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(r map[string]any)
		extraArg []string
		wantCode string
	}{
		{
			name:     "review_id path traversal CWE-22",
			mutate:   func(r map[string]any) { r["review_id"] = "../escape" },
			wantCode: "INVALID_REVIEW_ID",
		},
		{
			name:     "review_id slash",
			mutate:   func(r map[string]any) { r["review_id"] = "a/b" },
			wantCode: "INVALID_REVIEW_ID",
		},
		{
			name:     "auto_applied true forbidden",
			mutate:   func(r map[string]any) { r["auto_applied"] = true },
			wantCode: "AUTO_APPLY_FORBIDDEN",
		},
		{
			name: "same model exact match",
			mutate: func(r map[string]any) {
				r["author_model"] = "gemini-4-neon-xhigh"
				r["reviewers"] = []string{"gemini-4-neon-xhigh"}
			},
			wantCode: "SINGLE_MODEL_FAMILY_REVIEW",
		},
		{
			name: "same model sub-family neon high vs xhigh",
			mutate: func(r map[string]any) {
				r["author_model"] = "gemini-4-neon-xhigh"
				r["reviewers"] = []string{"gemini-4-neon-high"}
			},
			wantCode: "SINGLE_MODEL_FAMILY_REVIEW",
		},
		{
			name: "same gpt family",
			mutate: func(r map[string]any) {
				r["author_model"] = "gpt-4o"
				r["reviewers"] = []string{"gpt-5-codex"}
			},
			wantCode: "SINGLE_MODEL_FAMILY_REVIEW",
		},
		{
			name: "empty reviewers",
			mutate: func(r map[string]any) {
				r["reviewers"] = []string{}
			},
			wantCode: "SINGLE_MODEL_FAMILY_REVIEW",
		},
		{
			name: "invalid finding bucket",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				findings[1]["bucket"] = "wontfix"
			},
			wantCode: "INVALID_BUCKET",
		},
		{
			name: "placeholder evidence TBD",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				findings[1]["evidence"] = "TBD"
			},
			wantCode: "INVALID_EVIDENCE",
		},
		{
			name: "fix finding missing repro",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				delete(findings[0], "repro")
			},
			wantCode: "FIX_MISSING_REPRO_PROOF",
		},
		{
			name: "fix finding repro exit_code zero",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				repro := findings[0]["repro"].(map[string]any)
				repro["exit_code"] = 0
			},
			wantCode: "FIX_MISSING_REPRO_PROOF",
		},
		{
			name: "fix finding repro omitted exit_code",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				repro := findings[0]["repro"].(map[string]any)
				delete(repro, "exit_code")
			},
			wantCode: "FIX_MISSING_REPRO_PROOF",
		},
		{
			name: "fix finding repro missing command and test_name",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				repro := findings[0]["repro"].(map[string]any)
				repro["command"] = ""
				repro["test_name"] = ""
			},
			wantCode: "FIX_MISSING_REPRO_PROOF",
		},
		{
			name: "fix finding repro placeholder proof_evidence",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				repro := findings[0]["repro"].(map[string]any)
				repro["proof_evidence"] = "N/A"
			},
			wantCode: "FIX_MISSING_REPRO_PROOF",
		},
		{
			name: "safe verdict missing blast_radius_proof",
			mutate: func(r map[string]any) {
				verdicts := r["focus_verdicts"].([]map[string]any)
				delete(verdicts[0], "blast_radius_proof")
			},
			wantCode: "UNPROVEN_SAFE_CLAIM",
		},
		{
			name: "safe verdict blast_radius_proof omitted exit_code fail-closed",
			mutate: func(r map[string]any) {
				verdicts := r["focus_verdicts"].([]map[string]any)
				brp := verdicts[0]["blast_radius_proof"].(map[string]any)
				delete(brp, "exit_code")
			},
			wantCode: "UNPROVEN_SAFE_CLAIM",
		},
		{
			name: "safe verdict blast_radius_proof non-zero exit_code",
			mutate: func(r map[string]any) {
				verdicts := r["focus_verdicts"].([]map[string]any)
				brp := verdicts[0]["blast_radius_proof"].(map[string]any)
				brp["exit_code"] = 1
			},
			wantCode: "UNPROVEN_SAFE_CLAIM",
		},
		{
			name: "deferred verdict placeholder rationale",
			mutate: func(r map[string]any) {
				verdicts := r["focus_verdicts"].([]map[string]any)
				verdicts[6]["summary"] = "-"
			},
			wantCode: "DEFERRED_MISSING_RATIONALE",
		},
		{
			name: "require-focus-all rejects missing focus item",
			mutate: func(r map[string]any) {
				verdicts := r["focus_verdicts"].([]map[string]any)
				r["focus_verdicts"] = verdicts[:6]
			},
			extraArg: []string{"--require-focus-all"},
			wantCode: "MISSING_FOCUS_ITEM",
		},
		{
			name: "duplicate finding IDs",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				findings[2]["id"] = findings[0]["id"]
			},
			wantCode: "DUPLICATE_FINDING_ID",
		},
		{
			name: "duplicate focus IDs",
			mutate: func(r map[string]any) {
				verdicts := r["focus_verdicts"].([]map[string]any)
				verdicts[1]["focus_id"] = verdicts[0]["focus_id"]
			},
			wantCode: "DUPLICATE_FOCUS_ID",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			report := makeValidFullReport()
			tc.mutate(report)
			reportPath := filepath.Join(dir, "report.json")
			writeJSON(t, reportPath, report)

			args := append([]string{"validate", reportPath, "--format", "json"}, tc.extraArg...)
			code, stdout, stderr := runCLI(t, args...)
			if code != 1 {
				t.Fatalf("expected exit 1, got %d; stdout=%s stderr=%s", code, stdout, stderr)
			}
			var res struct {
				Valid  bool               `json:"valid"`
				Errors []loopreview.Issue `json:"errors"`
			}
			if err := json.Unmarshal([]byte(stdout), &res); err != nil {
				t.Fatalf("unmarshal validate error json: %v\nstdout=%s", err, stdout)
			}
			if res.Valid {
				t.Fatalf("expected valid=false")
			}
			found := false
			for _, iss := range res.Errors {
				if iss.Code == tc.wantCode {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected error code %q in %+v", tc.wantCode, res.Errors)
			}
		})
	}
}

func TestValidateAcceptsMixedReviewersWithOneDistinctFamily(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	report := makeValidFullReport()
	report["author_model"] = "gemini-4-neon-xhigh"
	report["reviewers"] = []string{"gemini-4-neon-high", "claude-opus-4-1"}
	for _, f := range report["findings"].([]map[string]any) {
		f["reviewer_model"] = "claude-opus-4-1"
	}
	reportPath := filepath.Join(dir, "report.json")
	writeJSON(t, reportPath, report)

	code, stdout, stderr := runCLI(t, "validate", reportPath, "--format", "json")
	if code != 0 {
		t.Fatalf("expected exit 0 when at least one reviewer is distinct family, got %d; stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestBriefWithGitRepoCorpusScreenerAndOutSafety(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	// Create minimal CORPUS-01 entries.
	c1Dir := filepath.Join(repo, "scripts", "harness", "corpus", "C-01")
	if err := os.MkdirAll(c1Dir, fsperm.Dir); err != nil {
		t.Fatalf("mkdir corpus: %v", err)
	}
	metaToml := `id = "C-01"
title = "errorlint autofix drops RHS of && condition"
category = "autofix-logic-corruption"
language = "go"
`
	if err := os.WriteFile(filepath.Join(c1Dir, "meta.toml"), []byte(metaToml), fsperm.File); err != nil {
		t.Fatalf("write meta.toml: %v", err)
	}

	// Initialize git repo and commit a Go file, then introduce a CHECK-01 condition change.
	runGit := func(cmd *exec.Cmd) {
		t.Helper()
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=tester",
			"GIT_AUTHOR_EMAIL=tester@example.com",
			"GIT_COMMITTER_NAME=tester",
			"GIT_COMMITTER_EMAIL=tester@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", cmd.Args, err, string(out))
		}
	}
	ctx := context.Background()
	runGit(exec.CommandContext(ctx, "git", "init", "-b", "feat/test-loop04"))
	goFile := filepath.Join(repo, "pkg", "check.go")
	if err := os.MkdirAll(filepath.Dir(goFile), fsperm.Dir); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	beforeSrc := `package pkg

import "errors"

type HTTPError struct{ Code int }

func (e *HTTPError) Error() string { return "http" }

func ShouldRetry(err error) bool {
	var he *HTTPError
	if errors.As(err, &he) && he.Code >= 500 {
		return true
	}
	return false
}
`
	if err := os.WriteFile(goFile, []byte(beforeSrc), fsperm.File); err != nil {
		t.Fatalf("write before go: %v", err)
	}
	runGit(exec.CommandContext(ctx, "git", "add", "."))
	runGit(exec.CommandContext(ctx, "git", "commit", "-m", "initial commit"))

	afterSrc := `package pkg

import "errors"

type HTTPError struct{ Code int }

func (e *HTTPError) Error() string { return "http" }

func ShouldRetry(err error) bool {
	var he *HTTPError
	if errors.As(err, &he) {
		return true
	}
	return false
}
`
	if err := os.WriteFile(goFile, []byte(afterSrc), fsperm.File); err != nil {
		t.Fatalf("write after go: %v", err)
	}

	// JSON brief with --base HEAD
	code, stdout, stderr := runCLI(t, "brief", "--repo-root", repo, "--base", "HEAD", "--format", "json")
	if code != 0 {
		t.Fatalf("brief json failed: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var brief loopreview.Brief
	if err := json.Unmarshal([]byte(stdout), &brief); err != nil {
		t.Fatalf("unmarshal brief json: %v\nraw=%s", err, stdout)
	}
	if len(brief.FocusAreas) != 7 {
		t.Fatalf("expected 7 canonical focus areas, got %d", len(brief.FocusAreas))
	}
	if brief.FocusAreas[0].ID != "F-01" || brief.FocusAreas[0].Title != "errorlint が書き換えた条件式" {
		t.Fatalf("unexpected F-01: %+v", brief.FocusAreas[0])
	}
	if brief.FocusAreas[6].ID != "F-07" || brief.FocusAreas[6].Title != "長大関数の分割" {
		t.Fatalf("unexpected F-07: %+v", brief.FocusAreas[6])
	}
	if len(brief.CorpusSummary) != 1 || brief.CorpusSummary[0].ID != "C-01" {
		t.Fatalf("expected C-01 corpus summary, got %+v", brief.CorpusSummary)
	}
	if len(brief.ScreenerCandidates) == 0 {
		t.Fatalf("expected CHECK-01 screener candidates for dropped && he.Code >= 500, got 0")
	}
	if brief.Diff == nil || len(brief.Diff.Files) != 1 || brief.Diff.Files[0].Path != "pkg/check.go" {
		t.Fatalf("expected diff file stats for pkg/check.go, got %+v", brief.Diff)
	}
	// Must NEVER dump raw diff hunks.
	if strings.Contains(stdout, "@@ -") || strings.Contains(stdout, "ShouldRetry") && strings.Contains(stdout, "+func ShouldRetry") {
		t.Fatalf("brief must not dump raw diff hunks: %s", stdout)
	}

	// Markdown brief written via --out
	outPath := filepath.Join(t.TempDir(), "brief.md")
	code, _, stderr = runCLI(t, "brief", "--repo-root", repo, "--base", "HEAD", "--format", "markdown", "--out", outPath)
	if code != 0 {
		t.Fatalf("brief markdown --out failed: code=%d stderr=%s", code, stderr)
	}
	md := string(readTestFile(t, outPath))
	for _, want := range []string{"F-01", "errorlint が書き換えた条件式", "F-07", "長大関数の分割", "C-01", "pkg/check.go"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown brief missing %q", want)
		}
	}

	// Symlink --out MUST be rejected without overwriting target!
	outside := filepath.Join(t.TempDir(), "protected.txt")
	if err := os.WriteFile(outside, []byte("untouched"), fsperm.File); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	linkOut := filepath.Join(t.TempDir(), "evil-link.md")
	if err := os.Symlink(outside, linkOut); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	code, _, _ = runCLI(t, "brief", "--repo-root", repo, "--out", linkOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit when --out is a symlink")
	}
	afterOutside := readTestFile(t, outside)
	if string(afterOutside) != "untouched" {
		t.Fatalf("symlink target was modified: %q", string(afterOutside))
	}

	// Reject flag-like --base revision (git option injection).
	code, _, _ = runCLI(t, "brief", "--repo-root", repo, "--base", "--evil-flag")
	if code != 2 {
		t.Fatalf("expected exit 2 on flag-like --base, got %d", code)
	}

	// Custom --focus-file
	customFocusPath := filepath.Join(t.TempDir(), "focus.json")
	writeJSON(t, customFocusPath, []map[string]any{
		{"id": "X-01", "title": "Custom audit check"},
	})
	code, stdout, stderr = runCLI(t, "brief", "--repo-root", repo, "--focus-file", customFocusPath, "--format", "json")
	if code != 0 {
		t.Fatalf("brief --focus-file failed: code=%d stderr=%s", code, stderr)
	}
	var customBrief loopreview.Brief
	if err := json.Unmarshal([]byte(stdout), &customBrief); err != nil {
		t.Fatalf("unmarshal custom brief: %v", err)
	}
	if len(customBrief.FocusAreas) != 1 || customBrief.FocusAreas[0].ID != "X-01" {
		t.Fatalf("expected custom focus X-01, got %+v", customBrief.FocusAreas)
	}
}

func TestRecordFailClosedAndCoverage(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	runsDir := filepath.Join(t.TempDir(), "runs")

	// 1. Fail-closed: invalid report MUST NOT write anything.
	badReport := makeValidFullReport()
	badReport["auto_applied"] = true
	badPath := filepath.Join(t.TempDir(), "bad.json")
	writeJSON(t, badPath, badReport)

	code, _, _ := runCLI(t, "record", badPath, "--repo-root", repo, "--runs-dir", runsDir, "--branch", "feat/demo")
	if code != 1 {
		t.Fatalf("expected exit 1 for invalid report on record, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(runsDir, "feat-demo")); !os.IsNotExist(err) {
		t.Fatalf("invalid report must not create run directory or files")
	}

	// 2. Valid full report -> saves copy + appends 1 row/finding + 1 row/verdict cleanly.
	goodReport := makeValidFullReport()
	goodPath := filepath.Join(t.TempDir(), "good.json")
	writeJSON(t, goodPath, goodReport)

	code, stdout, stderr := runCLI(t, "record", goodPath, "--repo-root", repo, "--runs-dir", runsDir, "--branch", "feat/demo", "--require-focus-all", "--format", "json")
	if code != 0 {
		t.Fatalf("record valid report failed: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}

	savedReport := filepath.Join(runsDir, "feat-demo", "reviews", "LOOP04-2026-11-28.json")
	if st, err := os.Lstat(savedReport); err != nil || !st.Mode().IsRegular() {
		t.Fatalf("expected saved report at %s: err=%v", savedReport, err)
	}

	decisionsPath := filepath.Join(runsDir, "feat-demo", "decisions.tsv")
	rawTSV := readTestFile(t, decisionsPath)
	lines := strings.Split(strings.TrimSpace(string(rawTSV)), "\n")
	// 1 header + 1 init + 4 findings + 7 focus verdicts = 13 lines
	if len(lines) < 12 {
		t.Fatalf("expected at least 12 lines in decisions.tsv, got %d:\n%s", len(lines), string(rawTSV))
	}
	if _, findings, err := guards.ValidateDecisionsFile(decisionsPath); err != nil || len(findings) != 0 {
		t.Fatalf("recorded decisions.tsv failed ValidateDecisionsFile: err=%v findings=%+v\n%s", err, findings, string(rawTSV))
	}
	initCount := 0
	interrogateCount := 0
	loop04Count := 0
	for i, line := range lines[1:] {
		cols := strings.Split(line, "\t")
		if len(cols) != 6 {
			t.Fatalf("row %d has %d columns (want 6): %q", i+1, len(cols), line)
		}
		if cols[5] != "info" {
			t.Errorf("row %d result=%q, want info", i+1, cols[5])
		}
		switch cols[1] {
		case "init":
			initCount++
		case "interrogate":
			interrogateCount++
			if !strings.HasPrefix(cols[2], "[F-") {
				t.Errorf("interrogate decision unexpected: %q", cols[2])
			}
			if !strings.Contains(cols[3], "(reviewer=gemini-4-carbon-high)") {
				t.Errorf("interrogate why missing reviewer tag: %q", cols[3])
			}
		case "loop-04":
			loop04Count++
			if !strings.HasPrefix(cols[2], "focus F-") {
				t.Errorf("loop-04 decision unexpected: %q", cols[2])
			}
		default:
			t.Errorf("unexpected phase %q in row %d", cols[1], i+1)
		}
	}
	if initCount != 1 || interrogateCount != 4 || loop04Count != 7 {
		t.Fatalf("expected 1 init, 4 interrogate, 7 loop-04 rows; got init=%d interrogate=%d loop-04=%d", initCount, interrogateCount, loop04Count)
	}

	// 3. check-coverage passes (7/7)
	code, stdout, stderr = runCLI(t, "check-coverage", "--repo-root", repo, "--runs-dir", runsDir, "--branch", "feat/demo", "--format", "json")
	if code != 0 {
		t.Fatalf("check-coverage expected 0, got %d; stdout=%s stderr=%s", code, stdout, stderr)
	}
	var cov loopreview.CoverageResult
	if err := json.Unmarshal([]byte(stdout), &cov); err != nil {
		t.Fatalf("unmarshal coverage json: %v", err)
	}
	if !cov.Covered || len(cov.CoveredIDs) != 7 || len(cov.MissingIDs) != 0 {
		t.Fatalf("unexpected coverage result: %+v", cov)
	}
}

func TestCheckCoveragePartialAndMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	decisions := filepath.Join(dir, "decisions.tsv")

	// Missing file -> exit 1 with all 7 missing.
	code, stdout, _ := runCLI(t, "check-coverage", "--decisions-file", decisions, "--format", "json")
	if code != 1 {
		t.Fatalf("expected exit 1 on missing decisions file, got %d", code)
	}
	var cov loopreview.CoverageResult
	if err := json.Unmarshal([]byte(stdout), &cov); err != nil {
		t.Fatalf("unmarshal missing coverage: %v\nstdout=%s", err, stdout)
	}
	if cov.Covered || len(cov.MissingIDs) != 7 {
		t.Fatalf("expected 7 missing focus IDs, got %+v", cov)
	}

	// Partial coverage (only F-01 and F-02; F-03 only in interrogate phase must NOT count as loop-04 verdict!)
	tsv := strings.Join([]string{
		"ts\tphase\tdecision\twhy\tevidence\tresult",
		"2026-11-28T00:00:00Z\tinit\tpolicy=fix-existing; goal=loop04 test; completion=go test ./...\tinit run\tdocs/ADR/001039.md\tinfo",
		"2026-11-28T00:00:01Z\tloop-04\tfocus F-01 (safe): errorlint が書き換えた条件式\tverified\tfoo_test.go\tinfo",
		"2026-11-28T00:00:02Z\tloop-04\tfocus F-02 (fixed): protogetter の nil ガード\tfixed\tbar_test.go\tinfo",
		"2026-11-28T00:00:03Z\tinterrogate\t[F-03/note] REV-1: note only\tjust a note\tbaz.go\tinfo",
	}, "\n") + "\n"
	if err := os.WriteFile(decisions, []byte(tsv), fsperm.File); err != nil {
		t.Fatalf("write tsv: %v", err)
	}

	code, stdout, stderr := runCLI(t, "check-coverage", "--decisions-file", decisions, "--format", "json")
	if code != 1 {
		t.Fatalf("expected exit 1 on partial coverage, got %d; stdout=%s stderr=%s", code, stdout, stderr)
	}
	cov = loopreview.CoverageResult{}
	if err := json.Unmarshal([]byte(stdout), &cov); err != nil {
		t.Fatalf("unmarshal partial coverage: %v", err)
	}
	if cov.Covered || len(cov.CoveredIDs) != 2 || len(cov.MissingIDs) != 5 {
		t.Fatalf("expected 2 covered and 5 missing (F-03..F-07), got %+v", cov)
	}
	if cov.MissingIDs[0] != "F-03" || cov.MissingIDs[4] != "F-07" {
		t.Fatalf("unexpected missing IDs: %v", cov.MissingIDs)
	}
}

func TestRecordRejectsSymlinkRunsDirAndDecisionsFile(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	outside := t.TempDir()
	runsDir := filepath.Join(t.TempDir(), "runs")
	if err := os.MkdirAll(runsDir, fsperm.Dir); err != nil {
		t.Fatalf("mkdir runsDir: %v", err)
	}
	// Symlink branch directory inside runsDir -> outside.
	if err := os.Symlink(outside, filepath.Join(runsDir, "feat-evil")); err != nil {
		t.Fatalf("symlink branch: %v", err)
	}
	reportPath := filepath.Join(t.TempDir(), "report.json")
	writeJSON(t, reportPath, makeValidFullReport())

	code, _, stderr := runCLI(t, "record", reportPath, "--repo-root", repo, "--runs-dir", runsDir, "--branch", "feat/evil")
	if code == 0 {
		t.Fatalf("expected failure when branch runDir is a symlink, stderr=%s", stderr)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("symlink target dir should stay empty, got %d entries", len(entries))
	}

	// Symlinked --decisions-file should also be rejected cleanly.
	outsideFile := filepath.Join(outside, "victim.tsv")
	if err := os.WriteFile(outsideFile, []byte("keep"), fsperm.File); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	linkTSV := filepath.Join(t.TempDir(), "decisions.tsv")
	if err := os.Symlink(outsideFile, linkTSV); err != nil {
		t.Fatalf("symlink decisions: %v", err)
	}
	cleanRuns := filepath.Join(t.TempDir(), "runs-clean")
	code, _, _ = runCLI(t, "record", reportPath, "--repo-root", repo, "--runs-dir", cleanRuns, "--branch", "feat/clean", "--decisions-file", linkTSV)
	if code == 0 {
		t.Fatalf("expected failure when --decisions-file is a symlink")
	}
	raw := readTestFile(t, outsideFile)
	if string(raw) != "keep" {
		t.Fatalf("symlinked decisions target was modified: %q", string(raw))
	}
}

func TestBriefRealRepoCorpusSummary(t *testing.T) {
	t.Parallel()
	// Locate actual repository root (4 levels up from scripts/altharness/internal/loopreview).
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(repoRoot, "scripts", "harness", "corpus", "C-01", "meta.toml")); err != nil {
		t.Skipf("real corpus not found at %s: %v", repoRoot, err)
	}
	code, stdout, stderr := runCLI(t, "brief", "--repo-root", repoRoot, "--format", "json")
	if code != 0 {
		t.Fatalf("brief against real repo failed: code=%d stderr=%s", code, stderr)
	}
	var brief loopreview.Brief
	if err := json.Unmarshal([]byte(stdout), &brief); err != nil {
		t.Fatalf("unmarshal real brief: %v", err)
	}
	if len(brief.CorpusSummary) < 22 {
		t.Fatalf("expected >= 22 CORPUS-01 entries, got %d", len(brief.CorpusSummary))
	}
	if brief.CorpusSummary[0].ID != "C-01" || brief.CorpusSummary[0].Language != "go" {
		t.Fatalf("unexpected first corpus entry: %+v", brief.CorpusSummary[0])
	}
}

func TestCLIErrorAndEdgeCases(t *testing.T) {
	t.Parallel()

	// Help and unknown subcommand.
	if code, out, _ := runCLI(t, "--help"); code != 0 || !strings.Contains(out, "check-coverage") {
		t.Fatalf("expected help exit 0, got %d out=%q", code, out)
	}
	if code, _, _ := runCLI(t); code != 2 {
		t.Fatalf("expected exit 2 on missing subcommand, got %d", code)
	}
	if code, _, _ := runCLI(t, "unknown"); code != 2 {
		t.Fatalf("expected exit 2 on unknown subcommand, got %d", code)
	}

	// Malformed JSON -> exit 1 with PARSE_ERROR.
	dir := t.TempDir()
	badJSON := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badJSON, []byte("{not json"), fsperm.File); err != nil {
		t.Fatalf("write bad json: %v", err)
	}
	code, stdout, _ := runCLI(t, "validate", badJSON, "--format", "json")
	if code != 1 || !strings.Contains(stdout, "PARSE_ERROR") {
		t.Fatalf("expected PARSE_ERROR exit 1, got code=%d stdout=%s", code, stdout)
	}

	// Fix finding with ONLY test_name (no command) should PASS.
	rep := makeValidFullReport()
	findings := rep["findings"].([]map[string]any)
	repro := findings[0]["repro"].(map[string]any)
	repro["command"] = ""
	repro["test_name"] = "TestQueueFullPropagates"
	onlyTestPath := filepath.Join(dir, "only-test.json")
	writeJSON(t, onlyTestPath, rep)
	if code, stdout, stderr := runCLI(t, "validate", onlyTestPath, "--require-focus-all"); code != 0 {
		t.Fatalf("expected fix finding with test_name-only repro to pass, got %d stdout=%s stderr=%s", code, stdout, stderr)
	}

	// Symlink report file MUST be rejected.
	linkRep := filepath.Join(dir, "link.json")
	if err := os.Symlink(onlyTestPath, linkRep); err != nil {
		t.Fatalf("symlink report: %v", err)
	}
	if code, _, _ := runCLI(t, "validate", linkRep); code != 2 {
		t.Fatalf("expected exit 2 for symlink report, got %d", code)
	}

	// Text output on partial check-coverage lists missing items cleanly.
	tsv := filepath.Join(dir, "partial.tsv")
	if err := os.WriteFile(tsv, []byte("ts\tphase\tdecision\twhy\tevidence\tresult\n"), fsperm.File); err != nil {
		t.Fatalf("write partial.tsv: %v", err)
	}
	code, _, stderr := runCLI(t, "check-coverage", "--decisions-file", tsv, "--format", "text")
	if code != 1 || !strings.Contains(stderr, "F-01") || !strings.Contains(stderr, "F-07") {
		t.Fatalf("expected text coverage failure listing F-01..F-07, got code=%d stderr=%q", code, stderr)
	}
}

func TestValidateCrossFamilyBypassProbes(t *testing.T) {
	t.Parallel()
	pairs := []struct {
		name     string
		author   string
		reviewer string
	}{
		{"neon version decimal", "gemini-4-neon-xhigh", "Gemini 4.0 Neon (XHigh)"},
		{"neon no version", "gemini-4-neon-xhigh", "Gemini Neon (XHigh)"},
		{"neon bare shorthand", "gemini-4-neon-xhigh", "neon-xhigh"},
		{"neon google dot prefix", "gemini-4-neon-xhigh", "google.gemini-4.0-neon-high"},
		{"carbon version decimal", "gemini-4-carbon-high", "Gemini 4.0 Carbon (High)"},
		{"carbon bare shorthand", "gemini-4-carbon-high", "carbon-high"},
		{"opus bedrock dot prefix vs version", "anthropic.claude-opus-4-6", "claude-opus-5-5-high"},
		{"opus bedrock vs display name", "anthropic.claude-opus-4-6", "Claude 4.6 Opus"},
		{"opus bedrock vs bare", "anthropic.claude-opus-4-6", "claude-opus"},
		{"sonnet bedrock vs version", "anthropic.claude-sonnet-4-5", "claude-sonnet-5-5-high"},
		{"sonnet bedrock vs display name", "anthropic.claude-sonnet-4-5", "Claude Sonnet 4.5"},
		{"haiku bedrock vs display name", "anthropic.claude-haiku-4-5", "Claude Haiku"},
		{"gpt version vs slash provider", "gpt-5.2", "openai/gpt-5"},
		{"gpt-4o vs gpt-5.2", "gpt-4o", "gpt-5.2"},
	}

	for _, tc := range pairs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			report := makeValidFullReport()
			report["author_model"] = tc.author
			report["reviewers"] = []string{tc.reviewer}
			for _, f := range report["findings"].([]map[string]any) {
				f["reviewer_model"] = tc.reviewer
			}
			reportPath := filepath.Join(dir, "report.json")
			writeJSON(t, reportPath, report)

			code, stdout, stderr := runCLI(t, "validate", reportPath, "--format", "json")
			if code != 1 {
				t.Fatalf("expected exit 1 for same-family (%q vs %q), got %d; stdout=%s stderr=%s", tc.author, tc.reviewer, code, stdout, stderr)
			}
			var res struct {
				Valid  bool               `json:"valid"`
				Errors []loopreview.Issue `json:"errors"`
			}
			if err := json.Unmarshal([]byte(stdout), &res); err != nil {
				t.Fatalf("unmarshal validate json: %v\nstdout=%s", err, stdout)
			}
			found := false
			for _, iss := range res.Errors {
				if iss.Code == "SINGLE_MODEL_FAMILY_REVIEW" {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected SINGLE_MODEL_FAMILY_REVIEW for %q vs %q, got %+v", tc.author, tc.reviewer, res.Errors)
			}
		})
	}
}

func TestValidateAuditProbesSpec05Sec05Sec02(t *testing.T) {
	t.Parallel()

	t.Run("SPEC-05 combined probe rejects all three violations", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		raw := `{"review_id":"R1","author_model":"gemini-4-neon-xhigh","reviewers":["Gemini 4.0 Neon (XHigh)"],"auto_applied":false,"focus_verdicts":[{"focus_id":"F-01","title":"t","status":"safe","summary":"s","evidence":"ADR-001039","blast_radius_proof":{"assumption":"ok predicate","command":"true","exit_code":0,"evidence":"ADR-001039"}}],"findings":[{"id":"F1","focus_id":"F-01","reviewer_model":"---","bucket":"fix","summary":"bug","rationale":"why","evidence":"ADR-001039","repro":{"command":"false","exit_code":1,"proof_evidence":"ADR-001039"}}]}`
		reportPath := filepath.Join(dir, "spec05.json")
		if err := os.WriteFile(reportPath, []byte(raw), fsperm.File); err != nil {
			t.Fatalf("write spec05: %v", err)
		}
		code, stdout, stderr := runCLI(t, "validate", reportPath, "--format", "json")
		if code != 1 {
			t.Fatalf("SPEC-05 probe must fail with exit 1, got %d; stdout=%s stderr=%s", code, stdout, stderr)
		}
		var res struct {
			Errors []loopreview.Issue `json:"errors"`
		}
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		codes := map[string]bool{}
		for _, iss := range res.Errors {
			codes[iss.Code] = true
		}
		for _, want := range []string{"SINGLE_MODEL_FAMILY_REVIEW", "EMPTY_REVIEWER_MODEL", "CONTRADICTORY_SAFE_VERDICT"} {
			if !codes[want] {
				t.Errorf("expected error code %q in %+v", want, res.Errors)
			}
		}
	})

	t.Run("SEC-05 combined probe rejects same-family and self-review finding", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		raw := `{"review_id":"R1","author_model":"gemini-4-neon-xhigh","reviewers":["Gemini Neon (XHigh)"],"auto_applied":false,"focus_verdicts":[],"findings":[{"id":"F1","focus_id":"F-01","reviewer_model":"gemini-4-neon-xhigh","bucket":"note","summary":"s","rationale":"r","evidence":"C-01"}]}`
		reportPath := filepath.Join(dir, "sec05.json")
		if err := os.WriteFile(reportPath, []byte(raw), fsperm.File); err != nil {
			t.Fatalf("write sec05: %v", err)
		}
		code, stdout, stderr := runCLI(t, "validate", reportPath, "--format", "json")
		if code != 1 {
			t.Fatalf("SEC-05 probe must fail with exit 1, got %d; stdout=%s stderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "SINGLE_MODEL_FAMILY_REVIEW") {
			t.Fatalf("expected SINGLE_MODEL_FAMILY_REVIEW in %s", stdout)
		}
	})

	cases := []struct {
		name     string
		mutate   func(r map[string]any)
		wantCode string
	}{
		{
			name: "finding reviewer_model dashes rejected",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				findings[1]["reviewer_model"] = "---"
			},
			wantCode: "EMPTY_REVIEWER_MODEL",
		},
		{
			name: "finding reviewer_model undeclared in reviewers",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				findings[1]["reviewer_model"] = "claude-opus-4-6"
			},
			wantCode: "UNDECLARED_FINDING_REVIEWER",
		},
		{
			name: "author self-review finding sneaking alongside rubber-stamp reviewer",
			mutate: func(r map[string]any) {
				r["author_model"] = "gemini-4-neon-xhigh"
				r["reviewers"] = []string{"gemini-4-neon-high", "gemini-4-carbon-high"}
				findings := r["findings"].([]map[string]any)
				findings[1]["reviewer_model"] = "gemini-4-neon-high"
			},
			wantCode: "SINGLE_MODEL_FAMILY_REVIEW",
		},
		{
			name: "contradictory safe verdict with fix finding on same focus_id",
			mutate: func(r map[string]any) {
				verdicts := r["focus_verdicts"].([]map[string]any)
				// F-06 has a bucket="fix" finding (REV-01); flipping F-06 to safe must fail.
				verdicts[5]["status"] = "safe"
				verdicts[5]["blast_radius_proof"] = map[string]any{
					"assumption": "safe despite open fix",
					"command":    "true",
					"exit_code":  0,
					"evidence":   "ADR-001039",
				}
			},
			wantCode: "CONTRADICTORY_SAFE_VERDICT",
		},
		{
			name: "vacuous report with zero verdicts and zero findings",
			mutate: func(r map[string]any) {
				r["focus_verdicts"] = []map[string]any{}
				r["findings"] = []map[string]any{}
			},
			wantCode: "EMPTY_REVIEW_REPORT",
		},
		{
			name: "SEC-02 evidence with embedded tab in cmd",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				findings[1]["evidence"] = "cmd:go test\t./..."
			},
			wantCode: "INVALID_EVIDENCE",
		},
		{
			name: "SEC-02 evidence with CR/LF",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				findings[1]["evidence"] = "cmd:go test\r\n./..."
			},
			wantCode: "INVALID_EVIDENCE",
		},
		{
			name: "SEC-02 evidence formula =cmd",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				findings[1]["evidence"] = "=cmd|'/C_calc'!A0"
			},
			wantCode: "INVALID_EVIDENCE",
		},
		{
			name: "SEC-02 evidence formula =HYPERLINK",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				findings[1]["evidence"] = `=HYPERLINK("http://evil.example/pwn","click")`
			},
			wantCode: "INVALID_EVIDENCE",
		},
		{
			name: "SEC-02 evidence formula leading +",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				findings[1]["evidence"] = "+1+1/evil.go"
			},
			wantCode: "INVALID_EVIDENCE",
		},
		{
			name: "SEC-02 evidence formula leading @",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				findings[1]["evidence"] = "@SUM(1,2)/evil.go"
			},
			wantCode: "INVALID_EVIDENCE",
		},
		{
			name: "SEC-02 evidence formula leading -[0-9=+@(]",
			mutate: func(r map[string]any) {
				findings := r["findings"].([]map[string]any)
				findings[1]["evidence"] = "-1+1/evil.go"
			},
			wantCode: "INVALID_EVIDENCE",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			report := makeValidFullReport()
			tc.mutate(report)
			reportPath := filepath.Join(dir, "report.json")
			writeJSON(t, reportPath, report)

			code, stdout, stderr := runCLI(t, "validate", reportPath, "--format", "json")
			if code != 1 {
				t.Fatalf("expected exit 1, got %d; stdout=%s stderr=%s", code, stdout, stderr)
			}
			var res struct {
				Errors []loopreview.Issue `json:"errors"`
			}
			if err := json.Unmarshal([]byte(stdout), &res); err != nil {
				t.Fatalf("unmarshal: %v\nstdout=%s", err, stdout)
			}
			found := false
			for _, iss := range res.Errors {
				if iss.Code == tc.wantCode {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected error code %q in %+v", tc.wantCode, res.Errors)
			}
		})
	}
}

func TestRecordNormalizesIDLineEvidenceAndRoundTripsValidate(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	runsDir := filepath.Join(t.TempDir(), "runs")

	report := makeValidFullReport()
	verdicts := report["focus_verdicts"].([]map[string]any)
	verdicts[0]["evidence"] = "ADR-001039:42"
	verdicts[1]["evidence"] = "PM-2026-045:12-20"
	verdicts[2]["evidence"] = "C-01:5"
	findings := report["findings"].([]map[string]any)
	findings[1]["evidence"] = "ADR-001039#L99"
	findings[2]["evidence"] = "PM-2026-045:7"
	findings[3]["evidence"] = "C-11:3"

	reportPath := filepath.Join(t.TempDir(), "idline.json")
	writeJSON(t, reportPath, report)

	code, stdout, stderr := runCLI(t, "record", reportPath, "--repo-root", repo, "--runs-dir", runsDir, "--branch", "feat/idline", "--require-focus-all", "--format", "json")
	if code != 0 {
		t.Fatalf("record failed: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}

	decisionsPath := filepath.Join(runsDir, "feat-idline", "decisions.tsv")
	raw := string(readTestFile(t, decisionsPath))
	for _, forbidden := range []string{"./ADR-001039", "./PM-2026-045", "./C-01", "./C-11"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("decisions.tsv must NEVER prefix ID ref with ./, found %q in:\n%s", forbidden, raw)
		}
	}
	for _, expected := range []string{"\tADR-001039\t", "\tPM-2026-045\t", "\tC-01\t", "\tC-11\t", "\t./ollama_gateway.py:118\t"} {
		if !strings.Contains(raw, expected) {
			t.Errorf("decisions.tsv missing normalized evidence %q in:\n%s", expected, raw)
		}
	}

	if _, valFindings, err := guards.ValidateDecisionsFile(decisionsPath); err != nil || len(valFindings) != 0 {
		t.Fatalf("decisions.tsv failed ValidateDecisionsFile: err=%v findings=%+v\n%s", err, valFindings, raw)
	}
}

func TestCheckCoverageHardened(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// 1. Malformed 3-column TSV mentioning F-01..F-07 MUST fail closed.
	malformed := filepath.Join(dir, "malformed.tsv")
	malformedContent := strings.Join([]string{
		"ts\tphase\tdecision",
		"2026-11-28T00:00:00Z\tloop-04\tfocus F-01 (safe): a",
		"2026-11-28T00:00:01Z\tloop-04\tfocus F-02 (safe): b",
		"2026-11-28T00:00:02Z\tloop-04\tfocus F-03 (safe): c",
		"2026-11-28T00:00:03Z\tloop-04\tfocus F-04 (safe): d",
		"2026-11-28T00:00:04Z\tloop-04\tfocus F-05 (safe): e",
		"2026-11-28T00:00:05Z\tloop-04\tfocus F-06 (safe): f",
		"2026-11-28T00:00:06Z\tloop-04\tfocus F-07 (safe): g",
	}, "\n") + "\n"
	if err := os.WriteFile(malformed, []byte(malformedContent), fsperm.File); err != nil {
		t.Fatalf("write malformed: %v", err)
	}
	code, stdout, _ := runCLI(t, "check-coverage", "--decisions-file", malformed, "--format", "json")
	if code != 1 {
		t.Fatalf("malformed 3-column TSV must fail closed, got exit %d; stdout=%s", code, stdout)
	}
	var cov loopreview.CoverageResult
	if err := json.Unmarshal([]byte(stdout), &cov); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cov.Covered || cov.Error == "" || cov.CoveredCount != 0 {
		t.Fatalf("expected fail-closed coverage error on malformed TSV, got %+v", cov)
	}

	// 2. Uninitialized 6-column TSV (missing init Row 1) MUST fail closed even with F-01..F-07.
	uninit := filepath.Join(dir, "uninit.tsv")
	uninitContent := strings.Join([]string{
		"ts\tphase\tdecision\twhy\tevidence\tresult",
		"2026-11-28T00:00:00Z\tloop-04\tfocus F-01 (safe): a\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:01Z\tloop-04\tfocus F-02 (safe): b\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:02Z\tloop-04\tfocus F-03 (safe): c\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:03Z\tloop-04\tfocus F-04 (safe): d\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:04Z\tloop-04\tfocus F-05 (safe): e\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:05Z\tloop-04\tfocus F-06 (safe): f\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:06Z\tloop-04\tfocus F-07 (safe): g\tok\tADR-001039\tinfo",
	}, "\n") + "\n"
	if err := os.WriteFile(uninit, []byte(uninitContent), fsperm.File); err != nil {
		t.Fatalf("write uninit: %v", err)
	}
	code, stdout, _ = runCLI(t, "check-coverage", "--decisions-file", uninit, "--format", "json")
	if code != 1 {
		t.Fatalf("uninitialized TSV must fail closed, got exit %d; stdout=%s", code, stdout)
	}

	// 3. Reverted / mismatch rows must NOT count toward coverage.
	reverted := filepath.Join(dir, "reverted.tsv")
	revertedContent := strings.Join([]string{
		"ts\tphase\tdecision\twhy\tevidence\tresult",
		"2026-11-28T00:00:00Z\tinit\tpolicy=fix-existing; goal=g; completion=c\tinit\tADR-001039\tinfo",
		"2026-11-28T00:00:01Z\tloop-04\tfocus F-01 (safe): a\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:02Z\tloop-04\tfocus F-02 (safe): b\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:03Z\tloop-04\tfocus F-03 (safe): c\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:04Z\tloop-04\tfocus F-04 (safe): d\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:05Z\tloop-04\tfocus F-05 (safe): e\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:06Z\tloop-04\tfocus F-06 (fixed): f\treverted fix\tADR-001039\treverted",
		"2026-11-28T00:00:07Z\tloop-04\tfocus F-07 (safe): g\tmismatch claim\tADR-001039\tmismatch",
	}, "\n") + "\n"
	if err := os.WriteFile(reverted, []byte(revertedContent), fsperm.File); err != nil {
		t.Fatalf("write reverted: %v", err)
	}
	code, stdout, _ = runCLI(t, "check-coverage", "--decisions-file", reverted, "--format", "json")
	if code != 1 {
		t.Fatalf("reverted/mismatch rows must not satisfy coverage, got exit %d; stdout=%s", code, stdout)
	}
	cov = loopreview.CoverageResult{}
	if err := json.Unmarshal([]byte(stdout), &cov); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cov.CoveredCount != 5 || len(cov.MissingIDs) != 2 || cov.MissingIDs[0] != "F-06" || cov.MissingIDs[1] != "F-07" {
		t.Fatalf("expected F-06 and F-07 missing, got %+v", cov)
	}

	// 4. Default mode accepts safe|fixed|deferred; --strict-resolved rejects deferred until resolved.
	deferredFile := filepath.Join(dir, "deferred.tsv")
	deferredContent := strings.Join([]string{
		"ts\tphase\tdecision\twhy\tevidence\tresult",
		"2026-11-28T00:00:00Z\tinit\tpolicy=fix-existing; goal=g; completion=c\tinit\tADR-001039\tinfo",
		"2026-11-28T00:00:01Z\tloop-04\tfocus F-01 (safe): a\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:02Z\tloop-04\tfocus F-02 (safe): b\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:03Z\tloop-04\tfocus F-03 (safe): c\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:04Z\tloop-04\tfocus F-04 (safe): d\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:05Z\tloop-04\tfocus F-05 (safe): e\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:06Z\tloop-04\tfocus F-06 (fixed): f\tok\tADR-001039\tinfo",
		"2026-11-28T00:00:07Z\tloop-04\tfocus F-07 (deferred): g\tdeferred for follow-up\tADR-001039\tinfo",
	}, "\n") + "\n"
	if err := os.WriteFile(deferredFile, []byte(deferredContent), fsperm.File); err != nil {
		t.Fatalf("write deferred: %v", err)
	}
	code, stdout, stderr := runCLI(t, "check-coverage", "--decisions-file", deferredFile, "--format", "json")
	if code != 0 {
		t.Fatalf("default check-coverage should accept deferred, got exit %d; stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, _ = runCLI(t, "check-coverage", "--decisions-file", deferredFile, "--strict-resolved", "--format", "json")
	if code != 1 {
		t.Fatalf("--strict-resolved must reject deferred F-07, got exit %d; stdout=%s", code, stdout)
	}
	cov = loopreview.CoverageResult{}
	if err := json.Unmarshal([]byte(stdout), &cov); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cov.Covered || len(cov.MissingIDs) != 1 || cov.MissingIDs[0] != "F-07" {
		t.Fatalf("expected only F-07 missing under --strict-resolved, got %+v", cov)
	}
	// Appending F-07 (fixed) satisfies --strict-resolved.
	resolvedContent := deferredContent + "2026-11-28T00:00:08Z\tloop-04\tfocus F-07 (fixed): g\tresolved\tADR-001039\tinfo\n"
	if err := os.WriteFile(deferredFile, []byte(resolvedContent), fsperm.File); err != nil {
		t.Fatalf("write resolved deferred file: %v", err)
	}
	code, stdout, stderr = runCLI(t, "check-coverage", "--decisions-file", deferredFile, "--strict-resolved", "--format", "json")
	if code != 0 {
		t.Fatalf("--strict-resolved should pass after F-07 fixed, got exit %d; stdout=%s stderr=%s", code, stdout, stderr)
	}

	// 5. Symlinked --decisions-file MUST be rejected fail-closed.
	linkDec := filepath.Join(dir, "link-decisions.tsv")
	if err := os.Symlink(deferredFile, linkDec); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	code, stdout, _ = runCLI(t, "check-coverage", "--decisions-file", linkDec, "--format", "json")
	if code != 1 {
		t.Fatalf("symlinked decisions file must fail coverage, got exit %d; stdout=%s", code, stdout)
	}
}

func TestRecordIntoPreInitializedDecisionsNoDuplicateInit(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	var initOut, initErr bytes.Buffer
	rc := guards.RunHarness(context.Background(), []string{
		"init", "--repo-root", repo, "--branch", "feat/preinit",
		"--policy", "block-new", "--goal", "loop04 preinit", "--completion", "go test ./...",
	}, nil, &initOut, &initErr)
	if rc != 0 {
		t.Fatalf("harness-run init failed: rc=%d out=%s err=%s", rc, initOut.String(), initErr.String())
	}

	reportPath := filepath.Join(t.TempDir(), "report.json")
	writeJSON(t, reportPath, makeValidFullReport())

	code, stdout, stderr := runCLI(t, "record", reportPath, "--repo-root", repo, "--branch", "feat/preinit", "--require-focus-all", "--format", "json")
	if code != 0 {
		t.Fatalf("record into pre-initialized repo failed: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	decisionsPath := filepath.Join(repo, ".harness", "runs", "feat-preinit", "decisions.tsv")
	raw := string(readTestFile(t, decisionsPath))
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	// 1 header + 1 existing init + 4 findings + 7 focus verdicts = 13 lines
	if len(lines) != 13 {
		t.Fatalf("expected 13 lines (no duplicate init row), got %d:\n%s", len(lines), raw)
	}
	if _, findings, err := guards.ValidateDecisionsFile(decisionsPath); err != nil || len(findings) != 0 {
		t.Fatalf("ValidateDecisionsFile failed: err=%v findings=%+v", err, findings)
	}
	code, stdout, stderr = runCLI(t, "check-coverage", "--repo-root", repo, "--branch", "feat/preinit", "--format", "json")
	if code != 0 {
		t.Fatalf("check-coverage failed: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestSymlinkAndDanglingSymlinkDefense(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	outside := t.TempDir()

	// 1. brief --out dangling symlink must be rejected without creating target outside.
	danglingOutTarget := filepath.Join(outside, "dangling-brief.md")
	danglingOutLink := filepath.Join(t.TempDir(), "brief-link.md")
	if err := os.Symlink(danglingOutTarget, danglingOutLink); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	code, _, _ := runCLI(t, "brief", "--repo-root", repo, "--out", danglingOutLink)
	if code == 0 {
		t.Fatalf("expected failure when brief --out is a dangling symlink")
	}
	if _, err := os.Lstat(danglingOutTarget); !os.IsNotExist(err) {
		t.Fatalf("dangling brief --out target must not be created")
	}

	reportPath := filepath.Join(t.TempDir(), "report.json")
	writeJSON(t, reportPath, makeValidFullReport())

	// 2. record --runs-dir dangling symlink must be rejected.
	danglingRunsTarget := filepath.Join(outside, "runs-created")
	danglingRunsLink := filepath.Join(t.TempDir(), "runs-link")
	if err := os.Symlink(danglingRunsTarget, danglingRunsLink); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	code, _, _ = runCLI(t, "record", reportPath, "--repo-root", repo, "--runs-dir", danglingRunsLink, "--branch", "feat/sec01")
	if code == 0 {
		t.Fatalf("expected failure when --runs-dir is a dangling symlink")
	}
	if _, err := os.Lstat(danglingRunsTarget); !os.IsNotExist(err) {
		t.Fatalf("dangling --runs-dir target must not be created")
	}

	// 3. record --decisions-file dangling symlink must be rejected.
	danglingDecTarget := filepath.Join(outside, "victim-dangling.tsv")
	danglingDecLink := filepath.Join(t.TempDir(), "decisions-dangling.tsv")
	if err := os.Symlink(danglingDecTarget, danglingDecLink); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	code, _, _ = runCLI(t, "record", reportPath, "--repo-root", repo, "--runs-dir", filepath.Join(t.TempDir(), "clean-runs"), "--branch", "feat/sec01", "--decisions-file", danglingDecLink)
	if code == 0 {
		t.Fatalf("expected failure when --decisions-file is a dangling symlink")
	}
	if _, err := os.Lstat(danglingDecTarget); !os.IsNotExist(err) {
		t.Fatalf("dangling --decisions-file target must not be created")
	}

	// 4. record with default repo .harness symlink escaping outside must be rejected.
	evilRepo := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(evilRepo, ".harness")); err != nil {
		t.Fatalf("symlink .harness: %v", err)
	}
	code, _, _ = runCLI(t, "record", reportPath, "--repo-root", evilRepo, "--branch", "feat/sec01")
	if code == 0 {
		t.Fatalf("expected failure when repo .harness is a symlink")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("outside dir must remain untouched, got %d entries", len(entries))
	}

	// 5. record with dangling symlink at <runDir>/reviews/<review_id>.json must be rejected.
	runsWithDanglingReview := filepath.Join(t.TempDir(), "runs-dangling-rev")
	reviewsDir := filepath.Join(runsWithDanglingReview, "feat-sec01", "reviews")
	if err := os.MkdirAll(reviewsDir, fsperm.Dir); err != nil {
		t.Fatalf("mkdir reviews: %v", err)
	}
	danglingReviewTarget := filepath.Join(outside, "escaped-review.json")
	if err := os.Symlink(danglingReviewTarget, filepath.Join(reviewsDir, "LOOP04-2026-11-28.json")); err != nil {
		t.Fatalf("symlink review json: %v", err)
	}
	code, _, _ = runCLI(t, "record", reportPath, "--repo-root", repo, "--runs-dir", runsWithDanglingReview, "--branch", "feat/sec01")
	if code == 0 {
		t.Fatalf("expected failure when <review_id>.json is a dangling symlink")
	}
	if _, err := os.Lstat(danglingReviewTarget); !os.IsNotExist(err) {
		t.Fatalf("dangling review json target must not be created")
	}
}
