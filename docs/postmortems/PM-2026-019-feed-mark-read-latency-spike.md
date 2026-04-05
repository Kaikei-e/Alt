# ポストモーテム: フィード既読操作の間欠的レイテンシスパイク

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-019 |
| 発生日時 | 2026-04-06 14:23 (JST) ※ログ上の最初の観測 |
| 復旧日時 | 2026-04-06（同日中に修正デプロイ完了） |
| 影響時間 | 断続的（間欠的に発生、推定数週間以上潜在） |
| 重大度 | SEV-3 |
| 作成者 | オンコール担当者 |
| レビュアー | — |
| ステータス | Action Items Complete |

## サマリー

デスクトップ UI でフィードを既読にする操作が間欠的に非常に遅くなる問題が発生した。nginx ログの計測で MarkAsRead のレスポンスタイムが通常 4–20 ms のところ最大 634 ms まで劣化し、GetUnreadFeeds も通常 2–18 ms のところ最大 424 ms に劣化していた。根本原因は `CachedFeedListUsecase.loadMergedFeeds()` の fan-out N+1 クエリパターンで、SharedCache の TTL expire 時に N 個の並列 DB クエリが PgBouncer のコネクションプールに殺到し、同時実行中の MarkAsRead がコネクション待ちでブロックされていた。usecase を既存の効率的な単一 SQL クエリに委譲する修正を適用し、問題を解消した。

## 影響

- **影響を受けたサービス:** alt-backend, alt-frontend-sv
- **影響を受けたユーザー数/割合:** 全デスクトップユーザー（間欠的に全員が影響を受ける可能性）
- **機能への影響:** パフォーマンス低下（既読操作が最大 634 ms、フィード一覧取得が最大 424 ms に劣化）
- **データ損失:** なし
- **SLO/SLA違反:** なし（機能は動作していたが UX が劣化）

### 定量的影響

| メトリクス | 正常値 | 劣化時ピーク | 劣化倍率 |
|---|---|---|---|
| MarkAsRead (nginx) | 4–20 ms | 634 ms | ~32–158x |
| GetUnreadFeeds (usecase_ms) | 2–18 ms | 424 ms | ~24–212x |
| DB コネクション消費/リクエスト | 1 | N+2（最大10） | ~10x |
| FE リクエスト/MarkAsRead 後 | 0 (optimistic) | 3 (invalidation cascade) | 3x |

## タイムライン

| 時刻 (JST) | イベント |
|---|---|
| 不明（推定数週間前） | `CachedFeedListUsecase` 導入時から問題が潜在。SharedCache の TTL expire タイミングでのみ顕在化するため見逃されていた |
| 2026-04-06 14:23 | **検知** — ユーザー報告により調査開始。nginx ログで MarkAsRead に 356 ms の遅延を確認 |
| 2026-04-06 14:24 | nginx ログで MarkAsRead が 634 ms / 518 ms の連続遅延を確認 |
| 2026-04-06 15:31 | alt-backend の `feed_read_perf` ログで GetUnreadFeeds が 424 ms（usecase_ms）を記録。全リクエストで `cache_hit: false` |
| 2026-04-06 15:35 | **原因特定** — `CachedFeedListUsecase.loadMergedFeeds()` の fan-out N+1 パターンが根本原因と特定。SharedCache TTL=2 分 expire 時の N 並列 DB クエリが PgBouncer コネクションプールを圧迫 |
| 2026-04-06 16:00 | **緩和策適用** — usecase を既存の単一 SQL ポートに委譲する修正を実装・テスト完了 |
| 2026-04-06 16:30 | **復旧確認** — alt-backend を `--build` で再デプロイ。ヘルスチェック正常、コンテナ healthy を確認 |

## 検知

- **検知方法:** ユーザー報告（デスクトップ UI の体感的な遅延）
- **検知までの時間 (TTD):** 推定数週間（問題は間欠的に発生していたが、顕在化するタイミングが限定的だったため発見が遅れた）
- **検知の評価:** 不十分。`feed_read_perf` ログに計測データが出力されていたが、閾値ベースのアラートが設定されていなかったため自動検知できなかった。`cache_hit` が常に `false` を報告していたこともキャッシュ層の問題を見えにくくしていた

## 根本原因分析

### 直接原因

`CachedFeedListUsecase.loadMergedFeeds()` が SharedCache（TTL=2 分、stale=1 分）expire 時に N 個のサブスクリプションに対して N 個の並列 DB クエリ（max 8 concurrent）を発行し、PgBouncer のコネクションプール（DEFAULT_POOL_SIZE=30）を圧迫。同時実行中の MarkAsRead がコネクション待ちでブロックされた。

### Five Whys

1. **なぜ MarkAsRead が 634 ms かかったのか？** → PgBouncer のコネクションプールが枯渇し、単純な UPSERT クエリがコネクション取得待ちでブロックされたため
2. **なぜコネクションプールが枯渇したのか？** → `loadMergedFeeds` が N 個の並列 DB クエリを同時発行し、最大 8 コネクションを占有していたため
3. **なぜ N 個の並列クエリが必要だったのか？** → `CachedFeedListUsecase` がサブスクリプションごとにフィードページを個別取得し、アプリケーション層でマージ・フィルタする設計だったため
4. **なぜアプリケーション層でマージする設計になっていたのか？** → キャッシュによる高速化を意図して導入されたが、TTL expire 時のコストが考慮されていなかったため。また、効率的な単一 SQL クエリが既に driver 層に存在することが見落とされていた
5. **なぜ TTL expire 時のコストが考慮されていなかったのか？** → キャッシュヒット時のレイテンシ（2 ms 以下）のみが評価対象で、キャッシュミス時の bimodal レイテンシ分布（2 ms vs 424 ms）が性能評価に含まれていなかったため

### 根本原因

キャッシュ導入時に「キャッシュヒット時の性能」のみを評価し、「キャッシュミス時の fan-out コスト」と「コネクションプールへの副作用」を考慮しなかった設計判断。加えて、同等の機能を持つ効率的な SQL クエリが既に存在していたことの認識不足。

### 寄与要因

- **`GetAllReadFeedIDs` が LIMIT なし:** ユーザーが読むほど結果セットが肥大し、追加の遅延を生んでいた
- **フロントエンドの invalidation cascade:** MarkAsRead 完了後に stats / unreadCount / read の 3 クエリを同時 invalidate し、バックエンドの負荷を増幅していた
- **perf timer のキャッシュ計測欠如:** `cache_hit: false` / `cache_ms: 0` が常時記録されており、SharedCache の実効状態が外部から観測不能だった
- **SharedCache TTL が短い（2 分）:** 3 分ごとに全エントリが同時 expire するため、cold cache storm が頻発していた

## 対応の評価

### うまくいったこと

- `feed_read_perf` の構造化ログと OTel スパンにより、問題箇所（usecase_ms = 424 ms）を迅速に特定できた
- 既存の `FetchUnreadFeedsListCursor` SQL クエリが十分に最適化されていたため（NOT EXISTS + composite index + cursor pagination）、修正が「新しいコードを書く」ではなく「既存コードへの委譲」で完結した
- TDD ワークフローにより、修正の正当性をテストで即座に検証できた

### うまくいかなかったこと

- 閾値アラートがなかったため、ユーザー報告まで問題が検知されなかった
- perf timer が SharedCache のヒット/ミスを正確に計測しておらず、キャッシュ層の実効性が不透明だった

### 運が良かったこと

- 効率的な SQL クエリが driver 層に既に実装されていたこと。これがなければ新規実装が必要で、修正に大幅に時間がかかっていた
- フロントエンドのオプティミスティック更新により、MarkAsRead の体感遅延はある程度緩和されていた（UI 上はフィードが即座に消えるが、バックグラウンドのリクエストが遅い状態）

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|---|---|---|---|---|
| 1 | 予防 | `CachedFeedListUsecase` を単一 SQL ポート委譲に切り替え、fan-out N+1 を除去 | 開発担当者 | 2026-04-06 | **完了** |
| 2 | 予防 | `GetAllReadFeedIDs` に LIMIT 10000 を追加 | 開発担当者 | 2026-04-06 | **完了** |
| 3 | 予防 | フロントエンド invalidation cascade を削減（optimistic count + active-only refetch） | 開発担当者 | 2026-04-06 | **完了** |
| 4 | 検知 | `feed_read_perf` の `usecase_ms` に閾値アラートを追加（p95 > 100 ms で発報） | 開発担当者 | 2026-04-20 | TODO |
| 5 | 検知 | perf timer に SharedCache のヒット/ミス率の正確な計測を追加 | 開発担当者 | 2026-04-20 | TODO |
| 6 | プロセス | キャッシュ導入時のレビューチェックリストに「cache miss 時のコスト」と「コネクションプールへの影響」を追加 | 開発担当者 | 2026-04-30 | TODO |

## 教訓

### 技術的な学び

- **キャッシュは万能薬ではない。** キャッシュヒット時の性能だけでなく、キャッシュミス時のフォールバックコスト（特に fan-out パターンの場合）とコネクションプールへの副作用を必ず評価する必要がある。今回のケースでは、キャッシュなしの単一 SQL クエリ（2–18 ms）の方が、キャッシュありの bimodal 分布（2 ms / 424 ms）よりも一貫した良い UX を提供した。
- **既存コードを知る。** 効率的な SQL クエリが既に存在していたにもかかわらず、別の経路（アプリケーション層のキャッシュ + マージ）で同じ機能を再実装してしまった。コードベースの既存実装を十分に調査してから新しいアーキテクチャを導入すべきだった。
- **bimodal latency は p50 では見えない。** 通常 2 ms、spike 424 ms の分布は p50 / average では「高速」に見える。パフォーマンス評価には p95 / p99 の監視が不可欠。

### 設計原則の再確認

- PostgreSQL の NOT EXISTS anti-join は十分に高速であり、適切なインデックスがあればアプリケーション層のキャッシュよりもシンプルで安定した選択肢になりうる
- PgBouncer のコネクションプールは共有リソースであり、1 つのリクエストが多数のコネクションを占有すると他のリクエストに波及する。fan-out クエリパターンはこの共有リソースの公平性を損なう

## 参考資料

- [[000624]] CachedFeedListUsecase の fan-out N+1 パターンを除去し単一 SQL 委譲に切り替える
- [[000527]] loadMergedFeeds 並列化と Connect-RPC OTel 計装で /feeds 初期表示レイテンシを改善
- [[000582]] GetUnreadFeeds パフォーマンス改善 — BFF キャッシュ・covering partial index・ReadState キャッシュの 3 層対策
- [Microservice Anti-Pattern: Data Fan Out (AKF Partners)](https://akfpartners.com/growth-blog/microservice-anti-pattern-data-fan-out)
- [PgBouncer Connection Queuing (Percona)](https://www.percona.com/blog/connection-queuing-in-pgbouncer-is-it-a-magical-remedy/)

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
