# Alt Frontend

_Last reviewed: November 10, 2025_

**Location:** `alt-frontend`

## Role
- Next.js 16 + React 19 App Router client that renders dashboards, desktop mode, and auth flows using Chakra UI themes.
- Acts as a thin server-side gateway to `alt-backend`/`auth-hub`, handling SSR data loading, optimistic UI, and accessibility.

## Service Snapshot
| Area | Details |
| --- | --- |
| Routing | App Router under `src/app`, segmented into `(auth)`, `home`, `desktop`, and API handlers. |
| State | React Query-lite via `SWR`, contexts in `src/contexts`, custom hooks under `src/hooks`. |
| Theming | Chakra provider + `ThemeProvider.tsx` with Vaporwave / Liquid-Beige / Alt-Paper palettes. |
| Build Guardrails | `scripts/check-env.js` + `scripts/ensure-default-stylesheet.mjs` invoked pre/post `next build`. |

## Architecture Notes
- `src/app/layout.tsx` wires providers (Chakra, theme context, analytics). Server components fetch via `src/server-fetch.ts`, which wraps shared headers + error handling.
- `src/lib/api/*` modules centralize REST endpoints; each returns typed data validated with Valibot before hydration.
- Middleware (`src/middleware.ts`) enforces auth redirects by delegating to `@ory/nextjs` helpers and caches session lookups.
- Desktop/mobile experiences reuse atoms from `src/components` and rely on `src/providers/ThemeProvider.tsx` for color mode persistence.

## Data & Integrations
- **Auth:** `@ory/nextjs` plus `auth-hub` header bridging; certain routes (e.g., `/desktop`) require SSR session validation before rendering.
- **Feeds/articles:** Calls `alt-backend` via node runtime fetch; SSE hydration uses `src/api.ts` to map streaming payloads.
- **Client storage:** Minimal—`localStorage` only stores theme + UI hints; any state that matters to other services must flow through backend APIs.
- **Third-party:** `lucide-react` icons, `next-themes`, `next-themes` interplay with Chakra handled via shared provider.

## Recap Experience
- `/mobile/recap/7days` loads data from `GET /v1/recap/7days` via `src/hooks/useRecapData`, and the page now renders Recap, Genres, Articles, and Jobs tabs that honour the new `RecapSummary` payload (genre clusters, evidence_links, totals).
- `RecapCard`/`RecapSevenDaysPage` updates focus on readability, spacing, and the new evidence link list so summarised sentences + supporting IDs stay legible across mobile breakpoints. The UI keeps trace metadata from `src/lib/api` for consistent diagnostics as the pipeline matures.

## Tooling & Testing
- Package pinned to `pnpm@10.18.3`, Node 24 enforced via `engines`.
- Commands:
  - `pnpm -C alt-frontend lint` (ESLint 9 rules)
  - `pnpm -C alt-frontend fmt` (Prettier)
  - `pnpm -C alt-frontend typecheck`
  - `pnpm -C alt-frontend test` (Vitest run, `vitest.config.ts`)
  - `pnpm -C alt-frontend test:e2e` (Playwright, per-project configs, requires backend stack)
- Testing layout:
  - `src/__tests__` for colocated unit specs.
  - `tests/unit` and `tests/utils` for shared helpers.
  - Playwright page objects live in `playwright/` with `tests/pre-test-setup.cjs` seeding fixtures.

## Operational Runbook
1. `pnpm -C alt-frontend dev` for local iteration (makes use of `NEXT_PUBLIC_*` envs from `.env`).
2. To reproduce E2E flows: `pnpm -C alt-frontend test:e2e --project authenticated-chrome` after `make up`.
3. Build check: `NEXT_PHASE=analyze pnpm -C alt-frontend build` to generate bundle stats via `@next/bundle-analyzer`.
4. Clearing caches: delete `.next/` + `node_modules/.cache` when React Server Components behave unexpectedly.

## Accessibility & Performance Guardrails
- Components must abide by WCAG 2.1 AA (Chakra handles base semantics; add `aria-*` props for custom widgets).
- Streaming routes should render skeleton loaders using `React.Suspense`; avoid blocking SSE dashboards by fetching critical data inside server components.
- Perf budgets: <2s LCP on Desktop; use `next/script` for deferring analytics.

## LLM Consumption Tips
- Specify whether generated UI should be a **server component** (no hooks) or **client component** (add `'use client'` pragma) to avoid hydration bugs.
- Provide exact path when requesting edits (e.g., `src/components/desktop/TrendingTopics.tsx`) so imports remain aligned with existing barrel files.
- Mention theme tokens (e.g., `altPalette.vaporwave`) when describing styling requirements to keep Chakra usage consistent.
