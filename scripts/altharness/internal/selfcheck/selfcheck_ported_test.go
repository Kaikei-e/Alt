package selfcheck

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

func gitInDir(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Harness Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Harness Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}

func writeTrack(t *testing.T, repo, rel, content string) {
	t.Helper()
	full := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(full), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), fsperm.File); err != nil {
		t.Fatal(err)
	}
	gitInDir(t, repo, "add", rel)
}

func buildCleanSelfcheckRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitInDir(t, repo, "init")
	writeTrack(t, repo, "scripts/altharness/alt-harness", "#!/usr/bin/env bash\nexit 0\n")
	writeTrack(t, repo, "knowledge-sovereign/app/usecase/knowledge_home_projector/projector.go", "package knowledge_home_projector\n")
	writeTrack(t, repo, "knowledge-sovereign/app/usecase/knowledge_trail_projector/projector.go", "package knowledge_trail_projector\n")
	writeTrack(t, repo, "alt-backend/app/main.go", "package main\n")
	writeTrack(t, repo, "alt-frontend-sv/src/routes/(app)/+page.svelte", "<main />\n")
	writeTrack(t, repo, ".claude/rules/paths.md", "---\npaths:\n  - \"alt-backend/**/*.go\"\n  - \"alt-frontend-sv/src/routes/(app)/*.{svelte,ts}\"\n---\nRule body\n")

	codexCfg := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" preflight --hook`},
						map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" selfcheck --hook`},
					},
				},
			},
			"PreToolUse": []any{
				map[string]any{
					"matcher": "apply_patch|Edit|Write",
					"hooks": []any{
						map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" preflight --hook`},
						map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" guard-protected`},
						map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" guard-event-time`},
						map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" guard-scope`},
					},
				},
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" guard-ops --on-match deny`},
						map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" guard-exec`},
					},
				},
			},
		},
	}
	rawCodex, _ := json.MarshalIndent(codexCfg, "", "  ")
	writeTrack(t, repo, ".codex/hooks.json", string(rawCodex)+"\n")

	agyCfg := map[string]any{
		"alt-agent-harness": map[string]any{
			"SessionStart": []any{
				map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" agy-hook session-start`},
			},
			"PreToolUse": []any{
				map[string]any{
					"matcher": "run_command|Bash|shell",
					"hooks":   []any{map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" agy-hook pre-shell`}},
				},
				map[string]any{
					"matcher": "write_file|edit_file|Edit|Write|apply_patch",
					"hooks":   []any{map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" agy-hook pre-edit`}},
				},
			},
			"PostToolUse": []any{
				map[string]any{
					"matcher": "write_file|edit_file|Edit|Write|apply_patch",
					"hooks":   []any{map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" agy-hook post-edit`}},
				},
			},
			"Stop": []any{
				map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" agy-hook stop`},
			},
		},
	}
	rawAgy, _ := json.MarshalIndent(agyCfg, "", "  ")
	writeTrack(t, repo, ".agents/hooks.json", string(rawAgy)+"\n")

	shared := "# Shared\n<!-- shared:core:start -->\nNever skip verification.\n<!-- shared:core:end -->\n"
	writeTrack(t, repo, "CLAUDE.md", shared)
	writeTrack(t, repo, "AGENTS.md", shared)
	writeTrack(t, repo, ".agents/skills/sample-skill/SKILL.md", "---\nname: sample-skill\n---\n")
	writeTrack(t, repo, "docs/ADR/000928.md", "# ADR-000928\n")
	writeTrack(t, repo, "docs/postmortems/PM-2026-031-sample.md", "# PM-2026-031\n")
	writeTrack(t, repo, failurePatterns, makeTwoSectionPatterns(
		"`scripts/altharness/alt-harness guard-event-time` と `scripts/<svc>/placeholder.go`",
		"`scripts/altharness/alt-harness guard-protected`",
	))
	writeTrack(t, repo, failureSkill, "# Failure Pattern Review\n\n## 経路表\n\n| 差分が触るもの | 読む型 |\n|---|---|\n| DI | FP-01 |\n| proto | FP-02 |\n")
	writeTrack(t, repo, "scripts/lint/ast-grep/rules/go-no-raw-http-server.yml", "id: go-no-raw-http-server\nlanguage: go\n")
	writeTrack(t, repo, "scripts/harness/corpus/C-01/meta.toml", "id = \"C-01\"\n")
	writeTrack(t, repo, ruleLedgerRel, "---\nmax_judgment_rules: 1\n---\n\n# Rule Ledger\n\n| ID | 規則 | 強制手段 | 由来 |\n|---|---|---|---|\n| R-01 | Projectors use event time | `scripts/altharness/alt-harness guard-event-time` | ADR-000928 |\n| R-02 | Use canonical HTTP server | `ast-grep:go-no-raw-http-server` | C-01 |\n| R-03 | Follow TDD | 判断（プロセス規律） | AGENTS.md |\n")
	gitInDir(t, repo, "commit", "-m", "initial clean fixture")
	return repo
}

func makeFPSection(fpID, title, precedent, cmdNote, checkCmd string) string {
	return "## " + fpID + " " + title + "\n\n" +
		"**機構.** Something fails silently.\n\n" +
		"**先例.**\n- " + precedent + " — example precedent.\n\n" +
		"**問い.**\n1. Is it checked?\n\n" +
		"**確認コマンド.**\n```bash\n" + checkCmd + "\n```\n\n" +
		"**機械的に見ているもの.** " + cmdNote + "\n"
}

func makeTwoSectionPatterns(note1, note2 string) string {
	return "# 失敗の型（FP-01 〜 FP-02）\n\n---\n\n" +
		makeFPSection("FP-01", "Silent failure", "ADR-000928", note1, "rg -n 'foo' <dir>/") +
		"\n---\n\n" +
		makeFPSection("FP-02", "Contract drift", "PM-2026-031", note2, "rg -n 'bar' <dir>/")
}

func runSC(root string, extra ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	args := append([]string{"--root", root}, extra...)
	rc := Run(context.Background(), args, &stdout, &stderr)
	return rc, stdout.String(), stderr.String()
}

func writeClaudeSettingsJSON(t *testing.T, repo string, cfg map[string]any) {
	t.Helper()
	p := filepath.Join(repo, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(p, append(raw, '\n'), fsperm.File); err != nil {
		t.Fatal(err)
	}
}

func TestSelfcheckH1ToH9Parallel(t *testing.T) {
	t.Parallel()

	t.Run("clean fixture and --hook and Claude settings", func(t *testing.T) {
		t.Parallel()
		repo := buildCleanSelfcheckRepo(t)
		rc, out, _ := runSC(repo)
		if rc != 0 || !strings.Contains(out, "settings.json") || !strings.Contains(strings.ToLower(out), "not checked") {
			t.Fatalf("clean rc=%d out=%s", rc, out)
		}
		rc, out, _ = runSC(repo, "--hook")
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if rc != 0 || len(lines) != 1 || !strings.HasPrefix(lines[0], "agent_harness_selfcheck: OK") {
			t.Fatalf("--hook rc=%d out=%q", rc, out)
		}

		writeClaudeSettingsJSON(t, repo, map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": "Edit|Write|NotebookEdit",
						"hooks": []any{
							map[string]any{"type": "command", "command": `"$CLAUDE_PROJECT_DIR/scripts/altharness/alt-harness" preflight --hook`, "onFailure": "block"},
							map[string]any{"type": "command", "command": `"$CLAUDE_PROJECT_DIR/scripts/altharness/alt-harness" guard-protected`, "onFailure": "block"},
							map[string]any{"type": "command", "command": `"${CLAUDE_PROJECT_DIR}/scripts/altharness/alt-harness" guard-event-time`, "onFailure": "block"},
							map[string]any{"type": "command", "command": `"$CLAUDE_PROJECT_DIR/scripts/altharness/alt-harness" guard-scope`, "onFailure": "block"},
						},
					},
					map[string]any{
						"matcher": "Bash",
						"hooks": []any{
							map[string]any{"type": "command", "command": `"$CLAUDE_PROJECT_DIR/scripts/altharness/alt-harness" guard-ops`, "onFailure": "block"},
							map[string]any{"type": "command", "command": `"$CLAUDE_PROJECT_DIR/scripts/altharness/alt-harness" guard-exec`, "onFailure": "block"},
						},
					},
				},
			},
		})
		rc, out, _ = runSC(repo)
		if rc != 0 {
			t.Fatalf("clean with claude settings rc=%d out=%s", rc, out)
		}
	})

	t.Run("H1 bad env var and --hook FAILED line", func(t *testing.T) {
		t.Parallel()
		repo := buildCleanSelfcheckRepo(t)
		p := filepath.Join(repo, ".codex", "hooks.json")
		raw, _ := os.ReadFile(p)
		var cfg map[string]any
		_ = json.Unmarshal(raw, &cfg)
		pre := cfg["hooks"].(map[string]any)["PreToolUse"].([]any)
		g0 := pre[0].(map[string]any)
		g0["hooks"] = append(g0["hooks"].([]any), map[string]any{"type": "command", "command": "echo $CLAUDE_FILE_PATHS"})
		outRaw, _ := json.Marshal(cfg)
		_ = os.WriteFile(p, outRaw, fsperm.File)

		rc, out, _ := runSC(repo)
		if rc != 1 || !strings.Contains(out, "H1 ") {
			t.Fatalf("H1 rc=%d out=%s", rc, out)
		}
		rc, out, _ = runSC(repo, "--hook")
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if rc != 0 || len(lines) != 1 || !strings.HasPrefix(lines[0], "agent_harness_selfcheck_FAILED:") || !strings.Contains(lines[0], "H1") || !strings.Contains(lines[0], "alt-harness selfcheck") {
			t.Fatalf("H1 --hook rc=%d out=%q", rc, out)
		}
	})

	t.Run("H2 missing script and unknown alt-harness subcommand", func(t *testing.T) {
		t.Parallel()
		repo := buildCleanSelfcheckRepo(t)
		p := filepath.Join(repo, ".codex", "hooks.json")
		raw, _ := os.ReadFile(p)
		var cfg map[string]any
		_ = json.Unmarshal(raw, &cfg)
		pre := cfg["hooks"].(map[string]any)["PreToolUse"].([]any)
		g0 := pre[0].(map[string]any)
		g0["hooks"] = append(g0["hooks"].([]any),
			map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/does_not_exist.sh"`},
			map[string]any{"type": "command", "command": `"$(git rev-parse --show-toplevel)/scripts/altharness/alt-harness" totally-bogus-cmd`},
		)
		outRaw, _ := json.Marshal(cfg)
		_ = os.WriteFile(p, outRaw, fsperm.File)

		rc, out, _ := runSC(repo)
		if rc != 1 || !strings.Contains(out, "does_not_exist.sh") || !strings.Contains(out, "totally-bogus-cmd") {
			t.Fatalf("H2 rc=%d out=%s", rc, out)
		}
	})

	t.Run("H3 Claude missing onFailure block", func(t *testing.T) {
		t.Parallel()
		repo := buildCleanSelfcheckRepo(t)
		writeClaudeSettingsJSON(t, repo, map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": "Edit|Write",
						"hooks": []any{
							map[string]any{"type": "command", "command": `"$CLAUDE_PROJECT_DIR/scripts/altharness/alt-harness" guard-protected`},
						},
					},
				},
			},
		})
		rc, out, _ := runSC(repo)
		if rc != 1 || !strings.Contains(out, "H3 ") {
			t.Fatalf("H3 rc=%d out=%s", rc, out)
		}
	})

	t.Run("H4 missing guards and Codex --on-match deny and missing .agents/hooks.json", func(t *testing.T) {
		t.Parallel()
		repo := buildCleanSelfcheckRepo(t)
		p := filepath.Join(repo, ".codex", "hooks.json")
		raw, _ := os.ReadFile(p)
		// Strip --on-match deny and preflight.
		mutated := strings.ReplaceAll(string(raw), "guard-ops --on-match deny", "guard-ops")
		mutated = strings.ReplaceAll(mutated, "preflight --hook", "selfcheck --hook")
		_ = os.WriteFile(p, []byte(mutated), fsperm.File)
		_ = os.Remove(filepath.Join(repo, ".agents", "hooks.json"))

		rc, out, _ := runSC(repo)
		for _, want := range []string{"H4 .codex/hooks.json:", "--on-match deny", "preflight", "H4 .agents/hooks.json:"} {
			if rc != 1 || !strings.Contains(out, want) {
				t.Fatalf("H4 missing %q in rc=%d out=%s", want, rc, out)
			}
		}
	})

	t.Run("H5 reports eventtime.go when only _test.go exists", func(t *testing.T) {
		t.Parallel()
		repo := buildCleanSelfcheckRepo(t)
		gitInDir(t, repo, "rm", "-f", "knowledge-sovereign/app/usecase/knowledge_trail_projector/projector.go")
		writeTrack(t, repo, "knowledge-sovereign/app/usecase/knowledge_trail_projector/projector_test.go", "package knowledge_trail_projector\n")
		gitInDir(t, repo, "commit", "-m", "only test file")

		rc, out, _ := runSC(repo)
		if rc != 1 || !strings.Contains(out, "H5 scripts/altharness/internal/guards/eventtime.go:") {
			t.Fatalf("H5 rc=%d out=%s", rc, out)
		}
	})

	t.Run("H6 dead rule path, H7 shared mismatch, H8 size limits, H9 broken skill symlink", func(t *testing.T) {
		t.Parallel()
		repo := buildCleanSelfcheckRepo(t)
		writeTrack(t, repo, ".claude/rules/dead.md", "---\npaths:\n  - \"nonexistent-dir/**/*.{go,py}\"\n---\nDead\n")
		gitInDir(t, repo, "commit", "-m", "dead rule")
		_ = os.WriteFile(filepath.Join(repo, "AGENTS.md"), []byte("# Shared\n<!-- shared:core:start -->\nDifferent.\n<!-- shared:core:end -->\n"+strings.Repeat("x", 33000)), fsperm.File)
		claudeRaw, _ := os.ReadFile(filepath.Join(repo, "CLAUDE.md"))
		_ = os.WriteFile(filepath.Join(repo, "CLAUDE.md"), append(claudeRaw, []byte(strings.Repeat("line\n", 205))...), fsperm.File)
		_ = os.Symlink(filepath.Join(repo, "does-not-exist"), filepath.Join(repo, ".agents", "skills", "broken-skill"))

		rc, out, _ := runSC(repo)
		for _, id := range []string{"H6 ", "H7 ", "H8 CLAUDE.md:", "H8 AGENTS.md:", "H9 "} {
			if rc != 1 || !strings.Contains(out, id) {
				t.Fatalf("missing %q in rc=%d out=%s", id, rc, out)
			}
		}
	})
}

func TestSelfcheckH10CatalogParallel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		mutate   func(t *testing.T, repo string)
		wantSubs []string
	}{
		{
			"missing patterns.md",
			func(t *testing.T, repo string) { _ = os.Remove(filepath.Join(repo, failurePatterns)) },
			[]string{"H10", "patterns.md", "missing"},
		},
		{
			"missing SKILL.md",
			func(t *testing.T, repo string) { _ = os.Remove(filepath.Join(repo, failureSkill)) },
			[]string{"H10", "SKILL.md", "missing"},
		},
		{
			"section gap FP-01 FP-03",
			func(t *testing.T, repo string) {
				p := "# 失敗の型（FP-01 〜 FP-03）\n\n" +
					makeFPSection("FP-01", "First", "ADR-000928", "`scripts/altharness/alt-harness selfcheck`", "rg -n x") + "\n" +
					makeFPSection("FP-03", "Third", "PM-2026-031", "`scripts/altharness/alt-harness selfcheck`", "rg -n x")
				s := "# Skill\n\n## 経路表\n\n| A | B |\n|---|---|\n| a | FP-01 |\n| b | FP-03 |\n"
				_ = os.WriteFile(filepath.Join(repo, failurePatterns), []byte(p), fsperm.File)
				_ = os.WriteFile(filepath.Join(repo, failureSkill), []byte(s), fsperm.File)
			},
			[]string{"missing section 'FP-02'"},
		},
		{
			"duplicate section FP-02",
			func(t *testing.T, repo string) {
				raw, _ := os.ReadFile(filepath.Join(repo, failurePatterns))
				_ = os.WriteFile(filepath.Join(repo, failurePatterns), append(raw, []byte("\n"+makeFPSection("FP-02", "Dup", "PM-2026-031", "`scripts/altharness/alt-harness selfcheck`", "rg -n x"))...), fsperm.File)
			},
			[]string{"duplicate section 'FP-02'"},
		},
		{
			"invalid heading FP-1",
			func(t *testing.T, repo string) {
				raw, _ := os.ReadFile(filepath.Join(repo, failurePatterns))
				_ = os.WriteFile(filepath.Join(repo, failurePatterns), append(raw, []byte("\n## FP-1 Bad\n\nText.\n")...), fsperm.File)
			},
			[]string{"invalid section heading '## FP-1 Bad'"},
		},
		{
			"FP-00 rejected",
			func(t *testing.T, repo string) {
				raw, _ := os.ReadFile(filepath.Join(repo, failurePatterns))
				mut := strings.Replace(string(raw), "## FP-01", makeFPSection("FP-00", "Zero", "ADR-000928", "`scripts/altharness/alt-harness selfcheck`", "rg -n x")+"\n## FP-01", 1)
				_ = os.WriteFile(filepath.Join(repo, failurePatterns), []byte(mut), fsperm.File)
				_ = os.WriteFile(filepath.Join(repo, failureSkill), []byte("# Skill\n\n## 経路表\n\n| A | B |\n|---|---|\n| z | FP-00 |\n| a | FP-01 |\n| b | FP-02 |\n"), fsperm.File)
			},
			[]string{"FP-00", "start at FP-01"},
		},
		{
			"missing lead-in",
			func(t *testing.T, repo string) {
				raw, _ := os.ReadFile(filepath.Join(repo, failurePatterns))
				_ = os.WriteFile(filepath.Join(repo, failurePatterns), []byte(strings.Replace(string(raw), "**問い.**\n1. Is it checked?\n\n", "", 1)), fsperm.File)
			},
			[]string{"FP-01 is missing lead-in **問い.**"},
		},
		{
			"duplicate lead-in",
			func(t *testing.T, repo string) {
				raw, _ := os.ReadFile(filepath.Join(repo, failurePatterns))
				_ = os.WriteFile(filepath.Join(repo, failurePatterns), []byte(strings.Replace(string(raw), "**機構.** Something fails silently.", "**機構.** Something fails silently.\n\n**機構.** Dup.", 1)), fsperm.File)
			},
			[]string{"FP-01 contains lead-in **機構.** 2 times"},
		},
		{
			"out-of-order lead-in",
			func(t *testing.T, repo string) {
				swapped := "## FP-01 Swapped\n\n**先例.**\n- ADR-000928 — p.\n\n**機構.** m.\n\n**問い.**\n1. q?\n\n**確認コマンド.**\n```bash\nrg -n x\n```\n\n**機械的に見ているもの.** `scripts/altharness/alt-harness selfcheck`\n"
				p := "# 失敗の型（FP-01 〜 FP-02）\n\n" + swapped + "\n" + makeFPSection("FP-02", "Second", "PM-2026-031", "`scripts/altharness/alt-harness selfcheck`", "rg -n x")
				_ = os.WriteFile(filepath.Join(repo, failurePatterns), []byte(p), fsperm.File)
			},
			[]string{"FP-01 lead-ins are out of order"},
		},
		{
			"missing ADR precedent adjacent to Japanese",
			func(t *testing.T, repo string) {
				raw, _ := os.ReadFile(filepath.Join(repo, failurePatterns))
				_ = os.WriteFile(filepath.Join(repo, failurePatterns), []byte(strings.Replace(string(raw), "ADR-000928", "ADR-000999は失敗例", 1)), fsperm.File)
			},
			[]string{"ADR-000999", "docs/ADR/000999.md"},
		},
		{
			"missing PM precedent adjacent to Japanese",
			func(t *testing.T, repo string) {
				raw, _ := os.ReadFile(filepath.Join(repo, failurePatterns))
				_ = os.WriteFile(filepath.Join(repo, failurePatterns), []byte(strings.Replace(string(raw), "PM-2026-031", "PM-2026-999で再発", 1)), fsperm.File)
			},
			[]string{"PM-2026-999"},
		},
		{
			"duplicate PM precedent file",
			func(t *testing.T, repo string) {
				writeTrack(t, repo, "docs/postmortems/PM-2026-031-duplicate.md", "# dup\n")
			},
			[]string{"PM-2026-031", "multiple"},
		},
		{
			"abbreviated precedent ID",
			func(t *testing.T, repo string) {
				raw, _ := os.ReadFile(filepath.Join(repo, failurePatterns))
				_ = os.WriteFile(filepath.Join(repo, failurePatterns), []byte(strings.Replace(string(raw), "PM-2026-031", "PM-2026-031 / 999", 1)), fsperm.File)
			},
			[]string{"999", "full"},
		},
		{
			"missing repo path and unknown alt-harness subcommand in patterns.md",
			func(t *testing.T, repo string) {
				p := "# 失敗の型（FP-01 〜 FP-02）\n\n" +
					makeFPSection("FP-01", "First", "ADR-000928", "`scripts/no_such_tool.sh`", "scripts/missing_in_fence.sh --check") + "\n" +
					makeFPSection("FP-02", "Second", "PM-2026-031", "`scripts/altharness/alt-harness bogus-sub`", "rg -n x")
				_ = os.WriteFile(filepath.Join(repo, failurePatterns), []byte(p), fsperm.File)
			},
			[]string{"scripts/no_such_tool.sh", "scripts/missing_in_fence.sh", "bogus-sub"},
		},
		{
			"routing table unknown FP-99 and missing FP-02",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, failureSkill), []byte("# Skill\n\n## 経路表\n\n| A | B |\n|---|---|\n| a | FP-01, FP-99と他 |\n"), fsperm.File)
			},
			[]string{"FP-99", "FP-02", "does not appear"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := buildCleanSelfcheckRepo(t)
			tc.mutate(t, repo)
			rc, out, _ := runSC(repo)
			if rc != 1 {
				t.Fatalf("expected rc=1, got %d out=%s", rc, out)
			}
			for _, sub := range tc.wantSubs {
				if !strings.Contains(out, sub) {
					t.Fatalf("missing %q in out=%s", sub, out)
				}
			}
		})
	}
}

func TestSelfcheckH11RuleLedgerParallel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		mutate   func(t *testing.T, repo string)
		wantCode int
		wantSubs []string
	}{
		{
			"missing rule-ledger.md",
			func(t *testing.T, repo string) { _ = os.Remove(filepath.Join(repo, ruleLedgerRel)) },
			1, []string{"H11", "rule-ledger.md"},
		},
		{
			"missing max_judgment_rules",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, ruleLedgerRel), []byte("| ID | 規則 | 強制手段 | 由来 |\n|---|---|---|---|\n| R-01 | S | `scripts/altharness/alt-harness guard-event-time` | ADR-000928 |\n"), fsperm.File)
			},
			1, []string{"max_judgment_rules"},
		},
		{
			"wrong columns",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, ruleLedgerRel), []byte("---\nmax_judgment_rules: 0\n---\n| ID | Rule | How |\n|---|---|---|\n| R-01 | S | `scripts/altharness/alt-harness selfcheck` |\n"), fsperm.File)
			},
			1, []string{"H11"},
		},
		{
			"missing script path",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, ruleLedgerRel), []byte("---\nmax_judgment_rules: 0\n---\n| ID | 規則 | 強制手段 | 由来 |\n|---|---|---|---|\n| R-01 | Missing | `scripts/lint/does_not_exist.sh` | ADR-000928 |\n"), fsperm.File)
			},
			1, []string{"scripts/lint/does_not_exist.sh"},
		},
		{
			"unknown alt-harness subcommand",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, ruleLedgerRel), []byte("---\nmax_judgment_rules: 0\n---\n| ID | 規則 | 強制手段 | 由来 |\n|---|---|---|---|\n| R-01 | Bad | `scripts/altharness/alt-harness not-a-cmd` | ADR-000928 |\n"), fsperm.File)
			},
			1, []string{"not-a-cmd"},
		},
		{
			"alt-harness without subcommand",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, ruleLedgerRel), []byte("---\nmax_judgment_rules: 0\n---\n| ID | 規則 | 強制手段 | 由来 |\n|---|---|---|---|\n| R-01 | Bare | `scripts/altharness/alt-harness` | ADR-000928 |\n"), fsperm.File)
			},
			1, []string{"without a subcommand"},
		},
		{
			"missing ast-grep rule ID",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, ruleLedgerRel), []byte("---\nmax_judgment_rules: 0\n---\n| ID | 規則 | 強制手段 | 由来 |\n|---|---|---|---|\n| R-01 | Bad | `ast-grep:no-such-rule` | C-01 |\n"), fsperm.File)
			},
			1, []string{"no-such-rule"},
		},
		{
			"vague non-judgment row",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, ruleLedgerRel), []byte("---\nmax_judgment_rules: 0\n---\n| ID | 規則 | 強制手段 | 由来 |\n|---|---|---|---|\n| R-01 | Vague | CI checks it | AGENTS.md |\n"), fsperm.File)
			},
			1, []string{"R-01"},
		},
		{
			"missing corpus C-99 and ADR-000999",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, ruleLedgerRel), []byte("---\nmax_judgment_rules: 0\n---\n| ID | 規則 | 強制手段 | 由来 |\n|---|---|---|---|\n| R-01 | Bad | `scripts/altharness/alt-harness selfcheck` | C-99 / ADR-000999 |\n"), fsperm.File)
			},
			1, []string{"C-99", "ADR-000999"},
		},
		{
			"judgment rows exceed max_judgment_rules",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, ruleLedgerRel), []byte("---\nmax_judgment_rules: 1\n---\n| ID | 規則 | 強制手段 | 由来 |\n|---|---|---|---|\n| R-01 | One | 判断 | AGENTS.md |\n| R-02 | Two | 判断（レビュー） | AGENTS.md |\n"), fsperm.File)
			},
			1, []string{"exceeds max_judgment_rules"},
		},
		{
			"judgment rows below max_judgment_rules",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, ruleLedgerRel), []byte("---\nmax_judgment_rules: 2\n---\n| ID | 規則 | 強制手段 | 由来 |\n|---|---|---|---|\n| R-01 | Mech | `scripts/altharness/alt-harness selfcheck` | ADR-000928 |\n| R-02 | One | 判断 | AGENTS.md |\n"), fsperm.File)
			},
			1, []string{"below max_judgment_rules"},
		},
		{
			"directory-only ref rejected",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, ruleLedgerRel), []byte("---\nmax_judgment_rules: 0\n---\n| ID | 規則 | 強制手段 | 由来 |\n|---|---|---|---|\n| R-01 | Dir | `scripts/` | ADR-000928 |\n"), fsperm.File)
			},
			1, []string{"scripts/"},
		},
		{
			"non-existent bare file token rejected",
			func(t *testing.T, repo string) {
				_ = os.WriteFile(filepath.Join(repo, ruleLedgerRel), []byte("---\nmax_judgment_rules: 0\n---\n| ID | 規則 | 強制手段 | 由来 |\n|---|---|---|---|\n| R-01 | Fake | `scripts/altharness/alt-harness selfcheck`, `fake_nonexistent.go` | ADR-000928 |\n"), fsperm.File)
			},
			1, []string{"fake_nonexistent.go"},
		},
		{
			"suppressions.toml governance mismatch",
			func(t *testing.T, repo string) {
				writeTrack(t, repo, ratchetRel, "version = 1\n\n[governance]\nmax_judgment_rules = 99\n")
			},
			1, []string{"suppressions.toml"},
		},
		{
			"suppressions.toml governance match passes",
			func(t *testing.T, repo string) {
				writeTrack(t, repo, ratchetRel, "version = 1\n\n[governance]\nmax_judgment_rules = 1\n")
			},
			0, nil,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := buildCleanSelfcheckRepo(t)
			tc.mutate(t, repo)
			rc, out, _ := runSC(repo)
			if rc != tc.wantCode {
				t.Fatalf("rc=%d want %d out=%s", rc, tc.wantCode, out)
			}
			for _, sub := range tc.wantSubs {
				if !strings.Contains(out, sub) {
					t.Fatalf("missing %q in out=%s", sub, out)
				}
			}
		})
	}
}
