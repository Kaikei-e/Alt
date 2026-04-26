-- Knowledge Loop entries に Row Level Security を有効化する。
--
-- 前提: Wave 4-D Phase 1 の caller-first session-var 配線が landed
-- し、本番ログで `alt_user_id_set=false` の read が 24h ゼロ件である
-- ことを確認してから投入すること。逆順で投入すると、SET LOCAL を
-- 持たない read 経路が `current_setting('alt.user_id', true)` で NULL
-- を引き、policy が false に倒れて全行不可視 → UI が空白になる。
--
-- 設計参照:
--   - canonical contract §3.10 / §11 (F-001 cross-user evidence_refs leak)
--   - docs/plan/knowledge-loop-wave4-remaining-tasks.md §3
--   - knowledge-sovereign/app/driver/sovereign_db/with_user_context.go
--
-- 切り戻し:
--   DROP POLICY knowledge_loop_entries_user_isolation ON knowledge_loop_entries;
--   ALTER TABLE knowledge_loop_entries DISABLE ROW LEVEL SECURITY;
-- 上記 2 文で即座に元に戻る (caller-first 配線は無害な状態で残るので
-- 後続の rollback で別途対処する)。

ALTER TABLE knowledge_loop_entries ENABLE ROW LEVEL SECURITY;

CREATE POLICY knowledge_loop_entries_user_isolation
  ON knowledge_loop_entries
  USING (user_id::text = current_setting('alt.user_id', true));

COMMENT ON POLICY knowledge_loop_entries_user_isolation ON knowledge_loop_entries IS
  'F-001 defense-in-depth. Each session must SET LOCAL alt.user_id = $user_id before reading.';
