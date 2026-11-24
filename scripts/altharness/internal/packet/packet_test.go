package packet

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckTemplates(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	code := Run(ctx, []string{"check-templates"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("check-templates exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "OK:") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "task-packet.md.tmpl") || !strings.Contains(stdout.String(), "task-handoff.md.tmpl") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestUnmeasuredPacketRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "packet.json")
	body := `{
  "packet_version": "1.0",
  "packet_id": "UNMEAS-1",
  "scope": {
    "target": "scripts",
    "allowed_paths": ["scripts/agent-harness/**"],
    "max_files": 10,
    "max_functions": 3,
    "allowed_rules": ["S101"],
    "stop_condition": "stop when the rule is fixed"
  },
  "execution_policy": {
    "foreground_only": true,
    "forbid_repo_wide_test_build": true,
    "forbid_bulk_autofix": true,
    "verification_command": "./scripts/lint/run.sh scripts",
    "max_command_budget_s": 180
  },
  "handoff_requirements": {
    "require_modified_files_and_lines": true,
    "require_runtime_behavior_impact": true,
    "require_change_rationale": true
  }
}`
	if err := os.WriteFile(path, []byte(body), fsperm.File); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"validate-packet", path}, &stdout, &stderr)
	if code != exitInvalid {
		t.Fatalf("exit %d stdout %s stderr %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "UNMEASURED_VERIFICATION_COMMAND") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestBackgroundTestCommandRejected(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"verify-sequence",
		"--base-commit", "HEAD",
		"--test-command", "echo hi &",
		"--repo-root", dir,
	}, &stdout, &stderr)
	if code != exitInvalid {
		t.Fatalf("exit %d stdout %s stderr %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "FORBIDDEN_BACKGROUND_EXECUTION") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestBulkAutofixRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "packet.json")
	body := strings.Replace(unmeasuredPacket, "./scripts/lint/run.sh scripts", "golangci-lint run --fix", 1)
	body = strings.Replace(body, `"packet_id": "UNMEAS-1"`, `"packet_id": "AUTOFIX-1"`, 1)
	if err := os.WriteFile(path, []byte(body), fsperm.File); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"validate-packet", path}, &stdout, &stderr)
	if code != exitInvalid {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "FORBIDDEN_AUTOFIX_COMMAND") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

const unmeasuredPacket = `{
  "packet_version": "1.0",
  "packet_id": "UNMEAS-1",
  "scope": {
    "target": "scripts",
    "allowed_paths": ["scripts/agent-harness/**"],
    "max_files": 10,
    "max_functions": 3,
    "allowed_rules": ["S101"],
    "stop_condition": "stop when the rule is fixed"
  },
  "execution_policy": {
    "foreground_only": true,
    "forbid_repo_wide_test_build": true,
    "forbid_bulk_autofix": true,
    "verification_command": "./scripts/lint/run.sh scripts",
    "max_command_budget_s": 180
  },
  "handoff_requirements": {
    "require_modified_files_and_lines": true,
    "require_runtime_behavior_impact": true,
    "require_change_rationale": true
  }
}`

func TestPacketIDTraversalRejected(t *testing.T) {
	if _, ok := safeEvidencePath(t.TempDir(), "branch", "../../escaped_outside"); ok {
		t.Fatal("packet id with traversal was accepted")
	}
	runs := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(runs, "branch")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, ok := safeEvidencePath(runs, "branch", "SAFE-ID"); ok {
		t.Fatal("symlink runs component was accepted")
	}
	repo := initGitRepo(t)
	pkt := filepath.Join(repo, "evil-packet.json")
	ho := filepath.Join(repo, "evil-handoff.json")
	if err := os.WriteFile(pkt, []byte(strings.Replace(measuredPacket, "SPEC-04-SAMPLE", "../../escaped_outside", 1)), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ho, []byte(strings.Replace(handoffDoc, "SPEC-04-SAMPLE", "../../escaped_outside", 1)), fsperm.File); err != nil {
		t.Fatal(err)
	}
	runsDir := filepath.Join(repo, "runs")
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"verify-handoff", ho,
		"--packet", pkt,
		"--repo-root", repo,
		"--base-commit", gitHead(t, repo),
		"--runs-dir", runsDir,
		"--no-rerun",
	}, &stdout, &stderr)
	if code != exitInvalid {
		t.Fatalf("exit %d stdout %s stderr %s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, "..", "escaped_outside.json")); err == nil {
		t.Fatal("wrote escaped_outside.json")
	}
	matches, _ := filepath.Glob(filepath.Join(runsDir, "**", "*.json"))
	if len(matches) != 0 {
		t.Fatalf("runs dir has json: %v", matches)
	}
	if strings.Contains(stderr.String(), "UNSAFE_EVIDENCE_PATH") || strings.Contains(stderr.String(), "SCHEMA_PATTERN") || strings.Contains(stderr.String(), "INVALID_PACKET_ID") {
		return
	}
	t.Fatalf("stderr missing traversal rejection: %s", stderr.String())
}

func TestContextCancelKillsChild(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "packet.json")
	body := strings.Replace(unmeasuredPacket, "./scripts/lint/run.sh scripts", "sleep 30", 1)
	if err := os.WriteFile(path, []byte(body), fsperm.File); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		var stdout, stderr bytes.Buffer
		done <- Run(ctx, []string{"measure", path, "--repo-root", dir}, &stdout, &stderr)
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case code := <-done:
		if code == exitOK {
			t.Fatal("cancelled measure returned 0")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("context cancellation did not kill the child process")
	}
}

const measuredPacket = `{
  "packet_version": "1.0",
  "packet_id": "SPEC-04-SAMPLE",
  "base_commit": "0123456",
  "scope": {
    "target": "scripts",
    "allowed_paths": ["scripts/**"],
    "max_files": 10,
    "max_functions": 3,
    "allowed_rules": ["S101"],
    "stop_condition": "stop when the rule is fixed"
  },
  "execution_policy": {
    "foreground_only": true,
    "forbid_repo_wide_test_build": true,
    "forbid_bulk_autofix": true,
    "verification_command": "./scripts/lint/run.sh scripts",
    "max_command_budget_s": 180,
    "measured": {
      "duration_s": 1.25,
      "exit_code": 0,
      "at_commit": "0123456",
      "command_sha256": "19a6f68d9a7983303fca6f0c924c02aaabc34fea3a0a84ac28bbf7a6eaa889e9"
    }
  },
  "handoff_requirements": {
    "require_modified_files_and_lines": true,
    "require_runtime_behavior_impact": true,
    "require_change_rationale": true
  }
}`

const handoffDoc = `{
  "packet_id": "SPEC-04-SAMPLE",
  "status": "completed",
  "modified_files": [],
  "modified_functions_count": 0,
  "runtime_behavior_impact": {
    "has_change": false,
    "checked_dimensions": ["timeout"],
    "summary": "変更なし"
  },
  "rationale": "変更なし",
  "verification": {"command": "./scripts/lint/run.sh scripts", "exit_code": 0}
}`

func TestNonexistentCandidateTest(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "tests"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tests", "test_real.py"), []byte("def test_real():\n    assert True\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ho := filepath.Join(dir, "handoff.json")
	body := `{
  "packet_id": "CAND-1",
  "status": "completed",
  "modified_files": [],
  "modified_functions_count": 0,
  "runtime_behavior_impact": {
    "has_change": true,
    "checked_dimensions": ["control_flow"],
    "summary": "intentional behavior change covered by a named test",
    "candidate_responses": [
      {"disposition": "intended", "test_name": "test_does_not_exist"}
    ]
  },
  "rationale": "the change is covered by the named test",
  "verification": {"command": "python3 -c 'raise SystemExit(0)'", "exit_code": 0}
}`
	if err := os.WriteFile(ho, []byte(body), fsperm.File); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"validate-handoff", ho, "--repo-root", repo}, &stdout, &stderr)
	if code != exitInvalid {
		t.Fatalf("exit %d stdout %s stderr %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "NONEXISTENT_CANDIDATE_TEST") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hi\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	git("add", "README")
	git("commit", "-q", "-m", "init")
	return dir
}

func TestGoFunctionCountMasksCommentsAndReceivers(t *testing.T) {
	before := "package p\nfunc (s *Server) Foo() int { return 1 } // keep\n"
	afterComment := "package p\nfunc (s *Server) Foo() int { return 1 } // changed comment\n"
	n, err := countModifiedFunctionsInFile("p.go", before, afterComment)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("comment-only change counted %d functions", n)
	}
	afterBody := "package p\nfunc (s *Server) Foo() int { return 2 } // keep\n"
	n, err = countModifiedFunctionsInFile("p.go", before, afterBody)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("body change counted %d functions", n)
	}
	bodies := extractGoFuncBodies("package p\nfunc (s *Server) Foo() int { return 1 }\nfunc Foo() {}\n")
	if _, ok := bodies["(Server).Foo"]; !ok {
		t.Fatalf("receiver was not normalized: %#v", bodies)
	}
	if _, ok := bodies["Foo"]; !ok {
		t.Fatalf("plain func missing: %#v", bodies)
	}
	quoted := "package p\nfunc Foo() {\n\ts := \"func Bar() { return 1 }\"\n\treturn\n}\n"
	quotedAfter := "package p\nfunc Foo() {\n\ts := \"func Bar() { return 2 }\"\n\treturn\n}\n"
	n, err = countModifiedFunctionsInFile("p.go", quoted, quotedAfter)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("string change counted %d functions (want only Foo)", n)
	}
}

func gitHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
