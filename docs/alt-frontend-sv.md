# Alt Frontend SV

_Last reviewed: December 18, 2025_

**Location:** `alt-frontend-sv`
**Base Path:** `/sv`

## Role
- **Next-Gen Frontend**: A SvelteKit (Svelte 5 Runes) + Vite application serving as the modern, high-performance interface for the platform.
- **Unified Interface**: Serves both desktop (dashboard) and mobile (feed reader) experiences with a focus on speed and interaction.
- **Auth Consumer**: Integrated with Ory Kratos for identity management, using `auth-hub` for backend token exchange.

## Architecture Snapshot

| Layer | Details |
| --- | --- |
| **Routing** | File-system based routing in `src/routes`. Served under **`/sv`** base path (configured in `svelte.config.js`). |
| **State Management** | **Svelte 5 Runes** (`$state`, `$derived`, `$effect`) for reactive state. `src/lib/stores` contains global stores (e.g., `auth.svelte.ts`). |
| **Data Fetching** | `src/lib/api.ts` wraps `fetch`. It automatically calls `auth-hub` to exchange the session cookie for an `X-Alt-Backend-Token` (JWT) before calling `alt-backend`. |
| **Real-time** | SSE (Server-Sent Events) integration via `src/lib/api/sse.ts` and `useSSEFeedsStats.svelte.ts` to stream feed processing stats. |
| **Styling** | **TailwindCSS v4** (using the new Vite plugin) with `bits-ui` for primitives and `lucide-svelte` for icons. |
| **Middleware** | `src/hooks.server.ts` validates Ory sessions, populates `event.locals.User` / `Session`, and handles redirects for protected routes. |

```mermaid
flowchart TD
    Browser -->|/sv/*| Frontend[alt-frontend-sv<br/>SvelteKit /sv]
    Frontend -- "Cookie" --> AuthHub[auth-hub<br/>(Token Exchange)]
    AuthHub -- "X-Alt-Backend-Token" --> Frontend
    Frontend -- "Bearer JWT" --> Backend[alt-backend REST]
    Backend -- "SSE Stream" --> Frontend
    Frontend -- "Ory Session" --> Ory[Ory Kratos]
```

## Key Directories

- `src/routes`:
    - `/sv/home`: Desktop dashboard (feeds, stats, system monitor).
    - `/sv/mobile`: Mobile-optimized feed reader (swipe interface).
    - `/sv/dashboard`: System administration and monitoring.
    - `/sv/login`, `/sv/register`: Authentication pages.
    - `/sv/api`: Internal SvelteKit API endpoints (if any).
- `src/lib`:
    - `api.ts`: Core API client. Handles token exchange and error normalization.
    - `components`: Reusable UI components (Atomic design-ish).
    - `stores`: Global state using Runes (e.g., `auth.svelte.ts`).
    - `hooks`: Custom Svelte hooks (e.g., `useSSEFeedsStats.svelte.ts`).

## Configuration
- **Svelte Config** (`svelte.config.js`): Sets `kit.paths.base = '/sv'` and uses `adapter-node`.
- **Vite Config** (`vite.config.ts`): Configures proxying and aliases.
- **Environment**:
    - `BACKEND_BASE_URL`: Internal URL for server-side fetches (e.g., `http://alt-backend:9000`).
    - `AUTH_HUB_INTERNAL_URL`: URL for token exchange (e.g., `http://auth-hub:8888`).

## Development

### Prerequisites
- Node.js 22+ (for SvelteKit)
- pnpm

### Commands
```bash
# Start development server
pnpm dev

# Build for production
pnpm build

# Type check
pnpm check

# Lint and Format (Biome)
pnpm lint
pnpm format
```

### LLM / Dev Notes
- **Runes Mode**: This project strictly uses Svelte 5 Runes. Do not use legacy `export let` or `$:`. Use `$props()` and `$state()`.
- **Base Path**: Always remember the app runs under `/sv`. Links should be relative or account for this.
- **Tailwind v4**: No `tailwind.config.js` (mostly). Configuration is CSS-first in `src/app.css`.
- **SSR vs CSR**: Data loading happens in `+page.server.ts` (SSR) for initial state, but client-side interactions use `api.ts` (CSR).
