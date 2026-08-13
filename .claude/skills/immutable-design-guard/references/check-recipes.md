# Check Recipes — Language- and Layer-specific grep / ripgrep

違反候補を機械的に当てるためのレシピ集。SKILL.md ワークフロー Step 3 で使う。
**全部実行しない**。今回触れた言語 / 層だけ走らせる。

## Contents

- [共通: append-first 違反](#共通-append-first-違反)
- [共通: time.Now() / NOW() による業務時刻汚染](#共通-timenow-now-による業務時刻汚染)
- [共通: reproject-unsafe な「latest を読む」projector](#共通-reproject-unsafe-なlatest-を読むprojector)
- [共通: write path が read model を mutate](#共通-write-path-が-read-model-を-mutate)
- [SQL / migration: merge-safe 違反 / business logic in SQL](#sql-migration-merge-safe-違反-business-logic-in-sql)
- [proto / event payload schema](#proto-event-payload-schema)
- [共通: Versioned artifacts 違反](#共通-versioned-artifacts-違反)
- [共通: Single emission / idempotency](#共通-single-emission-idempotency)
- [Knowledge model 固有 (例)](#knowledge-model-固有-例)
- [レポートに含める情報](#レポートに含める情報)

`<root>` は対象サービスのアプリルート。イミュータブル知識モデルの本体は
`knowledge-sovereign/app/`（`handler` / `usecase` / `driver` がその直下、migration は
`knowledge-sovereign/migrations/`）。alt-db 側の migration は `migrations-atlas/migrations/`。
`alt-backend/app/` は層が `adapter` / `orchestrator` / `dataplane` / `shared` の下に入れ子なので、
`<root>/handler` 形式のパスは `alt-backend/app/*/usecase` のように読み替える。
ripgrep (`rg`) があれば優先、無ければ `grep -rn`。

## 共通: append-first 違反

```bash
# event store への UPDATE / DELETE
rg -n 'UPDATE\s+\S*event\S*\s+SET|DELETE\s+FROM\s+\S*event\S*' <root>

# event store table への "soft delete" 列 (<migrations> = knowledge-sovereign/migrations 等)
rg -n 'deleted_at|is_deleted' <migrations>
```

## 共通: time.Now() / NOW() による業務時刻汚染

```bash
# Go: projector / reproject / event handler 内の time.Now()
rg -n --type go '(time\.Now\(\)|time\.Since)' \
   <root> -g '*projector*' -g '*reproject*' -g '*event*handler*'

# Rust
rg -n --type rust '(Utc::now\(\)|SystemTime::now\(\))' \
   <root> -g '*projector*' -g '*reproject*'

# Python
rg -n --type py '(datetime\.now|datetime\.utcnow|time\.time\()' \
   <root> -g '*projector*' -g '*reproject*'

# SQL: projection 系 migration 内の DEFAULT NOW()
rg -n 'DEFAULT\s+NOW\(\)|DEFAULT\s+CURRENT_TIMESTAMP' \
   knowledge-sovereign/migrations migrations-atlas/migrations
```

ヒットしたら **business fact か debug-only `projected_at` か** を判定。

## 共通: reproject-unsafe な「latest を読む」projector

```bash
# projector が latest を取りに行く呼び出し
rg -n '(GetLatest|FindCurrent|FindActive|SELECT.*FROM.*projection)' \
   <root> -g '*projector*' -g '*usecase*projector*'

# projector 内で active read model に SELECT
rg -n 'SELECT.*FROM\s+\S*projection\S*|SELECT.*FROM\s+\S*read_model\S*' \
   <root> -g '*projector*'
```

## 共通: write path が read model を mutate

```bash
# handler / usecase が projection table を直接 UPDATE/INSERT
rg -n 'UPDATE\s+\S*projection\S*\s+SET|INSERT\s+INTO\s+\S*projection\S*' \
   <root>/handler <root>/usecase
```

## SQL / migration: merge-safe 違反 / business logic in SQL

```bash
# SQL CASE WHEN で business 判定
rg -n 'CASE\s+WHEN' <migrations> <root>/driver

# 引き算で負数になりうる UPDATE
rg -n 'SET\s+\w+\s*=\s*\w+\s*-\s*' <migrations> <root>/driver

# COALESCE 漏れ (UPSERT で EXCLUDED 値を上書きしている)
rg -n 'ON CONFLICT.*DO UPDATE' <migrations> <root>/driver | head -50
```

ヒットしたら `GREATEST(0, …)` / `COALESCE(EXCLUDED.x, table.x)` /
`WHERE EXCLUDED.seq > table.seq_hiwater` への置換を要求する。

## proto / event payload schema

```bash
# event payload に複数の意味の異なる時刻が同居していないか
rg -n 'occurred_at|recorded_at|received_at|completed_at|started_at' \
   proto/

# generic な XxxUpdated event (Property Sourcing 疑い)
rg -n 'message\s+\w+Updated\b|message\s+\w+Changed\b' proto/

# version_id / artifact_version_ref が event 側にあるか
rg -n 'version_id|artifact_version_ref' proto/
```

## 共通: Versioned artifacts 違反

```bash
# *_versions テーブルへの UPDATE (append-only であるべき)
rg -n 'UPDATE\s+\S*_versions?\s+SET' <root> <migrations>

# event payload が article_id だけで version_id を持たない
rg -n 'article_id\s*string' proto/ -A2 | rg -B1 -L 'version_id'
```

## 共通: Single emission / idempotency

```bash
# client_transition_id (or equivalent UUIDv7 idempotency key) 必須化されているか
rg -n 'client_transition_id|idempotency_key|ClientTransitionId' \
   <root>/handler <root>/usecase proto/

# 同一 RPC が複数 event を発行している箇所
rg -n 'PublishEvent|EmitEvent|AppendEvent' <root>/usecase | \
   awk -F: '{ print $1 }' | sort | uniq -c | sort -rn | head
```

2 連発以上を発行しているファイルは要検査。

## Knowledge model 固有 (例)

```bash
# knowledge_events への UPDATE / DELETE (INSERT-only であるべき)
rg -n 'UPDATE\s+knowledge_events|DELETE\s+FROM\s+knowledge_events' \
   knowledge-sovereign/app/ knowledge-sovereign/migrations/

# projector 内の wall clock
rg -n 'time\.Now\(\)' knowledge-sovereign/app/usecase/knowledge_home_projector/ \
   knowledge-sovereign/app/usecase/knowledge_trail_projector/

# summary_versions / tag_set_versions への UPDATE
rg -n 'UPDATE\s+(summary_versions|tag_set_versions)\s+SET' \
   alt-backend/app/ migrations-atlas/migrations/

# write path (handler) が read model を直接 mutate
rg -n 'UPDATE\s+(knowledge_home_items|today_digest_view|recall_candidate_view)|INSERT\s+INTO\s+knowledge_home_items' \
   knowledge-sovereign/app/handler
```

## レポートに含める情報

各ヒットについて:

- ファイル:行番号
- 当該行 (1-2 行)
- どの原則に該当しそうか (推定)
- false positive の可能性 (debug 用 / migration 一回限り 等)

最終判定 (high / medium / low) は SKILL.md の出力テンプレに従う。

[← back to SKILL.md](../SKILL.md)
