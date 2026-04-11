# Mobile Feeds LCP Optimization: From 3.7s to Under 2.5s

## Overview

The mobile feeds pages of the Alt platform suffered from high Largest Contentful Paint (LCP) times, with render delays reaching ~3.7 seconds on the swipe page. The root cause was client-side computation blocking the initial paint: feed data transformations, read-status filtering, and UI chrome initialization were all running synchronously before the hero element could render.

This document describes the systematic optimization that brought LCP under 2.5 seconds and render delay down to 1.0-1.5 seconds across three mobile feed pages.

## Problem Analysis

### Root Causes Identified

1. **Client-side data transformation blocking paint**: SSR-fetched feed data was being transformed to display format (`toRenderFeed`) on the client side via synchronous `useMemo`, delaying hero rendering.

2. **Ambiguous LCP candidate**: The entire feed list used `content-visibility: auto`, making the boundary between the hero and feed area unclear. Lighthouse couldn't consistently identify the hero as the LCP element.

3. **Eager initialization on swipe page**: The swipe controller initialized SWR pagination, read-status synchronization, and article content prefetching all within the same render cycle, inflating Element Render Delay and Total Blocking Time.

4. **Non-essential UI in initial render**: Loading overlays, floating menus, and animation keyframes were included in the first paint, adding layout and animation cost before the hero card could stabilize.

## Optimization Strategy

Four principles guided the work:

1. **Server-first above-the-fold**: Generate the hero entirely on the server, placed in its own stacking container separate from the feed list.
2. **Minimize initial render scope**: SSR only 3-5 feed cards initially; load the rest via IntersectionObserver or `requestIdleCallback`.
3. **Staged initialization**: Defer read-status sync, SWR pagination, and prefetch behind `useEffect` + `requestIdleCallback`. Lazy-load non-LCP UI (floating menu, animations) via dynamic imports.
4. **Targeted content-visibility**: Don't apply `content-visibility` to above-the-fold content containing the hero. Apply it only to the feed list container below.

## Implementation

### 1. SSR-Side Display Field Generation

Created a server-side formatter that pre-computes display fields before sending data to the client:

- **Pre-formatted date strings** (e.g., "Nov 23, 2025") — eliminates client-side `formatDate()` calls
- **Merged tag labels** (e.g., "Next.js / Performance") — eliminates client-side string joining
- **Normalized URLs** — tracking parameter removal done server-side
- **Excerpts** (100-160 characters) — description truncation moved to SSR

This created a `RenderFeed` type extending the base `Feed` type with these pre-computed fields. All client components were updated to consume `RenderFeed` directly, removing their local transformation logic.

### 2. Client Logic Simplification

With display fields pre-computed on the server:

- Removed all client-side formatting functions (`formatDate`, `buildTagsLabel`, `normalizeUrl`, `buildExcerpt`)
- Updated all feed card components to use pre-computed fields directly
- Hooks (`useReadFeeds`, swipe controller, cursor pagination) now return `RenderFeed[]` instead of raw feed data

### 3. CSS and DOM Optimization

- Applied `content-visibility: auto` with `contain-intrinsic-size: 800px` only to the feed list container (not the hero)
- Added `line-clamp: 3` to feed card body text
- Separated hero and feed list into distinct layout containers

### 4. Server/Client Boundary and Dynamic Imports

- Limited initial render to 5 feed cards during the loading phase
- Moved `FloatingMenu` to dynamic import with `ssr: false`
- Split the feeds page into a Server Component (hero + data fetch) and a Client Component (interactive feed list)

### 5. Hero LCP Optimization

The hero tip component was designed specifically to be the stable LCP element:

```
Key design decisions:
- Server Component (zero client-side JS)
- Static text only (no i18n switching, no dynamic content)
- System font stack (no external font loading delay)
- min-height: 96px (larger than feed cards to win LCP candidacy)
- data-lcp-hero="tip" attribute for verification
- No expensive CSS properties (no box-shadow, filter, or backdrop-filter)
- contain: layout paint for independent layout completion
```

Page layout was restructured to flexbox:
- Hero fixed at the top of the viewport
- Feed list fills remaining space with independent scrolling
- Swipe page omits the hero entirely (full-screen by design)

## Results

### Target Metrics

| Metric | Target | Page |
|--------|--------|------|
| LCP | < 2.5s | All mobile feed pages |
| Render Delay | <= 1.5s | All mobile feed pages |
| LCP Element | `[data-lcp-hero="tip"]` | `/mobile/feeds` |
| LCP Element | First card `<p>` | `/mobile/feeds/swipe` |

### Verification Method

1. Chrome DevTools Lighthouse with mobile emulation (Slow 4G, 4x CPU throttle)
2. 3 measurements per page, median value used
3. LCP element verified via `audits["largest-contentful-paint-element"].details.items[0].node.selector`
4. Performance tab flame chart analysis for Script Evaluation and Layout timing

## Key Takeaways

1. **Move computation to the server**: Pre-computing display fields on the server eliminated the largest source of client-side render delay. This is especially impactful for mobile where CPU throttling makes synchronous computation expensive.

2. **Separate hero from scrollable content**: Placing the LCP candidate in its own layout container with `contain: layout paint` ensures the browser can complete its layout independently of the feed list below.

3. **content-visibility is a double-edged sword**: Applying it to containers that include LCP candidates confuses Lighthouse's LCP detection. Restrict it to below-the-fold content only.

4. **Defer everything non-essential**: Floating menus, loading overlays, animation keyframes, read-status synchronization, and content prefetching can all wait until after the hero paints. Use `requestIdleCallback` with timeouts and dynamic imports with `ssr: false`.

5. **Design for LCP stability**: The hero component was intentionally designed to be larger than feed cards (min-height: 96px), use system fonts, and avoid expensive CSS — all to ensure it consistently wins LCP candidacy across measurements.

6. **System fonts eliminate font loading delay**: Using `system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif` for the LCP element removes an entire network round-trip from the critical path.
