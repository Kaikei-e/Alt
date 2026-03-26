# PM-2026-009: Ask Augur チャットストリームが日本語テキストの不正 UTF-8 でフロントエンドに到達せず途中停止

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-3（主要機能の部分的障害：チャット応答が途中で停止し、ユーザー操作が不能になる） |
| 影響期間 | 構造的問題のため、日本語テキストを含む記事への Ask Augur クエリで常に再現可能。[[000595]] のインラインペイン実装後に顕在化 |
| 影響サービス | rag-orchestrator |
| 影響機能 | Ask Augur（RAG ベースの対話型回答）、Morning Letter |
| 関連 ADR | [[000595]], [[000568]], [[000579]] |
| 関連 PM | [[PM-2026-008-ask-augur-ollama-parameter-mismatch-model-reload]] |

## サマリー

Knowledge Home のインラインチャットペイン（[[000595]]）を実装した後、Ask Augur のチャットが途中で停止する事象が報告された。ストリーミングのテキストチャンク（delta イベント）はフロントエンドに到達するが、最終的な完了イベント（done）が届かず、UI のローディング状態が解除されない。

根本原因は 2 つの独立した問題の複合:

1. **Citation UTF-8 サニタイズ漏れ**: rag-orchestrator の Connect-RPC ハンドラーにおいて、Citation メタデータの一部の string フィールドに `sanitizeUTF8()` が適用されていなかった。protobuf シリアライゼーション時に `"string field contains invalid UTF-8"` エラーが発生し done イベントの送信に失敗

2. **Ollama structured output の unescaped quote 問題**: Ollama の constrained decoding が JSON string 値内の `"` を `\"` にエスケープしないケースがある（[ollama/ollama#10929](https://github.com/ollama/ollama/issues/10929)）。特に HTML コード片（`<meta property="og:title">`）を含む回答で発生。incremental parser が unescaped `"` を answer フィールドの終端と誤認し、回答テキストが途中で切り捨てられる

## 影響

- **Ask Augur チャット**: delta チャンク（テキスト）は表示されるが、done イベント未到達により `isLoading` が `true` のまま固定。入力欄が `disabled` のためユーザーは追加質問を送信できない
- **影響の再現条件**: 日本語テキストを含む記事に対する質問で高確率で発生。英語のみの記事では再現しにくい
- **ログ上の証拠**: rag-orchestrator で `"string field contains invalid UTF-8"` が `augur stream chat completed` の 3〜4 秒後に出力される

### 実測ログ

| 時刻 (UTC) | イベント | 備考 |
|---|---|---|
| 12:13:51 | `augur stream chat completed` | ストリーム正常完了 |
| 12:13:54 | `string field contains invalid UTF-8` | done イベント送信失敗 |
| 12:51:05 | `augur stream chat completed` | 2 回目の試行 |
| 12:51:09 | `string field contains invalid UTF-8` | 再現 |

## タイムライン

| 時刻 | イベント |
|---|---|
| 2026-03-26 午前 | [[000595]] インラインチャットペイン実装・デプロイ |
| 2026-03-26 12:51 (UTC) | ユーザーから「チャットが途中で止まる」との報告 |
| 2026-03-26 12:55 | rag-orchestrator ログ調査開始。`string field contains invalid UTF-8` を確認 |
| 2026-03-26 13:00 | PM-2026-008 および [[000579]] を参照し、過去の Ollama UTF-8 関連問題との類似性を確認 |
| 2026-03-26 13:05 | `augur/handler.go` の `sanitizeUTF8()` 適用範囲を精査。Citation の `PublishedAt` と `RetrieveContext` の全フィールドに未適用であることを特定 |
| 2026-03-26 13:10 | `morning_letter/handler.go` でも同様の未適用箇所を発見 |
| 2026-03-26 13:15 | TDD で修正実装。テスト追加 → GREEN 確認 → 全テスト PASS |
| 2026-03-26 13:20 | rag-orchestrator コンテナ再ビルド・起動 |

## 根本原因分析

### Five Whys

1. **なぜ Ask Augur のチャットが途中で止まったか？**
   → done イベントが protobuf シリアライゼーションに失敗し、フロントエンドに到達しなかったため

2. **なぜ protobuf シリアライゼーションが失敗したか？**
   → Citation メタデータの string フィールドに不正な UTF-8 バイト列が含まれていたため。protobuf3 は string フィールドに有効な UTF-8 を要求する

3. **なぜ Citation に不正 UTF-8 が含まれていたか？**
   → `convertContextsToCitations()` の `PublishedAt` フィールドと、`RetrieveContext` の全フィールドに `sanitizeUTF8()` が適用されていなかったため

4. **なぜ `sanitizeUTF8()` が一部にしか適用されていなかったか？**
   → `sanitizeUTF8()` は delta/done/error 等のメインストリームイベントの Answer テキストに対して導入された（既知の問題への対処）が、Citation メタデータ（URL、Title、PublishedAt）は「DB 由来の安全なデータ」として暗黙的に信頼されていた

5. **なぜ DB 由来のデータに不正 UTF-8 が混入していたか？**
   → RSS フィードから取得した記事メタデータが、フィード配信元のエンコーディング問題や中間処理での文字化けにより不正 UTF-8 を含んでいた。DB（PostgreSQL）は `bytea` ではなく `text` 型で格納しているが、Go の `database/sql` ドライバーはバイト列をそのまま string に変換するため、不正 UTF-8 がそのまま通過する

### 寄与要因

- **日本語固有**: 日本語は 3 バイト UTF-8 エンコーディングのため、1 バイトの欠損で不正シーケンスが発生しやすい。ASCII (1 バイト) ではバイト境界の問題が顕在化しにくい
- **[[000595]] による顕在化**: フルページの AugurChat (`/augur`) では `streamAugurChat()` のフォールバック（done イベントなしでもストリーム終了時に `onComplete` を呼ぶ）が機能していた可能性がある。インラインペインでも同じ `streamAugurChat()` を使用しているが、done イベント失敗時のタイミング差で挙動が異なった

## 対応の評価

### うまくいったこと

- PM-2026-008 と [[000579]] の過去事例が調査の起点になり、Ollama + 日本語 + UTF-8 の問題領域を即座に絞り込めた
- `sanitizeUTF8()` ユーティリティが既に存在しており、適用範囲を拡大するだけで修正が完了した
- rag-orchestrator の構造化ログ（`augur stream chat completed` → `string field contains invalid UTF-8` のタイムスタンプ差分）で問題箇所を正確に特定できた

### 改善が必要なこと

- `sanitizeUTF8()` の適用が手動・個別で、新しい protobuf フィールドを追加する際に漏れが発生しやすい
- protobuf 送信前に一括で UTF-8 バリデーションを行う仕組みがない
- フロントエンドに `onComplete` が届かない場合の UX が「永久ローディング」で、ユーザーにエラーが伝わらない

## 教訓

### 技術的教訓

1. **protobuf3 の string フィールドは有効な UTF-8 が必須**: Go の `string` 型は任意のバイト列を保持できるが、protobuf シリアライゼーション時にバリデーションが走る。外部データ（DB、RSS フィード、LLM 出力）を protobuf に渡す全箇所で `sanitizeUTF8()` が必要
2. **「DB 由来 = 安全」は誤った仮定**: RSS フィードから取得したメタデータは、配信元のエンコーディング問題を引き継ぐ。信頼境界は protobuf シリアライゼーション直前に設定すべき
3. **日本語環境では UTF-8 エッジケースの発生確率が高い**: 1 バイト文字（ASCII）では問題にならないバイト境界の分断が、3 バイト文字（日本語）では高確率で不正シーケンスを生む

### PM-2026-008 との関連

PM-2026-008 は Ollama パラメータ不一致によるモデルリロード遅延（TTFT 劣化）で、[[000579]] で修正済み。本事象は同じ Ask Augur ストリームパスで発生しているが、原因は異なる（パラメータ不一致ではなく UTF-8 バリデーション漏れ）。共通点は「Ollama + 日本語テキスト + ストリーミング」のデータパスで、異なるレイヤーの問題が順に顕在化した形。

## アクションアイテム

### 予防（Prevent）

| # | アクション | 期限 | 状態 |
|---|---|---|---|
| P-1 | `augur/handler.go` の `convertContextsToCitations` で `PublishedAt` に `sanitizeUTF8()` 適用 | 2026-03-26 | **完了** |
| P-2 | `augur/handler.go` の `RetrieveContext` で全 string フィールドに `sanitizeUTF8()` 適用 | 2026-03-26 | **完了** |
| P-3 | `morning_letter/handler.go` の Citation 変換 2 関数に `sanitizeUTF8()` 追加 | 2026-03-26 | **完了** |
| P-4 | `rag_answer_stream.go` の incremental parser に lookahead ロジック追加。unescaped `"` の後に JSON 構造の続き（`,` or `}`）が来るかを検証し、embedded quote を正しくスキップ（[[000596]]） | 2026-03-26 | **完了** |

### 検知（Detect）

| # | アクション | 期限 | 状態 |
|---|---|---|---|
| D-1 | `"string field contains invalid UTF-8"` ログメッセージに対するアラートルール追加 | 2026-04-07 | 未着手 |
| D-2 | フロントエンドで done イベント未到達時のタイムアウト処理追加（30 秒で自動 fallback） | 2026-04-14 | 未着手 |

### 緩和（Mitigate）

| # | アクション | 期限 | 状態 |
|---|---|---|---|
| M-1 | Connect-RPC ハンドラーレベルで protobuf 送信前に全 string フィールドを一括 UTF-8 バリデーションするミドルウェアの検討 | 2026-04-14 | 未着手 |
