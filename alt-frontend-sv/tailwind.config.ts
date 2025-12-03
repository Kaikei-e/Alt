import type { Config } from "tailwindcss";
import defaultTheme from "tailwindcss/defaultTheme";

const config: Config = {
	theme: {
		container: {
			center: true,
			padding: "2rem",
			screens: {
				"2xl": "1400px",
			},
		},
		extend: {
			colors: {
				border: "hsl(var(--border) / <alpha-value>)",
				input: "hsl(var(--input) / <alpha-value>)",
				ring: "hsl(var(--ring) / <alpha-value>)",
				background: "hsl(var(--background) / <alpha-value>)",
				foreground: "hsl(var(--foreground) / <alpha-value>)",
				primary: {
					DEFAULT: "hsl(var(--primary) / <alpha-value>)",
					foreground: "hsl(var(--primary-foreground) / <alpha-value>)",
				},
				secondary: {
					DEFAULT: "hsl(var(--secondary) / <alpha-value>)",
					foreground: "hsl(var(--secondary-foreground) / <alpha-value>)",
				},
				destructive: {
					DEFAULT: "hsl(var(--destructive) / <alpha-value>)",
					foreground: "hsl(var(--destructive-foreground) / <alpha-value>)",
				},
				muted: {
					DEFAULT: "hsl(var(--muted) / <alpha-value>)",
					foreground: "hsl(var(--muted-foreground) / <alpha-value>)",
				},
				accent: {
					DEFAULT: "hsl(var(--accent) / <alpha-value>)",
					foreground: "hsl(var(--accent-foreground) / <alpha-value>)",
				},
				popover: {
					DEFAULT: "hsl(var(--popover) / <alpha-value>)",
					foreground: "hsl(var(--popover-foreground) / <alpha-value>)",
				},
				card: {
					DEFAULT: "hsl(var(--card) / <alpha-value>)",
					foreground: "hsl(var(--card-foreground) / <alpha-value>)",
				},
				// Custom Theme Colors
				alt: {
					pink: "var(--alt-pink)",
					purple: "var(--alt-purple)",
					blue: "var(--alt-blue)",
					terracotta: "var(--alt-terracotta)",
					sage: "var(--alt-sage)",
					sand: "var(--alt-sand)",
					charcoal: "var(--alt-charcoal)",
					slate: "var(--alt-slate)",
					ash: "var(--alt-ash)",
					primary: "var(--alt-primary)",
					secondary: "var(--alt-secondary)",
					tertiary: "var(--alt-tertiary)",
					success: "var(--alt-success)",
					error: "var(--alt-error)",
					warning: "var(--alt-warning)",
					glass: "var(--alt-glass)",
				},
				// Alt Design System Colors
				"text-primary": "var(--text-primary)",
				"text-secondary": "var(--text-secondary)",
				"text-muted": "var(--text-muted)",
				"surface-bg": "var(--surface-bg)",
				"surface-border": "var(--surface-border)",
				"surface-hover": "var(--surface-hover)",
			},
			borderRadius: {
				lg: "0", // Alt-Paper style: no rounded corners
				md: "0",
				sm: "0",
			},
			boxShadow: {
				sm: "var(--shadow-sm)",
				md: "var(--shadow-md)",
				lg: "var(--shadow-lg)",
			},
			fontFamily: {
				sans: [...defaultTheme.fontFamily.sans],
			},
		},
	},
};

export default config;
