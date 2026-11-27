package loopcorrect_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alt/scripts/altharness/internal/corpus"
	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/loopcorrect"
)

func setupFakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// Create .git dir so guards.ResolveRepoRoot and ResolveGitBranch work deterministically.
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, fsperm.Dir); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), fsperm.File); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	// Create corpus cases C-01 and C-11 and C-22.
	writeCorpusCase(t, root, "C-01", "errorlint autofix drops RHS", "autofix-logic-corruption", "go", "go.condition_operator_change")
	writeCorpusCase(t, root, "C-11", "Unsolicited IdleTimeout added to http.Server", "unrequested-behavior-change", "go", "go-no-raw-http-server")
	writeCorpusCase(t, root, "C-22", "File-top blanket suppression", "blanket-suppression", "python", "source.broad.missing-adr")

	// Create ast-grep rule files.
	astDir := filepath.Join(root, "scripts", "lint", "ast-grep", "rules")
	if err := os.MkdirAll(astDir, fsperm.Dir); err != nil {
		t.Fatalf("mkdir ast-grep: %v", err)
	}
	if err := os.WriteFile(filepath.Join(astDir, "go-no-raw-http-server.yml"), []byte("id: go-no-raw-http-server\nlanguage: go\n"), fsperm.File); err != nil {
		t.Fatalf("write ast-grep rule: %v", err)
	}
	if err := os.WriteFile(filepath.Join(astDir, "python-no-manual-aclose-in-finally.yml"), []byte("id: python-no-manual-aclose-in-finally\nlanguage: python\n"), fsperm.File); err != nil {
		t.Fatalf("write ast-grep rule: %v", err)
	}

	// Create concrete repo files for enforcement_ref validation.
	for _, rel := range []string{
		"scripts/verify/verify-live.sh",
		".agents/rules/rule-ledger.md",
		"scripts/altharness/alt-harness",
	} {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), fsperm.Dir); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte("#!/bin/sh\n"), fsperm.File); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

func writeCorpusCase(t *testing.T, root, id, title, category, lang, signal string) {
	t.Helper()
	dir := filepath.Join(root, "scripts", "harness", "corpus", id)
	if err := os.MkdirAll(dir, fsperm.Dir); err != nil {
		t.Fatalf("mkdir corpus %s: %v", id, err)
	}
	ext := "go"
	if lang == "python" {
		ext = "py"
	}
	meta := "id = \"" + id + "\"\n" +
		"title = \"" + title + "\"\n" +
		"category = \"" + category + "\"\n" +
		"language = \"" + lang + "\"\n" +
		"before_file = \"before." + ext + "\"\n" +
		"after_file = \"after." + ext + "\"\n" +
		"support_files = []\n" +
		"evaluation_mode = \"diff\"\n\n" +
		"[origin]\n" +
		"branch = \"main\"\n" +
		"base_commit = \"abcdef1\"\n" +
		"files = [\"pkg/a." + ext + "\"]\n" +
		"commits = [\"abcdef1\"]\n" +
		"source_ref = \"docs/review/test.md\"\n" +
		"notes = \"Recorded mistake for regression testing.\"\n\n" +
		"[detection]\n" +
		"planned_checks = [\"CHECK-01\"]\n" +
		"active_checks = [\"CHECK-01\"]\n" +
		"currently_caught = true\n" +
		"current_method = \"CHECK-01\"\n" +
		"expected_signal = \"" + signal + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "meta.toml"), []byte(meta), fsperm.File); err != nil {
		t.Fatalf("write meta.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "before."+ext), []byte("// before\n"), fsperm.File); err != nil {
		t.Fatalf("write before: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "after."+ext), []byte("// after\n"), fsperm.File); err != nil {
		t.Fatalf("write after: %v", err)
	}
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := loopcorrect.Run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatalf("open root for %s: %v", path, err)
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(filepath.Base(path))
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

func TestCLIUsageAndHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var nilCtx context.Context
	if code := loopcorrect.Run(nilCtx, []string{"scan"}, &stdout, &stderr); code != 2 {
		t.Fatalf("expected exit 2 on nil ctx, got %d", code)
	}
	if code, _, _ := runCLI(t); code != 2 {
		t.Fatalf("expected exit 2 on empty args, got %d", code)
	}
	if code, out, _ := runCLI(t, "--help"); code != 0 || !strings.Contains(out, "scan") || !strings.Contains(out, "validate") || !strings.Contains(out, "record") || !strings.Contains(out, "scaffold-corpus") {
		t.Fatalf("expected exit 0 with subcommand usage, got code=%d out=%q", code, out)
	}
	if code, _, _ := runCLI(t, "unknown-sub"); code != 2 {
		t.Fatalf("expected exit 2 on unknown subcommand, got %d", code)
	}
}

func TestScanClassPrecedenceGroupingAndCorpusMapping(t *testing.T) {
	repo := setupFakeRepo(t)
	decisionsPath := filepath.Join(repo, ".harness", "runs", "main", "decisions.tsv")
	if err := os.MkdirAll(filepath.Dir(decisionsPath), fsperm.Dir); err != nil {
		t.Fatalf("mkdir runs: %v", err)
	}

	tsv := strings.Join([]string{
		"ts\tphase\tdecision\twhy\tevidence\tresult",
		"2026-11-28T00:00:01Z\tinit\tpolicy=fix-existing; completion=green\tinit row\t.harness/runs/main/decisions.tsv\tinfo",
		"2026-11-28T00:01:00Z\thandoff\thandoff p1: adopted\tall good\t.harness/runs/main/handoffs/p1.md\tadopted",
		"2026-11-28T00:02:00Z\thandoff\thandoff p2: cancelled\tstopped\t.harness/runs/main/handoffs/p2.md\tcancelled",
		// Precedence 1: explicit class=<id> overrides C-01 and [CODE]
		"2026-11-28T00:03:00Z\thandoff\tclass=errorlint-and-drop C-01 [ERR_X]\treverted autofix\t.harness/runs/main/handoffs/p3.md\treverted",
		"2026-11-28T00:04:00Z\treview\tfix failed\tclass=errorlint-and-drop repeated\tcommit:1111111\tmismatch",
		// Precedence 2: explicit C-11 in evidence / decision (no class= tag)
		"2026-11-28T00:05:00Z\thandoff\thandoff p4: reverted\tadded IdleTimeout on C-11\t.harness/runs/main/handoffs/p4.md\treverted",
		"2026-11-28T00:06:00Z\thandoff\thandoff p5: mismatch\tunrequested timeout\tscripts/harness/corpus/C-11/meta.toml\tmismatch",
		// Precedence 3: explicit bracketed error code [VERIFICATION_EXIT_CODE_MISMATCH]
		"2026-11-28T00:07:00Z\tverify\t[VERIFICATION_EXIT_CODE_MISMATCH] rerun failed\tclaimed 0 got 1\t.harness/runs/main/verify/v1.log\tmismatch",
		"2026-11-28T00:08:00Z\tverify\trerun mismatch\t[VERIFICATION_EXIT_CODE_MISMATCH] host test failed\t.harness/runs/main/verify/v2.log\tmismatch",
		// Precedence 4: normalized fallback from phase + first clause of why (grouped despite differing second clause & casing)
		"2026-11-28T00:09:00Z\thandoff\thandoff p6: reverted\tMissing env var at startup; retried once\t.harness/runs/main/handoffs/p6.md\treverted",
		"2026-11-28T00:10:00Z\thandoff\thandoff p7: reverted\tmissing   env var at startup; escalated to user\t.harness/runs/main/handoffs/p7.md\treverted",
		// Single occurrence (< threshold 2)
		"2026-11-28T00:11:00Z\tverify\t[ONE_OFF_FLAKE] transient\tnetwork hiccup\t.harness/runs/main/verify/v3.log\treverted",
	}, "\n") + "\n"
	if err := os.WriteFile(decisionsPath, []byte(tsv), fsperm.File); err != nil {
		t.Fatalf("write decisions.tsv: %v", err)
	}

	code, out, stderr := runCLI(t, "scan", "--decisions-file", decisionsPath, "--repo-root", repo, "--format", "json")
	if code != 0 {
		t.Fatalf("scan failed: code=%d stderr=%s out=%s", code, stderr, out)
	}

	var report struct {
		Threshold            int `json:"threshold"`
		TotalFailuresScanned int `json:"total_failures_scanned"`
		RecurringClasses     []struct {
			ClassKey     string   `json:"class_key"`
			MatchSource  string   `json:"match_source"`
			Count        int      `json:"count"`
			CorpusMapped bool     `json:"corpus_mapped"`
			CorpusIDs    []string `json:"corpus_ids"`
			Evidence     []string `json:"evidence"`
			Timestamps   []string `json:"timestamps"`
		} `json:"recurring_classes"`
		SingleOccurrences []struct {
			ClassKey string `json:"class_key"`
			Count    int    `json:"count"`
		} `json:"single_occurrences"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("unmarshal scan json: %v\nout=%s", err, out)
	}
	if report.Threshold != 2 {
		t.Fatalf("expected threshold 2, got %d", report.Threshold)
	}
	if report.TotalFailuresScanned != 9 {
		t.Fatalf("expected 9 total failures scanned (only reverted+mismatch), got %d", report.TotalFailuresScanned)
	}
	if len(report.RecurringClasses) != 4 {
		t.Fatalf("expected 4 recurring classes, got %d: %+v", len(report.RecurringClasses), report.RecurringClasses)
	}
	if len(report.SingleOccurrences) != 1 || report.SingleOccurrences[0].ClassKey != "ONE_OFF_FLAKE" {
		t.Fatalf("expected 1 single occurrence ONE_OFF_FLAKE, got %+v", report.SingleOccurrences)
	}

	byKey := map[string]struct {
		matchSource  string
		count        int
		corpusMapped bool
		corpusIDs    []string
		evLen        int
		tsLen        int
	}{}
	for _, rc := range report.RecurringClasses {
		byKey[rc.ClassKey] = struct {
			matchSource  string
			count        int
			corpusMapped bool
			corpusIDs    []string
			evLen        int
			tsLen        int
		}{rc.MatchSource, rc.Count, rc.CorpusMapped, rc.CorpusIDs, len(rc.Evidence), len(rc.Timestamps)}
	}

	if got, ok := byKey["errorlint-and-drop"]; !ok || got.count != 2 || got.matchSource != "class_tag" {
		t.Fatalf("unexpected errorlint-and-drop entry: %+v (ok=%v)", got, ok)
	}
	if got, ok := byKey["C-11"]; !ok || got.count != 2 || got.matchSource != "corpus_id" || !got.corpusMapped || len(got.corpusIDs) != 1 || got.corpusIDs[0] != "C-11" {
		t.Fatalf("unexpected C-11 entry: %+v (ok=%v)", got, ok)
	}
	if got, ok := byKey["VERIFICATION_EXIT_CODE_MISMATCH"]; !ok || got.count != 2 || got.matchSource != "error_code" || got.corpusMapped {
		t.Fatalf("unexpected VERIFICATION_EXIT_CODE_MISMATCH entry: %+v (ok=%v)", got, ok)
	}
	if got, ok := byKey["handoff:missing-env-var-at-startup"]; !ok || got.count != 2 || got.matchSource != "fallback" || got.evLen != 2 || got.tsLen != 2 {
		t.Fatalf("unexpected fallback entry: %+v (ok=%v)", got, ok)
	}

	// Text format smoke test.
	code, txt, stderr := runCLI(t, "scan", "--branch", "main", "--repo-root", repo, "--format", "text")
	if code != 0 || !strings.Contains(txt, "C-11") || !strings.Contains(txt, "VERIFICATION_EXIT_CODE_MISMATCH") {
		t.Fatalf("scan text failed: code=%d stderr=%s out=%s", code, stderr, txt)
	}
}

func TestScanRunsDirAggregationAndErrors(t *testing.T) {
	repo := setupFakeRepo(t)
	runsDir := filepath.Join(repo, ".harness", "runs")
	for _, pair := range []struct {
		branch string
		row    string
	}{
		{"feat-a", "2026-11-28T01:00:00Z\thandoff\tclass=shared-bug\tfirst hit\t.harness/runs/feat-a/h1.md\treverted\n"},
		{"feat-b", "2026-11-28T02:00:00Z\thandoff\tclass=shared-bug\tsecond hit\t.harness/runs/feat-b/h2.md\tmismatch\n"},
	} {
		d := filepath.Join(runsDir, pair.branch)
		if err := os.MkdirAll(d, fsperm.Dir); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
		content := "ts\tphase\tdecision\twhy\tevidence\tresult\n" + pair.row
		if err := os.WriteFile(filepath.Join(d, "decisions.tsv"), []byte(content), fsperm.File); err != nil {
			t.Fatalf("write tsv: %v", err)
		}
	}

	code, out, stderr := runCLI(t, "scan", "--runs-dir", runsDir, "--repo-root", repo, "--format", "json")
	if code != 0 {
		t.Fatalf("scan --runs-dir failed: code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(out, `"class_key": "shared-bug"`) || !strings.Contains(out, `"count": 2`) {
		t.Fatalf("expected cross-branch aggregation for shared-bug, got:\n%s", out)
	}

	// Threshold=3 moves count=2 into single_occurrences.
	code, out, _ = runCLI(t, "scan", "--runs-dir", runsDir, "--threshold", "3", "--repo-root", repo, "--format", "json")
	if code != 0 {
		t.Fatalf("scan threshold=3 failed: %d", code)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if recs, _ := parsed["recurring_classes"].([]any); len(recs) != 0 {
		t.Fatalf("expected 0 recurring classes at threshold 3, got %v", recs)
	}

	// Mutually exclusive flags -> exit 2.
	if code, _, _ := runCLI(t, "scan", "--decisions-file", filepath.Join(runsDir, "feat-a", "decisions.tsv"), "--runs-dir", runsDir); code != 2 {
		t.Fatalf("expected exit 2 for mutually exclusive flags, got %d", code)
	}
	// Missing explicit file -> exit 2.
	if code, _, _ := runCLI(t, "scan", "--decisions-file", filepath.Join(repo, "missing.tsv")); code != 2 {
		t.Fatalf("expected exit 2 for missing --decisions-file, got %d", code)
	}
	// Non-positive threshold -> exit 2.
	if code, _, _ := runCLI(t, "scan", "--runs-dir", runsDir, "--threshold", "0"); code != 2 {
		t.Fatalf("expected exit 2 for --threshold 0, got %d", code)
	}
}

func writeJSONFixture(t *testing.T, dir, name string, payload any) string {
	t.Helper()
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, raw, fsperm.File); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return p
}

func baseValidDisposition() map[string]any {
	return map[string]any{
		"milestone_id": "service-auth-hub-2026-11-28",
		"service":      "auth-hub",
		"classes": []map[string]any{
			{
				"class_id":              "C-11",
				"title":                 "Unsolicited IdleTimeout added to http.Server",
				"occurrences":           2,
				"evidence":              []string{".harness/runs/main/decisions.tsv", "docs/review/lint-rollout-harness-learnings-2026-11-21.md#2.2"},
				"corpus_id":             "C-11",
				"chosen_rung":           "ARCH",
				"higher_rungs_rejected": map[string]string{},
				"enforcement_ref":       "ast-grep:go-no-raw-http-server",
				"notes":                 "Canonical httpserver.New entrypoint + ast-grep rule catches C-11.",
			},
		},
	}
}

func assertHasIssueCode(t *testing.T, jsonOut, wantCode string) {
	t.Helper()
	var rep struct {
		Valid  bool `json:"valid"`
		Issues []struct {
			Code    string `json:"code"`
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &rep); err != nil {
		t.Fatalf("unmarshal validation json: %v\nout=%s", err, jsonOut)
	}
	if rep.Valid {
		t.Fatalf("expected valid=false for code %s, got valid=true", wantCode)
	}
	for _, iss := range rep.Issues {
		if iss.Code == wantCode {
			return
		}
	}
	t.Fatalf("expected issue code %s, got issues: %+v", wantCode, rep.Issues)
}

func TestValidateValidRungLadderAndEnforcementForms(t *testing.T) {
	repo := setupFakeRepo(t)
	tmp := t.TempDir()

	// All 4 rungs valid with required higher-rung explanations and all 3 concrete enforcement_ref forms.
	disp := map[string]any{
		"milestone_id": "milestone-all-rungs-2026-11-28",
		"service":      "multi-service",
		"classes": []map[string]any{
			{
				"class_id":              "C-11",
				"title":                 "Raw http.Server construction",
				"occurrences":           2,
				"evidence":              []string{".harness/runs/main/decisions.tsv"},
				"corpus_id":             "C-11",
				"chosen_rung":           "ARCH",
				"higher_rungs_rejected": map[string]string{},
				"enforcement_ref":       "ast-grep:go-no-raw-http-server",
				"notes":                 "Structural entrypoint prevents raw http.Server.",
			},
			{
				"class_id":    "C-01",
				"title":       "Autofix drops && RHS condition",
				"occurrences": 3,
				"evidence":    []string{"commit:d8ce79dec"},
				"corpus_id":   "C-01",
				"chosen_rung": "CHECK",
				"higher_rungs_rejected": map[string]string{
					"ARCH": "Go type assertions cannot be eliminated across third-party Echo error handlers structurally.",
				},
				"enforcement_ref": "scripts/altharness/alt-harness screener",
				"notes":           "CHECK-01 AST screener flags dropped condition clauses.",
			},
			{
				"class_id":    "C-22-live",
				"title":       "Runtime verify probe needed",
				"occurrences": 2,
				"evidence":    []string{"cmd:./scripts/verify/verify-live.sh run --probe all"},
				"corpus_id":   "C-22",
				"chosen_rung": "VERIFY",
				"higher_rungs_rejected": map[string]string{
					"ARCH":  "Third-party middleware lifecycle requires runtime integration rather than type-level wrappers.",
					"CHECK": "Static AST analysis cannot observe actual socket EOF termination under cancellation.",
				},
				"enforcement_ref": "scripts/verify/verify-live.sh",
				"notes":           "Live loopback probe catches runtime hang.",
			},
			{
				"class_id":    "ledger-only-case",
				"title":       "Build flag rebuild discipline",
				"occurrences": 2,
				"evidence":    []string{"ADR-001038"},
				"corpus_id":   "C-22",
				"chosen_rung": "LEDGER",
				"higher_rungs_rejected": map[string]string{
					"ARCH":   "Docker image rebuilds occur outside compiled Go/Rust module boundaries.",
					"CHECK":  "Static source checks cannot inspect local Docker daemon image layer timestamps.",
					"VERIFY": "Offline sandbox tests do not run full Docker Compose rebuild pipelines.",
				},
				"enforcement_ref": ".agents/rules/rule-ledger.md#R-27",
				"notes":           "Tracked explicitly in rule-ledger.md until mechanized.",
			},
		},
	}

	path := writeJSONFixture(t, tmp, "valid.json", disp)
	code, out, stderr := runCLI(t, "validate", path, "--repo-root", repo, "--format", "json")
	if code != 0 {
		t.Fatalf("expected valid disposition to pass, code=%d stderr=%s out=%s", code, stderr, out)
	}
	if !strings.Contains(out, `"valid": true`) {
		t.Fatalf("expected valid:true in output, got:\n%s", out)
	}

	// Also verify short form `alt-harness <subcommand>` works (including new loop-correct subcommand).
	shortDisp := baseValidDisposition()
	shortDisp["classes"].([]map[string]any)[0]["enforcement_ref"] = "alt-harness loop-correct"
	shortPath := writeJSONFixture(t, tmp, "short.json", shortDisp)
	if code, out, stderr := runCLI(t, "validate", shortPath, "--repo-root", repo, "--format", "json"); code != 0 {
		t.Fatalf("expected short alt-harness subcommand to pass, code=%d stderr=%s out=%s", code, stderr, out)
	}
}

func TestValidateRejectsViolationsFailClosed(t *testing.T) {
	repo := setupFakeRepo(t)
	tmp := t.TempDir()

	cases := []struct {
		name     string
		mutate   func(d map[string]any)
		wantCode string
	}{
		{
			name:     "milestone_id traversal CWE-22",
			mutate:   func(d map[string]any) { d["milestone_id"] = "../escape" },
			wantCode: "INVALID_MILESTONE_ID",
		},
		{
			name:     "milestone_id double dot",
			mutate:   func(d map[string]any) { d["milestone_id"] = "foo..bar" },
			wantCode: "INVALID_MILESTONE_ID",
		},
		{
			name:     "empty service",
			mutate:   func(d map[string]any) { d["service"] = "   " },
			wantCode: "MISSING_SERVICE",
		},
		{
			name:     "empty classes",
			mutate:   func(d map[string]any) { d["classes"] = []map[string]any{} },
			wantCode: "EMPTY_CLASSES",
		},
		{
			name: "duplicate class_id",
			mutate: func(d map[string]any) {
				cls := d["classes"].([]map[string]any)
				dup := map[string]any{}
				for k, v := range cls[0] {
					dup[k] = v
				}
				d["classes"] = append(cls, dup)
			},
			wantCode: "DUPLICATE_CLASS_ID",
		},
		{
			name: "invalid rung",
			mutate: func(d map[string]any) {
				d["classes"].([]map[string]any)[0]["chosen_rung"] = "DOCS"
			},
			wantCode: "INVALID_RUNG",
		},
		{
			name: "below 2-strike class threshold",
			mutate: func(d map[string]any) {
				d["classes"].([]map[string]any)[0]["occurrences"] = 1
			},
			wantCode: "BELOW_CLASS_THRESHOLD",
		},
		{
			name: "placeholder evidence",
			mutate: func(d map[string]any) {
				d["classes"].([]map[string]any)[0]["evidence"] = []string{"TBD"}
			},
			wantCode: "INVALID_EVIDENCE_POINTER",
		},
		{
			name: "prose evidence",
			mutate: func(d map[string]any) {
				d["classes"].([]map[string]any)[0]["evidence"] = []string{"it happened twice yesterday"}
			},
			wantCode: "INVALID_EVIDENCE_POINTER",
		},
		{
			name: "missing corpus case on disk",
			mutate: func(d map[string]any) {
				d["classes"].([]map[string]any)[0]["corpus_id"] = "C-99"
			},
			wantCode: "MISSING_CORPUS_CASE",
		},
		{
			name: "malformed corpus_id",
			mutate: func(d map[string]any) {
				d["classes"].([]map[string]any)[0]["corpus_id"] = "C-1"
			},
			wantCode: "MISSING_CORPUS_CASE",
		},
		{
			name: "empty corpus_id",
			mutate: func(d map[string]any) {
				d["classes"].([]map[string]any)[0]["corpus_id"] = ""
			},
			wantCode: "MISSING_CORPUS_CASE",
		},
		{
			name: "missing ast-grep rule",
			mutate: func(d map[string]any) {
				d["classes"].([]map[string]any)[0]["enforcement_ref"] = "ast-grep:nonexistent-rule"
			},
			wantCode: "MISSING_ENFORCEMENT_REF",
		},
		{
			name: "missing repo file enforcement_ref",
			mutate: func(d map[string]any) {
				d["classes"].([]map[string]any)[0]["enforcement_ref"] = "scripts/lint/does-not-exist.sh"
			},
			wantCode: "MISSING_ENFORCEMENT_REF",
		},
		{
			name: "bare alt-harness without subcommand",
			mutate: func(d map[string]any) {
				d["classes"].([]map[string]any)[0]["enforcement_ref"] = "scripts/altharness/alt-harness"
			},
			wantCode: "MISSING_ENFORCEMENT_REF",
		},
		{
			name: "unknown alt-harness subcommand",
			mutate: func(d map[string]any) {
				d["classes"].([]map[string]any)[0]["enforcement_ref"] = "alt-harness phantom-cmd"
			},
			wantCode: "MISSING_ENFORCEMENT_REF",
		},
		{
			name: "CHECK missing ARCH rejection reason",
			mutate: func(d map[string]any) {
				c := d["classes"].([]map[string]any)[0]
				c["chosen_rung"] = "CHECK"
				c["higher_rungs_rejected"] = map[string]string{}
			},
			wantCode: "MISSING_HIGHER_RUNG_REASON",
		},
		{
			name: "VERIFY missing CHECK rejection reason",
			mutate: func(d map[string]any) {
				c := d["classes"].([]map[string]any)[0]
				c["chosen_rung"] = "VERIFY"
				c["higher_rungs_rejected"] = map[string]string{
					"ARCH": "Structural wrapper is not possible for third-party streaming handler.",
				}
			},
			wantCode: "MISSING_HIGHER_RUNG_REASON",
		},
		{
			name: "LEDGER missing VERIFY rejection reason",
			mutate: func(d map[string]any) {
				c := d["classes"].([]map[string]any)[0]
				c["chosen_rung"] = "LEDGER"
				c["higher_rungs_rejected"] = map[string]string{
					"ARCH":  "Cannot eliminate structurally because external runtime controls image build.",
					"CHECK": "Static AST rules cannot inspect Docker layer timestamps.",
				}
			},
			wantCode: "MISSING_HIGHER_RUNG_REASON",
		},
		{
			name: "trivial placeholder higher rung reason (too hard)",
			mutate: func(d map[string]any) {
				c := d["classes"].([]map[string]any)[0]
				c["chosen_rung"] = "CHECK"
				c["higher_rungs_rejected"] = map[string]string{
					"ARCH": "too hard",
				}
			},
			wantCode: "MISSING_HIGHER_RUNG_REASON",
		},
		{
			name: "placeholder prefix higher rung reason (TBD: ...)",
			mutate: func(d map[string]any) {
				c := d["classes"].([]map[string]any)[0]
				c["chosen_rung"] = "CHECK"
				c["higher_rungs_rejected"] = map[string]string{
					"ARCH": "TBD: will write a real reason later",
				}
			},
			wantCode: "MISSING_HIGHER_RUNG_REASON",
		},
		{
			name: "short higher rung reason (< 12 runes)",
			mutate: func(d map[string]any) {
				c := d["classes"].([]map[string]any)[0]
				c["chosen_rung"] = "CHECK"
				c["higher_rungs_rejected"] = map[string]string{
					"ARCH": "cannot do",
				}
			},
			wantCode: "MISSING_HIGHER_RUNG_REASON",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := baseValidDisposition()
			tc.mutate(d)
			p := writeJSONFixture(t, tmp, strings.ReplaceAll(tc.name, " ", "_")+".json", d)
			code, out, _ := runCLI(t, "validate", p, "--repo-root", repo, "--format", "json")
			if code != 1 {
				t.Fatalf("expected exit 1 for %s, got %d; out=%s", tc.name, code, out)
			}
			assertHasIssueCode(t, out, tc.wantCode)
		})
	}

	// Malformed JSON -> exit 1 with PARSE_ERROR.
	badJSON := filepath.Join(tmp, "malformed.json")
	if err := os.WriteFile(badJSON, []byte("{broken"), fsperm.File); err != nil {
		t.Fatalf("write malformed: %v", err)
	}
	code, out, _ := runCLI(t, "validate", badJSON, "--repo-root", repo, "--format", "json")
	if code != 1 {
		t.Fatalf("expected exit 1 on malformed json, got %d", code)
	}
	assertHasIssueCode(t, out, "PARSE_ERROR")

	// Missing file -> exit 2.
	if code, _, _ := runCLI(t, "validate", filepath.Join(tmp, "nope.json"), "--repo-root", repo); code != 2 {
		t.Fatalf("expected exit 2 on missing file, got %d", code)
	}
}

func TestRecordWritesMarkdownJSONAndAppendsDecisions(t *testing.T) {
	repo := setupFakeRepo(t)
	tmp := t.TempDir()

	disp := map[string]any{
		"milestone_id": "service-auth-hub-2026-11-28",
		"service":      "auth-hub",
		"classes": []map[string]any{
			{
				"class_id":              "C-11",
				"title":                 "Unsolicited IdleTimeout added to http.Server",
				"occurrences":           2,
				"evidence":              []string{".harness/runs/main/decisions.tsv", "docs/review/lint-rollout-harness-learnings-2026-11-21.md#2.2"},
				"corpus_id":             "C-11",
				"chosen_rung":           "ARCH",
				"higher_rungs_rejected": map[string]string{},
				"enforcement_ref":       "ast-grep:go-no-raw-http-server",
				"notes":                 "Canonical httpserver.New entrypoint.",
			},
			{
				"class_id":    "C-01",
				"title":       "errorlint drops && RHS condition",
				"occurrences": 3,
				"evidence":    []string{"commit:d8ce79dec"},
				"corpus_id":   "C-01",
				"chosen_rung": "CHECK",
				"higher_rungs_rejected": map[string]string{
					"ARCH": "Third-party Echo error checks\tneed runtime\ntype assertion.",
				},
				"enforcement_ref": "scripts/altharness/alt-harness screener",
				"notes":           "Screener detects dropped logical operator.",
			},
		},
	}
	dispPath := writeJSONFixture(t, tmp, "disp.json", disp)

	code, out, stderr := runCLI(t, "record", dispPath, "--branch", "main", "--repo-root", repo, "--format", "json")
	if code != 0 {
		t.Fatalf("record failed: code=%d stderr=%s out=%s", code, stderr, out)
	}

	mdPath := filepath.Join(repo, ".harness", "runs", "main", "correct", "service-auth-hub-2026-11-28.md")
	jsonPath := filepath.Join(repo, ".harness", "runs", "main", "correct", "service-auth-hub-2026-11-28.json")
	decisionsPath := filepath.Join(repo, ".harness", "runs", "main", "decisions.tsv")

	mdBytes := readTestFile(t, mdPath)
	md := string(mdBytes)
	wantHeader := "| クラス | 発生回数 | 証拠 | 選んだ段 | 上の段を使わなかった理由 | コーパス | 強制手段 |"
	if !strings.Contains(md, wantHeader) {
		t.Fatalf("markdown missing canonical table header %q:\n%s", wantHeader, md)
	}
	if !strings.Contains(md, "C-11") || !strings.Contains(md, "ast-grep:go-no-raw-http-server") || !strings.Contains(md, "C-01") {
		t.Fatalf("markdown missing expected class entries:\n%s", md)
	}

	if _, err := os.Stat(jsonPath); err != nil {
		t.Fatalf("expected json artifact at %s: %v", jsonPath, err)
	}

	tsvBytes := readTestFile(t, decisionsPath)
	lines := strings.Split(strings.TrimSpace(string(tsvBytes)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 appended rows, got %d lines:\n%s", len(lines), string(tsvBytes))
	}
	row1 := strings.Split(lines[1], "\t")
	if len(row1) != 6 {
		t.Fatalf("expected 6 TSV columns, got %d: %q", len(row1), lines[1])
	}
	if row1[1] != "correct" {
		t.Fatalf("expected phase=correct, got %q", row1[1])
	}
	if row1[2] != "correct C-11: rung=ARCH enforcement=ast-grep:go-no-raw-http-server" {
		t.Fatalf("unexpected row1 decision: %q", row1[2])
	}
	if row1[3] != "occurrences=2; rejected_higher=none" {
		t.Fatalf("unexpected row1 why: %q", row1[3])
	}
	if row1[4] != ".harness/runs/main/correct/service-auth-hub-2026-11-28.md" {
		t.Fatalf("expected repo-relative evidence path, got %q", row1[4])
	}
	if row1[5] != "info" {
		t.Fatalf("expected result=info, got %q", row1[5])
	}

	// Row 2 verifies tabs/newlines in rejected_higher are sanitized to single-line.
	row2 := strings.Split(lines[2], "\t")
	if len(row2) != 6 {
		t.Fatalf("expected 6 TSV columns in row2, got %d: %q", len(row2), lines[2])
	}
	if !strings.HasPrefix(row2[3], "occurrences=3; rejected_higher=ARCH=") || strings.ContainsAny(row2[3], "\t\n\r") {
		t.Fatalf("unexpected row2 why: %q", row2[3])
	}
}

func TestRecordFailClosedOnInvalidAndSymlink(t *testing.T) {
	repo := setupFakeRepo(t)
	tmp := t.TempDir()

	// Invalid disposition must fail closed and write nothing.
	bad := baseValidDisposition()
	bad["classes"].([]map[string]any)[0]["occurrences"] = 1
	badPath := writeJSONFixture(t, tmp, "bad.json", bad)
	code, _, _ := runCLI(t, "record", badPath, "--branch", "clean-branch", "--repo-root", repo)
	if code != 1 {
		t.Fatalf("expected exit 1 on invalid record, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(repo, ".harness", "runs", "clean-branch")); !os.IsNotExist(err) {
		t.Fatalf("expected no files written when record validation fails")
	}

	// Symlink in <runDir>/correct must be rejected fail-closed.
	runDir := filepath.Join(repo, ".harness", "runs", "sym-branch")
	if err := os.MkdirAll(runDir, fsperm.Dir); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(runDir, "correct")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	validPath := writeJSONFixture(t, tmp, "valid.json", baseValidDisposition())
	code, _, stderr := runCLI(t, "record", validPath, "--branch", "sym-branch", "--repo-root", repo)
	if code != 1 {
		t.Fatalf("expected exit 1 on symlinked correct dir, got %d (stderr=%s)", code, stderr)
	}
}

func TestScaffoldCorpusDryRunWriteAndNoOverwrite(t *testing.T) {
	repo := setupFakeRepo(t)

	// Dry-run computes C-23 after C-22 and does NOT write to disk.
	code, out, stderr := runCLI(t, "scaffold-corpus",
		"--class-id", "errorlint-and-drop",
		"--title", `Drop RHS in "errors.As" rewrite`,
		"--language", "go",
		"--category", "autofix-logic-corruption",
		"--repo-root", repo,
		"--format", "json",
	)
	if code != 0 {
		t.Fatalf("scaffold-corpus dry-run failed: code=%d stderr=%s out=%s", code, stderr, out)
	}
	var dry struct {
		CorpusID   string `json:"corpus_id"`
		Written    bool   `json:"written"`
		MetaPath   string `json:"meta_path"`
		BeforePath string `json:"before_path"`
		AfterPath  string `json:"after_path"`
		MetaTOML   string `json:"meta_toml"`
	}
	if err := json.Unmarshal([]byte(out), &dry); err != nil {
		t.Fatalf("unmarshal dry-run: %v\nout=%s", err, out)
	}
	if dry.CorpusID != "C-23" || dry.Written {
		t.Fatalf("expected C-23 written=false, got %+v", dry)
	}
	if !strings.HasSuffix(dry.BeforePath, "before.go") || !strings.HasSuffix(dry.AfterPath, "after.go") {
		t.Fatalf("unexpected snippet paths: %+v", dry)
	}
	if _, err := os.Stat(filepath.Join(repo, "scripts", "harness", "corpus", "C-23")); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not create C-23 directory")
	}

	// --write creates C-23 and generated files pass corpus.LoadCase!
	code, out, stderr = runCLI(t, "scaffold-corpus",
		"--class-id", "errorlint-and-drop",
		"--title", `Drop RHS in "errors.As" rewrite`,
		"--language", "go",
		"--category", "autofix-logic-corruption",
		"--write",
		"--repo-root", repo,
		"--format", "json",
	)
	if code != 0 {
		t.Fatalf("scaffold-corpus --write failed: code=%d stderr=%s out=%s", code, stderr, out)
	}
	caseDir := filepath.Join(repo, "scripts", "harness", "corpus", "C-23")
	loaded, errs := corpus.LoadCase(caseDir)
	if len(errs) > 0 || loaded == nil {
		t.Fatalf("scaffolded corpus case should pass corpus.LoadCase, got errs=%v", errs)
	}

	// Next invocation computes C-24 without overwriting C-23.
	code, out, _ = runCLI(t, "scaffold-corpus",
		"--class-id", "py-isolation",
		"--title", "Direct sys.modules mutation",
		"--language", "python",
		"--category", "test-isolation-hazard",
		"--write",
		"--repo-root", repo,
		"--format", "json",
	)
	if code != 0 || !strings.Contains(out, `"corpus_id": "C-24"`) || !strings.Contains(out, "before.py") {
		t.Fatalf("expected C-24 with before.py, got code=%d out=%s", code, out)
	}

	// Invalid language / category rejected with exit 2.
	if code, _, _ := runCLI(t, "scaffold-corpus", "--class-id", "x", "--title", "t", "--language", "cobol", "--category", "worktree-hygiene", "--repo-root", repo); code != 2 {
		t.Fatalf("expected exit 2 for invalid language, got %d", code)
	}
	if code, _, _ := runCLI(t, "scaffold-corpus", "--class-id", "x", "--title", "t", "--language", "go", "--category", "made-up-category", "--repo-root", repo); code != 2 {
		t.Fatalf("expected exit 2 for invalid category, got %d", code)
	}
}

func TestScaffoldCorpusAllLanguagesAndEmptyRepo(t *testing.T) {
	emptyRepo := t.TempDir()
	langs := []struct {
		lang string
		ext  string
		id   string
	}{
		{"go", "go", "C-01"},
		{"python", "py", "C-02"},
		{"rust", "rs", "C-03"},
		{"typescript", "ts", "C-04"},
		{"shell", "sh", "C-05"},
		{"binary", "bin", "C-06"},
	}
	for _, tc := range langs {
		code, out, stderr := runCLI(t, "scaffold-corpus",
			"--class-id", "cls-"+tc.lang,
			"--title", "Sample "+tc.lang+" mistake",
			"--language", tc.lang,
			"--category", "unrequested-behavior-change",
			"--write",
			"--repo-root", emptyRepo,
			"--format", "json",
		)
		if code != 0 {
			t.Fatalf("scaffold %s failed: code=%d stderr=%s out=%s", tc.lang, code, stderr, out)
		}
		if !strings.Contains(out, `"corpus_id": "`+tc.id+`"`) || !strings.Contains(out, "before."+tc.ext) || !strings.Contains(out, "after."+tc.ext) {
			t.Fatalf("unexpected scaffold output for %s: %s", tc.lang, out)
		}
		caseDir := filepath.Join(emptyRepo, "scripts", "harness", "corpus", tc.id)
		if loaded, errs := corpus.LoadCase(caseDir); len(errs) > 0 || loaded == nil {
			t.Fatalf("corpus.LoadCase failed for %s (%s): errs=%v", tc.id, tc.lang, errs)
		}
	}
}

func TestRecordWithExplicitDecisionsFileAndJapaneseReasons(t *testing.T) {
	repo := setupFakeRepo(t)
	tmp := t.TempDir()
	decFile := filepath.Join(repo, ".harness", "runs", "custom", "decisions.tsv")

	disp := baseValidDisposition()
	c := disp["classes"].([]map[string]any)[0]
	c["chosen_rung"] = "CHECK"
	c["higher_rungs_rejected"] = map[string]string{
		"ARCH": "外部ライブラリのミドルウェア構造上、型レベルで完全に禁止することができないため",
	}
	c["enforcement_ref"] = "scripts/altharness/alt-harness screener"
	p := writeJSONFixture(t, tmp, "jp.json", disp)

	code, out, stderr := runCLI(t, "record", p, "--decisions-file", decFile, "--repo-root", repo, "--format", "json")
	if code != 0 {
		t.Fatalf("record with --decisions-file failed: code=%d stderr=%s out=%s", code, stderr, out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".harness", "runs", "custom", "correct", "service-auth-hub-2026-11-28.md")); err != nil {
		t.Fatalf("expected markdown alongside --decisions-file: %v", err)
	}
	if _, err := os.Stat(decFile); err != nil {
		t.Fatalf("expected decisions.tsv created at %s: %v", decFile, err)
	}
}

func TestAgainstRealRepositoryAssets(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(repoRoot, "scripts", "harness", "corpus", "C-11", "meta.toml")); err != nil {
		t.Skipf("real repo corpus not found at %s: %v", repoRoot, err)
	}

	// Validate a real disposition against the real repo's C-11 + ast-grep rule.
	tmp := t.TempDir()
	dispPath := writeJSONFixture(t, tmp, "real-c11.json", baseValidDisposition())
	code, out, stderr := runCLI(t, "validate", dispPath, "--repo-root", repoRoot, "--format", "json")
	if code != 0 {
		t.Fatalf("validate against real repo failed: code=%d stderr=%s out=%s", code, stderr, out)
	}

	// Dry-run scaffold-corpus against real repo must compute next unused ID (>= C-23) without modifying repo.
	code, out, stderr = runCLI(t, "scaffold-corpus",
		"--class-id", "real-dry-run",
		"--title", "Dry-run check against real corpus",
		"--language", "go",
		"--category", "worktree-hygiene",
		"--repo-root", repoRoot,
		"--format", "json",
	)
	if code != 0 {
		t.Fatalf("scaffold-corpus dry-run against real repo failed: code=%d stderr=%s out=%s", code, stderr, out)
	}
	if !strings.Contains(out, `"written": false`) || !strings.Contains(out, `"corpus_id": "C-23"`) {
		t.Fatalf("unexpected real repo dry-run output:\n%s", out)
	}
}
