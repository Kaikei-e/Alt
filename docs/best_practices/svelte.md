# Svelte 5 & SvelteKit Best Practices — Alt

## 1. Svelte 5 Runes

- Use `$state` for state that should trigger UI updates; plain `let` variables are still fine for non-reactive locals
- Use `$derived` for computed values — replaces `$:` reactive declarations
- Use `$effect` for browser-only side effects (DOM, external libraries, subscriptions, network coordination)
- Use `$props()` with destructuring for component props — replaces `export let`
- Prefer a single component style; avoid mixing legacy reactivity (`$:`) and rune-based reactivity in the same component unless you are migrating incrementally

### $state

- `$state<T>(initial)` creates a deeply reactive proxy — mutations to nested properties trigger updates automatically
- Use `$state.raw<T>(initial)` for large immutable data replaced wholesale (avoids proxy overhead on every property access)
- Use `$state.snapshot(value)` to extract a plain (non-proxied) object — useful for logging, serialization, or passing to external libraries

```typescript
// ✅ Basic $state with type annotation
let selectedProjectId = $state<string | null>(null);
let targets = $state<Target[]>([]);
let filters = $state<FindingFilters>({ activeOnly: true });

// ✅ $state.raw for large data sets that are replaced, not mutated
let findings = $state.raw<Finding[]>([]);
// Replace entirely — triggers reactivity
findings = newFindings;
// ❌ findings.push(item) — would NOT trigger reactivity with .raw

// ✅ $state.snapshot for serialization
console.log($state.snapshot(filters));
```

### $derived / $derived.by()

- Use `$derived(expression)` for single-expression computations
- Use `$derived.by(() => { ... })` for multi-line computations
- Never cause side effects inside `$derived` — use `$effect` instead

```typescript
// ✅ Single-expression derivation
const ecosystems = $derived(
  [...new Set(appState.findings.map((f) => f.ecosystem))].sort()
);

// ✅ Multi-line derivation with $derived.by()
const filteredFindings = $derived.by(() => {
  let result = appState.findings;
  if (appState.filters.severity) {
    result = result.filter((f) => f.severity === appState.filters.severity);
  }
  return result;
});

// ❌ Side effect in $derived — use $effect instead
const bad = $derived(fetch('/api/data'));
```

### $effect

- Use `$effect` for DOM manipulation, external library integration, and network requests
- Remember that `$effect` runs only in the browser, never during server-side rendering
- Return a cleanup function from `$effect` for resource disposal (observers, listeners, connections)
- Do not derive state inside `$effect`; use `$derived` instead
- Avoid updating `$state` inside effects unless you are bridging to an external system and the dependency flow is explicit
- Read reactive values at the top of `$effect` to establish dependencies, use `untrack()` for reads that should not trigger re-runs

```svelte
<script lang="ts">
// ✅ $effect with cleanup — ResizeObserver
$effect(() => {
  if (!containerEl) return;
  const observer = new ResizeObserver(() => renderer?.resize());
  observer.observe(containerEl);
  return () => observer.disconnect();
});

// ✅ $effect for external library lifecycle
$effect(() => {
  const type = rendererType;
  const container = containerEl;
  if (!container) return;

  let cancelled = false;
  (async () => {
    const r = await createRenderer(type);
    if (cancelled) return;
    r.mount(container);
    renderer = r;
  })();

  return () => {
    cancelled = true;
    renderer?.dispose();
    renderer = null;
  };
});

// ✅ Read dependencies first, then synchronize an external resource
$effect(() => {
  const id = projectId; // tracked dependency
  if (!id) return;

  sseManager.connect(id);

  return () => sseManager.disconnect();
});

// ❌ State synchronization — use $derived instead
$effect(() => {
  fullName = firstName + ' ' + lastName;
});
</script>
```

### $effect Dependency-Tracking Pitfalls

- `$effect` tracks every reactive source read synchronously **across the whole call stack** — `$state` reads inside functions called from the effect are registered too. If such a function also writes that state, the effect re-fires itself: self-refiring loops have produced request storms and `effect_update_depth_exceeded` crashes (ADR-000320, ADR-000441, PM-2026-039)
- Guards against self-refire: write the guard condition directly in the effect body (not inside a helper that reads a `$state` flag), wrap reads that must not become dependencies in `untrack()`, or gate on a `$derived` value so the effect only re-runs when the value actually changes (ADR-000441, ADR-000847)
- Dependency tracking is **runtime, not static**: with `if (!a || b) return`, `b` is never read — and never registered — while `a` is falsy, so the dependency set shifts between runs and the effect behaves unstably. Keep guards to a single variable and rely on template structure (`bind:this` inside `{#if}`) for implicit guarantees (ADR-000224)
- Auto-fetch effects must record a terminal state on "empty success" responses — otherwise the still-empty state re-triggers the fetch forever (ADR-000581)

```typescript
// ❌ Guard flag read and written inside a helper — tracked, loops forever
$effect(() => {
  syncIfNeeded(); // reads + writes isFetching ($state) internally
});

// ✅ Guard directly in the effect body, non-dependency work in untrack()
$effect(() => {
  if (isFetching) return;
  const id = articleId;
  untrack(() => startFetch(id));
});
```

> **Alt:** In `alt-frontend-sv`, `.svelte.ts` singleton state modules are appropriate for shared client-side state. Use `$effect` in route components for lifecycle work such as subscriptions, observers, and streaming connections.

## 2. Component Patterns

### Props: interface Props + $props()

- Define a `Props` interface before destructuring with `$props()`
- Use default values in the destructuring pattern
- Use `$bindable()` only when two-way binding is genuinely needed

```svelte
<script lang="ts">
interface Props {
  graphModel: GraphModel;
  rendererType: RendererChoice;
  onNodeClick: (nodeId: string) => void;
  onNodeHover: (nodeId: string | null, position?: { x: number; y: number }) => void;
}

const { graphModel, rendererType, onNodeClick, onNodeHover }: Props = $props();

// ✅ With defaults
interface Props {
  limit?: number;
  showHeader?: boolean;
}
const { limit = 10, showHeader = true }: Props = $props();

// ✅ $bindable for two-way binding (use sparingly)
interface Props {
  value: string;
}
const { value = $bindable('') }: Props = $props();
</script>
```

### Snippets (replacing slots)

- Prefer `{#snippet name(params)}...{/snippet}` + `{@render name(params)}` in Svelte 5
- Accept `children` as a prop for the default content
- Type snippet props with `Snippet<[ParamTypes]>` from `'svelte'`

```svelte
<!-- ✅ Root layout — children snippet -->
<script lang="ts">
let { children } = $props();
</script>

<main>
  {@render children()}
</main>

<!-- ✅ Named snippet with typed parameters -->
<script lang="ts">
import type { Snippet } from 'svelte';

interface Props {
  items: Finding[];
  row: Snippet<[Finding]>;
  empty?: Snippet;
}

const { items, row, empty }: Props = $props();
</script>

{#if items.length === 0}
  {#if empty}{@render empty()}{/if}
{:else}
  {#each items as item}
    {@render row(item)}
  {/each}
{/if}

<!-- ✅ Inline snippet at call site -->
<DataTable items={findings}>
  {#snippet row(finding)}
    <tr><td>{finding.advisory_id}</td></tr>
  {/snippet}
</DataTable>
```

### Event Handling

- Prefer native HTML attributes for DOM events: `onclick`, `oninput`, `onchange`
- Prefer callback props for component events; `createEventDispatcher` is legacy in rune-first code
- Name callback props with `on` prefix: `onNodeClick`, `onFiltersChange`

```svelte
<!-- ✅ DOM events — HTML attribute syntax -->
<button onclick={() => renderer?.zoomIn()}>Zoom In</button>
<select onchange={setSeverity}>...</select>
<input type="range" oninput={setMinEpss} />
<svelte:window onkeydown={handleKeydown} />

<!-- ✅ Component events — callback props -->
<FilterBar
  {filters}
  {rendererType}
  {ecosystems}
  onFiltersChange={(f) => appState.filters = f}
  onRendererChange={(t) => appState.rendererType = t}
/>

<!-- ❌ Svelte 4 syntax — do not use -->
<button on:click={handler}>...</button>
```

> **Alt:** Prefer callback props for new rune-first components in Alt. Reserve legacy event dispatchers for incremental migration only.

### Keyed {#each} and Duplicate Keys

- Duplicate keys in a keyed `{#each}` corrupt the reconciler's internal linked list and crash with `RangeError`/`TypeError` — **no warning is emitted** before the crash (ADR-000228, ADR-000229)
- Key by an ID that is unique per rendered item: lists that can return multiple articles from the same feed URL must key by the article-specific ID, not the feed URL (ADR-000237)
- Keying IDs are a presentation-layer concern — generate a stable-enough UUID fallback at the API-mapping boundary rather than leaking DB primary keys into the domain layer (ADR-000238)
- Offset pagination against a backend with dynamic ranking (e.g. Meilisearch hybrid search) can return the same ID on consecutive pages — "pages are disjoint" is an implicit contract that ranking changes silently break. Dedupe on the frontend before appending (PM-2026-044)

```typescript
// ✅ Pure helper — dedupe before append so the keyed each stays safe
export function appendUniqueById<T extends { id: string }>(current: T[], incoming: T[]): T[] {
  const seen = new Set(current.map((item) => item.id));
  return [...current, ...incoming.filter((item) => !seen.has(item.id))];
}
```

## 3. State Management

### .svelte.ts Files with Rune State

- Use `.svelte.ts` extension for files that contain `$state`, `$derived`, or `$effect` runes
- Export a singleton created by a factory function — the closure captures the `$state` variables
- Expose state via methods or getter/setter pairs when the state will be reassigned across module boundaries

```typescript
// ✅ lib/state/app.svelte.ts — closure + getter/setter pattern
import type { Finding, Target } from '$lib/types/api';

export type RendererType = '3d' | '2d';

export interface FindingFilters {
  severity?: string;
  ecosystem?: string;
  minEpss?: number;
  activeOnly: boolean;
}

function createAppState() {
  let selectedProjectId = $state<string | null>(null);
  let targets = $state<Target[]>([]);
  let findings = $state<Finding[]>([]);
  let filters = $state<FindingFilters>({ activeOnly: true });
  let rendererType = $state<RendererType>('3d');
  let error = $state<string | null>(null);

  return {
    get selectedProjectId() { return selectedProjectId; },
    set selectedProjectId(v: string | null) { selectedProjectId = v; },

    get targets() { return targets; },
    set targets(v: Target[]) { targets = v; },

    get findings() { return findings; },
    set findings(v: Finding[]) { findings = v; },

    get filters() { return filters; },
    set filters(v: FindingFilters) { filters = v; },

    get rendererType() { return rendererType; },
    set rendererType(v: RendererType) { rendererType = v; },

    get error() { return error; },
    set error(v: string | null) { error = v; },

    reset() {
      selectedProjectId = null;
      targets = [];
      findings = [];
      filters = { activeOnly: true };
      rendererType = '3d';
      error = null;
    },
  };
}

export const appState = createAppState();
```

### Context API (setContext / getContext)

- Use `setContext`/`getContext` for state scoped to a component tree (SSR-safe)
- Global singletons are fine for `ssr: false` pages, but use context when SSR is possible
- Pass getter functions through context to preserve reactivity

```svelte
<!-- ✅ Context API for tree-scoped state -->
<script lang="ts">
import { setContext } from 'svelte';

const state = createSomeState();
setContext('my-state', state);
</script>

<!-- Child component -->
<script lang="ts">
import { getContext } from 'svelte';

const state = getContext<ReturnType<typeof createSomeState>>('my-state');
</script>
```

### Bridging Non-Reactive Class Instances

- Module-scope class instances live outside runes reactivity — wrapping one in `$state(...)` does not make mutations performed by its own methods observable (ADR-000269)
- Bridge with a change callback plus a version counter: the instance notifies on mutation, a `$state` counter increments, and `$derived.by()` reads the counter to re-query the instance

```typescript
let version = $state(0);
manager.onChange(() => { version++; });

const entries = $derived.by(() => {
  version; // establish the dependency
  return manager.snapshot();
});
```

### When NOT to Use Global State

- Do not put UI-only state (open/close, hover, scroll position) in global state
- Do not put derived data in state — use `$derived` instead
- Do not put server-loaded data in state when SvelteKit `load()` data suffices

> **Alt:** Use global singletons only for genuinely app-wide client state. If a route can render on the server, prefer context or route-local state over module singletons.

## 4. SvelteKit Routing & Data Loading

### File-Based Routing

| File | Purpose |
|------|---------|
| `+page.svelte` | Page component |
| `+page.ts` | Universal load function (runs on server + client) |
| `+page.server.ts` | Server-only load function (DB, secrets) |
| `+layout.svelte` | Shared layout wrapping child pages |
| `+layout.ts` | Layout load function (data shared with children) |
| `+server.ts` | API endpoint (GET, POST, etc.) |
| `+error.svelte` | Error boundary |

### load() Functions

- Use `+page.ts` / `+layout.ts` for universal load functions (run on both server and client)
- Use `+page.server.ts` for server-only data (DB queries, secrets)
- Accept `{ params, fetch, url }` from the load function argument
- Always use the SvelteKit-provided `fetch` — it handles cookies, relative URLs, and server-side optimization
- Use `Promise.all` to parallelize independent data fetches

```typescript
// ✅ Parallel data loading in +layout.ts
import { getFindings, getTargets, getTopRisks } from '$lib/api/client';
import type { LayoutLoad } from './$types';

export const ssr = false;

export const load: LayoutLoad = async ({ params, fetch }) => {
  const { projectId } = params;
  const [targets, findings, topRisks] = await Promise.all([
    getTargets(projectId, fetch),
    getFindings(projectId, { active_only: true }, fetch),
    getTopRisks(projectId, undefined, fetch),
  ]);

  return {
    projectId,
    targets,
    findings: findings.data,
    topRisks,
  };
};
```

### Layout Data Inheritance

- Data returned from `+layout.ts` is available in all child `+page.svelte` and nested `+layout.svelte`
- Access via `let { data, children } = $props()` in layout components

```svelte
<!-- ✅ +layout.svelte consuming load data -->
<script lang="ts">
let { children, data } = $props();

const projectId = $derived(page.params.projectId);

$effect(() => {
  const id = projectId;
  if (!id) return;
  untrack(() => {
    appState.targets = data.targets;
    appState.findings = data.findings;
  });
});
</script>

{@render children()}
```

### Invalidation & Stream-Driven Refresh

- Never call `invalidateAll()` unconditionally from a stream/SSE callback — combined with `$effect`s that read `data`, one event amplifies into a fetch-storm positive feedback loop (50+ fetches in the same millisecond, `ERR_INSUFFICIENT_RESOURCES`) (ADR-000847, PM-2026-039)
- Scope invalidation: register `depends('scoped:key')` in `load()` and call `invalidate('scoped:key')` so only the affected loads re-run
- Debounce stream-driven refreshes and coalesce concurrent ones with a single-flight guard — at most one refresh in flight and one pending
- Gate `$effect`s that react to `data` behind value equality (`$derived` + `untrack`) so re-runs with unchanged values are no-ops (ADR-000847)

### URL Query State

- Read search/filter state from URL query params **once in `onMount`** — watching the URL in an `$effect` and writing it back into the input steals input ownership from the user while typing (ADR-000453)
- Redirects must carry `url.search` forward; extract query-parsing logic into pure functions and test them directly (ADR-000454)

### Route Groups

- Use parentheses `(groupName)/` to share layouts without affecting the URL
- Example: `routes/(app)/dashboard/+page.svelte` → URL is `/dashboard`

### API Endpoints (+server.ts)

```typescript
// ✅ Health check endpoint
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async () => {
  return new Response(
    JSON.stringify({ status: 'ok', service: 'alt-frontend-sv' }),
    { headers: { 'content-type': 'application/json' } },
  );
};
```

> **Alt:** In `alt-frontend-sv`, prefer `+layout.ts` when multiple child routes need the same upstream data, and pass SvelteKit's `fetch` through to shared API clients.

## 5. SSR, CSR & Prerendering

- Default: SvelteKit renders pages on the server (SSR) then hydrates on the client (CSR)
- Set `export const ssr = false` for pages using browser-only APIs (Three.js, WebGPU, Canvas, WebGL)
- Set `export const prerender = true` for static pages that don't change per request
- These options can be set in `+page.ts`, `+page.server.ts`, or `+layout.ts` (layout options cascade to children)

```typescript
// ✅ Disable SSR for browser-only pages
export const ssr = false;

// ✅ Enable prerendering for static pages
export const prerender = true;

// ✅ Disable CSR for fully server-rendered pages (rare)
export const csr = false;
```

> **Alt:** Do not disable SSR globally. Set `ssr = false` narrowly on routes that need browser-only rendering primitives such as WebGL, WebGPU, or direct DOM APIs.

## 6. TypeScript

### Props Typing

- Define `interface Props` in each component — keep it colocated, not shared
- Use generic components with `generics="T"` on the `<script>` tag when needed

```svelte
<script lang="ts">
interface Props {
  filters: FindingFilters;
  rendererType: RendererType;
  ecosystems: string[];
  onFiltersChange: (filters: FindingFilters) => void;
  onRendererChange: (type: RendererType) => void;
}

const { filters, rendererType, ecosystems, onFiltersChange, onRendererChange }: Props = $props();
</script>
```

### Snippet Typing

```typescript
import type { Snippet } from 'svelte';

interface Props {
  children: Snippet;
  header?: Snippet<[string]>;
}
```

### API Type Definitions

- Mirror backend types exactly — match field names, casing, and optionality
- Keep API types in a dedicated `types/api.ts` file
- Use `snake_case` field names to match the Go gateway JSON serialization

```typescript
// ✅ lib/types/api.ts — match Go gateway response exactly
export interface Finding {
  instance_id: string;
  target_id: string;
  target_name: string;
  package_name: string;
  package_version: string;
  ecosystem: string;
  advisory_id: string;
  severity?: string;
  ranking_score?: number;
  epss_score?: number;
  cvss_score?: number;
  is_active: boolean;
  last_observed_at?: string;
}

export interface PagedResponse<T> {
  data: T[];
  next_cursor?: string;
  has_more: boolean;
}

export interface ApiError {
  error: {
    code: string;
    message: string;
  };
}
```

### Backend Contract Discipline

- Do not run ahead of the backend contract: placeholder fields or UI actions with no backend support create contract drift and "looks wired but isn't" experiences — shrink the UI to what the contract actually provides (ADR-000492)
- Do not infer state on the frontend: "excerpt is empty → show Summarizing…" cannot distinguish not-requested / in-progress / failed, so the UX lies. Consume an explicit state field (e.g. `summary_state`) computed by the backend projection (ADR-000424)

> **Alt:** Keep `types/api.ts` aligned with backend JSON contracts. Preserve field names and optionality unless the backend contract itself changes.

## 7. API Client & Data Fetching

### SvelteKit fetch Injection

- Accept `customFetch: typeof fetch = fetch` as a parameter in API functions
- In `load()` functions, pass SvelteKit's `fetch` — it handles cookies and relative URLs
- Outside `load()`, the global `fetch` is used as the default

```typescript
// ✅ lib/api/client.ts — customFetch pattern
import { env } from '$env/dynamic/public';

const BASE_URL = env.PUBLIC_GATEWAY_URL ?? 'http://localhost:8400';

async function request<T>(
  path: string,
  unwrapData = true,
  customFetch: typeof fetch = fetch,
): Promise<T> {
  const res = await customFetch(`${BASE_URL}${path}`);
  if (!res.ok) {
    const body = (await res.json()) as ApiError;
    throw body;
  }
  const body = (await res.json()) as T | DataEnvelope<T>;
  if (unwrapData && typeof body === 'object' && body !== null
      && 'data' in body && !('has_more' in body)) {
    return (body as DataEnvelope<T>).data;
  }
  return body as T;
}

// ✅ Each API function accepts customFetch
export function getProjects(customFetch: typeof fetch = fetch): Promise<Project[]> {
  return request<Project[]>('/api/projects', true, customFetch);
}

export function getFindings(
  projectId: string,
  params?: FindingFilterParams,
  customFetch: typeof fetch = fetch,
): Promise<PagedResponse<Finding>> {
  const qs = buildQuery((params ?? {}) as Record<string, string | number | boolean | undefined>);
  return request<PagedResponse<Finding>>(
    `/api/projects/${projectId}/findings${qs}`,
    false,
    customFetch,
  );
}

// ✅ Called from +layout.ts with SvelteKit fetch
export const load: LayoutLoad = async ({ params, fetch }) => {
  const targets = await getTargets(params.projectId, fetch);
  // ...
};
```

### SSE (Server-Sent Events)

- Use the native `EventSource` API for SSE connections
- Track `lastEventId` for reconnection recovery
- Clean up connections in `$effect` cleanup or on disconnect

```typescript
// ✅ lib/api/sse.ts — SSE connection factory
export interface SSEConnection {
  close: () => void;
}

export function createSSEConnection(options: SSEOptions): SSEConnection {
  let url = options.url;
  if (options.lastEventId) {
    const separator = url.includes('?') ? '&' : '?';
    url = `${url}${separator}lastEventId=${encodeURIComponent(options.lastEventId)}`;
  }

  const source = new EventSource(url);

  source.onmessage = (e: MessageEvent) => {
    options.onEvent({
      id: e.lastEventId ?? '',
      type: e.type,
      data: e.data,
    });
  };

  source.onerror = (e: Event) => {
    options.onError?.(e);
  };

  return { close: () => source.close() };
}
```

### Stale-Response Guards for Async Callbacks

- Every async callback that writes UI state needs a stale-response guard: capture the request-time identifier (article ID, URL) and compare it against the current value before applying the result — otherwise a navigation-crossed stream overwrites the new screen with old content. `AbortController` alone is not enough; an abort can lose the race with an already-scheduled callback (ADR-000552, PM-2026-003)
- Check `signal.aborted` in `catch` blocks too — a reconnect-on-error loop that ignores abort races against `$effect` cleanup and resurrects dead streams (PM-2026-045)

```typescript
$effect(() => {
  const id = articleId;
  const controller = new AbortController();
  (async () => {
    try {
      for await (const chunk of streamSummary(id, controller.signal)) {
        if (articleId !== id) return; // stale-response guard
        content += chunk;
      }
    } catch (err) {
      if (controller.signal.aborted) return; // no reconnect after cleanup
      error = String(err);
    }
  })();
  return () => controller.abort();
});
```

### fetch in Background Tabs

- `await fetch` can stay pending forever when the tab is backgrounded or enters bfcache — an in-flight gate keyed on that promise then deadlocks the UI permanently (ADR-000858)
- Triple defense: (a) `AbortController` + timeout so the promise always settles and `finally` releases the gate, (b) garbage-collect stale gate entries when the underlying snapshot is replaced, (c) explicitly reset in-flight state on `visibilitychange`/`pageshow`

> **Alt:** Wrap streaming transports behind a small client module and let route or layout components own connection lifecycle through `$effect` cleanup.

## 8. Three.js / WebGPU Integration

### SceneRenderer Interface

- Define an interface for renderers — decouple components from specific implementations
- Implement mount/dispose lifecycle methods for clean resource management
- Support capability-based fallback (WebGL2 → Canvas2D)

```typescript
// ✅ lib/renderer/types.ts — renderer contract
export interface SceneRenderer {
  mount(container: HTMLElement): void;
  dispose(): void;
  setGraphModel(model: GraphModel): void;
  focusCluster(clusterId: string): void;
  focusNode(nodeId: string): void;
  resetView(): void;
  zoomIn(): void;
  zoomOut(): void;
  setViewPreset(preset: 'top' | 'front'): void;
  onNodeClick(callback: (nodeId: string) => void): void;
  onNodeHover(callback: (nodeId: string | null, position?: { x: number; y: number }) => void): void;
  resize(): void;
}
```

### Renderer Factory (Capability Detection)

```typescript
// ✅ lib/renderer/factory.ts — fallback chain
export type RendererChoice = '3d' | '2d';

export async function createRenderer(choice?: RendererChoice): Promise<SceneRenderer> {
  if (choice === '2d') {
    return new Canvas2DRenderer();
  }
  const cap = await detectCapability();
  if (cap === 'webgl2') {
    return new ThreeSceneRenderer();
  }
  return new Canvas2DRenderer();
}
```

### $effect Lifecycle for Renderers

- Create and mount the renderer inside `$effect`
- Return a cleanup function that calls `dispose()` — prevents WebGL context leaks
- Handle async renderer creation with a `cancelled` flag to avoid race conditions

```svelte
<script lang="ts">
let containerEl: HTMLElement | undefined = $state();
let renderer: SceneRenderer | null = $state(null);

// ✅ Renderer lifecycle — create, mount, cleanup
$effect(() => {
  const type = rendererType;
  const container = containerEl;
  if (!container) return;

  let cancelled = false;

  (async () => {
    let r: SceneRenderer;
    try {
      r = await createRenderer(type);
      if (cancelled || !containerEl) return;
      r.mount(container);
    } catch (err) {
      console.warn('3D renderer failed, falling back to 2D:', err);
      r = await createRenderer('2d');
      if (cancelled || !containerEl) return;
      r.mount(container);
    }
    r.onNodeClick(onNodeClick);
    r.onNodeHover(onNodeHover);
    r.setGraphModel(graphModel);
    renderer = r;
  })();

  return () => {
    cancelled = true;
    renderer?.dispose();
    renderer = null;
  };
});

// ✅ Separate $effect for reactive graph updates
$effect(() => {
  if (renderer) {
    renderer.setGraphModel(graphModel);
  }
});

// ✅ ResizeObserver via $effect cleanup
$effect(() => {
  if (!containerEl) return;
  const observer = new ResizeObserver(() => renderer?.resize());
  observer.observe(containerEl);
  return () => observer.disconnect();
});
</script>
```

> **Alt:** For WebGL or Three.js integrations, keep mount, resize, and dispose behavior inside `$effect` cleanup paths so GPU resources are released deterministically.

## 9. Styling (Tailwind CSS v4)

### @theme Design Tokens

- Define design tokens in `app.css` using `@theme { }` — the Tailwind v4 way
- Use semantic color names (`hud-danger`, `hud-safe`) instead of raw hex values
- Access tokens as Tailwind utilities: `bg-hud-base`, `text-hud-accent`, `border-hud-border`

```css
/* ✅ app.css — Tailwind v4 @theme */
@import "tailwindcss";

@theme {
  --color-hud-void: #050a0e;
  --color-hud-base: #0a1118;
  --color-hud-surface: #111a24;
  --color-hud-accent: #00e5ff;
  --color-hud-accent-dim: #006680;
  --color-hud-text: #e0f0ff;
  --color-hud-text-secondary: #7a9ab5;
  --color-hud-text-muted: #3d5a73;
  --color-hud-danger: #ff1744;
  --color-hud-warning: #ff9100;
  --color-hud-caution: #ffd600;
  --color-hud-info: #448aff;
  --color-hud-safe: #00e676;

  --font-mono: "JetBrains Mono", monospace;
  --font-sans: "Inter", system-ui, sans-serif;
}
```

### Utility-First + Custom HUD Classes

- Prefer Tailwind utility classes for one-off styling
- Define reusable HUD classes in `app.css` for complex patterns (`.hud-panel`, `.hud-border-glow`, `.hud-scanlines`)
- Use scoped `<style>` blocks in `.svelte` files only for component-specific styles that can't be expressed as utilities

```svelte
<!-- ✅ Tailwind utilities + HUD custom classes -->
<div class="hud-panel flex flex-wrap items-center gap-3 px-4 py-2 backdrop-blur bg-hud-base/80">
  <select class="bg-hud-surface text-hud-text border border-hud-border rounded-sm px-2 py-1 font-mono text-xs">
    ...
  </select>
</div>

<!-- ✅ Scoped styles for component-specific CSS -->
<style>
  .custom-animation {
    animation: pulse 2s ease-in-out infinite;
  }
</style>
```

> **Alt:** Treat the HUD palette here as an example of a coherent tokenized theme, not as a product-wide mandate. Match the active design system of the app you are editing.

## 10. Testing

### Vitest + @testing-library/svelte

- Use Vitest as the test runner — configure in `vite.config.ts`
- Use `@testing-library/svelte` for component tests — promotes testing user behavior
- Use `jsdom` environment for DOM simulation
- Colocate test files next to implementation: `app.svelte.ts` → `app.svelte.test.ts`

```typescript
// ✅ vite.config.ts — test configuration
export default defineConfig({
  test: {
    environment: 'jsdom',
    setupFiles: ['src/test-setup.ts'],
    include: ['src/**/*.test.ts'],
  },
});
```

```typescript
// ✅ src/test-setup.ts
import '@testing-library/jest-dom/vitest';
```

### State Module Testing (.svelte.ts)

- Import the singleton, call `reset()` in each test to ensure clean state
- Test getter/setter behavior and methods directly

```typescript
// ✅ lib/state/app.svelte.test.ts
import { describe, expect, it } from 'vitest';
import { appState } from './app.svelte';

describe('appState', () => {
  it('initializes with default values', () => {
    appState.reset();
    expect(appState.selectedProjectId).toBeNull();
    expect(appState.targets).toEqual([]);
    expect(appState.findings).toEqual([]);
    expect(appState.filters).toEqual({ activeOnly: true });
    expect(appState.rendererType).toBe('3d');
    expect(appState.error).toBeNull();
  });

  it('sets filters', () => {
    appState.reset();
    appState.filters = { severity: 'HIGH', activeOnly: false };
    expect(appState.filters.severity).toBe('HIGH');
    expect(appState.filters.activeOnly).toBe(false);
  });

  it('reset clears all state', () => {
    appState.selectedProjectId = '1';
    appState.error = 'err';
    appState.rendererType = '2d';

    appState.reset();

    expect(appState.selectedProjectId).toBeNull();
    expect(appState.error).toBeNull();
    expect(appState.rendererType).toBe('3d');
  });
});
```

### API Client Testing (fetch mocking)

```typescript
// ✅ Mock $env and fetch for API tests
import { describe, expect, it, vi } from 'vitest';

vi.mock('$env/dynamic/public', () => ({
  env: { PUBLIC_GATEWAY_URL: 'http://localhost:8400' },
}));

describe('getProjects', () => {
  it('fetches projects with custom fetch', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ data: [{ id: '1', name: 'test' }] }),
    });

    const projects = await getProjects(mockFetch);
    expect(mockFetch).toHaveBeenCalledWith('http://localhost:8400/api/projects');
    expect(projects).toEqual([{ id: '1', name: 'test' }]);
  });
});
```

### Waiting on $state in Browser-Mode Tests

- `vi.waitFor` polls outside any reactive context, so it cannot observe `$state`/`$derived` changes; fake timers flush microtasks incompletely (ADR-000477)
- Wrap the assertion in `$effect.root` + an `$effect` and resolve a promise when the expected state appears

```typescript
function waitForState(predicate: () => boolean): Promise<void> {
  return new Promise((resolve) => {
    const stop = $effect.root(() => {
      $effect(() => {
        if (predicate()) {
          resolve();
          queueMicrotask(stop);
        }
      });
    });
  });
}
```

- Mock streams must hang instead of completing — a stream that resolves `done` immediately does not mimic a live SSE connection and injects reconnect noise into the test (ADR-000477)

> **Alt:** In this repo, use `cd alt-frontend-sv && bun run test`, `cd alt-frontend-sv && bun run test:watch`, or run a focused Vitest command for a single file.

## 11. Linting & Formatting (Biome)

- Use Biome for both linting and formatting — no ESLint or Prettier
- Configure in `biome.json` — tab indentation, 100-char line width, single quotes
- Disable `noUnusedVariables` and `noUnusedImports` for `.svelte` files (Svelte uses variables in the template that Biome cannot see)
- Enable `tailwindDirectives: true` in the CSS parser for `@theme`, `@apply`, etc.

```json
{
  "$schema": "https://biomejs.dev/schemas/2.4.7/schema.json",
  "files": {
    "includes": ["src/**", "*.ts", "*.js", "!!**/build", "!!**/.svelte-kit", "!!**/node_modules"]
  },
  "linter": {
    "enabled": true,
    "rules": {
      "recommended": true,
      "correctness": {
        "noUnusedVariables": "error",
        "noUnusedImports": "error"
      }
    }
  },
  "formatter": {
    "enabled": true,
    "indentStyle": "tab",
    "lineWidth": 100
  },
  "css": {
    "parser": {
      "tailwindDirectives": true
    }
  },
  "javascript": {
    "formatter": {
      "quoteStyle": "single"
    }
  },
  "overrides": [
    {
      "includes": ["**/*.svelte"],
      "linter": {
        "rules": {
          "correctness": {
            "noUnusedVariables": "off",
            "noUnusedImports": "off"
          }
        }
      }
    }
  ]
}
```

```bash
# Lint check
bun run lint

# Auto-fix
bunx @biomejs/biome lint --write .

# Format
bun run format

# Type check (separate from Biome)
bun run check
```

> **Alt:** `alt-frontend-sv` uses `bun`, `Biome`, and `svelte-check`. Keep those as the primary local verification tools unless the service guidance changes.

## 12. Performance

### Fine-Grained Reactivity ($state proxy)

- `$state` creates a deep proxy — only the properties you read are tracked as dependencies
- This means changing `filters.severity` does not re-render components that only read `filters.activeOnly`
- No need for manual optimization like `React.memo` or selectors

### $state.raw for Large Immutable Data

- Use `$state.raw` for data that is replaced wholesale (e.g., API responses, graph models)
- Avoids proxying every property of large objects — significant performance gain for 1000+ node graphs
- Trade-off: nested mutation won't trigger reactivity — must replace the entire value

```typescript
// ✅ Large data set — replaced entirely on each fetch
let findings = $state.raw<Finding[]>([]);
findings = newFindingsFromAPI; // triggers reactivity

// ❌ Mutation will NOT trigger with $state.raw
findings.push(newFinding); // silent — no update
```

### Code Splitting

- SvelteKit automatically code-splits at route boundaries — each page loads only what it needs
- Use dynamic imports for heavy libraries to avoid blocking initial page load

```typescript
// ✅ Lazy load heavy renderer
const { ThreeSceneRenderer } = await import('./three/ThreeSceneRenderer');
```

### ResizeObserver with $effect Cleanup

- Always disconnect ResizeObservers in `$effect` cleanup to prevent memory leaks
- Batch resize handling — renderers should debounce internally if needed

```svelte
<script lang="ts">
$effect(() => {
  if (!containerEl) return;
  const observer = new ResizeObserver(() => renderer?.resize());
  observer.observe(containerEl);
  return () => observer.disconnect();
});
</script>
```

### IntersectionObserver for Infinite Scroll

- IntersectionObserver fires only on intersection **changes** — if the sentinel is still inside the viewport after a page of items loads, it never fires again and the scroll stalls (ADR-000226)
- Pattern: `unobserve` the sentinel, `await` the load callback, wait one `requestAnimationFrame` for layout to settle, then re-observe
- Reset `loading` flags in `try`/`finally` so a failed load does not leave the trigger permanently disabled
- Centralize this in one shared action — per-page mitigations (cooldowns, disabled toggles) accumulate as symptomatic fixes that all had to be torn out later (ADR-000226)

## 13. Security

### {@html} XSS Prevention

- Never use `{@html}` with unsanitized user input — it renders raw HTML
- **"Remote" counts as user input**: RSS feed content, search-hit snippets, and any HTML from an upstream API are attacker-controlled. `{@html hit.snippet}` without sanitization is XSS
- If `{@html}` is necessary, sanitize with DOMPurify — **on both server (SSR) and client**, since SvelteKit renders both
- Enable the `svelte/no-at-html-tags` lint rule; every suppression needs a comment proving the input is sanitized
- Prefer text interpolation `{value}` which auto-escapes by default

```svelte
<!-- ✅ Safe — auto-escaped -->
<p>{finding.advisory_id}</p>

<!-- ❌ Dangerous — XSS risk (also applies to RSS/API-derived HTML) -->
{@html userComment}
{@html hit.snippet}

<!-- ✅ Sanitized if {@html} is required -->
{@html DOMPurify.sanitize(markdownHtml)}
```

### Open Redirect Prevention

Login/register `redirect` / `returnTo` params must be validated with real URL parsing — string-prefix checks are bypassable (`//evil.com` is scheme-relative; `https:/evil.com` normalizes) ([OWASP](https://cheatsheetseries.owasp.org/cheatsheets/Unvalidated_Redirects_and_Forwards_Cheat_Sheet.html)):

```typescript
// ✅ resolve against own origin and require it to stay there
function safeRedirect(target: string | null, fallback = '/'): string {
  if (!target) return fallback;
  const url = new URL(target, 'http://localhost');
  if (url.origin !== 'http://localhost') return fallback; // absolute/external → reject
  return url.pathname + url.search;
}
```

### Session Cache Keys

- Key server-side session caches by a **hash of the full cookie/token** (e.g. SHA-256), never a substring/prefix — prefix collisions serve one user's session to another

### Environment Variables

- Use `$env/dynamic/public` for client-side environment variables (prefixed with `PUBLIC_`)
- Use `$env/dynamic/private` for server-only secrets (only in `+page.server.ts` / `+server.ts`)
- Never expose API keys or secrets in `PUBLIC_` variables

```typescript
// ✅ Public env — safe for client
import { env } from '$env/dynamic/public';
const gatewayUrl = env.PUBLIC_GATEWAY_URL;

// ✅ Private env — server only
import { env } from '$env/dynamic/private';
const apiKey = env.NVD_API_KEY;
```

### Input Validation

- Validate user input at system boundaries — route params, form inputs, query strings
- Use typed params from SvelteKit's `params` (already validated by route matchers)
- Never trust client-side data for security decisions

## 14. Docker & Deployment

### Multi-Stage pnpm Build

- Use `corepack` to install pnpm — do not install globally with npm
- Use `--frozen-lockfile` for reproducible builds
- Prune dev dependencies after build with `pnpm prune --prod`
- Run as a non-root user in the runtime stage

```dockerfile
# ✅ Multi-stage pnpm + adapter-node build
FROM node:20-alpine AS build
RUN corepack enable && corepack prepare pnpm@latest --activate
WORKDIR /app
COPY package.json pnpm-lock.yaml* ./
RUN pnpm install --frozen-lockfile
COPY . .
RUN pnpm run build
RUN pnpm prune --prod

FROM node:20-alpine
RUN addgroup -S decree && adduser -S decree -G decree
WORKDIR /app
COPY --from=build --chown=decree:decree /app/build ./build
COPY --from=build --chown=decree:decree /app/node_modules ./node_modules
COPY --from=build --chown=decree:decree /app/package.json ./
USER decree
EXPOSE 3400
ENV PORT=3400 HOST=0.0.0.0
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget -qO- http://127.0.0.1:3400/healthz || exit 1
CMD ["node", "build"]
```

### Health Check Endpoint

- Always expose a `/healthz` endpoint — used by Docker HEALTHCHECK and orchestrators
- Return `{ "status": "ok" }` with 200 — no database or external service checks in the health endpoint

### Deploy-Time Chunk 404 Self-Healing

- Every deploy changes `_app/immutable/*` chunk hashes; clients holding the previous HTML then 404 on dynamic imports (surfaces as "Cannot Open Page" on iOS Safari) (ADR-000898)
- Serve the SSR/app HTML with `Cache-Control: no-cache` (revalidate on every request) and keep the 1-year immutable cache on a separate location for hashed assets only — without this split, cached HTML keeps referencing stale chunk hashes after every rebuild (ADR-000412)
- Layered self-healing while the runtime is alive: detect chunk-load errors in `hooks.client` and reload once, set `version.pollInterval` in the SvelteKit config, and watch `updated.current` in `+layout.svelte` (ADR-000898)
- When the entry chunk itself 404s the runtime never boots, so runtime-level recovery cannot run: have nginx answer missing immutable assets with a 200 reload stub, add a capture-phase error listener as an inline script in `app.html`, and call `updated.check()` on BFCache restore (`pageshow`) (ADR-000902)

> **Alt:** Keep health endpoints simple and framework-native. Route, port, and adapter details should match the service's current deployment configuration rather than a copied example.

---

## References

- [Svelte 5 Documentation](https://svelte.dev/docs/svelte)
- [SvelteKit Documentation](https://svelte.dev/docs/kit)
- [Svelte 5 Runes](https://svelte.dev/docs/svelte/$state)
- [SvelteKit Load Functions](https://svelte.dev/docs/kit/load)
- [SvelteKit Page Options](https://svelte.dev/docs/kit/page-options)
- [Tailwind CSS v4](https://tailwindcss.com/docs)
- [Biome Documentation](https://biomejs.dev/)
- [Vitest Documentation](https://vitest.dev/)
- [Testing Library Svelte](https://testing-library.com/docs/svelte-testing-library/intro)
- [Three.js Documentation](https://threejs.org/docs/)
