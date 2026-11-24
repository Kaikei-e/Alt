package guards

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
	origSC, origPF, origLayers, origFmt := agyRunSelfcheckFn, agyRunPreflightFn, agyRunLayersFn, agyFormatFileFn
	t.Cleanup(func() {
		agyRunSelfcheckFn = origSC
		agyRunPreflightFn = origPF
		agyRunLayersFn = origLayers
		agyFormatFileFn = origFmt
	})

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

	// adr_preflight exit 2 -> decision: deny.
	agyRunPreflightFn = func(_ context.Context, _ string, _ map[string]any) (string, string, int) {
		return "", "ADR PREFLIGHT (not an error) — scope(s): scripts", 2
	}
	stdout.Reset()
	rc = RunAgyHook(ctx, []string{"pre-edit"}, strings.NewReader(`{"conversationId":"c1","cwd":"`+repo+`","toolCall":{"id":"e4","name":"edit_file","args":{"Path":"scripts/allowed.go","Replacement":"x := 1"}}}`), &stdout, &stderr)
	if rc != 0 || !strings.Contains(stdout.String(), `"deny"`) || !strings.Contains(stdout.String(), "ADR PREFLIGHT") {
		t.Fatalf("adr_preflight hold should deny: rc=%d out=%s", rc, stdout.String())
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
