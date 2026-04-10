# Alt Design Language — Alt-Paper

Alt's unified design language. Newspaper-inspired editorial aesthetic applied across every surface.

> **Mobile First**: Touch interactions first, desktop hover/density second.
> **Responsibility-Driven**: Each UI's visual expression must reflect its unique purpose. Shared philosophy, unique execution.

---

## Design Philosophy

### Core Principles

1. **Editorial Clarity** — Inspired by newspaper and magazine layouts. Readability and content hierarchy above all.
2. **Responsibility-Driven Design** — Design philosophy is shared, visual patterns are NOT copied between pages. A report listing (Acolyte) earns a masthead. A knowledge consultation (Ask Augur) earns an avatar byline. Each UI must justify its own visual expression.
3. **Mobile First** — Design for mobile screens first, enhance for desktop.
4. **Scoped Styles** — Editorial styling lives in scoped `<style>` blocks with CSS custom properties, not Tailwind utility classes. Tailwind for layout only (flex, gap, padding).
5. **Component Composition** — Small, composable primitives over monolithic components.
6. **Progressive Enhancement** — Core functionality works without JavaScript; enhanced with interactivity.

### Visual Identity

Alt-Paper is the **sole** design language. There are no alternative themes.

- **Sharp edges** — No border-radius on containers, cards, inputs, buttons
- **Serif headings** — Playfair Display for all display text
- **High contrast** — Charcoal text on cream backgrounds
- **Thin structural rules** — 1-2px lines as separators, not decoration
- **Minimal shadows** — Almost none in Alt-Paper; depth through typography hierarchy
- **Monochromatic + semantic accents** — No gradients, no glass effects

### Responsibility-Driven Examples

| UI Surface | Responsibility | Visual Expression |
|------------|---------------|-------------------|
| **Acolyte** (report listing) | Publication of intelligence briefings | Masthead with double rules, date, serif title |
| **Acolyte** (report detail) | Reading an analytical report | Section tabs, editorial prose, citation footnotes |
| **Ask Augur** (empty) | Inviting a knowledge consultation | Avatar presence, italic serif prompt, centered input |
| **Ask Augur** (active) | Evidence-based Q&A thread | User question as serif heading, Augur avatar byline on answers |
| **Knowledge Home** | Daily knowledge stream | Card-based stream with why-badges, TodayBar |
| **Feeds** | RSS article browsing | Swipeable cards, touch-optimized density |

**Anti-pattern**: Copying Acolyte's masthead onto Ask Augur because "it looks editorial." The masthead is Acolyte's expression of its publishing responsibility. Ask Augur's responsibility is consultation — its expression is the avatar byline and the question-as-heading pattern.

---

## Color System

### Alt-Paper Palette

All colors defined as CSS custom properties in `src/app.css`.

#### Text Colors

| Token | Value | Usage |
|-------|-------|-------|
| `--alt-charcoal` | `#1a1a1a` | Primary text, headings, high-emphasis elements |
| `--alt-slate` | `#666666` | Secondary text, subtitles, byline names |
| `--alt-ash` | `#999999` | Tertiary text, timestamps, labels, hints |

#### Surface Colors

| Token | Value | Usage |
|-------|-------|-------|
| `--surface-bg` | `#faf9f7` | Page background, card backgrounds |
| `--surface-2` | `#f5f4f1` | Code blocks, subtle alternation |
| `--surface-border` | `#c8c8c8` | Borders, rules, separators |
| `--surface-hover` | `#f3f1ed` | Hover state background |

#### Accent Colors

| Token | Value | Usage |
|-------|-------|-------|
| `--alt-primary` | `#2f4f4f` | Links, primary actions (dark teal) |

#### Status Colors (Acolyte-specific)

| Token | Value | Semantics |
|-------|-------|-----------|
| `--alt-sage` | `#7c9070` | Success / Completed |
| `--alt-sand` | `#d4a574` | Running / In-progress |
| `--alt-terracotta` | `#b85450` | Failed / Error |

#### Semantic Tokens (OKLCH, Tailwind v4)

```css
--background: oklch(1 0 0)              /* White */
--foreground: oklch(0.147 0.004 49.25)  /* Charcoal */
--primary: oklch(0.216 0.006 56.043)    /* Dark teal */
--border: oklch(0.923 0.003 48.717)     /* Light gray */
```

### Shadow System

Shadows are minimal in Alt-Paper. Use typography hierarchy for depth, not drop shadows.

```css
--shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.05)   /* Rare — only on floating elements */
--shadow-md: 0 2px 4px rgba(0, 0, 0, 0.08)    /* FloatingMenu only */
```

---

## Typography

### Three-Font Architecture

| Role | Family | Weights | Usage |
|------|--------|---------|-------|
| **Display** | `Playfair Display`, Georgia, serif | 400, 600, 700, 800, italic | Headings, titles, question text, mastheads |
| **Body** | `Source Sans 3`, system-ui, sans-serif | 300, 400, 500, 600, 700 | Body text, labels, UI text, buttons |
| **Mono** | `IBM Plex Mono`, Source Code Pro, monospace | 400, 500, 600 | Timestamps, version numbers, citation IDs, code |

Loaded via Google Fonts in `src/app.html`. Applied globally in `src/app.css`:
```css
html, body { font-family: var(--font-body); }
h1, h2, h3, h4, h5, h6 { font-family: var(--font-display); }
code, kbd, samp, pre { font-family: var(--font-mono); }
```

### Editorial Typography Patterns

#### Labels (metadata, categories)
```css
font-family: var(--font-body);
font-size: 0.65rem; font-weight: 600;
letter-spacing: 0.08em; text-transform: uppercase;
color: var(--alt-ash);
```

#### Prose Body (article text, answers)
```css
font-family: var(--font-body);
font-size: 0.95rem; line-height: 1.72;
color: var(--alt-charcoal);
max-width: 65ch;
```

#### Serif Heading (questions, section titles)
```css
font-family: var(--font-display);
font-size: 1.15rem; font-weight: 700; line-height: 1.3;
color: var(--alt-charcoal);
```

#### Masthead Title (Acolyte only)
```css
font-family: var(--font-display);
font-size: clamp(2rem, 5vw, 3rem); font-weight: 900;
letter-spacing: -0.02em; line-height: 1.1;
color: var(--alt-charcoal);
```

#### Monospace Metadata
```css
font-family: var(--font-mono);
font-size: 0.65rem; color: var(--alt-ash);
```

### iOS Mobile Constraint

**Input/textarea font-size must be >= 16px (1rem) on mobile.** iOS Safari auto-zooms when focusing on fields with smaller text. Always use `font-size: 1rem` for mobile input fields.

---

## Editorial Patterns

### Thin Rules

Structural separators, not decoration. Always 1px `--surface-border` unless a heavier rule (2px `--alt-charcoal`) serves a specific framing purpose (e.g., Acolyte masthead).

```css
/* Standard separator between content blocks */
.entry-rule { height: 1px; background: var(--surface-border); }

/* Heavy frame rule (Acolyte masthead only) */
.masthead-rule { height: 2px; background: var(--alt-charcoal); }
```

### Avatar Byline (Ask Augur)

Columnist-style attribution on Augur answers. Square, not circular — editorial, not chat.

```css
.entry-byline { display: flex; align-items: center; gap: 0.4rem; margin-bottom: 0.4rem; }
.byline-avatar { width: 24px; height: 24px; object-fit: cover; border: 1px solid var(--surface-border); }
.byline-name { font-size: 0.65rem; font-weight: 600; letter-spacing: 0.08em; text-transform: uppercase; color: var(--alt-ash); }
```

Mobile: 20px avatar. Desktop: 24px.

### Citation Footnotes

Numbered sources with monospace IDs and linked titles.

```css
.sources-heading { font-size: 0.6rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; color: var(--alt-ash); }
.source-id { font-family: var(--font-mono); font-size: 0.65rem; font-weight: 600; color: var(--alt-charcoal); }
.source-title { color: var(--alt-primary); text-decoration: underline; text-underline-offset: 2px; }
```

### Status Stripe (Acolyte cards)

3px left border indicating report status. Color from status tokens.

```css
.card-stripe { width: 3px; flex-shrink: 0; }
.card-stripe--succeeded { background: var(--alt-sage); }
.card-stripe--running { background: var(--alt-sand); }
.card-stripe--failed { background: var(--alt-terracotta); }
```

### Loading States

Single pulsing dot + italic stage text. Never bouncing dots or spinners.

```css
.loading-pulse { width: 8px; height: 8px; border-radius: 50%; background: var(--alt-ash); animation: pulse 1.2s ease-in-out infinite; }
@keyframes pulse { 0%, 100% { opacity: 0.3; } 50% { opacity: 1; } }
```

### Page Reveal Animation

Staggered entrance for content lists.

```css
/* Page container */
.page { opacity: 0; transform: translateY(6px); transition: opacity 0.4s ease, transform 0.4s ease; }
.page.revealed { opacity: 1; transform: translateY(0); }

/* List items */
.item { opacity: 0; animation: entry-in 0.3s ease forwards; animation-delay: calc(var(--stagger) * 60ms); }
@keyframes entry-in { to { opacity: 1; } }
```

Desktop: 60ms stagger. Mobile: 40ms stagger.

---

## CSS Convention

### Scoped Styles over Tailwind

All editorial visual styling belongs in scoped `<style>` blocks referencing CSS custom properties. Tailwind is used only for structural layout (flex, grid, gap, padding).

**Correct (editorial in scoped styles):**
```svelte
<h2 class="section-title">{title}</h2>

<style>
  .section-title {
    font-family: var(--font-display);
    font-size: 1.15rem; font-weight: 700;
    color: var(--alt-charcoal);
  }
</style>
```

**Incorrect (editorial in Tailwind):**
```svelte
<h2 class="font-serif text-lg font-bold text-gray-900">{title}</h2>
```

This convention ensures:
- Design tokens are the single source of truth
- Editorial aesthetics are self-contained and maintainable
- Theme changes propagate through CSS variables without touching markup

### `data-role` Attributes for Test Stability

Interactive content entries use `data-role` for E2E test selectors. Never use class-based selectors (`[class*="bg-primary"]`) — they break on style changes.

```svelte
<article class="thread-entry" data-role={role}>
```

---

## Components

### Button

Sharp-edged with charcoal border. Hover inverts to charcoal background.

```css
border: 1.5px solid var(--alt-charcoal);
background: transparent; color: var(--alt-charcoal);
font-size: 0.75rem; font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase;
min-height: 44px; /* touch target */
```

Hover: `background: var(--alt-charcoal); color: var(--surface-bg);`

### Card

No shadows. Thin border. No rounded corners.

```css
border: 1px solid var(--surface-border);
background: var(--surface-bg);
border-radius: 0;
```

### Input / Textarea

Sharp-edged. Focus state changes border to charcoal.

```css
border: 1px solid var(--surface-border); border-radius: 0;
background: transparent; font-size: 1rem; /* >= 16px for iOS */
min-height: 44px;
```

Focus: `border-color: var(--alt-charcoal); outline: none;`

---

## Layout

### Mobile-First Architecture

Separate component trees for mobile and desktop:

```
src/lib/components/
├── mobile/           # Touch-optimized (primary)
├── desktop/          # Pointer-optimized (enhanced)
└── ui/               # Shared primitives
```

Breakpoint: **768px** (`md:`). Detection via `useViewport()` hook.

### Mobile Layout

```
.augur-mobile (flex column, height: 100%)
  ├── .content-area (flex: 1, overflow-y: auto)
  └── .input-area (flex-shrink: 0)
```

- Use **flexbox layout** for full-height pages (thread + input)
- **Never** use `position: fixed` on `html`/`body` — it breaks layout on iOS Safari
- Use `touchmove` event prevention for iOS bounce control
- `height: 100dvh` on shell (dynamic viewport height for iOS)
- `env(safe-area-inset-top)` / `env(safe-area-inset-bottom)` for notch/home indicator
- Bottom input: `flex-shrink: 0`, NOT `position: fixed/absolute`

### Desktop Layout

Sidebar (240px) + main content. Max-width constraints per page type:
- Report/consultation pages: `max-width: 720px`
- Detail pages with sidebar: `max-width: 1080px`
- Dashboard/admin: `max-width: 1400px`

---

## Touch & Accessibility

### Touch Targets

Minimum **44x44px** on all interactive mobile elements (Apple HIG).

### Safe Area

```css
padding-top: calc(0.5rem + env(safe-area-inset-top, 0px));
padding-bottom: calc(0.75rem + env(safe-area-inset-bottom, 0px));
```

### Reduced Motion

```css
@media (prefers-reduced-motion: reduce) {
  .animated { animation: none; opacity: 1; }
  .page { transition: none; opacity: 1; transform: none; }
}
```

### Color Contrast (WCAG AA)

- `--alt-charcoal` (#1a1a1a) on `--surface-bg` (#faf9f7): **16.1:1**
- `--alt-slate` (#666666) on `--surface-bg` (#faf9f7): **5.7:1**
- `--alt-ash` (#999999) on `--surface-bg` (#faf9f7): **3.0:1** (decorative only)

### Semantic HTML

- `<article>` for thread entries
- `<footer>` for citation sections
- `<nav>` for section tabs and navigation
- `<h3>` for user questions (heading hierarchy)
- `aria-label` on icon-only buttons

---

## iOS Safari Considerations

Lessons from production. These are mandatory, not optional.

| Issue | Fix |
|-------|-----|
| Auto-zoom on input focus | `font-size >= 1rem` (16px) on all mobile inputs |
| Elastic bounce scroll | `touchmove` prevention on non-scroll areas, NOT `position: fixed` on body |
| `100vh` includes URL bar | Use `100dvh` (dynamic viewport height) |
| Safe area clipping | `env(safe-area-inset-top)` padding on scroll containers |
| Nested fixed positioning | Never nest `position: fixed` inside `position: fixed` — use flexbox |
| `overscroll-behavior` | Not reliable on iOS Safari alone — combine with JS `touchmove` |

---

## Animation Vocabulary

All motion is **editorial** — deliberate, restrained, purposeful. No playful bouncing.

| Pattern | Duration | Use |
|---------|----------|-----|
| Page reveal | 0.4s ease | Container fade-in + translateY |
| Entry stagger | 0.3s ease, 40-60ms delay | List items appearing |
| Pulse indicator | 1.2s ease-in-out infinite | Loading dot |
| Color transition | 0.15-0.2s | Hover/focus state changes |
| Border transition | 0.15s | Input focus |

---

## File References

| File | Purpose |
|------|---------|
| `src/app.css` | All design tokens, CSS variables, Tailwind v4 config |
| `src/app.html` | Google Fonts import (Playfair Display, Source Sans 3, IBM Plex Mono) |
| `src/lib/components/ui/` | Shared primitives (button, card, input, sheet) |
| `src/lib/components/mobile/` | Mobile components (105+) |
| `src/lib/components/desktop/` | Desktop components (80+) |
| `src/lib/stores/viewport.svelte.ts` | `useViewport()` — 768px breakpoint detection |
| `src/lib/utils.ts` | `cn()` — clsx + tailwind-merge |

---

## Version

- **SvelteKit**: 2.x
- **Svelte**: 5 (Runes — `$state`, `$derived`, `$effect`, `$props`)
- **TailwindCSS**: v4 (CSS-first, no tailwind.config.js)
- **TypeScript**: 7 (tsgo)
- **UI Primitives**: bits-ui / shadcn-svelte
- **Icons**: @lucide/svelte
