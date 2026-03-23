# PM-2026-004: StreamSummarize の HTTP 524 タイムアウトおよびストリーミング未表示

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-3（主要機能の停止、ユーザー操作の要約生成が不可） |
| 影響期間 | 不明 〜 2026-03-24（潜在的には REST → Connect-RPC 移行以前から。ストリーミング未表示は SvelteKit プロキシ実装当初から） |
| 影響サービス | alt-backend, pre-processor, alt-frontend-sv, news-creator（間接） |
| 影響機能 | Desktop Visual Preview のオンザフライ AI 要約生成 |
| 関連 ADR | [[000553]], [[000554]] |

## サマリー

Desktop UI の Visual Preview でオンザフライ要約（StreamSummarize）を実行すると、Cloudflare HTTP 524（100秒タイムアウト）が返るか、データがバックエンドで正常に生成されているにもかかわらずフロントエンドにストリーミング表示されない問題が発生した。調査の結果、3 つの独立した根本原因が重畳していることが判明した。(1) alt-backend が REST パスで pre-processor を呼んでおり `processingArticles` ガードで 409 拒否される、(2) Cloudflare の 100 秒アイドルタイムアウト内にレスポンスの最初のバイトが届かない、(3) SvelteKit プロキシが Connect-RPC JSON モードのストリーミングレスポンスを検出できず nginx がバッファする。

## 影響

- **オンザフライ要約生成の完全失敗**: StreamSummarize リクエストが 524 で失敗、またはフロントエンドで「Summarizing...」のまま無限ローディング
- **影響範囲**: Desktop Visual Preview の全ユーザー。Mobile Swipe の要約も同一パスを使用
- **データ損失**: なし（バックエンドでは要約が正常に生成・保存されており、リロード後はキャッシュから表示可能）
- **BE バッチ要約**: 影響なし（リモート分散処理は正常稼働）

## タイムライン

| 時刻 (JST) | イベント |
|---|---|
| 不明 | **発生**: StreamSummarize の 524 タイムアウトが散発。SvelteKit プロキシのストリーミング未検出は実装当初から潜在 |
| 2026-03-24 00:05 | **検知**: ユーザーが Visual Preview で 524 エラーを報告。コンソールに `[unknown] HTTP 524` |
| 2026-03-24 00:05 | **対応開始**: コンテナログの徹底調査を開始 |
| 2026-03-24 00:10 | alt-backend ログで `"pre-processor returned client error", code: "already_exists"` を確認。REST パスの `processingArticles` ガードが原因と特定 |
| 2026-03-24 00:15 | **原因特定 1**: `streamPreProcessorSummarize` が REST `/api/v1/summarize/stream` を使用し、Connect-RPC `StreamSummarize`（ガードなし）を使用していないことを確認 |
| 2026-03-24 00:20 | **緩和 1**: `streamPreProcessorSummarize` を Connect-RPC に移行。DI コンテナに `PreProcessorConnectClient` 追加 |
| 2026-03-24 00:25 | デプロイ後、`"protocol error: incomplete envelope: unexpected EOF"` エラーが発生 |
| 2026-03-24 00:30 | **原因特定 2**: pre-processor Connect-RPC サーバーの `WriteTimeout: 30s` がストリーム全体に適用されていることを確認 |
| 2026-03-24 00:35 | **緩和 2**: `WriteTimeout: 0` に変更、pre-processor 再ビルド |
| 2026-03-24 00:40 | Cloudflare 524 が再発。HAR 解析で 125 秒後にタイムアウト確認 |
| 2026-03-24 00:45 | **原因特定 3**: `streamPreProcessorSummarize` が blocking call であり、news-creator セマフォ待ちの間ブラウザにレスポンスが送られないことを確認 |
| 2026-03-24 00:50 | **緩和 3**: ハートビート機構を実装。goroutine で非同期接続 + 15 秒間隔で空チャンク送信 |
| 2026-03-24 00:55 | 524 は解消。しかしフロントエンドでストリーミングテキストが表示されない |
| 2026-03-24 01:00 | DevTools の Response タブでチャンクデータ到着を確認。バックエンド正常 |
| 2026-03-24 01:05 | **原因特定 4**: SvelteKit プロキシ (`+server.ts`) のストリーミング検出が `application/connect+proto` のみで `application/connect+json` を見逃している。`X-Accel-Buffering: no` 未設定 → nginx バッファ |
| 2026-03-24 01:10 | **部分復旧**: `application/connect+` プレフィックスマッチに修正、alt-frontend-sv 再ビルド |
| 2026-03-24 01:20 | **HAR 採取**: `https://curionoah.com/feeds` の HAR を記録。採取時刻は `2026-03-23T16:20:25Z` = `2026-03-24 01:20:25 JST` |
| 2026-03-24 01:22 | **未解決の証拠**: nginx access log に `POST /api/v2/alt.feeds.v2.FeedService/StreamSummarize` が 2 回記録され、1 回目は `rt=125.041` 秒で本文 `63` bytes、直後の再試行は `rt=70.185` 秒で本文 `44151` bytes |

## 根本原因分析

### Five Whys

1. **なぜ StreamSummarize が 524 を返したか？**
   → Cloudflare の 100 秒アイドルタイムアウト内にブラウザへレスポンスの最初のバイトが届かなかったため

2. **なぜ最初のバイトが届かなかったか？**
   → `streamPreProcessorSummarize()` が blocking call であり、pre-processor → news-creator のセマフォ待ちで数分ブロックされる間、alt-backend はブラウザに何も送信しなかったため

3. **なぜセマフォ待ちが長かったか？**
   → `HybridPrioritySemaphore` の RT 予約スロット（1 枠）が他のリクエストで使用中、または BE バッチジョブがセマフォスロットを保持したままリモート Ollama の応答を待っていたため

4. **なぜストリーミングテキストがフロントエンドに表示されなかったか？**
   → SvelteKit プロキシが `application/connect+json` を検出できず、`X-Accel-Buffering: no` を設定しなかったため、nginx がレスポンスをバッファし、チャンクがリアルタイムにブラウザに転送されなかった

5. **なぜ `application/connect+json` が検出されなかったか？**
   → プロキシのストリーミング検出条件が `application/connect+proto` と `application/grpc` のハードコードであり、Connect-RPC の JSON シリアライゼーションモードを考慮していなかったため。Proto モードでの初期実装時に JSON モードのテストが欠如していた

### 寄与要因

- **REST と Connect-RPC の混在**: alt-backend が pre-processor を REST と Connect-RPC の両方で呼ぶ混在状態。REST パスの `processingArticles` ガードが FE ストリーミングをブロック
- **pre-processor の `WriteTimeout: 30s`**: ストリーミング RPC に対して短すぎるタイムアウト。Go の `http.Server.WriteTimeout` がストリーム全体に適用されることが認識されていなかった
- **ハートビートの concurrent write**: 初回の修正でハートビート goroutine と `streamAndCapture` が同じ Connect-RPC stream に同時書き込みし、`"incomplete envelope"` プロトコルエラーを引き起こした

## 対応の評価

### うまくいったこと

- HAR ファイル + コンテナログの突合で、問題の所在（バックエンド vs フロントエンド vs プロキシ）を段階的に切り分けられた
- 既存の `preprocessor_connect/client.go` に `StreamSummarize` + `streamAdapter` が実装済みだったため、Connect-RPC 移行が迅速だった
- DevTools の Response タブでチャンク到着を確認し、バックエンドは正常でフロントエンドのプロキシ層に問題があることを絞り込めた

### 改善が必要なこと

- ハートビートの初回実装で concurrent write の問題を見逃した。Connect-RPC `ServerStream.Send()` がスレッドセーフでないことを事前に確認すべきだった
- SvelteKit プロキシの content-type 検出が Proto モードのみだったことに、StreamSummarize のデプロイ時に気づくべきだった
- 3 つの根本原因が重畳していたため、1 つ修正するたびに次の原因が露出するモグラ叩きになった。事前にエンドツーエンドのストリーミングテストがあれば防げた
- `01:10 復旧` と結論づけるには早すぎた。`01:20 JST` 採取 HAR と `01:22 JST` nginx access log は、少なくともその時点では StreamSummarize がまだ安定していなかったことを示している

### 運が良かったこと

- バックエンドでは要約が正常に生成・保存されており、ユーザーがリロードすればキャッシュから表示可能だった
- BE バッチ要約はリモート分散で影響を受けず、要約カバレッジの低下はなかった

## 教訓

### 技術的教訓

1. **Go の `http.Server.WriteTimeout` はストリーム全体に適用される**: ストリーミング RPC を提供するサーバーでは `WriteTimeout: 0` にし、RPC ごとの `context.WithTimeout` で制御すべき
2. **Connect-RPC `ServerStream.Send()` はスレッドセーフではない**: ハートビート等の concurrent write は goroutine 分離ではなく、単一 goroutine 内の `select` ループで実装すべき
3. **Cloudflare 524 はアイドルタイムアウト**: バイトが 1 つでも送られればタイマーリセット。LLM の TTFT が長い場合、blocking call の前に即座にレスポンスを開始する設計が必要
4. **プロキシ層の content-type 判定はプレフィックスマッチにすべき**: `application/connect+proto` のハードコードは将来の形式追加（`+json`, `+cbor` 等）で壊れる

### 組織的教訓

1. **エンドツーエンドのストリーミング E2E テストが必要**: ブラウザ → Cloudflare → nginx → SvelteKit → alt-backend → pre-processor → news-creator の全レイヤーを通したストリーミングテストがあれば、3 つの根本原因を事前に検出できた
2. **プロトコル移行時はシリアライゼーション形式の全パターンをテストすべき**: Connect-RPC は Proto/JSON/gRPC-Web の複数形式をサポートする。移行時に使用中の形式のみテストするのは不十分

## アクションアイテム

### 予防（Prevent）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| P-1 | `streamPreProcessorSummarize` を REST → Connect-RPC に移行 | 開発担当者 | 2026-03-24 | **完了** |
| P-2 | pre-processor Connect-RPC サーバーの `WriteTimeout` を 0 に変更 | 開発担当者 | 2026-03-24 | **完了** |
| P-3 | Cloudflare 524 回避のハートビート実装（goroutine 非同期 + 15s 間隔） | 開発担当者 | 2026-03-24 | **完了** |
| P-4 | SvelteKit プロキシの content-type 検出を `application/connect+` プレフィックスマッチに修正 | 開発担当者 | 2026-03-24 | **完了** |
| P-5 | FE オンザフライ要約リクエスト時に BE バッチジョブを mq-hub 経由でキャンセルする機構を実装（FE/BE 独立管理） | 開発担当者 | 2026-04-07 | 未着手 |

### 検知（Detect）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| D-1 | StreamSummarize の TTFT（最初のチャンク到着まで）のメトリクスを追加し、p95 > 30s でアラート | 開発担当者 | 2026-04-07 | 未着手 |

### 緩和（Mitigate）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| M-1 | フロントエンドで StreamSummarize タイムアウト時に「バッチ処理中です。しばらく後に再試行してください」のユーザーフレンドリーなメッセージを表示 | 開発担当者 | 2026-04-07 | 未着手 |

### プロセス（Process）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| R-1 | エンドツーエンドのストリーミング E2E テストを追加（ブラウザ → バックエンド → LLM の全レイヤー） | 開発担当者 | 2026-04-14 | 未着手 |
