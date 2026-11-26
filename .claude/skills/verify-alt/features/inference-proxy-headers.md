# Inference Proxy Upstream Header Forwarding (`docker/inference-proxy`)

## Sub-features

- **Unauthenticated health probe**: `GET /health` は認証不要で `200 OK` (`ok`) を返し、上流 Ollama へのヒット数は `0`。
- **Fail-closed auth & route allowlist**:
  - `Authorization: Bearer <token>` が欠落または不一致のリクエストは **`401 Unauthorized`**（上流ヒット `0`）。
  - 許可ルート（`POST /api/generate`, `/api/chat`, `/api/embed`, `/api/embeddings` および `GET /api/tags`, `/api/ps`）以外の管理ルート（例: `POST /api/pull`）は **`403 Forbidden`**（上流ヒット `0`）。
- **Upstream 6-header & URL parity (C-14)**:
  1. `Host` が `targetURL.Host` へ書き換えられる。
  2. `X-Forwarded-For` が入力ヘッダを破棄せず維持した上でクライアント IP（`127.0.0.1`）をカンマ追記する（例: `"203.0.113.10, 127.0.0.1"`）。
  3. 入力に無い `X-Forwarded-Host` を勝手に合成しない（空文字のまま）。
  4. 入力に無い `X-Forwarded-Proto` を勝手に合成しない（空文字のまま）。
  5. リクエストのパスとクエリ文字列（例: `/api/generate?stream=true&model=qwen`）が欠損なく転送される。
  6. カスタムヘッダ（例: `X-Custom-Header`）が保持され、同時に `Authorization` ヘッダは上流へ漏洩しないよう確実に除去される。

## How to get to it

- **Proxy implementation**: `docker/inference-proxy/main.go`
  - `NewProxyHandler(targetURL, expectedToken, responseHeaderTimeout, maxBodyBytes) http.Handler`
  - `Rewrite` コールバックが `Forwarded`, `X-Forwarded-For`, `X-Forwarded-Host`, `X-Forwarded-Proto` を明示クローン＋クライアント IP 追記し、`pr.Out.Header.Del("Authorization")` を実行する。
  - `NewServer(addr, handler, readTimeout...) *http.Server`（`WriteTimeout: 0`, `ReadHeaderTimeout: 10s`, `ReadTimeout: 30s`）
- **Existing characterization test**: `docker/inference-proxy/main_test.go` (`TestProxy_ForwardingHeadersAndURLParity`, `TestProxy_StripAuth_And_Forwarding`, `TestProxy_AllowedAndDisallowedRoutes`)
- **Corpus fixture (C-14)**: `scripts/harness/corpus/C-14/{before.go,after.go,meta.toml}`

## Driving it with the harness

```bash
# ローカル 127.0.0.1:0 上流エコースタブ＋実 inference-proxy を起動し全 6 ヘッダ契約をライブ検証
./scripts/verify/verify-live.sh run --task check-inference-proxy --probe inference-proxy
```

期待される観測結果（`.harness/evidence/<task>/inference-proxy.json`・`inference-proxy.log`）:
1. `GET /health` → `200 OK`、上流ヒット `0`
2. `POST /api/generate`（認証なし）→ `401 Unauthorized`、上流ヒット `0`
3. `POST /api/pull`（正当 Bearer・管理ルート）→ `403 Forbidden`、上流ヒット `0`
4. `POST /api/generate?stream=true&model=qwen`（正当 Bearer、`X-Forwarded-For: 203.0.113.10`、`X-Custom-Header: verify-live-c14`）→ `200 OK`、上流エコー応答で 6 不変条件＋`Authorization == ""` を全件一致確認。

## Gotchas

- **`pr.SetXForwarded()` の罠（C-14）**: Go の `httputil.ReverseProxy.Director` から `Rewrite` へ移行する際に `pr.SetXForwarded()` を素朴に呼ぶと、入力 `X-Forwarded-For` が捨てられ、`X-Forwarded-Host` と `X-Forwarded-Proto` が新規合成されて上流契約が壊れる。必ず上流実エコーサーバーで受信ヘッダを観測する。
- **本番ポート `:11436` や Ollama `:11434` を使わない**: ライブプローブは `127.0.0.1:0` にローカル上流エコーサーバーと `NewProxyHandler` サーバーを起動し、使い捨てトークンを使用する。
- **ボディ上限と `WriteTimeout`**: `inference-proxy` の既定ボディ上限は 16 MiB（`DefaultMaxRequestBodyBytes`）、ストリーミング対応のため `WriteTimeout` は明示的に `0`。
