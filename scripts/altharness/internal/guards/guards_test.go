package guards

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

func TestExecWrappersBlockAndBenignAllows(t *testing.T) {
	blocks := []struct {
		name string
		cmd  string
		work bool
	}{
		{"env -S", `env -S "golangci-lint run --fix"`, false},
		{"env --split-string", `env --split-string "cargo fix"`, false},
		{"quoted binary", `"golangci-lint" run --fix`, false},
		{"ansi-c single", `$'golangci-lint' run --fix`, false},
		{"ansi-c hex escape", `$'\x67olangci-lint' run --fix`, false},
		{"ansi-c octal escape", `$'\147olangci-lint' run --fix`, false},
		{"dollar double", `$"golangci-lint" run --fix`, false},
		{"quoted run.sh", `"./scripts/lint/run.sh" all`, true},
		{"quoted nohup", `"nohup" pytest`, true},
		{"uvx", "uvx ruff check --unsafe-fixes .", false},
		{"uv tool run", "uv tool run ruff check --unsafe-fixes .", false},
		{"bunx @version", "bunx @biomejs/biome@1.9.4 check --write --unsafe .", false},
		{"npx @version", "npx @biomejs/biome@latest check --write --unsafe .", false},
		{"bun x", "bun x --bun @biomejs/biome check --write --unsafe .", false},
		{"pnpm dlx", "pnpm dlx @biomejs/biome check --write --unsafe .", false},
		{"pnpm exec", "pnpm exec biome check --unsafe .", false},
		{"eval", `eval "golangci-lint run --fix"`, false},
		{"bash --rcfile -c", `bash --rcfile rcfile -c "golangci-lint run --fix"`, false},
		{"bash --init-file -lc", `bash --init-file rcfile -lc "cargo fix"`, false},
		{"background &", "sleep 5 &", true},
	}
	for _, tc := range blocks {
		t.Run(tc.name, func(t *testing.T) {
			code, stderr := runExec(t, tc.cmd, tc.work)
			if code != 2 || !strings.Contains(stderr, "BLOCKED [EXEC-01]") {
				t.Fatalf("code=%d stderr=%s", code, stderr)
			}
		})
	}
	allows := []string{
		`env -S "golangci-lint run ./..."`,
		"uvx ruff check --fix .",
		"pnpm dlx @biomejs/biome check --write .",
		"echo hello",
		`echo "a & b"`,
		"make lint TARGET=scripts",
		"go test ./...",
	}
	for _, cmd := range allows {
		code, stderr := runExec(t, cmd, false)
		if code != 0 {
			t.Fatalf("allow %q code=%d stderr=%s", cmd, code, stderr)
		}
	}
}

func TestPushClassCommandsBlockedAndBenignAllowed(t *testing.T) {
	blocked := []string{
		"git push origin main",
		`"git" push`,
		`$'git' push`,
		`$'\x67it' push`,
		"git -C/tmp/repo push",
		"git -C /tmp/repo push",
		"git --git-dir=/tmp/repo/.git push",
		"git --work-tree=/tmp/repo push origin HEAD",
		"git remote add upstream https://example.com/repo.git",
		"git remote set-url origin https://example.com/repo.git",
		"git remote update",
		"git fetch origin push",
		"gh pr create --title test",
		"gh pr merge 123",
		"gh release create v1.0.0",
		"gh release upload v1.0.0 dist.tar.gz",
		"gh repo sync",
		"gh workflow run ci.yml",
		"gh run rerun 999",
		`bash -c "git push origin main"`,
		`echo "$(git push)"`,
	}
	for _, cmd := range blocked {
		hit, err := CheckPushCommand(cmd, 0)
		if err != nil || hit == "" {
			t.Fatalf("CheckPushCommand(%q) = (%q, %v), want blocked", cmd, hit, err)
		}
		code, stderr := runExec(t, cmd, false)
		if code != 2 || !strings.Contains(stderr, "push-class") {
			t.Fatalf("RunExec(%q) code=%d stderr=%q", cmd, code, stderr)
		}
	}
	allowed := []string{
		"git status",
		"git fetch origin main",
		`git commit -m "never run git push"`,
		`echo "git push origin main"`,
		`rg "gh pr create" docs/`,
		"gh pr list",
		"gh pr view 123",
		"git remote -v",
	}
	for _, cmd := range allowed {
		hit, err := CheckPushCommand(cmd, 0)
		if err != nil || hit != "" {
			t.Fatalf("CheckPushCommand(%q) = (%q, %v), want allowed", cmd, hit, err)
		}
	}
}

func TestOpsFlagsAndQuotedBypassHardening(t *testing.T) {
	for _, badArgs := range [][]string{
		{"--on-match", "block"},
		{"--on-match"},
		{"--bogus"},
		{"--on-match=deny", "--unknown"},
	} {
		var stdout, stderr bytes.Buffer
		code := RunOps(context.Background(), badArgs, strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"docker ps"}}`), &stdout, &stderr)
		if code != 2 || !strings.Contains(strings.ToLower(stderr.String()), "usage") {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", badArgs, code, stdout.String(), stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("args=%v unexpected stdout=%q", badArgs, stdout.String())
		}
	}

	// Nil stdin must not panic and must emit fail-closed unreadable decision.
	var stdout, stderr bytes.Buffer
	code := RunOps(context.Background(), []string{"--on-match", "deny"}, nil, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), `"deny"`) {
		t.Fatalf("nil stdin code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	cases := []struct {
		cmd  string
		rule string
	}{
		{"docker compose -p alt up -d", "O3"},
		{"docker compose -p '' up -d", "O3"},
		{"docker compose -- up", "O3"},
		{"docker compose -p alt restart api", "O3"},
		{"docker compose -p alt up --build", "O4"},
		{"docker compose -p other up -d", ""},
		{`"docker" rm x`, "O2"},
		{`'docker' rm x`, "O2"},
		{`d'o'cker rm x`, "O2"},
		{`$'docker' rm x`, "O2"},
		{`$'\x64ocker' rm x`, "O2"},
		{`sudo "docker" rm x`, "O2"},
		{`"sh" -c "docker rm x"`, "O2"},
		{`rg "docker rm" docs/`, ""},
		{`rg "docker" rm docs/`, ""},
		{`echo "docker" rm x`, ""},
		{"echo hello", ""},
	}
	for _, tc := range cases {
		got, err := ClassifyOps(tc.cmd, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.rule {
			t.Fatalf("%q got %q want %q", tc.cmd, got, tc.rule)
		}
	}
}

func TestParallelismRatchet(t *testing.T) {
	if got := ComputeRecommendedParallelism(nil, 2).Recommended; got != 2 {
		t.Fatalf("base=%d", got)
	}
	clean := make([]string, 10)
	for i := range clean {
		clean[i] = "adopted"
	}
	if got := ComputeRecommendedParallelism(clean, 2).Recommended; got != 4 {
		t.Fatalf("scale up=%d", got)
	}
	trouble := []string{"reverted", "reverted", "reverted"}
	if got := ComputeRecommendedParallelism(trouble, 2).Recommended; got != 1 {
		t.Fatalf("scale down=%d", got)
	}
	if SanitizeBranchName("feat/test-branch") != "feat-test-branch" {
		t.Fatalf("sanitize=%s", SanitizeBranchName("feat/test-branch"))
	}
}

func TestProtectedSymlinksAndRelativeCwdBlocked(t *testing.T) {
	repo := initTestGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("SECRET=1\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repo, ".env"), filepath.Join(repo, "benign.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "secrets"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "secrets", "api_key.txt"), []byte("k"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "db", "migrations"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	migFile := filepath.Join(repo, "db", "migrations", "001.sql")
	if err := os.WriteFile(migFile, []byte("CREATE TABLE t (id INT);\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	runGitInTest(t, repo, "add", "db/migrations/001.sql")
	runGitInTest(t, repo, "commit", "-m", "add migration")
	if err := os.Symlink(migFile, filepath.Join(repo, "link.sql")); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	cases := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{
			"in-repo symlink to .env",
			map[string]any{"tool_name": "Write", "cwd": repo, "tool_input": map[string]any{"file_path": "benign.txt", "content": "x"}},
			".env",
		},
		{
			"relative path inside secrets cwd",
			map[string]any{"tool_name": "Edit", "cwd": filepath.Join(repo, "secrets"), "tool_input": map[string]any{"file_path": "api_key.txt", "new_string": "x"}},
			"secrets",
		},
		{
			"relative path inside tracked migrations cwd",
			map[string]any{"tool_name": "Edit", "cwd": filepath.Join(repo, "db", "migrations"), "tool_input": map[string]any{"file_path": "001.sql", "new_string": "x"}},
			"migrations",
		},
		{
			"in-repo symlink to tracked migration",
			map[string]any{"tool_name": "Write", "cwd": repo, "tool_input": map[string]any{"file_path": "link.sql", "content": "x"}},
			"migrations",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.payload)
			var stderr bytes.Buffer
			code := RunProtected(ctx, nil, bytes.NewReader(raw), nil, &stderr)
			if code != 2 || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("code=%d stderr=%q want substr %q", code, stderr.String(), tc.want)
			}
		})
	}
}

func TestProtectedAndEventTime(t *testing.T) {
	ctx := context.Background()
	var stderr bytes.Buffer
	payload := `{"tool_name":"Write","tool_input":{"file_path":".env","content":"X=1"}}`
	if code := RunProtected(ctx, nil, strings.NewReader(payload), nil, &stderr); code != 2 || !strings.Contains(stderr.String(), "Blocked edit") {
		t.Fatalf("protected code=%d %s", code, stderr.String())
	}
	stderr.Reset()
	okPayload := `{"tool_name":"Write","tool_input":{"file_path":"README.md","content":"hi"}}`
	if code := RunProtected(ctx, nil, strings.NewReader(okPayload), nil, &stderr); code != 0 {
		t.Fatalf("benign protected code=%d %s", code, stderr.String())
	}
	stderr.Reset()
	proj := `{"tool_name":"Edit","tool_input":{"file_path":"knowledge-sovereign/app/usecase/knowledge_home_projector/proj.go","new_string":"time.Now()"}}`
	if code := RunEventTime(ctx, nil, strings.NewReader(proj), nil, &stderr); code != 2 || !strings.Contains(stderr.String(), "time.Now") {
		t.Fatalf("event code=%d %s", code, stderr.String())
	}
	stderr.Reset()
	commented := `{"tool_name":"Edit","tool_input":{"file_path":"knowledge-sovereign/app/usecase/knowledge_home_projector/proj.go","new_string":"// time.Now()\nx := 1"}}`
	if code := RunEventTime(ctx, nil, strings.NewReader(commented), nil, &stderr); code != 0 {
		t.Fatalf("comment code=%d %s", code, stderr.String())
	}
}

func TestScopeAndGeneratedWithSymlinkRepoRoot(t *testing.T) {
	tmp := t.TempDir()
	realRepo := filepath.Join(tmp, "real-repo")
	linkRepo := filepath.Join(tmp, "link-repo")
	if err := os.MkdirAll(filepath.Join(realRepo, "scripts"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRepo, "scripts", "gen.pb.go"), []byte("// Code generated by protoc. DO NOT EDIT.\npackage scripts\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRepo, "scripts", "ok.go"), []byte("package scripts\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRepo, linkRepo); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if msg := CheckTargetPath(ctx, "scripts/ok.go", linkRepo, map[string]any{"packet_id": "PKT-1", "allowed_paths": []any{"scripts/**"}}, linkRepo); msg != "" {
		t.Fatalf("expected allow under symlink repoRoot, got %q", msg)
	}
	if msg := CheckTargetPath(ctx, "scripts/gen.pb.go", linkRepo, nil, linkRepo); !strings.Contains(msg, "DO NOT EDIT") {
		t.Fatalf("expected DO NOT EDIT block under symlink repoRoot, got %q", msg)
	}
}

func TestHarnessRunStatusSchemaAndPureGoPacketValidation(t *testing.T) {
	repo := initTestGitRepo(t)
	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	// Invalid --format xml must exit 2.
	code := RunHarness(ctx, []string{"status", "--repo-root", repo, "--format", "xml"}, nil, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("xml format code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	// Unknown flag must exit 2.
	stdout.Reset()
	stderr.Reset()
	code = RunHarness(ctx, []string{"status", "--repo-root", repo, "--bogus"}, nil, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("bogus flag code=%d stderr=%q", code, stderr.String())
	}

	// Init + activate-packet (pure Go!) + status JSON & 8-line text.
	stdout.Reset()
	stderr.Reset()
	code = RunHarness(ctx, []string{"init", "--repo-root", repo, "--policy", "block-new", "--goal", "Test", "--completion", "ok"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init code=%d stderr=%q", code, stderr.String())
	}

	pktPath := filepath.Join(repo, "pkt.md")
	if err := os.WriteFile(pktPath, []byte(sampleValidPacket("PKT-GO-1")), fsperm.File); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = RunHarness(ctx, []string{"activate-packet", "--repo-root", repo, pktPath}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("activate-packet code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	// Non-string element in forbidden_paths must fail closed.
	badForbid := filepath.Join(repo, "pkt-bad.md")
	badText := strings.Replace(sampleValidPacket("PKT-BAD-1"), `    - "scripts/forbidden.py"`, "    - 12345", 1)
	if err := os.WriteFile(badForbid, []byte(badText), fsperm.File); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = RunHarness(ctx, []string{"activate-packet", "--repo-root", repo, badForbid}, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-string forbidden_paths to fail closed, stdout=%q", stdout.String())
	}

	// status --format json checks all required keys.
	stdout.Reset()
	stderr.Reset()
	code = RunHarness(ctx, []string{"status", "--repo-root", repo, "--format", "json"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status json code=%d stderr=%q", code, stderr.String())
	}
	var parsed map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid status json: %v\n%s", err, stdout.String())
	}
	if _, ok := parsed["runs"].([]any); !ok {
		t.Fatalf("missing top-level runs array: %s", stdout.String())
	}
	cur, ok := parsed["current_run"].(map[string]any)
	if !ok {
		t.Fatalf("missing current_run: %s", stdout.String())
	}
	for _, key := range []string{"branch", "sanitized_branch", "path", "decisions_count", "active_packet", "packets", "handoffs", "verify", "evidence", "subagents_count", "parallelism"} {
		if _, exists := cur[key]; !exists {
			t.Fatalf("current_run missing key %q in %s", key, stdout.String())
		}
	}

	// status --format text must emit all 8 lines.
	stdout.Reset()
	stderr.Reset()
	code = RunHarness(ctx, []string{"status", "--repo-root", repo, "--format", "text"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status text code=%d stderr=%q", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 8 {
		t.Fatalf("expected 8 text status lines, got %d:\n%s", len(lines), stdout.String())
	}
}

func TestAgyHookLifecycle(t *testing.T) {
	repo := initTestGitRepo(t)
	artifactDir := t.TempDir()
	ctx := context.Background()

	// Save & restore seams.
	origSC, origPF, origLayers, origDocdag, origFmt := agyRunSelfcheckFn, agyRunPreflightFn, agyRunLayersFn, agyRunDocdagValidateFn, agyFormatFileFn
	t.Cleanup(func() {
		agyRunSelfcheckFn = origSC
		agyRunPreflightFn = origPF
		agyRunLayersFn = origLayers
		agyRunDocdagValidateFn = origDocdag
		agyFormatFileFn = origFmt
	})
	agyRunDocdagValidateFn = func(_ context.Context, _ string) (int, string) { return 0, "" }

	// 1. session-start combines adr_preflight + selfcheck into injectSteps.
	agyRunPreflightFn = func(_ context.Context, _ string, payload map[string]any) (string, string, int) {
		if payload["session_id"] != "conv-123" || payload["hook_event_name"] != "SessionStart" {
			t.Errorf("unexpected preflight session payload: %+v", payload)
		}
		return "adr_preflight_enabled: OK", "", 0
	}
	agyRunSelfcheckFn = func(_ context.Context, _ string) string {
		return "agent_harness_selfcheck: OK"
	}
	var stdout, stderr bytes.Buffer
	rc := RunAgyHook(ctx, []string{"session-start"}, strings.NewReader(`{"conversationId":"conv-123","hookEventName":"SessionStart","cwd":"`+repo+`"}`), &stdout, &stderr)
	if rc != 0 {
		t.Fatalf("session-start rc=%d", rc)
	}
	var ss map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &ss); err != nil {
		t.Fatalf("session-start invalid json: %v (%s)", err, stdout.String())
	}
	steps := ss["injectSteps"].([]any)
	msg := steps[0].(map[string]any)["systemMessage"].(map[string]any)["systemMessage"].(string)
	if !strings.Contains(msg, "adr_preflight_enabled") || !strings.Contains(msg, "agent_harness_selfcheck: OK") {
		t.Fatalf("unexpected session-start message: %q", msg)
	}

	// 2. pre-shell: push block, ops ask/deny, EXEC-01 + IsDaemon, malformed fail-closed, benign allow.
	t.Setenv("ALT_HARNESS_REPO_ROOT", repo)
	t.Setenv("ALT_HARNESS_ROLE", "worker")
	shellCases := []struct {
		name    string
		args    []string
		stdin   string
		wantDec string
		wantSub string
	}{
		{
			"push blocked",
			[]string{"pre-shell"},
			`{"toolCall":{"id":"1","name":"run_command","args":{"CommandLine":"\"git\" -C/tmp push origin main"}},"cwd":"` + repo + `"}`,
			"deny",
			"push-class",
		},
		{
			"ops ask default",
			[]string{"pre-shell"},
			`{"toolCall":{"id":"2","name":"run_command","args":{"CommandLine":"docker rm -f api"}},"cwd":"` + repo + `"}`,
			"ask",
			"Rule O2",
		},
		{
			"ops deny override",
			[]string{"pre-shell", "--on-match=deny"},
			`{"toolCall":{"id":"3","name":"run_command","args":{"CommandLine":"$'docker' rm -f api"}},"cwd":"` + repo + `"}`,
			"deny",
			"Rule O2",
		},
		{
			"exec E1 blocked",
			[]string{"pre-shell"},
			`{"toolCall":{"id":"4","name":"run_command","args":{"CommandLine":"$'golangci-lint' run --fix"}},"cwd":"` + repo + `"}`,
			"deny",
			"BLOCKED [EXEC-01]",
		},
		{
			"exec E3 IsDaemon blocked for worker",
			[]string{"pre-shell"},
			`{"toolCall":{"id":"5","name":"run_command","args":{"CommandLine":"go test ./...","IsDaemon":true}},"cwd":"` + repo + `"}`,
			"deny",
			"BLOCKED [EXEC-01]",
		},
		{
			"malformed JSON fail closed",
			[]string{"pre-shell"},
			`{bad json`,
			"deny",
			"BLOCKED",
		},
		{
			"benign allow",
			[]string{"pre-shell"},
			`{"toolCall":{"id":"6","name":"run_command","args":{"CommandLine":"go test ./...","IsDaemon":false}},"cwd":"` + repo + `"}`,
			"allow",
			"",
		},
	}
	for _, tc := range shellCases {
		t.Run("pre-shell/"+tc.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()
			rc := RunAgyHook(ctx, tc.args, strings.NewReader(tc.stdin), &stdout, &stderr)
			if rc != 0 {
				t.Fatalf("rc=%d want 0", rc)
			}
			var out map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
				t.Fatalf("invalid json: %v (%s)", err, stdout.String())
			}
			if out["decision"] != tc.wantDec {
				t.Fatalf("decision=%v want %q (full=%s)", out["decision"], tc.wantDec, stdout.String())
			}
			if tc.wantSub != "" {
				reason, _ := out["reason"].(string)
				if !strings.Contains(reason, tc.wantSub) {
					t.Fatalf("reason=%q missing %q", reason, tc.wantSub)
				}
			}
		})
	}

	// 3. pre-edit: active packet + artifactDirectoryPath exemption + protected + event-time + preflight.
	pktPath := filepath.Join(repo, "pkt.md")
	if err := os.WriteFile(pktPath, []byte(sampleValidPacket("PKT-AGY-1")), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if code := RunHarness(ctx, []string{"activate-packet", "--repo-root", repo, pktPath}, nil, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("activate-packet failed: %d", code)
	}

	// Preflight allows by default here.
	agyRunPreflightFn = func(_ context.Context, _ string, _ map[string]any) (string, string, int) {
		return "", "", 0
	}

	// Brain artifact write inside artifactDirectoryPath is EXEMPT from active-packet out-of-repo rejection!
	artPlan := filepath.Join(artifactDir, "scratch", "plan.json")
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"pre-edit"}, strings.NewReader(`{"conversationId":"c1","cwd":"`+repo+`","artifactDirectoryPath":"`+artifactDir+`","toolCall":{"id":"e1","name":"write_file","args":{"Path":"`+artPlan+`","Content":"{}"}}}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("artifact exemption failed: rc=%d out=%s", rc, stdout.String())
	}

	// Secret .env inside artifactDirectoryPath is STILL BLOCKED by CheckProtectedPath!
	artEnv := filepath.Join(artifactDir, ".env")
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"pre-edit"}, strings.NewReader(`{"conversationId":"c1","cwd":"`+repo+`","artifactDirectoryPath":"`+artifactDir+`","toolCall":{"id":"e2","name":"write_file","args":{"Path":"`+artEnv+`","Content":"X=1"}}}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"deny"`) {
		t.Fatalf("protected path in artifact dir should deny: rc=%d out=%s", rc, stdout.String())
	}

	// multi_replace_file_content catches event-time violation in projector!
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"pre-edit"}, strings.NewReader(`{"conversationId":"c1","cwd":"`+repo+`","toolCall":{"id":"e3","name":"multi_replace_file_content","args":{"TargetFile":"knowledge-sovereign/app/usecase/knowledge_home_projector/p.go","ReplacementChunks":[{"ReplacementContent":"t := time.Now()"}]}}}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"deny"`) || !strings.Contains(stdout.String(), "time.Now") {
		t.Fatalf("event-time in multi_replace_file_content should deny: rc=%d out=%s", rc, stdout.String())
	}

	// adr_preflight exit 2 -> decision: deny; ANY non-zero exit (e.g. exit 1) -> fail-closed deny.
	agyRunPreflightFn = func(_ context.Context, _ string, _ map[string]any) (string, string, int) {
		return "", "ADR PREFLIGHT BLOCKED (ADR required before code edits) — scope(s): scripts", 2
	}
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"pre-edit"}, strings.NewReader(`{"conversationId":"c1","cwd":"`+repo+`","toolCall":{"id":"e4","name":"edit_file","args":{"Path":"scripts/allowed.go","Replacement":"x := 1"}}}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"deny"`) || !strings.Contains(stdout.String(), "ADR PREFLIGHT BLOCKED") {
		t.Fatalf("adr_preflight exit 2 should deny: rc=%d out=%s", rc, stdout.String())
	}
	agyRunPreflightFn = func(_ context.Context, _ string, _ map[string]any) (string, string, int) {
		return "", "preflight unexpected failure", 1
	}
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"pre-edit"}, strings.NewReader(`{"conversationId":"c1","cwd":"`+repo+`","toolCall":{"id":"e4b","name":"edit_file","args":{"Path":"scripts/allowed.go","Replacement":"x := 1"}}}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"deny"`) || !strings.Contains(stdout.String(), "preflight unexpected failure") {
		t.Fatalf("adr_preflight exit 1 must fail-closed deny: rc=%d out=%s", rc, stdout.String())
	}

	// 4. post-edit calls formatter on in-repo file and emits {}.
	goFile := filepath.Join(repo, "scripts", "allowed.go")
	if err := os.MkdirAll(filepath.Dir(goFile), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goFile, []byte("package scripts\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	formatted := ""
	agyFormatFileFn = func(_ context.Context, _ string, path string) { formatted = path }
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"post-edit"}, strings.NewReader(`{"cwd":"`+repo+`","toolCall":{"id":"p1","name":"write_file","args":{"Path":"scripts/allowed.go","Content":"package scripts\n"}}}`), &stdout, &stderr)
	if rc != 0 || strings.TrimSpace(stdout.String()) != "{}" || filepath.Base(formatted) != "allowed.go" {
		t.Fatalf("post-edit rc=%d out=%q formatted=%q", rc, stdout.String(), formatted)
	}

	// 5. stop: violation on !continuationActive && NO_TOOL_CALL -> continue; continuationActive=true -> allow.
	agyRunLayersFn = func(_ context.Context, _ string) (int, string) {
		return 1, "VIOLATION: handler imports driver\nFIX: route via usecase"
	}
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"stop"}, strings.NewReader(`{"conversationId":"c1","cwd":"`+repo+`","continuationActive":false,"terminationReason":"NO_TOOL_CALL"}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"continue"`) || !strings.Contains(stdout.String(), "VIOLATION") {
		t.Fatalf("stop violation should continue: rc=%d out=%s", rc, stdout.String())
	}
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"stop"}, strings.NewReader(`{"conversationId":"c1","cwd":"`+repo+`","continuationActive":true,"terminationReason":"NO_TOOL_CALL"}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("stop with continuationActive=true should allow: rc=%d out=%s", rc, stdout.String())
	}

	// stop: when layers pass, dirty ADR docdag validate failure returns continue.
	agyRunLayersFn = func(_ context.Context, _ string) (int, string) { return 0, "" }
	agyRunDocdagValidateFn = func(_ context.Context, _ string) (int, string) {
		return 1, "ERROR docs/ADR/001032.md: missing Consequences section"
	}
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"stop"}, strings.NewReader(`{"conversationId":"c1","cwd":"`+repo+`","continuationActive":false,"terminationReason":"NO_TOOL_CALL"}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"continue"`) || !strings.Contains(stdout.String(), "001032.md") {
		t.Fatalf("stop docdag failure should continue: rc=%d out=%s", rc, stdout.String())
	}
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"stop"}, strings.NewReader(`{"conversationId":"c1","cwd":"`+repo+`","continuationActive":true,"terminationReason":"NO_TOOL_CALL"}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("stop docdag failure with continuationActive=true should allow: rc=%d out=%s", rc, stdout.String())
	}
	agyRunDocdagValidateFn = func(_ context.Context, _ string) (int, string) { return 0, "" }
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"stop"}, strings.NewReader(`{"conversationId":"c1","cwd":"`+repo+`","continuationActive":false,"terminationReason":"NO_TOOL_CALL"}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("stop clean should allow: rc=%d out=%s", rc, stdout.String())
	}
}

func TestAgyHookPreEditFailClosedAndDefaultStopDocdagValidate(t *testing.T) {
	repo := initTestGitRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs", "ADR"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docdag.yaml"), []byte("preset: adr\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	runGitInTest(t, repo, "add", "docdag.yaml")
	runGitInTest(t, repo, "commit", "-m", "init docdag")

	t.Setenv("ALT_HARNESS_REPO_ROOT", repo)
	t.Setenv("ALT_ACTIVE_PACKET", "")

	origPF, origLayers, origDocdag := agyRunPreflightFn, agyRunLayersFn, agyRunDocdagValidateFn
	t.Cleanup(func() {
		agyRunPreflightFn = origPF
		agyRunLayersFn = origLayers
		agyRunDocdagValidateFn = origDocdag
	})

	// 1. pre-edit denies with fallback message when preflight exits 1 with empty stderr.
	agyRunPreflightFn = func(_ context.Context, _ string, _ map[string]any) (string, string, int) {
		return "", "", 1
	}
	var stdout, stderr bytes.Buffer
	rc := RunAgyHook(context.Background(), []string{"pre-edit"}, strings.NewReader(`{"conversationId":"c-fc","cwd":"`+repo+`","toolCall":{"id":"e1","name":"write_file","args":{"Path":"scripts/foo.go","Content":"package scripts\n"}}}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"deny"`) || !strings.Contains(stdout.String(), "ADR PREFLIGHT BLOCKED") {
		t.Fatalf("pre-edit exit 1 empty stderr must fail-closed deny: rc=%d out=%s", rc, stdout.String())
	}

	// 2. defaultAgyRunDocdagValidate skips docdag when docs/ADR and docdag.yaml are clean,
	//    and catches broken dirty ADR / docdag.yaml when modified.
	binDir := t.TempDir()
	docdagLog := filepath.Join(binDir, "docdag.log")
	docdagExitFile := filepath.Join(binDir, "docdag.exit")
	if err := os.WriteFile(docdagExitFile, []byte("1"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	stubScript := "#!/bin/sh\necho \"$@\" >> \"" + docdagLog + "\"\ncode=$(cat \"" + docdagExitFile + "\")\nif [ \"$code\" != \"0\" ]; then\n  echo \"ERROR docs/ADR/000099.md: broken frontmatter\"\nfi\nexit \"$code\"\n"
	if err := fsperm.WriteExecutable(filepath.Join(binDir, "docdag"), []byte(stubScript)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	agyRunLayersFn = func(_ context.Context, _ string) (int, string) { return 0, "" }
	agyRunDocdagValidateFn = defaultAgyRunDocdagValidate

	stopPayload := `{"conversationId":"c-stop","cwd":"` + repo + `","continuationActive":false,"terminationReason":"NO_TOOL_CALL"}`

	// Clean repo: must allow without invoking docdag.
	stdout.Reset()
	rc = RunAgyHook(context.Background(), []string{"stop"}, strings.NewReader(stopPayload), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("clean stop should allow: rc=%d out=%s", rc, stdout.String())
	}
	if _, err := os.Stat(docdagLog); !os.IsNotExist(err) {
		t.Fatalf("docdag should not run when docs/ADR and docdag.yaml are clean")
	}

	// Dirty broken ADR: stop must return continue with validation error.
	if err := os.WriteFile(filepath.Join(repo, "docs", "ADR", "000099.md"), []byte("broken\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	rc = RunAgyHook(context.Background(), []string{"stop"}, strings.NewReader(stopPayload), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"continue"`) || !strings.Contains(stdout.String(), "broken frontmatter") {
		t.Fatalf("dirty broken ADR must return continue: rc=%d out=%s", rc, stdout.String())
	}

	// Fixed dirty ADR (docdag exits 0): stop allows.
	if err := os.WriteFile(docdagExitFile, []byte("0"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	rc = RunAgyHook(context.Background(), []string{"stop"}, strings.NewReader(stopPayload), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("valid dirty ADR should allow: rc=%d out=%s", rc, stdout.String())
	}

	// Dirty docdag.yaml with docdag validate exit 3: stop must return continue.
	_ = os.Remove(filepath.Join(repo, "docs", "ADR", "000099.md"))
	if err := os.WriteFile(filepath.Join(repo, "docdag.yaml"), []byte("preset: bad\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docdagExitFile, []byte("3"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	rc = RunAgyHook(context.Background(), []string{"stop"}, strings.NewReader(stopPayload), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"continue"`) || !strings.Contains(stdout.String(), "exit 3") {
		t.Fatalf("dirty broken docdag.yaml must return continue: rc=%d out=%s", rc, stdout.String())
	}
}

func TestAgyHookPreAdrRead(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	t.Setenv("ALT_ADR_PREFLIGHT_STATE_DIR", stateDir)
	t.Setenv("ALT_ADR_PREFLIGHT", "")

	// 1. 1st ADR shell read in conversation denies with specific docdag guidance and writes receipt.
	req := `{"conversationId":"agy-adr-1","cwd":"/repo","toolCall":{"id":"r1","name":"run_command","args":{"CommandLine":"sed -n '1,40p' docs/ADR/000979.md"}}}`
	var stdout, stderr bytes.Buffer
	rc := RunAgyHook(ctx, []string{"pre-adr-read"}, strings.NewReader(req), &stdout, &stderr)
	if rc != 0 {
		t.Fatalf("rc=%d want 0", rc)
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v (%s)", err, stdout.String())
	}
	if out["decision"] != "deny" {
		t.Fatalf("1st hold decision=%v want deny (out=%s)", out["decision"], stdout.String())
	}
	reason, _ := out["reason"].(string)
	if !strings.Contains(reason, "docdag context 000979") || !strings.Contains(reason, "--section Consequences") {
		t.Fatalf("reason missing docdag guidance: %q", reason)
	}

	// 2. 2nd call in same conversation allows.
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"pre-adr-read"}, strings.NewReader(req), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("2nd call should allow: rc=%d out=%s", rc, stdout.String())
	}

	// 3. Non-ADR command allows cleanly (without consuming receipt of a new conversation).
	stdout.Reset()
	benign := `{"conversationId":"agy-adr-2","cwd":"/repo","toolCall":{"id":"r2","name":"run_command","args":{"CommandLine":"git diff docs/ADR/000979.md"}}}`
	rc = RunAgyHook(ctx, []string{"pre-adr-read"}, strings.NewReader(benign), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("git diff should allow: rc=%d out=%s", rc, stdout.String())
	}

	// 4. Non-shell tool allows.
	stdout.Reset()
	nonShell := `{"conversationId":"agy-adr-2","cwd":"/repo","toolCall":{"id":"r3","name":"read_file","args":{"Path":"docs/ADR/000979.md"}}}`
	rc = RunAgyHook(ctx, []string{"pre-adr-read"}, strings.NewReader(nonShell), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("non-shell tool should allow: rc=%d out=%s", rc, stdout.String())
	}

	// 5. Malformed JSON and missing command fail-closed with decision: deny and exit 0.
	for _, bad := range []string{
		`{bad json`,
		`{"conversationId":"c","toolCall":{"id":"1","name":"run_command","args":{}}}`,
	} {
		stdout.Reset()
		rc = RunAgyHook(ctx, []string{"pre-adr-read"}, strings.NewReader(bad), &stdout, &stderr)
		if rc != 0 || !strings.Contains(stdout.String(), `"deny"`) || !strings.Contains(stdout.String(), "BLOCKED:") {
			t.Fatalf("bad input %q must fail-closed deny: rc=%d out=%s", bad, rc, stdout.String())
		}
	}

	// 6. Bash/shell argv command form also works in agy-hook pre-adr-read.
	stdout.Reset()
	bashArgvReq := `{"session_id":"agy-bash-argv","cwd":"/repo","tool_name":"Bash","tool_input":{"command":["cat","docs/ADR/000979.md"]}}`
	rc = RunAgyHook(ctx, []string{"pre-adr-read"}, strings.NewReader(bashArgvReq), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"deny"`) || !strings.Contains(stdout.String(), "docdag context 000979") {
		t.Fatalf("Bash argv 1st hold should deny: rc=%d out=%s", rc, stdout.String())
	}
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"pre-adr-read"}, strings.NewReader(bashArgvReq), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("Bash argv 2nd call should allow: rc=%d out=%s", rc, stdout.String())
	}

	// 7. ALT_ADR_PREFLIGHT=disabled allows immediately.
	t.Setenv("ALT_ADR_PREFLIGHT", "disabled")
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"pre-adr-read"}, strings.NewReader(`{"conversationId":"agy-disabled","cwd":"/repo","toolCall":{"id":"r5","name":"run_command","args":{"CommandLine":"cat docs/ADR/000979.md"}}}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("disabled kill switch should allow: rc=%d out=%s", rc, stdout.String())
	}
	t.Setenv("ALT_ADR_PREFLIGHT", "")

	// 8. Unwritable state dir fails closed with decision: deny.
	blocker := filepath.Join(t.TempDir(), "file-blocker")
	if err := os.WriteFile(blocker, []byte("x"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALT_ADR_PREFLIGHT_STATE_DIR", blocker)
	stdout.Reset()
	ioErrReq := `{"conversationId":"agy-ioerr","cwd":"/repo","toolCall":{"id":"r4","name":"run_command","args":{"CommandLine":"cat docs/ADR/000979.md"}}}`
	rc = RunAgyHook(ctx, []string{"pre-adr-read"}, strings.NewReader(ioErrReq), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"deny"`) || !strings.Contains(stdout.String(), "BLOCKED:") {
		t.Fatalf("unwritable state dir must fail-closed deny: rc=%d out=%s", rc, stdout.String())
	}
}

func TestAgyHookPreEditBatchesMultiScopePreflight(t *testing.T) {
	multiRepo := initTestGitRepo(t)
	artifactDir := t.TempDir()
	artPlan := filepath.Join(artifactDir, "scratch", "plan.json")
	t.Setenv("ALT_HARNESS_REPO_ROOT", multiRepo)
	t.Setenv("ALT_ACTIVE_PACKET", "")

	origPreflight := agyRunPreflightFn
	t.Cleanup(func() { agyRunPreflightFn = origPreflight })

	seenScopes := map[string]bool{}
	preflightCalls := 0
	agyRunPreflightFn = func(_ context.Context, _ string, pfPayload map[string]any) (string, string, int) {
		preflightCalls++
		ti, _ := pfPayload["tool_input"].(map[string]any)
		cmd, _ := ti["command"].(string)
		if pfPayload["tool_name"] != "apply_patch" {
			t.Errorf("multi-file pre-edit should batch via apply_patch, got tool_name=%v payload=%#v", pfPayload["tool_name"], pfPayload)
		}
		if strings.Contains(cmd, artPlan) {
			t.Errorf("artifact path should be excluded from preflight batch: %q", cmd)
		}
		var unack []string
		for _, line := range strings.Split(cmd, "\n") {
			if p, ok := strings.CutPrefix(line, "*** Update File: "); ok {
				scope := strings.SplitN(strings.TrimSpace(p), "/", 2)[0]
				if !seenScopes[scope] {
					seenScopes[scope] = true
					unack = append(unack, scope)
				}
			}
		}
		if len(unack) > 0 {
			return "", "ADR PREFLIGHT (not an error) — scope(s): " + strings.Join(unack, ", "), 2
		}
		return "", "", 0
	}

	multiPatchReq := `{"conversationId":"c-multi","cwd":"` + multiRepo + `","artifactDirectoryPath":"` + artifactDir + `","toolCall":{"id":"e5","name":"apply_patch","args":{"command":"*** Begin Patch\n*** Update File: alt-backend/app/main.go\n@@\n+x := 1\n*** Update File: tag-generator/app/main.py\n@@\n+y = 2\n*** Update File: ` + artPlan + `\n@@\n+{}\n*** End Patch"}}}`
	var stdout, stderr bytes.Buffer
	rc := RunAgyHook(context.Background(), []string{"pre-edit"}, strings.NewReader(multiPatchReq), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"deny"`) || !strings.Contains(stdout.String(), "alt-backend, tag-generator") {
		t.Fatalf("call 1 should deny with both scopes batched: rc=%d out=%s", rc, stdout.String())
	}
	if preflightCalls != 1 {
		t.Fatalf("call 1 invoked preflight %d times, want 1", preflightCalls)
	}

	stdout.Reset()
	stderr.Reset()
	rc = RunAgyHook(context.Background(), []string{"pre-edit"}, strings.NewReader(multiPatchReq), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("call 2 retry must succeed immediately: rc=%d out=%s", rc, stdout.String())
	}
	if preflightCalls != 2 {
		t.Fatalf("total preflight calls=%d, want 2", preflightCalls)
	}
}

func TestScopeBlocksHistoricalAndResolvesCwd(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "stopped-using-k8s"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "stopped-using-k8s", "old.yaml"), []byte("a"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(repo, "scripts")
	if err := os.MkdirAll(sub, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALT_HARNESS_REPO_ROOT", repo)
	t.Setenv("ALT_ACTIVE_PACKET", "")
	ctx := context.Background()
	var stderr bytes.Buffer
	body := `{"tool_name":"Edit","tool_input":{"file_path":"stopped-using-k8s/old.yaml","new_string":"x"},"cwd":"` + repo + `"}`
	if code := RunPacketScope(ctx, nil, strings.NewReader(body), nil, &stderr); code != 2 || !strings.Contains(stderr.String(), "stopped-using-k8s") {
		t.Fatalf("k8s code=%d %s", code, stderr.String())
	}
	msg := CheckTargetPath(ctx, "nested.go", repo, map[string]any{
		"packet_id":     "PKT-1",
		"allowed_paths": []any{"scripts/**"},
	}, sub)
	if msg != "" {
		t.Fatalf("cwd relative should be allowed, got %s", msg)
	}
}

func runExec(t *testing.T, cmd string, worker bool) (int, string) {
	t.Helper()
	payload := map[string]any{
		"tool_name": "Bash",
		"tool_input": map[string]any{
			"command": cmd,
		},
	}
	if worker {
		payload["agent_id"] = "w-1"
	}
	raw, _ := json.Marshal(payload)
	var stdout, stderr bytes.Buffer
	code := RunExec(context.Background(), nil, bytes.NewReader(raw), &stdout, &stderr)
	return code, stderr.String()
}

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGitInTest(t, repo, "init", "-b", "feat/test-branch")
	runGitInTest(t, repo, "config", "user.email", "test@example.com")
	runGitInTest(t, repo, "config", "user.name", "Test")
	return repo
}

func runGitInTest(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func sampleValidPacket(id string) string {
	verifyCmd := "./scripts/lint/run.sh scripts"
	sum := sha256.Sum256([]byte(verifyCmd))
	sha := hex.EncodeToString(sum[:])
	return "---\n" +
		`packet_version: "1.0"` + "\n" +
		`packet_id: "` + id + `"` + "\n" +
		`title: "Test packet"` + "\n" +
		`base_commit: "1111111111111111111111111111111111111111"` + "\n" +
		"scope:\n" +
		`  target: "scripts"` + "\n" +
		"  allowed_paths:\n" +
		`    - "scripts/**"` + "\n" +
		"  forbidden_paths:\n" +
		`    - "scripts/forbidden.py"` + "\n" +
		"  allow_generated_code_edits: false\n" +
		"  max_files: 5\n" +
		"  max_functions: 3\n" +
		"  allowed_rules:\n" +
		`    - "S101"` + "\n" +
		`  stop_condition: "done"` + "\n" +
		"execution_policy:\n" +
		`  policy: "fix-existing"` + "\n" +
		"  foreground_only: true\n" +
		"  forbid_repo_wide_test_build: true\n" +
		"  forbid_bulk_autofix: true\n" +
		`  verification_command: "` + verifyCmd + `"` + "\n" +
		"  measured:\n" +
		"    duration_s: 1.0\n" +
		"    exit_code: 0\n" +
		`    at_commit: "1111111111111111111111111111111111111111"` + "\n" +
		`    command_sha256: "` + sha + `"` + "\n" +
		"handoff_requirements:\n" +
		"  require_modified_files_and_lines: true\n" +
		"  require_runtime_behavior_impact: true\n" +
		"  require_change_rationale: true\n" +
		"---\n\n" +
		"# Task Packet\n\n" +
		"## 1. 範囲と作業量制約\n\n- Bounded.\n\n" +
		"## 2. 実行・検証手順\n\n```bash\n" + verifyCmd + "\n```\n\n" +
		"## 3. Hand-off 記述必須項目\n\n- Summary.\n"
}

func TestShellCombinedStrictFlagsCScriptBlocked(t *testing.T) {
	// 1. CheckPushCommand / agy-hook pre-shell must catch bash -euo pipefail -c / sh -eo pipefail -c.
	pushCmds := []string{
		`bash -euo pipefail -c 'git push origin main'`,
		`sh -eo pipefail -c 'gh pr merge 1'`,
		`bash -cuo pipefail 'git push origin main'`,
	}
	for _, cmd := range pushCmds {
		if msg, err := CheckPushCommand(cmd, 0); err != nil || msg == "" {
			t.Errorf("CheckPushCommand(%q) = (%q, %v), want blocked", cmd, msg, err)
		}
	}

	// 2. RunExec (E1/E2) must catch combined strict-mode flags.
	var out, errBuf bytes.Buffer
	e1Payload := `{"tool_name":"Bash","tool_input":{"command":"bash -euo pipefail -c 'golangci-lint run --fix'"}}`
	if code := RunExec(context.Background(), nil, strings.NewReader(e1Payload), &out, &errBuf); code != 2 || !strings.Contains(errBuf.String(), "golangci-lint --fix") {
		t.Errorf("RunExec(E1) code=%d stderr=%q, want 2 with golangci-lint --fix", code, errBuf.String())
	}
	out.Reset()
	errBuf.Reset()
	e2Payload := `{"tool_name":"Bash","tool_input":{"command":"bash -euo pipefail ./scripts/lint/run.sh all"}}`
	t.Setenv("ALT_HARNESS_ROLE", "worker")
	if code := RunExec(context.Background(), nil, strings.NewReader(e2Payload), &out, &errBuf); code != 2 || !strings.Contains(errBuf.String(), "全体検査") {
		t.Errorf("RunExec(E2) code=%d stderr=%q, want 2 with 全体検査", code, errBuf.String())
	}

	// 3. ClassifyOps must catch bash -euo pipefail -c and bash -o pipefail -c.
	opsCmds := []string{
		`bash -euo pipefail -c 'docker rm prod-db'`,
		`bash -o pipefail -c 'docker rm x'`,
		`bash +euo pipefail -c 'docker rm x'`,
	}
	for _, cmd := range opsCmds {
		if rule, err := ClassifyOps(cmd, 0); err != nil || rule != "O2" {
			t.Errorf("ClassifyOps(%q) = (%q, %v), want O2", cmd, rule, err)
		}
	}
}

func TestProtectedBlocksDanglingSymlinkToEnv(t *testing.T) {
	dir := t.TempDir()
	dangling := filepath.Join(dir, "dangling.txt")
	if err := os.Symlink(".env.local", dangling); err != nil {
		t.Fatal(err)
	}
	if msg := CheckProtectedPathInCwd(dangling, dir); msg == "" {
		t.Fatalf("expected CheckProtectedPathInCwd to block dangling symlink to .env.local")
	}
	payload := `{"cwd":"` + dir + `","tool_input":{"file_path":"dangling.txt"}}`
	var out, errBuf bytes.Buffer
	if code := RunProtected(context.Background(), nil, strings.NewReader(payload), &out, &errBuf); code != 2 {
		t.Fatalf("RunProtected dangling symlink code=%d want 2 (stderr=%s)", code, errBuf.String())
	}
}

func TestHarnessRunValidateDecisionsTSV(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("valid init and log passes in text and json", func(t *testing.T) {
		t.Parallel()
		repo := initTestGitRepo(t)
		var out, errB bytes.Buffer
		if rc := RunHarness(ctx, []string{"init", "--repo-root", repo, "--policy", "fix-existing", "--goal", "Fix lint", "--completion", "run.sh exits 0"}, nil, &out, &errB); rc != 0 {
			t.Fatalf("init rc=%d err=%s", rc, errB.String())
		}
		out.Reset()
		errB.Reset()
		if rc := RunHarness(ctx, []string{"log", "--repo-root", repo, "--phase", "verify", "--decision", "lint green", "--why", "passed check", "--evidence", "cmd:./scripts/lint/run.sh scripts", "--result", "adopted"}, nil, &out, &errB); rc != 0 {
			t.Fatalf("log rc=%d err=%s", rc, errB.String())
		}

		out.Reset()
		errB.Reset()
		if rc := RunHarness(ctx, []string{"validate", "--repo-root", repo}, nil, &out, &errB); rc != 0 || !strings.Contains(out.String(), "OK:") {
			t.Fatalf("validate text rc=%d out=%q err=%q", rc, out.String(), errB.String())
		}

		out.Reset()
		errB.Reset()
		if rc := RunHarness(ctx, []string{"validate", "--repo-root", repo, "--format", "json"}, nil, &out, &errB); rc != 0 {
			t.Fatalf("validate json rc=%d out=%q err=%q", rc, out.String(), errB.String())
		}
		var parsed map[string]any
		if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
			t.Fatalf("invalid json: %v (%s)", err, out.String())
		}
		if parsed["valid"] != true || parsed["rows"] != float64(2) {
			t.Fatalf("unexpected validate json: %#v", parsed)
		}
	})

	t.Run("missing file and bad flags return exit 2", func(t *testing.T) {
		t.Parallel()
		repo := initTestGitRepo(t)
		cases := [][]string{
			{"validate", "--repo-root", repo},
			{"validate", "--decisions-file", filepath.Join(t.TempDir(), "missing.tsv")},
			{"validate", "--repo-root", repo, "--format", "yaml"},
			{"validate", "--repo-root", repo, "--unknown"},
		}
		for _, args := range cases {
			var out, errB bytes.Buffer
			if rc := RunHarness(ctx, args, nil, &out, &errB); rc != 2 {
				t.Fatalf("args=%v rc=%d want 2 out=%q err=%q", args, rc, out.String(), errB.String())
			}
		}
	})

	t.Run("structural and semantic violations return exit 1", func(t *testing.T) {
		t.Parallel()
		validHeader := "ts\tphase\tdecision\twhy\tevidence\tresult\n"
		validInit := "2026-11-28T00:00:00Z\tinit\tpolicy=block-new; goal=Test; completion=all green\tLOOP-03 init\t.harness/runs/b/decisions.tsv\tinfo\n"

		cases := []struct {
			name     string
			content  string
			wantCode string
		}{
			{"wrong header", "ts\tphase\tdecision\n" + validInit, "INVALID_HEADER"},
			{"header only no init row", validHeader, "MISSING_INIT_ROW"},
			{"row 1 wrong phase", validHeader + "2026-11-28T00:00:00Z\treview\tpolicy=block-new; completion=ok\treason\tADR-001038\tinfo\n", "MISSING_INIT_PHASE"},
			{"row 1 missing policy", validHeader + "2026-11-28T00:00:00Z\tinit\tgoal=Test; completion=ok\treason\tADR-001038\tinfo\n", "INVALID_INIT_POLICY"},
			{"row 1 invalid policy", validHeader + "2026-11-28T00:00:00Z\tinit\tpolicy=allow-all; completion=ok\treason\tADR-001038\tinfo\n", "INVALID_INIT_POLICY"},
			{"row 1 missing completion", validHeader + "2026-11-28T00:00:00Z\tinit\tpolicy=fix-existing; goal=Test\treason\tADR-001038\tinfo\n", "MISSING_INIT_COMPLETION"},
			{"row 1 empty completion", validHeader + "2026-11-28T00:00:00Z\tinit\tpolicy=fix-existing; completion=   \treason\tADR-001038\tinfo\n", "MISSING_INIT_COMPLETION"},
			{"wrong column count", validHeader + validInit + "2026-11-28T00:01:00Z\tverify\tok\treason\tADR-001038\n", "INVALID_COLUMN_COUNT"},
			{"invalid timestamp", validHeader + validInit + "not-a-timestamp\tverify\tok\treason\tADR-001038\tinfo\n", "INVALID_TIMESTAMP"},
			{"empty why", validHeader + validInit + "2026-11-28T00:01:00Z\tverify\tok\t   \tADR-001038\tinfo\n", "EMPTY_FIELD"},
			{"non-pointer evidence", validHeader + validInit + "2026-11-28T00:01:00Z\tverify\tok\treason\tlooks good\tinfo\n", "INVALID_EVIDENCE"},
			{"invalid result", validHeader + validInit + "2026-11-28T00:01:00Z\tverify\tok\treason\tADR-001038\tshipped\n", "INVALID_RESULT"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := filepath.Join(t.TempDir(), "decisions.tsv")
				if err := os.WriteFile(f, []byte(tc.content), fsperm.File); err != nil {
					t.Fatal(err)
				}
				var out, errB bytes.Buffer
				rc := RunHarness(ctx, []string{"validate", "--decisions-file", f, "--format", "json"}, nil, &out, &errB)
				if rc != 1 {
					t.Fatalf("rc=%d want 1 out=%q err=%q", rc, out.String(), errB.String())
				}
				if !strings.Contains(out.String(), tc.wantCode) {
					t.Fatalf("expected %s in json output:\n%s", tc.wantCode, out.String())
				}
			})
		}
	})
}

func TestAppendDecisionFormulaInjectionHygiene(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.tsv")

	if err := AppendDecisionToFile(path, "init", "policy=fix-existing; completion=done", "init row", "ADR-001038", "info"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		decIn, decWant string
		whyIn, whyWant string
	}{
		{"=SUM(A1:A2)", "'=SUM(A1:A2)", "+cmd|'/C calc'!A0", "'+cmd|'/C calc'!A0"},
		{"@import(foo)", "'@import(foo)", "=1+1", "'=1+1"},
		{"-2 scale down", "'-2 scale down", "- normal hyphen prose", "- normal hyphen prose"},
		{"-=1+1", "'-=1+1", "-(1+1)", "'-(1+1)"},
		{"-+cmd", "'-+cmd", "-@SUM(A1)", "'-@SUM(A1)"},
	}
	for _, tc := range cases {
		if err := AppendDecisionToFile(path, "review", tc.decIn, tc.whyIn, "commit:abcdef1", "info"); err != nil {
			t.Fatalf("append (%q, %q): %v", tc.decIn, tc.whyIn, err)
		}
	}
	raw := readTestFile(t, path)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2+len(cases) {
		t.Fatalf("expected %d lines, got %d:\n%s", 2+len(cases), len(lines), string(raw))
	}
	for i, tc := range cases {
		parts := strings.Split(lines[2+i], "\t")
		if parts[2] != tc.decWant {
			t.Fatalf("row %d decision=%q want %q", i, parts[2], tc.decWant)
		}
		if parts[3] != tc.whyWant {
			t.Fatalf("row %d why=%q want %q", i, parts[3], tc.whyWant)
		}
	}
	var out, errB bytes.Buffer
	if rc := RunHarness(context.Background(), []string{"validate", "--decisions-file", path}, nil, &out, &errB); rc != 0 {
		t.Fatalf("validate sanitized decisions.tsv rc=%d out=%q err=%q", rc, out.String(), errB.String())
	}
}

func TestHarnessRunParallelismPersistenceAndTracking(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := initTestGitRepo(t)
	runDir := filepath.Join(repo, ".harness", "runs", "feat-test-branch")

	runH := func(args ...string) (int, string, string) {
		var out, errB bytes.Buffer
		rc := RunHarness(ctx, append(args, "--repo-root", repo), nil, &out, &errB)
		return rc, out.String(), errB.String()
	}

	// Usage lists validate and parallelism.
	if usage := harnessUsage(); !strings.Contains(usage, "validate") || !strings.Contains(usage, "parallelism") {
		t.Fatalf("harnessUsage missing validate/parallelism: %q", usage)
	}

	if rc, _, errB := runH("init", "--policy", "fix-existing", "--goal", "Test LOOP-06", "--completion", "all green"); rc != 0 {
		t.Fatalf("init rc=%d err=%s", rc, errB)
	}

	// Hold state: dry-run and --apply without --record-hold do NOT write parallelism.json or append row.
	rc, out, errB := runH("parallelism", "--apply", "--format", "json")
	if rc != 0 {
		t.Fatalf("hold apply rc=%d err=%s", rc, errB)
	}
	var holdRes map[string]any
	if err := json.Unmarshal([]byte(out), &holdRes); err != nil {
		t.Fatalf("hold json: %v (%s)", err, out)
	}
	if holdRes["current"] != float64(2) || holdRes["recommended"] != float64(2) || holdRes["changed"] != false {
		t.Fatalf("unexpected hold result: %#v", holdRes)
	}
	if _, err := os.Stat(filepath.Join(runDir, "parallelism.json")); !os.IsNotExist(err) {
		t.Fatalf("parallelism.json should not be written on unchanged hold without --record-hold")
	}

	// --apply --record-hold records hold row and writes parallelism.json.
	rc, _, errB = runH("parallelism", "--apply", "--record-hold")
	if rc != 0 {
		t.Fatalf("record-hold rc=%d err=%s", rc, errB)
	}
	parJSONBytes := readTestFile(t, filepath.Join(runDir, "parallelism.json"))
	if !strings.Contains(string(parJSONBytes), `"changed": false`) {
		t.Fatalf("expected changed:false in parallelism.json: %s", string(parJSONBytes))
	}

	// Record 10 adopted outcomes -> scale up 2 -> 4 with --apply.
	for i := 0; i < 10; i++ {
		if rc, _, errB := runH("log", "--phase", "handoff", "--decision", "packet ok", "--why", "verified", "--evidence", "commit:abcdef1", "--result", "adopted"); rc != 0 {
			t.Fatalf("log adopted %d rc=%d err=%s", i, rc, errB)
		}
	}
	rc, out, errB = runH("parallelism", "--apply", "--format", "json")
	if rc != 0 {
		t.Fatalf("scale up rc=%d out=%q err=%q", rc, out, errB)
	}
	var upRes map[string]any
	if err := json.Unmarshal([]byte(out), &upRes); err != nil {
		t.Fatalf("scale up json: %v (%s)", err, out)
	}
	if upRes["current"] != float64(2) || upRes["recommended"] != float64(4) || upRes["delta"] != float64(2) || upRes["changed"] != true {
		t.Fatalf("unexpected scale-up result: %#v", upRes)
	}
	decisionsRaw := readTestFile(t, filepath.Join(runDir, "decisions.tsv"))
	if !strings.Contains(string(decisionsRaw), "\tparallelism\tparallelism 2 -> 4\t") {
		t.Fatalf("decisions.tsv missing parallelism 2 -> 4 row:\n%s", string(decisionsRaw))
	}
	parJSONBytes = readTestFile(t, filepath.Join(runDir, "parallelism.json"))
	var parState map[string]any
	if err := json.Unmarshal(parJSONBytes, &parState); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"current", "recommended", "delta", "window_size", "counts", "reason", "updated_at", "changed"} {
		if _, ok := parState[key]; !ok {
			t.Fatalf("parallelism.json missing key %q: %s", key, string(parJSONBytes))
		}
	}

	// Record 2 mismatch outcomes -> without --current, picks up current=4 from decisions.tsv and scales down 4 -> 2!
	for i := 0; i < 2; i++ {
		if rc, _, errB := runH("log", "--phase", "handoff", "--decision", "mismatch", "--why", "drift", "--evidence", "commit:abcdef2", "--result", "mismatch"); rc != 0 {
			t.Fatalf("log mismatch %d rc=%d err=%s", i, rc, errB)
		}
	}
	rc, out, errB = runH("parallelism", "--apply", "--format", "json")
	if rc != 0 {
		t.Fatalf("scale down rc=%d out=%q err=%q", rc, out, errB)
	}
	var downRes map[string]any
	if err := json.Unmarshal([]byte(out), &downRes); err != nil {
		t.Fatalf("scale down json: %v (%s)", err, out)
	}
	if downRes["current"] != float64(4) || downRes["recommended"] != float64(2) || downRes["delta"] != float64(-2) || downRes["changed"] != true {
		t.Fatalf("expected tracked current=4 -> recommended=2, got: %#v", downRes)
	}
	decisionsRaw = readTestFile(t, filepath.Join(runDir, "decisions.tsv"))
	if !strings.Contains(string(decisionsRaw), "\tparallelism\tparallelism 4 -> 2\t") {
		t.Fatalf("decisions.tsv missing parallelism 4 -> 2 row:\n%s", string(decisionsRaw))
	}

	// Explicit --current 6 overrides tracked value: holds at 6 with 0 new outcomes since last parallelism row.
	rc, out, errB = runH("parallelism", "--current", "6", "--format", "json")
	if rc != 0 {
		t.Fatalf("explicit current hold rc=%d err=%s", rc, errB)
	}
	var holdOverride map[string]any
	if err := json.Unmarshal([]byte(out), &holdOverride); err != nil {
		t.Fatal(err)
	}
	if holdOverride["current"] != float64(6) || holdOverride["recommended"] != float64(6) || holdOverride["changed"] != false {
		t.Fatalf("expected explicit current=6 hold with 0 new outcomes, got: %#v", holdOverride)
	}

	// After 2 new mismatches, explicit --current 6 scales down 6 -> 4 (dry-run).
	for i := 0; i < 2; i++ {
		if rc, _, errB := runH("log", "--phase", "handoff", "--decision", "mismatch-2", "--why", "drift", "--evidence", "commit:abcdef3", "--result", "mismatch"); rc != 0 {
			t.Fatalf("log mismatch-2 %d rc=%d err=%s", i, rc, errB)
		}
	}
	rc, out, errB = runH("parallelism", "--current", "6", "--format", "json")
	if rc != 0 {
		t.Fatalf("explicit current rc=%d err=%s", rc, errB)
	}
	var overrideRes map[string]any
	if err := json.Unmarshal([]byte(out), &overrideRes); err != nil {
		t.Fatal(err)
	}
	if overrideRes["current"] != float64(6) || overrideRes["recommended"] != float64(4) {
		t.Fatalf("expected explicit current=6 -> recommended=4, got: %#v", overrideRes)
	}

	// Invalid --current exits 2.
	for _, bad := range []string{"0", "-2", "abc"} {
		if rc, _, _ := runH("parallelism", "--current", bad); rc != 2 {
			t.Fatalf("--current %q rc=%d want 2", bad, rc)
		}
	}

	// Entire decisions.tsv including all parallelism rows validates cleanly with exit 0!
	if rc, out, errB := runH("validate"); rc != 0 {
		t.Fatalf("validate after parallelism rows rc=%d out=%q err=%q", rc, out, errB)
	}
}

func TestParallelismNoInfiniteReRatchetAndStatusSync(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := initTestGitRepo(t)

	runH := func(args ...string) (int, string, string) {
		var out, errB bytes.Buffer
		rc := RunHarness(ctx, append(args, "--repo-root", repo), nil, &out, &errB)
		return rc, out.String(), errB.String()
	}

	if rc, _, errB := runH("init", "--policy", "block-new", "--goal", "No re-ratchet", "--completion", "go test passes"); rc != 0 {
		t.Fatalf("init rc=%d err=%s", rc, errB)
	}
	for i := 0; i < 10; i++ {
		if rc, _, errB := runH("log", "--phase", "handoff", "--decision", "pkt ok", "--why", "green", "--evidence", "ADR-001038", "--result", "adopted"); rc != 0 {
			t.Fatalf("log %d rc=%d err=%s", i, rc, errB)
		}
	}

	// First --apply scales 2 -> 4 (changed=true).
	rc, out, errB := runH("parallelism", "--apply", "--format", "json")
	if rc != 0 {
		t.Fatalf("first apply rc=%d err=%s", rc, errB)
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(out), &first); err != nil {
		t.Fatalf("json: %v (%s)", err, out)
	}
	if first["current"] != float64(2) || first["recommended"] != float64(4) || first["changed"] != true {
		t.Fatalf("first apply=%#v, want current=2 recommended=4 changed=true", first)
	}

	// Second --apply with 0 new outcomes MUST hold at 4 (changed=false), never infinitely re-ratchet!
	rc, out, errB = runH("parallelism", "--apply", "--format", "json")
	if rc != 0 {
		t.Fatalf("second apply rc=%d err=%s", rc, errB)
	}
	var second map[string]any
	if err := json.Unmarshal([]byte(out), &second); err != nil {
		t.Fatalf("json: %v (%s)", err, out)
	}
	if second["current"] != float64(4) || second["recommended"] != float64(4) || second["changed"] != false || second["window_size"] != float64(0) {
		t.Fatalf("second apply=%#v, want current=4 recommended=4 changed=false window_size=0", second)
	}

	// harness-run status MUST match tracked parallelism=4 (not stale base=2).
	rc, out, errB = runH("status", "--format", "json")
	if rc != 0 {
		t.Fatalf("status rc=%d err=%s", rc, errB)
	}
	var statusPayload map[string]any
	if err := json.Unmarshal([]byte(out), &statusPayload); err != nil {
		t.Fatalf("status json: %v (%s)", err, out)
	}
	curRun := statusPayload["current_run"].(map[string]any)
	par := curRun["parallelism"].(map[string]any)
	if par["base"] != float64(4) || par["recommended"] != float64(4) || par["delta"] != float64(0) || par["window_size"] != float64(0) {
		t.Fatalf("status parallelism=%#v, want base=4 recommended=4 delta=0 window_size=0", par)
	}

	// status & parallelism reject out-of-range --base-parallelism (<= 0 or > 8) with exit 2.
	for _, bad := range []string{"0", "-1", "9", "abc"} {
		if rc, _, _ := runH("status", "--base-parallelism", bad); rc != 2 {
			t.Fatalf("status --base-parallelism %q rc=%d want 2", bad, rc)
		}
		if rc, _, _ := runH("parallelism", "--base-parallelism", bad); rc != 2 {
			t.Fatalf("parallelism --base-parallelism %q rc=%d want 2", bad, rc)
		}
	}
	if rc, _, _ := runH("parallelism", "--current", "9"); rc != 2 {
		t.Fatalf("parallelism --current 9 rc=%d want 2", rc)
	}
}

func TestDecisionsRoundTripAutoInitAndInitSemicolon(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("uninitialized log auto-writes valid Row 1 init and passes validate", func(t *testing.T) {
		t.Parallel()
		repo := initTestGitRepo(t)
		var out, errB bytes.Buffer
		if rc := RunHarness(ctx, []string{"log", "--repo-root", repo, "--phase", "verify", "--decision", "green", "--why", "tests pass", "--evidence", "ADR-001038", "--result", "adopted"}, nil, &out, &errB); rc != 0 {
			t.Fatalf("log rc=%d err=%s", rc, errB.String())
		}
		out.Reset()
		errB.Reset()
		if rc := RunHarness(ctx, []string{"validate", "--repo-root", repo, "--format", "json"}, nil, &out, &errB); rc != 0 {
			t.Fatalf("validate after uninitialized log rc=%d out=%q err=%q", rc, out.String(), errB.String())
		}
		var parsed map[string]any
		if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
			t.Fatalf("json: %v", err)
		}
		if parsed["valid"] != true || parsed["rows"] != float64(2) {
			t.Fatalf("unexpected validate result: %#v", parsed)
		}
	})

	t.Run("init rejects semicolon-only/placeholder goal or completion and semicolon inside goal", func(t *testing.T) {
		t.Parallel()
		badCases := []struct {
			name, goal, completion string
		}{
			{"semicolon-only completion", "test", ";"},
			{"multi-semicolon completion", "test", " ; ; "},
			{"placeholder completion TBD", "test", "TBD"},
			{"placeholder goal N/A", "N/A", "go test ./..."},
			{"semicolon in goal", "fix a; fix b", "go test ./..."},
		}
		for _, tc := range badCases {
			repo := initTestGitRepo(t)
			var out, errB bytes.Buffer
			if rc := RunHarness(ctx, []string{"init", "--repo-root", repo, "--policy", "block-new", "--goal", tc.goal, "--completion", tc.completion}, nil, &out, &errB); rc != 2 {
				t.Fatalf("%s: init rc=%d want 2 out=%q err=%q", tc.name, rc, out.String(), errB.String())
			}
		}
	})

	t.Run("init with multi-command completion containing semicolon passes validate", func(t *testing.T) {
		t.Parallel()
		repo := initTestGitRepo(t)
		var out, errB bytes.Buffer
		if rc := RunHarness(ctx, []string{"init", "--repo-root", repo, "--policy", "block-new", "--goal", "Fix service", "--completion", "cd app; go test ./..."}, nil, &out, &errB); rc != 0 {
			t.Fatalf("init rc=%d err=%s", rc, errB.String())
		}
		out.Reset()
		errB.Reset()
		if rc := RunHarness(ctx, []string{"validate", "--repo-root", repo}, nil, &out, &errB); rc != 0 {
			t.Fatalf("validate multi-command completion rc=%d out=%q err=%q", rc, out.String(), errB.String())
		}
		m := initCompletionRE.FindStringSubmatch("policy=block-new; goal=Fix service; completion=cd app; go test ./...")
		if len(m) < 2 || m[1] != "cd app; go test ./..." {
			t.Fatalf("initCompletionRE truncated multi-command completion: %#v", m)
		}
	})
}

func TestSymlinkEscapeRejectedInHarnessRunAndAppendDecision(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("branch dir symlink escape blocked", func(t *testing.T) {
		t.Parallel()
		repo := initTestGitRepo(t)
		outside := t.TempDir()
		runsDir := filepath.Join(repo, ".harness", "runs")
		if err := os.MkdirAll(runsDir, fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(runsDir, "feat-test-branch")); err != nil {
			t.Fatal(err)
		}
		if _, err := RunDirForBranch(repo, ""); err == nil {
			t.Fatal("RunDirForBranch accepted symlinked branch dir escaping .harness/runs")
		}
		var out, errB bytes.Buffer
		if rc := RunHarness(ctx, []string{"init", "--repo-root", repo, "--policy", "block-new", "--goal", "g", "--completion", "c"}, nil, &out, &errB); rc == 0 {
			t.Fatalf("init succeeded through symlinked branch dir: %s", out.String())
		}
		if _, err := os.Stat(filepath.Join(outside, "decisions.tsv")); !os.IsNotExist(err) {
			t.Fatalf("init wrote decisions.tsv outside repo via symlink!")
		}
	})

	t.Run("harness or runs dir symlink escape blocked", func(t *testing.T) {
		t.Parallel()
		repo1 := initTestGitRepo(t)
		outside1 := t.TempDir()
		if err := os.Symlink(outside1, filepath.Join(repo1, ".harness")); err != nil {
			t.Fatal(err)
		}
		if _, err := RunDirForBranch(repo1, ""); err == nil {
			t.Fatal("RunDirForBranch accepted symlinked .harness dir")
		}

		repo2 := initTestGitRepo(t)
		outside2 := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repo2, ".harness"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside2, filepath.Join(repo2, ".harness", "runs")); err != nil {
			t.Fatal(err)
		}
		if _, err := RunDirForBranch(repo2, ""); err == nil {
			t.Fatal("RunDirForBranch accepted symlinked .harness/runs dir")
		}
	})

	t.Run("dangling decisions.tsv and parallelism.json symlinks rejected", func(t *testing.T) {
		t.Parallel()
		repo := initTestGitRepo(t)
		outside := t.TempDir()
		victimTSV := filepath.Join(outside, "victim.tsv")
		victimPar := filepath.Join(outside, "victim.json")
		runDir := filepath.Join(repo, ".harness", "runs", "feat-test-branch")
		if err := os.MkdirAll(runDir, fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		decPath := filepath.Join(runDir, "decisions.tsv")
		if err := os.Symlink(victimTSV, decPath); err != nil {
			t.Fatal(err)
		}
		if err := AppendDecisionToFile(decPath, "review", "d", "w", "ADR-001038", "info"); err == nil {
			t.Fatal("AppendDecisionToFile followed dangling symlink")
		}
		var out, errB bytes.Buffer
		if rc := RunHarness(ctx, []string{"init", "--repo-root", repo, "--force", "--policy", "block-new", "--goal", "g", "--completion", "c"}, nil, &out, &errB); rc == 0 {
			t.Fatalf("init --force followed dangling decisions.tsv symlink")
		}
		if _, err := os.Stat(victimTSV); !os.IsNotExist(err) {
			t.Fatal("victim.tsv was created through dangling symlink!")
		}

		// Replace decisions.tsv with real init, symlink parallelism.json -> dangling victim.json.
		_ = os.Remove(decPath)
		if rc := RunHarness(ctx, []string{"init", "--repo-root", repo, "--policy", "block-new", "--goal", "g", "--completion", "c"}, nil, &out, &errB); rc != 0 {
			t.Fatalf("init rc=%d err=%s", rc, errB.String())
		}
		if err := os.Symlink(victimPar, filepath.Join(runDir, "parallelism.json")); err != nil {
			t.Fatal(err)
		}
		if rc := RunHarness(ctx, []string{"parallelism", "--repo-root", repo, "--apply", "--record-hold"}, nil, &out, &errB); rc == 0 {
			t.Fatal("parallelism --apply --record-hold followed dangling parallelism.json symlink")
		}
		if _, err := os.Stat(victimPar); !os.IsNotExist(err) {
			t.Fatal("victim.json was created through dangling symlink!")
		}
	})
}

func TestFormulaInjectionAndTabCRInPhaseAndEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	badPointers := []string{
		`=HYPERLINK("http://evil.example/pwn","click")`,
		`=cmd|'/C_calc'!A0`,
		`+cmd|'/C_calc'!A0`,
		`@SUM(1,2)/x`,
		`-1+1/x`,
		`-=cmd/x`,
		`-(1)/x`,
		"cmd:go test\t./...",
		"cmd:go test\r./...",
		"cmd:go test\n./...",
	}
	for _, ev := range badPointers {
		if IsValidEvidencePointer(ev) {
			t.Errorf("IsValidEvidencePointer(%q) = true, want false", ev)
		}
	}

	repo := initTestGitRepo(t)
	var out, errB bytes.Buffer
	if rc := RunHarness(ctx, []string{"init", "--repo-root", repo, "--policy", "block-new", "--goal", "g", "--completion", "c"}, nil, &out, &errB); rc != 0 {
		t.Fatalf("init rc=%d err=%s", rc, errB.String())
	}

	// Repro SEC-02: formula evidence MUST be rejected.
	if rc := RunHarness(ctx, []string{"log", "--repo-root", repo, "--phase", "=SUM(1,2)", "--decision", "ok", "--why", "ok", "--evidence", "=cmd|'/C_calc'!A0", "--result", "info"}, nil, &out, &errB); rc == 0 {
		t.Fatal("log accepted formula-injection evidence")
	}

	// Formula-triggering phase and numeric/formula minus cells must be neutralized and pass validate.
	if rc := RunHarness(ctx, []string{"log", "--repo-root", repo, "--phase", "=SUM(1,2)", "--decision", "-2 scale down", "--why", "-=1+1", "--evidence", "ADR-001038", "--result", "info"}, nil, &out, &errB); rc != 0 {
		t.Fatalf("log with formula phase rc=%d err=%s", rc, errB.String())
	}
	decPath := filepath.Join(repo, ".harness", "runs", "feat-test-branch", "decisions.tsv")
	raw := readTestFile(t, decPath)
	if strings.Contains(string(raw), "\t=SUM(") || strings.Contains(string(raw), "\t-2 ") || strings.Contains(string(raw), "\t-=") {
		t.Fatalf("unescaped formula trigger remained in decisions.tsv:\n%s", string(raw))
	}
	out.Reset()
	errB.Reset()
	if rc := RunHarness(ctx, []string{"validate", "--repo-root", repo}, nil, &out, &errB); rc != 0 {
		t.Fatalf("validate sanitized decisions.tsv rc=%d out=%q err=%q", rc, out.String(), errB.String())
	}

	// Raw unescaped formula trigger in phase column fails ValidateDecisionsFile.
	badTSV := filepath.Join(t.TempDir(), "decisions.tsv")
	content := "ts\tphase\tdecision\twhy\tevidence\tresult\n" +
		"2026-11-28T00:00:00Z\tinit\tpolicy=block-new; completion=ok\tinit\tADR-001038\tinfo\n" +
		"2026-11-28T00:01:00Z\t=SUM(1,2)\tok\tok\tADR-001038\tinfo\n"
	if err := os.WriteFile(badTSV, []byte(content), fsperm.File); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errB.Reset()
	if rc := RunHarness(ctx, []string{"validate", "--decisions-file", badTSV, "--format", "json"}, nil, &out, &errB); rc != 1 {
		t.Fatalf("validate unescaped formula phase rc=%d want 1 out=%q", rc, out.String())
	}
}
