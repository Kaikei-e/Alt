# DESIGN_LANGUAGE.md - Alt Design System
*Version 2.0 - Theme Toggle System*

## Core Philosophy
Alt combines **Glassmorphism** with two distinct aesthetics: **Vaporwave** (neon retro-future) and **Liquid Glass Beige** (organic luxury). Toggle between them like dark/light mode.

**Key Principles:**
- Glass creates structure, not decoration
- Theme-specific accent colors
- Minimal animations (hover effects only)
- Simplicity over complexity

---

## Theme Toggle System

### Two Variants (Toggle like Dark Mode)
```javascript
// Theme toggle implementation
function toggleTheme() {
  const current = document.body.getAttribute('data-style') || 'vaporwave';
  const next = current === 'vaporwave' ? 'liquid-beige' : 'vaporwave';
  document.body.setAttribute('data-style', next);
}
```

### Theme Specifications

| Theme | Background | Glass | Accent Colors |
|-------|------------|-------|---------------|
| **vaporwave** | Gradient (#1a1a2e→#0f0f23) | rgba(255,255,255,0.1) | Pink #ff006e / Purple #8338ec / Blue #3a86ff |
| **liquid-beige** | Warm beige (#D1C0A8) | rgba(225,213,197,0.18) | Terracotta #B85450 / Sage #7C9070 / Sand #D4A574 |

### CSS Variables (Auto-switch with theme)
```css
/* These variables change automatically when theme toggles */
--surface-bg      /* Glass background */
--surface-border  /* Glass border */
--surface-blur    /* Blur intensity */
--accent-primary  /* Primary action color */
--accent-secondary /* Secondary action color */
--accent-tertiary /* Information color */
--app-bg         /* Application background */
```

### Theme-Specific Colors
```css
/* Vaporwave Palette */
[data-style="vaporwave"] {
  --accent-primary: #ff006e;   /* Neon Pink */
  --accent-secondary: #8338ec; /* Purple */
  --accent-tertiary: #3a86ff;  /* Blue */
}

/* Liquid-Beige Palette (2025 Trend: Earthy Luxury) */
[data-style="liquid-beige"] {
  --accent-primary: #B85450;   /* Terracotta - warm CTAs */
  --accent-secondary: #7C9070; /* Sage Green - organic balance */
  --accent-tertiary: #D4A574;  /* Desert Sand - soft information */
}
```

---

## Implementation Guide

### Essential Alt Theme (Chakra UI)
```typescript
export const altTheme = extendTheme({
  components: {
    Button: {
      variants: {
        alt: {
          bg: 'var(--surface-bg)',
          backdropFilter: 'blur(var(--surface-blur))',
          border: '1px solid var(--surface-border)',
          _hover: {
            borderColor: '#ff006e',
            transform: 'translateY(-2px)'
          }
        }
      }
    }
  }
})
```

### Component Examples

#### Glass Card (Theme-aware)
```tsx
<Card
  bg="var(--surface-bg)"
  backdropFilter="blur(var(--surface-blur))"
  border="1px solid var(--surface-border)"
  borderRadius="12px"
>
  <CardBody>Content adapts to theme</CardBody>
</Card>
```

#### Theme Toggle Button
```tsx
<IconButton
  icon={<SunIcon />}
  onClick={toggleTheme}
  variant="alt"
  aria-label="Toggle theme"
/>
```

#### Theme-Aware Accent Button
```tsx
<Button
  variant="alt"
  _hover={{
    borderColor: 'var(--accent-primary)',
    boxShadow: '0 0 10px var(--accent-primary)33' // 20% opacity
  }}
>
  Primary Action
</Button>
```

---

## Claude Code Guidelines

### Prompt Examples (Max 25 words)
```bash
# ✅ Good
claude "Chakra Card with theme-aware glass effect"
claude "Toggle button switching between three themes"

# ❌ Too complex
claude "Create comprehensive theme system with animations..."
```

### Key Rules
1. Always use CSS variables for theme flexibility
2. Use Chakra UI components as base
3. Apply glass effects to all surfaces
4. Use theme-appropriate accent colors
5. Keep animations subtle (transform/opacity only)

### Migration Checklist
```css
/* Old → New */
bg="rgba(255,255,255,0.1)" → bg="var(--surface-bg)"
backdropFilter="blur(20px)" → backdropFilter="blur(var(--surface-blur))"
borderColor="rgba(255,255,255,0.2)" → borderColor="var(--surface-border)"
```

---

## Quick Reference

### Setting Theme
```html
<body data-style="vaporwave">    <!-- Neon retro-future -->
<body data-style="liquid-beige"> <!-- Earthy luxury -->
```

### Theme Colors
```css
/* Vaporwave Mode */
--accent-primary: #ff006e;   /* Neon Pink */
--accent-secondary: #8338ec; /* Purple */
--accent-tertiary: #3a86ff;  /* Blue */

/* Liquid-Beige Mode */
--accent-primary: #B85450;   /* Terracotta */
--accent-secondary: #7C9070; /* Sage Green */
--accent-tertiary: #D4A574;  /* Desert Sand */
```

### Quality Checklist
- [ ] Uses CSS variables for theming
- [ ] Glass effect on all surfaces
- [ ] Works in both themes
- [ ] Theme-appropriate accent colors
- [ ] Subtle hover animations only

---

**Remember:** Toggle between Vaporwave (neon cyber aesthetic) and Liquid-Beige (earthy luxury trend). Each theme has its own accent palette that reflects 2025 design trends.