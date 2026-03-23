# PM-2026-002: 要約不可記事の無限再エンキューループ

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-4（ヒヤリハット / リソース浪費） |
| 影響期間 | 2026-03-19 〜 2026-03-23（約 4 日間、潜在的にはそれ以前から） |
| 影響サービス | pre-processor, alt-backend |
| 影響機能 | Desktop UI Stats の Unsummarized カウント、summarize_job_queue のリソース消費 |
| 関連 ADR | [[000551]] |

## サマリー

Desktop UI の Stats に表示される "Unsummarized" カウントが 63 件で固定され減少しない状態が継続した。根本原因は、content が短すぎる（< 100文字）または長すぎる（> 100KB）記事が要約パイプラインでスキップされた際に `article_summaries` に何も保存されず、同一記事が `summarize_job_queue` に繰り返しエンキューされる無限ループ構造であった。修正時点で計 1,349 回の無駄なジョブ（completed 1,249 + dead_letter 100）が生成されていた。

## 影響

- **Unsummarized カウント**: Stats に常時 63 と表示され続け、ユーザーの進捗体験を損なった
- **ジョブキュー汚染**: `summarize_job_queue` に計 1,349 件の無駄なジョブが蓄積
  - 61 記事 × 平均 20 回 = 1,249 件の `completed`（too short スキップ）
  - 2 記事 × 50 回 = 100 件の `dead_letter`（too long 繰り返し失敗）
- **リソース浪費**: 各ジョブで alt-backend への `GetArticleContent` RPC + HTML 抽出 + LLM API 呼び出し（too long のみ）が実行され、CPU・ネットワーク帯域を消費
- **データ損失**: なし
- **リアルタイム要約処理**: 影響なし（ループは非同期バッチ処理内で発生）

## タイムライン

| 時刻 (JST) | イベント |
|---|---|
| 2026-03-19 以前 | Unsummarized カウントが 63 で固定化（正確な開始日は不明） |
| 2026-03-23 21:30 | **検知**: ユーザーが Desktop UI Stats の Unsummarized 63 が減らないことを報告 |
| 2026-03-23 21:40 | **対応開始**: DB 調査を開始。`articles LEFT JOIN article_summaries` で 63 件を確認 |
| 2026-03-23 21:45 | `summarize_job_queue` の状態確認。completed: 153,162 / dead_letter: 121 を確認 |
| 2026-03-23 21:50 | **原因特定**: 63 件全てが too short (61件) / too long (2件) であり、placeholder summary が保存されていないことを特定。無限ループ構造を確認 |
| 2026-03-23 22:00 | TDD で修正実装。`ErrContentTooShort` / `ErrContentTooLong` 時に placeholder summary を `article_summaries` に保存するロジックを追加 |
| 2026-03-23 22:11 | 全テスト PASS を確認 |
| 2026-03-23 22:12 | pre-processor コンテナ再ビルド・デプロイ |
| 2026-03-23 22:17 | **緩和**: 最初のバッチ sweep 実行。`"placeholder summary saved"` ログで 10 件の placeholder 保存を確認 |
| 2026-03-23 22:20 | Unsummarized カウントが 63 → 53 に減少。修正の有効性を確認 |
| 2026-03-23 22:50 頃 | **復旧予定**: 5 分間隔 × 10 件/バッチ で全 63 件が処理完了（推定） |

## 根本原因分析

### Five Whys

1. **なぜ Unsummarized カウントが減らなかったか？**
   → 63 件の記事に対して `article_summaries` テーブルにエントリが存在しなかったため

2. **なぜ `article_summaries` に保存されなかったか？**
   → pre-processor が content too short / too long でスキップした際に、ジョブを `completed` にするだけで `SaveArticleSummary` を呼んでいなかったため

3. **なぜ同じ記事が何度もエンキューされたか？**
   → `ShouldQueueSummarizeJob` ガードの `Exists()` が `article_summaries` を参照するが、エントリがないため常に false → `CreateJob` で新ジョブ作成が許可された。`HasRecentSuccessfulJob()` もジョブの `summary` カラムが NULL（スキップ時は空文字保存）のため false を返した

4. **なぜ設計時にこのケースが考慮されなかったか？**
   → quality checker に `knownPlaceholders`（`"本文が短すぎるため要約できませんでした。"`）が定義されており、placeholder 保存の設計意図は存在したが、`processJob` のスキップ処理に `summaryRepo.Create` 呼び出しが実装されていなかった。設計と実装の間にギャップがあった

5. **なぜ content が短い記事が大量に存在するか？**
   → `articles.content` には RSS フィードの description（短い HTML スニペット）が格納されており、fulltext fetch が行われていない記事ではHTML 抽出後の text が 100 文字未満になる。NHK・BBC 等のニュースフィードは description が特に短い

### 寄与要因

- **ジョブ重複防止の不完全性**: `CreateJob` は `pending` / `running` の既存ジョブのみブロックし、`completed` / `dead_letter` の過去ジョブを考慮しない設計。content エラーによるスキップが想定されていなかった
- **バッチサイズの影響**: バッチ sweep が 10 件ずつカーソルページネーションで処理するため、大量の unsummarized 記事がキューを占有し、新着記事の要約処理を遅延させる可能性があった
- **too long の retry 設計**: `ErrContentTooLong` は `ErrContentTooShort` と異なり明示的にハンドリングされておらず、汎用の retry → dead_letter フローに入っていた。3 回のリトライは無駄

## 対応の評価

### うまくいったこと

- DB クエリで 63 件の記事特徴と `summarize_job_queue` の状態を迅速に突合できた
- `quality_judger.go` の `knownPlaceholders` 定義から placeholder 保存の設計意図を即座に把握できた
- TDD（RED → GREEN → REFACTOR）で安全に修正を実装できた
- デプロイ後すぐにログで `"placeholder summary saved"` を確認し、修正の有効性を検証できた

### 改善が必要なこと

- 無限ループが数日間検知されなかった（能動的なモニタリング不足）
- `summarize_job_queue` に 1,349 件の無駄なジョブが蓄積していたが、アラートがなかった
- `ErrContentTooLong` が汎用 retry フローに入る設計は、リトライ回数分の API コールを無駄に消費した

### 運が良かったこと

- リアルタイム要約処理（ユーザーが記事を開いた際のオンデマンド要約）は影響を受けなかった
- 無限ループはバッチバックグラウンドジョブ内で発生しており、他の pre-processor 機能（記事同期、品質チェック等）はブロックされていなかった

## 教訓

### 技術的教訓

1. **設計意図と実装の整合性を検証する仕組みが必要**: `knownPlaceholders` の定義と `processJob` のスキップ処理が一致していなかった。設計文書から実装へのトレーサビリティが不足している
2. **「何もしない」パスは最もバグが潜みやすい**: エラーハンドリングで「スキップ」を選ぶ場合、下流のシステム（Stats カウント、ガードチェック）への影響を網羅的に考慮すべき
3. **冪等性ガードは全ての終了パスをカバーする必要がある**: `ShouldQueueSummarizeJob` は正常完了パスのみを想定しており、スキップ完了パスを考慮していなかった
4. **content too long は即座に非再試行エラーとして扱うべき**: content サイズは変わらないため、リトライしても同じ結果になる

### 組織的教訓

1. **Stats 表示が一定期間変化しない場合のアラートがあれば、数日ではなく数時間で検知できた**
2. **`summarize_job_queue` の同一 article_id に対するジョブ件数を監視すれば、無限ループの兆候を早期検出できた**

## アクションアイテム

### 予防（Prevent）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| P-1 | `ErrContentTooShort` 時に placeholder summary を `article_summaries` に保存 | 開発担当者 | 2026-03-23 | **完了** |
| P-2 | `ErrContentTooLong` 時に placeholder summary を保存し、即座に `completed` に遷移（retry しない） | 開発担当者 | 2026-03-23 | **完了** |
| P-3 | placeholder 定数を `quality_judger.go` の `knownPlaceholders` と統一 | 開発担当者 | 2026-03-23 | **完了** |

### 検知（Detect）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| D-1 | `summarize_job_queue` の同一 `article_id` に対する `dead_letter` ジョブが N 件を超えた場合のアラート追加 | 開発担当者 | 2026-04-06 | 未着手 |
| D-2 | Unsummarized カウントが 24 時間以上変化しない場合の監視追加 | 開発担当者 | 2026-04-06 | 未着手 |

### 緩和（Mitigate）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| M-1 | 既存の dead_letter ジョブ 121 件のクリーンアップ（不要なレコードの整理） | 開発担当者 | 2026-03-30 | 未着手 |

### プロセス（Process）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| R-1 | 要約パイプラインのエラーハンドリングパスの設計レビュー（全ての終了パスで `article_summaries` への影響を確認） | 開発担当者 | 2026-04-06 | 未着手 |
