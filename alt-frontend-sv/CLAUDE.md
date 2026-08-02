# alt-frontend-sv/CLAUDE.md

## Overview

Primary frontend. **SvelteKit 2.x**, **Svelte 5 Runes**, **TailwindCSS v4**, **TypeScript**. Serves at root path (`/`).

> Details: `docs/services/alt-frontend-sv.md`

## Commands

```bash
# Test (TDD first)
bun run test              # Unit/Component (vitest — DO NOT use `bun test`)
bun run test:e2e          # E2E (requires stack)

# Dev
bun dev

# Lint & Type Check & Build
bun run lint && bun run format
bun run check && bun run build   # svelte-check (tsc). --tsgo は使用不可 — 下記参照
```

## TypeScript 7 を採用しない理由

**`typescript` は `^6.0.3` に固定する。TS7 へ上げてはいけない。** 2026-08-02 時点で
経路が 2 つとも塞がっている。

1. **TS7 単体では `svelte-check` が起動しない。** `svelte-check@4.7.4` は
   `bin/ts-version-check.js` で `typescript` の major を検査し、5/6 以外なら
   `throw` する（"requires TypeScript >= 5.0 and <= 6.0"）。これは peer レンジの
   書き忘れではなく実行時ハードゲート。根本原因は `typescript@7.0.2` が Go 実装で
   JS コンパイラ API を公開しなくなったこと — `main: null` / `exports["."]` は
   バージョン文字列スタブのみ / `tsserver` バイナリ無し。`svelte2tsx` が必要とする
   `LanguageService` が存在しないため、動きようがない。安定 API は TS 7.1 予定。
2. **公式にサポートされる唯一の経路（TS6 と TS7 を併存させ `--tsgo` を付ける）は
   このコードベースでハングする。** 2026-08-02 に `@typescript/native@npm:typescript@7.0.2`
   (stable GA) で実測: 診断出力ゼロのまま RSS が単調増加し、2分19秒で 7.5GB、
   約 50MB/秒 で増え続けたため OOM 前に強制停止。通常の `svelte-check` (tsc) は
   同一ツリー 7120 files を約 20 秒 / 0 errors で完走する。
   2026-07-08 の撤退時 (nightly `7.0.0-dev.20260707.2`) と同じ症状が **stable でも
   再現する**ため、「悪い nightly を踏んだだけ」ではない。

再評価の条件は 2 つとも満たされたとき: TS 7.1 が安定 API を出荷し、かつ
`svelte-check` が上記バージョンゲートを緩和すること
(sveltejs/language-tools#3063 / #2733)。

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Component**: Use `@testing-library/svelte`, mock API calls
- **Store**: Test Runes reactivity in isolation
- **E2E**: Page Object Model in `tests/e2e/`

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Runes Only**: Use `$state`, `$derived`, `$effect`, `$props` - NEVER legacy syntax
3. **Root Path**: App runs at `/` - use relative paths or `$app/paths`
4. **TailwindCSS v4**: CSS-first config in `src/app.css` - no `tailwind.config.js`
5. **Biome**: Run `bun run lint && bun run format` before commits
