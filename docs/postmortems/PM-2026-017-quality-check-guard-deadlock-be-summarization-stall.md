# PM-2026-017: 品質チェック削除後の recent_success ガードデッドロックによる BE 要約停滞

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-017 |
| 発生日時 | 2026-04-02 07:36 以前（潜在的にはそれ以前から） |
| 復旧日時 | 2026-04-03 16:49 (JST)（修正デプロイ完了） |
| 影響時間 | 推定 24 時間以上（構造的問題のため正確な開始時刻は不明） |
| 重大度 | SEV-4（ヒヤリハット / 機能劣化：要約は最大24h遅延するが最終的に処理される） |
| ステータス | Approved |

## サマリー

品質チェック (`JudgeArticleQuality`) が低品質と判定した要約を `article_summaries` テーブルから削除した後、`summarize_job_queue` の completed ジョブが残存し、`ShouldQueueSummarizeJob` ガードの `HasRecentSuccessfulJob` チェック（24 時間ウィンドウ）が re-enqueue をブロックする構造的デッドロックが発生していた。検知時点で 51 件の記事が未要約のまま停滞し、バッチエンキューは `found=10, enqueued=0, skipped=10` を繰り返していた。根本原因は、品質チェック削除が `article_summaries` のみを更新し `summarize_job_queue` を補償しないデータ不整合であった。

## 影響

- **影響を受けたサービス:** pre-processor（バッチ要約パイプライン）
- **停滞記事数:** 51 件
- **機能への影響:** 部分的劣化 — 品質チェックで削除された記事の再要約が最大 24 時間遅延
- **データ損失:** なし（24 時間経過後にガードが自然解除されれば再処理される）
- **SLO/SLA 違反:** なし
- **追加の発見:**
  - dead_letter 231 件: news-creator コンテナ再起動時の DNS 解決失敗（`no such host`）に起因。本インシデントとは独立した問題
  - リモート Ollama `100.99.107.87` が `ServerDisconnectedError` を 30 秒ごとに発生（[[PM-2026-014]] と同系統）
  - 毎リクエストで COLD_START（`load_duration=0.2-0.36s`）発生 — モデルが VRAM から追い出されている

## タイムライン

| 時刻 (JST) | イベント |
|---|---|
| 不明 | **発生**: 品質チェックが低品質要約を削除。以降、対象記事の re-enqueue がガードにブロックされ始める |
| 2026-04-02 16:36 | 記事 `69949cbf` の completed ジョブ（summary 非 NULL）が作成。以降 24h ガードに捕捉される |
| 2026-04-03 11:38 | news-creator コンテナ再起動に伴い DNS 解決失敗。dead_letter ジョブが発生（独立した問題） |
| 2026-04-03 16:20 | **検知**: ユーザーが「BE 要約が詰まっている」と報告。調査開始 |
| 2026-04-03 16:21 | **対応開始**: コンテナ状態、pre-processor / news-creator / mq-hub のログ、DB 状態を並行調査 |
| 2026-04-03 16:23 | pre-processor ログで `batch enqueue completed: found=10, enqueued=0, skipped=10` を確認。全記事が `recent_success` でスキップされていることを特定 |
| 2026-04-03 16:25 | `summarize_job_queue` と `article_summaries` のクロスチェック。記事 `3a0f6a43` が pre-processor では completed（summary あり）だが alt-backend では未要約であることを確認 |
| 2026-04-03 16:28 | **原因特定**: 品質チェック削除が `article_summaries` のみ更新し `summarize_job_queue` を補償しないデータ不整合を特定。[[PM-2026-002]] の逆パターンであることを認識 |
| 2026-04-03 16:30 | Obsidian vault で関連 ADR（[[000513]], [[000551]], [[000251]]）とポストモーテム（[[PM-2026-002]]）を確認。Web 調査で Compensating Transaction パターンを検討 |
| 2026-04-03 16:42 | **緩和策実装**: TDD で `InvalidateCompletedJobSummary` メソッドと補償トランザクションを実装。全 23 パッケージのテスト PASS |
| 2026-04-03 16:49 | **復旧**: pre-processor コンテナ再ビルド・デプロイ完了。ヘルスチェック healthy 確認 |

## 検知

- **検知方法:** ユーザーによる手動確認（「BE 要約が詰まっている」という報告）
- **検知までの時間 (TTD):** 不明（構造的問題のため発生時刻が特定不能。最も古い停滞記事は 24h 以上前）
- **検知の評価:** [[PM-2026-002]] の D-2 アクションアイテム（「Unsummarized カウントが 24 時間以上変化しない場合の監視追加」）が未実装のまま残っており、自動検知できなかった。同種の問題が 2 度目の手動検知となった

## 根本原因分析

### 直接原因

品質チェック (`RemoveLowScoreSummary`) が `article_summaries` から要約を削除する際に、`summarize_job_queue` の対応する completed ジョブの `summary` カラムを更新しない。`HasRecentSuccessfulJob()` が `summary IS NOT NULL AND summary <> ''` を条件に含むため、削除後もガードが `true` を返し re-enqueue をブロックする。

### Five Whys

1. **なぜ記事が未要約のまま停滞したか？**
   → `ShouldQueueSummarizeJob` ガードが `recent_success` で re-enqueue をブロックしたため

2. **なぜガードがブロックしたか？**
   → `summarize_job_queue` に 24h 以内の completed ジョブ（summary 非 NULL）が残存していたため

3. **なぜ completed ジョブが残存していたか？**
   → 品質チェックが `article_summaries` を削除した際に `summarize_job_queue` を更新しなかったため

4. **なぜ品質チェックがジョブキューを更新しなかったか？**
   → [[000513]] のガード設計が「上流再通知への防御」を目的としており、品質チェックによる意図的な削除を想定していなかったため。`qualityCheckerService` は `SummarizeJobRepository` への依存を持っていなかった

5. **なぜこの設計ギャップが検出されなかったか？**
   → 品質チェック機能とガード機能が異なるタイミング・ADR で導入され、相互作用の統合テストが存在しなかったため

### 根本原因

`article_summaries`（alt-backend DB）と `summarize_job_queue`（pre-processor DB）という 2 つのデータストアにまたがるデータ整合性が、品質チェック削除パスで保証されていなかった。ガード ([[000513]]) と品質チェック ([[000251]]) が独立した ADR で設計・実装されたため、両者の相互作用（品質チェック削除 → ガード無効化）が設計時に考慮されなかった。

### 寄与要因

- [[PM-2026-002]] の D-2（Unsummarized 24h 停滞の監視）が未実装のまま期限（2026-04-06）を迎えていた
- 品質チェックのスコアリングが `</end_of_turn>` レスポンスをパース失敗 → フォールバックスコア 1 で削除判定 → 本来削除不要な要約も削除されていた可能性がある
- バッチサイズ 10 + `has_more: true` の組み合わせで、ガードにブロックされた記事がバッチを占有し、新しい記事の要約も遅延する

## 対応の評価

### うまくいったこと

- DB クロスチェック（`summarize_job_queue` vs `article_summaries`）で記事 `3a0f6a43` のデータ不整合を迅速に特定できた
- [[PM-2026-002]] の経験から、ガードと要約テーブルの関係性を素早く理解し、逆パターンであることを認識できた
- Obsidian vault の ADR 検索（[[000513]], [[000551]], [[000251]]）で設計背景を即座に把握し、適切な修正方針を立案できた
- Web 調査で Compensating Transaction パターンを参照し、設計根拠を明確化できた
- TDD で安全に実装 → 全テスト PASS → デプロイまで約 30 分で完了

### うまくいかなかったこと

- [[PM-2026-002]] の D-2 アクションアイテムが未実装のまま同種の問題が再発した
- 品質チェックの LLM スコアリングが接続エラー時にフォールバックスコア 1 を返し、低品質と誤判定 → 不要な削除が発生していた（今回の調査で発見）
- `CheckQuality` 内の品質チェックパスは `JudgeArticleQuality`（パッケージ関数、実 LLM 呼び出し）を経由するため、unit test で補償の E2E パスを直接テストできない

### 運が良かったこと

- 24 時間ウィンドウの経過後にガードが自然解除されるため、完全な永久停滞にはならなかった
- 品質チェック対象はバッチ処理のみで、FE からのオンデマンド要約（ストリーミング）は影響を受けなかった
- 停滞記事が 51 件と比較的少数で済んだ（品質チェックの削除率が低かったため）

## アクションアイテム

### 予防（Prevent）

| # | アクション | 担当 | 期限 | ステータス |
|---|---|---|---|---|
| P-1 | `InvalidateCompletedJobSummary` 補償トランザクションを `qualityCheckerService` に追加（[[000615]]） | 開発担当者 | 2026-04-03 | **完了** |
| P-2 | `scoreSummaryWithRetry` の全リトライ失敗時のフォールバック判定を見直し — パース失敗はスコア 1 ではなく「スキップ（要約保持）」にする | 開発担当者 | 2026-04-10 | 未着手 |

### 検知（Detect）

| # | アクション | 担当 | 期限 | ステータス |
|---|---|---|---|---|
| D-1 | [[PM-2026-002]] D-2 を実装: Unsummarized カウントが 24 時間以上変化しない場合の監視追加 | 開発担当者 | 2026-04-10 | 未着手 |
| D-2 | `batch enqueue completed: enqueued=0, skipped=N` が連続 K 回発生した場合の WARNING ログ追加 | 開発担当者 | 2026-04-10 | 未着手 |

### 緩和（Mitigate）

| # | アクション | 担当 | 期限 | ステータス |
|---|---|---|---|---|
| M-1 | 既存 51 件の停滞記事を手動 SQL で復旧: `UPDATE summarize_job_queue SET summary = NULL WHERE article_id IN (...) AND status = 'completed'` | 開発担当者 | 2026-04-04 | 未着手 |

### プロセス（Process）

| # | アクション | 担当 | 期限 | ステータス |
|---|---|---|---|---|
| O-1 | 要約パイプラインのエラーハンドリングパスの設計レビュー（[[PM-2026-002]] R-1 と統合、全ての削除パスで `summarize_job_queue` への影響を確認） | 開発担当者 | 2026-04-10 | 未着手 |

## 教訓

### 技術的教訓

1. **2 つのデータストアにまたがる操作には補償トランザクションが必要**: `article_summaries`（alt-backend）と `summarize_job_queue`（pre-processor）は異なる DB だが論理的に結合している。一方を更新する操作は、他方への影響を必ず考慮すべき
2. **ガードの設計は全ての終了パスを網羅する必要がある**: [[PM-2026-002]] で「スキップ完了パス」のガード漏れを修正したが、今回は「品質チェック削除パス」で同種の漏れが発生した。ガードに影響を与える状態変更を行う全ての箇所をレビューする必要がある
3. **LLM スコアリングの失敗パスは「安全側」にフォールバックすべき**: パース失敗 → スコア 1 → 削除は「安全側」ではない。パース失敗時は要約を保持（スキップ）し、次回バッチで再評価する方が安全

### 組織的教訓

1. **アクションアイテムの追跡と期限管理を強化すべき**: [[PM-2026-002]] の D-2（Unsummarized 監視）が未実装のまま期限を迎えており、今回の問題を自動検知できなかった。2 度目の同種インシデントである
2. **独立した ADR で導入された機能間の相互作用テストが必要**: ガード ([[000513]]) と品質チェック ([[000251]]) は異なる時期に導入されたが、統合テスト（品質チェック削除 → ガード → re-enqueue）が存在しなかった

## 参考資料

- [[000615]] 品質チェック削除時に summarize_job_queue の補償トランザクションを追加する
- [[000513]] 要約 queue 作成前に summary 既存と直近成功を確認する
- [[000551]] Placeholder summary で無限再エンキューを防止
- [[000251]] Quality checker で placeholder summary をスキップ
- [[PM-2026-002-unsummarized-infinite-enqueue-loop]] 要約不可記事の無限再エンキューループ
- [[PM-2026-014-news-creator-slow-summarization-recap-degradation]] news-creator 要約遅延と複合障害
- [Compensating Transaction Pattern — Azure Architecture Center](https://learn.microsoft.com/en-us/azure/architecture/patterns/compensating-transaction)
- [Saga Pattern — microservices.io](https://microservices.io/patterns/data/saga.html)

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
