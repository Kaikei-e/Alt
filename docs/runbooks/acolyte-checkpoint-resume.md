---
title: Acolyte パイプラインの checkpoint resume 手順
date: 2026-04-10
tags:
  - acolyte
  - operations
  - checkpoint
---

# Acolyte checkpoint resume 手順

## 概要

Acolyte パイプラインは LangGraph の Postgres checkpointer を使い、super-step 境界で state を永続化する。基本は node 間保存だが、checkpointer 有効時の QuoteSelector / FactNormalizer は incremental self-loop に分割されるため、article / quote 単位でも途中成果が保存される。パイプラインが中断した場合、同じ `run_id` で resume することで、完了済み step をスキップし、失敗した地点から再開できる。

## 前提条件

- `CHECKPOINT_ENABLED=true` が設定されていること
- `acolyte-db` が稼働し、checkpoint テーブルが存在すること（初回起動時に `setup()` で自動作成）
- resume 対象の `run_id` が既知であること

## checkpoint の仕組み

### 保存タイミング

LangGraph は **super-step 境界**で checkpoint を保存する。Acolyte では QuoteSelector / FactNormalizer だけは self-loop 化されているため、1 node = 1 article / 1 quote 相当の粒度で保存される。

```
planner → [checkpoint] → gatherer → [checkpoint] → curator → [checkpoint] →
hydrator → [checkpoint] → compressor → [checkpoint] →
quote_selector(item 1) → [checkpoint] → quote_selector(item 2) → [checkpoint] → ... →
fact_normalizer(quote 1) → [checkpoint] → fact_normalizer(quote 2) → [checkpoint] → ... →
section_planner → [checkpoint] →
writer → [checkpoint] → critic → [checkpoint] → finalizer → [checkpoint] → END
```

### thread_id

各 run は `acolyte-run:{run_id}` という thread_id で一意に識別される。同じ run_id で resume すると、同じ thread_id の checkpoint から state を復元する。

### durability

`durability="sync"` が設定されており、各 super-step の完了時に checkpoint が確実に Postgres に書き込まれてから次の step に進む。

## resume 手順

### 1. 障害の確認

```bash
# ログで失敗した run_id を特定
docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator --tail=100 | grep "Pipeline crashed\|Pipeline failed"

# run の状態を確認（DB 直接クエリ）
docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c \
  "SELECT run_id, report_id, status, created_at FROM runs WHERE run_id = '<run_id>';"
```

### 2. DB の健全性確認

```bash
# acolyte-db が稼働していることを確認
docker compose -f compose/compose.yaml -p alt ps acolyte-db

# checkpoint テーブルの存在確認
docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c \
  "SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename LIKE 'checkpoint%';"
```

### 3. resume 実行

```bash
# コンテナ内から resume script を実行
docker exec -it acolyte-orchestrator \
  python scripts/resume_run.py --run-id <run_id>
```

### 4. 結果の確認

```bash
# ログで resume 結果を確認
docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator --tail=50

# "Resuming pipeline from checkpoint" → resume が発動
# "Pipeline completed" + final_version → 成功
# "Pipeline crashed" → 再度失敗（原因調査が必要）
```

## 障害パターン別の対処

| パターン | 症状 | 対処 |
|---------|------|------|
| LLM タイムアウト | `Pipeline crashed` + `ReadTimeout` | LLM (news-creator / AIX) の稼働確認後に resume |
| OOM | コンテナ再起動 + `Pipeline crashed` なし | `docker compose up -d acolyte-orchestrator` 後に resume |
| DB 接続断 | `Pipeline crashed` + `ConnectionError` | `acolyte-db` 復旧確認後に resume |
| 完了済み run | ログに `Pipeline already completed` | resume は no-op。新規 run を作成する |

## 制約事項

### Writer の mid-node 途中再開は不可

QuoteSelector / FactNormalizer は checkpointer 有効時に incremental self-loop へ分割されるため、処理済み article / quote は保持されたまま resume できる。いま mid-node 途中再開ができないのは Writer で、section / paragraph ループの途中で失敗すると writer node を頭からやり直す。

影響のある node:
- **WriterNode** — 全 section x paragraph のループ

### resume は replay

LangGraph の resume は「同じ行から継続」ではなく、「最後の成功 super-step の次の node から再実行」である。失敗した node は再度呼び出される。

### side effects と idempotency

Finalizer は DB に report version を書き込む。同じ run を複数回 resume すると、同じ version が重複して書き込まれる可能性がある。現在の実装では version_no は monotonic increment なので実害はないが、注意が必要。

### checkpoint テーブル

checkpoint テーブルは LangGraph の `setup()` が自動作成・管理する。Atlas migration の対象外。テーブル名は `checkpoints`, `checkpoint_blobs`, `checkpoint_writes`。
