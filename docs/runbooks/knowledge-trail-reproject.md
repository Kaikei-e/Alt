---
title: Knowledge Trail Reproject Runbook
date: 2026-07-18
status: accepted
tags:
  - runbook
  - knowledge-trail
  - projection
  - reproject
aliases:
  - knowledge-trail-reproject
---

# Knowledge Trail Reproject Runbook

Supersedes: [[knowledge-loop-reproject]] (Loop projections were DROPped by migration 00028)
Plan: `docs/plan/knowledge-trail-implementation-plan.md` (D22)

Full-reproject procedure for the Knowledge Trail read models
(`knowledge_trail_footprints` / `knowledge_trail_branches` /
`knowledge_trail_act_outcomes`, DB: knowledge_sovereign)。Use this when:

- `trailProjectionVersion` in
  `knowledge-sovereign/app/usecase/knowledge_trail_projector/projector.go` bumps
  (v2 = act_outcome side table joined the projection, 2026-07-18)
- projector fold rules change (verb mapping, outcome folding)
- a migration modifies the projection schema in a non-idempotent way

Wear-band 派生 (`engagedDwellMs` / CASE 式 in `read_trail.go`) の変更は
**reproject 不要** — wear は read 時導出で、生計測値から毎回再計算される。

## Pre-flight

1. Confirm the write side is healthy.
   ```sql
   SELECT COUNT(*), MAX(event_seq) FROM knowledge_events;
   ```
2. Confirm the checkpoint row exists.
   ```sql
   SELECT projector_name, last_event_seq, updated_at
   FROM knowledge_projection_checkpoints
   WHERE projector_name = 'knowledge-trail-projector';
   ```
3. Snapshot row counts for post-check comparison.
   ```sql
   SELECT 'footprints' AS t, COUNT(*) FROM knowledge_trail_footprints
   UNION ALL SELECT 'branches', COUNT(*) FROM knowledge_trail_branches
   UNION ALL SELECT 'act_outcomes', COUNT(*) FROM knowledge_trail_act_outcomes;
   ```

## Procedure

**PM-2026-010 invariant: reproject swap と checkpoint reset は不可分。**
TRUNCATE と checkpoint UPDATE は必ず同一トランザクションで行う。片方だけだと
「空のテーブル + 進んだ checkpoint」= 恒久的なデータ欠落になる。

1. Trail projector の tick を止める必要はない (in-process ticker は checkpoint
   基準で再開する) が、swap は 1 トランザクションで閉じること。
2. Inside a single transaction:
   ```sql
   BEGIN;
   TRUNCATE knowledge_trail_footprints;
   TRUNCATE knowledge_trail_branches;
   TRUNCATE knowledge_trail_act_outcomes;
   UPDATE knowledge_projection_checkpoints
   SET last_event_seq = 0, updated_at = now()
   WHERE projector_name = 'knowledge-trail-projector';
   COMMIT;
   ```
   (`knowledge_events` と dedupe registry には**絶対に**触らない —
   append-only の歴史と ingest-only barrier は disposable ではない。)
3. projector は次 tick から全歴史を再射影する。進捗は checkpoint の
   `last_event_seq` が `MAX(event_seq)` に追いつくことで確認する。

## Post-check

1. Row counts が pre-flight と一致すること (act_outcomes は v1→v2 初回は
   増える — 歴史 `knowledge_loop.act_outcome.v1` の初回取り込み)。
2. 決定性: 同一 log からの再実行で同一結果になることは
   `TestProjector_ReprojectIsDeterministic` /
   `TestProjector_ActOutcomeReplayIsDeterministic` が CI で保証。密度耐性
   (batch 境界での silent truncation) は
   `TestProjector_HighDensityReplayIsExactAtBatchBoundaries` が保証。
3. `/knowledge/trail` の spine が表示され、歴史 Loop 期イベント由来の
   footprint が残っていること (event log 永久保存の検証)。
