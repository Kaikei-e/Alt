package guards

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
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
		"go test ./...",
	}
	for _, cmd := range allows {
		code, stderr := runExec(t, cmd, false)
		if code != 0 {
			t.Fatalf("allow %q code=%d stderr=%s", cmd, code, stderr)
		}
	}
}

func TestExecMatchesPython(t *testing.T) {
	py := filepath.Join(repoRoot(t), "scripts", "agent-harness", "guard_exec_commands.py")
	cmds := []string{
		`env -S "golangci-lint run --fix"`,
		`eval "golangci-lint run --fix"`,
		"uv tool run ruff check --unsafe-fixes .",
		"bun x --bun @biomejs/biome check --write --unsafe .",
		"pnpm dlx @biomejs/biome check --write --unsafe .",
		`bash --rcfile rcfile -c "golangci-lint run --fix"`,
		"sleep 5 &",
		"echo hello",
		`echo "a & b"`,
		"make lint TARGET=scripts",
	}
	for _, cmd := range cmds {
		want := pythonExec(t, py, cmd)
		got, err := ClassifyCommand(cmd, true, true, 0)
		if err != nil {
			t.Fatal(err)
		}
		gotLabel := ""
		if got != nil {
			gotLabel = got.rule + "\t" + got.detail
		}
		if gotLabel != want {
			t.Fatalf("cmd %q\n got %q\nwant %q", cmd, gotLabel, want)
		}
	}
}

func TestOpsComposeRules(t *testing.T) {
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

func pythonExec(t *testing.T, script, cmd string) string {
	t.Helper()
	py := exec.Command("python3", "-c", `
import importlib.util, sys
spec = importlib.util.spec_from_file_location("gexec", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
v = mod.classify_command(sys.argv[2], enforce_e2=True, enforce_e3=True)
print("" if v is None else v[0] + "\t" + v[1])
`, script, cmd)
	out, err := py.Output()
	if err != nil {
		t.Fatalf("python: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "agent-harness", "guard_exec_commands.py")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo not found")
		}
		dir = parent
	}
}
