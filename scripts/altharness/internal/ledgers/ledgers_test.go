package ledgers

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

func TestPostmortemUsageAndBaseFailClosed(t *testing.T) {
	repo := initGitRepo(t)
	writePatterns(t, repo)
	writeReadme(t, repo, nil)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	cases := []struct {
		name  string
		args  []string
		want  int
		token string
		out   bool
	}{
		{"base without check", []string{"--root", repo, "--base", head}, 2, "--base requires --check", false},
		{"base starts with dash", []string{"--root", repo, "--check", "--base", "--output=/tmp/alt-ledger-nope"}, 2, "invalid base revision", false},
		{"unknown base", []string{"--root", repo, "--check", "--base", "does-not-exist"}, 2, "unknown base revision", false},
		{"empty base", []string{"--root", repo, "--check", "--base="}, 2, "invalid base revision", false},
		{"missing root", []string{"--root", filepath.Join(repo, "missing")}, 2, "does not exist", false},
		{"no added pm", []string{"--root", repo, "--check", "--base", head}, 0, head, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errText := runPost(t, tc.args)
			if code != tc.want {
				t.Fatalf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.want, out, errText)
			}
			blob := errText
			if tc.out {
				blob = out
			}
			if !strings.Contains(blob, tc.token) {
				t.Fatalf("missing %q\nstdout:\n%s\nstderr:\n%s", tc.token, out, errText)
			}
			if strings.Contains(tc.name, "dash") || strings.Contains(tc.name, "empty") {
				if _, err := os.Stat("/tmp/alt-ledger-nope"); err == nil {
					t.Fatal("invalid --base created a path")
				}
			}
		})
	}
}

func TestPostmortemLedgerFixture(t *testing.T) {
	repo := t.TempDir()
	mustMkdir(t, filepath.Join(repo, "docs", "postmortems"))
	pm1 := "PM-2026-001-current-style.md"
	pm2 := "PM-2026-002-legacy-style.md"
	pm3 := "PM-2026-003-no-table.md"
	os.WriteFile(filepath.Join(repo, "docs", "postmortems", pm1), []byte(validPM("PM-2026-001", nil, "")), fsperm.File)
	os.WriteFile(filepath.Join(repo, "docs", "postmortems", pm2), []byte("# PM-2026-002: Legacy\n\n"+
		"## アクションアイテム\n\n"+
		"### 予防（Prevent）\n\n"+
		"| # | アクション | 担当 | 期限 | 状態 |\n"+
		"|---|---|---|---|---|\n"+
		"| P-1 | Fix retry loop | alice | 2026-03-20 | Done |\n"+
		"| P-2 | Drop legacy | bob | 2026-03-25 | Won't do |\n\n"+
		"### 検知（Detect）\n\n"+
		"| # | アクション | 担当 | 期限 | 状態 |\n"+
		"|---|---|---|---|---|\n"+
		"| D-1 | Add alert for PM-2026-001 item | carol | 2026-03-30 | 未着手 |\n"+
		"| D-2 | Check metric | dave | 2026-04-05 | 進行中 |\n"+
		"| D-3 | Verify dashboard | eve | 2026-04-10 | ✓ |\n"+
		"| D-4 | Weird status | frank | 2026-04-12 | 検討中 |\n"), fsperm.File)
	os.WriteFile(filepath.Join(repo, "docs", "postmortems", pm3), []byte("# PM-2026-003: Prose only\n\n## アクションアイテム\n\nNo table or checklist here.\n"), fsperm.File)

	code, out, errText := runPost(t, []string{"--root", repo, "--json"})
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, errText)
	}
	var payload struct {
		Rows []struct {
			PostmortemID     string `json:"postmortem_id"`
			ItemID           string `json:"item_id"`
			Category         string `json:"category"`
			NormalisedStatus string `json:"normalised_status"`
		} `json:"rows"`
		Unparsed []string `json:"unparsed"`
		Summary  struct {
			Postmortems int `json:"postmortems"`
			Rows        int `json:"rows"`
			Unparsed    int `json:"unparsed"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if payload.Summary.Postmortems != 3 || payload.Summary.Rows != 8 || payload.Summary.Unparsed != 1 {
		t.Fatalf("summary %+v", payload.Summary)
	}
	if len(payload.Unparsed) != 1 || !strings.Contains(payload.Unparsed[0], pm3) {
		t.Fatalf("unparsed %#v", payload.Unparsed)
	}
	got := map[string]string{}
	for _, row := range payload.Rows {
		got[row.PostmortemID+" "+row.ItemID] = row.Category + "/" + row.NormalisedStatus
	}
	checks := map[string]string{
		"PM-2026-002 D-1": "検知/open",
		"PM-2026-002 P-1": "予防/done",
		"PM-2026-002 P-2": "予防/dropped",
		"PM-2026-002 D-2": "検知/open",
		"PM-2026-002 D-3": "検知/done",
		"PM-2026-002 D-4": "検知/unknown",
	}
	for key, want := range checks {
		if got[key] != want {
			t.Fatalf("%s = %q, want %q\nall %#v", key, got[key], want, got)
		}
	}

	code, out, errText = runPost(t, []string{"--root", repo, "--overdue", "--today", "2026-04-20", "--json"})
	if code != 0 {
		t.Fatal(errText)
	}
	payload.Rows = nil
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatal(err)
	}
	ids := map[string]struct{}{}
	for _, row := range payload.Rows {
		ids[row.PostmortemID+" "+row.ItemID] = struct{}{}
	}
	for _, key := range []string{"PM-2026-002 D-1", "PM-2026-002 D-2", "PM-2026-002 D-4"} {
		if _, ok := ids[key]; !ok {
			t.Fatalf("overdue missing %s in %#v", key, ids)
		}
	}
	if len(ids) != 3 {
		t.Fatalf("overdue %#v", ids)
	}
}

func TestPostmortemGateFindings(t *testing.T) {
	repo := initGitRepo(t)
	writePatterns(t, repo)
	basePM := "PM-2026-001-base.md"
	os.WriteFile(filepath.Join(repo, "docs", "postmortems", basePM), []byte(validPM("PM-2026-001", nil, "")), fsperm.File)
	writeReadme(t, repo, [][2]string{{"PM-2026-001", basePM}})
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	newPM := "PM-2026-002-new.md"
	os.WriteFile(filepath.Join(repo, "docs", "postmortems", newPM), []byte(validPM("PM-2026-002", []string{
		"| 1 | 予防 | Fixed the bug without citing anything | alice | 2026-05-01 | 完了 |",
	}, "")), fsperm.File)
	writeReadme(t, repo, [][2]string{{"PM-2026-001", basePM}, {"PM-2026-002", newPM}})
	code, out, errText := runPost(t, []string{"--root", repo, "--check", "--base", base})
	if code != 1 || !strings.Contains(out, "P5") {
		t.Fatalf("P5 exit %d\n%s\n%s", code, out, errText)
	}

	os.WriteFile(filepath.Join(repo, "docs", "postmortems", "notes.md"), []byte("# bad\n"), fsperm.File)
	code, out, errText = runPost(t, []string{"--root", repo, "--check", "--base", base})
	if code != 1 || !strings.Contains(out, "P11") {
		t.Fatalf("P11 exit %d\n%s\n%s", code, out, errText)
	}
}

func TestMigrationStatements(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want int
		kind string
	}{
		{"drop table", "DROP TABLE old_feeds;\n", 1, "DROP TABLE"},
		{"drop column", "ALTER TABLE items DROP COLUMN legacy_flag;\n", 1, "DROP COLUMN"},
		{"truncate", "TRUNCATE TABLE logs;\n", 1, "TRUNCATE"},
		{"delete", "DELETE FROM recap_jobs;\n", 1, "DELETE FROM without WHERE"},
		{"using null", "ALTER TABLE rag_chunks\n  ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);\n", 1, "USING NULL"},
		{"update null", "UPDATE reports SET summary = NULL;\n", 1, "UPDATE"},
		{"lower", "drop table if exists scratch_jobs;\n", 1, "DROP TABLE"},
		{"ok comment", "-- destructive-ok: table is a disposable projection rebuilt from the event log\nDROP TABLE old_feeds;\n", 0, ""},
		{"short comment", "-- destructive-ok: safe enough\nDROP TABLE scratch;\n", 1, "DROP TABLE"},
		{"blank comment", "-- destructive-ok:                     \nTRUNCATE scratch;\n", 1, "TRUNCATE"},
		{"comments ignored", "-- DROP TABLE foo;\n/* ALTER TABLE x DROP COLUMN y; */\nINSERT INTO audit_log (note) VALUES ('DROP TABLE users');\nCREATE TABLE ok_table (id INT);\n", 0, ""},
		{"where passes", "DELETE FROM jobs WHERE status = 'obsolete';\nUPDATE jobs SET error = NULL WHERE status = 'retried';\n", 0, ""},
		{"line number", "/* line 1\n   line 2 */\n\nDROP TABLE target;\n", 1, ":4:"},
		{"do where then delete", "DO $$ BEGIN IF EXISTS (SELECT 1 FROM pg_tables WHERE tablename='x') THEN DELETE FROM x; END IF; END $$;", 1, "DELETE FROM"},
		{"cte where", "WITH k AS (SELECT id FROM t WHERE f) DELETE FROM t2;", 1, "DELETE FROM"},
		{"using where passes", "DELETE FROM t USING (SELECT 1 FROM u WHERE u.a=1) s WHERE t.id = s.id;\n", 0, ""},
		{"e string", "INSERT INTO notes VALUES (E'it\\'s fine');  DROP TABLE users;\n", 1, "DROP TABLE"},
		{"dollar quote", "COMMENT ON TABLE t IS $$it's a table$$;  DROP TABLE users;\n", 1, "DROP TABLE"},
		{"drop schema", "DROP SCHEMA legacy_schema;\n", 1, "DROP SCHEMA"},
		{"drop type cascade", "DROP TYPE old_enum CASCADE;\n", 1, "DROP ... CASCADE"},
		{"on delete cascade", "ALTER TABLE t DROP CONSTRAINT fk, ADD CONSTRAINT fk FOREIGN KEY (x) REFERENCES p(x) ON DELETE CASCADE;\n", 0, ""},
		{"drop constraint", "ALTER TABLE t DROP CONSTRAINT fk_user;\n", 0, ""},
		{"ch delete", "ALTER TABLE logs DELETE WHERE level = 'DEBUG';\n", 1, "ALTER TABLE ... DELETE"},
		{"modify ttl", "ALTER TABLE logs MODIFY TTL timestamp + INTERVAL 1 DAY DELETE;\n", 1, "MODIFY TTL"},
		{"second statement", "-- destructive-ok: table is a disposable projection rebuilt from event log\nDROP TABLE temp_one;\nDROP TABLE temp_two;\n", 1, ":3:"},
		{"ok in block comment", "/*\n-- destructive-ok: block comment must not excuse the statement below\n*/\nDROP TABLE temp_one;\n", 1, "DROP TABLE"},
		{"ok inside do", "DO $$\nBEGIN\n  -- destructive-ok: projection table is rebuilt from event log on demand\n  DELETE FROM proj;\nEND\n$$;\n", 0, ""},
		{"ok above do", "-- destructive-ok: projection table is rebuilt from event log on demand\nDO $$\nBEGIN\n  DELETE FROM proj;\nEND\n$$;\n", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkMigrationText("migrations/x.sql", tc.sql)
			if tc.want == 0 && len(got) != 0 {
				t.Fatalf("findings %#v", got)
			}
			if tc.want != 0 && len(got) == 0 {
				t.Fatal("expected a finding")
			}
			if tc.kind != "" && !strings.Contains(strings.Join(got, "\n"), tc.kind) {
				t.Fatalf("missing %q in %#v", tc.kind, got)
			}
		})
	}
}

func TestMigrationGateFailClosed(t *testing.T) {
	repo := initGitRepo(t)
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("init\n"), fsperm.File)
	git(t, repo, "add", "README.md")
	git(t, repo, "commit", "-m", "init")
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	rel := "migrations-atlas/migrations/20260101_drop.sql"
	mustMkdir(t, filepath.Join(repo, filepath.Dir(rel)))
	os.WriteFile(filepath.Join(repo, rel), []byte("DROP TABLE old_feeds;\n"), fsperm.File)
	git(t, repo, "add", rel)
	git(t, repo, "commit", "-m", "add drop")

	cases := []struct {
		name  string
		args  []string
		want  int
		token string
	}{
		{"finding", []string{"--root", repo, "--base", head}, 1, "M1 " + rel},
		{"unknown", []string{"--root", repo, "--base", "does-not-exist"}, 2, "unknown base revision"},
		{"dash base", []string{"--base=--output=/tmp/alt-mig-nope", "--root", repo}, 2, "invalid base revision"},
		{"missing base", []string{"--root", repo}, 2, "--base is required"},
		{"nongit", []string{"--root", t.TempDir(), "--base", "HEAD"}, 2, "not inside a git repository"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := RunMigration(context.Background(), tc.args, &out, &errBuf)
			blob := out.String() + errBuf.String()
			if code != tc.want || !strings.Contains(blob, tc.token) {
				t.Fatalf("exit %d want %d\n%s", code, tc.want, blob)
			}
		})
	}
	if _, err := os.Stat("/tmp/alt-mig-nope"); err == nil {
		t.Fatal("invalid --base created a path")
	}

	bare := initGitRepo(t)
	os.WriteFile(filepath.Join(bare, "README.md"), []byte("init\n"), fsperm.File)
	git(t, bare, "add", "README.md")
	git(t, bare, "commit", "-m", "init")
	base := strings.TrimSpace(git(t, bare, "rev-parse", "HEAD"))
	var out, errBuf bytes.Buffer
	if code := RunMigration(context.Background(), []string{"--root", bare, "--base", base}, &out, &errBuf); code != 0 || !strings.Contains(strings.ToLower(out.String()), "no migration files added") || !strings.Contains(out.String(), base) {
		t.Fatalf("empty exit message\n%s\n%s", out.String(), errBuf.String())
	}
}

func TestAdrGate(t *testing.T) {
	repo := initGitRepo(t)
	adr := filepath.Join(repo, "docs", "ADR")
	mustMkdir(t, adr)
	os.WriteFile(filepath.Join(adr, "000001.md"), []byte("---\ntitle: ADR 000001\n---\n# ADR-000001\n\nInitial.\n"), fsperm.File)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	os.WriteFile(filepath.Join(adr, "000002.md"), []byte("---\ntitle: ADR 000002\nsupersedes: [\"000001\"]\n---\n# ADR-000002\n\nReplaces [[000001]] because it is obsolete.\n"), fsperm.File)
	var out, errBuf bytes.Buffer
	code := RunAdr(context.Background(), []string{"--base", base, "--dir", adr}, &out, &errBuf)
	if code != 0 || !strings.Contains(out.String(), "Summary:") || !strings.Contains(out.String(), "000001") {
		t.Fatalf("pass exit %d\n%s\n%s", code, out.String(), errBuf.String())
	}

	os.WriteFile(filepath.Join(adr, "000002.md"), []byte("---\ntitle: ADR 000002\nsupersedes: [\"000001\"]\n---\n# ADR-000002\n\nMentions 000001 without wikilink.\n"), fsperm.File)
	out.Reset()
	errBuf.Reset()
	code = RunAdr(context.Background(), []string{"--base", base, "--dir", adr}, &out, &errBuf)
	want := "ADR 000002 adds supersedes: 000001 but its body never references [[000001]]"
	if code != 1 || !strings.Contains(out.String(), want) {
		t.Fatalf("finding exit %d\n%s\n%s", code, out.String(), errBuf.String())
	}

	cases := []struct {
		name string
		args []string
		want int
		tok  string
	}{
		{"unknown", []string{"--base", "nonexistent-revision-xyz", "--dir", adr}, 2, "unknown base revision"},
		{"dash", []string{"--base", "--stat", "--dir", adr}, 2, "invalid base revision"},
		{"missing", []string{"--dir", adr}, 2, "--base is required"},
		{"nodir", []string{"--base", base, "--dir", filepath.Join(repo, "nope")}, 2, "does not exist"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := RunAdr(context.Background(), tc.args, &stdout, &stderr)
			if code != tc.want || !strings.Contains(stdout.String()+stderr.String(), tc.tok) {
				t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
			}
		})
	}
}

func runPost(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := RunPostmortem(context.Background(), args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init")
	mustMkdir(t, filepath.Join(dir, "docs", "postmortems"))
	mustMkdir(t, filepath.Join(dir, "docs", "ADR"))
	return dir
}

func writePatterns(t *testing.T, repo string) {
	t.Helper()
	dir := filepath.Join(repo, ".claude", "skills", "failure-pattern-review", "references")
	mustMkdir(t, dir)
	os.WriteFile(filepath.Join(dir, "patterns.md"), []byte("## FP-01 失敗を成功・空・healthy に写す\n\n## FP-02 境界の片側だけを新契約にする\n"), fsperm.File)
	os.WriteFile(filepath.Join(repo, "docs", "ADR", "000100.md"), []byte("# ADR\n"), fsperm.File)
	os.WriteFile(filepath.Join(repo, "docs", "ADR", "000123.md"), []byte("# ADR\n"), fsperm.File)
	mustMkdir(t, filepath.Join(repo, "alt-backend", "app"))
	os.WriteFile(filepath.Join(repo, "alt-backend", "app", "handler.go"), []byte("package app\n"), fsperm.File)
}

func writeReadme(t *testing.T, repo string, entries [][2]string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("# Alt Postmortems\n\n## インシデント一覧\n\n")
	b.WriteString("| インシデント | 発生日 |\n|:---|:---:|\n")
	for _, entry := range entries {
		b.WriteString("| [" + entry[0] + "](" + entry[1] + ") | 2026-04-01 |\n")
	}
	os.WriteFile(filepath.Join(repo, "docs", "postmortems", "README.md"), []byte(b.String()), fsperm.File)
}

func validPM(id string, rows []string, pattern string) string {
	if rows == nil {
		rows = []string{
			"| 1 | 予防 | Add guard in `alt-backend/app/handler.go` | backend-team | 2026-05-01 | TODO |",
			"| 2 | 検知 | Add alert ([[000123]]) | obs-team | 2026-04-15 | 完了 |",
		}
	}
	if pattern == "" {
		pattern = "## 失敗の型と再発\n\n- 失敗の型: FP-01, FP-02\n- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: 契約テストの追加\n"
	}
	return "# " + id + ": Incident\n\n## 根本原因分析\n\nRoot cause text.\n\n" + pattern + "\n## アクションアイテム\n\n| # | カテゴリ | アクション | 担当 | 期限 | ステータス |\n|---|---|---|---|---|---|\n" + strings.Join(rows, "\n") + "\n"
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test Author",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test Committer",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
}
