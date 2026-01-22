# Alt Frontend SV

_Last reviewed: January 22, 2026_

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
| **Data Fetching** | `src/lib/api.ts` wraps `fetch` for REST. `src/lib/connect/` handles Connect-RPC directly to alt-backend:9101. TanStack Svelte Query (`src/lib/queries/`) for caching. Token exchange with `auth-hub` provides `X-Alt-Backend-Token` (JWT). |
| **Real-time** | SSE (Server-Sent Events) via `src/lib/api/sse.ts` for REST streams. Connect-RPC streaming via `streamingAdapter.ts` for typed streams. |
| **Styling** | **TailwindCSS v4** (using the new Vite plugin) with `bits-ui` for primitives and `lucide-svelte` for icons. |
| **Middleware** | `src/hooks.server.ts` validates Ory sessions, populates `event.locals.User` / `Session`, and handles redirects for protected routes. |
| **Feature Flags** | `src/lib/features/flags.ts` controls gradual migration from REST to Connect-RPC per feature area. |

### Data Path Overview

| Protocol | Path | Use Case |
|----------|------|----------|
| **REST** | Frontend → alt-backend:9000 | Legacy endpoints, SSE streams |
| **Connect-RPC** | Frontend → alt-backend:9101 | Typed RPC, streaming procedures (direct connection) |

```mermaid
flowchart TD
    Browser["Browser"]:::browser -->|"/sv/*"| Frontend["alt-frontend-sv<br/>SvelteKit :4173"]:::frontend

    subgraph Auth["Authentication"]
        direction LR
        Ory["Ory Kratos<br/>:4433"]:::auth
        AuthHub["auth-hub<br/>:8888"]:::auth
    end

    subgraph DataFlow["Data Paths"]
        direction TB
        Backend["alt-backend<br/>REST :9000"]:::backend
        BackendRPC["alt-backend<br/>Connect-RPC :9101"]:::backend
    end

    %% Auth Flow
    Frontend -. "Session Cookie" .-> Ory
    Ory -. "Session Valid" .-> Frontend
    Frontend -- "Exchange Token" --> AuthHub
    AuthHub -- "X-Alt-Backend-Token<br/>(JWT)" --> Frontend

    %% Data Flow
    Frontend -- "REST API" --> Backend
    Frontend -- "Connect-RPC<br/>(direct)" --> BackendRPC

    %% Streaming
    Backend -. "SSE Stream" .-> Frontend
    BackendRPC -. "RPC Stream" .-> Frontend

    %% Styles
    classDef browser fill:#6b7280,stroke:#374151,color:#fff
    classDef frontend fill:#3b82f6,stroke:#1d4ed8,color:#fff
    classDef auth fill:#8b5cf6,stroke:#6d28d9,color:#fff
    classDef backend fill:#10b981,stroke:#059669,color:#fff
```

## Key Directories

- `src/routes`:
    - `/sv/desktop/home`: Desktop dashboard (feeds, stats).
    - `/sv/desktop/augur`: AI chat interface.
    - `/sv/desktop/recap/*`: 7-day recap, morning letter, job status.
    - `/sv/desktop/feeds/favorites`: Favorite feeds.
    - `/sv/mobile`: Mobile-optimized feed reader (swipe interface).
    - `/sv/mobile/recap/*`: Mobile recap views.
    - `/sv/dashboard`: System administration and monitoring.
    - `/sv/login`, `/sv/register`: Authentication pages.
    - `/sv/api/v2/[...path]`: Connect-RPC proxy endpoint.
- `src/lib`:
    - `api.ts`: REST API client. Handles token exchange and error normalization.
    - `connect/`: Connect-RPC transport and client setup (direct to alt-backend:9101).
    - `gen/`: Generated protobuf definitions (feeds, articles, augur, rss, recap, morning_letter, etc.).
    - `features/`: Feature flag system for gradual Connect-RPC migration.
    - `queries/`: TanStack Svelte Query hooks for data fetching and caching.
    - `actions/`: Svelte actions (swipe, infinite-scroll).
    - `schema/`: Validation schemas using Valibot.
    - `components/`: Reusable UI components (Atomic design-ish).
    - `stores/`: Global state using Runes (e.g., `auth.svelte.ts`).
    - `hooks/`: Custom Svelte hooks (e.g., `useSSEFeedsStats.svelte.ts`).

## Connect-RPC Modules

Located in `src/lib/connect/`:

| Module | Description |
|--------|-------------|
| `feeds.ts` | Feed stats, unread/read/favorite feeds, search, streaming, mark-as-read |
| `articles.ts` | Article operations (fetch, update, favorite) |
| `recap.ts` | 7-day recap generation and retrieval |
| `augur.ts` | AI chat streaming interface |
| `rss.ts` | RSS feed management (add, remove, import OPML) |
| `morning_letter.ts` | Morning letter generation and retrieval |
| `streamingAdapter.ts` | Streaming support utilities for Connect-RPC |

## Feature Flags

The feature flag system (`src/lib/features/flags.ts`) enables gradual migration from REST to Connect-RPC:

| Flag | Environment Variable | Description |
|------|---------------------|-------------|
| `use_connect_feeds` | `PUBLIC_USE_CONNECT_FEEDS` | Use Connect-RPC for feed operations |
| `use_connect_articles` | `PUBLIC_USE_CONNECT_ARTICLES` | Use Connect-RPC for article operations |
| `use_connect_rss` | `PUBLIC_USE_CONNECT_RSS` | Use Connect-RPC for RSS management |

## Components Overview

### Desktop Components (`src/lib/components/desktop/`)
- `augur/`: AI chat interface components
- `dashboard/`: Admin dashboard panels
- `feeds/`: Feed list and article display
- `layout/`: Page layouts and navigation
- `morning-letter/`: Morning letter display
- `recap/`: Recap display components
- `settings/`: User settings
- `stats/`: Statistics visualizations

### Mobile Components (`src/lib/components/mobile/`)
- `feeds/`: Mobile feed reader with swipe
- `morning-letter/`: Mobile morning letter
- `recap/`: Mobile recap views
- `search/`: Mobile search interface

### Dashboard Tabs (`src/lib/components/dashboard/`)
- `OverviewTab`: System overview
- `ClassificationTab`: Article classification metrics
- `ClusteringTab`: Clustering metrics
- `SummarizationTab`: Summarization metrics
- `AdminJobsTab`: Admin job management
- `LogAnalysisTab`: Log analysis
- `RecapJobsTab`: Recap job monitoring
- `SystemMonitorTab`: System health monitoring

### UI Primitives (`src/lib/components/ui/`)
Based on `bits-ui`: button, card, input, label, dialog, sheet, accordion, textarea, system-loader, etc.

## Configuration
- **Svelte Config** (`svelte.config.js`): Sets `kit.paths.base = '/sv'` and uses `adapter-node`.
- **Vite Config** (`vite.config.ts`): Configures proxying and aliases. Uses TailwindCSS v4 Vite plugin, oxc minifier, experimental native plugin v1.
- **Environment**:

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKEND_BASE_URL` | http://alt-backend:9000 | REST API endpoint |
| `BACKEND_CONNECT_URL` | http://alt-backend:9101 | Connect-RPC endpoint (direct) |
| `AUTH_HUB_INTERNAL_URL` | http://auth-hub:8888 | Token exchange endpoint |
| `KRATOS_INTERNAL_URL` | http://kratos:4433 | Ory Kratos internal URL |
| `PUBLIC_USE_CONNECT_FEEDS` | false | Enable Connect-RPC for feeds |
| `PUBLIC_USE_CONNECT_ARTICLES` | false | Enable Connect-RPC for articles |
| `PUBLIC_USE_CONNECT_RSS` | false | Enable Connect-RPC for RSS |
| `PUBLIC_USE_CONNECT_STREAMING` | false | Enable Connect-RPC streaming features |

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

# Run unit tests (Vitest)
pnpm test

# Run E2E tests (Playwright)
pnpm test:e2e
```

### LLM / Dev Notes
- **Runes Mode**: This project strictly uses Svelte 5 Runes. Do not use legacy `export let` or `$:`. Use `$props()` and `$state()`.
- **Base Path**: Always remember the app runs under `/sv`. Links should be relative or account for this.
- **Tailwind v4**: No `tailwind.config.js` (mostly). Configuration is CSS-first in `src/app.css`.
- **SSR vs CSR**: Data loading happens in `+page.server.ts` (SSR) for initial state, but client-side interactions use `api.ts` (CSR).
- **Connect-RPC Direct**: All Connect-RPC calls go directly to alt-backend:9101 (not via BFF). The frontend handles JWT tokens from auth-hub.
- **Feature Flags for Migration**: Use feature flags (`PUBLIC_USE_CONNECT_*`) to gradually migrate from REST to Connect-RPC per feature area.
- **Dual Protocol**: REST calls go to alt-backend:9000, while Connect-RPC calls go to alt-backend:9101. This separation allows typed streaming via Connect-RPC while maintaining REST compatibility.
- **Generated Protobufs**: `src/lib/gen/` contains generated TypeScript from protobuf definitions. Do not edit manually.
