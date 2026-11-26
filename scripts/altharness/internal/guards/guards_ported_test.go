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

func claudeWriteJSON(path, content string) string {
	raw, _ := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Write",
		"tool_input":      map[string]any{"file_path": path, "content": content},
	})
	return string(raw)
}

func claudeEditJSON(path, oldStr, newStr string) string {
	raw, _ := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Edit",
		"tool_input":      map[string]any{"file_path": path, "old_string": oldStr, "new_string": newStr},
	})
	return string(raw)
}

func codexPatchJSON(patch string) string {
	raw, _ := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "apply_patch",
		"tool_input":      map[string]any{"command": patch},
	})
	return string(raw)
}

func bashHookJSON(cmd any, agentID, subagentType string) string {
	payload := map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": cmd},
	}
	if agentID != "" {
		payload["agent_id"] = agentID
	}
	if subagentType != "" {
		payload["subagent_type"] = subagentType
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func editHookWithCwdJSON(path, tool, cwd string) string {
	var input map[string]any
	switch tool {
	case "Write":
		input = map[string]any{"file_path": path, "content": "x = 1\n"}
	case "apply_patch":
		input = map[string]any{"command": "*** Begin Patch\n*** Update File: " + path + "\n@@\n+x = 1\n*** End Patch\n"}
	default:
		input = map[string]any{"file_path": path, "old_string": "a", "new_string": "b"}
	}
	payload := map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       tool,
		"tool_input":      input,
	}
	if cwd != "" {
		payload["cwd"] = cwd
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func samplePacketWithScope(id string, allowed, forbidden []string, allowGen, includeMeasured bool) string {
	verifyCmd := "./scripts/lint/run.sh scripts"
	sum := sha256.Sum256([]byte(verifyCmd))
	sha := hex.EncodeToString(sum[:])
	var allowB, forbidB strings.Builder
	for _, p := range allowed {
		allowB.WriteString("    - \"" + p + "\"\n")
	}
	for _, p := range forbidden {
		forbidB.WriteString("    - \"" + p + "\"\n")
	}
	allowGenStr := "false"
	if allowGen {
		allowGenStr = "true"
	}
	measuredBlock := ""
	if includeMeasured {
		measuredBlock = "  measured:\n" +
			"    duration_s: 1.5\n" +
			"    exit_code: 0\n" +
			"    at_commit: \"1111111111111111111111111111111111111111\"\n" +
			"    command_sha256: \"" + sha + "\"\n"
	}
	return "---\n" +
		"packet_version: \"1.0\"\n" +
		"packet_id: \"" + id + "\"\n" +
		"title: \"Test packet\"\n" +
		"scope:\n" +
		"  target: \"scripts\"\n" +
		"  allowed_paths:\n" + allowB.String() +
		"  forbidden_paths:\n" + forbidB.String() +
		"  allow_generated_code_edits: " + allowGenStr + "\n" +
		"  max_files: 5\n" +
		"  max_functions: 3\n" +
		"  allowed_rules:\n    - \"S101\"\n" +
		"  stop_condition: \"done\"\n" +
		"execution_policy:\n" +
		"  policy: \"fix-existing\"\n" +
		"  foreground_only: true\n" +
		"  forbid_repo_wide_test_build: true\n" +
		"  forbid_bulk_autofix: true\n" +
		"  verification_command: \"" + verifyCmd + "\"\n" +
		measuredBlock +
		"handoff_requirements:\n" +
		"  require_modified_files_and_lines: true\n" +
		"  require_runtime_behavior_impact: true\n" +
		"  require_change_rationale: true\n" +
		"---\n\n# Task Packet " + id + "\n\n## 1. 範囲と作業量制約\n\n## 2. 実行・検証手順\n\n## 3. Hand-off 記述必須項目\n"
}

func sampleHandoffMD(id string) string {
	return "---\n" +
		"packet_version: \"1.0\"\n" +
		"packet_id: \"" + id + "\"\n" +
		"status: \"completed\"\n" +
		"modified_files:\n" +
		"  - path: \"scripts/altharness/foo.go\"\n" +
		"    added_lines: 10\n" +
		"    deleted_lines: 2\n" +
		"modified_functions_count: 1\n" +
		"runtime_behavior_impact:\n" +
		"  has_change: false\n" +
		"  checked_dimensions:\n" +
		"    - \"control_flow\"\n" +
		"  summary: \"変更なし\"\n" +
		"rationale: \"test handoff\"\n" +
		"verification:\n" +
		"  command: \"./scripts/lint/run.sh scripts\"\n" +
		"  exit_code: 0\n" +
		"---\n\n# Hand-off " + id + "\n"
}

func TestOpsBranchesParallel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cmd  string
		want string
	}{
		{"double quote escaped quote", `sh -c "docker rm \"x\""`, "O2"},
		{"double quote line continuation", "sh -c \"docker rm \\\nx\"", "O2"},
		{"normal backslash line continuation", "docker \\\nrm x", "O2"},
		{"normal escaped space in arg", `docker rm my\ container`, "O2"},
		{"normal escaped semicolon", `echo a\; docker rm x`, "O2"},
		{"normal trailing backslash", `docker ps\`, ""},
		{"comment line skips rm then allows ps", "# docker rm x\ndocker ps", ""},
		{"comment line followed by rm asks O2", "# benign comment\ndocker rm x", "O2"},
		{"mid-token hash preserves token", "docker rm foo#1", "O2"},
		{"subshell parens split docker rm", "(docker rm x)", "O2"},
		{"escaped dollar-paren", `echo \$(docker rm x)`, "O2"},
		{"single-quoted dollar-paren", "echo '$(docker rm x)'", ""},
		{"single quote inside $(...)", "echo $(sh -c 'docker rm x')", "O2"},
		{"double quote + escape inside $(...)", `echo $(sh -c "docker rm \"x\"")`, "O2"},
		{"backslash escape inside $(...)", `echo $(echo \) && docker rm x)`, "O2"},
		{"nested parens inside $(...)", "echo $((1+1)) && echo $( (docker rm x) )", "O2"},
		{"backtick substitution", "echo `docker rm x`", "O2"},
		{"escaped backtick inside backticks", "echo `echo \\` && docker rm x`", "O2"},
		{"compose -- up -d", "docker compose -- up -d", "O3"},
		{"compose -- up --build -d", "docker compose -- up --build -d", "O4"},
		{"compose -- down", "docker compose -- down", "O3"},
		{"compose -- stop", "docker compose -- stop alt-backend", "O2"},
		{"compose -- restart", "docker compose -- restart alt-backend", "O3"},
		{"compose up -- --build", "docker compose up -- --build", "O3"},
		{"compose up -- -p alt-test", "docker compose up -- -p alt-test", "O3"},
		{"compose -- no subcmd", "docker compose --", ""},
		{"compose dangling -p", "docker compose up -p", "O3"},
		{"compose dangling -p with staging env", "COMPOSE_PROJECT_NAME=staging docker compose up -p", "O3"},
		{"compose -p=alt-test", "docker compose -p=alt-test up -d", ""},
		{"compose -p=alt", "docker compose -p=alt up -d", "O3"},
		{"compose empty -p", `docker compose -p "" up -d`, "O3"},
		{"compose empty -p with staging env", `COMPOSE_PROJECT_NAME=staging docker compose -p "" up -d`, "O3"},
		{"compose empty --project-name with staging env", `COMPOSE_PROJECT_NAME=staging docker compose --project-name="" up --build -d`, "O4"},
		{"compose empty -p= with staging env", `COMPOSE_PROJECT_NAME=staging docker compose -p="" up -d`, "O3"},
		{"compose down --build is O3", "docker compose -p alt down --build", "O3"},
		{"compose restart prod", "docker compose -p alt restart alt-backend", "O3"},
		{"compose restart default", "docker compose restart alt-backend", "O3"},
		{"compose restart isolated", "docker compose -p alt-test restart alt-backend", ""},
		{"docker -- rm x", "docker -- rm x", "O2"},
		{"docker -- compose -- up -d", "docker -- compose -- up -d", "O3"},
		{"docker --", "docker --", ""},
		{"docker --debug rm x", "docker --debug rm x", "O2"},
		{"bare docker", "docker", ""},
		{"docker --help", "docker --help", ""},
		{"docker container -- rm x", "docker container -- rm x", "O2"},
		{"docker system -- prune -af", "docker system -- prune -af", "O1"},
		{"docker container --help", "docker container --help", ""},
		{"docker container prune -f", "docker container prune -f", "O1"},
		{"docker volume prune -f", "docker volume prune -f", "O1"},
		{"docker network remove", "docker network remove alt-net", "O1"},
		{"docker container kill", "docker container kill alt-backend", "O2"},
		{"podman rm x", "podman rm x", ""},
		{"quoted double docker rm", `"docker" rm x`, "O2"},
		{"quoted single docker rm", `'docker' rm x`, "O2"},
		{"mid-word quoted docker rm", `d'o'cker rm x`, "O2"},
		{"ANSI-C docker rm", `$'docker' rm x`, "O2"},
		{"ANSI-C hex docker rm", `$'\x64ocker' rm x`, "O2"},
		{"rg docker rm", `rg "docker rm" docs/`, ""},
		{"rg quoted docker", `rg "docker" rm docs/`, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			rc := RunOps(context.Background(), nil, strings.NewReader(bashHookJSON(tc.cmd, "", "")), &stdout, &stderr)
			if rc != 0 {
				t.Fatalf("rc=%d stderr=%q", rc, stderr.String())
			}
			out := strings.TrimSpace(stdout.String())
			if tc.want == "" {
				if out != "" {
					t.Fatalf("expected silent allow for %q, got %s", tc.cmd, out)
				}
				return
			}
			var parsed struct {
				HookSpecificOutput struct {
					HookEventName            string `json:"hookEventName"`
					PermissionDecision       string `json:"permissionDecision"`
					PermissionDecisionReason string `json:"permissionDecisionReason"`
				} `json:"hookSpecificOutput"`
			}
			if err := json.Unmarshal([]byte(out), &parsed); err != nil {
				t.Fatalf("invalid json %q: %v", out, err)
			}
			if parsed.HookSpecificOutput.PermissionDecision != "ask" || !strings.Contains(parsed.HookSpecificOutput.PermissionDecisionReason, "Rule "+tc.want+" ") {
				t.Fatalf("cmd=%q want Rule %s ask, got %s", tc.cmd, tc.want, out)
			}
		})
	}
}

func TestOpsRulesAskDenyAndUnreadableParallel(t *testing.T) {
	t.Parallel()
	askCases := []struct {
		name string
		cmd  string
		rule string
		pm   string
	}{
		{"O1 system prune", "docker --context default system prune -af", "O1", "PM-2026-054"},
		{"O1 chained sudo image prune", "docker ps && sudo docker image prune -a -f", "O1", "PM-2026-054"},
		{"O1 volume rm and rmi", "docker volume rm old_certs; docker rmi alt-backend:old", "O1", "PM-2026-046"},
		{"O1 DOCKER_HOST image rm", "DOCKER_HOST=unix:///var/run/docker.sock docker image rm myimg", "O1", "PM-2026-054"},
		{"O1 network prune", "docker network prune -f", "O1", "PM-2026-046"},
		{"O1 builder prune", "docker builder prune --all", "O1", "PM-2026-054"},
		{"O1 network rm", "docker network rm alt-net", "O1", "PM-2026-046"},
		{"O1 image remove", "docker image remove myimg:old", "O1", "PM-2026-054"},
		{"O1 volume remove", "docker volume remove old_certs", "O1", "PM-2026-046"},
		{"O1 buildx prune", "docker buildx prune -f", "O1", "PM-2026-054"},
		{"O1 sh -c double quotes", `sh -c "docker system prune -af"`, "O1", "PM-2026-054"},
		{"O2 docker -l debug rm", "docker -l debug rm -f pki-agent", "O2", "PM-2026-028"},
		{"O2 chained container stop", "echo checking && docker container stop -t 5 alt-backend", "O2", "PM-2026-028"},
		{"O2 sudo -E kill", "sudo -E docker kill alt-data-hub", "O2", "PM-2026-054"},
		{"O2 container rm", "docker container rm alt-backend", "O2", "PM-2026-028"},
		{"O2 container remove", "docker container remove alt-backend", "O2", "PM-2026-028"},
		{"O2 compose stop", "docker compose -f compose/compose.yaml -p alt stop alt-backend", "O2", "PM-2026-028"},
		{"O2 compose kill", "docker compose kill alt-backend", "O2", "PM-2026-028"},
		{"O2 for loop", "for c in $(docker ps -aq --filter name=alt-staging-); do docker rm -f $c; done", "O2", "PM-2026-028"},
		{"O2 while read loop", "while read c; do docker stop $c; done < ids.txt", "O2", "PM-2026-028"},
		{"O2 pipe xargs -r", "docker ps -q --filter name=alt-staging- | xargs -r docker rm -f", "O2", "PM-2026-028"},
		{"O2 bare xargs", "xargs docker rm", "O2", "PM-2026-028"},
		{"O2 braces block", "{ docker rm x; }", "O2", "PM-2026-028"},
		{"O2 command substitution", `echo "$(docker rm x)"`, "O2", "PM-2026-028"},
		{"O2 sudo -iu root", "sudo -iu root docker rm x", "O2", "PM-2026-028"},
		{"O3 compose up default", "docker compose -f compose/compose.yaml up -d alt-backend", "O3", "PM-2026-001"},
		{"O3 compose down -p alt", "docker compose -f compose/compose.yaml -p alt down", "O3", "PM-2026-005"},
		{"O3 COMPOSE_PROJECT_NAME=alt rm", "COMPOSE_PROJECT_NAME=alt docker-compose rm -f", "O3", "PM-2026-001"},
		{"O3 chained --project-name alt", "git status || docker compose --project-name alt up -d", "O3", "PM-2026-005"},
		{"O3 if then fi", "if true; then docker compose -p alt down; fi", "O3", "PM-2026-001"},
		{"O3 bash -c single quotes", "bash -c 'docker compose -p alt down'", "O3", "PM-2026-001"},
		{"O3 time prefix", "time docker compose -p alt down", "O3", "PM-2026-001"},
		{"O3 -p $PROJ", "docker compose -p $PROJ down", "O3", "PM-2026-001"},
		{"O3 -p $COMPOSE_PROJECT_NAME", `docker compose -p "$COMPOSE_PROJECT_NAME" up -d`, "O3", "PM-2026-001"},
		{"O4 compose up --build", "docker compose up --build -d alt-backend", "O4", "PM-2026-031"},
		{"O4 compose up --force-recreate", "docker compose -f compose/compose.yaml -p alt up -d --no-deps --force-recreate alt-backend", "O4", "PM-2026-031"},
		{"O4 chained docker-compose --build", "echo start\nCOMPOSE_PROJECT_NAME=alt docker-compose -f compose/compose.yaml up --build -d", "O4", "PM-2026-031"},
		{"O4 timeout prefix", "timeout 300 docker compose -f compose/compose.yaml -p alt up -d --build alt-backend", "O4", "PM-2026-031"},
		{"O4 nohup background", "nohup docker compose -p alt up -d --build &", "O4", "PM-2026-031"},
	}
	for _, tc := range askCases {
		tc := tc
		t.Run("ask/"+tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			rc := RunOps(context.Background(), nil, strings.NewReader(bashHookJSON(tc.cmd, "", "")), &stdout, &stderr)
			if rc != 0 {
				t.Fatalf("rc=%d stderr=%q", rc, stderr.String())
			}
			var parsed map[string]map[string]string
			if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
				t.Fatalf("json: %v (%s)", err, stdout.String())
			}
			h := parsed["hookSpecificOutput"]
			reason := h["permissionDecisionReason"]
			if h["hookEventName"] != "PreToolUse" || h["permissionDecision"] != "ask" || !strings.Contains(reason, tc.rule) || !strings.Contains(reason, tc.pm) {
				t.Fatalf("unexpected ask output: %s", stdout.String())
			}
			if tc.rule == "O4" && (!strings.Contains(reason, "release path") || !strings.Contains(reason, "contract gate")) {
				t.Fatalf("O4 missing guidance: %s", reason)
			}
		})
	}

	t.Run("G5 argv bash -lc", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		rc := RunOps(context.Background(), nil, strings.NewReader(bashHookJSON([]string{"bash", "-lc", "docker compose -p alt down"}, "", "")), &stdout, &stderr)
		if rc != 0 || !strings.Contains(stdout.String(), `"ask"`) || !strings.Contains(stdout.String(), "O3") {
			t.Fatalf("rc=%d out=%s err=%s", rc, stdout.String(), stderr.String())
		}
	})

	allows := []struct {
		name  string
		stdin string
	}{
		{"docker ps", bashHookJSON("docker ps -a", "", "")},
		{"docker logs", bashHookJSON("docker logs --tail 50 alt-backend", "", "")},
		{"docker inspect", bashHookJSON("docker inspect alt-backend", "", "")},
		{"docker images", bashHookJSON("docker images", "", "")},
		{"docker version", bashHookJSON("docker version", "", "")},
		{"docker exec", bashHookJSON("docker exec alt-backend /alt-backend healthcheck", "", "")},
		{"compose config", bashHookJSON("docker compose -f compose/compose.yaml config", "", "")},
		{"compose ps", bashHookJSON("docker compose -f compose/compose.yaml -p alt ps", "", "")},
		{"compose logs", bashHookJSON("docker compose -p alt logs -f alt-backend", "", "")},
		{"-p alt-test", bashHookJSON("docker compose -f compose/compose.yaml -p alt-test up --build -d", "", "")},
		{"-p after subcmd", bashHookJSON("docker compose -f compose/compose.yaml up --build -d -p alt-test", "", "")},
		{"--project-name=e2e", bashHookJSON("docker compose --project-name=e2e down", "", "")},
		{"COMPOSE_PROJECT_NAME=staging", bashHookJSON("COMPOSE_PROJECT_NAME=staging docker compose up --force-recreate -d", "", "")},
		{"cli -p overrides env", bashHookJSON("COMPOSE_PROJECT_NAME=alt docker compose -p test-proj up -d", "", "")},
		{"alt-test stop", bashHookJSON("docker compose -p alt-test stop alt-backend", "", "")},
		{"echo double quotes", bashHookJSON(`echo "docker system prune -af && docker compose up --build"`, "", "")},
		{"git commit msg", bashHookJSON(`git commit -m 'PM-2026-031: never run docker compose up --build -d alt-backend'`, "", "")},
		{"G5 argv echo semicolon", bashHookJSON([]string{"echo", "a; docker rm x"}, "", "")},
		{"empty command", bashHookJSON("", "", "")},
		{"whitespace command", bashHookJSON("   \n  ", "", "")},
		{"non-shell tool Read", `{"tool_name":"Read","tool_input":{"file_path":".env"}}`},
	}
	for _, tc := range allows {
		tc := tc
		t.Run("allow/"+tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			rc := RunOps(context.Background(), nil, strings.NewReader(tc.stdin), &stdout, &stderr)
			if rc != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("expected silent allow, got rc=%d out=%q err=%q", rc, stdout.String(), stderr.String())
			}
		})
	}

	unreadable := []struct {
		name string
		raw  []byte
	}{
		{"empty stdin", []byte("")},
		{"invalid json", []byte("{bad")},
		{"json array", []byte("[]")},
		{"empty object", []byte("{}")},
		{"missing tool_input", []byte(`{"tool_name":"Bash"}`)},
		{"missing command", []byte(`{"tool_name":"Bash","tool_input":{}}`)},
		{"non-string command", []byte(`{"tool_name":"Bash","tool_input":{"command":42}}`)},
		{"invalid utf8", []byte{0xff, 0xfe}},
		{"deeply nested json", []byte(strings.Repeat("[", 2000) + strings.Repeat("]", 2000))},
	}
	for _, tc := range unreadable {
		tc := tc
		t.Run("unreadable/"+tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			rc := RunOps(context.Background(), nil, bytes.NewReader(tc.raw), &stdout, &stderr)
			lower := strings.ToLower(stdout.String())
			if rc != 0 || !strings.Contains(stdout.String(), `"ask"`) || (!strings.Contains(lower, "unreadable") && !strings.Contains(lower, "could not be analysed")) {
				t.Fatalf("unreadable %s: rc=%d out=%q err=%q", tc.name, rc, stdout.String(), stderr.String())
			}
		})
	}

	denyRules := []struct {
		rule string
		cmd  string
	}{
		{"O1", "docker system prune -af"},
		{"O2", "docker rm -f alt-backend"},
		{"O3", "docker compose -p alt down"},
		{"O4", "docker compose -p alt up --build -d alt-backend"},
	}
	for _, tc := range denyRules {
		tc := tc
		t.Run("deny/"+tc.rule, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			rc := RunOps(context.Background(), []string{"--on-match", "deny"}, strings.NewReader(bashHookJSON(tc.cmd, "", "")), &stdout, &stderr)
			lower := strings.ToLower(stdout.String())
			if rc != 0 || !strings.Contains(stdout.String(), `"deny"`) || !strings.Contains(stdout.String(), tc.rule) || !strings.Contains(lower, "retry") || !strings.Contains(lower, "user") || !strings.Contains(lower, "quote") {
				t.Fatalf("deny %s: rc=%d out=%q", tc.rule, rc, stdout.String())
			}
		})
	}

	t.Run("deny/unreadable", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		rc := RunOps(context.Background(), []string{"--on-match", "deny"}, strings.NewReader("{bad"), &stdout, &stderr)
		if rc != 0 || !strings.Contains(stdout.String(), `"deny"`) || !strings.Contains(strings.ToLower(stdout.String()), "unreadable") {
			t.Fatalf("rc=%d out=%q", rc, stdout.String())
		}
	})
}

func TestProtectedPathsComprehensiveParallel(t *testing.T) {
	t.Parallel()
	blocked := []string{
		".env",
		"/repo/.env",
		".env.local",
		"compose/.env.production",
		"irodori-tts/configs/C1.env",
		"certs/server.key",
		"certs/ca.pem",
		"secrets/clickhouse_password.txt",
		"/repo/secrets/token.txt",
		".kisage/state.json",
		".git/config",
	}
	for _, p := range blocked {
		p := p
		t.Run("block/"+p, func(t *testing.T) {
			t.Parallel()
			for _, payload := range []string{
				claudeWriteJSON(p, "x=1\n"),
				claudeEditJSON(p, "a", "b"),
				codexPatchJSON("*** Begin Patch\n*** Update File: " + p + "\n@@\n+x=1\n*** End Patch\n"),
			} {
				var stderr bytes.Buffer
				rc := RunProtected(context.Background(), nil, strings.NewReader(payload), nil, &stderr)
				if rc != 2 || !strings.Contains(stderr.String(), p) {
					t.Fatalf("path=%s rc=%d stderr=%q", p, rc, stderr.String())
				}
			}
		})
	}

	allowed := []string{
		".env.example",
		"compose/.env.example",
		".env.template",
		".env.sample",
		"alt-backend/app/config/secrets.go",
		"e2e/fixtures/staging-secrets/x.txt",
		"alt-backend/app/main.go",
	}
	for _, p := range allowed {
		p := p
		t.Run("allow/"+p, func(t *testing.T) {
			t.Parallel()
			for _, payload := range []string{
				claudeWriteJSON(p, "package main\n"),
				claudeEditJSON(p, "a", "b"),
				codexPatchJSON("*** Begin Patch\n*** Add File: " + p + "\n+hello\n*** End Patch\n"),
			} {
				var stderr bytes.Buffer
				rc := RunProtected(context.Background(), nil, strings.NewReader(payload), nil, &stderr)
				if rc != 0 {
					t.Fatalf("path=%s rc=%d stderr=%q", p, rc, stderr.String())
				}
			}
		})
	}

	t.Run("apply_patch Move to secrets", func(t *testing.T) {
		t.Parallel()
		patch := "*** Begin Patch\n*** Update File: alt-backend/app/main.go\n@@\n+x := 1\n*** Move to: secrets/leaked.txt\n*** End Patch\n"
		var stderr bytes.Buffer
		if rc := RunProtected(context.Background(), nil, strings.NewReader(codexPatchJSON(patch)), nil, &stderr); rc != 2 {
			t.Fatalf("rc=%d stderr=%q", rc, stderr.String())
		}
	})

	t.Run("migrations tracked vs untracked vs non-git", func(t *testing.T) {
		t.Parallel()
		repo := initTestGitRepo(t)
		migDir := filepath.Join(repo, "db", "migrations")
		if err := os.MkdirAll(migDir, fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		tracked := filepath.Join(migDir, "001_init.sql")
		if err := os.WriteFile(tracked, []byte("CREATE TABLE t (id INT);\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		runGitInTest(t, repo, "add", "db/migrations/001_init.sql")
		runGitInTest(t, repo, "commit", "-m", "add migration")

		for _, payload := range []string{
			claudeWriteJSON(tracked, "ALTER TABLE t;\n"),
			claudeEditJSON(tracked, "id", "uid"),
			editHookWithCwdJSON("db/migrations/001_init.sql", "apply_patch", repo),
		} {
			var stderr bytes.Buffer
			rc := RunProtected(context.Background(), nil, strings.NewReader(payload), nil, &stderr)
			if rc != 2 || !strings.Contains(strings.ToLower(stderr.String()), "atlas") {
				t.Fatalf("tracked migration rc=%d stderr=%q", rc, stderr.String())
			}
		}

		untracked := filepath.Join(migDir, "002_new.sql")
		newDirMig := filepath.Join(repo, "new_svc", "migrations", "001_init.sql")
		for _, payload := range []string{
			claudeWriteJSON(untracked, "CREATE TABLE u (id INT);\n"),
			claudeEditJSON(untracked, "a", "b"),
			claudeWriteJSON(newDirMig, "CREATE TABLE v (id INT);\n"),
		} {
			var stderr bytes.Buffer
			if rc := RunProtected(context.Background(), nil, strings.NewReader(payload), nil, &stderr); rc != 0 {
				t.Fatalf("untracked migration rc=%d stderr=%q", rc, stderr.String())
			}
		}

		nonGitDir := t.TempDir()
		nonGitMig := filepath.Join(nonGitDir, "db", "migrations", "001_init.sql")
		if err := os.MkdirAll(filepath.Dir(nonGitMig), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		if rc := RunProtected(context.Background(), nil, strings.NewReader(claudeWriteJSON(nonGitMig, "CREATE TABLE x;\n")), nil, &stderr); rc != 2 {
			t.Fatalf("non-git migration should fail closed, rc=%d stderr=%q", rc, stderr.String())
		}
	})

	t.Run("fail-closed and other tools", func(t *testing.T) {
		t.Parallel()
		badInputs := []string{
			"", "{bad", "[]", `"hello"`, "{}",
			`{"tool_name":"Edit","tool_input":{"new_string":"x"}}`,
			`{"tool_name":"Write","tool_input":{"file_path":"   "}}`,
			`{"tool_name":"NotebookEdit","tool_input":{}}`,
			codexPatchJSON("just plain text"),
		}
		for _, raw := range badInputs {
			var stderr bytes.Buffer
			rc := RunProtected(context.Background(), nil, strings.NewReader(raw), nil, &stderr)
			if rc != 2 || !strings.Contains(strings.ToLower(stderr.String()), "payload") {
				t.Fatalf("raw=%q rc=%d stderr=%q", raw, rc, stderr.String())
			}
		}
		var stderr bytes.Buffer
		if rc := RunProtected(context.Background(), nil, strings.NewReader(`{"tool_name":"Read","tool_input":{"file_path":".env"}}`), nil, &stderr); rc != 0 {
			t.Fatalf("Read should allow, rc=%d", rc)
		}
	})
}

func TestEventTimeComprehensiveParallel(t *testing.T) {
	t.Parallel()
	homeGo := "/abs/worktree/knowledge-sovereign/app/usecase/knowledge_home_projector/projector.go"
	trailGo := "knowledge-sovereign/app/usecase/knowledge_trail_projector/folds.go"
	testGo := "knowledge-sovereign/app/usecase/knowledge_home_projector/projector_test.go"
	unguardedGo := "knowledge-sovereign/app/driver/repo.go"

	snippets := []struct {
		name string
		code string
	}{
		{"time.Now", "now := time.Now()\n"},
		{"time.Since", "d := time.Since(ev.OccurredAt)\n"},
		{"time.Until", "d := time.Until(deadline)\n"},
		{"GetLatestVersion", "v := repo.GetLatestVersion(ctx)\n"},
	}
	for _, s := range snippets {
		s := s
		t.Run("block/"+s.name, func(t *testing.T) {
			t.Parallel()
			for _, target := range []string{homeGo, trailGo} {
				var stderr bytes.Buffer
				rc := RunEventTime(context.Background(), nil, strings.NewReader(claudeWriteJSON(target, s.code)), nil, &stderr)
				if rc != 2 || !strings.Contains(stderr.String(), "ADR-000468") || !strings.Contains(stderr.String(), "ADR-000447") || !strings.Contains(stderr.String(), "OccurredAt") {
					t.Fatalf("Write %s rc=%d stderr=%q", target, rc, stderr.String())
				}
				stderr.Reset()
				rc = RunEventTime(context.Background(), nil, strings.NewReader(claudeEditJSON(target, "x := 1", s.code)), nil, &stderr)
				if rc != 2 {
					t.Fatalf("Edit %s rc=%d stderr=%q", target, rc, stderr.String())
				}
			}
		})
	}

	t.Run("exemptions and multi-file apply_patch", func(t *testing.T) {
		t.Parallel()
		allows := []string{
			claudeWriteJSON(testGo, "now := time.Now()\nv := repo.GetLatestItem(ctx)\n"),
			claudeWriteJSON(unguardedGo, "now := time.Now()\nv := repo.GetLatestItem(ctx)\n"),
			claudeWriteJSON(homeGo, "// Never call time.Now() or GetLatestVersion here.\nt := ev.OccurredAt // instead of time.Since(x)\n"),
			claudeEditJSON(homeGo, "now := time.Now()", "now := ev.OccurredAt"),
			codexPatchJSON("*** Begin Patch\n*** Update File: " + unguardedGo + "\n@@\n+now := time.Now()\n*** Update File: " + trailGo + "\n@@\n+ts := ev.OccurredAt\n*** End Patch\n"),
			`{"tool_name":"Bash","tool_input":{"command":"echo hi"}}`,
		}
		for _, payload := range allows {
			var stderr bytes.Buffer
			if rc := RunEventTime(context.Background(), nil, strings.NewReader(payload), nil, &stderr); rc != 0 {
				t.Fatalf("expected allow, rc=%d stderr=%q", rc, stderr.String())
			}
		}

		badPatch := codexPatchJSON("*** Begin Patch\n*** Update File: " + unguardedGo + "\n@@\n+ts := ev.OccurredAt\n*** Update File: " + trailGo + "\n@@\n+s := store.GetLatestState(ctx)\n*** End Patch\n")
		var stderr bytes.Buffer
		if rc := RunEventTime(context.Background(), nil, strings.NewReader(badPatch), nil, &stderr); rc != 2 {
			t.Fatalf("expected block on guarded file in apply_patch, rc=%d", rc)
		}
	})
}

func TestExecComprehensiveParallel(t *testing.T) {
	t.Parallel()
	e1Blocked := []struct {
		name string
		cmd  any
	}{
		{"golangci-lint --fix", "golangci-lint run --fix ./..."},
		{"golangci-lint --fix=true", "golangci-lint --fix=true"},
		{"cargo clippy --fix", "cargo clippy --fix --allow-dirty"},
		{"cargo +nightly clippy --fix", "cargo +nightly clippy --all-targets --fix"},
		{"clippy-driver --fix", "clippy-driver --fix foo.rs"},
		{"cargo fix", "cargo fix --allow-dirty"},
		{"ruff --unsafe-fixes", "ruff check --unsafe-fixes ."},
		{"uv run ruff --unsafe-fixes", "uv run --no-sync ruff check --fix --unsafe-fixes ."},
		{"biome --unsafe", "biome check --write --unsafe src/"},
		{"npx biome --unsafe", "npx --no-install biome lint --unsafe src/"},
		{"bash -c golangci-lint --fix", "bash -c 'golangci-lint run --fix ./...'"},
		{"subshell ruff --unsafe-fixes", `echo "$(ruff check --unsafe-fixes .)"`},
		{"chained cargo fix", "echo ok && cargo fix"},
		{"env sudo golangci-lint --fix", "GOFLAGS=-v sudo -E golangci-lint run --fix"},
		{"argv list form", []string{"golangci-lint", "run", "--fix", "./..."}},
	}
	for _, tc := range e1Blocked {
		tc := tc
		t.Run("E1/block/"+tc.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			rc := RunExec(context.Background(), nil, strings.NewReader(bashHookJSON(tc.cmd, "", "")), nil, &stderr)
			if rc != 2 || !strings.Contains(stderr.String(), "BLOCKED [EXEC-01]") {
				t.Fatalf("rc=%d stderr=%q", rc, stderr.String())
			}
		})
	}

	e1Allowed := []string{
		"golangci-lint run ./...",
		"golangci-lint run --fix=false ./...",
		"cargo clippy --all-targets -- -D warnings",
		"uv run ruff check --fix .",
		"biome check --write src/",
		`git commit -m "ban golangci-lint --fix"`,
		`echo "--unsafe-fixes"`,
		"rg golangci-lint --fix docs/",
	}
	for _, cmd := range e1Allowed {
		cmd := cmd
		t.Run("E1/allow/"+cmd, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			rc := RunExec(context.Background(), nil, strings.NewReader(bashHookJSON(cmd, "", "")), nil, &stderr)
			if rc != 0 || stderr.Len() != 0 {
				t.Fatalf("rc=%d stderr=%q", rc, stderr.String())
			}
		})
	}

	t.Run("worker E2 and E3 rules", func(t *testing.T) {
		t.Parallel()
		e2Blocked := []string{
			bashHookJSON("./scripts/lint/run.sh all", "w-1", ""),
			bashHookJSON("scripts/lint/run.sh all", "w-1", ""),
			bashHookJSON("bash ./scripts/lint/run.sh --policy block-new all", "w-1", ""),
			bashHookJSON("make lint", "", "impl"),
			bashHookJSON("make lint TARGET=all", "w-1", ""),
		}
		for _, payload := range e2Blocked {
			var stderr bytes.Buffer
			if rc := RunExec(context.Background(), nil, strings.NewReader(payload), nil, &stderr); rc != 2 || !strings.Contains(stderr.String(), "BLOCKED [EXEC-01]") {
				t.Fatalf("E2 should block %s: rc=%d stderr=%q", payload, rc, stderr.String())
			}
		}

		e2Allowed := []string{"./scripts/lint/run.sh scripts", "make lint TARGET=scripts", "make test"}
		for _, cmd := range e2Allowed {
			var stderr bytes.Buffer
			if rc := RunExec(context.Background(), nil, strings.NewReader(bashHookJSON(cmd, "w-1", "")), nil, &stderr); rc != 0 {
				t.Fatalf("E2 should allow %q: rc=%d stderr=%q", cmd, rc, stderr.String())
			}
		}

		e3Blocked := []string{"go test ./... &", "cmd1 & cmd2", "go test ./... 2>&1 &", "nohup go test ./...", "bash -c 'sleep 1 &'"}
		for _, cmd := range e3Blocked {
			var stderr bytes.Buffer
			if rc := RunExec(context.Background(), nil, strings.NewReader(bashHookJSON(cmd, "w-1", "")), nil, &stderr); rc != 2 || !strings.Contains(stderr.String(), "BLOCKED [EXEC-01]") {
				t.Fatalf("E3 should block %q: rc=%d stderr=%q", cmd, rc, stderr.String())
			}
		}

		e3Allowed := []string{
			"echo a && echo b", "echo a || echo b", "go test ./... 2>&1",
			"echo err 1>&2", "echo err >&2", "go test ./... &> out.log",
			"go test ./... |& cat", `curl "https://example.com?a=1&b=2"`,
			"echo 'a & b'", `echo a \& b`,
		}
		for _, cmd := range e3Allowed {
			var stderr bytes.Buffer
			if rc := RunExec(context.Background(), nil, strings.NewReader(bashHookJSON(cmd, "w-1", "")), nil, &stderr); rc != 0 {
				t.Fatalf("E3 should allow %q: rc=%d stderr=%q", cmd, rc, stderr.String())
			}
		}

		for _, bad := range []string{"", "{bad", `{"tool_input":{"command":"echo hi"}}`, `{"tool_name":"Bash"}`, `{"tool_name":"Bash","tool_input":{}}`} {
			var stderr bytes.Buffer
			if rc := RunExec(context.Background(), nil, strings.NewReader(bad), nil, &stderr); rc != 2 {
				t.Fatalf("fail-closed %q rc=%d", bad, rc)
			}
		}
		var stderr bytes.Buffer
		if rc := RunExec(context.Background(), nil, strings.NewReader(`{"tool_name":"Read","tool_input":{"file_path":"foo.go"}}`), nil, &stderr); rc != 0 {
			t.Fatalf("non-shell Read rc=%d", rc)
		}
	})
}

func TestDenyUnsafeCommandsShellHook(t *testing.T) {
	t.Parallel()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := ""
	for {
		if _, err := os.Stat(filepath.Join(dir, ".claude", "hooks", "deny-unsafe-commands.sh")); err == nil {
			repoRoot = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
	hookPath := filepath.Join(repoRoot, ".claude", "hooks", "deny-unsafe-commands.sh")
	st, err := os.Stat(hookPath)
	if err != nil || st.Mode()&0o111 == 0 {
		t.Fatalf("%s must exist and be executable", hookPath)
	}
	raw, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "python") || !strings.Contains(string(raw), "alt-harness") {
		t.Fatalf("deny-unsafe-commands.sh must exec alt-harness without python: %s", string(raw))
	}

	cmdBlock := exec.Command("bash", hookPath)
	cmdBlock.Dir = repoRoot
	cmdBlock.Stdin = strings.NewReader(bashHookJSON("golangci-lint run --fix ./...", "", ""))
	var outB, errB bytes.Buffer
	cmdBlock.Stdout = &outB
	cmdBlock.Stderr = &errB
	err = cmdBlock.Run()
	if err == nil || cmdBlock.ProcessState.ExitCode() != 2 || !strings.Contains(errB.String(), "BLOCKED [EXEC-01]") {
		t.Fatalf("expected exit 2 BLOCKED [EXEC-01], err=%v stderr=%q", err, errB.String())
	}

	cmdOK := exec.Command("bash", hookPath)
	cmdOK.Dir = repoRoot
	cmdOK.Stdin = strings.NewReader(bashHookJSON("golangci-lint run ./...", "", ""))
	if out, err := cmdOK.CombinedOutput(); err != nil {
		t.Fatalf("clean command should pass, err=%v out=%s", err, string(out))
	}
}

func TestScopeAndCwdComprehensiveParallel(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repo, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	runGitInTest(t, repo, "init", "-b", "feat/test-branch")
	runGitInTest(t, repo, "config", "user.email", "test@example.com")
	runGitInTest(t, repo, "config", "user.name", "Test")

	outside := filepath.Join(tmp, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}

	commitFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(full), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), fsperm.File); err != nil {
			t.Fatal(err)
		}
		runGitInTest(t, repo, "add", "--", rel)
		runGitInTest(t, repo, "commit", "-m", "add "+rel)
	}
	commitFile("rag-orchestrator/openapi/server.gen.go", "// Code generated by oapi-codegen v2.7.0 DO NOT EDIT.\npackage openapi\n")
	commitFile("scripts/allowed.go", "package scripts\n")
	commitFile("stopped-using-k8s/deploy.yaml", "kind: Deployment\n")
	commitFile("scripts/harness/corpus/C-16/after.go", "// Code generated by oapi-codegen DO NOT EDIT.\npackage openapi\n")
	commitFile("alt-backend/app/driver/db.go", "package driver\n")

	ctx := context.Background()
	for _, tool := range []string{"Edit", "Write", "apply_patch"} {
		var stderr bytes.Buffer
		rc := RunScope(ctx, nil, strings.NewReader(editHookWithCwdJSON("rag-orchestrator/openapi/server.gen.go", tool, repo)), nil, &stderr)
		if rc != 2 || !strings.Contains(stderr.String(), "DO NOT EDIT") {
			t.Fatalf("tool=%s expected DO NOT EDIT block, rc=%d stderr=%q", tool, rc, stderr.String())
		}
	}

	// Stripping header on disk still blocked via HEAD check.
	genDisk := filepath.Join(repo, "rag-orchestrator/openapi/server.gen.go")
	if err := os.WriteFile(genDisk, []byte("package openapi\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if rc := RunScope(ctx, nil, strings.NewReader(editHookWithCwdJSON("rag-orchestrator/openapi/server.gen.go", "Edit", repo)), nil, &stderr); rc != 2 || !strings.Contains(stderr.String(), "DO NOT EDIT") {
		t.Fatalf("stripped header should block via HEAD, rc=%d stderr=%q", rc, stderr.String())
	}

	// Corpus fixture allowed.
	stderr.Reset()
	if rc := RunScope(ctx, nil, strings.NewReader(editHookWithCwdJSON("scripts/harness/corpus/C-16/after.go", "Edit", repo)), nil, &stderr); rc != 0 {
		t.Fatalf("corpus fixture should allow, rc=%d stderr=%q", rc, stderr.String())
	}

	// stopped-using-k8s relative to subdir cwd blocked.
	stderr.Reset()
	if rc := RunScope(ctx, nil, strings.NewReader(editHookWithCwdJSON("deploy.yaml", "Edit", filepath.Join(repo, "stopped-using-k8s"))), nil, &stderr); rc != 2 || !strings.Contains(stderr.String(), "stopped-using-k8s") {
		t.Fatalf("subdir k8s should block, rc=%d stderr=%q", rc, stderr.String())
	}

	// Outside repo allowed without active packet.
	stderr.Reset()
	if rc := RunScope(ctx, nil, strings.NewReader(editHookWithCwdJSON(outside, "Edit", repo)), nil, &stderr); rc != 0 {
		t.Fatalf("outside repo without packet should allow, rc=%d stderr=%q", rc, stderr.String())
	}

	// Symlink escaping repo root blocked.
	escLink := filepath.Join(repo, "scripts", "escape_link.txt")
	if err := os.Symlink(outside, escLink); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if rc := RunScope(ctx, nil, strings.NewReader(editHookWithCwdJSON("scripts/escape_link.txt", "Edit", repo)), nil, &stderr); rc != 2 {
		t.Fatalf("escaping symlink should block, rc=%d stderr=%q", rc, stderr.String())
	}

	// Activate packet and test scope boundaries + opt-in.
	pktPath := filepath.Join(repo, "pkt.md")
	if err := os.WriteFile(pktPath, []byte(samplePacketWithScope("PKT-SCOPE", []string{"scripts/**", "alt-backend/app/**"}, []string{"scripts/forbidden.go"}, false, true)), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if rc := RunHarness(ctx, []string{"activate-packet", "--repo-root", repo, pktPath}, nil, &bytes.Buffer{}, &bytes.Buffer{}); rc != 0 {
		t.Fatalf("activate-packet failed: %d", rc)
	}

	stderr.Reset()
	if rc := RunScope(ctx, nil, strings.NewReader(editHookWithCwdJSON("driver/db.go", "Edit", filepath.Join(repo, "alt-backend", "app"))), nil, &stderr); rc != 0 {
		t.Fatalf("in-scope subdir relative should allow, rc=%d stderr=%q", rc, stderr.String())
	}
	stderr.Reset()
	if rc := RunScope(ctx, nil, strings.NewReader(editHookWithCwdJSON("../../other/outside.go", "Edit", filepath.Join(repo, "alt-backend", "app"))), nil, &stderr); rc != 2 || !strings.Contains(stderr.String(), "allowed_paths") {
		t.Fatalf("out-of-scope subdir relative should block, rc=%d stderr=%q", rc, stderr.String())
	}
	stderr.Reset()
	if rc := RunScope(ctx, nil, strings.NewReader(editHookWithCwdJSON("scripts/forbidden.go", "Edit", repo)), nil, &stderr); rc != 2 || !strings.Contains(stderr.String(), "forbidden_paths") {
		t.Fatalf("forbidden_paths should block, rc=%d stderr=%q", rc, stderr.String())
	}
	stderr.Reset()
	if rc := RunScope(ctx, nil, strings.NewReader(editHookWithCwdJSON(outside, "Edit", repo)), nil, &stderr); rc != 2 {
		t.Fatalf("outside repo with active packet should block, rc=%d", rc)
	}

	// Opt-in packet for generated code + stopped-using-k8s.
	optPath := filepath.Join(repo, "pkt-optin.md")
	if err := os.WriteFile(optPath, []byte(samplePacketWithScope("PKT-OPTIN", []string{"rag-orchestrator/openapi/**", "stopped-using-k8s/**"}, []string{"scripts/lint/run.sh"}, true, true)), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if rc := RunHarness(ctx, []string{"activate-packet", "--repo-root", repo, optPath}, nil, &bytes.Buffer{}, &bytes.Buffer{}); rc != 0 {
		t.Fatalf("activate opt-in failed: %d", rc)
	}
	stderr.Reset()
	if rc := RunScope(ctx, nil, strings.NewReader(editHookWithCwdJSON("rag-orchestrator/openapi/server.gen.go", "Edit", repo)), nil, &stderr); rc != 0 {
		t.Fatalf("opt-in generated should allow, rc=%d stderr=%q", rc, stderr.String())
	}
	stderr.Reset()
	if rc := RunScope(ctx, nil, strings.NewReader(editHookWithCwdJSON("stopped-using-k8s/deploy.yaml", "Edit", repo)), nil, &stderr); rc != 0 {
		t.Fatalf("opt-in k8s should allow, rc=%d stderr=%q", rc, stderr.String())
	}
}

func TestHarnessRunFullLifecycleParallel(t *testing.T) {
	t.Parallel()
	repo := initTestGitRepo(t)
	ctx := context.Background()

	runH := func(args []string, stdin string) (int, string, string) {
		var out, errB bytes.Buffer
		var r io.Reader
		if stdin != "" {
			r = strings.NewReader(stdin)
		}
		code := RunHarness(ctx, append(args, "--repo-root", repo), r, &out, &errB)
		return code, out.String(), errB.String()
	}

	rc, _, errText := runH([]string{"init", "--policy", "block-new", "--goal", "Harden harness", "--completion", "./scripts/lint/run.sh harness exits 0"}, "")
	if rc != 0 {
		t.Fatalf("init rc=%d err=%s", rc, errText)
	}
	runDir := filepath.Join(repo, ".harness", "runs", "feat-test-branch")
	for _, sub := range []string{"packets", "handoffs", "verify", "evidence"} {
		if st, err := os.Stat(filepath.Join(runDir, sub)); err != nil || !st.IsDir() {
			t.Fatalf("missing subdir %s", sub)
		}
	}
	tsvRaw, err := os.ReadFile(filepath.Join(runDir, "decisions.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	tsvLines := strings.Split(strings.TrimSpace(string(tsvRaw)), "\n")
	if len(tsvLines) != 2 || tsvLines[0] != "ts\tphase\tdecision\twhy\tevidence\tresult" || !strings.Contains(tsvLines[1], "block-new") {
		t.Fatalf("unexpected decisions.tsv: %q", string(tsvRaw))
	}

	// log rejects prose evidence and tabs; accepts cmd: pointer.
	if rc, _, _ := runH([]string{"log", "--phase", "review", "--decision", "approve", "--why", "ok", "--evidence", "looks good", "--result", "info"}, ""); rc != 2 {
		t.Fatalf("prose evidence should exit 2, got %d", rc)
	}
	if rc, _, _ := runH([]string{"log", "--phase", "review", "--decision", "bad\tdec", "--why", "ok", "--evidence", "commit:abc1234", "--result", "info"}, ""); rc != 2 {
		t.Fatalf("tab field should exit 2, got %d", rc)
	}
	if rc, _, errText := runH([]string{"log", "--phase", "verify", "--decision", "green", "--why", "passed", "--evidence", "cmd:./scripts/lint/run.sh scripts", "--result", "info"}, ""); rc != 0 {
		t.Fatalf("cmd: pointer should pass, rc=%d err=%s", rc, errText)
	}

	// activate-packet rejects unmeasured & stale command_sha256.
	unmPath := filepath.Join(repo, "pkt-unm.md")
	_ = os.WriteFile(unmPath, []byte(samplePacketWithScope("PKT-UNM", []string{"scripts/**"}, []string{"scripts/f.go"}, false, false)), fsperm.File)
	if rc, _, errText := runH([]string{"activate-packet", unmPath}, ""); rc == 0 || !strings.Contains(errText, "UNMEASURED_VERIFICATION_COMMAND") {
		t.Fatalf("unmeasured should fail, rc=%d err=%s", rc, errText)
	}

	stalePath := filepath.Join(repo, "pkt-stale.md")
	staleText := strings.Replace(samplePacketWithScope("PKT-STALE", []string{"scripts/**"}, []string{"scripts/f.go"}, false, true), `./scripts/lint/run.sh scripts`, `./scripts/lint/run.sh harness`, 1)
	_ = os.WriteFile(stalePath, []byte(staleText), fsperm.File)
	if rc, _, errText := runH([]string{"activate-packet", stalePath}, ""); rc == 0 || !strings.Contains(errText, "STALE_MEASURED_COMMAND") {
		t.Fatalf("stale sha should fail, rc=%d err=%s", rc, errText)
	}

	// Valid activate-packet + record-handoff + subagent-stop.
	validPkt := filepath.Join(repo, "pkt-loop.md")
	_ = os.WriteFile(validPkt, []byte(samplePacketWithScope("PKT-LOOP-01", []string{"scripts/**"}, []string{"scripts/f.go"}, false, true)), fsperm.File)
	if rc, _, errText := runH([]string{"activate-packet", validPkt}, ""); rc != 0 {
		t.Fatalf("activate valid rc=%d err=%s", rc, errText)
	}
	hoPath := filepath.Join(repo, "handoff-loop.md")
	_ = os.WriteFile(hoPath, []byte(sampleHandoffMD("PKT-LOOP-01")), fsperm.File)
	if rc, _, errText := runH([]string{"record-handoff", hoPath, "--result", "adopted"}, ""); rc != 0 {
		t.Fatalf("record-handoff rc=%d err=%s", rc, errText)
	}
	activeRaw, _ := os.ReadFile(filepath.Join(runDir, "active-packet.json"))
	if !strings.Contains(string(activeRaw), `"active": false`) && !strings.Contains(string(activeRaw), `"active":false`) {
		t.Fatalf("expected deactivated active-packet.json, got %s", string(activeRaw))
	}

	subPayload := `{"hook_event_name":"SubagentStop","session_id":"s1","agent_id":"worker-9","agent_transcript_path":"/tmp/t.jsonl","last_assistant_message":"ok"}`
	if rc, _, errText := runH([]string{"subagent-stop"}, subPayload); rc != 0 {
		t.Fatalf("subagent-stop rc=%d err=%s", rc, errText)
	}
	if rc, _, _ := runH([]string{"subagent-stop"}, "{bad"); rc != 0 {
		t.Fatalf("subagent-stop bad json should exit 0, got %d", rc)
	}

	// Parallelism ratchet edge cases (C-01 / LOOP-06).
	parCases := []struct {
		name     string
		outcomes []string
		base     int
		want     int
	}{
		{"empty holds base", nil, 2, 2},
		{"1 adopted holds base", []string{"adopted"}, 2, 2},
		{"9 adopted holds base", strings.Split(strings.Repeat("adopted,", 8)+"adopted", ","), 2, 2},
		{"10 adopted scales +2", strings.Split(strings.Repeat("adopted,", 9)+"adopted", ","), 2, 4},
		{"9 adopted + 1 reverted scales +2", append(strings.Split(strings.Repeat("adopted,", 8)+"adopted", ","), "reverted"), 2, 4},
		{"caps at 8", strings.Split(strings.Repeat("adopted,", 9)+"adopted", ","), 7, 8},
		{"3 reverted scales down immediately", []string{"reverted", "reverted", "reverted"}, 2, 1},
		{"2 mismatch scales down from 4", append([]string{"mismatch", "mismatch"}, strings.Split(strings.Repeat("adopted,", 7)+"adopted", ",")...), 4, 2},
		{"2 reverted + 0 mismatch holds 4", append([]string{"reverted", "reverted"}, strings.Split(strings.Repeat("adopted,", 7)+"adopted", ",")...), 4, 4},
	}
	for _, tc := range parCases {
		if got := ComputeRecommendedParallelism(tc.outcomes, tc.base).Recommended; got != tc.want {
			t.Fatalf("%s: got %d want %d", tc.name, got, tc.want)
		}
	}
}

func TestAgyPreflightRunnerInProcessAndBinary(t *testing.T) {
	// Sequential because it touches package-level PreflightRunner seam.
	repo := initTestGitRepo(t)
	origRunner := PreflightRunner
	origPF := agyRunPreflightFn
	origSC := agyRunSelfcheckFn
	t.Cleanup(func() {
		PreflightRunner = origRunner
		agyRunPreflightFn = origPF
		agyRunSelfcheckFn = origSC
	})

	agyRunPreflightFn = defaultAgyRunPreflight
	agyRunSelfcheckFn = func(_ context.Context, _ string) string { return "agent_harness_selfcheck: OK" }

	// 1. In-process PreflightRunner callback invoked with --hook and JSON stdin.
	var capturedArgs []string
	var capturedStdin string
	PreflightRunner = func(_ context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
		capturedArgs = append([]string(nil), args...)
		b, _ := io.ReadAll(stdin)
		capturedStdin = string(b)
		_, _ = io.WriteString(stderr, "ADR PREFLIGHT: held in-process")
		return 2
	}
	var out bytes.Buffer
	rc := RunAgyHook(context.Background(), []string{"pre-edit"}, strings.NewReader(`{"conversationId":"c1","cwd":"`+repo+`","toolCall":{"id":"e1","name":"write_file","args":{"Path":"scripts/foo.go","Content":"package scripts\n"}}}`), &out, &bytes.Buffer{})
	if rc != 0 || !strings.Contains(out.String(), `"deny"`) || !strings.Contains(out.String(), "held in-process") {
		t.Fatalf("in-process PreflightRunner failed: rc=%d out=%s", rc, out.String())
	}
	if len(capturedArgs) != 1 || capturedArgs[0] != "--hook" || !strings.Contains(capturedStdin, "scripts/foo.go") {
		t.Fatalf("unexpected PreflightRunner args=%v stdin=%s", capturedArgs, capturedStdin)
	}

	// 2. Fallback to repo alt-harness launcher binary when PreflightRunner is nil (zero python3!).
	PreflightRunner = nil
	launcher := filepath.Join(repo, "scripts", "altharness", "alt-harness")
	if err := os.MkdirAll(filepath.Dir(launcher), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	stub := "#!/usr/bin/env bash\n" +
		"if [ \"$1\" = \"preflight\" ] && [ \"$2\" = \"--hook\" ]; then\n" +
		"  echo 'adr_preflight_enabled: OK (go-stub)'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 1\n"
	if err := fsperm.WriteExecutable(launcher, []byte(stub)); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	rc = RunAgyHook(context.Background(), []string{"session-start"}, strings.NewReader(`{"conversationId":"c2","cwd":"`+repo+`"}`), &out, &bytes.Buffer{})
	if rc != 0 || !strings.Contains(out.String(), "adr_preflight_enabled: OK (go-stub)") {
		t.Fatalf("launcher fallback failed: rc=%d out=%s", rc, out.String())
	}
}
