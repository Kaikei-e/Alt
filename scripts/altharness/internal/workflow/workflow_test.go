package workflow

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSwallowedFailureRequiresReason(t *testing.T) {
	unannotated := "jobs:\n  test:\n    steps:\n      - run: grep -q foo bar || true\n"
	errors := checkWorkflowText("sample.yaml", unannotated)
	if len(errors) != 1 || !strings.Contains(errors[0], "sample.yaml:4: swallowed failure without a reason.") || !strings.Contains(errors[0], "# swallow-ok:") {
		t.Fatalf("unannotated: %#v", errors)
	}
	same := "jobs:\n  test:\n    steps:\n      - run: grep -q foo bar || true  # swallow-ok: grep exits 1 when no match\n"
	if errs := checkWorkflowText("sample.yaml", same); len(errs) != 0 {
		t.Fatalf("same line: %#v", errs)
	}
	prev := "jobs:\n  test:\n    steps:\n      - run: |\n          # swallow-ok: grep exits 1 when nothing matches\n\n          matches=$(grep foo bar || true)\n"
	if errs := checkWorkflowText("sample.yaml", prev); len(errs) != 0 {
		t.Fatalf("prev line: %#v", errs)
	}
	short := "jobs:\n  test:\n    steps:\n      - run: grep -q foo bar || true  # swallow-ok: too short\n"
	if len(checkWorkflowText("sample.yaml", short)) != 1 {
		t.Fatal("short same-line reason should fail")
	}
	shortPrev := "jobs:\n  test:\n    steps:\n      - run: |\n          # swallow-ok: short\n          grep -q foo bar || true\n"
	if len(checkWorkflowText("sample.yaml", shortPrev)) != 1 {
		t.Fatal("short previous-line reason should fail")
	}
	coe := "jobs:\n  test:\n    steps:\n      - name: Upload report\n        continue-on-error:  true\n        run: echo upload\n"
	coeErrs := checkWorkflowText("sample.yaml", coe)
	if len(coeErrs) != 1 || !strings.Contains(coeErrs[0], "sample.yaml:5: swallowed failure without a reason.") {
		t.Fatalf("coe: %#v", coeErrs)
	}
	coeOK := "jobs:\n  test:\n    steps:\n      - name: Upload report\n        # swallow-ok: optional telemetry upload must not block CI\n        continue-on-error: true\n        run: echo upload\n"
	if errs := checkWorkflowText("sample.yaml", coeOK); len(errs) != 0 {
		t.Fatalf("annotated coe: %#v", errs)
	}
	if errs := checkWorkflowText("sample.yaml", "jobs:\n  test:\n    steps:\n      - continue-on-error: false\n"); len(errs) != 0 {
		t.Fatalf("coe false: %#v", errs)
	}
	commented := "jobs:\n  test:\n    steps:\n      # - run: grep foo bar || true\n      - run: |\n          # old command: foo || true\n          echo ok\n"
	if errs := checkWorkflowText("sample.yaml", commented); len(errs) != 0 {
		t.Fatalf("comments: %#v", errs)
	}
	for _, snippet := range []string{
		"      - run: curl -fsS http://localhost || echo '{}'\n",
		"      - run: some-cmd ||  exit 0\n",
		"      - run: some-cmd || :\n",
		"      - run: |\n          set +e\n          some-cmd\n",
		"      - run: set +e; some-cmd\n",
	} {
		errs := checkWorkflowText("sample.yaml", snippet)
		if len(errs) != 1 || !strings.Contains(errs[0], "swallowed failure without a reason.") {
			t.Fatalf("snippet %q -> %#v", snippet, errs)
		}
	}
	setOK := "jobs:\n  test:\n    steps:\n      - run: |\n          # swallow-ok: capture diagnostic exit code from probe command\n          set +e\n          probe-cmd\n"
	if errs := checkWorkflowText("sample.yaml", setOK); len(errs) != 0 {
		t.Fatalf("annotated set +e: %#v", errs)
	}
}

func TestLintJobsForbidSwallows(t *testing.T) {
	header := "name: sample\non:\n  pull_request:\npermissions:\n  contents: read\n"
	for _, jobName := range []string{"lint", "quality-gates", "static-checks"} {
		bodies := []string{
			"jobs:\n  " + jobName + ":\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    # swallow-ok: attempting to bypass lint failure\n    continue-on-error: true\n    steps:\n      - run: ./scripts/lint/run.sh repo\n",
			"jobs:\n  " + jobName + ":\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - name: Lint\n        # swallow-ok: attempting to bypass lint failure\n        continue-on-error: true\n        run: ./scripts/lint/run.sh repo\n",
			"jobs:\n  " + jobName + ":\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - run: ./scripts/lint/run.sh repo || true  # swallow-ok: trying to ignore lint error\n",
			"jobs:\n  " + jobName + ":\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - run: |\n          # swallow-ok: trying to ignore lint error\n          ./scripts/lint/run.sh repo || :\n",
			"jobs:\n  " + jobName + ":\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - run: ./scripts/lint/run.sh repo || echo ok  # swallow-ok: trying to ignore lint error\n",
			"jobs:\n  " + jobName + ":\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - run: ./scripts/lint/run.sh repo || exit 0  # swallow-ok: trying to ignore lint error\n",
			"jobs:\n  " + jobName + ":\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - run: |\n          # swallow-ok: trying to disable errexit in lint job\n          set +e\n          ./scripts/lint/run.sh repo\n",
		}
		for _, body := range bodies {
			text := header + body
			if len(checkWorkflowText("sample.yaml", text)) < 1 {
				t.Fatalf("text accepted lint swallow for %s:\n%s", jobName, body)
			}
			doc, err := parseYAML(text)
			if err != nil {
				t.Fatalf("parse: %v\n%s", err, text)
			}
			if len(checkWorkflowDoc("sample.yaml", doc)) < 1 {
				t.Fatalf("doc accepted lint swallow for %s:\n%s", jobName, body)
			}
		}
	}
	mixed := header + "jobs:\n  python-job:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - name: Diagnostics\n        run: |\n          # swallow-ok: diagnostic directory listing must not fail the job\n          ls -la . || true\n      - name: Run lint\n        run: ./scripts/lint/run.sh scripts\n"
	if errs := checkWorkflowText("sample.yaml", mixed); len(errs) != 0 {
		t.Fatalf("mixed text: %#v", errs)
	}
	doc, err := parseYAML(mixed)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDoc("sample.yaml", doc); len(errs) != 0 {
		t.Fatalf("mixed doc: %#v", errs)
	}
	bypasses := []string{
		"jobs:\n  lint:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - run: |\n          echo \"step #1\" && ./scripts/lint/run.sh repo || true\n",
		"jobs:\n  execute:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    # swallow-ok: attempting job-level swallow on execute job\n    continue-on-error: true\n    steps:\n      - run: ./scripts/lint/run.sh \"$PROJECT_DIR\"\n",
		"jobs:\n  execute:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - name: Conditional lint step\n        if: ${{ inputs.job-type == 'lint' }}\n        run: |\n          # swallow-ok: trying to swallow conditional lint step\n          set +e\n          echo running\n",
		"jobs:\n  lint:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    continue-on-error: ${{ true }}\n    steps:\n      - run: ./scripts/lint/run.sh repo\n",
		"jobs:\n  execute:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - name: Run lint\n        continue-on-error: ${{ true }}\n        run: ./scripts/lint/run.sh repo\n",
		"jobs:\n  lint:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - run: |\n          # swallow-ok: trying set +o errexit\n          set +o errexit\n          ./scripts/lint/run.sh repo\n",
		"jobs:\n  execute:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - run: |\n          \"$GITHUB_WORKSPACE/scripts/lint/run.sh\" repo || true  # swallow-ok: trying path prefix bypass\n",
	}
	for _, body := range bypasses {
		text := header + body
		if len(checkWorkflowText("sample.yaml", text)) < 1 {
			t.Fatalf("text accepted bypass:\n%s", body)
		}
		doc, err := parseYAML(text)
		if err != nil {
			t.Fatalf("parse bypass: %v\n%s", err, text)
		}
		if len(checkWorkflowDoc("sample.yaml", doc)) < 1 {
			t.Fatalf("doc accepted bypass:\n%s", body)
		}
	}
}

func TestDirectLintAndAllowed(t *testing.T) {
	forbidden := []string{
		"golangci-lint run ./...",
		"gofmt -l .",
		"go vet ./...",
		"ruff check .",
		"uv run --no-sync ruff check .",
		"python3 -m bandit -r .",
		"bunx svelte-check",
		"files=$(gofmt -l .)",
		"go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12",
		"env FOO=1 cargo clippy",
		"docdag validate --format github",
		"python3 scripts/compose-port-audit.py --overlays",
		"bash scripts/tests/test_compose_wiring.sh",
		"python3 .github/workflows/scripts/workflow-safety-check.py",
		"buf lint",
		"deno lint",
		"promtool check rules x.yml",
		"lkal check --json deploy/desired-env.lkal",
	}
	for _, cmd := range forbidden {
		text := baseWorkflowYAML("      - run: " + yamlQuote(cmd) + "\n")
		doc, err := parseYAML(text)
		if err != nil {
			t.Fatalf("parse %q: %v", cmd, err)
		}
		errs := checkWorkflowDoc("sample.yaml", doc)
		if !containsAll(errs, "scripts/lint/run.sh") {
			t.Fatalf("expected direct-lint error for %q, got %#v", cmd, errs)
		}
	}
	allowed := baseWorkflowYAML(`      - run: ./scripts/lint/run.sh "$TARGET"
      - run: |
          # comment: golangci-lint gofmt ruff
          echo "Run gofmt -w or ruff check or golangci-lint"
      - run: go install github.com/securego/gosec/v2/cmd/gosec@v2.22.11
      - run: go test ./...
      - run: cargo test --locked
      - run: python3 scripts/tests/test-compose-image-audit.py
      - uses: actions/cache@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
        with:
          path: ~/.cache
          key: golangci-lint-ruff-biome
      - uses: bufbuild/buf-action@bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
        with:
          setup_only: true
`)
	doc, err := parseYAML(allowed)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDoc("sample.yaml", doc); len(errs) != 0 {
		t.Fatalf("allowed: %#v", errs)
	}
	digest := strings.Repeat("a", 64)
	forbiddenUses := []string{
		"golangci/golangci-lint-action@" + strings.Repeat("c", 40),
		"docker://rhysd/actionlint:1.7.12@sha256:" + digest,
		"bufbuild/buf-action@" + strings.Repeat("1", 40),
	}
	for _, uses := range forbiddenUses {
		text := baseWorkflowYAML("      - uses: " + uses + "\n")
		doc, err := parseYAML(text)
		if err != nil {
			t.Fatalf("parse uses %s: %v", uses, err)
		}
		if !containsAll(checkWorkflowDoc("sample.yaml", doc), "scripts/lint/run.sh") {
			t.Fatalf("expected lint-action error for %s", uses)
		}
	}
}

func TestContainerTimeoutPathsAndTriggers(t *testing.T) {
	chdir(t, repoRoot(t))
	unpinned := baseWorkflowYAML("      - run: \"true\"\n")
	// rebuild with container by parsing a full document
	text := strings.Replace(baseWorkflowYAML("      - run: \"true\"\n"), "    timeout-minutes: 10\n", "    timeout-minutes: 10\n    container: mcr.microsoft.com/playwright:v1.62.1-jammy\n", 1)
	doc, err := parseYAML(text)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(checkWorkflowDoc("sample.yaml", doc), "container", "@sha256:") {
		t.Fatalf("container: %#v", checkWorkflowDoc("sample.yaml", doc))
	}
	pinned := strings.Replace(text, "container: mcr.microsoft.com/playwright:v1.62.1-jammy\n", "container: mcr.microsoft.com/playwright:v1.62.1-jammy@sha256:"+strings.Repeat("a", 64)+"\n", 1)
	doc, err = parseYAML(pinned)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDoc("sample.yaml", doc); len(errs) != 0 {
		t.Fatalf("pinned container: %#v", errs)
	}
	_ = unpinned

	missingTimeout := "name: sample\non:\n  pull_request:\npermissions:\n  contents: read\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: \"true\"\n"
	doc, err = parseYAML(missingTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(checkWorkflowDoc("sample.yaml", doc), "timeout-minutes") {
		t.Fatal("expected timeout error")
	}
	expr := strings.Replace(missingTimeout, "runs-on: ubuntu-latest\n", "runs-on: ubuntu-latest\n    timeout-minutes: ${{ inputs.timeout-minutes }}\n", 1)
	doc, err = parseYAML(expr)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDoc("sample.yaml", doc); len(errs) != 0 {
		t.Fatalf("expr timeout: %#v", errs)
	}
	reusable := "name: sample\non:\n  pull_request:\npermissions:\n  contents: read\njobs:\n  test:\n    uses: ./.github/workflows/reusable-test-go.yaml\n    with:\n      working-directory: alt-backend/app\n"
	doc, err = parseYAML(reusable)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDoc("sample.yaml", doc); len(errs) != 0 {
		t.Fatalf("reusable: %#v", errs)
	}
	forbidden := "name: sample\non:\n  pull_request_target:\n  workflow_run:\npermissions:\n  contents: read\njobs:\n  test:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - run: \"true\"\n"
	doc, err = parseYAML(forbidden)
	if err != nil {
		t.Fatal(err)
	}
	errs := checkWorkflowDoc("sample.yaml", doc)
	if !containsAll(errs, "pull_request_target") || !containsAll(errs, "workflow_run") {
		t.Fatalf("triggers: %#v", errs)
	}

	wfPath := ".github/workflows/backend-go.yaml"
	missing := "name: backend-go\non:\n  push:\n    paths:\n      - alt-backend/**\n  pull_request:\n    paths:\n      - alt-backend/**\npermissions:\n  contents: read\njobs:\n  unit:\n    uses: ./.github/workflows/reusable-test-go.yaml\n  gates:\n    uses: ./.github/workflows/reusable-go-quality-gates.yaml\n"
	doc, err = parseYAML(missing)
	if err != nil {
		t.Fatal(err)
	}
	errs = checkWorkflowDoc(wfPath, doc)
	if !containsAll(errs, "backend-go.yaml") || !containsAll(errs, "on.push.paths") || !containsAll(errs, "reusable-test-go.yaml") {
		t.Fatalf("missing paths: %#v", errs)
	}
	dot := "name: backend-go\non:\n  push:\n    paths:\n      - alt-backend/**\n      - github/workflows/backend-go.yaml\n      - github/workflows/reusable-test-go.yaml\npermissions:\n  contents: read\njobs:\n  unit:\n    uses: ./.github/workflows/reusable-test-go.yaml\n"
	doc, err = parseYAML(dot)
	if err != nil {
		t.Fatal(err)
	}
	errs = checkWorkflowDoc(wfPath, doc)
	if !containsAll(errs, "on.push.paths does not match '.github/workflows/backend-go.yaml'") {
		t.Fatalf("leading dot: %#v", errs)
	}
	for _, badPat := range []string{
		".github/workflows/backend-go.y?ml",
		".github/workflows/backend-go.+",
		".github/workflows/[b]ackend-go.yaml",
	} {
		bad := "name: backend-go\non:\n  push:\n    paths:\n      - '" + badPat + "'\n      - .github/workflows/backend-go.yaml\n      - .github/workflows/reusable-test-go.yaml\npermissions:\n  contents: read\njobs:\n  unit:\n    uses: ./.github/workflows/reusable-test-go.yaml\n"
		doc, err = parseYAML(bad)
		if err != nil {
			t.Fatal(err)
		}
		errs = checkWorkflowDoc(wfPath, doc)
		if !containsAll(errs, badPat) || !containsAll(errs, "unsupported") {
			t.Fatalf("glob %s: %#v", badPat, errs)
		}
	}
}

func TestRepoMatchesCleanContract(t *testing.T) {
	root := repoRoot(t)
	chdir(t, root)
	files, err := workflowFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected workflow files")
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		if errs := checkWorkflowText(path, text); len(errs) != 0 {
			t.Fatalf("%s text errors:\n%s", path, strings.Join(errs, "\n"))
		}
		doc, err := parseYAML(text)
		if err != nil {
			t.Fatalf("%s parse: %v", path, err)
		}
		if errs := checkWorkflowDoc(path, doc); len(errs) != 0 {
			t.Fatalf("%s doc errors:\n%s", path, strings.Join(errs, "\n"))
		}
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run code %d\nstderr:\n%s", code, stderr.String())
	}
	want := "OK: no forbidden triggers, GitHub-hosted labels only, permissions declared, checkouts credential-free, no untrusted context in run bodies, every action SHA-pinned, path filters resolve (rule 8) and match own and called reusable workflows (rule 11), every container/service image digest-pinned, job timeouts declared, and swallowed failures carry reasons (rule 12).\nOK: client vitest project is run by .github/workflows/alt-frontend-sv-unit-test.yaml::component-test.\n"
	if stdout.String() != want {
		t.Fatalf("stdout:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr: %s", stderr.String())
	}
}

func TestFailClosedWhenNoWorkflows(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), nil, &stdout, &stderr)
	if code == 0 {
		t.Fatal("empty tree exited 0")
	}
	if !strings.Contains(stderr.String(), "no workflow files found under .github/workflows") {
		t.Fatalf("stderr: %s", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout: %s", stdout.String())
	}
}

func baseWorkflowYAML(steps string) string {
	return "name: sample\non:\n  pull_request:\npermissions:\n  contents: read\njobs:\n  test:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n" + steps
}

func yamlQuote(s string) string {
	return "\"" + strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), "\"", "\\\"") + "\""
}

func containsAll(errs []string, parts ...string) bool {
	blob := strings.Join(errs, "\n")
	for _, part := range parts {
		if !strings.Contains(blob, part) {
			return false
		}
	}
	return true
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}
