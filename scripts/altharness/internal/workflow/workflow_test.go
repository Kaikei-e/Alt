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
	t.Parallel()
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
	t.Parallel()
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
	for _, lintCmd := range []string{"./scripts/lint/run.sh scripts", "make lint TARGET=scripts", "./scripts/altharness/alt-harness adr-gate --base origin/main"} {
		for _, variant := range []string{
			"      - name: Run lint\n        # swallow-ok: should still be forbidden\n        continue-on-error: true\n        run: " + lintCmd + "\n",
			"      - name: Run lint\n        run: " + lintCmd + " || true  # swallow-ok: should still be forbidden\n",
			"      - name: Run lint\n        run: |\n          # swallow-ok: should still be forbidden\n          set +e\n          " + lintCmd + "\n",
		} {
			wfText := header + "jobs:\n  python-job:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n" + variant
			if len(checkWorkflowText("sample.yaml", wfText)) < 1 {
				t.Fatalf("text accepted lint step swallow for %s:\n%s", lintCmd, variant)
			}
			d, err := parseYAML(wfText)
			if err != nil {
				t.Fatal(err)
			}
			if len(checkWorkflowDoc("sample.yaml", d)) < 1 {
				t.Fatalf("doc accepted lint step swallow for %s:\n%s", lintCmd, variant)
			}
		}
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
	t.Parallel()
	forbidden := []string{
		"golangci-lint run ./...",
		"gofmt -l .",
		"go vet ./...",
		"gosec ./...",
		"govulncheck ./...",
		"ruff check .",
		"bandit -r .",
		"pyrefly check",
		"cargo fmt --check",
		"cargo clippy -- -D warnings",
		"biome check .",
		"svelte-check",
		"deno fmt --check",
		"deno lint",
		"actionlint -shellcheck=",
		"buf lint",
		"buf breaking --against origin/main",
		"uv run --no-sync ruff check .",
		"python3 -m bandit -r .",
		"bunx svelte-check",
		"files=$(gofmt -l .)",
		"\"$GOPATH/bin/govulncheck\" ./...",
		"go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12",
		"env FOO=1 cargo clippy",
		"docdag validate --format github --immutable-since origin/main",
		"docdag lint --strict --format github",
		"promtool check rules observability/prometheus/rules/*.yml",
		"amtool check-config observability/alertmanager/alertmanager.yml",
		"lkal check --json deploy/desired-env.lkal",
		"python3 scripts/compose-port-audit.py --overlays",
		"python3 scripts/compose-netns-cascade-audit.py",
		"python3 scripts/compose-feature-gate.py --gate-report",
		"python3 scripts/compose-image-audit.py",
		"./scripts/check-compose-variables.sh",
		"bash scripts/tests/test_compose_wiring.sh",
		"python3 scripts/kisage-env.py verify",
		"python3 scripts/regen-service-registry.py --check",
		"scripts/observability-validate.sh",
		"python3 scripts/agent-harness/harness_selfcheck.py",
		"python3 scripts/agent-harness/postmortem_ledger.py --check --base origin/main",
		"python3 scripts/agent-harness/migration_destructive_gate.py --base origin/main",
		"python3 scripts/agent-harness/adr_supersede_gate.py --base origin/main",
		"bash e2e/playwright/_lib/check-suites.sh",
		"python3 .github/workflows/scripts/workflow-safety-check.py",
		"./scripts/altharness/alt-harness selfcheck",
		"./scripts/altharness/alt-harness postmortem --check --base origin/main",
		"./scripts/altharness/alt-harness migration-gate --base origin/main",
		"./scripts/altharness/alt-harness adr-gate --base origin/main",
		"./scripts/altharness/alt-harness workflow",
		"./scripts/altharness/alt-harness ratchet",
		"./scripts/altharness/alt-harness suppressions --base origin/main",
		"./scripts/altharness/alt-harness generated --base origin/main",
		"./scripts/altharness/alt-harness hygiene",
		"./scripts/altharness/alt-harness pyver",
		"./scripts/altharness/alt-harness sync-go --check",
		"./scripts/altharness/alt-harness pki --check",
		"./scripts/altharness/alt-harness lang go alt-backend/app",
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
	// adr-graph.yml is explicitly exempted for `alt-harness adr-gate`.
	adrGraphDoc, err := parseYAML(baseWorkflowYAML("      - run: ./scripts/altharness/alt-harness adr-gate --base origin/main\n"))
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDoc(".github/workflows/adr-graph.yml", adrGraphDoc); len(errs) != 0 {
		t.Fatalf("expected adr-graph.yml exemption for alt-harness adr-gate, got %#v", errs)
	}
	allowed := baseWorkflowYAML(`      - run: ./scripts/lint/run.sh "$TARGET"
      - run: ./scripts/altharness/alt-harness lint "$TARGET"
      - run: ./scripts/altharness/alt-harness --help
      - run: |
          # comment: golangci-lint gofmt ruff
          echo "Run gofmt -w or ruff check or golangci-lint"
          printf '::error title=gofmt::Run gofmt -w\n'
      - run: go install github.com/securego/gosec/v2/cmd/gosec@v2.22.11
      - run: go install "github.com/Kaikei-e/DocDag/cmd/docdag@v0.6.0"
      - run: make install-lkal install-kisage
      - run: pip install --quiet ruff bandit pyrefly
      - run: uv tool install ruff
      - run: go test ./...
      - run: go build ./...
      - run: cargo test --locked
      - run: deno test --allow-all tests/
      - run: buf build --exclude-source-info -o out.binpb
      - run: python3 scripts/tests/test-compose-image-audit.py
      - run: python3 scripts/tests/test-kisage-env.py
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
		"astral-sh/ruff-action@" + strings.Repeat("d", 40),
		"bufbuild/buf-lint-action@" + strings.Repeat("e", 40),
		"bufbuild/buf-breaking-action@" + strings.Repeat("f", 40),
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
	t.Parallel()
	root := repoRoot(t)
	digestA := strings.Repeat("a", 64)
	digestB := strings.Repeat("b", 64)

	// Container string requires @sha256
	text := strings.Replace(baseWorkflowYAML("      - run: \"true\"\n"), "    timeout-minutes: 10\n", "    timeout-minutes: 10\n    container: mcr.microsoft.com/playwright:v1.62.1-jammy\n", 1)
	doc, err := parseYAML(text)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(checkWorkflowDocIn("sample.yaml", doc, root), "container", "@sha256:") {
		t.Fatalf("container: %#v", checkWorkflowDocIn("sample.yaml", doc, root))
	}
	pinned := strings.Replace(text, "container: mcr.microsoft.com/playwright:v1.62.1-jammy\n", "container: mcr.microsoft.com/playwright:v1.62.1-jammy@sha256:"+digestA+"\n", 1)
	doc, err = parseYAML(pinned)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDocIn("sample.yaml", doc, root); len(errs) != 0 {
		t.Fatalf("pinned container: %#v", errs)
	}

	// Container mapping requires @sha256
	mapUnpinned := strings.Replace(baseWorkflowYAML("      - run: \"true\"\n"), "    timeout-minutes: 10\n", "    timeout-minutes: 10\n    container:\n      image: mcr.microsoft.com/playwright:v1.62.1-jammy\n", 1)
	doc, err = parseYAML(mapUnpinned)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(checkWorkflowDocIn("sample.yaml", doc, root), "container", "@sha256:") {
		t.Fatalf("container mapping: %#v", checkWorkflowDocIn("sample.yaml", doc, root))
	}
	mapPinned := strings.Replace(mapUnpinned, "image: mcr.microsoft.com/playwright:v1.62.1-jammy\n", "image: mcr.microsoft.com/playwright:v1.62.1-jammy@sha256:"+digestA+"\n", 1)
	doc, err = parseYAML(mapPinned)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDocIn("sample.yaml", doc, root); len(errs) != 0 {
		t.Fatalf("pinned container mapping: %#v", errs)
	}

	// Service image requires @sha256
	svcUnpinned := strings.Replace(baseWorkflowYAML("      - run: \"true\"\n"), "    timeout-minutes: 10\n", "    timeout-minutes: 10\n    services:\n      pact-db:\n        image: postgres:16-alpine\n", 1)
	doc, err = parseYAML(svcUnpinned)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(checkWorkflowDocIn("sample.yaml", doc, root), "pact-db", "@sha256:") {
		t.Fatalf("service unpinned: %#v", checkWorkflowDocIn("sample.yaml", doc, root))
	}
	svcPinned := strings.Replace(svcUnpinned, "image: postgres:16-alpine\n", "image: postgres:16-alpine@sha256:"+digestB+"\n", 1)
	doc, err = parseYAML(svcPinned)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDocIn("sample.yaml", doc, root); len(errs) != 0 {
		t.Fatalf("service pinned: %#v", errs)
	}

	// Timeout validation: missing, zero, literal string, expression, reusable caller
	missingTimeout := "name: sample\non:\n  pull_request:\npermissions:\n  contents: read\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: \"true\"\n"
	for _, badTimeout := range []string{
		missingTimeout,
		strings.Replace(missingTimeout, "runs-on: ubuntu-latest\n", "runs-on: ubuntu-latest\n    timeout-minutes: 0\n", 1),
		strings.Replace(missingTimeout, "runs-on: ubuntu-latest\n", "runs-on: ubuntu-latest\n    timeout-minutes: ten\n", 1),
	} {
		doc, err = parseYAML(badTimeout)
		if err != nil {
			t.Fatal(err)
		}
		if !containsAll(checkWorkflowDocIn("sample.yaml", doc, root), "timeout-minutes") {
			t.Fatalf("expected timeout error for:\n%s", badTimeout)
		}
	}
	expr := strings.Replace(missingTimeout, "runs-on: ubuntu-latest\n", "runs-on: ubuntu-latest\n    timeout-minutes: ${{ inputs.timeout-minutes }}\n", 1)
	doc, err = parseYAML(expr)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDocIn("sample.yaml", doc, root); len(errs) != 0 {
		t.Fatalf("expr timeout: %#v", errs)
	}
	reusable := "name: sample\non:\n  pull_request:\npermissions:\n  contents: read\njobs:\n  test:\n    uses: ./.github/workflows/reusable-test-go.yaml\n    with:\n      working-directory: alt-backend/app\n"
	doc, err = parseYAML(reusable)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDocIn("sample.yaml", doc, root); len(errs) != 0 {
		t.Fatalf("reusable: %#v", errs)
	}

	// Forbidden triggers
	forbidden := "name: sample\non:\n  pull_request_target:\n  workflow_run:\npermissions:\n  contents: read\njobs:\n  test:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - run: \"true\"\n"
	doc, err = parseYAML(forbidden)
	if err != nil {
		t.Fatal(err)
	}
	errs := checkWorkflowDocIn("sample.yaml", doc, root)
	if !containsAll(errs, "pull_request_target") || !containsAll(errs, "workflow_run") {
		t.Fatalf("triggers: %#v", errs)
	}

	// Paths filters
	wfPath := ".github/workflows/backend-go.yaml"
	missing := "name: backend-go\non:\n  push:\n    paths:\n      - alt-backend/**\n  pull_request:\n    paths:\n      - alt-backend/**\npermissions:\n  contents: read\njobs:\n  unit:\n    uses: ./.github/workflows/reusable-test-go.yaml\n  gates:\n    uses: ./.github/workflows/reusable-go-quality-gates.yaml\n"
	doc, err = parseYAML(missing)
	if err != nil {
		t.Fatal(err)
	}
	errs = checkWorkflowDocIn(wfPath, doc, root)
	if !containsAll(errs, "backend-go.yaml") || !containsAll(errs, "on.push.paths") || !containsAll(errs, "reusable-test-go.yaml") || !containsAll(errs, "reusable-go-quality-gates.yaml") {
		t.Fatalf("missing paths: %#v", errs)
	}

	validExplicit := "name: backend-go\non:\n  push:\n    paths:\n      - alt-backend/**\n      - .github/workflows/backend-go.yaml\n      - .github/workflows/reusable-test-go.yaml\n      - .github/workflows/reusable-go-quality-gates.yaml\n      - '!**/CLAUDE.md'\n  pull_request:\n    paths:\n      - alt-backend/**\n      - .github/workflows/backend-go.yaml\n      - .github/workflows/reusable-test-go.yaml\n      - .github/workflows/reusable-go-quality-gates.yaml\n      - '!**/CLAUDE.md'\npermissions:\n  contents: read\njobs:\n  unit:\n    uses: ./.github/workflows/reusable-test-go.yaml\n  gates:\n    uses: ./.github/workflows/reusable-go-quality-gates.yaml\n"
	doc, err = parseYAML(validExplicit)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDocIn(wfPath, doc, root); len(errs) != 0 {
		t.Fatalf("valid explicit paths: %#v", errs)
	}

	validGlob := "name: backend-go\non:\n  push:\n    paths:\n      - '**'\n      - '!docs/**'\n  pull_request:\n    paths:\n      - '.github/workflows/*.yaml'\n      - 'alt-backend/**'\npermissions:\n  contents: read\njobs:\n  unit:\n    uses: ./.github/workflows/reusable-test-go.yaml\n"
	doc, err = parseYAML(validGlob)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDocIn(wfPath, doc, root); len(errs) != 0 {
		t.Fatalf("valid glob paths: %#v", errs)
	}

	negated := "name: backend-go\non:\n  push:\n    paths:\n      - '**'\n      - '!.github/workflows/**'\n  pull_request:\n    paths:\n      - alt-backend/**\n      - .github/workflows/backend-go.yaml\n      - .github/workflows/reusable-test-go.yaml\n      - '!.github/workflows/reusable-*.yaml'\npermissions:\n  contents: read\njobs:\n  unit:\n    uses: ./.github/workflows/reusable-test-go.yaml\n"
	doc, err = parseYAML(negated)
	if err != nil {
		t.Fatal(err)
	}
	errs = checkWorkflowDocIn(wfPath, doc, root)
	if !containsAll(errs, "backend-go.yaml", "on.push.paths", "reusable-test-go.yaml", "on.pull_request.paths") {
		t.Fatalf("negated paths: %#v", errs)
	}

	pathsIgnore := "name: backend-go\non:\n  push:\n    paths-ignore:\n      - docs/**\n  pull_request:\n    branches:\n      - main\npermissions:\n  contents: read\njobs:\n  unit:\n    uses: ./.github/workflows/reusable-test-go.yaml\n"
	doc, err = parseYAML(pathsIgnore)
	if err != nil {
		t.Fatal(err)
	}
	if errs := checkWorkflowDocIn(wfPath, doc, root); len(errs) != 0 {
		t.Fatalf("paths-ignore: %#v", errs)
	}

	dot := "name: backend-go\non:\n  push:\n    paths:\n      - alt-backend/**\n      - github/workflows/backend-go.yaml\n      - github/workflows/reusable-test-go.yaml\npermissions:\n  contents: read\njobs:\n  unit:\n    uses: ./.github/workflows/reusable-test-go.yaml\n"
	doc, err = parseYAML(dot)
	if err != nil {
		t.Fatal(err)
	}
	errs = checkWorkflowDocIn(wfPath, doc, root)
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
		errs = checkWorkflowDocIn(wfPath, doc, root)
		if !containsAll(errs, badPat) || !containsAll(errs, "unsupported") {
			t.Fatalf("glob %s: %#v", badPat, errs)
		}
	}
}

func TestRepoMatchesCleanContract(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	files, err := workflowFilesIn(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected workflow files")
	}
	for _, relPath := range files {
		raw, err := os.ReadFile(joinRepo(root, relPath))
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		if errs := checkWorkflowText(relPath, text); len(errs) != 0 {
			t.Fatalf("%s text errors:\n%s", relPath, strings.Join(errs, "\n"))
		}
		doc, err := parseYAML(text)
		if err != nil {
			t.Fatalf("%s parse: %v", relPath, err)
		}
		if errs := checkWorkflowDocIn(relPath, doc, root); len(errs) != 0 {
			t.Fatalf("%s doc errors:\n%s", relPath, strings.Join(errs, "\n"))
		}
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"--repo-root", root}, &stdout, &stderr)
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
	t.Parallel()
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"--repo-root", dir}, &stdout, &stderr)
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
