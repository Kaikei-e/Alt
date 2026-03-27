# PM-2026-010: Knowledge Home V3 Reproject チェックポイントギャップによる summary_state 不整合

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-4（ユーザー体験劣化・データ不整合、サービス全面停止なし） |
| 影響期間 | 2026-03-23 07:37 〜 2026-03-27 07:12（約 3 日 23 時間） |
| 影響サービス | alt-backend（KnowledgeProjector）、knowledge-sovereign |
| 影響機能 | Knowledge Home のフィード表示、記事要約ステータス |
| 関連 ADR | — |
| 関連 PM | [[PM-2026-002-unsummarized-infinite-enqueue-loop]] |

## サマリー

Knowledge Home で特定の記事が "Summarizing" のまま更新されない問題が報告された。`article_summaries` テーブルには要約が正常に保存済みであり、`knowledge_events` テーブルにも `SummaryVersionCreated` イベントが正常に記録されていた。根本原因は、V3 Reproject の swap 時に KnowledgeProjector のチェックポイントがリセットされなかったことによる**チェックポイントギャップ**であった。V3 reproject が event_seq 1116081 まで処理した後、LIVE projector のチェックポイントは既にそれを大幅に超えていたため、seq 1116082 以降のイベントが version 3 に投影されなかった。結果として **326 件の記事が Knowledge Home から完全欠落**し、**1,947 件の記事で `summary_state` が stale（v1=ready なのに v3=pending/missing）** となった。

## 影響

- **Knowledge Home フィード**: projection version 3（アクティブ）で 326 件の記事が表示されない状態が約 4 日間継続
- **要約ステータス**: 1,947 件の記事で `summary_state` が `pending`（FE では "Summarizing"）のまま。実際には要約は生成済みだが、Knowledge Home の投影テーブルに反映されていなかった
- **影響ユーザー数**: 全 Knowledge Home ユーザー（2 テナント）
- **データ損失**: なし（イベントログと `article_summaries` は完全に正常。read model のみが不整合）
- **記事重複**: 報告対象の記事「InfoQ: The "Safety" Myth in AI」は RSS フィードからの重複取り込みにより 5 件の duplicate が存在。うち 3 件が version 3 に完全欠落、1 件が `summary_state = pending`、1 件のみ正常

## タイムライン

| 時刻 (JST) | イベント |
|---|---|
| 2026-03-22 08:56 | V3 Reproject 開始（V2 → V3、full モード） |
| 2026-03-22 09:28 | V3 Reproject 完了。event_seq 0 → 1116081 を version 3 に投影。ステータス `swappable` に遷移 |
| 2026-03-22 09:28 〜 2026-03-23 07:37 | LIVE projector は version 1 で稼働継続。チェックポイントは 1116081 を大幅に超えて進行 |
| 2026-03-23 07:37 | **V3 activated（swap）**: `ActivateVersion(3)` により version 3 がアクティブに。**しかしチェックポイントはリセットされず** |
| 2026-03-23 07:37 〜 2026-03-27 06:50 | **ギャップ期間**: seq 1116082 〜 swap 時点のチェックポイントのイベントが version 3 に未投影のまま運用継続 |
| 2026-03-27 06:50 | **検知**: ユーザーが Knowledge Home で「InfoQ: The "Safety" Myth in AI」が "Summarizing" のまま更新されないことを報告 |
| 2026-03-27 06:50 | **対応開始**: コンテナログ・DB 調査を開始 |
| 2026-03-27 06:55 | `article_summaries` に要約が正常保存済みであることを確認。問題は Knowledge Home 側の投影にあると判断 |
| 2026-03-27 07:00 | `knowledge_home_items` テーブルで article `e6a5fc89` の projection_version 3 が `summary_state = pending` であることを確認 |
| 2026-03-27 07:03 | `knowledge_projection_versions` で V3 reproject のチェックポイント（`last_event_seq: 1116081`）と LIVE projector のチェックポイント（1142812）の乖離を確認 |
| 2026-03-27 07:07 | **原因特定**: `SwapReproject()` がチェックポイントをリセットしないバグを特定。影響範囲を定量化（326 件欠落、1,947 件 stale） |
| 2026-03-27 07:11 | **緩和策適用**: `knowledge_projection_checkpoints.last_event_seq` を 1116081 に手動リセット |
| 2026-03-27 07:12 | **復旧確認**: LIVE projector がギャップ（seq 1116082 〜 1142818）を約 20 秒で再投影完了。v3 の `ready` が 37,631 → 38,987 に増加、`pending` が 2,676 → 1,659 に減少 |
| 2026-03-27 07:12 | 該当記事 5 件全ての `summary_state = ready` を確認 |
| 2026-03-27 07:16 | **再発防止実装**: TDD で `SwapReproject()` にチェックポイントリセットロジックを追加。テスト 6 件全 PASS |
| 2026-03-27 07:17 | alt-backend コンテナ再ビルド・起動。ヘルスチェック正常 |

## 検知

- **検知方法**: ユーザーによる手動報告（Knowledge Home で特定記事が "Summarizing" のまま更新されないことに気付いた）
- **検知までの時間 (TTD)**: 約 3 日 23 時間（V3 swap から報告まで）
- **検知の評価**: 著しく遅い。summary_state の不整合を検知するモニタリングが存在しなかった。PM-2026-002 のアクションアイテム D-2（Unsummarized カウントが 24 時間以上変化しない場合の監視）が未着手のままであったことも寄与

## 根本原因分析

### 直接原因

`SwapReproject()` が projection version を切り替える際に、KnowledgeProjector のチェックポイントを reproject の最終 event_seq にリセットしなかった。

### Five Whys

1. **なぜ記事が "Summarizing" のまま更新されなかったか？**
   → `knowledge_home_items` テーブルの projection_version 3 で `summary_state = pending` のままだったため

2. **なぜ `summary_state` が `pending` のままだったか？**
   → `SummaryVersionCreated` イベント（seq 1128630）が version 3 に投影されなかったため

3. **なぜイベントが version 3 に投影されなかったか？**
   → V3 reproject は seq 1116081 までしか処理せず、LIVE projector は swap 時に既に seq ~1142000 まで進んでいたため、seq 1116082 〜 ~1142000 のイベントが version 3 にとって「ギャップ」となった

4. **なぜ LIVE projector がギャップを埋めなかったか？**
   → `knowledge_projection_checkpoints` テーブルがバージョン非依存（`projector_name` のみで管理）であり、swap 時にチェックポイントがリセットされなかったため、LIVE projector は既に処理済みのイベントとしてギャップをスキップした

5. **なぜ swap 時にチェックポイントがリセットされなかったか？**
   → `SwapReproject()` の実装が `ActivateVersion()` のみを呼び出し、チェックポイントリセットのステップが設計・実装されていなかった。チェックポイントテーブルの設計がバージョン非依存であることによるリスクが、reproject 機能の実装時に考慮されていなかった

### 根本原因

`SwapReproject()` が `ActivateVersion()` のみを呼び出し、`UpdateProjectionCheckpoint()` を呼び出してチェックポイントを reproject の最終 event_seq にリセットするステップが欠落していた。`knowledge_projection_checkpoints` テーブルがバージョン非依存（`projector_name` のみの PK）であるため、version 切り替え時にチェックポイントの一貫性が保証されない構造的な問題があった。

### 寄与要因

- **チェックポイントテーブルのバージョン非依存設計**: `knowledge_projection_checkpoints` に `projection_version` カラムがなく、単一のチェックポイントが全バージョンで共有されている。この設計では、reproject と live projector のチェックポイントが独立管理されず、swap 時にギャップが発生するリスクが構造的に存在する
- **reproject と swap の時間差**: V3 reproject 完了（2026-03-22 09:28）から swap（2026-03-23 07:37）まで約 22 時間の間隔があった。この間に LIVE projector のチェックポイントが大幅に進行し、ギャップが拡大した
- **summary_state 不整合の監視不在**: PM-2026-002 で提案された D-2（Unsummarized カウントの監視）が未着手のままだった
- **RSS 記事重複**: 該当記事が 5 件の duplicate として登録されており、問題の影響が分散して気付きにくくなった

## 対応の評価

### うまくいったこと

- DB 調査で `knowledge_events`（イベントログ）→ `knowledge_home_items`（投影テーブル）→ `knowledge_projection_checkpoints`（チェックポイント）→ `knowledge_reproject_runs`（reproject 履歴）の突合を体系的に行い、約 20 分で根本原因を特定できた
- イミュータブルデータモデルの設計原則（append-first, disposable projections）により、チェックポイントリセットだけでデータ修復が完了した。read model の rebuild は UPSERT で安全に実行された
- Obsidian vault の過去の PM（PM-002, PM-004）が類似事象の知見として調査を加速した
- TDD で修正を実装し、既存テスト 4 件 + 新規テスト 2 件の全 6 件が PASS

### うまくいかなかったこと

- 約 4 日間、326 件の記事欠落と 1,947 件の stale summary_state が放置された（能動的なモニタリング不足）
- PM-2026-002 で提起された D-2 アクションアイテム（監視追加）が未着手のまま、類似の検知遅延が再発した
- `SwapReproject()` の設計レビューで、チェックポイントの一貫性が検証されていなかった

### 運が良かったこと

- イベントログが完全に保持されており、チェックポイントリセットのみで full reproject なしに修復できた
- 影響は Knowledge Home の read model（disposable projection）のみで、article_summaries や knowledge_events 等の source of truth には一切影響がなかった
- ギャップ区間の再投影が約 20 秒で完了し、サービス影響を最小限に抑えられた

## アクションアイテム

### 予防（Prevent）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| P-1 | `SwapReproject()` にチェックポイントリセットロジックを追加（reproject の `checkpoint_payload.last_event_seq` を projector checkpoint に書き込み） | 開発担当者 | 2026-03-27 | **完了** |
| P-2 | `WithUpdateCheckpointPort()` による DI 配線を追加 | 開発担当者 | 2026-03-27 | **完了** |
| P-3 | `SwapReproject` のテストにチェックポイントリセット検証を追加（2 件） | 開発担当者 | 2026-03-27 | **完了** |

### 検知（Detect）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| D-1 | projection_version 間の `summary_state` 分布比較メトリクスを追加し、version 間の乖離が閾値（5%）を超えた場合にアラート | 開発担当者 | 2026-04-10 | 未着手 |
| D-2 | PM-2026-002 D-2 の再掲: Unsummarized カウントが 24 時間以上変化しない場合の監視追加 | 開発担当者 | 2026-04-10 | 未着手 |
| D-3 | Reproject swap 実行後に自動 sanity check（v_old と v_new の item count / summary_state 分布比較）を実行するジョブを追加 | 開発担当者 | 2026-04-10 | 未着手 |

### 緩和（Mitigate）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| M-1 | reproject swap 操作の runbook に「swap 後のチェックポイント確認」ステップを追加 | 開発担当者 | 2026-04-03 | 未着手 |

### プロセス（Process）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| R-1 | reproject 機能の設計レビュー: チェックポイントテーブルへの `projection_version` カラム追加の検討 | 開発担当者 | 2026-04-17 | 未着手 |

## 教訓

### 技術的教訓

1. **チェックポイントはバージョンに紐づけるべき**: バージョン非依存のチェックポイントは、projection version の切り替え時にギャップを生む構造的リスクがある。チェックポイントテーブルに `projection_version` カラムを追加するか、swap 時に明示的にリセットする仕組みが必要
2. **reproject swap はアトミックな操作であるべき**: version activation とチェックポイントリセットは不可分な操作として扱うべき。片方だけ実行すると不整合が発生する
3. **イミュータブルデータモデルの「disposable projections」原則が修復を容易にした**: read model が使い捨て可能であるという設計原則のおかげで、チェックポイントリセットだけで完全修復できた。この設計がなければ、複雑なデータマイグレーションが必要だった
4. **reproject と swap の時間差はリスクを増大させる**: 時間差が大きいほどギャップが広がる。swap を自動化するか、swap 前にギャップ量を計算・表示する仕組みが有効

### 組織的教訓

1. **過去の PM のアクションアイテム未完了が再発を招いた**: PM-2026-002 D-2（監視追加）が未着手のまま、同種の検知遅延が再発した。アクションアイテムの進捗管理を強化すべき
2. **reproject は高リスク操作として運用ガードレールが必要**: 現状 reproject swap は管理者が手動実行するが、swap 後の sanity check が組み込まれていない。高リスク操作にはガードレール（自動検証、ロールバック手順）を付帯すべき

## 参考資料

- `alt-backend/app/usecase/knowledge_reproject_usecase/usecase.go` — SwapReproject() 修正箇所
- `alt-backend/app/usecase/knowledge_reproject_usecase/usecase_test.go` — 追加テスト
- `alt-backend/app/di/knowledge_module.go` — DI 配線
- [[PM-2026-002-unsummarized-infinite-enqueue-loop]] — 関連 PM（要約パイプラインの無限ループ）
- [[PM-2026-004-streamsummarize-524-and-streaming-failure]] — 関連 PM（StreamSummarize タイムアウト）
- `plan/knowledge-home-phase0-canonical-contract.md` — Knowledge Home canonical contract
- `features/knowledge-home/data-flow.md` — Knowledge Home データフロー
- `features/knowledge-home/architecture.md` — Knowledge Home アーキテクチャ

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
