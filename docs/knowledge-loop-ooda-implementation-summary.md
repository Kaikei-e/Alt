# Knowledge Loop OODA 実装まとめ

**実施日**: 2026-05-02  
**対象**: `knowledge-sovereign`, `alt-frontend-sv`, `alt-backend`, `compose`

## 概要

Knowledge Loop の `Continue` / `Changed` / `Review` がプレースホルダーに見えていた問題を、投影データの補強と UI の OODA 化の両方から解消した。

DB 確認では、`continue` / `changed` / `review` の行数自体は存在していたが、`continue_context`, `change_summary`, `source_observed_at` が全 bucket で未投影だった。加えて `surface_planner_v2` の checkpoint が大きく遅れており、`knowledge_loop.surface_plan_recomputed.v1` が発火していなかった。

## 変更内容

### 1. Knowledge Sovereign の投影を補強

- `HomeItemOpened` で `continue_context` を生成し、`source_observed_at` も event payload から埋めるようにした。
- `HomeItemDismissed` の why を、単一の固定文言ではなく、dismiss 時刻と evidence を含む文脈付きの文にした。
- `SummarySuperseded` の payload が旧 excerpt しか持たない場合でも、`change_summary` が空にならないようにした。
- replay-safe を維持するため、DB 参照ではなく event payload と既存 artifact だけから JSONB を組み立てる形にした。

### 2. Surface Planner の catch-up を追加

- `surface_planner_v2` に `MaxBatchesPerTick` を追加し、1 tick で複数 batch を追えるようにした。
- `KNOWLEDGE_SOVEREIGN_PLANNER_BATCH_SIZE` と `KNOWLEDGE_SOVEREIGN_PLANNER_MAX_BATCHES_PER_TICK` を compose から設定できるようにした。
- staging では catch-up を優先する値を入れ、本番では高めの既定値を与えた。

### 3. `/loop` の UX を OODA workspace に変更

- `OodaPipeline` をクリック可能な stage controller にした。
- `Now` の選択 entry に対して、workspace を追加して Observe / Orient / Decide / Act を明示化した。
- `Continue` / `Changed` は単なるカード一覧ではなく、次の操作が分かる文脈表示を持つようにした。
- `Review` の Open を foreground と同じ SPA reader 経路に統一した。

### 4. 継続している設計ルール

- Clean Architecture の境界は維持した。
- read model は UI から直接更新していない。
- projection は event-time 基準のままにした。
- 既存 proto を優先し、新しい schema は追加しなかった。

## DB で見えた事実

- `Continue` visible: 7
- `Changed` visible: 10,035
- `Review` visible: 522
- `continue_context` / `change_summary` / `source_observed_at`: いずれも未投影
- `surface_planner_v2` checkpoint: 大きく遅延
- `knowledge_loop.surface_plan_recomputed.v1`: 0 件

## テスト

- `go test ./...` in `knowledge-sovereign/app`
- `go test ./...` in `alt-backend/app`
- `vitest run` in `alt-frontend-sv` 全件
- `svelte-check --tsgo --tsconfig ./tsconfig.json`
- `vite build`
- `biome lint` / `git diff --check`

## 運用メモ

- 既存行へ新しい `continue_context` / `change_summary` を反映するには、reproject を実行する必要がある。
- planner lag を縮めるには、必要に応じて `KNOWLEDGE_SOVEREIGN_PLANNER_BATCH_SIZE` と `KNOWLEDGE_SOVEREIGN_PLANNER_MAX_BATCHES_PER_TICK` を上げる。
