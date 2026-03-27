# PM-2026-012: Visual Swipe UI StreamSummarize タイムアウト — HybridPrioritySemaphore スロットリーク

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-3（主要機能の一時停止、ただし他機能は正常。データ損失なし） |
| 影響期間 | 2026-03-27 18:35 JST 〜 18:50 JST（約 15 分間） |
| 影響サービス | news-creator（HybridPrioritySemaphore）、alt-backend（StreamSummarize ハンドラ） |
| 影響機能 | Visual Swipe UI のオンザフライ AI 要約生成（StreamSummarize） |
| 関連 ADR | [[000601]], [[000553]], [[000556]], [[000567]] |

## サマリー

Visual Swipe UI（Mobile Safari）でオンザフライ要約（StreamSummarize）を実行すると、「Summarizing...」のまま 125 秒以上スタックしタイムアウトする事象が発生した。ユーザーが 2 回リトライしたが同様に失敗し、3 回目のリクエストでようやく成功した（140 秒）。原因は `HybridPrioritySemaphore` のプリエンプション → リリースチェーンにおいてスロットの出身プール追跡が欠如しており、RT スロットが BE プールに永久移行するバグだった。ローカル GPU は約 10 分間完全にアイドルだったにもかかわらず、`_rt_available=0` のため RT リクエストがスロットを取得できない work-conserving 違反が発生していた。

## 影響

- **StreamSummarize の失敗**: 2 回のリクエストがタイムアウト（nginx `rt=125s`、レスポンス 63 bytes = ハートビートのみ）
- **影響範囲**: Visual Swipe UI の RT 要約全般。local Ollama / `total_slots=2` / `rt_reserved=1` 構成で、プリエンプション発動後に再現
- **GPU アイドル時間**: 約 10 分間（09:39:35 〜 09:49:28 UTC）ローカル GPU への `/api/generate` リクエスト 0 件
- **データ損失**: なし（バックエンドでは要約が最終的に生成・保存され、キャッシュから表示可能）
- **他機能への影響**: BE バッチ要約、Ask Augur、Recap はリモート GPU またはキュー経由で正常稼働

## タイムライン

全時刻は 2026-03-27 UTC（括弧内は JST）。

| 時刻 (UTC) | イベント |
|---|---|
| 09:35:22 (18:35) | RT ストリーミング #1 が RT スロットを取得。RT ストリーミング #2 が到着し BE プリエンプション発動 |
| 09:36:28 (18:36) | **発生**: BE タスク `7230fa68` がプリエンプションでキャンセル。スロット移行チェーン開始 |
| 09:36:42 (18:36) | RT #1 release → BE キュー待ちに転送。`_rt_available` は 0 のまま復旧せず |
| 09:36:52 (18:36) | RT #2 release → 同様に BE に転送。以降 BE チェーンが継続 |
| 09:39:35 (18:39) | BE チェーン終了。最終状態: `_rt_available=0, _be_available=1`（1 スロット消失）。**ローカル GPU アイドル開始** |
| 09:45:29 (18:45) | **検知**: ユーザーが Visual Swipe UI で要約を実行。RT リクエストがセマフォキューに投入されるが `_rt_available=0` のため取得不可 |
| 09:47:34 (18:47) | ユーザーリトライ #2。同様にキュー待ち。nginx: `rt=125.034s`, 63 bytes |
| 09:49:18 (18:49) | BE バッチ要約（`56f2662f`）が `_be_available=1` を取得 → ローカル GPU 再稼働 |
| 09:49:28 (18:49) | **原因特定**: `56f2662f` 完了 → release → RT キュー発見 → RT 起床。セマフォログ: `Long queue wait detected: 113.33s` |
| 09:49:39 (18:49) | リトライ #2 タイムアウト。nginx: `rt=125.038s`, 63 bytes |
| 09:50:16 (18:50) | リトライ #1 のクライアント切断 (`context canceled`) |
| 09:52:03 (18:52) | **部分復旧**: リトライ #3 が成功（`rt=140.175s`, 38,383 bytes） |
| 同日中 | **恒久修正**: `home_pool` スロット所有権追跡を実装・テスト・コミット（ADR-601） |

## 根本原因分析

### Five Whys

1. **なぜ StreamSummarize がタイムアウトしたか？**
   → RT ストリーミングリクエストが `HybridPrioritySemaphore` のキューで 113 秒間待たされ、ハートビート以外のデータがブラウザに届かなかったため

2. **なぜ RT リクエストがスロットを取得できなかったか？**
   → `_rt_available=0` であり、`_rt_reserved=1` のため BE スロットへのフォールバックも許可されず、プリエンプション対象の BE も存在しなかったため

3. **なぜ `_rt_available` が 0 のままだったか？**
   → プリエンプション → RT→BE リリースチェーンを経て、RT スロットが BE プールに移行。最後の BE が `release(was_high_priority=False)` で返却した際、`_be_available` が `min(+1, be_slots=1)` でキャップされ、余分なスロットが消失したため

4. **なぜスロットが消失したか？**
   → `release()` メソッドがスロットの「出身プール」ではなく「呼び出し元の優先度（`was_high_priority`）」に基づいて返却先を決定しており、プリエンプション経由で移行したスロットの所有権が追跡されていなかったため

5. **なぜ所有権追跡が欠如していたか？**
   → `HybridPrioritySemaphore` の初期設計時に、スロットが acquire 先と異なるプールから release される（プリエンプション経由のクロスプール移行）パスが想定されていなかったため。テストもプリエンプション後の invariant（`available + acquired == total_slots`）を検証していなかった

### 寄与要因

- **長時間の BE Map-Reduce ジョブ**: 記事 `3dc023fb`（16 チャンク）がリモート Ollama に 10 分以上ディスパッチされており、その間 BE キューに複数のリクエストが蓄積。これがプリエンプション発動のトリガーとなった
- **ユーザーリトライによるスパイラル**: 応答が返らないユーザーが同一記事を再送信し、複数の RT リクエストがキュー待ちすることでバックログが増加
- **リモートの 1 台が `ServerDisconnectedError`**: `100.99.107.87:11434` がヘルスチェックに失敗しており、利用可能なリモートが 2 台に減少。チャンクのローカルフォールバック頻度が増加し、セマフォ負荷が上がった

## 対応の評価

### うまくいったこと

- alt-backend のハートビート機構（ADR-553 で導入）が Cloudflare 524 タイムアウトを防止し、セマフォ待ち中もコネクションを維持できた
- news-creator の構造化ログ（`RT request queued`, `Long queue wait detected`, `Acquired semaphore`）により、セマフォキューのスタック状況をログから正確に再構成できた
- news-creator-backend（Ollama）の GIN ログとの突合で、「GPU アイドルなのにセマフォがブロック」という矛盾を定量的に証明できた
- 既存の `_acquired_slots` トラッキングインフラが `home_pool` 拡張の土台となり、修正が最小差分で実装できた

### 改善が必要なこと

- `release()` のスロット返却ロジックに invariant チェック（`available + acquired == total_slots`）がなく、スロット消失を検知できなかった → 修正で追加済み
- プリエンプション後のスロット状態をテストするケースが存在しなかった → TDD で 3 テスト追加済み
- `release(slot_id=...)` のフォールバック（slot_id なし時に oldest matching slot を推定）は、priority だけで出身プールを推定するため ownership バグの温床。段階的な `slot_id` 必須化が必要
- GPU アイドル状態の外部監視メトリクスがなく、スロットリークの検知が手動ログ分析に依存していた

### 運が良かったこと

- BE バッチ要約リクエスト（`56f2662f`）が約 4 分後に到着し、BE スロットを取得 → release 時に RT キューを起床させたことで、状態が自然回復した。BE リクエストが来なければ、RT は永久にブロックされ続けた可能性がある

## 教訓

### 技術的教訓

- **セマフォのスロット管理はスロット所有権（ownership）を追跡すべき**。caller priority に基づく推論は、プリエンプションやキュー転送でスロットがクロスプール移行する場合に破綻する。Trio の `CapacityLimiter` が borrower/token ownership で管理するパターンが参考になる
- **invariant チェックは release パスに入れる**。acquire 側だけでなく、release 時に `available + acquired == total_slots` を検証することで、スロットリークを即時検知できる

### 組織的教訓

- PM-2026-004、PM-2026-006 と合わせて、`HybridPrioritySemaphore` は 3 回目のインシデントを引き起こした。セマフォのような並行制御コンポーネントには、形式的な invariant テストと property-based testing の導入を検討すべき

## アクションアイテム

| カテゴリ | アクション | 担当 | 期限 | 状態 |
|----------|-----------|------|------|------|
| 予防 | `AcquiredSlot.home_pool` による所有権追跡を実装（ADR-601） | — | 2026-03-27 | **完了** |
| 予防 | プリエンプション後のスロット invariant テスト 3 件追加 | — | 2026-03-27 | **完了** |
| 予防 | `release()` 末尾に invariant チェック（ERROR ログ）追加 | — | 2026-03-27 | **完了** |
| 予防 | `release(slot_id=...)` を全 caller で必須化し、フォールバック推定を段階的に廃止 | — | 2026-04-11 | 未着手 |
| 検知 | GPU idle + RT queue non-empty の組み合わせで警告するメトリクス/ログアラート追加 | — | 2026-04-11 | 未着手 |
| 検知 | `rt_available + be_available + acquired != total_slots` を外部監視（Grafana）で可視化 | — | 2026-04-18 | 未着手 |
| 緩和 | プリエンプション無効化（`preemption_enabled=False`）時の RT 遅延許容度を評価し、緊急時の無効化オプションを文書化 | — | 2026-04-18 | 未着手 |
| プロセス | HybridPrioritySemaphore の property-based testing（Hypothesis）導入を検討 | — | 2026-04-25 | 未着手 |
