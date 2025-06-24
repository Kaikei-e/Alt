# DESIGN_LANGUAGE.md - Alt Vaporwave Glass Design System
*Version 1.0 - June 2025*

## Design Philosophy

### Core Vision
Alt represents the harmony between **Glassmorphism** and **Vaporwave** - where transparency meets neon, and minimalism embraces retro-futurism. Our design language prioritizes **simplicity** over complexity, **purpose** over decoration.

> "Less is more neon. Simplicity glows brightest."

### Fundamental Principles

#### 1. Restrained Neon
Vaporwave colors demand attention - use them sparingly. Pink for primary actions, purple for secondary states, blue for information. Transparency provides the breathing room.

#### 2. Glass as Structure
Glass effects create depth without weight. Every layer serves hierarchy, not decoration.

#### 3. Motion with Purpose
Animation communicates state changes, never entertains. Prefer subtle transforms over complex sequences.

#### 4. Elegant Simplicity
The most sophisticated interface feels effortless. Remove the unnecessary to reveal the essential.

---

## Claude Code Guidelines (Concise)

### Core Requirements
1. **Chakra UI foundation** - Use Chakra components, extend with Alt styling
2. **Glass-first design** - Every surface uses glassmorphism
3. **Purposeful color** - Pink/purple/blue for specific functions only
4. **Minimal animation** - Subtle hover effects, no complex sequences

### Prompt Structure (Under 30 words)
```bash
# ✅ Effective prompt
claude "Chakra Button + Alt glass effect + pink hover glow"

# ❌ Too complex
claude "Create a comprehensive button system with multiple variants..."
```

### Essential Alt Theme
```typescript
// alt-theme.ts (Chakra UI)
export const altTheme = extendTheme({
  colors: {
    alt: {
      pink: '#ff006e',
      purple: '#8338ec',
      blue: '#3a86ff',
      glass: 'rgba(255, 255, 255, 0.1)',
    }
  },
  components: {
    Button: {
      variants: {
        alt: {
          bg: 'alt.glass',
          backdropFilter: 'blur(16px)',
          border: '1px solid rgba(255, 255, 255, 0.2)',
          _hover: { borderColor: 'alt.pink' }
        }
      }
    }
  }
})
```

### Quick Implementation Patterns

#### Glass Card
```tsx
// ✅ Simple Alt card
<Card bg="rgba(255,255,255,0.1)" backdropFilter="blur(16px)">
  <CardBody>Content</CardBody>
</Card>
```

#### Neon Button
```tsx
// ✅ Alt button with purpose
<Button
  variant="alt"
  _hover={{ boxShadow: '0 0 20px rgba(255,0,110,0.3)' }}
>
  Action
</Button>
```

---

## Visual Foundation

### Color Philosophy
Colors have meaning. Use intentionally.

```css
/* Primary Colors - Use Sparingly */
--alt-pink: #ff006e;     /* Primary CTAs only */
--alt-purple: #8338ec;   /* Secondary actions */
--alt-blue: #3a86ff;     /* Information states */

/* Glass Foundation */
--alt-glass: rgba(255, 255, 255, 0.1);
--alt-glass-border: rgba(255, 255, 255, 0.2);

/* Backgrounds - Deep & Minimal */
--alt-bg: #1a1a2e;
```

### Typography - Clarity First
```css
/* Clean, readable fonts */
--font-primary: 'Geist', system-ui;
--font-display: 'Geist', system-ui;

/* Simple scale */
--text-sm: 0.875rem;
--text-base: 1rem;
--text-lg: 1.125rem;
--text-xl: 1.25rem;
```

### Glass Effects - The Core
```css
/* Standard glass surface */
.glass {
  background: rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(16px);
  border: 1px solid rgba(255, 255, 255, 0.2);
  border-radius: 12px;
}

/* Hover enhancement - subtle only */
.glass:hover {
  background: rgba(255, 255, 255, 0.15);
  border-color: rgba(255, 255, 255, 0.3);
}
```

---

## Motion Philosophy

### Less is More
Animations serve function, not aesthetics.

#### Allowed Animations
- **Hover feedback**: Subtle color/border changes
- **State transitions**: Opacity, transform (small)
- **Loading states**: Simple progress indicators

#### Prohibited Animations
- Complex sequences
- Bouncing effects
- Rotating elements (unless functional)
- Multiple simultaneous animations

```css
/* ✅ Good - Purposeful hover */
.interactive:hover {
  transform: translateY(-1px);
  transition: transform 0.15s ease;
}

/* ❌ Bad - Excessive animation */
.flashy {
  animation: bounce 0.5s infinite;
  transform: rotate(360deg);
}
```

---

## Component Principles

### Simplicity Guidelines

#### Button Design
```tsx
// ✅ Alt button - clean and purposeful
<Button
  bg="rgba(255,255,255,0.1)"
  backdropFilter="blur(16px)"
  border="1px solid rgba(255,255,255,0.2)"
  _hover={{
    borderColor: '#ff006e',
    boxShadow: '0 0 10px rgba(255,0,110,0.2)'
  }}
>
  Simple Action
</Button>
```

#### Card Design
```tsx
// ✅ Glass card - content focused
<Card
  bg="rgba(255,255,255,0.1)"
  backdropFilter="blur(16px)"
  border="1px solid rgba(255,255,255,0.2)"
>
  <CardBody>
    <Text>Clear, readable content</Text>
  </CardBody>
</Card>
```

### Quality Control

#### Essential Checklist
- [ ] Uses Chakra UI components
- [ ] Glass effect present (backdrop-filter: blur)
- [ ] Minimal color usage (pink/purple/blue)
- [ ] Subtle hover only
- [ ] Clean, readable text

#### Avoid These Patterns
```typescript
// ❌ Don't create custom components unnecessarily
const FlashyComponent = () => <div className="complex-animation">...</div>

// ✅ Use Chakra + Alt styling
<Box bg="alt.glass" backdropFilter="blur(16px)">...</Box>
```

---

## Team Standards

### Claude Code Best Practices

#### Context Management
```bash
# Clear frequently
claude /clear

# Provide minimal context
echo "Chakra UI + Alt glass theme" > .clauderc
```

#### Effective Prompts
```bash
# ✅ Focused (under 25 words)
claude "Chakra Card with glass background and subtle pink border hover"

# ✅ Specific variant
claude "Button variant='alt' with neon glow effect"

# ❌ Too complex
claude "Create a comprehensive design system with multiple variants..."
```

#### Quality Gates
```bash
# Quick verification
pnpm run typecheck
pnpm run lint

# Test glass effects
claude "Add backdrop-filter blur to this component"
```

---


## Key Reminders

### Design Philosophy
- **Simplicity over complexity**
- **Purpose over decoration**
- **Glass creates structure**
- **Neon provides accent**

### Claude Code Usage
- **Keep prompts under 30 words**
- **Use Chakra UI as foundation**
- **Apply Alt styling consistently**
- **Avoid complex animations**

### Quality Standards
- Glass effects on all surfaces
- Minimal color usage
- Subtle hover feedback only
- Clean, readable typography

---

*Alt Design System: Where glass meets neon, simplicity glows brightest.*