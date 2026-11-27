# Frontend Surfaces & Navigation (`alt-frontend-sv`)

## Sub-features

実コード（`alt-frontend-sv/src/routes/`、`src/lib/config/navigation.ts`、`src/lib/components/desktop/layout/Sidebar.svelte`）および ADR-001022・ADR-001026 に基づく現行 Web UI ルート面:

- **Root landing & redirect (ADR-001022)**:
  - `/` (`+page.server.ts`): 認証済みセッション（`locals.backendToken` あり）は `303` で **`/today`** へ遷移。未認証訪問者は `+page.svelte` を表示。
  - `/health` (`+server.ts`): `GET` で `200 OK` (`text/plain; charset=utf-8`, body `"OK"`)。
- **Mobile 5-tab persistent bottom navigation (`NAV_TABS`)**:
  - `Today` → `/today`（「今日読む」キュー表示、空時は Swipe / Gallery への入口）
  - `Swipe` → `/feeds/swipe/visual-preview`（没入スワイプ面、ADR-001020 / ADR-000948）
  - `Gallery` → `/feeds/visual-preview`
  - `Search` → `/search`
  - `Menu` → `/menu`
- **Mobile Menu secondary surfaces (`MOBILE_MENU_SECTIONS`, `NAV_TABS` と `href` 重複禁止)**:
  - **Browse**: `/home` (Knowledge Home), `/knowledge` (Themes), `/knowledge/trail` (Trail), `/feeds` (Library), `/feeds/favorites`, `/feeds/viewed`, `/feeds/tag-trail`, `/feeds/tag-verse` (Desktop badge), `/articles/by-tag`
  - **Recap**: `/recap` (3-Day Recap), `/recap/morning-letter`, `/recap/job-status`
  - **AI & Insights**: `/augur`, `/augur/history`, `/acolyte`, `/stats`, `/dashboard`
  - **Settings**: `/settings/feeds`, `/settings/notifications`
  - **Admin** (`requiresAdmin: true`): `/admin/knowledge-home`, `/admin/monitor`
- **Desktop Sidebar (`Sidebar.svelte`)**:
  - Main: `/home`, `/knowledge`, `/knowledge/trail`, `/search`, `/dashboard`, `/acolyte`, `/stats`
  - Feeds: `/feeds`, `/feeds/visual-preview`, `/feeds/viewed`, `/feeds/favorites`, `/feeds/search`
  - Recap: `/recap`, `/recap/morning-letter`, `/recap/job-status`
  - Explore: `/feeds/tag-trail`, `/feeds/tag-verse`, `/articles/by-tag`
  - Augur: `/augur`, `/augur/history`
  - Settings: `/settings/feeds`, `/settings/notifications`
  - Admin (`userRole === "admin"`): `/admin/knowledge-home`, `/admin/monitor`
- **Detail, Auth & Legacy redirect routes**:
  - Detail pages: `/articles/[id]`, `/knowledge/themes/[themeId]`, `/recap/cards`, `/augur/[conversationId]`, `/acolyte/new`, `/acolyte/reports/[id]`
  - Auth / Error: `/auth/login`, `/login`, `/register`, `/logout`, `/error`
  - Legacy `/sv/*` redirects: `/sv` (→ `/feeds`), `/sv/auth/login`, `/sv/error`, `/sv/home`, `/sv/register`

## How to get to it

- **Route tree**: `alt-frontend-sv/src/routes/`
- **Navigation SSOT**:
  - Mobile tabs & menu: `alt-frontend-sv/src/lib/config/navigation.ts`
  - Desktop sidebar: `alt-frontend-sv/src/lib/components/desktop/layout/Sidebar.svelte`
  - Root redirect: `alt-frontend-sv/src/routes/+page.server.ts`
- **Unit / Component tests**: `alt-frontend-sv/src/routes/**/page.*.test.ts`, `alt-frontend-sv/src/routes/page.server.test.ts`
- **Governing ADRs**: `docs/ADR/001022.md`（`/today` 起動面と 5 タブ組み替え）、`docs/ADR/001026.md`（Evening Pulse / 7-day Recap 廃止）

## Driving it with the harness

```bash
# 1. Feature Map 整合と隔離状態を確認（Mode A）
./scripts/verify/verify-live.sh doctor

# 2. ルート・ナビゲーション単体テスト（オフライン実行）
cd alt-frontend-sv && bun run test -- src/routes/page.server.test.ts src/routes/\(app\)/today/page.svelte.test.ts src/routes/\(app\)/menu/page.svelte.test.ts

# 3. 隔離 Compose スタックでのブラウザ E2E（Mode B: 明示オプトイン・本番 -p alt 禁止）
# 必ず非本番プロジェクト名 (-p alt-verify-<run_id>) と非本番ポート帯 (19xxx) を使い、
# 終了後は `docker compose -p alt-verify-<run_id> down -v` で隔離環境のみ解放する。
```

期待される観測結果:
- 認証済み `/` リクエストが `303 Location: /today` を返す。
- `NAV_TABS` の 5 タブと `MOBILE_MENU_SECTIONS` の間に `href` 重複が無い。
- `GET /health` が `200 OK` (`OK`) を返す。
- 廃止済みルート（`/recap/evening-pulse`）がナビゲーションにも `src/routes/` にも存在しない。

## Gotchas

- **廃止済みルートを書かない（ADR-001026）**: Evening Pulse（`/recap/evening-pulse`）および 7-day Recap（`/recap?window=7`、`/api/v1/generate/recaps/7days`）は廃止・削除済み。古いドキュメントを鵜呑みにせず必ず `alt-frontend-sv/src/routes/` の実ファイルと照合する。現行 Recap 面は 3-day Recap (`/recap`)、Topic Cards (`/recap/cards`)、Morning Letter (`/recap/morning-letter`)、Job Status (`/recap/job-status`) の 4 面である。
- **未リンク候補ルート（ADR-001026 §7）**: `/eval-dashboard` と `/settings/knowledge-home-admin`（`/admin/knowledge-home` への 307 リダイレクト）はナビに未リンクだが `src/routes/` に残存しているため、勝手な削除やナビ追加を行わない。
- **Vitest 実行コマンド**: `alt-frontend-sv` では素の `bun test` ではなく **`bun run test`**（Vitest）を使う。
- **本番ポート `:4173` / `:80` へ接続しない**: Mode B でブラウザ検証する場合は必ず隔離プロジェクト `-p alt-verify-<run_id>` と `19xxx` ポート帯を使う。
