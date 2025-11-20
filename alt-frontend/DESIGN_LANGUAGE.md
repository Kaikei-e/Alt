# Alt Design Language

This document outlines the design principles, tokens, and patterns used in the Alt frontend application. The design system is built to support multiple themes while maintaining a premium, fluid, and responsive user experience.

## 1. Core Philosophy

Alt's design philosophy centers on **"Dynamic Premium"**. It combines modern glassmorphism with fluid animations and distinct thematic personalities.

*   **Glassmorphism**: Extensive use of translucent backgrounds, blurs, and subtle borders to create depth and context.
*   **Fluid Motion**: Interactions are enhanced with `framer-motion` for smooth entry, exit, and gesture-based animations.
*   **Thematic Flexibility**: The system supports three distinct visual languages (Vaporwave, Liquid-Beige, Alt-Paper) that completely transform the app's mood.

## 2. Themes

The application supports three primary themes, controlled via the `data-style` attribute on the `<body>` tag. **Alt-Paper is the default theme.**

### A. Alt-Paper (Default)
*   **Vibe**: Newspaper, Instapaper-style, Minimalist.
*   **Palette**: Monochrome, Slate, Charcoal.
*   **Key Characteristics**:
    *   **No Glassmorphism**: Solid backgrounds.
    *   **No Border Radius**: Sharp edges (0px radius).
    *   **Serif Fonts**: Uses Georgia/Times New Roman.

### B. Vaporwave
*   **Vibe**: Neon Cyber, Dark Mode, High Contrast.
*   **Palette**: Deep blues/purples with neon pink/purple accents.
*   **Key Colors**:
    *   Primary: `var(--alt-pink)` (#ff006e)
    *   Secondary: `var(--alt-purple)` (#8338ec)
    *   Background: Linear gradient (Deep Blue/Purple)

### C. Liquid-Beige
*   **Vibe**: Earthy Luxury, Warm, Organic.
*   **Palette**: Terracotta, Sage, Sand.
*   **Key Colors**:
    *   Primary: `var(--alt-terracotta)` (#b85450)
    *   Secondary: `var(--alt-sage)` (#7c9070)
    *   Background: Warm beige gradients.



## 3. Design Tokens

### Colors
The system uses semantic variable names that map to theme-specific values.

| Token | Description |
| :--- | :--- |
| `--alt-primary` | Main brand color (Theme dependent) |
| `--alt-secondary` | Secondary accent color |
| `--app-bg` | Main application background (often a gradient) |
| `--surface-bg` | Background for cards and glass elements |
| `--text-primary` | High-emphasis text |
| `--text-secondary` | Medium-emphasis text |

### Typography
Fluid typography scales using `clamp()` for responsiveness.

*   **Primary Font**: `Inter Variable` (Sans-serif)
*   **Display Font**: `Space Grotesk Variable`
*   **Mono Font**: `Fira Code Variable`
*   **Serif Font**: `Georgia` (Used in Alt-Paper theme)

### Spacing
Spacing follows a Fibonacci-inspired scale:
`--space-1` (0.25rem) to `--space-20` (5rem).

### Radius
*   `--radius-lg` (1rem): Standard card radius.
*   `--radius-full` (9999px): Buttons and pills.
*   **Note**: All radii become `0` in the **Alt-Paper** theme.

## 4. UI Components & Patterns

### Glassmorphism (The `.glass` class)
The core building block for UI containers.
```css
.glass {
  background: var(--surface-bg);
  border: 1px solid var(--surface-border);
  backdrop-filter: blur(var(--surface-blur)) saturate(120%);
}
```

### Feed Cards (Viewed & Swipe)
Cards are the primary content containers.

*   **Style**:
    *   `borderRadius="1rem"`
    *   `bg="var(--alt-glass)"`
    *   `backdropFilter="blur(20px)"`
    *   `border="2px solid var(--alt-glass-border)"`
    *   `boxShadow`: Heavy shadows for depth (`0 12px 40px rgba(0, 0, 0, 0.3)`).
*   **Interactions**:
    *   Hover: Lift up (`translateY(-2px)`), increased shadow, brighter border.
    *   Gestures: Swipeable cards use `framer-motion` for drag interactions (`x`, `rotate`, `opacity`).

### Buttons
*   **Primary**: Glass background, pill shape (`rounded-full`), subtle border.
*   **Accent**: Gradient background (`--accent-gradient`), white text, no border.
*   **Hover Effects**: All interactive elements lift up and increase brightness/shadow on hover.

## 5. Animation Guidelines

Animations should be smooth and physics-based using `framer-motion`.

*   **Entry**: `initial={{ opacity: 0, y: 20 }}` -> `animate={{ opacity: 1, y: 0 }}`
*   **Exit**: `exit={{ opacity: 0, y: -20 }}`
*   **Swipe**: Use spring physics for natural drag-and-release feel.
*   **Reduced Motion**: Always respect `prefers-reduced-motion` by disabling or simplifying animations.

## 6. Implementation Example (React/Chakra)

```tsx
<Box
  className="glass"
  p={4}
  borderRadius="lg"
  _hover={{
    transform: "translateY(-2px)",
    boxShadow: "lg",
    borderColor: "var(--alt-primary)"
  }}
  transition="all 0.2s ease"
>
  <Text color="var(--text-primary)" fontSize="lg" fontWeight="bold">
    Content Title
  </Text>
</Box>
```
