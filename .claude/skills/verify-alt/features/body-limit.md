# Pre-processor Request Body Limit (`pre-processor`)

## Sub-features

- **Decimal 1 MB boundary (C-15)**: `/api/v1/*` グループに適用される Echo v5 `middleware.BodyLimit(apiBodyLimitBytes)` は、Echo v4 の `"1M"`（`gommon/bytes` の 10 進メガバイト）と一致する **`1,000,000` bytes**（`1 << 20 = 1,048,576` ではない）を上限とする。
- **Exact accept / reject transition**:
  - `POST /api/v1/summarize` にちょうど **`1,000,000` bytes** を送ると `BodyLimit` を通過する（HTTP ステータス `!= 413`）。
  - `POST /api/v1/summarize` に **`1,000,001` bytes** を送ると `BodyLimit` が即座に **`413 Request Entity Too Large`** で拒否する。
- **Unauthenticated health routes**: `/health` および `/api/v1/health` は `/api/v1` グループ外に置かれ、`200 OK` (`{"status":"healthy"}`) を返す。

## How to get to it

- **Server wiring**: `pre-processor/app/bootstrap/server.go`
  - `const apiBodyLimitBytes = 1_000_000`
  - `NewHTTPServer(deps, otelEnabled, otelServiceName) *echo.Echo`
  - `api := e.Group("/api/v1")` → `api.Use(middleware.BodyLimit(apiBodyLimitBytes))` → `POST /summarize`, `POST /summarize/stream`, `POST /summarize/queue`, `GET /summarize/status/:job_id`
- **Existing unit boundary test**: `pre-processor/app/bootstrap/server_body_limit_test.go` (`TestHTTPServer_BodyLimitMatchesDecimalMegabyte`)
- **Corpus fixture (C-15)**: `scripts/harness/corpus/C-15/{before.go,after.go,meta.toml}`

## Driving it with the harness

```bash
# pre-processor 実 Echo v5 サーバーを 127.0.0.1:0 で起動し 1,000,000 vs 1,000,001 bytes を実 HTTP 送信
./scripts/verify/verify-live.sh run --task check-body-limit --probe body-limit
```

期待される観測結果（`.harness/evidence/<task>/body-limit.json`・`body-limit.log`）:
1. `GET /health` → `200 OK`、`{"status":"healthy"}`
2. `POST /api/v1/summarize`（`Content-Type: application/json`、`Expect: 100-continue`、厳密に `1,000,000` bytes）→ `BodyLimit` 通過（HTTP ステータス `!= 413`、例: 後段 `peer.EchoMiddleware()` の `401` または業務バリデーションの `400`）
3. `POST /api/v1/summarize`（`Content-Type: application/json`、`Expect: 100-continue`、厳密に `1,000,001` bytes）→ HTTP `413 Request Entity Too Large`

## Gotchas

- **Echo v4 `"1M"` vs Go `1 << 20` の罠**: Echo v5 で `BodyLimit` が文字列から `int64` バイト数に変わった際、`1 << 20`（`1,048,576` bytes）と書くと上限が 48,576 bytes 膨らむ。`ast-grep` ルール `go-no-inline-body-limit` とライブ境界プローブの双方で二重防御している。
- **mTLS ピア検証ミドルウェア**: `MTLS_ALLOWED_PEERS` が設定されている環境では `peer.EchoMiddleware()` が `BodyLimit` の後段で動く。`BodyLimit` は前段にあるため `1,000,001` bytes は常に 413 で落ちる。隔離ループバックプローブでは `MTLS_ALLOWED_PEERS` を空にして実行する。
- **本番ポート `:9200` / `:9202` / `:9443` を使わない**: ハーネスは `go -C pre-processor/app test -overlay` 経由で `127.0.0.1:0` に直接バインドするため、リポジトリ内にバイナリを残さず本番ポートとも衝突しない。
