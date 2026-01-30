# Alt Design Language

A comprehensive design system documentation for the Alt frontend application.

> **Mobile First**: This design system prioritizes mobile experiences. Components are designed for touch interactions first, then enhanced for desktop with hover states and dense layouts.

## Table of Contents

1. [Design Philosophy](#design-philosophy)
   - [Mobile-First Architecture](#mobile-first-architecture)
2. [Color System](#color-system)
3. [Typography](#typography)
4. [Spacing](#spacing)
   - [Touch Target Guidelines](#touch-target-guidelines)
5. [Components](#components)
6. [Animation & Motion](#animation--motion)
   - [Mobile Touch Feedback](#mobile-touch-feedback)
7. [Iconography](#iconography)
8. [Layout Patterns](#layout-patterns)
   - [Mobile Layout](#mobile-layout-primary)
   - [Mobile Gestures](#mobile-gestures)
   - [Desktop Layout](#desktop-layout-enhanced)
9. [Accessibility](#accessibility)
   - [Mobile Accessibility](#mobile-accessibility)

---

## Design Philosophy

### Core Principles

1. **Mobile First** - Design and develop for mobile screens first, then enhance for larger viewports
2. **Editorial Clarity** - Inspired by newspaper and magazine layouts, prioritizing readability and content hierarchy
3. **Themeable Architecture** - CSS custom properties enable runtime theme switching without rebuilds
4. **Utility-First** - TailwindCSS v4 with semantic CSS variables for maintainability
5. **Component Composition** - Small, composable primitives over monolithic components
6. **Progressive Enhancement** - Core functionality works without JavaScript; enhanced with interactivity

### Mobile-First Architecture

The codebase maintains **separate component trees** for mobile and desktop experiences:

```
src/lib/components/
├── mobile/           # Mobile-optimized components (primary)
│   ├── feeds/
│   │   └── swipe/    # Gesture-based interactions
│   ├── recap/
│   ├── search/
│   └── morning-letter/
├── desktop/          # Desktop-enhanced components
│   ├── layout/
│   ├── feeds/
│   ├── recap/
│   └── augur/
└── ui/               # Shared primitives
```

This separation allows:
- **Touch-optimized interactions** on mobile (swipe gestures, larger tap targets)
- **Pointer-optimized interactions** on desktop (hover states, dense layouts)
- **Shared design tokens** across both experiences

### Visual Identity

The default "Alt-Paper" theme embodies a **clean editorial aesthetic**:
- Sharp edges (no border-radius)
- High contrast typography
- Minimal shadows
- Monochromatic palette with accent colors

---

## Color System

### Architecture

Colors are defined using CSS custom properties with three distinct theme variants, selectable via `data-style` attribute.

### Semantic Tokens (OKLCH Color Space)

```css
/* Light Mode */
--background: oklch(1 0 0)              /* Pure white */
--foreground: oklch(0.147 0.004 49.25)  /* Deep charcoal */
--primary: oklch(0.216 0.006 56.043)    /* Dark teal */
--destructive: oklch(0.577 0.245 27.325) /* Red */
--border: oklch(0.923 0.003 48.717)     /* Light gray */

/* Dark Mode (.dark class) */
--background: oklch(0.147 0.004 49.25)  /* Dark background */
--foreground: oklch(0.985 0.001 106.423) /* Bright white */
--border: oklch(1 0 0 / 10%)            /* Subtle borders */
```

### Theme Variants

#### 1. Alt-Paper (Default)

The newspaper-inspired theme with high contrast and sharp edges.

| Token | Value | Usage |
|-------|-------|-------|
| `--alt-primary` | `#2f4f4f` | Primary actions, links |
| `--alt-secondary` | `#696969` | Secondary elements |
| `--alt-tertiary` | `#808080` | Tertiary accents |
| `--surface-bg` | `#dedede` | Card backgrounds |
| `--surface-border` | `#e0e0e0` | Borders |
| `--surface-hover` | `#f8f8f8` | Hover states |
| `--text-primary` | `#1a1a1a` | Headings, body text |
| `--text-secondary` | `#333333` | Subtitles |
| `--text-muted` | `#666666` | Captions, hints |

**Characteristics:**
- No border-radius (`--surface-blur: 0px`)
- Minimal shadows
- High contrast text

#### 2. Vaporwave

Neon cyberpunk aesthetic with glassmorphism effects.

| Token | Value | Usage |
|-------|-------|-------|
| `--alt-primary` | `#ff006e` | Hot pink |
| `--alt-secondary` | `#8338ec` | Vibrant purple |
| `--alt-tertiary` | `#3a86ff` | Bright cyan-blue |
| `--surface-bg` | `rgba(255,255,255,0.1)` | Glass surfaces |
| `--surface-blur` | `20px` | Blur intensity |

**App Background:**
```css
linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f0f23 100%)
```

#### 3. Liquid-Beige

Earthy luxury aesthetic with warm tones.

| Token | Value | Usage |
|-------|-------|-------|
| `--alt-primary` | `#b85450` | Terracotta |
| `--alt-secondary` | `#7c9070` | Sage green |
| `--alt-tertiary` | `#d4a574` | Sandy beige |
| `--surface-bg` | `rgba(255,253,250,0.1)` | Warm glass |
| `--surface-blur` | `12px` | Softer blur |

**App Background:**
```css
linear-gradient(135deg, #e8ded1 0%, #dfd4c5 25%, #d6c8b9 50%, #cdbaa8 75%, #c8b8a1 100%)
```

### Status Colors

| Token | Value | Usage |
|-------|-------|-------|
| `--alt-success` | `#00ff00` | Success states |
| `--alt-error` | `#ff0000` | Error states |
| `--alt-warning` | `#ffff00` | Warning states |
| `--destructive` | `#dc2626` | Destructive actions |

### Shadow System

```css
--shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.05)
--shadow-md: 0 2px 4px rgba(0, 0, 0, 0.08)
--shadow-lg: 0 4px 8px rgba(0, 0, 0, 0.12)
```

### Theme Switching

```html
<!-- Apply theme via data attribute -->
<html data-style="alt-paper">  <!-- Default -->
<html data-style="vaporwave">  <!-- Neon theme -->
<html data-style="liquid-beige"> <!-- Earthy theme -->
```

---

## Typography

### Font Stack

System fonts for optimal performance:
```css
font-family: ui-sans-serif, system-ui, sans-serif, "Apple Color Emoji", "Segoe UI Emoji";
```

### Scale

| Class | Size | Usage |
|-------|------|-------|
| `text-xs` | 12px / 0.75rem | Captions, badges |
| `text-sm` | 14px / 0.875rem | Secondary text, labels |
| `text-base` | 16px / 1rem | Body text |
| `text-lg` | 18px / 1.125rem | Subheadings |
| `text-xl` | 20px / 1.25rem | Section titles |
| `text-2xl` | 24px / 1.5rem | Page headings |
| `text-3xl` | 30px / 1.875rem | Hero text |

### Weights

| Class | Weight | Usage |
|-------|--------|-------|
| `font-normal` | 400 | Body text |
| `font-medium` | 500 | Emphasized text |
| `font-semibold` | 600 | Subheadings |
| `font-bold` | 700 | Headings, buttons |

### Letter Spacing

| Class | Value | Usage |
|-------|-------|-------|
| `tracking-wider` | 0.05em | Uppercase labels |
| `tracking-widest` | 0.1em | Category headers |
| `tracking-[0.08em]` | 0.08em | UI labels |

### Line Height

| Class | Value | Usage |
|-------|-------|-------|
| `leading-normal` | 1.5 | Default |
| `leading-relaxed` | 1.625 | Long-form content |
| `leading-tight` | 1.25 | Headings |

### Text Colors

```css
color: var(--text-primary)   /* #1a1a1a - Headings, body */
color: var(--text-secondary) /* #333333 - Subtitles */
color: var(--text-muted)     /* #666666 - Captions, hints */
```

---

## Spacing

### Base Scale (Tailwind Default)

| Token | Value | Usage |
|-------|-------|-------|
| `1` | 4px / 0.25rem | Micro spacing |
| `2` | 8px / 0.5rem | Tight spacing |
| `3` | 12px / 0.75rem | Compact spacing |
| `4` | 16px / 1rem | Default spacing |
| `6` | 24px / 1.5rem | Section spacing |
| `8` | 32px / 2rem | Large spacing |

### Component Spacing Patterns

**Cards:**
```css
/* Mobile */
padding: 16px (p-4)
gap: 12px (gap-3)

/* Desktop */
padding: 16px-24px (p-4 md:p-6)
gap: 16px (gap-4)
```

**Buttons:**
```css
/* Mobile - larger touch targets */
height: 44px (h-11)         /* Minimum touch target */
padding: 16px horizontal (px-4)

/* Desktop */
height: 36px (h-9)
padding: 16px horizontal (px-4)

/* Small (desktop only) */
height: 32px (h-8)
padding: 12px horizontal (px-3)
```

**Mobile Layout:**
```css
/* Full-width with safe padding */
padding: 16px (p-4)
padding-bottom: 80px (pb-20)  /* Space for bottom nav */

/* Bottom navigation */
height: 64px (h-16)
position: fixed
bottom: 0
```

**Desktop Layout:**
```css
/* Container */
max-width: 1400px
padding: 32px (2rem)

/* Sidebar */
width: 240px (w-60)
padding: 24px (p-6)

/* Main content offset */
margin-left: 240px (ml-60)
```

### Touch Target Guidelines

Per Apple HIG and Material Design guidelines:

| Element | Minimum Size | Recommended |
|---------|--------------|-------------|
| Buttons | 44x44px (h-11 w-11) | 48x48px |
| Icons (tappable) | 44x44px | 48x48px |
| List items | 44px height | 48-56px |
| Form inputs | 44px height | 48px |

```svelte
<!-- Touch-friendly icon button -->
<button class="h-11 w-11 flex items-center justify-center">
  <Icon class="h-5 w-5" />
</button>
```

---

## Components

### Button

#### Variants

| Variant | Description |
|---------|-------------|
| `default` | Primary action - solid border, inverted on hover |
| `destructive` | Dangerous action - red themed |
| `outline` | Secondary action - subtle border |
| `secondary` | Tertiary action |
| `ghost` | Minimal - transparent background |
| `link` | Text link with underline |

#### Sizes

| Size | Dimensions |
|------|------------|
| `default` | h-9 (36px), px-4 |
| `sm` | h-8 (32px), px-3, text-sm |
| `lg` | h-10 (40px), px-6, text-lg |
| `icon` | 36x36px square |
| `icon-sm` | 32x32px square |
| `icon-lg` | 40x40px square |

#### Usage

```svelte
<Button variant="default">Primary Action</Button>
<Button variant="outline" size="sm">Secondary</Button>
<Button variant="ghost" size="icon"><Icon /></Button>
```

### Card

Sharp-edged container with subtle shadow.

```svelte
<Card>
  <CardHeader>
    <CardTitle>Title</CardTitle>
    <CardDescription>Description</CardDescription>
  </CardHeader>
  <CardContent>Content</CardContent>
  <CardFooter>Actions</CardFooter>
</Card>
```

**Base Styles:**
```css
border-radius: 0
border: 2px solid var(--surface-border)
background: var(--surface-bg)
box-shadow: var(--shadow-sm)
```

### Input

Rounded text input with focus ring.

```css
height: 36px (h-9)
border-radius: 12px
border: 2px solid var(--surface-border)
background: var(--surface-bg)

/* Focus state */
border-color: var(--alt-primary)
box-shadow: var(--shadow-sm)
```

### Status Badge

Dynamic status indicators with color coding.

| Status | Colors |
|--------|--------|
| `pending` | Gray background, gray text |
| `running` | Blue background, blue text, pulse animation |
| `completed` | Green background, green text |
| `failed` | Red background, red text |

### Glass Container

Glassmorphism effect for themed variants.

```css
.glass {
  background: var(--alt-glass);
  border: 1px solid var(--alt-glass-border);
  backdrop-filter: blur(var(--alt-glass-blur)) saturate(120%);
  box-shadow: 0 4px 6px var(--alt-glass-shadow);
}
```

---

## Animation & Motion

### Transitions

| Pattern | Duration | Easing |
|---------|----------|--------|
| Color changes | 200ms | ease |
| Transform | 200-300ms | ease-in-out |
| Layout | 300ms | ease |

### Mobile Touch Feedback

Touch interactions require immediate visual feedback:

**Tap feedback:**
```css
/* Active state for touch */
&:active {
  transform: scale(0.98);
  opacity: 0.9;
}
```

**Swipe with Spring Physics:**
```svelte
import { spring } from 'svelte/motion';

const coords = spring({ x: 0, y: 0 }, {
  stiffness: 0.2,
  damping: 0.4
});

<!-- Card follows finger with physics-based motion -->
<div style="transform: translateX({$coords.x}px)">
```

**Gesture Thresholds:**
```typescript
const SWIPE_THRESHOLD = 60;  // pixels to trigger action
const VELOCITY_THRESHOLD = 0.5;  // speed-based triggering
```

### Desktop Patterns

**Hover elevation:**
```css
transition: all 200ms ease;
&:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
}
```

**Button press:**
```css
&:hover { transform: scale(1.05); }
&:active { transform: scale(0.95); }
```

**Chevron rotation:**
```css
transition: transform 200ms ease;
&.expanded { transform: rotate(180deg); }
```

### Common Patterns

**Loading spinner:**
```css
animation: spin 1s linear infinite;
```

**Pulse (running state):**
```css
animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
```

**Typewriter effect (AI responses):**
```typescript
// Progressive text reveal for streaming content
function typewriter(text: string, speed: number = 30) {
  let index = 0;
  const interval = setInterval(() => {
    displayText = text.slice(0, ++index);
    if (index >= text.length) clearInterval(interval);
  }, speed);
}
```

### Svelte Transitions

```svelte
import { fade, slide, fly } from 'svelte/transition';

<!-- Fade for modals/overlays -->
<div transition:fade={{ duration: 200 }}>

<!-- Slide for drawers/sheets -->
<div transition:slide={{ duration: 300 }}>

<!-- Fly for mobile cards entering -->
<div in:fly={{ y: 50, duration: 300 }}>
```

---

## Iconography

### Library

**Lucide Svelte** (`@lucide/svelte`)

### Sizing

| Size | Class | Usage |
|------|-------|-------|
| XS | `h-3 w-3` | Inline indicators |
| SM | `h-3.5 w-3.5` | Child menu items |
| MD | `h-4 w-4` | Default icons |
| LG | `h-5 w-5` | Emphasis |

### Common Icons

| Icon | Usage |
|------|-------|
| `Home` | Dashboard |
| `Rss` | Feeds |
| `Eye` | Read/viewed |
| `Star` | Favorites |
| `Search` | Search |
| `CalendarRange` | Recap/timeline |
| `Settings` | Settings |
| `ChevronDown` | Expandable sections |
| `Loader2` | Loading (with `animate-spin`) |
| `Check` | Completed status |
| `Circle` | Pending status |
| `ExternalLink` | External links |

### Coloring

```svelte
<Icon class="h-4 w-4" style="color: var(--alt-primary);" />
<Icon class="h-4 w-4 text-[var(--text-muted)]" />
```

---

## Layout Patterns

### Mobile Layout (Primary)

Full-width, gesture-driven interface optimized for touch.

```
+---------------------------+
|  Header (sticky)          |
+---------------------------+
|                           |
|  Full-width Content       |
|  - Swipeable cards        |
|  - Touch-friendly spacing |
|                           |
+---------------------------+
|  Bottom Navigation        |
+---------------------------+
```

```svelte
<div class="min-h-screen bg-[var(--surface-bg)]">
  <header class="sticky top-0 z-50 p-4">
    <!-- Logo, search, menu -->
  </header>
  <main class="px-4 pb-20">
    <slot />
  </main>
  <nav class="fixed bottom-0 left-0 right-0 h-16 border-t">
    <!-- Bottom navigation -->
  </nav>
</div>
```

**Mobile Design Guidelines:**
- **Touch targets**: Minimum 44x44px (h-11 w-11)
- **Spacing**: Generous padding (p-4, gap-4) for finger-friendly interactions
- **Full-width cards**: Cards span 100% width for easy tapping
- **Bottom navigation**: Primary actions within thumb reach
- **Sticky headers**: Persistent access to navigation and search

### Mobile Gestures

The mobile experience leverages native gesture patterns:

**Swipe Cards (`SwipeFeedCard`):**
```svelte
<!-- Swipe left/right to dismiss, with spring animation -->
<div
  use:swipe={{ threshold: 60 }}
  style="transform: translateX({$springX}px) rotate({rotation}deg)"
>
  <FeedCard />
</div>
```

| Gesture | Action |
|---------|--------|
| Swipe left | Dismiss / Mark as read |
| Swipe right | Save / Favorite |
| Tap | Expand details |
| Long press | Context menu |

**Touch Action CSS:**
```css
/* Prevent scroll during horizontal swipe */
touch-action: pan-y;

/* Disable all touch actions during drag */
touch-action: none;
```

### Mobile Components

| Component | Purpose |
|-----------|---------|
| `SwipeFeedCard` | Gesture-based feed item (555 LOC) |
| `VirtualFeedList` | Virtualized scrolling for performance |
| `FeedCard` | Touch-friendly feed display |
| `MobileRecapCard` | Summary card for mobile |
| `MobileSearchInput` | Full-width search with suggestions |

### Desktop Layout (Enhanced)

Sidebar navigation with fixed positioning for larger screens.

```
+-----------------------------------------------------+
| Sidebar (fixed, 240px)  |  Main Content (flex-1)    |
|                         |                           |
| - Logo                  |  - Page Header            |
| - Navigation            |  - Content Grid           |
|   - Collapsible groups  |                           |
|                         |                           |
+-----------------------------------------------------+
```

```svelte
<div class="flex min-h-screen bg-[var(--surface-bg)]">
  <Sidebar /> <!-- fixed w-60 -->
  <main class="flex-1 ml-60 p-6">
    <slot />
  </main>
</div>
```

**Desktop Enhancements:**
- **Hover states**: Visual feedback on pointer interaction
- **Dense layouts**: More information per viewport
- **Keyboard navigation**: Full keyboard accessibility
- **Multi-column grids**: Efficient use of horizontal space

### Responsive Breakpoints

Following Tailwind's mobile-first approach:

| Breakpoint | Min Width | Usage |
|------------|-----------|-------|
| (default) | 0px | Mobile phones |
| `sm` | 640px | Large phones, small tablets |
| `md` | 768px | Tablets |
| `lg` | 1024px | Laptops, small desktops |
| `xl` | 1280px | Desktops |
| `2xl` | 1536px | Large desktops |

**Example - Mobile-First Grid:**
```css
/* Mobile: single column */
grid-cols-1

/* Tablet: two columns */
md:grid-cols-2

/* Desktop: three columns */
lg:grid-cols-3
```

### Grid Patterns

**Feed Grid (Responsive):**
```svelte
<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
  {#each feeds as feed}
    <FeedCard {feed} />
  {/each}
</div>
```

**Mobile-First Spacing:**
```css
/* Mobile */
padding: 16px (p-4)
gap: 16px (gap-4)

/* Desktop */
md:padding: 24px (md:p-6)
md:gap: 24px (md:gap-6)
```

### Container

```css
/* Mobile: full-width with padding */
width: 100%
padding: 16px (p-4)

/* Desktop: constrained width */
max-width: 1400px (max-w-2xl)
margin: 0 auto
padding: 32px (p-8)
```

---

## Accessibility

### Mobile Accessibility

**Touch Target Sizing:**
```css
/* Minimum 44x44px for all interactive elements */
.touch-target {
  min-height: 44px;
  min-width: 44px;
}
```

**Gesture Alternatives:**
```svelte
<!-- Swipe actions must have button alternatives -->
<div class="swipeable-card">
  <slot />
  <!-- Visible action buttons for accessibility -->
  <div class="flex gap-2 mt-2">
    <button aria-label="Mark as read">Read</button>
    <button aria-label="Save for later">Save</button>
  </div>
</div>
```

**Safe Area Insets (iOS):**
```css
/* Respect notch and home indicator */
padding-bottom: env(safe-area-inset-bottom);
padding-top: env(safe-area-inset-top);
```

### Focus States

```css
/* Visible focus for keyboard navigation */
focus-visible:outline-none
focus-visible:border-[var(--alt-primary)]
focus-visible:shadow-[var(--shadow-sm)]

/* Mobile: larger focus rings */
focus-visible:ring-2
focus-visible:ring-offset-2
```

### ARIA Patterns

```svelte
<!-- Buttons with icons -->
<button aria-label="Open feed details">
  <ExternalLink class="h-4 w-4" />
</button>

<!-- Loading states -->
<div aria-busy={isLoading} role="status">

<!-- Disabled links -->
<a aria-disabled={disabled} tabindex={disabled ? -1 : 0}>

<!-- Swipeable content -->
<div role="article" aria-label="Feed item: {title}">
```

### Semantic HTML

- Use `<button>` for actions, `<a>` for navigation
- Use `<nav>` for navigation sections
- Use `<main>` for primary content
- Use `<aside>` for sidebars

### Color Contrast

Alt-Paper theme maintains WCAG AA compliance:
- Text primary (#1a1a1a) on surface (#dedede): 10.5:1
- Text secondary (#333333) on surface (#dedede): 7.2:1
- Text muted (#666666) on surface (#dedede): 4.5:1

### Motion Preferences

```css
@media (prefers-reduced-motion: reduce) {
  * {
    animation-duration: 0.01ms !important;
    transition-duration: 0.01ms !important;
  }

  /* Disable swipe animations */
  .swipeable-card {
    transition: none !important;
  }
}
```

---

## Code Examples

### Mobile Swipe Card

```svelte
<script lang="ts">
  import { spring } from "svelte/motion";
  import { cn } from "$lib/utils";

  const SWIPE_THRESHOLD = 60;

  let startX = $state(0);
  let currentX = $state(0);
  let isDragging = $state(false);

  const springX = spring(0, { stiffness: 0.2, damping: 0.4 });

  function handleTouchStart(e: TouchEvent) {
    startX = e.touches[0].clientX;
    isDragging = true;
  }

  function handleTouchMove(e: TouchEvent) {
    if (!isDragging) return;
    currentX = e.touches[0].clientX - startX;
    springX.set(currentX, { hard: true });
  }

  function handleTouchEnd() {
    isDragging = false;
    if (Math.abs(currentX) > SWIPE_THRESHOLD) {
      // Trigger swipe action
      onSwipe(currentX > 0 ? "right" : "left");
    }
    springX.set(0);
  }
</script>

<div
  class="touch-pan-y"
  ontouchstart={handleTouchStart}
  ontouchmove={handleTouchMove}
  ontouchend={handleTouchEnd}
  style="transform: translateX({$springX}px)"
>
  <slot />
</div>
```

### Responsive Card

```svelte
<button
  type="button"
  class={cn(
    "w-full text-left border-2",
    "border-[var(--surface-border)] bg-white",
    "transition-all duration-200",
    /* Mobile: larger padding, touch feedback */
    "p-4 active:scale-[0.98] active:opacity-90",
    /* Desktop: hover effects */
    "md:p-4 md:hover:shadow-md md:hover:-translate-y-1",
    "cursor-pointer group"
  )}
>
  <h3 class={cn(
    "font-semibold text-[var(--text-primary)]",
    /* Mobile: larger text */
    "text-base",
    /* Desktop: smaller, with hover color */
    "md:text-sm md:group-hover:text-[var(--accent-primary)]",
    "transition-colors"
  )}>
    {title}
  </h3>
  <p class="text-sm md:text-xs text-[var(--text-secondary)] line-clamp-2">
    {description}
  </p>
</button>
```

### Themed Component

```svelte
<script lang="ts">
  import { cn } from "$lib/utils";

  interface Props {
    variant?: "primary" | "secondary";
  }

  let { variant = "primary" }: Props = $props();
</script>

<div
  class={cn(
    "p-4 border-2 transition-all duration-200",
    variant === "primary"
      ? "border-[var(--alt-primary)] bg-[var(--surface-bg)]"
      : "border-[var(--surface-border)] bg-[var(--surface-hover)]"
  )}
  style="color: var(--text-primary);"
>
  <slot />
</div>
```

### Status Indicator

```svelte
<script lang="ts">
  import { Check, Circle, Loader2 } from "@lucide/svelte";

  let { status }: { status: "pending" | "running" | "completed" } = $props();
</script>

<div class={cn(
  "rounded-full flex items-center justify-center",
  /* Mobile: larger touch target */
  "w-10 h-10 md:w-8 md:h-8",
  status === "completed" && "bg-green-100 text-green-700 border-2 border-green-500",
  status === "running" && "bg-blue-100 text-blue-700 border-2 border-blue-500",
  status === "pending" && "bg-gray-100 text-gray-500 border-2 border-gray-300"
)}>
  {#if status === "completed"}
    <Check class="w-5 h-5 md:w-4 md:h-4" />
  {:else if status === "running"}
    <Loader2 class="w-5 h-5 md:w-4 md:h-4 animate-spin" />
  {:else}
    <Circle class="w-5 h-5 md:w-4 md:h-4" />
  {/if}
</div>
```

### Mobile Bottom Navigation

```svelte
<nav class={cn(
  "fixed bottom-0 left-0 right-0",
  "h-16 px-4",
  "bg-[var(--surface-bg)] border-t border-[var(--surface-border)]",
  "flex items-center justify-around",
  "safe-area-pb" /* iOS safe area */
)}>
  {#each navItems as item}
    <a
      href={item.href}
      class={cn(
        "flex flex-col items-center justify-center",
        "h-11 w-11", /* Touch target */
        "text-[var(--text-muted)]",
        item.active && "text-[var(--alt-primary)]"
      )}
    >
      <item.icon class="h-5 w-5" />
      <span class="text-xs mt-1">{item.label}</span>
    </a>
  {/each}
</nav>
```

---

## File References

| File | Purpose |
|------|---------|
| `src/app.css` | Theme definitions, CSS variables |
| `tailwind.config.ts` | Tailwind extensions |
| `src/lib/components/ui/` | Base UI components |
| `src/lib/components/mobile/` | Mobile-specific components |
| `src/lib/components/desktop/` | Desktop-specific components |
| `src/routes/mobile/` | Mobile route handlers |
| `src/routes/desktop/` | Desktop route handlers |
| `src/lib/utils.ts` | `cn()` utility function |
| `components.json` | shadcn-svelte configuration |

---

## Version

- **SvelteKit**: 2.x
- **Svelte**: 5 (Runes)
- **TailwindCSS**: v4 (CSS-first)
- **UI Library**: shadcn-svelte / bits-ui
- **Icons**: Lucide Svelte
