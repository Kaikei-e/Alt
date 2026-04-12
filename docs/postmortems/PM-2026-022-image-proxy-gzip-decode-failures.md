# ポストモーテム: 画像プロキシが gzip 圧縮された JPEG を復号できず 502 を返していた問題

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-022 |
| 発生日時 | 起源は画像プロキシ導入時から潜在。顕在化は運用ログで継続観測、今回の根本解析実施は 2026-04-12 (JST) |
| 復旧日時 | 2026-04-12 13:07 (JST) ※alt-backend 再ビルド完了 |
| 影響時間 | 潜在期間不定。ユーザー影響としては「画像プレースホルダーが表示されない / サムネイル欠け」が継続 |
| 重大度 | SEV-3（主要機能の一部劣化。記事本文・一覧は正常動作、サムネイル表示のみ欠落） |
| 作成者 | オンコール担当者 |
| レビュアー | — |
| ステータス | Resolved |

## サマリー

alt-backend の画像プロキシ (`/v1/images/proxy/:sig/:url_b64`) が、特定の外部 CDN が返す画像に対して `WARN: decode image: image: unknown format` を出し 502 を返し続けていた。ユーザー側のフィード一覧・記事詳細でサムネイル画像が表示されない状態が継続していた。観測性を強化して magic bytes をログ出力したところ、`1f 8b 08 00` すなわち gzip バイト列が画像デコーダに渡っていることが判明。原因は `ImageFetchGateway` が HTTP リクエストに `Accept-Encoding: gzip, deflate` を手動設定しており、Go の `http.Transport` による gzip 透過解凍が無効化され、圧縮バイトがそのまま `image.Decode` に流れていたこと。ヘッダを削除して Transport の既定挙動に戻し、未対応エンコーディングに対する defense-in-depth ガードを追加して復旧した。

## 影響

- **影響を受けたサービス:** alt-backend（image proxy エンドポイントのみ）
- **影響を受けたリクエスト:** 外部 CDN が `Content-Encoding: gzip` で返す全ての画像プロキシリクエスト
- **機能への影響:** フィード一覧・記事詳細でサムネイル / OGP 画像が表示されない。ただし alt-frontend-sv は画像失敗時にレイアウトを崩さないフォールバックを持つため、本文閲覧は継続可能
- **データ損失:** なし
- **他機能への影響:** なし（画像プロキシ以外のエンドポイントは正常）

### 定量的影響

| メトリクス | 期待値 | 実際（修正前） |
|---|---|---|
| `unknown format` エラー / 5 分 | 0 | 2〜数十件（アクセス頻度とキャッシュ効率に依存） |
| 502 応答 (image proxy) | SSRF / 許可ドメイン外由来のみ | 502 に gzip 起因のものが混在 |
| サムネイル表示失敗率（同一画像） | 0%（キャッシュ成功後） | 100%（キャッシュにも到達せず毎回失敗） |
| 修正後 5 分間の `unknown format` 発生件数 | 0 | **0 件（達成）** |

## タイムライン

| 時刻 (JST) | イベント |
|---|---|
| 不定（画像プロキシ実装時から） | **起源**: `ImageFetchGateway` の HTTP リクエスト構築で `req.Header.Set("Accept-Encoding", "gzip, deflate")` が記述される |
| 継続的 | **潜在**: gzip を返す CDN への画像プロキシリクエストは全て decode 失敗 → 502 返却。サーバログ上は `image: unknown format` の WARN として残るが、「どの形式の画像か」がログから追えず調査が進まない状態が続く |
| 2026-04-12 12:40 頃 | **検知**: ユーザーから「alt-backend がおかしな挙動」としてログサンプルが共有される。dezeen.com 画像で `decode image: image: unknown format`、hackernoon 系 URL で 404 が混在 |
| 2026-04-12 12:45 | **分類**: 観測対象 4 事象を切り分け。gzip 系は alt-backend 内の修正対象、hackernoon の二重 URL prefix は上流 bug として別タスク化、`article_id:null` と SIGTERM は正常動作と分類 |
| 2026-04-12 12:50 | **観測性強化 (TDD)**: `ProcessingGateway.ProcessImage` のデコード失敗時エラーに `upstream_content_type` / `detected_format (magic-byte 判定)` / `magic=<hex>` / `size` を含めるよう変更。`IsValidImageContentType` を明示ホワイトリスト化し `image/avif` / `image/svg+xml` 等を境界で拒否 |
| 2026-04-12 12:58 | **1st rebuild + 実ログ採取**: `docker compose up --build -d alt-backend`。新ログに `magic=1f8b0800...` を確認。gzip であることが判明 |
| 2026-04-12 13:00 | **真因特定**: `image_fetch_gateway.go:284` の `Accept-Encoding: gzip, deflate` が Go の透過解凍を無効化していたことを特定。[Go net/http Transport ドキュメント](https://pkg.go.dev/net/http#Transport) で仕様を確認 |
| 2026-04-12 13:00 | **セキュリティ要件確認**: [AWS CodeGuru Detector Library](https://docs.aws.amazon.com/codeguru/detector-library/go/decompression-bomb/) 等で decompression bomb 対策の要件を再確認。既存の `io.LimitReader(resp.Body, MaxSize+1)` が解凍後サイズに作用するため引き続き有効であることを確認 |
| 2026-04-12 13:02 | **修正 (TDD)**: `Accept-Encoding` ヘッダ削除 + `validateResponseEncoding` ガード追加（未解凍エンコーディングを body 読込前に拒否）|
| 2026-04-12 13:05 | **テスト**: `TestValidateResponseEncoding` 8 ケース追加・全パス、`go test ./...` 全パス |
| 2026-04-12 13:07 | **復旧**: alt-backend 再ビルド完了、`/v1/health` 200 |
| 2026-04-12 13:10 | **検証**: 5 分間のログ監視で `unknown format` / `magic=1f8b` の発生 0 件を確認 |

## 検知

- **検知方法:** ユーザーからのログ共有（alt-backend ログサンプル提示）
- **検知までの時間 (TTD):** 不明（潜在期間が長期）。今回の共有は即時対応
- **検知の評価:** エラーログ自体は継続的に発生していたが「`unknown format` は外部 CDN の仕様」と片付けられやすく、本格調査に入るトリガーが弱かった。観測性の情報密度が低かったことが原因究明を遅らせた

## 根本原因分析

### 直接原因

`alt-backend/app/gateway/image_fetch_gateway/image_fetch_gateway.go:284` にて HTTP リクエストに `Accept-Encoding: gzip, deflate` を明示設定していた。Go `net/http` の Transport は [ドキュメント](https://pkg.go.dev/net/http#Transport) に記載の通り、`Accept-Encoding` を自身で付与した場合のみ gzip レスポンスを透過解凍する仕様で、ユーザーが明示した場合は「呼び出し側が解凍する」と解釈し自動解凍を無効化する。結果として gzip 圧縮された画像バイト列が生のまま `image.Decode` に到達し、`unknown format` になっていた。

### Five Whys

1. **なぜ画像がデコードできなかったか？**
   → 画像バイト列が実は gzip 圧縮されており、`image.Decode` が対応する画像形式（jpeg/png/gif/webp）のいずれでもなかったから。
2. **なぜ画像バイトが gzip のまま `image.Decode` に到達したのか？**
   → Go の `http.Transport` が gzip を透過解凍しなかったから。
3. **なぜ Transport が透過解凍しなかったのか？**
   → リクエストに `Accept-Encoding: gzip, deflate` が手動設定されており、Transport が「呼び出し側が自前で解凍する」と解釈したから。
4. **なぜこのヘッダを手動設定していたのか？**
   → 「CDN に gzip を要求すればレスポンスが軽くなる」という意図で、Go の透過解凍仕様を把握せずに設定された。レビュー時にも同仕様が参照されなかった。
5. **なぜこの不整合が長期間潜在したのか？**
   → (a) デコード失敗時のエラーログに形式情報が含まれず「CDN のせい」で片付けられていた。(b) 画像プロキシは frontend 側に graceful fallback があるためユーザー影響が派手ではなく、優先度が上がらなかった。(c) 画像プロキシ層に対する契約テスト / gzip レスポンスのユニットテストが存在しなかった。

### 寄与要因

- **観測性の弱さ**: デコード失敗時のログが `decode image: image: unknown format` のみで、upstream Content-Type や先頭バイトが含まれていなかった
- **契約テスト不在**: 画像プロキシは外部 HTTP を扱うにもかかわらず、gzip / brotli / 空レスポンス等のエンコーディングバリエーションに対するユニットテストが存在しなかった
- **Content-Type 検査の緩さ**: `IsValidImageContentType` が `image/*` プレフィックスマッチで、`image/avif` / `image/svg+xml` 等のサポート外形式を境界でブロックできておらず、結果として「unknown format は全て同じ」に見えていた
- **コメント欠如**: `Accept-Encoding` を手動設定することの Go 側の副作用についてコードコメントがなく、意図不明だった

### 貢献した設計的・組織的要因

- Go の `net/http` 仕様は一部が「標準ライブラリの慣例」に依存しており、Transport の `Accept-Encoding` 特例挙動はドキュメントを読まないと気づきにくい
- 画像プロキシのような「副次的機能」は壊れていても主要機能が動くため、低優先度に滞留しやすい

## 緩和策・復旧

- `Accept-Encoding` ヘッダ削除 → Go Transport の透過解凍が有効化され、`resp.Body` が解凍済みストリームとして返る
- `validateResponseEncoding` ガード追加 → `Content-Encoding` に `br` / `zstd` / `deflate` 等が残留した場合、body を読む前に `VALIDATION_ERROR` として拒否
- 既存 `io.LimitReader(resp.Body, MaxSize+1)` は解凍後サイズに作用するため、gzip bomb 対策として引き続き有効（[AWS CodeGuru 勧告](https://docs.aws.amazon.com/codeguru/detector-library/go/decompression-bomb/) / [Datadog Static Analysis](https://docs.datadoghq.com/security/code_security/static_analysis/static_analysis_rules/go-security/decompression-bomb/) 準拠）

## うまくいったこと

- **観測性強化を先に入れたこと**: 実装を推測せず、まず診断情報（magic bytes / Content-Type）をログに出す改修を先行させたことで、「AVIF 説」から「gzip 説」へ方針転換できた。Eval before implementation の原則が機能した
- **TDD サイクル維持**: 観測性強化も gzip 修正も RED → GREEN でテスト先行。`validateResponseEncoding` は 8 ケースのテーブル駆動テストで境界ケースを網羅
- **既存の bomb 対策が機能していたこと**: `io.LimitReader` が元々 5 MB 上限を設定していたため、透過解凍に切り替えても gzip bomb リスクが増えなかった

## うまくいかなかったこと

- **長期間の放置**: ログに `unknown format` が継続的に出ていたにもかかわらず深掘りされず、ユーザーからの指摘で初めて根本解析に至った
- **観測性の初期設計不足**: 画像プロキシ導入時（[[000272]]）にデコード失敗時の診断情報設計が弱く、6 階層の Clean Architecture を通じて error が握りつぶされる形になっていた

## 教訓

### 技術的教訓

- Go `net/http` Transport の `Accept-Encoding` 自動管理は、ユーザーが手動設定した瞬間に無効化される。**明示的に Accept-Encoding を設定する際は必ず `Content-Encoding` レスポンスを自前で解釈する責任を負う**
- 外部 HTTP レスポンスを扱う layer では、**未知の `Content-Encoding` を常に明示的に拒否**するのがミスを防ぐ一番安い方法（defense-in-depth）
- デコード系エラーは「入力の形式」が謎だと原因特定に時間がかかる。**常に magic bytes と upstream metadata をエラーに含める**ことで調査工数を大幅に削減できる

### 組織的・プロセス的教訓

- 「副次機能のエラー」を定期的に棚卸しする仕組みがないと長期放置される。ログの WARN/ERROR の上位 N 種類を週次でレビューする運用が必要
- 観測性ファースト（先にログを充実させてから修正）は遠回りに見えて最短経路であることが再確認できた

## アクションアイテム

### 予防（Prevent）

| ID | 内容 | 担当者 | 期限 |
|---|---|---|---|
| P-1 | `Accept-Encoding` を明示設定するコードが他の gateway にないか棚卸し。あれば削除または自前解凍を実装 | alt-backend オーナー | 2026-04-19 |
| P-2 | `validateResponseEncoding` と同等の encoding ガードを、外部 HTTP を叩く他の gateway (fetch_article_gateway 等) にも横展開 | alt-backend オーナー | 2026-04-26 |
| P-3 | 画像プロキシに gzip / 未知 encoding / 巨大 body / 空 body の契約テスト（httptest ベース）を追加 | alt-backend オーナー | 2026-05-03 |

### 検知（Detect）

| ID | 内容 | 担当者 | 期限 |
|---|---|---|---|
| D-1 | 画像プロキシの `detected_format` ログを Prometheus/Grafana に集計（format 別の 5xx 率ダッシュボード）。`detected_format=avif` 等の頻度から将来のデコーダ追加判断に使う | Observability 担当 | 2026-04-30 |
| D-2 | `image proxy error` の WARN を Alert rule に登録。閾値を 5 分あたり 20 件超で通知（短期スパイクを検知） | Observability 担当 | 2026-04-26 |

### 緩和（Mitigate）

| ID | 内容 | 担当者 | 期限 |
|---|---|---|---|
| M-1 | 画像プロキシが 502 を返す場合の frontend 側プレースホルダー表現を統一（現状は機能済みだが、エラーの種類別挙動が未整理） | alt-frontend-sv オーナー | 2026-05-10 |

### プロセス（Process）

| ID | 内容 | 担当者 | 期限 |
|---|---|---|---|
| Pr-1 | 週次で alt-backend の上位 ERROR/WARN 種別を棚卸しし、未調査のものを 1 件以上クローズする運用を開始 | オンコールローテーション | 2026-04-19 〜継続 |
| Pr-2 | `docs/runbooks/` に「外部 HTTP gateway で decode/parse が失敗した際の一次調査手順」を追加（magic bytes の読み方、`Content-Encoding` 検査、Go Transport の挙動メモ） | alt-backend オーナー | 2026-05-03 |
| Pr-3 | 今回特定した hackernoon の二重 URL prefix (`https://hackernoon.com/https://cdn.hackernoon.com/...`) は別タスクとして pre-processor / BFF を調査 | pre-processor オーナー | 2026-04-26 |

## 関連 ADR / Issue

- [[000702]] 画像プロキシの gzip 透過解凍を net/http Transport に委譲する（本インシデント対応の設計記録）
- [[000272]] OGP 画像プロキシ — バックエンド取得・圧縮・キャッシュ（元の導入 ADR）
- [[000308]] 画像プロキシのドメイン許可リストにサブドメインマッチングを導入

## 参考資料

- [net/http Transport — Go 標準ライブラリ](https://pkg.go.dev/net/http#Transport)
- [Decompression Bomb — AWS CodeGuru Detector Library](https://docs.aws.amazon.com/codeguru/detector-library/go/decompression-bomb/)
- [Prevent decompression bomb — Datadog Static Analysis](https://docs.datadoghq.com/security/code_security/static_analysis/static_analysis_rules/go-security/decompression-bomb/)
