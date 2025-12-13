# alt-frontend to alt-frontend-sv Migration Execution Guide

This document serves as the **master instruction manual** for the migration of `alt-frontend` (Next.js) to `alt-frontend-sv` (SvelteKit). It is designed to be read and executed by an AI agent or developer, with explicit steps, code snippets, and verification criteria.

## 0. Prerequisite Context
- **Source Repo**: `alt-frontend` (Next.js 16, Chakra UI, SWR, Framer Motion)
- **Target Repo**: `alt-frontend-sv` (SvelteKit 2, Svelte 5 Runes, Tailwind CSS, shadcn-svelte, TanStack Query v5)
- **Runtime**: **Bun** (Development & Build), Node.js (Docker Deployment via `adapter-node`).
- **Design System**: "Premium" aesthetic (Dark mode, Glassmorphism, Tailwind).
    - **Themes to Port**: Vaporwave, Liquid-Beige, Alt-Paper.
- **Auth**: Ory Kratos (Cookie-based).
- **State Management**: Svelte 5 Runes (`$state`, `$derived`) + TanStack Query (for complex client-side caching).
- **Real-time**: Server-Sent Events (SSE) for feed stats.
- **Tooling**: Biome (Linting/Formatting).

---

## Phase 1: Project Scaffolding & Configuration

**Goal**: Initialize a production-ready SvelteKit project with the target stack, specifically configured for Svelte 5 and Bun.

### Step 1.1: Initialize Project
**Instruction**: Run the following commands in the parent directory (`/home/koko/Documents/dev/Alt`).

```bash
# 1. Create SvelteKit project (if not already done)
# npx sv create alt-frontend-sv --template minimal --types ts --no-add-ons
# (Select: TypeScript, Vitest, Playwright. DESELECT: ESLint, Prettier - we use Biome)

# 2. Navigate to directory
cd alt-frontend-sv

# 3. Clean up Deno artifacts (if switching from Deno)
rm deno.json deno.lock

# 4. Install dependencies with Bun
bun install
```

### Step 1.2: Install & Configure Tailwind CSS
**Instruction**: Use `sv add` to set up Tailwind CSS. **This is REQUIRED** before initializing shadcn-svelte.
```bash
bunx sv add tailwindcss
bun install
```

### Step 1.3: Install Core Dependencies
**Instruction**: Install the UI, Logic, and Animation libraries using `bun add`.
```bash
# UI Components (shadcn-svelte)
# Note: We will init shadcn-svelte in the next step.

# Auth & Validation
bun add @ory/client valibot

# Data Fetching (SWR Replacement)
bun add @tanstack/svelte-query

# Node Adapter for Docker (Crucial for deployment)
bun add -d @sveltejs/adapter-node

# Accessibility Testing
bun add -d axe-playwright

# Image Optimization
bun add -d @sveltejs/enhanced-img

# Biome (Linter/Formatter)
bun add -d --exact @biomejs/biome
bunx biome init
```

### Step 1.4: Initialize shadcn-svelte
**Instruction**: Initialize `shadcn-svelte` for Svelte 5.
```bash
bun x shadcn-svelte@latest init
```
**Configuration Prompts**:
-   **Style**: Default (or New York if preferred)
-   **Base Color**: Slate (or Zinc)
-   **CSS Variables**: Yes
-   **Tailwind CSS config**: `tailwind.config.ts`
-   **Components alias**: `$components` (Ensure this matches `svelte.config.js`)
-   **Utils alias**: `$utils` (Ensure this matches `svelte.config.js`)

**Action**:
1.  Verify `components.json` is created.
    -   **Critical Fix**: If `registry` URL is `https://next.shadcn-svelte.com/registry`, change it to `https://shadcn-svelte.com/registry` in `components.json`.
    -   **Critical Fix**: If `$schema` URL is `https://next.shadcn-svelte.com/schema.json`, change it to `https://shadcn-svelte.com/schema.json`.
2.  Update `tailwind.config.ts` to include the "Premium" color palette and existing themes.
    -   **Themes**: Define colors for `vaporwave`, `liquid-beige`, and `alt-paper` using CSS variables in `src/app.css` (or wherever shadcn put the base styles).
    -   **Glassmorphism**: Ensure utilities for `backdrop-blur`, `bg-opacity`, etc., are available.

### Step 1.5: Configure Aliases & Biome
**Instruction**:
1.  Edit `svelte.config.js` to ensure aliases match shadcn config:
    ```javascript
    // svelte.config.js
    import adapter from '@sveltejs/adapter-node';
    import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

    const config = {
        preprocess: vitePreprocess(),
        kit: {
            adapter: adapter(),
            alias: {
                $components: 'src/lib/components',
                $lib: 'src/lib',
                $stores: 'src/lib/stores',
                $utils: 'src/lib/utils'
            }
        }
    };
    export default config;
    ```
2.  Configure `biome.json` to handle Svelte files (experimental support) or ignore them if it causes issues, focusing on `.ts` files.
    ```json
    {
      "files": {
        "ignore": [".svelte-kit", "build", "node_modules", "bun.lockb"]
      },
      "formatter": {
        "enabled": true,
        "indentStyle": "space",
        "indentWidth": 4
      },
      "linter": {
        "enabled": true,
        "rules": {
          "recommended": true
        }
      }
    }
    ```

---

## Phase 2: Core Infrastructure (Auth, API, SSE)

**Goal**: Replicate the authentication middleware, API client logic, and SSE handling.

### Step 2.1: Server Hooks (Auth Middleware & Routing)
**Context**: Replaces `alt-frontend/middleware.ts`.
**Instruction**: Create `src/hooks.server.ts`.
**Code Specification**:
1.  **Public Route Logic**: Implement a check similar to the existing `PUBLIC_ROUTES` regex. If a user is unauthenticated and tries to access a protected route, redirect to `/public/landing` (or `/login`).
2.  **Auth Validation**:
    -   Import `FrontendApi` from `@ory/client`.
    -   Configure it to point to `http://kratos:4433` (internal Docker network).
    -   Implement `handle` function:
        -   Extract `cookie` header from `event.request`.
        -   Call `ory.toSession({ cookie })`.
        -   If valid, set `event.locals.user` and `event.locals.session`.
        -   Handle errors gracefully (set user to null).
        -   **Important**: Pass the session to the client via `event.locals`.

### Step 2.2: Type Definitions
**Instruction**: Update `src/app.d.ts`.
**Code Specification**:
```typescript
declare global {
    namespace App {
        interface Locals {
            user: import('@ory/client').Identity | null;
            session: import('@ory/client').Session | null;
        }
        interface PageData {
            user: import('@ory/client').Identity | null;
        }
        // ...
    }
}
export {};
```

### Step 2.3: Universal API Client
**Context**: Replaces `alt-frontend/src/lib/api.ts` and sub-modules.
**Instruction**: Create `src/lib/api/index.ts` and specific modules (feeds, articles, recap).
**Code Specification**:
-   Export a function `createApiClient(fetch: typeof globalThis.fetch)` that returns methods (`get`, `post`, etc.).
-   **Why**: SvelteKit's `load` functions provide a custom `fetch` that automatically forwards cookies to the server. We MUST use this `fetch` instance for server-side calls.

### Step 2.4: SSE Client (Real-time Feeds)
**Context**: Replaces `alt-frontend/src/lib/apiSse.ts`.
**Instruction**: Create `src/lib/api/sse.ts` (or similar).
**Implementation Details**:
-   Use Svelte 5 `$effect` to manage the `EventSource` connection lifecycle.
-   Implement auto-reconnect logic with exponential backoff (parity with existing implementation).
-   Expose a reactive state (e.g., using a class with `$state` fields) that components can subscribe to for feed stats.

---

## Phase 3: UI Design System (Atoms)

**Goal**: Rebuild base components using shadcn-svelte and Tailwind, replacing Chakra UI.

### Step 3.1: Install Base Components
**Instruction**: Use the CLI to add necessary components.
```bash
bunx shadcn-svelte@next add button input card separator skeleton
```

### Step 3.2: Customize Components
**Instruction**:
-   **Button**: Check `src/lib/components/ui/button/button.svelte`. Ensure variants match requirements.
-   **Card**: Check `src/lib/components/ui/card/card.svelte`. Add "glassmorphism" styles to the base class or create a variant if needed.
    -   *Example*: Add `bg-card/50 backdrop-blur-lg border-white/10` to the Card root.

---

## Phase 4: Feature Migration (Molecules & Pages)

**Goal**: Port business logic and complex UIs, respecting the Desktop/Mobile split where necessary.

### Step 4.1: Feed Card Component
**Context**: `alt-frontend/src/components/Feed/FeedCard.tsx`.
**Instruction**: Create `src/lib/components/feed/feed-card.svelte`.
**Changes**:
-   **Layout**: Convert `HStack`/`VStack` to `flex flex-row gap-x` / `flex flex-col gap-y`.
-   **Animation**: Replace `framer-motion` with `svelte/transition` (`fade`, `fly`) or `svelte/motion` (`spring`).
-   **Props**: Define interface using `$props()`.

### Step 4.2: Home Page (Feed) & Desktop/Mobile Views
**Context**: `alt-frontend/src/app/page.tsx`, `src/app/desktop`, `src/app/mobile`.
**Instruction**:
1.  **Route Structure**:
    -   `src/routes/+page.svelte`: Main entry point.
2.  **Data Loading**:
    -   `src/routes/+page.server.ts`: Fetch feed data using `createApiClient(fetch)`.
    -   **Performance**: Use SvelteKit's promise streaming (return a promise in the load function) for non-critical data.
3.  **Infinite Scroll**:
    -   Implement using `@tanstack/svelte-query`'s `createInfiniteQuery`.

### Step 4.3: Article Viewer
**Context**: `alt-frontend/src/app/article/[id]/page.tsx`.
**Instruction**:
1.  Create `src/routes/article/[id]/+page.server.ts`:
    -   Get `params.id`.
    -   Fetch article details.
    -   Sanitize HTML on the server if possible, or use `dompurify` in the component.
2.  Create `src/routes/article/[id]/+page.svelte`:
    -   Render content using `{@html sanitizedContent}`.
    -   **Security**: Ensure `dompurify` is used before rendering.

### Step 4.4: Recap Feature (Mobile)
**Context**: `alt-frontend/src/app/mobile/recap/7days/page.tsx`.
**Instruction**:
1.  Create `src/routes/mobile/recap/7days/+page.svelte` (or unified route).
2.  Migrate `useRecapData` hook logic to a `load` function or Svelte 5 reactive state.
3.  Port `RecapCard` and `EvidenceList` components.

---

## Phase 5: Verification & Cleanup

**Goal**: Ensure parity, stability, and accessibility.

### Step 5.1: E2E Testing
**Instruction**:
-   Copy `alt-frontend/e2e` to `alt-frontend-sv/e2e`.
-   Update `playwright.config.ts` `webServer` command to `bun run build && bun run preview` (or `bun run dev`).
-   **Crucial**: Svelte components often need `data-testid` added manually.
-   **Accessibility**: Ensure `axe-playwright` tests are running and passing (WCAG 2.1 AA).

### Step 5.2: Docker Integration
**Instruction**:
-   Create `Dockerfile` for SvelteKit (Node adapter).
    -   *Note*: Even though we use Bun for dev, the `adapter-node` build output is a standard Node.js app. We can use a standard Node.js Dockerfile.
-   Update `docker-compose.yml` to point to the new frontend service.

### Step 5.3: Performance & Security Check
**Instruction**:
-   **Images**: Use `@sveltejs/enhanced-img` or `loading="lazy"` for all images.
-   **Security**: Configure Content Security Policy (CSP) in `svelte.config.js` or `hooks.server.ts` to prevent XSS.
-   **Bundle Size**: Run `bun run build` and check the output size. Use dynamic imports (`await import(...)`) for heavy components.

---

## Execution Checklist for LLM

When executing this plan, follow this loop for each step:
1.  **Read Context**: Analyze the source file in `alt-frontend`.
2.  **Generate Code**: Write the Svelte 5 equivalent in `alt-frontend-sv`.
3.  **Verify**: Check for type errors and basic functionality.
4.  **Commit**: `git add . && git commit -m "feat: migrate [component/page]"`

**Critical Rules**:
-   **Do NOT** use `export let` (Svelte 4). Use `$props()`.
-   **Do NOT** use `$:`. Use `$derived()` or `$effect()`.
-   **Do NOT** use `class` for styling. Use Tailwind utility classes.
-   **ALWAYS** check for `client` vs `server` logic separation.
