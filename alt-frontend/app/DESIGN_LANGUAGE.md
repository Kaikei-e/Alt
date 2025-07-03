# DESIGN_LANGUAGE.md - Alt Design System
*Version 2.0 - Glassmorphism × Theme Toggle*

## Core Philosophy
**Alt** = Glassmorphism with dual personality. Toggle between **Vaporwave** (neon cyber) and **Liquid-Beige** (earthy luxury) themes.

### Design Principles
1. **Glass surfaces** - Every UI element uses glassmorphism
2. **Theme-aware colors** - Colors swap based on active theme
3. **Minimal motion** - Only subtle hover effects (translateY, scale)
4. **Clean typography** - Inter/Space Grotesk fonts

---

## Theme System

### Toggle Implementation
```javascript
// Simple theme toggle
function toggleTheme() {
  const current = document.body.getAttribute('data-style') || 'vaporwave';
  document.body.setAttribute('data-style',
    current === 'vaporwave' ? 'liquid-beige' : 'vaporwave'
  );
}
```

### Theme Specifications

| Aspect | Vaporwave | Liquid-Beige |
|--------|-----------|--------------|
| **Background** | `#1a1a2e → #0f0f23` gradient | `#E8DED1 → #C8B8A1` gradient |
| **Glass** | `rgba(255,255,255,0.1)` | `rgba(255,255,255,0.08)` |
| **Border** | `rgba(255,255,255,0.2)` | `rgba(160,129,108,0.25)` |
| **Text** | White on dark | Dark `#2c2416` on light |
| **Primary** | `#ff006e` (Pink) | `#B85450` (Terracotta) |
| **Secondary** | `#8338ec` (Purple) | `#7C9070` (Sage) |
| **Tertiary** | `#3a86ff` (Blue) | `#D4A574` (Sand) |

### Swappable Variables
```css
/* These auto-switch with theme */
--alt-primary    /* Pink ↔ Terracotta */
--alt-secondary  /* Purple ↔ Sage */
--alt-tertiary   /* Blue ↔ Sand */
```

---

## Component Patterns

### Glass Base
```css
.glass {
  background: var(--surface-bg);
  border: 1px solid var(--surface-border);
  backdrop-filter: blur(var(--surface-blur)) saturate(120%);
  border-radius: var(--radius-lg);
  transition: all 0.2s ease;
}
```

### Button Variants
```css
/* Glass button */
.btn-primary {
  background: var(--surface-bg);
  backdrop-filter: blur(var(--surface-blur));
  border: 1px solid var(--surface-border);
}

/* Gradient accent */
.btn-accent {
  background: var(--accent-gradient);
  /* Auto-adjusts text color per theme */
}
```

### Hover States
```css
/* Consistent hover pattern */
:hover {
  transform: translateY(-2px);
  /* Optional: border-color: var(--alt-primary); */
}
```

---

## CSS Variable Reference

### Core Colors
```css
/* Static brand colors */
--alt-pink: #ff006e;
--alt-purple: #8338ec;
--alt-blue: #3a86ff;
--alt-terracotta: #B85450;
--alt-sage: #7C9070;
--alt-sand: #D4A574;

/* Theme-aware (use these) */
--alt-primary
--alt-secondary
--alt-tertiary
```

### Surface Properties
```css
--surface-bg      /* Glass background */
--surface-border  /* Glass border */
--surface-hover   /* Hover state */
--surface-blur    /* Blur amount (16-20px) */
```

### Typography Scale
```css
--text-xs: clamp(0.75rem, 0.7rem + 0.25vw, 0.875rem);
--text-sm: clamp(0.875rem, 0.8rem + 0.375vw, 1rem);
--text-base: clamp(1rem, 0.925rem + 0.375vw, 1.125rem);
--text-lg: clamp(1.125rem, 1rem + 0.625vw, 1.375rem);
--text-xl: clamp(1.25rem, 1.1rem + 0.75vw, 1.625rem);
```

### Spacing (Fibonacci)
```css
--space-1: 0.25rem;  /* 4px */
--space-2: 0.5rem;   /* 8px */
--space-3: 0.75rem;  /* 12px */
--space-4: 1rem;     /* 16px */
--space-6: 1.5rem;   /* 24px */
--space-8: 2rem;     /* 32px */
```

---

## Implementation Guide

### Chakra UI Integration
```typescript
// theme.ts
export const altTheme = extendTheme({
  styles: {
    global: {
      body: {
        bg: 'var(--app-bg)',
        color: 'var(--foreground)',
      }
    }
  },
  components: {
    Button: {
      baseStyle: {
        transition: 'all 0.2s',
        _hover: {
          transform: 'translateY(-2px)',
        }
      },
      variants: {
        glass: {
          bg: 'var(--surface-bg)',
          border: '1px solid',
          borderColor: 'var(--surface-border)',
          backdropFilter: 'blur(var(--surface-blur))',
          _hover: {
            bg: 'var(--surface-hover)',
            borderColor: 'var(--alt-primary)',
          }
        }
      }
    }
  }
})
```

### Component Examples
```tsx
// Glass Card
<Box
  className="glass"
  p="var(--space-6)"
  borderRadius="var(--radius-lg)"
>
  <Text color="var(--text-primary)">Content</Text>
</Box>

// Theme Toggle
<IconButton
  className="theme-toggle"
  onClick={toggleTheme}
  aria-label="Toggle theme"
  icon={<SunIcon />}
/>

// Accent Button
<Button
  className="btn-accent"
  bg="var(--accent-gradient)"
>
  Action
</Button>
```

---

## Quality Checklist

- [ ] All surfaces use `.glass` or glass properties
- [ ] Colors use `--alt-primary/secondary/tertiary`
- [ ] Hover effects limited to `translateY(-2px)`
- [ ] Text uses `--text-primary/secondary/muted`
- [ ] Spacing uses `--space-*` variables
- [ ] Works in both themes without hardcoded colors

---

## Quick Reference

```css
/* Theme toggle */
data-style="vaporwave"     /* Neon cyber */
data-style="liquid-beige"  /* Earthy luxury */

/* Use these everywhere */
var(--alt-primary)         /* Theme-aware accent */
var(--surface-bg)          /* Glass background */
var(--text-primary)        /* Main text color */
var(--space-4)            /* Consistent spacing */
```

**Remember**: Let CSS variables handle theme switching. Never hardcode colors.