# Ory Kratos

_Last reviewed: September 5, 2026_

**Location:** `kratos/`
**Port:** 4433 (public API, host bound `127.0.0.1:4433`), 4434 (admin API, internal `kratos-admin` network only)

## Role
- Alt の Identity Provider (IdP)。身元確認 (registration, login, verification, recovery) とセッション管理を一元的に担う (Ory Kratos v1.3.0)
- 自前で identity / session を実装せず Kratos に集約し、下流の認可および backend token 変換は [[auth-hub]] が担う二段構え
- ブラウザからのアクセスは edge proxy (`plecto-proxy`) の `/ory/*` 経由で中継される (`KRATOS_PUBLIC_URL`)

## Architecture & Networking

| Property | Value |
| --- | --- |
| Image | `oryd/kratos:v1.3.0` (`compose/auth.yaml`) |
| Public Port | 4433 (host: `127.0.0.1:4433`, health `/health/ready`) |
| Admin Port | 4434 (internal network `kratos-admin`, reachable by auth-hub) |
| Dedicated DB | `kratos-db` (`postgres:16.15-alpine`, host port 5434, [[kratos-db]]) |
| Connection Pool | `pgbouncer-kratos` (port 6432, transaction pooling) |
| Configuration | `../kratos:/etc/config/kratos:ro` (`kratos.yml`, `identity.schema.json`) |
| Secrets | `kratos_db_password`, `kratos_cookie_secret`, `kratos_cipher_secret` |

Admin リスナー (:4434) は internal な `kratos-admin` ネットワークに隔離されており、`auth-hub` のみが通信可能。ホストループバックには公開されない (Health check も 4433 疎通かつ 4434 非公開をアサートする)。

## Database & Migrations

- **専用 DB**: `kratos-db` (PostgreSQL 16, port 5434)。kratos が自前でマイグレーションを発行するため、Alt 側で手動 DDL や別マイグレーションツールによる変更を行わない。
- **接続経路**: `kratos serve` は直接 DB に接続せず `pgbouncer-kratos` (port 6432, transaction pooling) を経由する。
- **マイグレーション適用**: ワンショットの `kratos-migrate` コンテナ (`compose/auth.yaml`) が `kratos migrate sql -e --yes` を実行する。マイグレーション時は DDL および session-level lock が必要なため、PgBouncer をバイパスして `kratos-db:5432` へ直接接続する。

## Dependencies & Data Flow

- **下流**: `pgbouncer-kratos` → `kratos-db`
- **上流**:
  - `auth-hub`: Kratos の session cookie (`ory_kratos_session`) を検証し、`X-Alt-Backend-Token` (JWT) に再署名・変換する。Kratos の session token は他サービスやフロントエンドにそのまま流さず、必ず `auth-hub` 経由で伝播させる。
  - `plecto-proxy`: `plecto/manifest.toml` の `/ory/` route により、ブラウザからの認証 API リクエストと `Set-Cookie` を透過中継する。
  - `alt-frontend-sv`: `/login`, `/register`, `/settings` 等の UI フローを提供。

## Operational Runbook & Health Check

```bash
# Public API ヘルスチェック
curl http://localhost:4433/health/ready

# サービス再起動
docker compose -f compose/auth.yaml up -d kratos

# マイグレーション実行
docker compose -f compose/auth.yaml run --rm kratos-migrate
```

## Known Failure Patterns & Invariants

Cross-cutting incident patterns are catalogued in [[runbooks/crystallized-knowledge]].

- **Session が無効なのに UI が動いているように見える**: `auth-hub` のセッション検査が緩いと、Kratos 側で expire 済みのセッションを通過させてしまう。`auth-hub` 側を fail-closed に保ち、Kratos の最新状態またはキャッシュ有効期限を厳格に照合する。
- **Cookie が FE に飛ばない / FE で session が読めない**: `domain` / `secure` / `samesite` の設定不整合が主因。Plecto (edge proxy) と Kratos の cookie 設定が環境ごとに整合しているか確認する。
- **`kratos-db` への接続失敗による全認証停止**: fail-closed 設計のため、DB 接続断時は全ユーザーのログインおよびセッション検証が停止する。`kratos-db` のヘルス状態および `pgbouncer-kratos` の接続設定を確認する。
