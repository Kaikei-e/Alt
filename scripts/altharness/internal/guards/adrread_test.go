package guards

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func adrReadBashPayload(t *testing.T, cmd any, cwd, sessionID, agentID string) string {
	t.Helper()
	payload := map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": cmd},
	}
	if cwd != "" {
		payload["cwd"] = cwd
	}
	if sessionID != "" {
		payload["session_id"] = sessionID
	}
	if agentID != "" {
		payload["agent_id"] = agentID
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestDetectAdrReadTargets(t *testing.T) {
	t.Parallel()
	detectCases := []struct {
		name     string
		cmd      string
		cwd      string
		wantIDs  []string
		wantGlob bool
	}{
		{"cat relative", "cat docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"cat dot-relative", "cat ./docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"cat absolute", "cat /home/dev/Alt/docs/ADR/000979.md", "/tmp", []string{"000979"}, false},
		{"cat worktree", "cat .claude/worktrees/w1/docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"cat multiple IDs", "cat docs/ADR/000980.md docs/ADR/000979.md", "/repo", []string{"000979", "000980"}, false},
		{"head -n", "head -n 40 docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"head attached -n", "head -n40 docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"head numeric flag", "head -40 docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"tail -n +10", "tail -n +10 docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"less +/pattern", "less +/Decision docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"grep basic", "grep Decision docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"egrep -n", "egrep -n 'foo|bar' docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"fgrep -C", "fgrep -C 2 foo docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"grep -e explicit pattern", "grep -e Decision docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"rg basic", "rg Decision docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"rg with value flags", "rg -n --color never -C 2 Decision docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"rg --files dir", "rg --files docs/ADR", "/repo", nil, true},
		{"sed script then file", "sed -n '1,80p' docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"sed -ne script then file", "sed -ne '1,80p' docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"awk program then file", "awk '/Decision/,/Consequences/' docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"gawk with flags and var", "gawk -F: -v x=1 '{print $1}' foo=2 docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"mawk -f program", "mawk -f prog.awk docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"nawk basic", "nawk 'NR<=20' docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"dir search rg", "rg 'fixed skeleton' docs/ADR", "/repo", nil, true},
		{"dir search rg trailing slash", "rg 'fixed skeleton' docs/ADR/", "/repo", nil, true},
		{"dir search grep -rn", "grep -rn 'fixed skeleton' docs/ADR/", "/repo", nil, true},
		{"glob star", "grep -l foo docs/ADR/0009*.md", "/repo", nil, true},
		{"glob question", "cat docs/ADR/0009??.md", "/repo", nil, true},
		{"glob bracket", "head -n 5 docs/ADR/0009[0-9]*.md", "/repo", nil, true},
		{"cd then glob", "cd docs/ADR && grep -l foo 0009*.md", "/repo", nil, true},
		{"cd then rg implicit dot", "cd docs/ADR && rg 'fixed skeleton'", "/repo", nil, true},
		{"cd then recursive grep implicit dot", "cd docs/ADR && grep -rn 'fixed skeleton'", "/repo", nil, true},
		{"cd then specific file", "cd docs/ADR && cat 000979.md", "/repo", []string{"000979"}, false},
		{"pushd then rg", "pushd docs/ADR && rg Decision", "/repo", nil, true},
		{"cwd already in docs/ADR with rg", "rg 'fixed skeleton'", "/repo/docs/ADR", nil, true},
		{"cwd already in docs/ADR with cat", "cat 000979.md", "/repo/docs/ADR", []string{"000979"}, false},
		{"env + wrappers", "LC_ALL=C time command env -u FOO sudo timeout 10 nice -n 5 cat docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"xargs grep", "printf '' | xargs grep -n Decision docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"bash -c nested", `bash -euo pipefail -c 'sed -n "1,20p" docs/ADR/000979.md'`, "/repo", []string{"000979"}, false},
		{"sh -c nested", `sh -c 'rg Decision docs/ADR/'`, "/repo", nil, true},
		{"pipeline left side", "cat docs/ADR/000979.md | head -n 5", "/repo", []string{"000979"}, false},
		{"semicolon second segment", "echo start; head -n 20 docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"command substitution dollar-paren", `echo "$(head -n 5 docs/ADR/000979.md)"`, "/repo", []string{"000979"}, false},
		{"command substitution after cd", `cd docs/ADR && echo "$(head -n 5 000979.md)"`, "/repo", []string{"000979"}, false},
		{"command substitution backticks", "echo `cat docs/ADR/000979.md`", "/repo", []string{"000979"}, false},
		{"input redirect spaced", "cat < docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"input redirect attached", "grep Decision <docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"input redirect fd 0", "sed -n '1,10p' 0<docs/ADR/000979.md", "/repo", []string{"000979"}, false},
		{"prefix input redirect", "< docs/ADR/000979.md head -n 10", "/repo", []string{"000979"}, false},
	}

	for _, tc := range detectCases {
		tc := tc
		t.Run("detect/"+tc.name, func(t *testing.T) {
			t.Parallel()
			hit, err := detectAdrReadCommand(tc.cmd, tc.cwd, 0)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !hit.Matched {
				t.Fatalf("expected match for %q (cwd=%q), got no match", tc.cmd, tc.cwd)
			}
			if strings.Join(hit.IDs, ",") != strings.Join(tc.wantIDs, ",") {
				t.Fatalf("IDs=%v want %v for %q", hit.IDs, tc.wantIDs, tc.cmd)
			}
			if tc.wantGlob && len(hit.IDs) != 0 {
				t.Fatalf("expected directory/glob hit with no specific IDs, got %v", hit.IDs)
			}
		})
	}

	nonDetectCases := []struct {
		name string
		cmd  string
		cwd  string
	}{
		{"docdag context", "docdag context 000979", "/repo"},
		{"docdag context section", "docdag context 000979 --section Consequences", "/repo"},
		{"docdag validate touching", "docdag validate --touching docs/ADR/000979.md", "/repo"},
		{"docdag query", "docdag query --binding --fields id,title,status", "/repo"},
		{"git diff", "git diff docs/ADR/000979.md", "/repo"},
		{"git log", "git log -p docs/ADR/000979.md", "/repo"},
		{"git show", "git show HEAD:docs/ADR/000979.md", "/repo"},
		{"git add", "git add docs/ADR/000979.md", "/repo"},
		{"git mv", "git mv docs/ADR/000979.md docs/ADR/000980.md", "/repo"},
		{"template.md", "cat docs/ADR/template.md", "/repo"},
		{"README.md", "cat docs/ADR/README.md", "/repo"},
		{"index.md", "head -n 20 docs/ADR/index.md", "/repo"},
		{"pattern arg in grep", `grep -n "docs/ADR/000979.md" scripts/foo.go`, "/repo"},
		{"pattern arg in rg", `rg "docs/ADR/000979.md" scripts/`, "/repo"},
		{"pattern -e arg in grep", `grep -e "docs/ADR/000979.md" scripts/foo.go`, "/repo"},
		{"script arg in sed", `sed 's|docs/ADR/000979.md|x|' scripts/foo.go`, "/repo"},
		{"program arg in awk", `awk '/docs\/ADR\/000979\.md/' scripts/foo.go`, "/repo"},
		{"non-ADR grep", "grep -rn foo scripts/", "/repo"},
		{"non-ADR sed", "sed -n '1,5p' docs/runbooks/x.md", "/repo"},
		{"non-ADR cat", "cat docs/plan/knowledge-trail.md", "/repo"},
		{"cd out of docs/ADR", "cd docs/ADR && cd .. && grep -rn foo runbooks/", "/repo"},
		{"pushd then popd out of docs/ADR", "pushd docs/ADR && popd && rg foo", "/repo"},
		{"single-quoted command substitution", `echo '$(cat docs/ADR/000979.md)'`, "/repo"},
		{"less -p pattern arg", "less -p docs/ADR/000979.md scripts/foo.go", "/repo"},
		{"piped rg after docdag in docs/ADR", "cd docs/ADR && docdag context 000979 | rg Decision", "/repo"},
		{"non-recursive grep in pipe", "cd docs/ADR && docdag context 000979 | grep Decision", "/repo"},
		{"quoted redirect token in pattern", `grep '<docs/ADR/000979.md>' scripts/foo.go`, "/repo"},
		{"echo literal", `echo "cat docs/ADR/000979.md"`, "/repo"},
		{"empty command", "   ", "/repo"},
	}

	for _, tc := range nonDetectCases {
		tc := tc
		t.Run("ignore/"+tc.name, func(t *testing.T) {
			t.Parallel()
			hit, err := detectAdrReadCommand(tc.cmd, tc.cwd, 0)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if hit.Matched {
				t.Fatalf("expected NO match for %q (cwd=%q), got %+v", tc.cmd, tc.cwd, hit)
			}
		})
	}
}

func TestRunAdrReadFirstHoldReceiptGuidanceAndIsolation(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("ALT_ADR_PREFLIGHT_STATE_DIR", stateDir)
	t.Setenv("ALT_ADR_PREFLIGHT", "")

	ctx := context.Background()

	// 1. Specific ADR ID: 1st call holds (exit 2), writes receipt, emits ID-specific guidance.
	payload1 := adrReadBashPayload(t, "sed -n '1,80p' docs/ADR/000979.md", "/repo", "sess-a", "")
	var stdout, stderr bytes.Buffer
	rc := RunAdrRead(ctx, nil, strings.NewReader(payload1), &stdout, &stderr)
	if rc != 2 {
		t.Fatalf("1st hold rc=%d want 2 stderr=%q", rc, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	msg := stderr.String()
	for _, want := range []string{
		"docdag context 000979",
		"docdag context 000979 --section Consequences",
		"docdag query --binding --where affected_services=contains:<svc> --fields id,title,status",
		"docdag resolve 000979 --fields id,title,status",
		"once per session",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("specific-ID guidance missing %q:\n%s", want, msg)
		}
	}
	receiptPath := filepath.Join(stateDir, "sess-a", "main", "adr-read.receipt")
	if _, err := os.Stat(receiptPath); err != nil {
		t.Fatalf("expected receipt at %s: %v", receiptPath, err)
	}

	// 2. 2nd call in same session proceeds (exit 0).
	stdout.Reset()
	stderr.Reset()
	rc = RunAdrRead(ctx, nil, strings.NewReader(payload1), &stdout, &stderr)
	if rc != 0 || stderr.Len() != 0 {
		t.Fatalf("2nd call in same session should exit 0: rc=%d stderr=%q", rc, stderr.String())
	}

	// 3. Different session holds on 1st match with directory/glob guidance.
	globPayload := adrReadBashPayload(t, "cd docs/ADR && rg 'fixed skeleton'", "/repo", "sess-b", "")
	stdout.Reset()
	stderr.Reset()
	rc = RunAdrRead(ctx, nil, strings.NewReader(globPayload), &stdout, &stderr)
	if rc != 2 {
		t.Fatalf("new session glob hold rc=%d want 2 stderr=%q", rc, stderr.String())
	}
	globMsg := stderr.String()
	for _, want := range []string{
		"docdag query --binding --fields id,title,status",
		"docdag query --binding --where",
		"docdag context <id>",
		"docdag context <id> --section Consequences",
		"docdag resolve <id> --fields id,title,status",
		"once per session",
	} {
		if !strings.Contains(globMsg, want) {
			t.Errorf("glob guidance missing %q:\n%s", want, globMsg)
		}
	}
	if strings.Index(globMsg, "docdag query --binding --fields id,title,status") >= strings.Index(globMsg, "docdag context <id>") {
		t.Errorf("glob guidance should lead with cross-ADR docdag query:\n%s", globMsg)
	}

	// 4. Different agent_id in same session holds separately.
	subagentPayload := adrReadBashPayload(t, "cat docs/ADR/000979.md", "/repo", "sess-a", "worker-1")
	stdout.Reset()
	stderr.Reset()
	if rc := RunAdrRead(ctx, nil, strings.NewReader(subagentPayload), &stdout, &stderr); rc != 2 {
		t.Fatalf("different agent_id should hold once: rc=%d stderr=%q", rc, stderr.String())
	}

	// 5. Missing session_id & conversationId falls back to "default": holds 1st, allows 2nd.
	defaultPayload := adrReadBashPayload(t, "cat docs/ADR/000979.md", "/repo", "", "")
	stdout.Reset()
	stderr.Reset()
	if rc := RunAdrRead(ctx, nil, strings.NewReader(defaultPayload), &stdout, &stderr); rc != 2 {
		t.Fatalf("default session 1st call should hold: rc=%d stderr=%q", rc, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if rc := RunAdrRead(ctx, nil, strings.NewReader(defaultPayload), &stdout, &stderr); rc != 0 {
		t.Fatalf("default session 2nd call should allow: rc=%d stderr=%q", rc, stderr.String())
	}

	// 6. Non-Bash tools (Read, Grep, Glob) are ignored (exit 0).
	for _, tool := range []string{"Read", "Grep", "Glob"} {
		raw, _ := json.Marshal(map[string]any{
			"session_id": "sess-nonbash",
			"tool_name":  tool,
			"tool_input": map[string]any{"file_path": "docs/ADR/000979.md"},
		})
		stdout.Reset()
		stderr.Reset()
		if rc := RunAdrRead(ctx, nil, bytes.NewReader(raw), &stdout, &stderr); rc != 0 || stderr.Len() != 0 {
			t.Fatalf("tool %s should be ignored: rc=%d stderr=%q", tool, rc, stderr.String())
		}
	}

	// 7. Argv list form (direct and bash -lc).
	argvPayload := adrReadBashPayload(t, []string{"bash", "-lc", "cat docs/ADR/000979.md"}, "/repo", "sess-argv", "")
	stdout.Reset()
	stderr.Reset()
	if rc := RunAdrRead(ctx, nil, strings.NewReader(argvPayload), &stdout, &stderr); rc != 2 || !strings.Contains(stderr.String(), "docdag context 000979") {
		t.Fatalf("argv 1st hold failed: rc=%d stderr=%q", rc, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if rc := RunAdrRead(ctx, nil, strings.NewReader(argvPayload), &stdout, &stderr); rc != 0 {
		t.Fatalf("argv 2nd call should allow: rc=%d stderr=%q", rc, stderr.String())
	}

	// 8. XDG_STATE_HOME fallback when ALT_ADR_PREFLIGHT_STATE_DIR is unset.
	xdgDir := t.TempDir()
	t.Setenv("ALT_ADR_PREFLIGHT_STATE_DIR", "")
	t.Setenv("XDG_STATE_HOME", xdgDir)
	xdgPayload := adrReadBashPayload(t, "cat docs/ADR/000979.md", "/repo", "sess-xdg", "")
	stdout.Reset()
	stderr.Reset()
	if rc := RunAdrRead(ctx, nil, strings.NewReader(xdgPayload), &stdout, &stderr); rc != 2 {
		t.Fatalf("xdg 1st hold failed: rc=%d stderr=%q", rc, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(xdgDir, "alt-adr-preflight", "sess-xdg", "main", "adr-read.receipt")); err != nil {
		t.Fatalf("expected receipt under XDG_STATE_HOME: %v", err)
	}

	// 9. ALT_ADR_PREFLIGHT=disabled bypasses guard.
	t.Setenv("ALT_ADR_PREFLIGHT", "disabled")
	disabledPayload := adrReadBashPayload(t, "cat docs/ADR/000979.md", "/repo", "sess-disabled", "")
	stdout.Reset()
	stderr.Reset()
	if rc := RunAdrRead(ctx, nil, strings.NewReader(disabledPayload), &stdout, &stderr); rc != 0 || stderr.Len() != 0 {
		t.Fatalf("disabled kill switch should allow: rc=%d stderr=%q", rc, stderr.String())
	}
}

func TestRunAdrReadFailClosedInputsAndStateDir(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	t.Setenv("ALT_ADR_PREFLIGHT_STATE_DIR", stateDir)
	t.Setenv("ALT_ADR_PREFLIGHT", "")

	badInputs := []struct {
		name string
		raw  []byte
	}{
		{"empty", []byte("")},
		{"malformed json", []byte("{bad")},
		{"invalid utf8", []byte{0xff, 0xfe}},
		{"missing tool_name", []byte(`{"tool_input":{"command":"echo hi"}}`)},
		{"missing tool_input", []byte(`{"tool_name":"Bash"}`)},
		{"missing command", []byte(`{"tool_name":"Bash","tool_input":{}}`)},
		{"non-string command", []byte(`{"tool_name":"Bash","tool_input":{"command":123}}`)},
	}
	for _, tc := range badInputs {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			rc := RunAdrRead(ctx, nil, bytes.NewReader(tc.raw), &stdout, &stderr)
			if rc != 2 || !strings.Contains(stderr.String(), "BLOCKED:") {
				t.Fatalf("%s: rc=%d stderr=%q, want exit 2 with BLOCKED:", tc.name, rc, stderr.String())
			}
		})
	}

	// Nil stdin fail-closed.
	var stdout, stderr bytes.Buffer
	if rc := RunAdrRead(ctx, nil, nil, &stdout, &stderr); rc != 2 || !strings.Contains(stderr.String(), "BLOCKED:") {
		t.Fatalf("nil stdin: rc=%d stderr=%q", rc, stderr.String())
	}

	// Unwritable state dir (regular file blocker) fail-closed.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("file\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALT_ADR_PREFLIGHT_STATE_DIR", blocker)
	stdout.Reset()
	stderr.Reset()
	payload := adrReadBashPayload(t, "cat docs/ADR/000979.md", "/repo", "sess-ioerr", "")
	if rc := RunAdrRead(ctx, nil, strings.NewReader(payload), &stdout, &stderr); rc != 2 || !strings.Contains(stderr.String(), "BLOCKED:") {
		t.Fatalf("unwritable state dir: rc=%d stderr=%q", rc, stderr.String())
	}
}
