# Auth Hub Session Validation & Header Issuance (`auth-hub`)

## Sub-features

- **Health endpoint**: `GET /health` は未認証で `200 OK` (`{"status":"healthy"}`) を返し、リクエストログと OTel スパンをスキップする。
- **Unauthenticated `/validate` rejection**: `ory_kratos_session` クッキーが無いリクエスト、または Kratos `/sessions/whoami` が `401` を返す無効クッキーに対して `GET /validate` は **`401 Unauthorized`** を返し、`X-Alt-*` ヘッダを発行しない。
- **Authenticated `/validate` authoritative headers**: 有効な `ory_kratos_session` クッキーで Kratos がアクティブセッションを返すと、`GET /validate` は **`200 OK`**（空ボディ）とともに以下の正規ヘッダを返す:
  - `X-Alt-User-Id`: Kratos identity ID
  - `X-Alt-Tenant-Id`: シングルテナント規約により identity ID と同一値
  - `X-Alt-User-Email`: identity traits の `email`
  - `X-Alt-Backend-Token`: Ed25519 署名済みバックエンド JWT（発行失敗時は fail-closed で 200 を返さない）
- **Authenticated `/session` JSON & JWT**: 有効なセッションクッキーで `GET /session` は **`200 OK`** と JSON（`ok: true`, `user.id`, `user.tenantId`, `user.email`, `session.active: true`）および `X-Alt-Backend-Token` ヘッダを返す。

## How to get to it

- **Server entrypoint & wiring**: `auth-hub/cmd/auth-hub/main.go`
  - `buildPlaintextHandler(e, devPlaintextAllowed)`: 既定では平文ポートを `/health` のみに制限（A04 fail-closed）。本番は `FRONTEND_TLS_LISTEN=true`（`:8443`）と `MTLS_LISTEN=true`（`:9443`）を使う。
  - `configureTrustedProxies`: `TRUSTED_PROXY_CIDRS` 未設定時は起動パニック（明示的な CIDR または `none` が必須）。
- **Handlers**:
  - `auth-hub/internal/adapter/handler/validate.go` (`ValidateHandler.Handle`)
  - `auth-hub/internal/adapter/handler/session.go` (`SessionHandler.Handle`, `HandleInvalidate`)
  - `auth-hub/internal/adapter/handler/health.go` (`HealthHandler.Handle`)
- **Kratos gateway**: `auth-hub/internal/adapter/gateway/kratos.go` (`KratosGateway.ValidateSession` → `GET <KRATOS_URL>/sessions/whoami` に `Cookie` ヘッダを転送)

## Driving it with the harness

```bash
# ローカル 127.0.0.1:0 Kratos スタブ＋実 auth-hub Echo v5 サーバーを起動しライブ検証
./scripts/verify/verify-live.sh run --task check-auth-hub --probe auth-hub
```

期待される観測結果（`.harness/evidence/<task>/auth-hub.json`・`auth-hub.log`）:
1. `GET /health` → `200 OK`、`{"status":"healthy"}`
2. `GET /validate`（クッキーなし）→ `401 Unauthorized`、`X-Alt-User-Id` 空
3. `GET /validate`（無効クッキー `ory_kratos_session=invalid`）→ `401 Unauthorized`、`X-Alt-User-Id` 空
4. `GET /validate`（有効クッキー）→ `200 OK`、`X-Alt-User-Id == "user-verify-live-01"`、`X-Alt-Tenant-Id == "user-verify-live-01"`、`X-Alt-User-Email == "verify@alt.local"`、`X-Alt-Backend-Token` が 3 セグメント JWT 形式
5. `GET /session`（有効クッキー）→ `200 OK`、`ok: true`、`user.id == "user-verify-live-01"`、`session.active: true`、`X-Alt-Backend-Token` 付与

## Gotchas

- **本番 Kratos（`:4433`）や本番 `auth-hub`（`:8888` / `:8443` / `:9443`）には絶対に繋がない**: ハーネスは `127.0.0.1:0` に使い捨て Kratos スタブと一時 Ed25519 鍵をインメモリ生成し、実 `auth-hub` ハンドラ群（`KratosGateway`, `SessionCache`, `JWTIssuer`, `ValidateHandler`, `SessionHandler`, `HealthHandler`）を `127.0.0.1:0` で起動する。
- **平文リスナ制限（A04）**: `buildPlaintextHandler(e, false)` では `/validate` と `/session` は `403`（`business endpoints require HTTPS`）になる。隔離ループバックプローブ内でのみ `devPlaintextAllowed=true` 相当で検証し、本番設定（`compose/auth.yaml` の `FRONTEND_TLS_LISTEN=true`）とは混同しない。
- **秘密鍵・トークンをログに出さない**: 使い捨て検証用 Ed25519 鍵はメモリ上でのみ生成し、ファイルにも証拠 JSON にも秘密鍵本体を書き出さない（JWT の形式妥当性とヘッダ存在のみ記録する）。
