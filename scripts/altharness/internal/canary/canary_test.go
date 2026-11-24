package canary

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestZeroFileSARIFRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	proj := filepath.Join(dir, "proj")
	if err := os.Mkdir(proj, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "sample.go"), []byte("package sample\n\nfunc Sample() {}\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	sarif := filepath.Join(dir, "gosec.sarif")
	sarifBody := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gosec","rules":[]}},"results":[]}]}`
	if err := os.WriteFile(sarif, []byte(sarifBody), fsperm.File); err != nil {
		t.Fatal(err)
	}

	t.Run("files zero", func(t *testing.T) {
		out := filepath.Join(dir, "zero.out")
		body := "Summary:\n  Files  : 0\n  Lines  : 0\n  Issues : 0\n"
		if err := os.WriteFile(out, []byte(body), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "go-verify-scan", "--project-dir", proj, "--gosec-out", out, "--gosec-sarif", sarif)
		if code != exitFail {
			t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "ZERO-FILE SCAN DETECTED") {
			t.Fatalf("stderr %s", stderr)
		}
		if strings.Contains(stdout, "passed") || strings.Contains(stdout, "skipped") {
			t.Fatalf("zero-file scan must not pass: %s", stdout)
		}
	})

	t.Run("files zero with load error", func(t *testing.T) {
		out := filepath.Join(dir, "load.out")
		body := "Golang errors in file: [.]:\n  > go.mod requires go >= 1.99.0\nSummary:\n  Files  : 0\n"
		if err := os.WriteFile(out, []byte(body), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runAudit(t, "go-verify-scan", "--project-dir", proj, "--gosec-out", out, "--gosec-sarif", sarif)
		if code != exitFail || !strings.Contains(stderr, "ZERO-FILE SCAN DETECTED") || !strings.Contains(stderr, "package load errors") {
			t.Fatalf("exit %d stderr %s", code, stderr)
		}
	})

	t.Run("missing files summary", func(t *testing.T) {
		out := filepath.Join(dir, "nosummary.out")
		if err := os.WriteFile(out, []byte("Issues : 0\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runAudit(t, "go-verify-scan", "--project-dir", proj, "--gosec-out", out, "--gosec-sarif", sarif)
		if code != exitFail || !strings.Contains(stderr, "ZERO-FILE SCAN DETECTED") {
			t.Fatalf("exit %d stderr %s", code, stderr)
		}
	})

	t.Run("nonzero files pass", func(t *testing.T) {
		out := filepath.Join(dir, "ok.out")
		if err := os.WriteFile(out, []byte("Summary:\n  Files  : 2\n  Issues : 0\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "go-verify-scan", "--project-dir", proj, "--gosec-out", out, "--gosec-sarif", sarif)
		if code != exitOK {
			t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "gosec scanned 2 files") {
			t.Fatalf("stdout %s", stdout)
		}
	})

	t.Run("canary uri in sarif fails", func(t *testing.T) {
		bad := filepath.Join(dir, "contaminated.sarif")
		body := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gosec","rules":[{"id":"G402"}]}},"results":[{"ruleId":"G402","locations":[{"physicalLocation":{"artifactLocation":{"uri":"/tmp/alt-go-canary-xyz/canary.go"}}}]}]}]}`
		if err := os.WriteFile(bad, []byte(body), fsperm.File); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, "ok2.out")
		if err := os.WriteFile(out, []byte("Summary:\n  Files  : 1\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runAudit(t, "go-verify-scan", "--project-dir", proj, "--gosec-out", out, "--gosec-sarif", bad)
		if code != exitFail || !strings.Contains(stderr, "contaminated by canary artifact") {
			t.Fatalf("exit %d stderr %s", code, stderr)
		}
	})

	t.Run("dangling artifact index fails", func(t *testing.T) {
		bad := filepath.Join(dir, "dangling.sarif")
		body := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gosec","rules":[{"id":"G402"}]}},"results":[{"ruleId":"G402","locations":[{"physicalLocation":{"artifactLocation":{"uri":"main.go","index":0}}}]}]}]}`
		if err := os.WriteFile(bad, []byte(body), fsperm.File); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, "ok3.out")
		if err := os.WriteFile(out, []byte("Summary:\n  Files  : 1\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runAudit(t, "go-verify-scan", "--project-dir", proj, "--gosec-out", out, "--gosec-sarif", bad)
		if code != exitFail || !strings.Contains(stderr, "artifactLocation.index present without run.artifacts") {
			t.Fatalf("exit %d stderr %s", code, stderr)
		}
	})

	t.Run("single path zero packages", func(t *testing.T) {
		mod := filepath.Join(dir, "zero-pkg")
		if err := os.Mkdir(mod, fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module zero-pkg\n\ngo 1.27.4\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(mod, "ignored.go"), []byte("//go:build ignore\n\npackage main\n\nfunc main() {}\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "go-verify-scan", "--project-dir", mod, "--gosec-sarif", sarif)
		if code != exitFail || !strings.Contains(stderr, "ZERO-FILE SCAN DETECTED") {
			t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
		}
		if strings.Contains(stdout, "passed") {
			t.Fatalf("zero packages must not pass: %s", stdout)
		}
	})

	t.Run("generated only skips", func(t *testing.T) {
		gen := filepath.Join(dir, "gen-only")
		if err := os.MkdirAll(filepath.Join(gen, "gen"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"gen/api.go", "types.pb.go", "svc.connect.go", "server.gen.go"} {
			if err := os.WriteFile(filepath.Join(gen, name), []byte("package genonly\n"), fsperm.File); err != nil {
				t.Fatal(err)
			}
		}
		out := filepath.Join(dir, "gen.out")
		if err := os.WriteFile(out, []byte("Summary:\n  Files  : 0\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "go-verify-scan", "--project-dir", gen, "--gosec-out", out, "--gosec-sarif", sarif)
		if code != exitOK || !strings.Contains(stdout, "skipped") {
			t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
		}
	})
}

func TestGovulncheckEmptySBOMRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	proj := filepath.Join(dir, "proj")
	if err := os.Mkdir(proj, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "main.go"), []byte("package main\n\nfunc main() {}\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	sarif := filepath.Join(dir, "govuln.sarif")
	sarifBody := `{"runs":[{"tool":{"driver":{"name":"govulncheck","rules":[],"properties":{"db":"https://vuln.example","go_version":"go1.27.4"}}}}]}`
	if err := os.WriteFile(sarif, []byte(sarifBody), fsperm.File); err != nil {
		t.Fatal(err)
	}
	stream := filepath.Join(dir, "govuln.json")
	streamBody := "{\"config\":{\"db\":\"https://vuln.example\",\"go_version\":\"go1.27.4\"}}\n{\"SBOM\":{\"modules\":[],\"roots\":[],\"packages\":[]}}\n"
	if err := os.WriteFile(stream, []byte(streamBody), fsperm.File); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runAudit(t,
		"go-verify-govulncheck",
		"--project-dir", proj,
		"--govulncheck-sarif", sarif,
		"--govulncheck-json", stream,
	)
	if code != exitFail {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "ZERO-FILE SCAN DETECTED") {
		t.Fatalf("stderr %s", stderr)
	}
	if strings.Contains(stdout, "passed") {
		t.Fatalf("empty SBOM must not pass: %s", stdout)
	}
}

func TestPythonZeroFilesRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, rel := range []string{".venv/site.py", "build/out.py", "gen/proto.py", "foo_pb2.py"} {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x = 1\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
	}
	code, stdout, stderr := runAudit(t, "python-verify-files", "--project-dir", dir)
	if code != exitFail || !strings.Contains(stderr, "ZERO-FILE SCAN DETECTED") {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	kept := filepath.Join(dir, "tag_generator", "main.py")
	if err := os.MkdirAll(filepath.Dir(kept), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, []byte("VALUE = 1\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = runAudit(t, "python-verify-files", "--project-dir", dir)
	if code != exitOK || !strings.Contains(stdout, "1 non-generated .py files") {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestVerifyLedgerTemp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	lintDir := filepath.Join(root, "scripts", "lint")
	if err := os.MkdirAll(lintDir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/bash\nif [ \"$1\" = \"--list\" ]; then\n  printf '%s\\n' svc-a svc-py\n  exit 0\nfi\necho unexpected >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(lintDir, "run.sh"), []byte(script), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	pyDir := filepath.Join(root, "svc-py")
	if err := os.MkdirAll(pyDir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	pyproject := "[project]\nname = \"svc-py\"\n[dependency-groups]\ndev = [\"ruff>=0.15.0\", \"bandit>=1.8.0\", \"pyrefly>=0.42.0\"]\n"
	if err := os.WriteFile(filepath.Join(pyDir, "pyproject.toml"), []byte(pyproject), fsperm.File); err != nil {
		t.Fatal(err)
	}

	const goTarget = "" +
		"version = 1\n" +
		"[targets.\"svc-a\"]\nkind = \"go\"\n" +
		"[targets.\"svc-a\".scanners.golangci-lint]\nstatus = \"canary\"\ncanary = \"go-canary\"\n" +
		"[targets.\"svc-a\".scanners.gosec]\nstatus = \"canary\"\ncanary = \"go-canary\"\n" +
		"[targets.\"svc-a\".scanners.govulncheck]\nstatus = \"canary\"\ncanary = \"go-verify-govulncheck\"\n"
	const pyTarget = "" +
		"[targets.\"svc-py\"]\nkind = \"python\"\n" +
		"[targets.\"svc-py\".scanners.ruff]\nstatus = \"canary\"\ncanary = \"python-canary\"\n" +
		"[targets.\"svc-py\".scanners.bandit]\nstatus = \"canary\"\ncanary = \"python-canary\"\n" +
		"[targets.\"svc-py\".scanners.pyrefly]\nstatus = \"canary\"\ncanary = \"python-canary\"\n"
	validBody := goTarget + pyTarget

	t.Run("missing target fails", func(t *testing.T) {
		ledger := filepath.Join(root, "missing.toml")
		if err := os.WriteFile(ledger, []byte(goTarget), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "verify-ledger", "--repo-root", root, "--ledger", ledger)
		if code != exitFail {
			t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "LEDGER CHECK FAILED") || !strings.Contains(stderr, "svc-py") {
			t.Fatalf("stderr %s", stderr)
		}
		if strings.Contains(stdout, "canary ledger verified") {
			t.Fatalf("missing ledger entry must not pass: %s", stdout)
		}
	})

	t.Run("complete ledger passes", func(t *testing.T) {
		ledger := filepath.Join(root, "valid.toml")
		if err := os.WriteFile(ledger, []byte(validBody), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "verify-ledger", "--repo-root", root, "--ledger", ledger)
		if code != exitOK {
			t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "canary ledger verified: 2 targets accounted for") {
			t.Fatalf("stdout %s", stdout)
		}
	})

	t.Run("empty exempt reason fails", func(t *testing.T) {
		body := goTarget + "[targets.\"svc-a\".scanners.gofmt]\nstatus = \"exempt\"\nreason = \"\"\n" + pyTarget
		ledger := filepath.Join(root, "empty-reason.toml")
		if err := os.WriteFile(ledger, []byte(body), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runAudit(t, "verify-ledger", "--repo-root", root, "--ledger", ledger)
		if code != exitFail || !strings.Contains(strings.ToLower(stderr), "reason") {
			t.Fatalf("exit %d stderr %s", code, stderr)
		}
	})

	t.Run("bandit drift fails", func(t *testing.T) {
		body := goTarget +
			"[targets.\"svc-py\"]\nkind = \"python\"\n" +
			"[targets.\"svc-py\".scanners.ruff]\nstatus = \"canary\"\ncanary = \"python-canary\"\n" +
			"[targets.\"svc-py\".scanners.bandit]\nstatus = \"not-applicable\"\nreason = \"falsely marked not applicable\"\n" +
			"[targets.\"svc-py\".scanners.pyrefly]\nstatus = \"canary\"\ncanary = \"python-canary\"\n"
		ledger := filepath.Join(root, "drift.toml")
		if err := os.WriteFile(ledger, []byte(body), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runAudit(t, "verify-ledger", "--repo-root", root, "--ledger", ledger)
		if code != exitFail || !strings.Contains(stderr, "bandit") {
			t.Fatalf("exit %d stderr %s", code, stderr)
		}
	})

	t.Run("stale extra fails unless sparse", func(t *testing.T) {
		extra := validBody +
			"[targets.\"unmaterialized-svc\"]\nkind = \"python\"\n" +
			"[targets.\"unmaterialized-svc\".scanners.ruff]\nstatus = \"canary\"\ncanary = \"python-canary\"\n" +
			"[targets.\"unmaterialized-svc\".scanners.bandit]\nstatus = \"not-applicable\"\nreason = \"bandit not declared in dev deps\"\n" +
			"[targets.\"unmaterialized-svc\".scanners.pyrefly]\nstatus = \"not-applicable\"\nreason = \"pyrefly not declared in dev deps\"\n"
		ledger := filepath.Join(root, "stale.toml")
		if err := os.WriteFile(ledger, []byte(extra), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runAudit(t, "verify-ledger", "--repo-root", root, "--ledger", ledger, "--allow-sparse")
		if code != exitFail || !strings.Contains(stderr, "unmaterialized-svc") {
			t.Fatalf("exit %d stderr %s", code, stderr)
		}
		gitDir := filepath.Join(root, ".git")
		if err := os.Mkdir(gitDir, fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		cfg := "[core]\n\tsparseCheckout = true\n\tsparseCheckoutCone = true\n"
		if err := os.WriteFile(filepath.Join(gitDir, "config.worktree"), []byte(cfg), fsperm.File); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(gitDir) })
		code, _, stderr = runAudit(t, "verify-ledger", "--repo-root", root, "--ledger", ledger)
		if code != exitFail || !strings.Contains(stderr, "core.sparseCheckout=true") {
			t.Fatalf("strict sparse exit %d stderr %s", code, stderr)
		}
		code, stdout, stderr := runAudit(t, "verify-ledger", "--repo-root", root, "--ledger", ledger, "--allow-sparse")
		if code != exitOK || !strings.Contains(stdout, "sparse materialized=2") {
			t.Fatalf("allow-sparse exit %d stdout=%s stderr=%s", code, stdout, stderr)
		}
	})

	t.Run("run.sh failure fails closed", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(lintDir, "run.sh"), []byte("#!/bin/bash\necho boom >&2\nexit 3\n"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = os.WriteFile(filepath.Join(lintDir, "run.sh"), []byte(script), fsperm.Dir)
		})
		ledger := filepath.Join(root, "valid.toml")
		code, _, stderr := runAudit(t, "verify-ledger", "--repo-root", root, "--ledger", ledger)
		if code != exitFail || !strings.Contains(stderr, "LEDGER CHECK FAILED") {
			t.Fatalf("exit %d stderr %s", code, stderr)
		}
	})
}

func TestRealLedgerParses(t *testing.T) {
	t.Parallel()
	root, ok := findRepoRoot(".")
	if !ok {
		t.Fatal("repo root not found")
	}
	path := filepath.Join(root, "scripts", "lint", "canary_ledger.toml")
	targets, errText := loadLedgerTargets(path)
	if errText != "" {
		t.Fatal(errText)
	}
	if len(targets) < 45 {
		t.Fatalf("ledger targets = %d, want at least 45", len(targets))
	}
	proto, ok := targets["proto"]
	if !ok || proto.Kind != "proto" {
		t.Fatalf("proto target = %+v", proto)
	}
	repo := targets["repo"]
	actionlint, ok := repo.Scanners["actionlint"]
	if !ok || actionlint.Status != "canary" || actionlint.Canary != "actionlint-canary" {
		t.Fatalf("repo actionlint = %+v", actionlint)
	}
	backend := targets["alt-backend/app"]
	if backend.Kind != "go" || backend.Scanners["gosec"].Canary != "go-canary" {
		t.Fatalf("alt-backend/app = %+v", backend)
	}
	deps, err := readPythonDevDeps(filepath.Join(root, "tag-generator", "app", "pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bandit", "pyrefly", "ruff"} {
		if _, ok := deps[name]; !ok {
			t.Fatalf("dev deps missing %s: %v", name, deps)
		}
	}
}

func TestAuditCLI(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runAudit(t)
	if code != exitUsage || !strings.Contains(stderr, "command") {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, _ = runAudit(t, "--help")
	if code != exitOK || !strings.Contains(stdout, "verify-ledger") || !strings.Contains(stdout, "proto-canary") {
		t.Fatalf("help exit %d stdout %s", code, stdout)
	}
	code, _, stderr = runAudit(t, "nope")
	if code != exitUsage || !strings.Contains(stderr, "invalid choice") || !strings.Contains(stderr, "actionlint-canary") {
		t.Fatalf("exit %d stderr %s", code, stderr)
	}
	code, _, stderr = runAudit(t, "python-canary", "--project-dir", t.TempDir(), "--check-bandit", "maybe")
	if code != exitUsage || !strings.Contains(stderr, "invalid boolean value") {
		t.Fatalf("exit %d stderr %s", code, stderr)
	}
	var bufOut, bufErr bytes.Buffer
	if got := RunAudit(nil, []string{"--help"}, &bufOut, &bufErr); got != exitUsage {
		t.Fatalf("nil context exit %d", got)
	}
}

func TestEntrypointCLI(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := RunEntrypoint(context.Background(), []string{"--help"}, &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), "--languages") || !strings.Contains(stdout.String(), "--skip-baseline") {
		t.Fatalf("help exit %d stdout %s", code, stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = RunEntrypoint(context.Background(), []string{"--repo-root", t.TempDir(), "--languages", ","}, &stdout, &stderr)
	if code != exitFail || !strings.Contains(stderr.String(), "no languages selected") {
		t.Fatalf("empty languages exit %d stderr %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = RunEntrypoint(context.Background(), []string{"--repo-root", t.TempDir(), "--languages", "cobol"}, &stdout, &stderr)
	if code != exitUsage || !strings.Contains(stderr.String(), "unknown language") {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code = RunEntrypoint(ctx, []string{"--repo-root", t.TempDir(), "--languages", "rust,go,proto", "--skip-baseline"}, &stdout, &stderr)
	if code != exitFail {
		t.Fatalf("cancelled exit %d stdout %s stderr %s", code, stdout.String(), stderr.String())
	}
	text := stdout.String()
	rustAt := strings.Index(text, "[rust")
	goAt := strings.Index(text, "[go")
	protoAt := strings.Index(text, "[proto")
	if rustAt < 0 || goAt < 0 || protoAt < 0 || !(rustAt < goAt && goAt < protoAt) {
		t.Fatalf("summary order not deterministic:\n%s", text)
	}
	if !strings.Contains(text, "entrypoint canary summary: 0/3 passed") {
		t.Fatalf("summary %s", text)
	}
}

func TestLanguagePoolOrderAndBound(t *testing.T) {
	t.Parallel()
	started := make(chan string, 3)
	release := make(chan struct{})
	done := make(chan []caseResult, 1)
	go func() {
		done <- runLanguagePool(context.Background(), []string{"rust", "go", "proto"}, 2, time.Second, func(ctx context.Context, lang string) caseResult {
			started <- lang
			select {
			case <-ctx.Done():
				return caseResult{language: lang, target: lang, passed: false, detail: ctx.Err().Error()}
			case <-release:
				return caseResult{language: lang, target: "t-" + lang, passed: true, detail: "ok", duration: time.Millisecond}
			}
		})
	}()
	var got []string
	timeout := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case lang := <-started:
			got = append(got, lang)
		case <-timeout:
			t.Fatal("timed out waiting for the first two workers")
		}
	}
	select {
	case lang := <-started:
		t.Fatalf("third language %s started before a slot freed", lang)
	case <-time.After(80 * time.Millisecond):
	}
	close(release)
	var results []caseResult
	select {
	case results = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pool did not finish")
	}
	if len(results) != 3 || results[0].language != "rust" || results[1].language != "go" || results[2].language != "proto" {
		t.Fatalf("results %+v", results)
	}
	if !results[0].passed || !results[1].passed || !results[2].passed {
		t.Fatalf("results %+v started %v", results, got)
	}
}

func TestLanguagePoolContextCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := runLanguagePool(ctx, []string{"go", "python"}, 2, time.Second, func(ctx context.Context, lang string) caseResult {
		t.Errorf("runner invoked for %s after cancel", lang)
		return caseResult{language: lang, passed: true}
	})
	if len(results) != 2 || results[0].passed || results[1].passed {
		t.Fatalf("results %+v", results)
	}
	if results[0].language != "go" || results[1].language != "python" {
		t.Fatalf("order %+v", results)
	}
}

func TestActionlintCanaryStub(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stub := filepath.Join(dir, "actionlint")
	script := "#!/bin/sh\nfor tok in \"$@\"; do\n  if [ -f \"$tok\" ] && grep -q invalid_context \"$tok\"; then\n    echo \"$tok:7:23: undefined variable \\\"invalid_context\\\" [expression]\"\n    exit 1\n  fi\ndone\nexit 0\n"
	if err := os.WriteFile(stub, []byte(script), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runAudit(t, "actionlint-canary", "--actionlint-bin", stub)
	if code != exitOK || !strings.Contains(stdout, "canary audit passed (repo)") {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	silent := filepath.Join(dir, "silent")
	if err := os.WriteFile(silent, []byte("#!/bin/sh\nexit 0\n"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runAudit(t, "actionlint-canary", "--actionlint-bin", silent)
	if code != exitFail || !strings.Contains(stderr, "CANARY AUDIT FAILED") {
		t.Fatalf("exit %d stderr %s", code, stderr)
	}
}

func TestCanaryGroupsAToIStubs(t *testing.T) {
	t.Parallel()

	t.Run("Group A go-canary stub pass and silent fail", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		proj := filepath.Join(dir, "proj")
		if err := os.MkdirAll(proj, fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(proj, "go.mod"), []byte("module demo\n\ngo 1.27.4\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		cfg := filepath.Join(dir, ".golangci.yml")
		if err := os.WriteFile(cfg, []byte("version: \"2\"\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		good := filepath.Join(dir, "golangci-good")
		goodScript := `#!/bin/sh
sarif=""
while [ $# -gt 0 ]; do
  if [ "$1" = "--output.sarif.path" ]; then shift; sarif="$1"; fi
  shift
done
cat >"$sarif" <<'EOF'
{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"golangci-lint","rules":[]}},"results":[{"ruleId":"gosec","message":{"text":"G402: TLS InsecureSkipVerify"}},{"ruleId":"ineffassign","message":{"text":"ineffectual assignment"}}]}]}
EOF
exit 1
`
		if err := os.WriteFile(good, []byte(goodScript), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "go-canary", "--project-dir", proj, "--config", cfg, "--golangci-lint-bin", good)
		if code != exitOK || !strings.Contains(stdout, "canary audit passed (go)") {
			t.Fatalf("go-canary good: code=%d stdout=%s stderr=%s", code, stdout, stderr)
		}

		silent := filepath.Join(dir, "golangci-silent")
		if err := os.WriteFile(silent, []byte("#!/bin/sh\nexit 0\n"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		code, _, stderr = runAudit(t, "go-canary", "--project-dir", proj, "--config", cfg, "--golangci-lint-bin", silent)
		if code != exitFail || !strings.Contains(stderr, "CANARY AUDIT FAILED") {
			t.Fatalf("go-canary silent: code=%d stderr=%s", code, stderr)
		}
	})

	t.Run("Group A9 go-verify-govulncheck valid stream passes", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		proj := filepath.Join(dir, "proj")
		if err := os.MkdirAll(proj, fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(proj, "main.go"), []byte("package main\nfunc main() {}\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		sarif := filepath.Join(dir, "govuln.sarif")
		if err := os.WriteFile(sarif, []byte(`{"runs":[{"tool":{"driver":{"name":"govulncheck","rules":[],"properties":{"db":"https://vuln.go.dev","go_version":"go1.27.4"}}}}]}`), fsperm.File); err != nil {
			t.Fatal(err)
		}
		stream := filepath.Join(dir, "govuln.json")
		if err := os.WriteFile(stream, []byte("{\"config\":{\"db\":\"https://vuln.go.dev\",\"go_version\":\"go1.27.4\"}}\n{\"SBOM\":{\"modules\":[{\"path\":\"demo\"}],\"roots\":[\"demo\"]}}\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "go-verify-govulncheck", "--project-dir", proj, "--govulncheck-sarif", sarif, "--govulncheck-json", stream)
		if code != exitOK || !strings.Contains(stdout, "govulncheck scan check passed") {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
		}
	})

	t.Run("Group B python-verify-bandit-scan zero vs nonzero", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		proj := filepath.Join(dir, "proj")
		if err := os.MkdirAll(proj, fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(proj, "app.py"), []byte("x = 1\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		zeroOut := filepath.Join(dir, "zero.out")
		if err := os.WriteFile(zeroOut, []byte("Run metrics:\n  Total lines of code: 0\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runAudit(t, "python-verify-bandit-scan", "--project-dir", proj, "--bandit-out", zeroOut)
		if code != exitFail || !strings.Contains(stderr, "ZERO-FILE SCAN DETECTED") {
			t.Fatalf("zero bandit: code=%d stderr=%s", code, stderr)
		}
		okOut := filepath.Join(dir, "ok.out")
		if err := os.WriteFile(okOut, []byte("Run metrics:\n  Total lines of code: 42\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "python-verify-bandit-scan", "--project-dir", proj, "--bandit-out", okOut)
		if code != exitOK || !strings.Contains(stdout, "42 lines of code scanned") {
			t.Fatalf("ok bandit: code=%d stdout=%s stderr=%s", code, stdout, stderr)
		}
	})

	t.Run("Group C rust-canary stub", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		proj := filepath.Join(dir, "crate")
		if err := os.MkdirAll(proj, fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		tc := filepath.Join(proj, "rust-toolchain.toml")
		if err := os.WriteFile(tc, []byte("[toolchain]\nchannel = \"1.86.0\"\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		good := filepath.Join(dir, "cargo-good")
		if err := os.WriteFile(good, []byte("#!/bin/sh\necho 'error: equal expressions as operands to `==` (clippy::eq_op)' >&2\nexit 101\n"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "rust-canary", "--project-dir", proj, "--cargo-bin", good, "--target-dir", filepath.Join(dir, "target"))
		if code != exitOK || !strings.Contains(stdout, "canary audit passed (rust)") {
			t.Fatalf("rust good: code=%d stdout=%s stderr=%s", code, stdout, stderr)
		}
		silent := filepath.Join(dir, "cargo-silent")
		if err := os.WriteFile(silent, []byte("#!/bin/sh\nexit 0\n"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		code, _, stderr = runAudit(t, "rust-canary", "--project-dir", proj, "--cargo-bin", silent, "--target-dir", filepath.Join(dir, "target"))
		if code != exitFail || !strings.Contains(stderr, "CANARY AUDIT FAILED") {
			t.Fatalf("rust silent: code=%d stderr=%s", code, stderr)
		}
	})

	t.Run("Group D frontend-canary stub", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		good := filepath.Join(dir, "biome-good")
		if err := os.WriteFile(good, []byte("#!/bin/sh\necho 'lint/suspicious/noDebugger' >&2\nexit 1\n"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "frontend-canary", "--project-dir", dir, "--biome-bin", good)
		if code != exitOK || !strings.Contains(stdout, "canary audit passed (frontend)") {
			t.Fatalf("frontend good: code=%d stdout=%s stderr=%s", code, stdout, stderr)
		}
		silent := filepath.Join(dir, "biome-silent")
		if err := os.WriteFile(silent, []byte("#!/bin/sh\nexit 0\n"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		code, _, stderr = runAudit(t, "frontend-canary", "--project-dir", dir, "--biome-bin", silent)
		if code != exitFail || !strings.Contains(stderr, "CANARY AUDIT FAILED") {
			t.Fatalf("frontend silent: code=%d stderr=%s", code, stderr)
		}
	})

	t.Run("Group E deno-canary stub", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		good := filepath.Join(dir, "deno-good")
		if err := os.WriteFile(good, []byte("#!/bin/sh\necho '(no-debugger) debugger statement is not allowed' >&2\nexit 1\n"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "deno-canary", "--project-dir", dir, "--deno-bin", good)
		if code != exitOK || !strings.Contains(stdout, "canary audit passed (deno)") {
			t.Fatalf("deno good: code=%d stdout=%s stderr=%s", code, stdout, stderr)
		}
		silent := filepath.Join(dir, "deno-silent")
		if err := os.WriteFile(silent, []byte("#!/bin/sh\nexit 0\n"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		code, _, stderr = runAudit(t, "deno-canary", "--project-dir", dir, "--deno-bin", silent)
		if code != exitFail || !strings.Contains(stderr, "CANARY AUDIT FAILED") {
			t.Fatalf("deno silent: code=%d stderr=%s", code, stderr)
		}
	})

	t.Run("Group F proto-canary stub", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		good := filepath.Join(dir, "buf-good")
		if err := os.WriteFile(good, []byte("#!/bin/sh\necho 'canary.proto:5:6:Enum name \"bad_enum\" should be PascalCase' >&2\nexit 100\n"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runAudit(t, "proto-canary", "--buf-bin", good)
		if code != exitOK || !strings.Contains(stdout, "canary audit passed (proto)") {
			t.Fatalf("proto good: code=%d stdout=%s stderr=%s", code, stdout, stderr)
		}
		silent := filepath.Join(dir, "buf-silent")
		if err := os.WriteFile(silent, []byte("#!/bin/sh\nexit 0\n"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		code, _, stderr = runAudit(t, "proto-canary", "--buf-bin", silent)
		if code != exitFail || !strings.Contains(stderr, "CANARY AUDIT FAILED") {
			t.Fatalf("proto silent: code=%d stderr=%s", code, stderr)
		}
	})

	t.Run("Group H real repository verify-ledger passes", func(t *testing.T) {
		t.Parallel()
		root, ok := findRepoRoot(".")
		if !ok {
			t.Fatal("repo root not found")
		}
		code, stdout, stderr := runAudit(t, "verify-ledger", "--repo-root", root)
		if code != exitOK || !strings.Contains(stdout, "canary ledger verified:") {
			t.Fatalf("real verify-ledger: code=%d stdout=%s stderr=%s", code, stdout, stderr)
		}
	})

	t.Run("Group I stageCommon requires no python scripts", func(t *testing.T) {
		t.Parallel()
		src := t.TempDir()
		if err := os.MkdirAll(filepath.Join(src, "scripts", "lint"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "scripts", "lint", "run.sh"), []byte("#!/bin/sh\nexit 0\n"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		dst := t.TempDir()
		if err := stageCommon(context.Background(), src, dst); err != nil {
			t.Fatalf("stageCommon without .py scripts failed: %v", err)
		}
		if !isFile(filepath.Join(dst, "scripts", "lint", "run.sh")) {
			t.Fatal("staged run.sh missing")
		}
	})
}

func runAudit(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := RunAudit(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}
