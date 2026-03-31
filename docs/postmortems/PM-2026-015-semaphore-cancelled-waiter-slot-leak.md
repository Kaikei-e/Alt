# PM-2026-015: HybridPrioritySemaphore CancelledError 時スロット消失 — オンザフライ要約 551秒キュー待ち

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-3（主要機能の部分的劣化：オンザフライ要約遅延。完全停止ではない） |
| 影響期間 | 2026-03-29 〜 2026-03-31（[[000610]] デプロイ後も継続。構造的バグのため潜在期間は不明） |
| 影響サービス | news-creator |
| 影響機能 | オンザフライ・ストリーミング要約（StreamSummarize）、BE バッチ要約 |
| 関連 ADR | [[000612]], [[000610]], [[000601]], [[000606]] |
| 関連 PM | [[PM-2026-014-news-creator-slow-summarization-recap-degradation]], [[PM-2026-012-visual-swipe-summarize-semaphore-slot-leak]] |

## サマリー

[[000610]] の `call_soon_threadsafe` 修正デプロイ後も、`HybridPrioritySemaphore` で **86件の SLOT INVARIANT VIOLATION** と**最大 551秒のキュー待ち**が継続していた。ログ調査の結果、[[000610]] とは異なるリーク経路を特定した。`release()` がスロットを `_try_wake_waiter()` 経由で次の waiter に転送した後、その waiter の asyncio タスクが HTTP クライアント切断等でキャンセルされると、`acquire()` の `CancelledError` ハンドラが転送済みスロットを回収せず、スロットが永久に消失していた。副次的に、`release()` 末尾の invariant check がスロット「転送中」の一時的状態を false positive として検出し、86件の ERROR ログを生成していた。

## 影響

- **オンザフライ要約**: RT ストリーミングリクエストのキュー待ちが **31〜75秒**（通常 0秒）
- **BE バッチ要約**: キュー待ちが最大 **551.59秒**（9分超、通常 0秒）
- **SLOT INVARIANT VIOLATION**: ERROR レベルで **86回** ログ出力（可観測性のノイズ）
- **COLD_START**: WARNING レベルで **183回**（minor: 0.12〜0.23秒、モデル不一致ではない）
- **データ損失**: なし
- **SLO/SLA 違反**: なし（パフォーマンス劣化のみ）

## タイムライン

| 時刻 | イベント |
|------|---------|
| 2026-03-29 21:33 JST 頃 | news-creator コンテナ再ビルド（[[000610]] 修正含む）|
| 2026-03-29 21:49 JST | [[000610]] 修正コミット（`_try_wake_waiter` 導入）。コンテナはこのコミットを含むイメージで稼働中 |
| 2026-03-29 〜 2026-03-31 | SLOT INVARIANT VIOLATION が継続的に発生（86件。手動ログ分析でのみ検知可能） |
| 2026-03-31 11:57 JST | BE リクエストで **281秒** のキュー待ちが発生（TTFT=282.84s） |
| 2026-03-31 16:22 JST | プリエンプション発動 → RT ストリーミング取得 → BE 記事がプリエンプションで失敗 |
| 2026-03-31 16:30 JST | 新規 RT ストリーミングリクエストが queue_size=3 でキュー待ち |
| 2026-03-31 16:31 JST | BE リクエストで **551.59秒** のキュー待ちが発生（TTFT=552.22s） |
| 2026-03-31 16:31 JST | **検知**: ユーザーがオンザフライ要約の体感劣化を確認、コンテナログ調査を開始 |
| 2026-03-31 16:45 JST 頃 | **原因特定**: `acquire()` の `CancelledError` ハンドラでスロット回収が欠如していることを特定 |
| 2026-03-31 16:50 JST | **修正実装**: TDD で失敗テスト 4件 → 実装 → 全 327件パス |
| 2026-03-31 16:50 JST | **復旧**: news-creator 再ビルド・起動。SLOT INVARIANT VIOLATION 消失を確認 |

## 検知

- **検知方法**: ユーザーによる手動確認（オンザフライ要約の体感劣化 + コンテナログ分析）
- **検知までの時間 (TTD)**: 不明（構造的バグのため発生時刻が特定不能。[[000610]] デプロイ直後から潜在的に発生していた可能性が高い）
- **検知の評価**: [[PM-2026-014-news-creator-slow-summarization-recap-degradation]] D-1 で提案された `SLOT INVARIANT VIOLATION` アラートが未実装のため、手動ログ分析でのみ検知可能だった。同アラートが設定されていれば、[[000610]] デプロイ直後に検知できた可能性がある

## 根本原因分析

### 直接原因

`release()` が `_try_wake_waiter()` でスロットを次の waiter に転送した後、waiter の asyncio タスクが HTTP クライアント切断等でキャンセルされると、`acquire()` の `CancelledError` ハンドラがスロット回収を行わず、スロットが永久に消失する。

### Five Whys

1. **なぜスロットが消失したか？** → `release()` が `_track_release(slot_id)` で `_acquired_slots` からスロットを削除し、`_try_wake_waiter()` で `set_result(home_pool)` を呼んだ後、waiter のタスクがキャンセルされた。`woke_up=True` のためプールカウンタは非加算で return したが、waiter 側の `_track_acquire()` は `CancelledError` により呼ばれなかったため
2. **なぜ `CancelledError` ハンドラがスロットを回収しなかったか？** → `future.done()=True`（`set_result()` 済み）のため future のキャンセルはスキップされたが、転送済みスロットの回収ロジックが実装されていなかったため
3. **なぜ転送済みスロットの回収が欠如していたか？** → `acquire()` の `CancelledError` ハンドラは元々「キューから外れる」ケース（`future.done()=False`）のみを想定しており、「スロット転送済みだがまだ追跡されていない」という transient state が考慮されていなかったため
4. **なぜこの状態が[[000610]]の修正後に顕在化したか？** → [[000610]] で `call_soon_threadsafe` を直接 `set_result()` に置換したことにより、`set_result()` が同期的に即時実行されるようになった。これにより `release()` が return する前にスロット転送が完了し、`acquire()` 側で `await future` が即座に完了するようになったが、その直後の `_track_acquire()` 前にタスクキャンセルが入る窓が新たに生まれた
5. **なぜテストで発見されなかったか？** → 既存のキャンセルテスト（`test_be_slots_zero_cancelled_waiter_no_slot_loss`）は waiter の future がキャンセルされた後に `release()` が呼ばれるケース（`_try_wake_waiter` が `False` を返す）をテストしていた。`set_result()` 成功後にタスクがキャンセルされるケースのテストが存在しなかった

### 寄与要因

- **`be_slots=0` 構成**: `total_slots=1, rt_reserved=1` で運用しているため、1スロットの消失が全リクエストのブロックに直結する
- **INVARIANT VIOLATION の false positive**: `release()` 末尾の invariant check がスロット転送中の transient state を ERROR として検出しており、真のスロットリークと false positive の区別がログ上で不可能だった。86件の ERROR のうち、真のリークと false positive が混在していた
- **[[PM-2026-014-news-creator-slow-summarization-recap-degradation]] D-1 の未着手**: `SLOT INVARIANT VIOLATION` アラートが未設定のため、自動検知ができなかった

## 対応の評価

### うまくいったこと

- news-creator の構造化ログ（`SLOT INVARIANT VIOLATION`、`Long queue wait detected`、`TTFT breakdown`）が根本原因の特定に直結した
- `slot_id` の連番追跡（[[000606]]）により、スロットの acquire → release チェーンをログから正確に再構成できた
- TDD ワークフロー（RED 4件 → GREEN → REFACTOR）で修正の品質を保証できた。全 327件パス

### うまくいかなかったこと

- [[000610]] の修正コミット時に、`CancelledError` パスのスロット回収を見落とした。`release()` 側のリーク修正に集中し、`acquire()` 側の transient state を考慮しなかった
- INVARIANT VIOLATION の false positive が真のリークを覆い隠していた。86件の ERROR がすべて false positive に見えるため、真のスロットリークの検知が困難だった
- [[PM-2026-014-news-creator-slow-summarization-recap-degradation]] で D-1（INVARIANT VIOLATION アラート）が未着手のまま放置されており、2日間の検知遅延に直結した

### 運が良かったこと

- スロットリーク後も、次の `release()` → `_try_wake_waiter()` チェーンで waiter のタスクがキャンセルされなかった場合はスロットが回復するため、完全な処理停止には至らなかった。ただしこれは確率的な回復であり、依存すべきではない
- リモート GPU への分散 BE ディスパッチが有効だったため、ローカルスロットが消失しても一部の BE リクエストはリモートで処理された

## アクションアイテム

### 予防（Prevent）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| P-1 | `acquire()` の `CancelledError` ハンドラで転送済みスロットの回収ロジックを追加（[[000612]]） | 開発担当者 | 2026-03-31 | **完了** |
| P-2 | `release()` の invariant check で `woke_up=True` 時の transient state を正常とみなす修正（[[000612]]） | 開発担当者 | 2026-03-31 | **完了** |
| P-3 | HybridPrioritySemaphore の property-based testing（Hypothesis）導入 | 開発担当者 | 2026-04-25 | 未着手 |

### 検知（Detect）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| D-1 | `SLOT INVARIANT VIOLATION` ログに対するアラートルール追加（[[PM-2026-014-news-creator-slow-summarization-recap-degradation]] D-1 と同一） | 開発担当者 | 2026-04-14 | 未着手 |
| D-2 | コンテナ再ビルド後 30分以内の INVARIANT VIOLATION 発生を検出する smoke test 追加 | 開発担当者 | 2026-04-14 | 未着手 |

### 緩和（Mitigate）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| M-1 | `total_slots==1` 時にプリエンプションを自動無効化する安全弁追加（[[PM-2026-014-news-creator-slow-summarization-recap-degradation]] M-1 と同一） | 開発担当者 | 2026-04-07 | 未着手 |

### プロセス（Process）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| O-1 | HybridPrioritySemaphore 修正時のテストチェックリスト作成: `acquire()` 側の全 exit path（immediate return、`await future` 完了、`CancelledError`）でスロット invariant を検証すること | 開発担当者 | 2026-04-07 | 未着手 |

## 教訓

### 技術的教訓

1. **スロット転送の「in transit」状態は明示的に追跡すべき**: `release()` がスロットを `set_result()` で転送した後、`acquire()` 側で `_track_acquire()` が呼ばれるまでの間、スロットはどのデータ構造にも属さない。この transient state を invariant check で考慮しないと、false positive がノイズになり真のリークを覆い隠す
2. **`CancelledError` ハンドラは「持っているリソース」を棚卸しすべき**: asyncio の `CancelledError` は任意の `await` ポイントで発生する。ハンドラは「この時点でどのリソースが取得済みだが返却されていないか」を網羅的に確認すべき。特に future に `set_result()` 済みの場合、そのリソースは呼び出し側に「渡されたが受け取られていない」状態であり、回収が必要
3. **同一コンポーネントの連続インシデントは、テスト設計の盲点を示す**: HybridPrioritySemaphore は PM-2026-004/006/012/014/015 で 5回のインシデントを引き起こした。各修正は直前のバグを修正するが、新たなコードパスの副作用を見落とす。Property-based testing の導入が急務

### 組織的教訓

1. **アクションアイテムの未着手は再発を招く**: PM-2026-014 D-1（INVARIANT VIOLATION アラート）が未着手のまま 2日間放置され、本インシデントの検知遅延に直結した。未着手アイテムの定期レビューが必要

## 参考資料

- [[000612]] HybridPrioritySemaphore の CancelledError 時スロット回収と invariant check の false positive 排除
- [[000610]] HybridPrioritySemaphore の call_soon_threadsafe 排除によるスロットリーク修正
- [[PM-2026-014-news-creator-slow-summarization-recap-degradation]] 先行する複合障害 PM
- [[PM-2026-012-visual-swipe-summarize-semaphore-slot-leak]] 先行するスロットリーク PM

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
