import { createSystem, defaultConfig, defineConfig } from "@chakra-ui/react";

// Alt Vaporwave Design System Theme (TASK.md + DESIGN_LANGUAGE.md v1.0)
// Complete implementation of vaporwave glass morphism aesthetic
const altConfig = defineConfig({
  theme: {
    tokens: {
      colors: {
        // Core Brand Colors (DESIGN_LANGUAGE.md Primary Palette)
        alt: {
          pink: { value: "#ff006e" },     // Primary accent - CTAs, highlights
          purple: { value: "#8338ec" },   // Secondary accent - links, states
          blue: { value: "#3a86ff" },     // Tertiary - info, complements
        },

        // Backgrounds - Deep Space (DESIGN_LANGUAGE.md)
        bg: {
          primary: { value: "#1a1a2e" },   // Deep space base
          secondary: { value: "#16213e" }, // Elevated surfaces
          tertiary: { value: "#0f0f23" },  // Recessed areas
        },

        // Glass Effects - Enhanced for 2025 (DESIGN_LANGUAGE.md)
        glass: {
          base: { value: "rgba(255, 255, 255, 0.1)" },     // --alt-glass
          border: { value: "rgba(255, 255, 255, 0.2)" },   // --alt-glass-border
          hover: { value: "rgba(255, 255, 255, 0.15)" },   // --alt-glass-hover
          active: { value: "rgba(255, 255, 255, 0.05)" },  // --alt-glass-active
        },

        // Status with Vaporwave Touch (DESIGN_LANGUAGE.md)
        semantic: {
          success: { value: "#00ff88" },   // Neon green
          error: { value: "#ff4757" },     // Neon red
          warning: { value: "#ffaa00" },   // Neon orange
          info: { value: "#3a86ff" },      // Alt blue
        },

        // Text Hierarchy (DESIGN_LANGUAGE.md)
        text: {
          primary: { value: "#ffffff" },                    // --alt-text-primary
          secondary: { value: "rgba(255, 255, 255, 0.8)" }, // --alt-text-secondary
          muted: { value: "rgba(255, 255, 255, 0.6)" },     // --alt-text-muted
          glow: { value: "#ff006e" },                       // --alt-text-glow
        }
      },

      // Modern variable fonts for performance (DESIGN_LANGUAGE.md)
      // fonts: {
      //   heading: { value: "var(--font-space-grotesk), 'Space Grotesk Variable', system-ui, sans-serif" },
      //   body: { value: "var(--font-inter), 'Inter Variable', system-ui, sans-serif" },
      //   mono: { value: "var(--font-fira-code), 'Fira Code Variable', ui-monospace, monospace" },
      // },

      // Fluid typography scale (DESIGN_LANGUAGE.md)
      fontSizes: {
        xs: { value: "clamp(0.75rem, 0.7rem + 0.25vw, 0.875rem)" },    // --text-xs
        sm: { value: "clamp(0.875rem, 0.8rem + 0.375vw, 1rem)" },      // --text-sm
        md: { value: "clamp(1rem, 0.925rem + 0.375vw, 1.125rem)" },    // --text-base
        lg: { value: "clamp(1.125rem, 1rem + 0.625vw, 1.375rem)" },    // --text-lg
        xl: { value: "clamp(1.25rem, 1.1rem + 0.75vw, 1.625rem)" },    // --text-xl
        "2xl": { value: "clamp(1.5rem, 1.3rem + 1vw, 2rem)" },         // --text-2xl
        "3xl": { value: "clamp(1.875rem, 1.6rem + 1.375vw, 2.625rem)" }, // --text-3xl
        "4xl": { value: "clamp(2.25rem, 1.9rem + 1.75vw, 3.375rem)" }, // --text-4xl
      },

      // Mathematical spacing for visual harmony (DESIGN_LANGUAGE.md Fibonacci + Golden Ratio)
      spacing: {
        1: { value: "0.25rem" },  // 4px
        2: { value: "0.5rem" },   // 8px
        3: { value: "0.75rem" },  // 12px
        4: { value: "1rem" },     // 16px
        5: { value: "1.25rem" },  // 20px
        6: { value: "1.5rem" },   // 24px
        8: { value: "2rem" },     // 32px
        10: { value: "2.5rem" },  // 40px
        12: { value: "3rem" },    // 48px
        16: { value: "4rem" },    // 64px
        20: { value: "5rem" },    // 80px
      },

      // Organic Shapes (DESIGN_LANGUAGE.md)
      radii: {
        xs: { value: "0.25rem" },   // 4px
        sm: { value: "0.5rem" },    // 8px
        md: { value: "0.75rem" },   // 12px
        lg: { value: "1rem" },      // 16px
        xl: { value: "1.5rem" },    // 24px
        "2xl": { value: "2rem" },   // 32px
        full: { value: "9999px" },  // Pill shape
      },

      // Signature Gradients (DESIGN_LANGUAGE.md Vaporwave Heritage)
      gradients: {
        vaporwave: {
          value: "linear-gradient(45deg, {colors.alt.pink}, {colors.alt.purple}, {colors.alt.blue})"
        },
        button: {
          value: "linear-gradient(45deg, {colors.alt.pink}, {colors.alt.purple})"
        },
        background: {
          value: "linear-gradient(135deg, {colors.bg.primary} 0%, {colors.bg.secondary} 50%, {colors.bg.tertiary} 100%)"
        },
        glow: {
          value: "radial-gradient(circle, rgba(255, 0, 110, 0.3), transparent)"
        },
        aurora: {
          value: "linear-gradient(135deg, {colors.alt.pink}, {colors.alt.purple}, {colors.alt.blue}, #00d4ff)"
        }
      }
    },

    semanticTokens: {
      colors: {
        // Background semantic tokens (DESIGN_LANGUAGE.md)
        bg: {
          canvas: { value: "{colors.bg.primary}" },       // Deep space base
          subtle: { value: "{colors.glass.base}" },       // Glass surfaces
          muted: { value: "{colors.bg.secondary}" },      // Elevated surfaces
          surface: { value: "{colors.bg.tertiary}" },     // Recessed areas
        },

        // Foreground semantic tokens (DESIGN_LANGUAGE.md)
        fg: {
          default: { value: "{colors.text.primary}" },    // Primary text
          muted: { value: "{colors.text.secondary}" },    // Secondary text
          subtle: { value: "{colors.text.muted}" },       // Muted text
          glow: { value: "{colors.text.glow}" },          // Accent text
        },

        // Accent semantic tokens (DESIGN_LANGUAGE.md)
        accent: {
          default: { value: "{colors.alt.pink}" },        // Primary accent
          emphasized: { value: "{colors.alt.purple}" },   // Secondary accent
          subtle: { value: "{colors.alt.blue}" },         // Tertiary accent
          fg: { value: "{colors.text.primary}" },         // Accent foreground
        },

        // Border semantic tokens (DESIGN_LANGUAGE.md)
        border: {
          default: { value: "{colors.glass.border}" },    // Glass borders
          subtle: { value: "{colors.glass.base}" },       // Subtle borders
          hover: { value: "{colors.glass.hover}" },       // Hover borders
          active: { value: "{colors.glass.active}" },     // Active borders
        },

        // Semantic status colors (DESIGN_LANGUAGE.md)
        semantic: {
          success: { value: "{colors.semantic.success}" },
          error: { value: "{colors.semantic.error}" },
          warning: { value: "{colors.semantic.warning}" },
          info: { value: "{colors.semantic.info}" },
        }
      }
    }
  },

  // Global CSS for vaporwave glass aesthetic (DESIGN_LANGUAGE.md)
  globalCss: {
    // Base body styles
    body: {
      bg: "bg.canvas",
      color: "fg.default",
      fontFamily: "body",
      background: "{gradients.background}",
      minHeight: "100dvh", // Dynamic viewport height for mobile
      backgroundAttachment: "fixed",
      fontSize: "md",
      lineHeight: "1.6",
    },

    // Glass morphism base class
    ".glass": {
      background: "bg.subtle",
      backdropFilter: "blur(16px) saturate(1.2)",
      border: "1px solid",
      borderColor: "border.default",
      boxShadow: "0 8px 32px rgba(0, 0, 0, 0.1), inset 0 1px 0 rgba(255, 255, 255, 0.1)",
    },

    // Energy-efficient hover effects (DESIGN_LANGUAGE.md)
    ".glass:hover": {
      background: "border.hover",
      borderColor: "border.hover",
      transition: "all 0.2s cubic-bezier(0.4, 0, 0.2, 1)",
    },

    // Touch-friendly interactions (DESIGN_LANGUAGE.md)
    ".touch-target": {
      minHeight: "44px",
      minWidth: "44px",
      touchAction: "manipulation",
    }
  }
});

export const altSystem = createSystem(defaultConfig, altConfig);