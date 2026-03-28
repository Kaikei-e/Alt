# PM-2026-013: Ask Augur follow-up クエリのタイムアウト — planner 過剰 clarification + handler event ドロップ + セマフォスロットリーク

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-3（主要機能の部分的障害：follow-up クエリが全て無応答。初回クエリは正常） |
| 影響期間 | 2026-03-28 [[000608]] デプロイ後 〜 2026-03-29 修正デプロイ（構造的問題のため常に再現） |
| 影響サービス | rag-orchestrator, news-creator |
| 影響機能 | Ask Augur（RAG 対話型回答）の follow-up クエリ全般 |
| 関連 ADR | [[000608]], [[000609]], [[000604]], [[000606]], [[000556]] |
| 関連 PM | [[PM-2026-012-visual-swipe-summarize-semaphore-slot-leak]] |

## サマリー

[[000608]] の品質ゲート修正（`stream_short_answer_hard_stop` の誤検知排除）をデプロイした直後から、Ask Augur の follow-up クエリ（`PyO3について詳しく教えて`、`そのリスクについて詳しく教えて` 等）が全てタイムアウトする事象が発生した。初回クエリは正常に回答が返るが、2ターン目以降の follow-up が無応答となる。

根本原因は **3つの独立した問題の複合**:

1. **ConversationPlanner の過剰 clarification 判定**: `isAmbiguousFollowUp()` が `詳しく教えて` を含む全クエリを ambiguous と判定。ConversationStore への state 書き込みが未接続のため state は常に nil → 即座に `NeedsClarification=true` を返す
2. **Connect-RPC handler の clarification event ドロップ**: `convertStreamEvent()` に `StreamEventKindClarification` の case がなく、default case で skip → FE に何も届かずタイムアウト
3. **HybridPrioritySemaphore のスロットリーク**: `.env` の設定ドリフト（`OLLAMA_NUM_PARALLEL=2`）と `gemma3-4b-8k` 残存により、`total_slots=1` の設計意図に反して `Parallel:2` で稼働。是正後は `be_slots=0` 構成でのスロット消失バグ（SLOT INVARIANT VIOLATION）が顕在化

## 影響

- **Follow-up クエリの成功率**: 0%（clarification event がドロップされ、FE にレスポンスが到達しない）
- **初回クエリ**: 正常動作（planner は explicit SubIntent を持つクエリに clarification を要求しない）
- **ログ上の証拠**:
  - `"unknown stream event kind" kind:"clarification"` — 3件
  - follow-up の `augur stream chat completed` が開始から 1ms 未満で完了（生成なし）
- **セマフォ**: `SLOT INVARIANT VIOLATION` が 10件以上発生。`rt_available=0, be_available=0, acquired_count=0`（スロット完全消失）
- **データ損失**: なし
- **他機能への影響**: バッチ要約がセマフォスロットリークの影響で 36秒のキュー待ち発生

### なぜ品質ゲート修正後に顕在化したか

[[000608]] 以前は全回答が `stream_short_answer_hard_stop` で fallback 送信 → ストリームが終了していた。品質ゲートの誤検知修正により正常なストリームフローが初めて走り、**planner の clarification 判定が実行されるパスが活性化**した。

## タイムライン

全時刻は 2026-03-28 UTC。

| 時刻 (UTC) | イベント |
|---|---|
| 14:40 | [[000608]] 修正を含む rag-orchestrator を再ビルド・デプロイ |
| 15:17 | ベンチマーク（`benchmark_augur.sh`）完了。article-scoped 4/4 PASS を確認 |
| 15:24:00 | 初回クエリ（記事「R CausalImpact を Rust+PyO3 で再実装」の技術詳細）が正常に回答 (31s) |
| 15:24:48 | **発生**: follow-up `PyO3について詳しく教えて` → planner: `clarify, confidence:0.3` → `unknown stream event kind` → 即座に完了 (1ms) |
| 15:28:48 | 同クエリ再送信 → 同一パターンで失敗 |
| 15:30:46 | 別トピック初回クエリ（イランの石油危機）は正常に回答 (17s) |
| 15:32:24 | follow-up（米軍の上陸作戦の島）は正常に回答 (9s) — planner が clarify を返さなかった |
| 15:32:55 | 2ターン目 follow-up `そのリスクについて詳しく教えて` → planner: `clarify, confidence:0.3` → 即座に完了 |
| 16:00:44 | **セマフォ障害**: `SLOT INVARIANT VIOLATION` 初発。以降連続発生 |
| 16:03:00 | バッチ要約が 36秒のキュー待ち (`Long queue wait detected: 36.24s`) |
| 16:03:05 | プリエンプション発動 → `PreemptedException` → 500 Internal Server Error |
| ― | **原因特定**: rag-orchestrator ログで `planner_output operation:clarify` パターンを確認。handler ログで `unknown stream event kind: clarification` を確認 |
| ― | **修正実装**: Fix 1 (planner 厳密化), Fix 2 (handler mapping), Fix 3 (12k 統一), Fix 4 (state 永続化) |
| ― | **デプロイ**: rag-orchestrator + news-creator + news-creator-backend を再ビルド |
| ― | **復旧確認**: ベンチマーク 7/9 PASS、follow-up タイムアウト解消、`unknown stream event kind` 0件 |

## 根本原因分析

### Five Whys

1. **なぜ follow-up クエリがタイムアウトしたか？**
   → FE に clarification event が到達せず、done event もなく、ストリームが即座に終了したため

2. **なぜ clarification event が FE に到達しなかったか？**
   → Connect-RPC handler の `convertStreamEvent()` に `StreamEventKindClarification` の case がなく、default case で skip されたため

3. **なぜ clarification event が送信されたか？**
   → ConversationPlanner が全ての `詳しく教えて` 含有クエリを ambiguous と判定し、state が nil のため即座に clarification を要求したため

4. **なぜ state が常に nil だったか？**
   → `ConversationStore.Put()` が `Stream()` の完了パスに接続されておらず、`DeriveStateUpdate()` が呼ばれていなかったため（ADR-000604 の実装が不完全）

5. **なぜ [[000608]] のデプロイ前にこの問題が検出されなかったか？**
   → 品質ゲートの誤検知（`stream_short_answer_hard_stop`）が全回答を fallback で終了させていたため、planner の clarification パスに到達する前にストリームが停止していた。品質ゲート修正で正常フローが活性化し、隠れていた3つのバグが同時に露出した

### 寄与要因

- **設定ドリフト**: `.env` に旧設定（`OLLAMA_NUM_PARALLEL=2`、`OLLAMA_MODEL=gemma3-4b-8k`）が残存し、compose/ai.yaml のデフォルト（`1`、`gemma3-4b-12k`）をオーバーライドしていた
- **テストギャップ**: planner のテストは `state != nil` のケースを中心にカバーしていたが、`state == nil + history あり` のフォールバックパスがテストされていなかった
- **handler のイベントマッピング漏れ**: `StreamEventKindClarification` は ADR-000604 で型定義されたが、Connect-RPC handler への case 追加がコミットされていなかった
- **cascading failure**: 品質ゲート修正（望ましい変更）が隠れていた3つの独立バグを同時に露出させた

## 対応の評価

### うまくいったこと

- rag-orchestrator の構造化ログ（`planner_output operation:clarify`、`unknown stream event kind`）により問題箇所を迅速に特定できた
- `ConversationStore`、`DeriveStateUpdate`、`StreamEventKindClarification` の型定義が既に存在しており、接続するだけで機能した
- TDD による修正で既存テスト全パスを確認しながら安全にデプロイできた

### 改善が必要なこと

- `StreamEventKind` を追加した際に handler のマッピング追加が漏れた。新しい event kind を追加する際のチェックリストが必要
- `ConversationStore.Put()` の接続が ADR-000604 の実装時に漏れた。DI 注入されたコンポーネントの「read-only 使用」を検出するテストパターンが必要
- `.env` と compose defaults の乖離を検出する仕組みがない。startup assertion で `.env` の値と期待値を照合するべき

### 運が良かったこと

- 初回クエリ（explicit SubIntent あり）は影響を受けなかったため、Ask Augur が完全に使用不能にはならなかった
- セマフォの SLOT INVARIANT VIOLATION は COLD_START の load_duration が 0.1-0.3s と軽微で、PM-2026-008 の 259s タイムアウトほど深刻ではなかった

## 教訓

### 技術的教訓

1. **品質ゲートの修正は隠れたバグを露出させる**: 過剰な品質ゲートがエラーハンドリングのショートカットとして機能しており、その除去で downstream のバグが同時に顕在化した。品質ゲート変更時は、正常フローの全パスを end-to-end でテストすべき
2. **新しい StreamEventKind は handler マッピングとセットで追加すべき**: usecase 層で新しいイベント種別を定義しても、adapter 層での変換が漏れると無言で失敗する
3. **DI 注入 != 機能有効化**: コンポーネントを DI で注入しても、read パスしか使われていない場合がある。write パス（state 永続化）の接続は別途テストで検証すべき
4. **`.env` のドリフトは時限爆弾**: compose defaults と `.env` の乖離は、コンテナ再ビルド時に意図しない設定で起動する原因になる

### PM-2026-012 との関連

PM-2026-012 は `total_slots=2, rt_reserved=1` でのスロットリークを `home_pool` tracking で修正した。本事象では `total_slots=1`（RAG 専用 single-slot 構成）での新たなスロットリークが発生。`be_slots=0` 構成ではセマフォの acquire が available チェックなしに成功し続ける問題が判明し、根本的な `be_slots=0` パス監査が必要。

## アクションアイテム

### 予防（Prevent）

| # | アクション | 期限 | 状態 |
|---|---|---|---|
| P-1 | `isAmbiguousFollowUp()` をパターン残余チェック方式に変更（具体的トピック付きクエリを除外） | 2026-03-29 | **完了** |
| P-2 | `resolveAmbiguous()` で state=nil + history 非空の場合に OpDetail フォールバック追加 | 2026-03-29 | **完了** |
| P-3 | `convertStreamEvent()` に `StreamEventKindClarification` の case を追加 | 2026-03-29 | **完了** |
| P-4 | `Stream()` 完了パスで `ConversationStore.Put()` を接続し state を永続化 | 2026-03-29 | **完了** |
| P-5 | `.env` / `.env.template` / `pre-processor` / `recap-evaluator` を gemma3-4b-12k に統一 | 2026-03-29 | **完了** |
| P-6 | HybridPrioritySemaphore の `be_slots=0` パス全体を監査し、acquire の overcounting を修正 | 2026-04-07 | 未着手 |
| P-7 | `total_slots==1` 時にプリエンプションを自動無効化する安全弁を追加 | 2026-04-07 | 未着手 |

### 検知（Detect）

| # | アクション | 期限 | 状態 |
|---|---|---|---|
| D-1 | `unknown stream event kind` ログに対するアラートルール追加 | 2026-04-14 | 未着手 |
| D-2 | `SLOT INVARIANT VIOLATION` ログに対するアラートルール追加 | 2026-04-14 | 未着手 |
| D-3 | Ask Augur の follow-up 成功率メトリクスをダッシュボードに追加 | 2026-04-14 | 未着手 |

### 緩和（Mitigate）

| # | アクション | 期限 | 状態 |
|---|---|---|---|
| M-1 | 新しい `StreamEventKind` 追加時に handler マッピング追加を要求するチェックリスト作成 | 2026-04-07 | 未着手 |

### プロセス（Process）

| # | アクション | 期限 | 状態 |
|---|---|---|---|
| O-1 | 品質ゲート変更時の end-to-end follow-up テスト実行をデプロイチェックリストに追加 | 2026-04-07 | 未着手 |
| O-2 | `.env` と compose defaults の乖離を検出する startup assertion の設計検討 | 2026-04-14 | 未着手 |
