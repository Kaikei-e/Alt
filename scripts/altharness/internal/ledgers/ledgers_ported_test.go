package ledgers

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeADRFile(t *testing.T, repo, id, supersedesBlock, body string) {
	t.Helper()
	lines := []string{
		"---",
		"title: ADR " + id,
		"date: 2026-10-24",
		"status: accepted",
	}
	if supersedesBlock != "" {
		lines = append(lines, supersedesBlock)
	}
	lines = append(lines, "---", body)
	p := filepath.Join(repo, "docs", "ADR", id+".md")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
}

func TestAdrSupersedeGatePortedParallel(t *testing.T) {
	t.Parallel()

	t.Run("widened 6 to 60 produces 54 findings", func(t *testing.T) {
		t.Parallel()
		repo := initGitRepo(t)
		adrDir := filepath.Join(repo, "docs", "ADR")
		var baseIDs, allIDs []string
		for i := 1; i <= 60; i++ {
			id := fmt.Sprintf("%06d", i)
			writeADRFile(t, repo, id, "", "# ADR-"+id+"\n\nBody.")
			allIDs = append(allIDs, id)
			if i <= 6 {
				baseIDs = append(baseIDs, id)
			}
		}
		var refs, baseQuoted, allQuoted []string
		for _, id := range baseIDs {
			refs = append(refs, "[["+id+"]]")
			baseQuoted = append(baseQuoted, `"`+id+`"`)
		}
		for _, id := range allIDs {
			allQuoted = append(allQuoted, `"`+id+`"`)
		}
		namedBody := "# ADR-000940\n\nRetires " + strings.Join(refs, " ")
		writeADRFile(t, repo, "000940", "supersedes: ["+strings.Join(baseQuoted, ", ")+"]", namedBody)
		git(t, repo, "add", ".")
		git(t, repo, "commit", "-m", "base")
		base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

		writeADRFile(t, repo, "000940", "supersedes: ["+strings.Join(allQuoted, ", ")+"]", namedBody)
		var out, errB bytes.Buffer
		rc := RunAdr(context.Background(), []string{"--base", base, "--dir", adrDir}, &out, &errB)
		if rc != 1 {
			t.Fatalf("rc=%d out=%s err=%s", rc, out.String(), errB.String())
		}
		findings := 0
		hasSummary54 := false
		for _, ln := range strings.Split(out.String(), "\n") {
			if strings.HasPrefix(ln, "ADR 000940 adds supersedes:") {
				findings++
			}
			if strings.HasPrefix(ln, "Summary:") && strings.Contains(ln, "54") {
				hasSummary54 = true
			}
		}
		if findings != 54 || !hasSummary54 {
			t.Fatalf("findings=%d hasSummary54=%v\n%s", findings, hasSummary54, out.String())
		}
	})

	t.Run("existing edge at base produces 0 findings", func(t *testing.T) {
		t.Parallel()
		repo := initGitRepo(t)
		adrDir := filepath.Join(repo, "docs", "ADR")
		writeADRFile(t, repo, "000010", "", "# ADR-000010\n\nOld.")
		writeADRFile(t, repo, "000011", `supersedes: ["000010"]`, "# ADR-000011\n\nNo wikilink.")
		git(t, repo, "add", ".")
		git(t, repo, "commit", "-m", "base")
		base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

		writeADRFile(t, repo, "000011", `supersedes: ["000010"]`, "# ADR-000011\n\nEdited body.")
		var out, errB bytes.Buffer
		rc := RunAdr(context.Background(), []string{"--base", base, "--dir", adrDir}, &out, &errB)
		if rc != 0 || !strings.Contains(out.String(), "Summary:") || !strings.Contains(out.String(), "0") {
			t.Fatalf("rc=%d out=%s err=%s", rc, out.String(), errB.String())
		}
	})

	t.Run("inline and block list forms and piped wikilink", func(t *testing.T) {
		t.Parallel()
		repo := initGitRepo(t)
		adrDir := filepath.Join(repo, "docs", "ADR")
		for i := 1; i <= 9; i++ {
			id := fmt.Sprintf("%06d", i)
			writeADRFile(t, repo, id, "", "# ADR-"+id+"\n\nBase.")
		}
		git(t, repo, "add", ".")
		git(t, repo, "commit", "-m", "base")
		base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

		writeADRFile(t, repo, "000100", `supersedes: [000001, "000002", "[[000003]]", [[000004]]]`, "# ADR-000100\n\nReplaces [[000001]], [[000002]], [[000003|ADR-3]].")
		writeADRFile(t, repo, "000101", "supersedes:\n  - 000005\n  - \"000006\"\n  - \"[[000007]]\"\n  - [[000008]]", "# ADR-000101\n\nReplaces [[000005]], [[000006]], [[000007]].")
		var out, errB bytes.Buffer
		rc := RunAdr(context.Background(), []string{"--base", base, "--dir", adrDir}, &out, &errB)
		var lines []string
		for _, ln := range strings.Split(out.String(), "\n") {
			if strings.Contains(ln, " adds supersedes: ") {
				lines = append(lines, ln)
			}
		}
		if rc != 1 || len(lines) != 2 || !strings.Contains(out.String(), "000100 adds supersedes: 000004") || !strings.Contains(out.String(), "000101 adds supersedes: 000008") {
			t.Fatalf("rc=%d lines=%v\nout=%s", rc, lines, out.String())
		}
	})
}

func TestMigrationDestructiveGatePortedParallel(t *testing.T) {
	t.Parallel()

	t.Run("extended positive and negative SQL probes", func(t *testing.T) {
		t.Parallel()
		positives := []struct {
			name string
			sql  string
		}{
			{"DO IF EXISTS WHERE then UPDATE NULL", "DO $$ BEGIN IF EXISTS (SELECT 1 FROM pg_tables WHERE tablename='x') THEN UPDATE rag_chunks SET embedding = NULL; END IF; END $$;"},
			{"DELETE USING subquery without top WHERE", "DELETE FROM t USING (SELECT 1 FROM u WHERE u.a=1) s;"},
			{"tagged dollar quote", "COMMENT ON TABLE t IS $msg$it's a table$msg$; DROP TABLE users;"},
			{"ALTER TYPE USING NULL no COLUMN", "ALTER TABLE t ALTER c TYPE text USING NULL;\n"},
			{"USING parenthesized NULL", "ALTER TABLE rag_chunks ALTER COLUMN embedding TYPE vector(1024) USING (NULL::vector(1024));\n"},
			{"DROP col no COLUMN", "ALTER TABLE t DROP c;\n"},
			{"DROP DATABASE", "DROP DATABASE legacy_db;\n"},
			{"DROP VIEW CASCADE", "DROP VIEW old_view CASCADE;\n"},
			{"ClickHouse DROP PARTITION", "ALTER TABLE logs DROP PARTITION '202601';\n"},
			{"ClickHouse ALTER DELETE no WHERE", "ALTER TABLE logs DELETE;\n"},
			{"ok-comment in string literal", "SELECT '\n-- destructive-ok: string literal must not excuse anything at all\n';\nDROP TABLE temp_one;\n"},
		}
		for _, tc := range positives {
			if got := checkMigrationText("migrations/x.sql", tc.sql); len(got) == 0 {
				t.Fatalf("positive %s produced 0 findings", tc.name)
			}
		}
		negatives := []struct {
			name string
			sql  string
		}{
			{"DROP DEFAULT", "ALTER TABLE t ALTER COLUMN c DROP DEFAULT;\n"},
			{"DROP NOT NULL", "ALTER TABLE t ALTER c DROP NOT NULL;\n"},
			{"DROP EXPRESSION", "ALTER TABLE t ALTER COLUMN c DROP EXPRESSION IF EXISTS;\n"},
			{"DROP IDENTITY", "ALTER TABLE t ALTER c DROP IDENTITY IF EXISTS;\n"},
			{"DROP INDEX", "DROP INDEX IF EXISTS idx_old;\n"},
			{"DROP VIEW", "DROP VIEW IF EXISTS v_old;\n"},
		}
		for _, tc := range negatives {
			if got := checkMigrationText("migrations/x.sql", tc.sql); len(got) != 0 {
				t.Fatalf("negative %s produced findings: %v", tc.name, got)
			}
		}
	})

	t.Run("existing at base, non-migration sql, and untracked sql ignored", func(t *testing.T) {
		t.Parallel()
		repo := initGitRepo(t)
		oldRel := "rag-migration-atlas/migrations/20260802_old.sql"
		mustMkdir(t, filepath.Join(repo, filepath.Dir(oldRel)))
		_ = os.WriteFile(filepath.Join(repo, oldRel), []byte("ALTER TABLE rag_chunks ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);\n"), fsperm.File)
		git(t, repo, "add", oldRel)
		git(t, repo, "commit", "-m", "historical")
		base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

		// Touch existing migration + add non-migration sql + leave untracked migration.
		_ = os.WriteFile(filepath.Join(repo, oldRel), []byte("ALTER TABLE rag_chunks ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);\n-- note\n"), fsperm.File)
		mustMkdir(t, filepath.Join(repo, "scripts", "sql"))
		_ = os.WriteFile(filepath.Join(repo, "scripts", "sql", "cleanup.sql"), []byte("DROP TABLE temp_cache;\n"), fsperm.File)
		git(t, repo, "add", oldRel, "scripts/sql/cleanup.sql")
		git(t, repo, "commit", "-m", "touch old and add non-mig")

		untracked := filepath.Join(repo, "migrations-atlas", "migrations", "999999_untracked.sql")
		mustMkdir(t, filepath.Dir(untracked))
		_ = os.WriteFile(untracked, []byte("DROP TABLE untracked;\n"), fsperm.File)

		var out, errB bytes.Buffer
		if rc := RunMigration(context.Background(), []string{"--base", base, "--root", repo}, &out, &errB); rc != 0 {
			t.Fatalf("rc=%d out=%s err=%s", rc, out.String(), errB.String())
		}
	})

	t.Run("real repository smoke on HEAD exits 0", func(t *testing.T) {
		t.Parallel()
		root := findLedgerRepo(t)
		var out, errB bytes.Buffer
		if rc := RunMigration(context.Background(), []string{"--base", "HEAD", "--root", root}, &out, &errB); rc != 0 {
			t.Fatalf("real repo smoke rc=%d out=%s err=%s", rc, out.String(), errB.String())
		}
	})
}

func TestPostmortemGateAndLedgerPortedParallel(t *testing.T) {
	t.Parallel()

	runSingleViolation := func(t *testing.T, pmContent *string, readmeEntries [][2]string) (int, string, string) {
		t.Helper()
		repo := initGitRepo(t)
		writePatterns(t, repo)
		mustMkdir(t, filepath.Join(repo, "backend", "frontend"))
		_ = os.WriteFile(filepath.Join(repo, "backend", "frontend", "sre"), []byte("file\n"), fsperm.File)
		basePM := "PM-2026-001-base.md"
		_ = os.WriteFile(filepath.Join(repo, "docs", "postmortems", basePM), []byte(validPM("PM-2026-001", nil, "")), fsperm.File)
		writeReadme(t, repo, [][2]string{{"PM-2026-001", basePM}})
		git(t, repo, "add", ".")
		git(t, repo, "commit", "-m", "base")
		base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

		entries := [][2]string{{"PM-2026-001", basePM}}
		if pmContent != nil {
			newPM := "PM-2026-002-new.md"
			_ = os.WriteFile(filepath.Join(repo, "docs", "postmortems", newPM), []byte(*pmContent), fsperm.File)
			entries = append(entries, [2]string{"PM-2026-002", newPM})
		}
		if readmeEntries != nil {
			entries = readmeEntries
		}
		writeReadme(t, repo, entries)
		git(t, repo, "add", ".")
		git(t, repo, "commit", "-m", "change")
		return runPost(t, []string{"--root", repo, "--check", "--base", base})
	}

	strPtr := func(s string) *string { return &s }

	t.Run("P1 to P12 gate violations", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name    string
			content *string
			readme  [][2]string
			wantID  string
		}{
			{"P1 missing table", strPtr("# PM-2026-002: Missing\n\n## 失敗の型と再発\n\n- 失敗の型: FP-01\n- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: test\n\n## アクションアイテム\n\nNo table.\n"), nil, "P1"},
			{"P1 checklist instead of table", strPtr("# PM-2026-002: Checklist\n\n## 失敗の型と再発\n\n- 失敗の型: FP-01\n- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: test\n\n## アクションアイテム\n\n### 予防（Prevent）\n\n- [ ] Fix — **担当: alice、期限: 2026-05-01**\n"), nil, "P1"},
			{"P1 unmappable table", strPtr("# PM-2026-002: Unmap\n\n## 根本原因分析\n\nRoot.\n\n## 失敗の型と再発\n\n- 失敗の型: FP-01\n- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: test\n\n## アクションアイテム\n\n| 項目 | 値 |\n|---|---|\n| foo | bar |\n"), nil, "P1"},
			{"P2 TBD owner", strPtr(validPM("PM-2026-002", []string{"| 1 | 予防 | Do something | TBD | 2026-05-01 | TODO |"}, "")), nil, "P2"},
			{"P3 non-ISO due", strPtr(validPM("PM-2026-002", []string{"| 1 | 予防 | Do something | alice | 来月 | TODO |"}, "")), nil, "P3"},
			{"P4 non-standard status", strPtr(validPM("PM-2026-002", []string{"| 1 | 予防 | Do something | alice | 2026-05-01 | 未着手 |"}, "")), nil, "P4"},
			{"P6 empty 持ち越し", strPtr(validPM("PM-2026-002", nil, "## 失敗の型と再発\n\n- 失敗の型: FP-01\n- 再発: なし\n- 持ち越し: \n- 止められたはずの検査: test\n")), nil, "P6"},
			{"P6 missing section", strPtr("# PM-2026-002: Missing FP\n\n## 根本原因分析\n\nRoot.\n\n## アクションアイテム\n\n| # | カテゴリ | アクション | 担当 | 期限 | ステータス |\n|---|---|---|---|---|---|\n| 1 | 予防 | Do | alice | 2026-05-01 | TODO |\n"), nil, "P6"},
			{"P6 wrong position", strPtr("# PM-2026-002: Wrong pos\n\n## 根本原因分析\n\nRoot.\n\n## 対応の評価\n\nEval.\n\n## 失敗の型と再発\n\n- 失敗の型: FP-01\n- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: test\n\n## アクションアイテム\n\n| # | カテゴリ | アクション | 担当 | 期限 | ステータス |\n|---|---|---|---|---|---|\n| 1 | 予防 | Do | alice | 2026-05-01 | TODO |\n"), nil, "P6"},
			{"P7 unknown FP-99", strPtr(validPM("PM-2026-002", nil, "## 失敗の型と再発\n\n- 失敗の型: FP-99\n- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: test\n")), nil, "P7"},
			{"P7 prose without 型の外", strPtr(validPM("PM-2026-002", nil, "## 失敗の型と再発\n\n- 失敗の型: よく分からない\n- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: test\n")), nil, "P7"},
			{"P7 short 型の外", strPtr(validPM("PM-2026-002", nil, "## 失敗の型と再発\n\n- 失敗の型: 型の外: 短い\n- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: test\n")), nil, "P7"},
			{"P8 missing PM in 再発", strPtr(validPM("PM-2026-002", nil, "## 失敗の型と再発\n\n- 失敗の型: FP-01\n- 再発: PM-2026-999\n- 持ち越し: なし\n- 止められたはずの検査: test\n")), nil, "P8"},
			{"P8 missing PM in 持ち越し", strPtr(validPM("PM-2026-002", nil, "## 失敗の型と再発\n\n- 失敗の型: FP-01\n- 再発: なし\n- 持ち越し: PM-2026-999\n- 止められたはずの検査: test\n")), nil, "P8"},
			{"P9 README missing PM", strPtr(validPM("PM-2026-002", nil, "")), [][2]string{{"PM-2026-001", "PM-2026-001-base.md"}}, "P9"},
			{"P10 README nonexistent PM", nil, [][2]string{{"PM-2026-001", "PM-2026-001-base.md"}, {"PM-2026-999", "PM-2026-999-ghost.md"}}, "P10"},
			{"P12 破棄 without reason", strPtr(validPM("PM-2026-002", []string{"| 1 | 予防 | Drop this item | alice | 2026-05-01 | 破棄 |"}, "")), nil, "P12"},
			{"P12 破棄 short reason", strPtr(validPM("PM-2026-002", []string{"| 1 | 予防 | Drop item 破棄理由: 不要 | alice | 2026-05-01 | 破棄 |"}, "")), nil, "P12"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				rc, out, _ := runSingleViolation(t, tc.content, tc.readme)
				if rc != 1 || !strings.Contains(out, tc.wantID) {
					t.Fatalf("%s: rc=%d out=%s", tc.name, rc, out)
				}
			})
		}
	})

	t.Run("P5 accidental vs accepted evidence", func(t *testing.T) {
		t.Parallel()
		accidental := [][2]string{
			{"2026/05/01 に対応済み", "alice"},
			{"Go/Python/Rust の全サービスで確認", "alice"},
			{"p50/p95/p99 を計測した", "alice"},
			{"`application/json` を返す", "alice"},
			{"1500000 件を再処理した", "alice"},
			{"20260501 に確認", "alice"},
			{"550e8400-e29b-41d4-a716-446655440000 を処理", "alice"},
			{"デプロイは #1 に含む", "alice"},
			{"色を #333 に統一", "alice"},
			{"`does/not/exist.go` を修正", "alice"},
			{"[[999999]] で対応", "alice"},
			{"対応完了", "backend/frontend/sre"},
		}
		for _, pair := range accidental {
			rc, out, _ := runSingleViolation(t, strPtr(validPM("PM-2026-002", []string{"| 1 | 予防 | " + pair[0] + " | " + pair[1] + " | 2026-05-01 | 完了 |"}, "")), nil)
			if rc != 1 || !strings.Contains(out, "P5") {
				t.Fatalf("accidental %q should fail P5: rc=%d out=%s", pair[0], rc, out)
			}
		}

		accepted := [][2]string{
			{"PR #286で修正", "完了"},
			{"PR#286 で修正", "完了"},
			{"https://example.com/org/repo/pull/123 で修正", "完了"},
			{"コミット1c7679801で修正", "完了"},
			{"[[000123]] で対応", "完了"},
			{"`alt-backend/app/handler.go` を修正", "完了"},
			{"PR #286で修正", "**完了**"},
		}
		for _, pair := range accepted {
			rc, out, errText := runSingleViolation(t, strPtr(validPM("PM-2026-002", []string{"| 1 | 予防 | " + pair[0] + " | alice | 2026-05-01 | " + pair[1] + " |"}, "")), nil)
			if rc != 0 {
				t.Fatalf("accepted %q should pass: rc=%d out=%s err=%s", pair[0], rc, out, errText)
			}
		}
	})

	t.Run("rename counts as added and malformed existing ignored on valid new PM", func(t *testing.T) {
		t.Parallel()
		repo := initGitRepo(t)
		writePatterns(t, repo)
		oldPM := "PM-2026-001-old.md"
		_ = os.WriteFile(filepath.Join(repo, "docs", "postmortems", oldPM), []byte("# PM-2026-001: Old\n\nProse.\n"), fsperm.File)
		writeReadme(t, repo, [][2]string{{"PM-2026-001", oldPM}})
		git(t, repo, "add", ".")
		git(t, repo, "commit", "-m", "base")
		base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

		newPM := "PM-2026-001-renamed.md"
		git(t, repo, "mv", "docs/postmortems/"+oldPM, "docs/postmortems/"+newPM)
		writeReadme(t, repo, [][2]string{{"PM-2026-001", newPM}})
		git(t, repo, "add", ".")
		git(t, repo, "commit", "-m", "rename")
		rc, out, _ := runPost(t, []string{"--root", repo, "--check", "--base", base})
		if rc != 1 || !strings.Contains(out, "P1") {
			t.Fatalf("rename should fail P1: rc=%d out=%s", rc, out)
		}
	})

	t.Run("filters, checklist, mixed table+checklist, and short carried PM-NNN", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		mustMkdir(t, filepath.Join(repo, "docs", "postmortems"))
		_ = os.WriteFile(filepath.Join(repo, "docs", "postmortems", "PM-2026-028-checklist.md"), []byte(
			"# PM-2026-028: Checklist\n\n"+
				"## アクションアイテム\n\n"+
				"### 予防（Prevent）\n\n"+
				"- [x] **[Platform] Done item** — 完了: 2026-04-16\n"+
				"- [ ] **[Platform] Open item** — 何かを直す。**担当: platform、期限: 2026-04-30**\n\n"+
				"### 検知 (Detect)\n\n"+
				"- [X] **[Platform] Uppercase X** — 完了: 2026-04-17\n"+
				"- [ ] Wrapped multiline\n"+
				"  detail — **担当:** recap-worker\n"+
				"  owner、**期限:** 2026-05-15。\n",
		), fsperm.File)
		_ = os.WriteFile(filepath.Join(repo, "docs", "postmortems", "PM-2026-036-follow.md"), []byte(
			"# PM-2026-036: Follow\n\n"+
				"## アクションアイテム\n\n"+
				"| # | カテゴリ | アクション | 担当 | 期限 | 状態 |\n"+
				"|---|---|---|---|---|---|\n"+
				"| 1 | 予防 | PM-028 から継承した対策 | alice | 2026-04-19 〜継続 | TODO |\n"+
				"| 2 | 検知 | 監視追加 | bob | 2026-09-08（本番反映） | 保留 |\n"+
				"| 3 | 緩和 | Self ref PM-036 only | carol | 2026-12-01 | TODO |\n",
		), fsperm.File)

		rc, txt, _ := runPost(t, []string{"--root", repo})
		if rc != 0 || !strings.Contains(txt, "postmortems") || (!strings.Contains(strings.ToLower(txt), "lag") && !strings.Contains(txt, "乖離")) {
			t.Fatalf("text output rc=%d out=%s", rc, txt)
		}

		rc, carOut, _ := runPost(t, []string{"--root", repo, "--carried", "--json"})
		var parsed struct {
			Rows []struct {
				PostmortemID     string `json:"postmortem_id"`
				ItemID           string `json:"item_id"`
				Owner            string `json:"owner"`
				Due              string `json:"due"`
				NormalisedStatus string `json:"normalised_status"`
			} `json:"rows"`
		}
		_ = json.Unmarshal([]byte(carOut), &parsed)
		if rc != 0 || len(parsed.Rows) != 1 || parsed.Rows[0].PostmortemID != "PM-2026-036" || parsed.Rows[0].ItemID != "1" {
			t.Fatalf("--carried rc=%d rows=%+v", rc, parsed.Rows)
		}

		rc, matchOut, _ := runPost(t, []string{"--root", repo, "--match", "recap-worker", "--json"})
		parsed.Rows = nil
		_ = json.Unmarshal([]byte(matchOut), &parsed)
		if rc != 0 || len(parsed.Rows) != 1 || parsed.Rows[0].Owner != "recap-worker owner" || parsed.Rows[0].Due != "2026-05-15" {
			t.Fatalf("--match wrapped checklist rc=%d rows=%+v", rc, parsed.Rows)
		}
	})

	t.Run("real repository ledger pinned counts", func(t *testing.T) {
		t.Parallel()
		root := findLedgerRepo(t)
		rc, out, errText := runPost(t, []string{"--root", root, "--json"})
		if rc != 0 {
			t.Fatalf("real repo ledger rc=%d err=%s", rc, errText)
		}
		var data struct {
			Rows []struct {
				PostmortemID     string `json:"postmortem_id"`
				NormalisedStatus string `json:"normalised_status"`
			} `json:"rows"`
			Summary struct {
				Postmortems int `json:"postmortems"`
				Rows        int `json:"rows"`
				Unparsed    int `json:"unparsed"`
			} `json:"summary"`
		}
		if err := json.Unmarshal([]byte(out), &data); err != nil {
			t.Fatal(err)
		}
		if data.Summary.Unparsed != 0 || data.Summary.Rows == 0 {
			t.Fatalf("unexpected real summary: %+v", data.Summary)
		}
		pinned := map[string][3]int{
			"PM-2026-024": {13, 4, 9},
			"PM-2026-028": {16, 11, 5},
			"PM-2026-029": {10, 4, 6},
		}
		for pmID, exp := range pinned {
			var total, done, open int
			for _, r := range data.Rows {
				if r.PostmortemID == pmID {
					total++
					switch r.NormalisedStatus {
					case "done":
						done++
					case "open":
						open++
					}
				}
			}
			if [3]int{total, done, open} != exp {
				t.Fatalf("%s got (%d,%d,%d) want %v", pmID, total, done, open, exp)
			}
		}
	})
}

func findLedgerRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "altharness", "alt-harness")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}
