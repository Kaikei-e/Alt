# PM-2026-003: Visual Preview 要約不一致（記事と無関係なサマリー表示）

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-3（一部機能の誤動作、データ整合性に影響） |
| 影響期間 | 不明 〜 2026-03-23（潜在的には Visual Preview 実装当初から） |
| 影響サービス | alt-frontend-sv, alt-backend |
| 影響機能 | Desktop Visual Preview の AI Summary 表示、Mobile Swipe の AI Summary 表示 |
| 関連 ADR | [[000552]] |

## サマリー

Desktop UI の Visual Preview モーダルで記事タイトルとまったく無関係な AI Summary が表示される不具合が報告された。"Astro 5.0 in 2026: The Content Framework That's Killing WordPress" の記事を開いた際に、ソニー "Project Chimera" ロボットに関する日本語要約が表示されていた。根本原因は、フロントエンドの要約ストリーミングコールバックに stale-response guard が欠如していたことによる race condition であった。副次的にバックエンドの読み取りクエリに `user_id` スコープが欠如していた。

## 影響

- **UI の信頼性**: 記事と無関係な要約が表示されることで、AI Summary 機能の信頼性が低下
- **再現条件**: 矢印キーで記事間をナビゲートしながら要約を生成する操作パターンで再現。キャッシュ済みサマリーがある記事から未生成の記事に移動する際に発生しやすい
- **影響範囲**: Desktop Visual Preview（`FeedDetailModal`）、Mobile Swipe（`SwipeFeedCard`、`VisualPreviewCard`）の全プラットフォーム
- **データ損失**: なし（DB のデータ自体は正しく保存されている。表示のみの問題）

## タイムライン

| 時刻 (JST) | イベント |
|---|---|
| 不明 | **発生**: Visual Preview 実装時から潜在的に存在。`handleFetchFullArticle` には stale guard があるが `handleSummarize` に漏れていた |
| 2026-03-23 22:30 | **検知**: ユーザーが Desktop UI Visual Preview で "Astro 5.0" 記事に "Project Chimera" の要約が表示されていることを報告 |
| 2026-03-23 22:30 | **対応開始**: コンテナログの徹底調査を開始。過去類似 ADR（000509, 000447, 000551）を参照 |
| 2026-03-23 22:45 | バックエンドログ確認: StreamSummarize の stream completed / summary saved ログは正常。DB 保存パスに問題なし |
| 2026-03-23 22:50 | フロントエンド `FeedDetailModal.svelte` 調査: `handleFetchFullArticle` には stale guard あり、`handleSummarize` には欠如していることを特定 |
| 2026-03-23 23:00 | バックエンド `FetchArticleSummaryByArticleID` / `FetchArticleByURL` に `user_id` フィルター欠如を発見 |
| 2026-03-23 23:00 | **原因特定**: フロントエンド race condition + バックエンド user_id スコープ欠如の 2 重原因 |
| 2026-03-23 23:10 | フロントエンド修正: 3 コンポーネントに stale-response guard 追加 |
| 2026-03-23 23:15 | バックエンド修正: 2 ドライバー関数に user_id スコープ追加、テスト更新 |
| 2026-03-23 23:20 | 全テスト PASS 確認（Go `go test ./...` 0 FAIL、svelte-check 0 ERRORS） |
| 2026-03-23 23:25 | **復旧**: alt-backend, alt-frontend-sv コンテナ再ビルド・デプロイ、health check OK |

## 根本原因分析

### Five Whys

1. **なぜ無関係な要約が表示されたか？**
   → 前の記事のストリーミングレスポンスが、ナビゲーション後の新しい記事の `summary` state を上書きしたため

2. **なぜ前の記事のレスポンスが上書きできたか？**
   → `handleSummarize` の `onChunk` コールバックに stale-response guard がなく、コールバックは feed が変わっても元の reactive state に書き込むため

3. **なぜ guard がなかったか？**
   → `handleFetchFullArticle` には guard が実装されていたが、`handleSummarize` は別のタイミングで追加された機能であり、同じパターンが適用されていなかった。コードレビューで検出できなかった

4. **なぜ AbortController で防げなかったか？**
   → `$effect` のリセットで `abortController.abort()` は呼ばれるが、Connect-RPC の `for await` ループが既にバッファしたレスポンス（特にキャッシュ済みの単一レスポンス）は abort シグナル伝搬前に `onChunk` に到達する。JavaScript の microtask キューの特性上、abort 呼び出しとコールバック実行の間に race window が存在する

5. **なぜ同じ guard パターンが統一されていなかったか？**
   → `handleFetchFullArticle` と `handleSummarize` が同一コンポーネント内で独立に実装されており、共通の stale-response 防御パターンが抽出されていなかった。Svelte 5 の `$effect` + async コールバックにおける race condition 防止パターンの標準化が不足していた

### 寄与要因

- **バックエンドの user_id スコープ欠如**: `FetchArticleSummaryByArticleID` と `FetchArticleByURL` が `user_id` でフィルターしていなかった。単一ユーザー環境では顕在化しにくいが、UNIQUE 制約の設計と読み取りクエリが不一致
- **キャッシュ済みサマリーの即時返却**: バックエンドがキャッシュヒット時に単一メッセージで全文を返すため、レスポンスが即座にバッファされ race window が拡大
- **Mobile コンポーネントの同一パターン**: `SwipeFeedCard.svelte` と `VisualPreviewCard.svelte` にも同じ脆弱性が存在

## 対応の評価

### うまくいったこと

- 過去 ADR（000509, 000447, 000551）の調査から、要約パイプラインの既知の弱点を効率的に除外できた
- `handleFetchFullArticle` の既存 guard パターンがあったため、修正パターンが明確だった
- バックエンドログから StreamSummarize の正常動作を確認し、フロントエンド原因に絞り込めた

### 改善が必要なこと

- `handleSummarize` 追加時に `handleFetchFullArticle` と同じ guard パターンを適用すべきだった
- Svelte 5 の async + reactive state における race condition パターンの標準チェックリストがなかった
- E2E テストで「素早いナビゲーション中の要約表示」を検証するシナリオがなかった

### 運が良かったこと

- DB のデータ自体は正しく保存されていた（表示のみの問題でデータ損失なし）
- 修正パターンが既にコードベースに存在していたため、修正が迅速だった

## 教訓

### 技術的教訓

1. **async コールバックと reactive state の組み合わせは必ず stale guard を入れる**: Svelte 5 の `$state` + async ストリーミングコールバックでは、`AbortController` だけでは race condition を完全に防げない。キャプチャした URL/ID との比較による guard が必須
2. **同一コンポーネント内の類似パターンは統一する**: content fetch と summary fetch で同じ guard パターンが必要なのに片方だけ実装された。パターンの不統一はバグの温床
3. **DB の UNIQUE 制約と読み取りクエリのスコープを一致させる**: `ON CONFLICT (article_id, user_id)` で保存するなら、読み取りも `user_id` でフィルターすべき

### 組織的教訓

1. **ストリーミング UI コンポーネントのレビューチェックリストに「stale-response guard」を追加すべき**
2. **E2E テストに「素早い操作」シナリオを含めるべき**: 通常操作では再現しにくい race condition を検出するため

## アクションアイテム

### 予防（Prevent）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| P-1 | `FeedDetailModal.svelte` の `handleSummarize` に stale-response guard 追加 | 開発担当者 | 2026-03-23 | **完了** |
| P-2 | `SwipeFeedCard.svelte` の `handleGenerateAISummary` に stale-response guard 追加 | 開発担当者 | 2026-03-23 | **完了** |
| P-3 | `VisualPreviewCard.svelte` の `handleGenerateAISummary` に stale-response guard 追加 | 開発担当者 | 2026-03-23 | **完了** |
| P-4 | `FetchArticleSummaryByArticleID` に user_id フィルター追加 | 開発担当者 | 2026-03-23 | **完了** |
| P-5 | `FetchArticleByURL` に user_id フィルター追加 | 開発担当者 | 2026-03-23 | **完了** |

### 検知（Detect）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| D-1 | E2E テストに「矢印キー連打中の要約表示整合性」シナリオを追加 | 開発担当者 | 2026-04-06 | 未着手 |

### プロセス（Process）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| R-1 | ストリーミング UI コンポーネントのコードレビューチェックリストに「stale-response guard」項目を追加 | 開発担当者 | 2026-04-06 | 未着手 |
