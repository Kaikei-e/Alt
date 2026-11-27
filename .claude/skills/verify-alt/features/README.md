# Alt Verification Feature Map

Alt のライブ検証対象となる主要機能・契約の索引である。
`verify-live doctor` は本ディレクトリの**索引衛生**を機械的に検査する（孤児ファイル・リンク切れ・必須 4 セクション欠落は fail-closed）。

## Feature Index

| Feature | Target Service / Path | Live Probe | Corpus / ADR | Doc |
|---|---|---|---|---|
| Summarize SSE Termination | `news-creator/app/news_creator/handler/sse_stream.py` | `sse` | C-07 | [summarize-sse.md](summarize-sse.md) |
| Pre-processor Body Limit | `pre-processor/app/bootstrap/server.go` | `body-limit` | C-15 | [body-limit.md](body-limit.md) |
| Auth Hub Session & Validate | `auth-hub/cmd/auth-hub/main.go`, `internal/adapter/handler/` | `auth-hub` | — | [auth-hub-session.md](auth-hub-session.md) |
| Inference Proxy Upstream Headers | `docker/inference-proxy/main.go` | `inference-proxy` | C-14 | [inference-proxy-headers.md](inference-proxy-headers.md) |
| Frontend Surfaces & Navigation | `alt-frontend-sv/src/routes/`, `src/lib/config/navigation.ts` | Mode B / UI | ADR-001022, ADR-001026 | [frontend-surfaces.md](frontend-surfaces.md) |

## Feature File Convention (4 Required Sections)

各機能ドキュメントは必ず以下の 4 セクション（レベル 2 見出し）をこの順で持つ:

1. `## Sub-features` — 検証すべき具体的な振る舞い・契約の一覧
2. `## How to get to it` — 対象ソースコード・ルート・エンドポイント・前提設定
3. `## Driving it with the harness` — `verify-live` での具体的な駆動コマンドと期待される観測結果
4. `## Gotchas` — 過去の障害・落とし穴・本番隔離上の注意点

## Staleness & Update Rules

- 機能追加・ルート変更・ヘッダや境界値の変更を行う PR は、**同一コミット／PR 内で**該当の `features/*.md` を更新する。
- 廃止された機能（例: ADR-001026 で廃止された Evening Pulse や 7-day Recap）は現行ルートとして残さず、削除または明示的な廃止注記へ移す。
- 乖離点検と更新手順は [maintain-verification-skill/SKILL.md](../../maintain-verification-skill/SKILL.md) を参照する。

## Attribution

4 セクション Feature Map と索引衛生ループは、Alt の既存不変条件（`-p alt` 隔離・ADR-001028・C-07/C-14/C-15）に適合させた上で、`cursor/plugins` `pstack`（MIT License、<https://github.com/cursor/plugins/blob/main/pstack/docs/guide/README.md>）の設計様式をクリーンルーム適用している。
