# Knowledge Home — Context Glossary

`/home` を surface とするダッシュボード型境界。Today's Digest と Recall candidate を束ね、
append-only の `knowledge_events` から disposable read model へ投影する。
knowledge-sovereign / alt-backend / alt-frontend-sv にまたがる。

> Knowledge Loop → Knowledge Trail の置換 (2026-06-10) とは別系統。Home はその前から独立に存在し、
> 現役継続する。両者は `knowledge_events` を共有するが、read model・projector・語彙は独立。

## Language

**Home item**:
`knowledge_home_items` に投影された、Home 上の 1 件の表示単位。event payload と安定 resource のみから
構築され、latest state を参照しない。
_Avoid_: feed item, card (実装詳細)

**Today's Digest**:
`today_digest_view` が保持する、当日分の Home item の日次まとめ。

**Recall candidate**:
`recall_candidate_view` が保持する、再想起のために再浮上させる対象。

**Why (why_json)**:
Home item が surface された理由の必須随伴データ。code 単位でマージし、既存の why を削除しない。
「Why as first-class」— why を持たない item は Home の不変条件違反。

**Projector**:
`knowledge_events` を fold して read model を構築・更新するロジック。reproject-safe (event payload と
安定 resource のみ、latest state 参照禁止) が必須。現在 alt-backend / knowledge-sovereign に分裂して
存在する (F-01 で sovereign への re-own を計画中)。
_Avoid_: sync job (責務を矮小化する呼び方)

**Projection checkpoint**:
projector がイベントログのどこまで処理したかを保持する状態。heartbeat 更新により freshness 指標を提供する。

**Reproject**:
read model を event log から再構築すること。決定的 (同じイベント列から同じ結果) でなければならない。

**Supersede**:
summary/tag の新版が旧版を置き換えること。旧版の why 履歴は保持し、削除しない。

**Summary state**:
`missing → pending → ready` の一方向遷移。`ready` からの逆行は禁止。

## Anti-terms

**Latest state read**:
projector が read model や外部サービスの「今の状態」を問い合わせて fold すること。event payload と
安定 resource 以外からの入力は reproject-safe を壊す。
