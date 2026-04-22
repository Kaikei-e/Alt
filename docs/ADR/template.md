---
title: タイトル（動詞始まりの行動指向で記述）
date: YYYY-MM-DD
status: proposed
tags:
  -
affected_services:
  -
aliases:
  - ADR-NNNN
  - ADR-000NNN
---
# ADR-000NNN: タイトル（動詞始まりの行動指向で記述）

<!-- 例: ADR-0033: singleflightパターンによるフィード登録の重複リクエスト排除 -->

## Status

<!-- Proposed | Accepted | Deprecated | Superseded by ADR-NNNN -->

Proposed

## Date

<!-- YYYY-MM-DD -->

## Affected Services

<!-- この決定の影響を受けるサービスを列挙する -->
<!-- 例: alt-backend (Go), pre-processor (Go), recap-worker (Rust) -->

-

## Context

<!-- なぜこの決定が必要になったのか。課題・背景・制約を記述する -->
<!-- 計測データがあれば含める（レイテンシ、メモリ使用量、エラー率など） -->

## Decision

<!-- 何を決めたか。選択肢がある場合は検討した代替案とその評価も記述する -->
<!-- 「〜を採用する」という明確な一文で決定を宣言すること -->

## Consequences

### Pros

<!-- この決定がもたらす良い影響 -->

### Cons / Tradeoffs

<!-- この決定がもたらすリスク・制約・技術的負債 -->

## Related ADRs

<!-- wikilink形式で記載。Obsidianのグラフビュー・バックリンクが自動生成される -->
<!-- 例: - [[000030]] PgBouncer導入 -->
<!-- 例: - [[000031]] Circuit Breaker適用 -->

- なし

## Deploy-model 整合性セルフチェック

<!--
本 ADR が採用する compose / CI / deploy パターンが、Alt の per-service
rolling deploy model と整合するかを確認する。詳細は [[000826]] / [[PM-2026-037]] 参照。
該当しない (ADR が compose や deploy pattern を触らない) 場合は「N/A」と明記。
-->

- [ ] 新設 compose service が `depends_on` で他 service を gate するか？
  - YES かつ condition が `service_completed_successfully`: rolling deploy (サービス単独 `docker compose up <svc>` 相当) で init が **起動されない**。[[PM-2026-037]] 参照。init 相当の責務を compose engine の fail-fast 挙動 (directory-scoped bind の missing-source refuse 等) / image の ENTRYPOINT / deploy tooling 側での明示起動、のいずれかに寄せる
  - YES かつ condition が `service_started` / `service_healthy`: 依存先が既に running なら rolling 下でも OK。依存先がまだ start していないケースの挙動を確認
- [ ] 新設 compose service に `restart: "no"` の one-shot init container を含むか？
  - YES: rolling deploy で一度も起動されない前提で再設計する
- [ ] 新設名前付き volume を populate する責務はどこにあるか？
  - init container 由来: 上記 rolling 非互換問題に当たる
  - 外部 (host bootstrap / baked image / CI artefact fetch): その経路を runbook で明文化し、prod host の pre-condition として確認できるようにする
- [ ] いずれも該当しない: 「rolling 互換」と明記して pass
